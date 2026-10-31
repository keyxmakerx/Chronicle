package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

type fakeImportUsers map[string]bool

func (f fakeImportUsers) GetUser(_ context.Context, id string) (*auth.User, error) {
	if id == "db-down" {
		return nil, errors.New("db down")
	}
	if !f[id] {
		return nil, apperror.NewNotFound("user not found")
	}
	return &auth.User{ID: id}, nil
}

// An imported pin keeps its player rules only when every player they name is
// on this site; otherwise it is Director-only, never shown to every player.
func TestImportPinVisibility(t *testing.T) {
	str := func(s string) *string { return &s }
	users := fakeImportUsers{"ann": true, "bob": true}
	tests := []struct {
		name        string
		users       importUserLookup
		vis         string
		rules       *string
		wantVis     string
		wantRules   bool
		wantDemoted bool
	}{
		{"no rules", users, "everyone", nil, "everyone", false, false},
		{"empty rules", users, "everyone", str(`{}`), "everyone", false, false},
		{"known deny", users, "everyone", str(`{"denied_users":["bob"]}`), "everyone", true, false},
		{"known allow", users, "everyone", str(`{"allowed_users":["ann","bob"]}`), "everyone", true, false},
		{"unknown deny", users, "everyone", str(`{"denied_users":["ghost"]}`), "dm_only", false, true},
		{"one unknown among allowed", users, "everyone", str(`{"allowed_users":["ann","ghost"]}`), "dm_only", false, true},
		{"lookup error", users, "everyone", str(`{"denied_users":["db-down"]}`), "dm_only", false, true},
		{"unparseable rules", users, "everyone", str(`{not json`), "dm_only", false, true},
		{"no user lookup wired", nil, "everyone", str(`{"denied_users":["bob"]}`), "dm_only", false, true},
		{"dm_only stays dm_only", users, "dm_only", str(`{"denied_users":["bob"]}`), "dm_only", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &mapImportAdapter{users: tt.users}
			vis, rules, demoted := a.importPinVisibility(context.Background(), tt.vis, tt.rules)
			if vis != tt.wantVis || (rules != nil) != tt.wantRules || demoted != tt.wantDemoted {
				t.Fatalf("got (%q, rules kept %v, demoted %v), want (%q, %v, %v)",
					vis, rules != nil, demoted, tt.wantVis, tt.wantRules, tt.wantDemoted)
			}
		})
	}
}
