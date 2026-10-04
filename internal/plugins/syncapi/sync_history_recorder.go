package syncapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// historyRecordTimeout bounds the write a request leaves behind; the
// history is best effort and never slows or fails the call itself.
const historyRecordTimeout = 2 * time.Second

// historyResourceKey carries the id and name a handler already holds, so
// the history row needs no second lookup (and names a page it just deleted).
const historyResourceKey = "syncHistoryResource"

// historyActorKey carries the member a call acted as, once the service has
// accepted that member (StashAPI.ActorFor: the key holder is the owner or a
// co-DM and the named user is in the campaign). Set only after success, so
// a module's unchecked claim never names who did something.
const historyActorKey = "syncHistoryActor"

// noteSyncActor tells the history recorder the verified acting member.
func noteSyncActor(c echo.Context, userID string) {
	if userID != "" {
		c.Set(historyActorKey, userID)
	}
}

type historyResource struct {
	id, name, was string
}

// noteSyncResource tells the history recorder what this call touched.
func noteSyncResource(c echo.Context, id, name string) {
	c.Set(historyResourceKey, historyResource{id: id, name: name})
}

// noteSyncChange is noteSyncResource for a change that replaces a value
// people know by name: name is what was asked for, was what it replaced.
// Noted before the call is allowed, so a refused change still says what it
// asked for.
func noteSyncChange(c echo.Context, id, name, was string) {
	c.Set(historyResourceKey, historyResource{id: id, name: name, was: was})
}

// EntityNamer resolves a page's name for a history row when the handler
// didn't note one.
type EntityNamer interface {
	EntityName(ctx context.Context, campaignID, entityID string) (string, bool)
}

// historyRoute says how one route appears in the history. Routes not
// listed here are recorded with a generic kind and action.
type historyRoute struct {
	kind, action string
	skip         bool
}

// historyRoutes is keyed by method and the route template after
// /api/v1/campaigns/:id.
var historyRoutes = map[string]historyRoute{
	"POST /entities":                          {kind: "page", action: "page created"},
	"PUT /entities/:entityID":                 {kind: "page", action: "page updated"},
	"DELETE /entities/:entityID":              {kind: "page", action: "page deleted"},
	"PUT /entities/:entityID/fields":          {kind: "character", action: "fields updated"},
	"PUT /entities/:entityID/permissions":     {kind: "page", action: "access changed"},
	"POST /entities/:entityID/reveal":         {kind: "page", action: "shown to players"},
	"PUT /entities/:entityID/tags":            {kind: "page", action: "tags changed"},
	"POST /entities/:entityID/relations":      {kind: "page", action: "link added"},
	"POST /entities/bulk-update":              {kind: "page", action: "pages updated"},
	"POST /entities/bulk-tags":                {kind: "page", action: "tags changed"},
	"POST /calendar":                          {kind: historyKindCalendar, action: "calendar created"},
	"PUT /calendar/date":                      {kind: historyKindCalendar, action: "date set"},
	"POST /calendar/date/confirm":             {kind: historyKindCalendar, action: "date confirmed"},
	"POST /calendar/events":                   {kind: historyKindCalendar, action: "event created"},
	"PUT /calendar/events/:eventID":           {kind: historyKindCalendar, action: "event updated"},
	"DELETE /calendar/events/:eventID":        {kind: historyKindCalendar, action: "event deleted"},
	"POST /maps/:mapID/markers":               {kind: "map", action: "pin added"},
	"PUT /maps/:mapID/markers/:markerID":      {kind: "map", action: "pin updated"},
	"DELETE /maps/:mapID/markers/:markerID":   {kind: "map", action: "pin removed"},
	"POST /maps/:mapID/drawings":              {kind: "map", action: "drawing added"},
	"PUT /maps/:mapID/drawings/:drawingID":    {kind: "map", action: "drawing updated"},
	"DELETE /maps/:mapID/drawings/:drawingID": {kind: "map", action: "drawing removed"},
	"POST /maps/:mapID/tokens":                {kind: "map", action: "token added"},
	"PUT /maps/:mapID/tokens/:tokenID":        {kind: "map", action: "token updated"},
	"DELETE /maps/:mapID/tokens/:tokenID":     {kind: "map", action: "token removed"},
	"POST /maps/:mapID/fog":                   {kind: "map", action: "fog changed"},
	"DELETE /maps/:mapID/fog/:fogID":          {kind: "map", action: "fog changed"},
	"DELETE /maps/:mapID/fog":                 {kind: "map", action: "fog reset"},
	"POST /notes":                             {kind: "note", action: "note created"},
	"PUT /notes/:noteID":                      {kind: "note", action: "note updated"},
	"DELETE /notes/:noteID":                   {kind: "note", action: "note deleted"},
	"POST /sync/mappings":                     {kind: "link", action: "linked"},
	"DELETE /sync/mappings/:mappingID":        {kind: "link", action: "unlinked"},
	"POST /stashes/moves":                     {kind: "stash", action: "item moved"},
	"POST /armory/shops/:eid/buy":             {kind: "shop", action: "bought"},
	// A token drag sends a position on every step; one drag would bury the
	// rest of the history.
	"PATCH /maps/:mapID/tokens/:tokenID/position": {skip: true},
	// The module's own reports are recorded as what they describe.
	"POST /sync/history": {skip: true},
	// The player list is a status report, refreshed every few minutes.
	"POST /sync/players": {skip: true},
}

