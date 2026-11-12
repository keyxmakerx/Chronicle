package designs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// memRepo is an in-memory Repository with the real one's visible behaviour:
// copies are separate rows and stopping a share clears the token and owners.
type memRepo struct {
	designs map[string]*Design
	order   []string
	tokens  map[string]string // design id -> token
	on      map[string]bool
	owners  map[string]map[string]bool
	search  []OwnerMatch
}

func newMemRepo() *memRepo {
	return &memRepo{designs: map[string]*Design{}, tokens: map[string]string{}, on: map[string]bool{}, owners: map[string]map[string]bool{}}
}

func (m *memRepo) shared(id string) bool { return m.on[id] || len(m.owners[id]) > 0 }

func (m *memRepo) List(_ context.Context, campaignID, kind string) ([]Design, error) {
	var out []Design
	for _, id := range m.order {
		d := m.designs[id]
		if d != nil && d.CampaignID == campaignID && (kind == "" || d.Kind == kind) {
			c := *d
			c.Shared = m.shared(id)
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memRepo) Get(_ context.Context, id string) (*Design, error) {
	d := m.designs[id]
	if d == nil {
		return nil, nil
	}
	c := *d
	c.Data = append(json.RawMessage(nil), d.Data...)
	c.Shared = m.shared(id)
	return &c, nil
}

func (m *memRepo) Count(ctx context.Context, campaignID string) (int, error) {
	l, _ := m.List(ctx, campaignID, "")
	return len(l), nil
}

func (m *memRepo) Insert(_ context.Context, d *Design) error {
	c := *d
	c.Data = append(json.RawMessage(nil), d.Data...)
	m.designs[d.ID] = &c
	m.order = append(m.order, d.ID)
	return nil
}

func (m *memRepo) Update(_ context.Context, id, name, note string) error {
	m.designs[id].Name, m.designs[id].Note = name, note
	return nil
}

func (m *memRepo) Delete(_ context.Context, id string) error {
	delete(m.designs, id)
	delete(m.tokens, id)
	delete(m.on, id)
	delete(m.owners, id)
	return nil
}

func (m *memRepo) Share(_ context.Context, id string) (string, bool, []Owner, error) {
	owners := []Owner{}
	for u := range m.owners[id] {
		owners = append(owners, Owner{UserID: u, Name: u})
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].UserID < owners[j].UserID })
	return m.tokens[id], m.on[id], owners, nil
}

func (m *memRepo) SetLink(_ context.Context, id, token string, on bool) error {
	m.tokens[id], m.on[id] = token, on
	return nil
}

func (m *memRepo) FindByToken(ctx context.Context, token string) (*Design, bool, error) {
	for id, t := range m.tokens {
		if t != "" && t == token {
			d, _ := m.Get(ctx, id)
			return d, m.on[id], nil
		}
	}
	return nil, false, nil
}

func (m *memRepo) AddOwner(_ context.Context, id, user string) error {
	if m.owners[id] == nil {
		m.owners[id] = map[string]bool{}
	}
	m.owners[id][user] = true
	return nil
}

func (m *memRepo) RemoveOwner(_ context.Context, id, user string) error {
	delete(m.owners[id], user)
	return nil
}

func (m *memRepo) IsNamedOwner(_ context.Context, id, user string) (bool, error) {
	return m.owners[id][user], nil
}

func (m *memRepo) StopSharing(_ context.Context, id string) error {
	m.tokens[id], m.on[id] = "", false
	delete(m.owners, id)
	return nil
}

func (m *memRepo) SharedWith(_ context.Context, user, system, kind, except string) ([]SharedDesign, error) {
	out := []SharedDesign{}
	for _, id := range m.order {
		d := m.designs[id]
		if d != nil && m.owners[id][user] && d.SystemID == system && d.CampaignID != except && (kind == "" || d.Kind == kind) {
			out = append(out, SharedDesign{ID: id, Kind: d.Kind, Name: d.Name, MakerName: d.MakerName, campaignID: d.CampaignID})
		}
	}
	return out, nil
}

func (m *memRepo) SearchOwners(_ context.Context, q, except string, limit int) ([]OwnerMatch, error) {
	var out []OwnerMatch
	for _, o := range m.search {
		if o.UserID != except && strings.Contains(strings.ToLower(o.DisplayName), strings.ToLower(q)) && len(out) < limit {
			out = append(out, o)
		}
	}
	return out, nil
}

// fakeDir has campaigns with their owners; coDMs are members who are not owners.
type fakeDir struct {
	campaigns map[string]CampaignInfo
	owners    map[string]string // campaign id -> owner user id
	names     map[string]string // user id -> display name
	members   map[string]bool   // "campaign/user" for non-owner members
	archived  map[string]bool   // campaigns that exist but are archived
}

func (f *fakeDir) Campaign(_ context.Context, id string) (*CampaignInfo, error) {
	c, ok := f.campaigns[id]
	if !ok {
		return nil, nil
	}
	return &c, nil
}

func (f *fakeDir) Live(_ context.Context, id string) (bool, error) {
	_, ok := f.campaigns[id]
	return ok && !f.archived[id], nil
}

func (f *fakeDir) Member(_ context.Context, campaignID, userID string) (string, bool, error) {
	if f.owners[campaignID] == userID {
		return f.names[userID], true, nil
	}
	if f.members[campaignID+"/"+userID] {
		return f.names[userID], false, nil
	}
	return "", false, nil
}

func (f *fakeDir) OwnedCampaigns(_ context.Context, userID string) ([]CampaignInfo, error) {
	var out []CampaignInfo
	for id, o := range f.owners {
		if o == userID {
			out = append(out, f.campaigns[id])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// themeKind accepts objects with a string "part" from a known list, and says a
// campaign lacks "silver" unless its id is in has.
type themeKind struct{ lack map[string]bool }

var errPart = errors.New("unknown part")

func (k themeKind) Validate(raw json.RawMessage) (json.RawMessage, error) {
	var v struct {
		Part string `json:"part"`
	}
	if json.Unmarshal(raw, &v) != nil || (v.Part != "oak" && v.Part != "silver") {
		return nil, fmt.Errorf("%w: %s", errPart, raw)
	}
	return json.RawMessage(fmt.Sprintf(`{"part":%q}`, v.Part)), nil
}

func (k themeKind) Lacks(_ context.Context, campaignID string, raw json.RawMessage) ([]string, error) {
	if k.lack[campaignID] && strings.Contains(string(raw), "silver") {
		return []string{"This campaign has no silver."}, nil
	}
	return nil, nil
}
