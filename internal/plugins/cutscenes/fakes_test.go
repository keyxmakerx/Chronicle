package cutscenes

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// memRepo is the repository in memory, with the same campaign scoping and
// answers as the SQL one.
type memRepo struct {
	mu       sync.Mutex
	scenes   map[string]Scene  // id -> scene
	sceneCmp map[string]string // id -> campaign
	showings map[string]Showing
}

func newMemRepo() *memRepo {
	return &memRepo{scenes: map[string]Scene{}, sceneCmp: map[string]string{}, showings: map[string]Showing{}}
}

func (r *memRepo) ListScenes(_ context.Context, campaignID string) ([]Scene, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Scene{}
	for id, s := range r.scenes {
		if r.sceneCmp[id] == campaignID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *memRepo) GetScene(_ context.Context, campaignID, id string) (*Scene, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.scenes[id]
	if !ok || r.sceneCmp[id] != campaignID {
		return nil, apperror.NewNotFound("scene not found")
	}
	return &s, nil
}

func (r *memRepo) taken(campaignID, name, except string) bool {
	for id, s := range r.scenes {
		if id != except && r.sceneCmp[id] == campaignID && strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}

func (r *memRepo) CreateScene(_ context.Context, campaignID, _ string, s Scene) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taken(campaignID, s.Name, "") {
		return nameTaken(s.Name)
	}
	r.scenes[s.ID] = s
	r.sceneCmp[s.ID] = campaignID
	return nil
}

func (r *memRepo) UpdateScene(_ context.Context, campaignID string, s Scene) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.scenes[s.ID]; !ok || r.sceneCmp[s.ID] != campaignID {
		return apperror.NewNotFound("scene not found")
	}
	if r.taken(campaignID, s.Name, s.ID) {
		return nameTaken(s.Name)
	}
	r.scenes[s.ID] = s
	return nil
}

func (r *memRepo) DeleteScene(_ context.Context, campaignID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.scenes[id]; !ok || r.sceneCmp[id] != campaignID {
		return apperror.NewNotFound("scene not found")
	}
	delete(r.scenes, id)
	for k, sh := range r.showings {
		if sh.SceneID == id {
			sh.SceneID = ""
			r.showings[k] = sh
		}
	}
	return nil
}

func (r *memRepo) CreateShowing(_ context.Context, s Showing) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	vs := make([]ShowingViewer, len(s.Viewers))
	for i, v := range s.Viewers {
		vs[i] = ShowingViewer{UserID: v.UserID}
	}
	s.Viewers = vs
	r.showings[s.ID] = s
	return nil
}

func copyShowing(s Showing) Showing {
	s.Viewers = append([]ShowingViewer(nil), s.Viewers...)
	return s
}

func (r *memRepo) GetShowing(_ context.Context, campaignID, id string) (*Showing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.showings[id]
	if !ok || s.CampaignID != campaignID {
		return nil, apperror.NewNotFound("showing not found")
	}
	c := copyShowing(s)
	return &c, nil
}

func (r *memRepo) StopShowing(_ context.Context, campaignID, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.showings[id]
	if ok && s.CampaignID == campaignID && s.StoppedAt == nil {
		s.StoppedAt = &at
		r.showings[id] = s
	}
	return nil
}

func (r *memRepo) sorted(campaignID string) []Showing {
	var out []Showing
	for _, s := range r.showings {
		if s.CampaignID == campaignID {
			out = append(out, copyShowing(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ShownAt.After(out[j].ShownAt) })
	return out
}

func (r *memRepo) RecentShowings(_ context.Context, campaignID string, limit int) ([]Showing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.sorted(campaignID)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *memRepo) ViewerShowings(_ context.Context, campaignID, userID string, now time.Time) ([]Showing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Showing{}
	for _, s := range r.sorted(campaignID) {
		if !s.RewatchUntil.After(now) {
			continue
		}
		for _, v := range s.Viewers {
			if v.UserID == userID {
				s.Viewers = []ShowingViewer{v}
				out = append(out, s)
				break
			}
		}
	}
	return out, nil
}

func (r *memRepo) MarkWatched(_ context.Context, campaignID, showingID, userID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.showings[showingID]
	if !ok || s.CampaignID != campaignID {
		return nil
	}
	for i, v := range s.Viewers {
		if v.UserID == userID && v.WatchedAt == nil {
			t := at
			s.Viewers[i].WatchedAt = &t
		}
	}
	r.showings[showingID] = s
	return nil
}

// fakeMembers knows camp-1: dir (owner), codm (granted), ana and bo
// (players); camp-2: other (owner).
type fakeMembers struct{}

func (fakeMembers) Members(_ context.Context, campaignID string) ([]Member, error) {
	if campaignID == "camp-2" {
		return []Member{{UserID: "other", Name: "Other", Director: true}}, nil
	}
	return []Member{
		{UserID: "dir", Name: "Dee", Director: true},
		{UserID: "codm", Name: "Cy", Director: true},
		{UserID: "ana", Name: "Ana"},
		{UserID: "bo", Name: "Bo"},
	}, nil
}

type fakePresence map[string]Place

func (p fakePresence) Places(context.Context, string) (map[string]Place, error) { return p, nil }

// spyPublisher records what went out live.
type spyPublisher struct {
	mu    sync.Mutex
	shown []Showing
	stops []Showing
}

func (p *spyPublisher) Show(_ string, s Showing) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shown = append(p.shown, s)
}

func (p *spyPublisher) Stop(_ string, s Showing) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stops = append(p.stops, s)
}

// spyFoundry records Foundry previews; offered and connected are settable.
type spyFoundry struct {
	mu        sync.Mutex
	offered   bool
	connected bool
	sent      []string
	scenes    []Scene
}

func (f *spyFoundry) Offered(context.Context, string) bool { return f.offered }

func (f *spyFoundry) Preview(campaignID, directorID string, sc Scene) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.connected {
		return false
	}
	f.sent = append(f.sent, campaignID+"/"+directorID+"/"+sc.Name)
	f.scenes = append(f.scenes, sc)
	return true
}

// clock is a settable now.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type rig struct {
	repo    *memRepo
	pub     *spyPublisher
	foundry *spyFoundry
	clock   *clock
	svc     Service
}

var (
	vDir   = Viewer{UserID: "dir", Director: true}
	vCoDM  = Viewer{UserID: "codm", Director: true}
	vAna   = Viewer{UserID: "ana"}
	vBo    = Viewer{UserID: "bo"}
	vOther = Viewer{UserID: "other", Director: true}
)

func newRig() *rig {
	r := &rig{repo: newMemRepo(), pub: &spyPublisher{}, foundry: &spyFoundry{offered: true, connected: true}, clock: &clock{t: time.Date(2026, 11, 20, 20, 0, 0, 0, time.UTC)}}
	r.svc = NewService(r.repo, fakeMembers{},
		WithPresence(fakePresence{"ana": {Chronicle: true}, "bo": {Foundry: true}, "dir": {Chronicle: true, Foundry: true}}),
		WithPublisher(r.pub), WithFoundryPreview(r.foundry), WithClock(r.clock.now))
	return r
}

// scene makes a scene as the Director, failing the test on error.
func (r *rig) scene(t interface {
	Helper()
	Fatalf(string, ...any)
}, name string) *Scene {
	t.Helper()
	in := NewScene()
	in.Name = name
	sc, err := r.svc.Create(context.Background(), "camp-1", vDir, in)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return sc
}
