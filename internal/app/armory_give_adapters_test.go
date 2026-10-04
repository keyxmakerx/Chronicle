package app

import (
	"encoding/json"
	"strings"
	"testing"

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
	}{
		{"private default starts from Scribe and up", entities.Entity{IsPrivate: true}, nil, []string{"u1"}, true, []string{"role:2", "user:u1"}},
		{"private default, no players, still private", entities.Entity{IsPrivate: true}, nil, nil, true, []string{"role:2"}},
		{"public default is left alone", entities.Entity{}, nil, []string{"u1"}, false, nil},
		{"custom keeps its grants and adds the holder", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role, user("u1")}, []string{"u2"}, true, []string{"role:2", "user:u1", "user:u2"}},
		{"custom already allowing the holder changes nothing", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role, user("u1")}, []string{"u1"}, false, []string{"role:2", "user:u1"}},
		{"blank and repeated ids are skipped", entities.Entity{Visibility: entities.VisibilityCustom}, []entities.EntityPermission{role}, []string{"", "u1", "u1"}, true, []string{"role:2", "user:u1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			grants, changed := viewGrants(&tc.entity, tc.existing, tc.users)
			if changed != tc.wantChanged {
				t.Fatalf("changed=%v", changed)
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
	grants, _ := viewGrants(&entities.Entity{IsPrivate: true}, nil, []string{"u1"})
	if grants[0].Permission != entities.PermEdit || grants[1].Permission != entities.PermView {
		t.Fatalf("%+v", grants)
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
