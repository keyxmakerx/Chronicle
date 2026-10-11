// place_service_test.go pins the rules for listing one page in more than one
// place in the page tree: what is refused, that an extra listing never
// changes who sees a page, and that a real move keeps listings honest.
package entities

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakePlaces is an in-memory PlaceRepository whose AncestorIDs walks real
// parents and listings together, as the SQL does.
type fakePlaces struct {
	pages  map[string]*Entity
	links  []Place
	failOn string // "insert" makes Insert fail
}

func (f *fakePlaces) Insert(_ context.Context, p *Place) error {
	if f.failOn == "insert" {
		return errors.New("db down")
	}
	f.links = append(f.links, *p)
	return nil
}

func (f *fakePlaces) Delete(_ context.Context, entityID, parentID string) error {
	out := f.links[:0]
	for _, l := range f.links {
		if l.EntityID != entityID || l.ParentEntityID != parentID {
			out = append(out, l)
		}
	}
	f.links = out
	return nil
}

func (f *fakePlaces) view(l Place) PlaceLink {
	return PlaceLink{
		EntityID: l.EntityID, EntityName: f.pages[l.EntityID].Name,
		ParentID: l.ParentEntityID, ParentName: f.pages[l.ParentEntityID].Name,
	}
}

func (f *fakePlaces) ListOf(_ context.Context, _, entityID string) ([]PlaceLink, error) {
	var out []PlaceLink
	for _, l := range f.links {
		if l.EntityID == entityID {
			out = append(out, f.view(l))
		}
	}
	return out, nil
}

func (f *fakePlaces) ListUnder(_ context.Context, _ string, parentIDs []string) ([]PlaceLink, error) {
	var out []PlaceLink
	for _, l := range f.links {
		for _, p := range parentIDs {
			if l.ParentEntityID == p {
				out = append(out, f.view(l))
			}
		}
	}
	return out, nil
}

func (f *fakePlaces) ListAll(_ context.Context, _ string) ([]Place, error) {
	return append([]Place(nil), f.links...), nil
}

func (f *fakePlaces) AncestorIDs(_ context.Context, _, id string) (map[string]bool, error) {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(x string) {
		if seen[x] {
			return
		}
		seen[x] = true
		if p := f.pages[x]; p != nil && p.ParentID != nil {
			walk(*p.ParentID)
		}
		for _, l := range f.links {
			if l.EntityID == x {
				walk(l.ParentEntityID)
			}
		}
	}
	walk(id)
	return seen, nil
}

func pageIn(camp, id string, parent string) *Entity {
	e := &Entity{ID: id, Name: "Page " + id, CampaignID: camp}
	if parent != "" {
		e.ParentID = &parent
	}
	return e
}

// newPlaceFixture builds this tree (all in camp-1 unless noted):
//
//	city
//	  guild
//	    npc
//	other            (a separate root)
//	foreign          (camp-2)
//	gone             (in the Trash: FindByID does not find it)
func newPlaceFixture(t *testing.T, visible func(role int, id string) bool) (PlaceService, *fakePlaces) {
	t.Helper()
	pages := map[string]*Entity{
		"city":    pageIn("camp-1", "city", ""),
		"guild":   pageIn("camp-1", "guild", "city"),
		"npc":     pageIn("camp-1", "npc", "guild"),
		"other":   pageIn("camp-1", "other", ""),
		"foreign": pageIn("camp-2", "foreign", ""),
	}
	repo := &fakePlaces{pages: pages}
	role := 0
	ents := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			if p, ok := pages[id]; ok {
				c := *p
				return &c, nil
			}
			return nil, apperror.NewNotFound("entity not found")
		},
	}
	ents.filterViewableFn = func(ids []string) (map[string]bool, error) {
		out := map[string]bool{}
		for _, id := range ids {
			if visible == nil || visible(role, id) {
				out[id] = true
			}
		}
		return out, nil
	}
	svc := &roleSetter{PlaceService: NewPlaceService(ents, repo), role: &role}
	return svc, repo
}

// roleSetter lets a test choose the viewer role the fake visibility sees,
// because the service passes it straight through to the repository.
type roleSetter struct {
	PlaceService
	role *int
}

func (r *roleSetter) PlacesOf(ctx context.Context, camp, id string, role int, uid string) ([]PlaceLink, error) {
	*r.role = role
	return r.PlaceService.PlacesOf(ctx, camp, id, role, uid)
}

func (r *roleSetter) PlacesUnder(ctx context.Context, camp string, ids []string, role int, uid string) ([]PlaceLink, error) {
	*r.role = role
	return r.PlaceService.PlacesUnder(ctx, camp, ids, role, uid)
}

