package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestViewGrants(t *testing.T) {
	user := func(id string) entities.EntityPermission {
		return entities.EntityPermission{SubjectType: entities.SubjectUser, SubjectID: id, Permission: entities.PermView}
	}
	role := entities.EntityPermission{SubjectType: entities.SubjectRole, SubjectID: "2", Permission: entities.PermEdit}
	tests := []struct {
		name        string
		entity      entities.Entity
		existing    []entities.EntityPermission
		users       []string
		wantChanged bool
		wantSubject []string // "type:id" in order
		wantAdded   []string
	}{
		{"private default starts from Scribe and up", entities.Entity{IsPrivate: true}, nil, []string{"u1"}, true, []string{"role:2", "user:u1"}, []string{"u1"}},
		{"private default, no players, still private", entities.Entity{IsPrivate: true}, nil, nil, true, []string{"role:2"}, nil},
		{"public default is left alone", entities.Entity{}, nil, []string{"u1"}, false, nil, nil},
		{"custom keeps its grants and adds the holder", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role, user("u1")}, []string{"u2"}, true, []string{"role:2", "user:u1", "user:u2"}, []string{"u2"}},
		{"custom already allowing the holder changes nothing", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role, user("u1")}, []string{"u1"}, false, []string{"role:2", "user:u1"}, nil},
		{"blank and repeated ids are skipped", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role}, []string{"", "u1", "u1"}, true, []string{"role:2", "user:u1"}, []string{"u1"}},
		{"only the new users count as added", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role, user("u1")}, []string{"u1", "u2"}, true, []string{"role:2", "user:u1", "user:u2"}, []string{"u2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			grants, added, changed := viewGrants(&tc.entity, tc.existing, tc.users)
			if changed != tc.wantChanged {
				t.Fatalf("changed=%v", changed)
			}
			if strings.Join(added, ",") != strings.Join(tc.wantAdded, ",") {
				t.Fatalf("added %v want %v", added, tc.wantAdded)
			}
			var got []string
			for _, g := range grants {
				got = append(got, string(g.SubjectType)+":"+g.SubjectID)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantSubject, ",") {
				t.Fatalf("grants %v want %v", got, tc.wantSubject)
			}
		})
	}
}

// The scribe-and-up grant must be able to edit, as a private entity allows,
// and a holder may only view.
func TestViewGrants_Permissions(t *testing.T) {
	grants, _, _ := viewGrants(&entities.Entity{IsPrivate: true}, nil, []string{"u1"})
	if grants[0].Permission != entities.PermEdit || grants[1].Permission != entities.PermView {
		t.Fatalf("%+v", grants)
	}
}

// Taking a share back removes only plain view grants for those users: a
// role grant, another user's grant and a grant above view all stay.
func TestWithoutViewers(t *testing.T) {
	g := func(st entities.SubjectType, id string, p entities.Permission) entities.EntityPermission {
		return entities.EntityPermission{SubjectType: st, SubjectID: id, Permission: p}
	}
	existing := []entities.EntityPermission{
		g(entities.SubjectRole, "2", entities.PermEdit),
		g(entities.SubjectUser, "u1", entities.PermView),
		g(entities.SubjectUser, "u2", entities.PermView),
		g(entities.SubjectUser, "u3", entities.PermEdit),
	}
	tests := []struct {
		name        string
		users       []string
		wantChanged bool
		want        string
	}{
		{"one viewer", []string{"u2"}, true, "role:2,user:u1,user:u3"},
		{"an editor set by hand stays", []string{"u3"}, false, "role:2,user:u1,user:u2,user:u3"},
		{"nobody", nil, false, "role:2,user:u1,user:u2,user:u3"},
		{"someone with no grant", []string{"u9"}, false, "role:2,user:u1,user:u2,user:u3"},
		{"a role id is not a user", []string{"2"}, false, "role:2,user:u1,user:u2,user:u3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			grants, changed := withoutViewers(existing, tc.users)
			if changed != tc.wantChanged {
				t.Fatalf("changed=%v", changed)
			}
			var got []string
			for _, x := range grants {
				got = append(got, string(x.SubjectType)+":"+x.SubjectID)
			}
			if strings.Join(got, ",") != tc.want {
				t.Fatalf("grants %v want %s", got, tc.want)
			}
		})
	}
}

func TestHandoutEntry_LinksTheMapAndEscapes(t *testing.T) {
	j, h := handoutEntry("camp-1", armory.NamedRef{ID: "m-1", Name: `Crypt <b>"of" Doom`})
	if !strings.Contains(h, `href="/campaigns/camp-1/maps/m-1"`) || strings.Contains(h, "<b>") {
		t.Fatalf("html %s", h)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(j), &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(j, `"type":"link"`) || !strings.Contains(j, `/campaigns/camp-1/maps/m-1`) {
		t.Fatalf("json %s", j)
	}
}

func TestPickHandout(t *testing.T) {
	m1, m2 := "m1", "m2"
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	marked := func(id string, age int, mapID *string, marker any) entities.Entity {
		return entities.Entity{ID: id, Name: "Map: Dungeon", MapID: mapID, CreatedAt: t0.Add(time.Duration(age) * time.Hour),
			FieldsData: map[string]any{handoutMarkerField: marker}}
	}
	tests := []struct {
		name string
		list []entities.Entity
		want string
	}{
		{"a Scribe's own note with the same name and map is not taken", []entities.Entity{
			{ID: "note", Name: "Map: Dungeon", MapID: &m1, CreatedAt: t0}}, ""},
		{"marker for another map is not taken", []entities.Entity{marked("a", 0, &m1, "m2")}, ""},
		{"same marker but another assigned map is not taken", []entities.Entity{marked("a", 0, &m2, "m1")}, ""},
		{"oldest marked one wins", []entities.Entity{marked("new", 5, &m1, "m1"), marked("old", 1, &m1, "m1"), marked("mid", 3, &m1, "m1")}, "old"},
		{"same age breaks the tie by id", []entities.Entity{marked("b", 1, &m1, "m1"), marked("a", 1, &m1, "m1")}, "a"},
		{"the plain note is skipped even when older", []entities.Entity{
			{ID: "note", Name: "Map: Dungeon", MapID: &m1, CreatedAt: t0.Add(-time.Hour)}, marked("h", 1, &m1, "m1")}, "h"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickHandout(tc.list, "Map: Dungeon", "m1")
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("picked %s", got.ID)
			case tc.want != "" && (got == nil || got.ID != tc.want):
				t.Fatalf("picked %v want %s", got, tc.want)
			}
		})
	}
}
