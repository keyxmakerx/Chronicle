package connector

import (
	"context"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// memRepo is an in-memory Repository. disabled marks makers whose account is
// disabled, which FindByHash hides as the SQL does.
type memRepo struct {
	mu       sync.Mutex
	conns    []*Connection
	calls    []Call
	reqs     []*Request
	disabled map[string]bool
}

func (r *memRepo) Replace(_ context.Context, c *Connection) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.conns {
		if old.CampaignID == c.CampaignID && old.RevokedAt == nil {
			t := c.CreatedAt
			old.RevokedAt = &t
		}
	}
	cp := *c
	cp.ID = int64(len(r.conns) + 1)
	r.conns = append(r.conns, &cp)
	return cp.ID, nil
}

func (r *memRepo) RevokeAll(_ context.Context, campaignID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		if c.CampaignID == campaignID && c.RevokedAt == nil {
			t := at
			c.RevokedAt = &t
		}
	}
	return nil
}

func (r *memRepo) SetPaused(_ context.Context, campaignID string, id int64, at *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		if c.CampaignID == campaignID && c.ID == id && c.RevokedAt == nil {
			c.PausedAt = at
		}
	}
	return nil
}

func (r *memRepo) SetAccess(_ context.Context, campaignID string, id int64, privacy aiexport.PrivacyMode, canAsk bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		if c.CampaignID == campaignID && c.ID == id && c.RevokedAt == nil {
			c.Privacy, c.CanAsk = privacy, canAsk
		}
	}
	return nil
}

func (r *memRepo) Current(_ context.Context, campaignID string) (*Connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.conns) - 1; i >= 0; i-- {
		if c := r.conns[i]; c.CampaignID == campaignID && c.RevokedAt == nil {
			cp := *c
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRepo) FindByHash(_ context.Context, hash string) (*Connection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		if c.KeyHash == hash && !r.disabled[c.UserID] {
			cp := *c
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRepo) MarkUsed(_ context.Context, id int64, at time.Time, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.conns {
		if c.ID == id {
			t := at
			c.LastUsedAt = &t
			if name != "" {
				c.ClientName = name
			}
		}
	}
	return nil
}

func (r *memRepo) AddCall(_ context.Context, call *Call) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *call
	cp.ID = int64(len(r.calls) + 1)
	r.calls = append(r.calls, cp)
	return nil
}

func (r *memRepo) RecentCalls(_ context.Context, campaignID string, limit int) ([]Call, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Call
	for i := len(r.calls) - 1; i >= 0 && len(out) < limit; i-- {
		if r.calls[i].CampaignID == campaignID {
			out = append(out, r.calls[i])
		}
	}
	return out, nil
}

func (r *memRepo) PruneCalls(context.Context, string, int) error { return nil }

func (r *memRepo) AddRequest(_ context.Context, q *Request, max int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	waiting := 0
	for _, x := range r.reqs {
		if x.CampaignID == q.CampaignID && x.Status == RequestWaiting {
			waiting++
		}
	}
	if waiting >= max {
		return 0, ErrTooManyWaiting
	}
	cp := *q
	cp.ID = int64(len(r.reqs) + 1)
	r.reqs = append(r.reqs, &cp)
	return cp.ID, nil
}

func (r *memRepo) Requests(_ context.Context, campaignID, status string, limit int) ([]Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Request
	for i := len(r.reqs) - 1; i >= 0 && len(out) < limit; i-- {
		if q := r.reqs[i]; q.CampaignID == campaignID && q.Status == status {
			cp := *q
			cp.Body = ""
			out = append(out, cp)
		}
	}
	return out, nil
}

func (r *memRepo) Request(_ context.Context, campaignID string, id int64) (*Request, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.ID == id && q.CampaignID == campaignID {
			cp := *q
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRepo) FinishRequest(_ context.Context, campaignID string, id int64, status, outcome string, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.ID == id && q.CampaignID == campaignID && q.Status == RequestWaiting {
			t := at
			q.Status, q.Outcome, q.DecidedAt = status, outcome, &t
			return true, nil
		}
	}
	return false, nil
}

func (r *memRepo) PruneRequests(context.Context, string, int) error { return nil }

// owners says who owns which campaign; a campaign missing from the map is
// deleted or in the Trash.
type owners map[string]string

func (o owners) Check(_ context.Context, campaignID, userID string) (string, error) {
	if owner, ok := o[campaignID]; ok && owner == userID {
		return "Ashfall", nil
	}
	return "", ErrNotOwner
}

// memTables is a rolling-tables store that fails the test's read-only
// promise if anything writes to it.
type memTables struct {
	doc    records.TableDoc
	writes int
}

func (m *memTables) Get(context.Context, string) (records.TableDoc, error) { return m.doc, nil }
func (m *memTables) Put(context.Context, string, records.TableDoc, string) error {
	m.writes++
	return nil
}

func lootTables() *memTables {
	return &memTables{doc: records.TableDoc{Tables: []records.Table{{ID: "loot", Name: "Loot", Entries: []records.Entry{{Name: "Gem", Weight: 1}}}}}}
}