func TestAddPlace_Rules(t *testing.T) {
	tests := []struct {
		name       string
		existing   []Place // listings already there
		entity     string
		parent     string
		wantStatus int // 0 = accepted
		wantRows   int // rows after the call
	}{
		{name: "lists a page under an unrelated page", entity: "npc", parent: "other", wantRows: 1},
		{name: "lists it under a page elsewhere in the same tree", entity: "npc", parent: "city", wantRows: 1},
		{name: "refuses listing a page under itself", entity: "npc", parent: "npc", wantStatus: http.StatusBadRequest},
		{name: "refuses its own real parent", entity: "npc", parent: "guild", wantStatus: http.StatusBadRequest},
		{name: "refuses a real descendant", entity: "city", parent: "npc", wantStatus: http.StatusBadRequest},
		{name: "refuses a direct real child", entity: "city", parent: "guild", wantStatus: http.StatusBadRequest},
		{
			name:     "refuses a page that reaches it through another listing",
			existing: []Place{{EntityID: "other", ParentEntityID: "npc", CampaignID: "camp-1"}},
			entity:   "npc", parent: "other", wantStatus: http.StatusBadRequest, wantRows: 1,
		},
		{
			name:     "refuses a chain of listings that loops back",
			existing: []Place{{EntityID: "other", ParentEntityID: "city", CampaignID: "camp-1"}},
			entity:   "city", parent: "other", wantStatus: http.StatusBadRequest, wantRows: 1,
		},
		{name: "refuses a page in another campaign as the parent", entity: "npc", parent: "foreign", wantStatus: http.StatusNotFound},
		{name: "refuses a page in another campaign as the page", entity: "foreign", parent: "city", wantStatus: http.StatusNotFound},
		{name: "refuses a missing or trashed parent", entity: "npc", parent: "gone", wantStatus: http.StatusNotFound},
		{name: "refuses a missing or trashed page", entity: "gone", parent: "city", wantStatus: http.StatusNotFound},
		{
			name:     "adding the same listing twice is a no-op",
			existing: []Place{{EntityID: "npc", ParentEntityID: "other", CampaignID: "camp-1"}},
			entity:   "npc", parent: "other", wantRows: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newPlaceFixture(t, nil)
			repo.links = append(repo.links, tc.existing...)
			err := svc.AddPlace(context.Background(), "camp-1", tc.entity, tc.parent, "u1")
			if tc.wantStatus == 0 {
				if err != nil {
					t.Fatalf("AddPlace: %v", err)
				}
			} else {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tc.wantStatus {
					t.Fatalf("AddPlace error = %v, want status %d", err, tc.wantStatus)
				}
			}
			if len(repo.links) != tc.wantRows {
				t.Fatalf("rows = %d, want %d (%+v)", len(repo.links), tc.wantRows, repo.links)
			}
		})
	}
}

func TestAddPlace_CapsPlacesPerPage(t *testing.T) {
	svc, repo := newPlaceFixture(t, nil)
	for i := 0; i < maxPlacesPerPage; i++ {
		id := "x" + string(rune('a'+i))
		repo.pages[id] = pageIn("camp-1", id, "")
		repo.links = append(repo.links, Place{EntityID: "npc", ParentEntityID: id, CampaignID: "camp-1"})
	}
	err := svc.AddPlace(context.Background(), "camp-1", "npc", "other", "u1")
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		t.Fatalf("expected a 400 at the cap, got %v", err)
	}
}

func TestAddPlace_DatabaseFailureIsInternal(t *testing.T) {
	svc, repo := newPlaceFixture(t, nil)
	repo.failOn = "insert"
	err := svc.AddPlace(context.Background(), "camp-1", "npc", "other", "u1")
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusInternalServerError {
		t.Fatalf("expected a 500, got %v", err)
	}
}

func TestRemovePlace(t *testing.T) {
	svc, repo := newPlaceFixture(t, nil)
	repo.links = []Place{{EntityID: "npc", ParentEntityID: "other", CampaignID: "camp-1"}}
	if err := svc.RemovePlace(context.Background(), "camp-1", "npc", "other"); err != nil {
		t.Fatalf("RemovePlace: %v", err)
	}
	if len(repo.links) != 0 {
		t.Fatalf("listing not removed: %+v", repo.links)
	}
	if _, ok := repo.pages["npc"]; !ok {
		t.Fatal("removing a listing must never touch the page")
	}
	// Another campaign's page can't be reached by id.
	err := svc.RemovePlace(context.Background(), "camp-1", "foreign", "city")
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a foreign page, got %v", err)
	}
}

