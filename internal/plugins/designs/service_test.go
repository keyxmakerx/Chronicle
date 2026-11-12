package designs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// world: alice owns A1 (draw steel) and A2 (dnd5e); bob owns B1 (draw steel);
// cara owns C1 (dnd5e). dan is a co-DM and pat a player in A1.
func newWorld() (*service, *memRepo, *fakeDir) {
	repo := newMemRepo()
	dir := &fakeDir{
		campaigns: map[string]CampaignInfo{
			"A1": {ID: "A1", Name: "Ashgate", SystemID: "drawsteel"},
			"A2": {ID: "A2", Name: "Alder", SystemID: "dnd5e"},
			"B1": {ID: "B1", Name: "Bramble", SystemID: "drawsteel"},
			"C1": {ID: "C1", Name: "Cairn", SystemID: "dnd5e"},
		},
		owners:  map[string]string{"A1": "alice", "A2": "alice", "B1": "bob", "C1": "cara"},
		names:   map[string]string{"alice": "Alice", "bob": "Bob", "cara": "Cara", "dan": "Dan", "pat": "Pat"},
		members: map[string]bool{"A1/dan": true, "A1/pat": true},
	}
	kinds := NewKinds()
	kinds.Register(KindRoomTheme, themeKind{lack: map[string]bool{"B1": true}})
	return NewService(repo, dir, kinds).(*service), repo, dir
}

func code(t *testing.T, s *service, id string) string {
	t.Helper()
	st, err := s.ShareState(context.Background(), "A1", "alice", id)
	if err != nil {
		t.Fatalf("share state: %v", err)
	}
	return st.Code
}

func mustSave(t *testing.T, s *service, name, part string) *Design {
	t.Helper()
	d, err := s.Save(context.Background(), "A1", "alice", SaveInput{Kind: KindRoomTheme, Name: name, Note: "warm", Data: json.RawMessage(`{"part":"` + part + `"}`)})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	return d
}

func wantCode(t *testing.T, err error, want int) {
	t.Helper()
	var ae *apperror.AppError
	if want == 0 {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if !errors.As(err, &ae) || ae.Code != want {
		t.Fatalf("error = %v, want HTTP %d", err, want)
	}
}

// Only the campaign's Owner reaches any call; a co-DM, a player, a stranger
// and no user at all are refused, so a service caller cannot skip the route gate.
func TestEveryCallRefusesNonOwners(t *testing.T) {
	s, _, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "oak")
	theCode := code(t, s, d.ID)
	calls := map[string]func(user string) error{
		"list": func(u string) error { _, err := s.List(ctx, "A1", u, ""); return err },
		"get":  func(u string) error { _, err := s.Get(ctx, "A1", u, d.ID); return err },
		"save": func(u string) error {
			_, err := s.Save(ctx, "A1", u, SaveInput{Kind: KindRoomTheme, Name: "x", Data: json.RawMessage(`{"part":"oak"}`)})
			return err
		},
		"update": func(u string) error {
			_, err := s.Update(ctx, "A1", u, d.ID, UpdateDesignInput{Name: patch.Of("y")})
			return err
		},
		"delete":      func(u string) error { return s.Delete(ctx, "A1", u, d.ID) },
		"share state": func(u string) error { _, err := s.ShareState(ctx, "A1", u, d.ID); return err },
		"link":        func(u string) error { _, err := s.SetLink(ctx, "A1", u, d.ID, true); return err },
		"search":      func(u string) error { _, err := s.SearchOwners(ctx, "A1", u, "bo"); return err },
		"add owner":   func(u string) error { _, err := s.AddOwner(ctx, "A1", u, d.ID, "bob"); return err },
		"remove":      func(u string) error { _, err := s.RemoveOwner(ctx, "A1", u, d.ID, "bob"); return err },
		"stop":        func(u string) error { _, err := s.StopSharing(ctx, "A1", u, d.ID); return err },
		"preview":     func(u string) error { _, err := s.PreviewCode(ctx, "A1", u, theCode); return err },
		"take code":   func(u string) error { _, err := s.TakeCode(ctx, "A1", u, theCode); return err },
		"shared list": func(u string) error { _, err := s.SharedWithMe(ctx, "A1", u, ""); return err },
		"take shared": func(u string) error { _, err := s.TakeShared(ctx, "A1", u, d.ID); return err },
	}
	for name, call := range calls {
		for _, user := range []string{"dan", "pat", "bob", "nobody", ""} {
			t.Run(name+"/"+user, func(t *testing.T) { wantCode(t, call(user), http.StatusForbidden) })
		}
		t.Run(name+"/owner", func(t *testing.T) {
			// Not every call succeeds for the owner (taking one's own design by
			// name has no share), but none may be a permission error.
			var ae *apperror.AppError
			if err := call("alice"); errors.As(err, &ae) && ae.Code == http.StatusForbidden {
				t.Fatalf("owner was refused: %v", err)
			}
		})
	}
}

