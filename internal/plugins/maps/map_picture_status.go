package maps

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// pictureStatusTTL bounds how long a stale answer can outlive a write made
	// by another process; writes through this service invalidate at once.
	pictureStatusTTL = 30 * time.Second
	// pictureStatusMaxEntries keeps the cache small; hitting it drops everything
	// rather than tracking recency, since a refill is two cheap queries.
	pictureStatusMaxEntries = 1024
)

// pictureStatus is what the media server needs to know about a file: whether
// any map in the campaign uses it as its picture, and whether one of those maps
// has a shadow.
type pictureStatus struct {
	isMapPicture bool
	shadowed     bool
}

type pictureStatusEntry struct {
	campaignID string
	status     pictureStatus
	at         time.Time
}

// pictureStatusCache answers the media server's per-request question without
// listing maps and shadows each time. It never keeps an answer computed before
// an invalidation: a "not shadowed" entry that survived a shadow write would
// serve the original, so put drops results whose generation is out of date.
type pictureStatusCache struct {
	mu      sync.Mutex
	entries map[string]pictureStatusEntry
	gen     uint64
	now     func() time.Time
}

func newPictureStatusCache() *pictureStatusCache {
	return &pictureStatusCache{entries: map[string]pictureStatusEntry{}, now: time.Now}
}

func pictureStatusKey(campaignID, mediaID string) string { return campaignID + "\x00" + mediaID }

// get returns a live entry if there is one, plus the generation a fresh
// computation must present to put.
func (c *pictureStatusCache) get(campaignID, mediaID string) (pictureStatus, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[pictureStatusKey(campaignID, mediaID)]
	if ok && c.now().Sub(e.at) < pictureStatusTTL {
		return e.status, c.gen, true
	}
	return pictureStatus{}, c.gen, false
}

func (c *pictureStatusCache) put(campaignID, mediaID string, st pictureStatus, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	if len(c.entries) >= pictureStatusMaxEntries {
		c.entries = map[string]pictureStatusEntry{}
	}
	c.entries[pictureStatusKey(campaignID, mediaID)] = pictureStatusEntry{campaignID: campaignID, status: st, at: c.now()}
}

// invalidate forgets a campaign's entries, or every entry when the campaign is
// unknown (a shadow write whose map could not be resolved), so an unresolved
// lookup can only over-invalidate.
func (c *pictureStatusCache) invalidate(campaignID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	if campaignID == "" {
		c.entries = map[string]pictureStatusEntry{}
		return
	}
	for k, e := range c.entries {
		if e.campaignID == campaignID {
			delete(c.entries, k)
		}
	}
}

// pictureStatusOf computes (or reads from cache) the status of a file. On error
// the returned status is the most restrictive, and nothing is cached.
func (s *mapService) pictureStatusOf(ctx context.Context, campaignID, mediaID string) (pictureStatus, error) {
	// The generation is read before the queries so a write that lands while
	// they run keeps this result out of the cache.
	st, gen, ok := s.pictureCache.get(campaignID, mediaID)
	if ok {
		return st, nil
	}
	closed := pictureStatus{isMapPicture: true, shadowed: true}
	list, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return closed, fmt.Errorf("list maps: %w", err)
	}
	for _, m := range list {
		if m.ImageID == nil || *m.ImageID != mediaID {
			continue
		}
		st.isMapPicture = true
		if st.shadowed {
			continue
		}
		areas, err := s.shadowedAreas(ctx, m.ID)
		if err != nil {
			return closed, fmt.Errorf("list shadow areas: %w", err)
		}
		st.shadowed = len(areas) > 0
	}
	s.pictureCache.put(campaignID, mediaID, st, gen)
	return st, nil
}

// IsShadowedMapImage implements the media plugin's MapImageGuard: is the file
// the picture of any map in the campaign that has a shadow. It is true on error
// so a caller that ignores the error still refuses.
func (s *mapService) IsShadowedMapImage(ctx context.Context, campaignID, mediaID string) (bool, error) {
	st, err := s.pictureStatusOf(ctx, campaignID, mediaID)
	return st.shadowed, err
}

// IsMapPicture implements the other half of MapImageGuard: is the file the
// picture of any map in the campaign, shadowed or not. True on error.
func (s *mapService) IsMapPicture(ctx context.Context, campaignID, mediaID string) (bool, error) {
	st, err := s.pictureStatusOf(ctx, campaignID, mediaID)
	return st.isMapPicture, err
}

// InvalidateMapPictures drops the campaign's cached picture answers. Shadow and
// map writes call it so a new shadow takes effect on the next request.
func (s *mapService) InvalidateMapPictures(campaignID string) {
	s.pictureCache.invalidate(campaignID)
}

// shadowChangeNotifier is implemented by the shadow source (the drawing
// service) so the map service can hear about shadow writes.
type shadowChangeNotifier interface {
	SetShadowChangeHook(fn func(campaignID string))
}