// TestPlaces_VisibilityFollowsThePage: a listing is shown only to a viewer who
// can see both the page and the parent it is listed under.
func TestPlaces_VisibilityFollowsThePage(t *testing.T) {
	// "secret" is a hidden page for players (role < 2) and "vault" a hidden parent.
	visible := func(role int, id string) bool {
		if role >= 2 {
			return true
		}
		return id != "secret" && id != "vault"
	}
	tests := []struct {
		name string
		role int
		page string
		want int
	}{
		{"player sees a public page under a public parent", 1, "npc", 1},
		{"player does not see a hidden page under a public parent", 1, "secret", 0},
		{"player can see the page but not the hidden extra parent", 1, "other", 0},
		{"scribe sees the hidden page under the public parent", 2, "secret", 1},
		{"scribe sees the page under the hidden parent", 2, "other", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newPlaceFixture(t, visible)
			repo.pages["secret"] = pageIn("camp-1", "secret", "")
			repo.pages["vault"] = pageIn("camp-1", "vault", "")
			repo.links = []Place{
				{EntityID: "npc", ParentEntityID: "other", CampaignID: "camp-1"},
				{EntityID: "secret", ParentEntityID: "city", CampaignID: "camp-1"},
				{EntityID: "other", ParentEntityID: "vault", CampaignID: "camp-1"},
			}
			byPage, err := svc.PlacesOf(context.Background(), "camp-1", tc.page, tc.role, "u1")
			if err != nil {
				t.Fatalf("PlacesOf: %v", err)
			}
			if tc.page == "npc" {
				// npc is public and listed under public "other".
				if len(byPage) != tc.want {
					t.Fatalf("PlacesOf(npc) = %d, want %d", len(byPage), tc.want)
				}
				return
			}
			if len(byPage) != tc.want {
				t.Fatalf("PlacesOf(%s) = %d, want %d", tc.page, len(byPage), tc.want)
			}
			// The tree read applies the same rule.
			parent := map[string]string{"secret": "city", "other": "vault"}[tc.page]
			under, err := svc.PlacesUnder(context.Background(), "camp-1", []string{parent}, tc.role, "u1")
			if err != nil {
				t.Fatalf("PlacesUnder: %v", err)
			}
			if len(under) != tc.want {
				t.Fatalf("PlacesUnder(%s) = %d, want %d", parent, len(under), tc.want)
			}
		})
	}
}

func TestExportPlaces(t *testing.T) {
	svc, repo := newPlaceFixture(t, nil)
	repo.links = []Place{{EntityID: "npc", ParentEntityID: "other", CampaignID: "camp-1"}}
	got, err := svc.ExportPlaces(context.Background(), "camp-1")
	if err != nil || len(got) != 1 {
		t.Fatalf("ExportPlaces = %v, %v", got, err)
	}
}

// TestReorderEntity_RefusesCycleThroughListing: moving a page for real under a
// page that is listed below it is refused, and moving it under a page it was
// also listed under drops the now-redundant listing.
func TestReorderEntity_PlaceGuard(t *testing.T) {
	svc, repo := newPlaceFixture(t, nil)
	repo.links = []Place{{EntityID: "other", ParentEntityID: "npc", CampaignID: "camp-1"}}

	ents := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			if p, ok := repo.pages[id]; ok {
				c := *p
				return &c, nil
			}
			return nil, apperror.NewNotFound("entity not found")
		},
	}
	es := newTestService(ents, &mockEntityTypeRepo{})
	es.(*entityService).SetPlaceGuard(svc)

	// city -> under other: other is listed below npc, which sits below city.
	other := "other"
	err := es.ReorderEntity(context.Background(), "camp-1", "city", &other, nil, 0)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		t.Fatalf("expected the loop to be refused, got %v", err)
	}

	// A plain real-parent loop (no listings involved) must not talk about
	// listings: city under its own descendant npc.
	repo.links = nil
	npc := "npc"
	err = es.ReorderEntity(context.Background(), "camp-1", "city", &npc, nil, 0)
	if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
		t.Fatalf("expected the real loop to be refused, got %v", err)
	}
	if !strings.Contains(ae.Message, "is below this page") || strings.Contains(ae.Message, "listed") {
		t.Errorf("real-loop message = %q, want it to say the parent is below this page", ae.Message)
	}

	// npc really moves under other, where it was already listed: the listing goes.
	repo.links = []Place{{EntityID: "npc", ParentEntityID: "other", CampaignID: "camp-1"}}
	if err := es.ReorderEntity(context.Background(), "camp-1", "npc", &other, nil, 0); err != nil {
		t.Fatalf("ReorderEntity: %v", err)
	}
	if len(repo.links) != 0 {
		t.Fatalf("the listing under the new real parent should be dropped, got %+v", repo.links)
	}
}
