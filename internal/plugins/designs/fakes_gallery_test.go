package designs

import (
	"context"
	"strings"
	"time"
)

// memGallery is an in-memory GalleryRepository with the real one's visible
// behaviour: Browse shows visible entries only, newest first.
type memGallery struct {
	entries map[string]*GalleryEntry
	order   []string
	reports map[string][]GalleryReport
	events  map[string][]time.Time
	clock   func() time.Time
}

func newMemGallery() *memGallery {
	return &memGallery{entries: map[string]*GalleryEntry{}, reports: map[string][]GalleryReport{}, events: map[string][]time.Time{}, clock: time.Now}
}

func cloneEntry(e *GalleryEntry) *GalleryEntry {
	c := *e
	c.Data = append([]byte(nil), e.Data...)
	c.Reasons = append([]string{}, e.Reasons...)
	return &c
}

func (m *memGallery) Insert(_ context.Context, e *GalleryEntry) error {
	c := cloneEntry(e)
	c.PublishedAt = m.clock()
	m.entries[e.ID] = c
	m.order = append(m.order, e.ID)
	return nil
}

func (m *memGallery) Get(_ context.Context, id string) (*GalleryEntry, error) {
	if e := m.entries[id]; e != nil {
		return cloneEntry(e), nil
	}
	return nil, nil
}

func (m *memGallery) BySource(_ context.Context, publisher, src string) (*GalleryEntry, error) {
	for _, id := range m.order {
		if e := m.entries[id]; e != nil && e.PublisherUserID == publisher && e.SourceDesignID == src {
			return cloneEntry(e), nil
		}
	}
	return nil, nil
}

func (m *memGallery) Save(_ context.Context, e *GalleryEntry) error {
	pub := m.entries[e.ID].PublishedAt
	c := cloneEntry(e)
	c.PublishedAt = pub
	m.entries[e.ID] = c
	return nil
}

func (m *memGallery) Delete(_ context.Context, id string) error {
	delete(m.entries, id)
	delete(m.reports, id)
	return nil
}

func (m *memGallery) pick(ok func(*GalleryEntry) bool) []GalleryEntry {
	var out []GalleryEntry
	for i := len(m.order) - 1; i >= 0; i-- {
		if e := m.entries[m.order[i]]; e != nil && ok(e) {
			out = append(out, *cloneEntry(e))
		}
	}
	return out
}

func page(list []GalleryEntry, limit, offset int) ([]GalleryEntry, int) {
	total := len(list)
	if limit <= 0 {
		return list, total
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return list[offset:end], total
}

func (m *memGallery) Browse(_ context.Context, q BrowseQuery) ([]GalleryEntry, int, error) {
	list := m.pick(func(e *GalleryEntry) bool {
		if e.Status != StatusVisible || (!q.AnySys && e.SystemID != q.SystemID) {
			return false
		}
		if len(q.Kinds) > 0 && !contains(q.Kinds, e.Kind) {
			return false
		}
		if q.Prefix != "" && !strings.HasPrefix(e.Kind, q.Prefix) {
			return false
		}
		t := strings.ToLower(q.Text)
		return t == "" || strings.Contains(strings.ToLower(e.Name), t) || strings.Contains(strings.ToLower(e.MakerName), t)
	})
	out, total := page(list, q.Limit, q.Offset)
	return out, total, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (m *memGallery) ByPublisher(_ context.Context, p string) ([]GalleryEntry, error) {
	return m.pick(func(e *GalleryEntry) bool { return e.PublisherUserID == p }), nil
}

func (m *memGallery) ByStatus(_ context.Context, status string, limit, offset int) ([]GalleryEntry, int, error) {
	out, total := page(m.pick(func(e *GalleryEntry) bool { return e.Status == status }), limit, offset)
	return out, total, nil
}

func (m *memGallery) DuplicateExists(_ context.Context, p, hash, except string) (bool, error) {
	return len(m.pick(func(e *GalleryEntry) bool {
		return e.PublisherUserID == p && e.DataHash == hash && e.ID != except && e.Status != StatusRemoved
	})) > 0, nil
}

func (m *memGallery) CountEvents(_ context.Context, u string, since time.Time) (int, error) {
	n := 0
	for _, t := range m.events[u] {
		if !t.Before(since) {
			n++
		}
	}
	return n, nil
}

func (m *memGallery) AddEvent(_ context.Context, u string) error {
	m.events[u] = append(m.events[u], m.clock())
	return nil
}

func (m *memGallery) AddReport(_ context.Context, id, reporter, reason, note string) (bool, error) {
	for _, r := range m.reports[id] {
		if r.ReporterUserID == reporter {
			return false, nil
		}
	}
	m.reports[id] = append(m.reports[id], GalleryReport{ReporterUserID: reporter, Reason: reason, Note: note, CreatedAt: m.clock()})
	return true, nil
}

func (m *memGallery) Reports(_ context.Context, id string) ([]GalleryReport, error) {
	return append([]GalleryReport{}, m.reports[id]...), nil
}

func (m *memGallery) ReportedBy(_ context.Context, reporter string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		for _, r := range m.reports[id] {
			if r.ReporterUserID == reporter {
				out[id] = true
			}
		}
	}
	return out, nil
}

func (m *memGallery) ClearReports(_ context.Context, id string) error {
	delete(m.reports, id)
	return nil
}