// classifyHistoryCall maps a request to its history kind and action, and
// says whether it belongs in the history at all: writes always, reads only
// when the server failed them.
func classifyHistoryCall(method, routePath string, status int) (historyRoute, string, bool) {
	suffix := routePath
	if i := strings.Index(routePath, "/campaigns/:id"); i >= 0 {
		suffix = routePath[i+len("/campaigns/:id"):]
	}
	if suffix == "" {
		suffix = "/"
	}
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		if status < 500 {
			return historyRoute{}, "", false
		}
		return historyRoute{kind: firstSegment(suffix), action: "read failed"}, suffix, true
	}
	r, ok := historyRoutes[method+" "+suffix]
	if ok {
		return r, suffix, !r.skip
	}
	verb := map[string]string{http.MethodPost: "created", http.MethodPut: "updated", http.MethodPatch: "updated", http.MethodDelete: "deleted"}[method]
	kind := firstSegment(suffix)
	return historyRoute{kind: kind, action: strings.TrimSpace(strings.ReplaceAll(kind, "-", " ") + " " + verb)}, suffix, true
}

func firstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.Index(p, "/"); i >= 0 {
		p = p[:i]
	}
	return p
}

// RecordSyncHistory records each sync call a real API key makes, after it
// ran, with its result. Chronicle's own pages calling these routes with a
// session are not sync and are skipped. Mounted after RequireCampaignMatch
// so the campaign is the key's own.
func RecordSyncHistory(repo SyncHistoryRepository, namer EntityNamer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if repo == nil {
				return next(c)
			}
			start := time.Now()
			err := next(c)
			key := GetAPIKey(c)
			if key == nil || key.ID == synthKeySessionID {
				return err
			}
			status := c.Response().Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok {
					status = he.Code
				} else {
					status = apperror.SafeCode(err)
				}
			}
			route, suffix, ok := classifyHistoryCall(c.Request().Method, c.Path(), status)
			if !ok {
				return err
			}
			ev := SyncEvent{
				OccurredAt: start,
				Direction:  DirToChronicle,
				ReportedBy: reportedByChronicle,
				Kind:       route.kind,
				Action:     route.action,
				Call:       c.Request().Method + " " + suffix,
				Status:     strconv.Itoa(status),
				OK:         status < 400,
				DurationMs: int(time.Since(start).Milliseconds()),
				UserID:     &key.UserID,
				APIKeyID:   &key.ID,
			}
			if err != nil {
				ev.Message = truncate(apperror.UserMessage(err, "the request failed"), 500)
			} else if actor, ok := c.Get(historyActorKey).(string); ok {
				ev.UserID = &actor
			}
			if res, ok := c.Get(historyResourceKey).(historyResource); ok {
				ev.ResourceID, ev.ResourceName, ev.Was = res.id, res.name, truncate(res.was, 200)
			} else if id := c.Param("entityID"); id != "" {
				ev.ResourceID = id
			} else if id := c.Param("noteID"); id != "" {
				ev.ResourceID = id
			} else if id := c.Param("mapID"); id != "" {
				ev.ResourceID = id
			}
			campaignID := key.CampaignID
			lookupEntity := ev.ResourceName == "" && c.Param("entityID") != ""
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), historyRecordTimeout)
				defer cancel()
				if lookupEntity && namer != nil {
					if name, ok := namer.EntityName(ctx, campaignID, ev.ResourceID); ok {
						ev.ResourceName = name
					}
				}
				ev.ResourceName = truncate(ev.ResourceName, 200)
				if _, rerr := repo.Insert(ctx, campaignID, &ev); rerr != nil {
					slog.Warn("sync history not recorded", slog.String("campaign_id", campaignID), slog.Any("error", rerr))
				}
			}()
			return err
		}
	}
}

// StartHistoryPruner deletes expired history rows and stale Foundry player
// lists at startup and then once a day until ctx ends. players may be nil.
func StartHistoryPruner(ctx context.Context, repo SyncHistoryRepository, players FoundryPlayerRepository) {
	pruneHistory(ctx, repo, players)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneHistory(ctx, repo, players)
		}
	}
}

func pruneHistory(ctx context.Context, repo SyncHistoryRepository, players FoundryPlayerRepository) {
	n, err := repo.PruneOlderThan(ctx, time.Now().UTC().Add(-historyRetention))
	if err != nil {
		slog.Warn("sync history prune failed", slog.Any("error", err))
	} else if n > 0 {
		slog.Info("sync history pruned", slog.Int64("rows", n))
	}
	if n, err := prunePlayers(ctx, players, time.Now().UTC()); err != nil {
		slog.Warn("foundry players prune failed", slog.Any("error", err))
	} else if n > 0 {
		slog.Info("foundry players pruned", slog.Int64("rows", n))
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