func TestSaveRules(t *testing.T) {
	s, _, _ := newWorld()
	tests := []struct {
		name string
		in   SaveInput
		want int
		msg  string
	}{
		{"valid", SaveInput{Kind: KindRoomTheme, Name: " Ward ", Data: json.RawMessage(`{"part":"oak","extra":1}`)}, 0, ""},
		{"unknown kind is refused", SaveInput{Kind: "poster", Name: "x", Data: json.RawMessage(`{}`)}, 400, "not a kind"},
		{"the kind refuses an unknown part by name", SaveInput{Kind: KindRoomTheme, Name: "x", Data: json.RawMessage(`{"part":"lava"}`)}, 400, "unknown part"},
		{"empty name", SaveInput{Kind: KindRoomTheme, Name: "  ", Data: json.RawMessage(`{"part":"oak"}`)}, 400, "required"},
		{"name too long", SaveInput{Kind: KindRoomTheme, Name: strings.Repeat("n", 61), Data: json.RawMessage(`{"part":"oak"}`)}, 400, "at most 60"},
		{"note too long", SaveInput{Kind: KindRoomTheme, Name: "x", Note: strings.Repeat("n", 141), Data: json.RawMessage(`{"part":"oak"}`)}, 400, "at most 140"},
		{"no data", SaveInput{Kind: KindRoomTheme, Name: "x"}, 400, "missing"},
		{"data too large", SaveInput{Kind: KindRoomTheme, Name: "x", Data: json.RawMessage(`{"part":"oak","pad":"` + strings.Repeat("a", MaxDataBytes) + `"}`)}, 400, "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := s.Save(context.Background(), "A1", "alice", tt.in)
			wantCode(t, err, tt.want)
			if tt.want != 0 {
				if !strings.Contains(err.Error(), tt.msg) {
					t.Errorf("error %q does not mention %q", err, tt.msg)
				}
				return
			}
			if d.Name != "Ward" || string(d.Data) != `{"part":"oak"}` || d.SystemID != "drawsteel" || d.MakerName != "Alice" || d.Origin != OriginMade {
				t.Errorf("saved design = %+v", d)
			}
		})
	}
}

func TestLibraryIsFull(t *testing.T) {
	s, repo, _ := newWorld()
	for i := 0; i < MaxLibrary; i++ {
		_ = repo.Insert(context.Background(), &Design{ID: string(rune('a'+i%26)) + strings.Repeat("x", i), CampaignID: "A1"})
	}
	_, err := s.Save(context.Background(), "A1", "alice", SaveInput{Kind: KindRoomTheme, Name: "x", Data: json.RawMessage(`{"part":"oak"}`)})
	wantCode(t, err, http.StatusConflict)
}

func TestAnotherCampaignsDesignIsNotFound(t *testing.T) {
	s, _, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "oak")
	// Bob owns B1 and asks for Alice's design by id through his own campaign.
	_, err := s.Get(ctx, "B1", "bob", d.ID)
	wantCode(t, err, http.StatusNotFound)
	wantCode(t, s.Delete(ctx, "B1", "bob", d.ID), http.StatusNotFound)
	_, err = s.SetLink(ctx, "B1", "bob", d.ID, true)
	wantCode(t, err, http.StatusNotFound)
	_, err = s.Update(ctx, "B1", "bob", d.ID, UpdateDesignInput{Name: patch.Of("mine")})
	wantCode(t, err, http.StatusNotFound)
}

func patchOf(s string) patch.Field[string] { return patch.Of(s) }

func TestUpdateIsPartial(t *testing.T) {
	s, _, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "oak") // note "warm"
	tests := []struct {
		name     string
		in       UpdateDesignInput
		wantName string
		wantNote string
		wantErr  bool
	}{
		{"absent keys keep both", UpdateDesignInput{}, "Ward", "warm", false},
		{"a rename keeps the note", UpdateDesignInput{Name: patch.Of("Seal")}, "Seal", "warm", false},
		{"a note keeps the name", UpdateDesignInput{Note: patch.Of("cold")}, "Seal", "cold", false},
		{"null clears the note", UpdateDesignInput{Note: patch.Null[string]()}, "Seal", "", false},
		{"null keeps the name", UpdateDesignInput{Name: patch.Null[string]()}, "Seal", "", false},
		{"a blank name is refused", UpdateDesignInput{Name: patch.Of(" ")}, "Seal", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Update(ctx, "A1", "alice", d.ID, tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			cur, _ := s.Get(ctx, "A1", "alice", d.ID)
			if cur.Name != tt.wantName || cur.Note != tt.wantNote {
				t.Errorf("stored = %q/%q, want %q/%q", cur.Name, cur.Note, tt.wantName, tt.wantNote)
			}
			_ = got
		})
	}
}

func TestLikeEscape(t *testing.T) {
	for in, want := range map[string]string{"bob": "bob", "100%": "100|%", "a_b": "a|_b", "x|y": "x||y", "%_|": "|%|_||"} {
		if got := likeEscape(in); got != want {
			t.Errorf("likeEscape(%q) = %q, want %q", in, got, want)
		}
	}
}
