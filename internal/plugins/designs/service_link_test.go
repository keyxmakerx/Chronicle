package designs

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func tokenOf(t *testing.T, st *ShareState) string {
	t.Helper()
	if !st.LinkOn || !strings.HasPrefix(st.LinkURL, LinkPath) {
		t.Fatalf("link state = %+v", st)
	}
	return strings.TrimPrefix(st.LinkURL, LinkPath)
}

func TestLinkRules(t *testing.T) {
	s, _, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "silver")

	// Nothing is reachable before the link is on, and a made-up token is the same.
	_, err := s.PreviewLink(ctx, "bob", "nothing")
	wantCode(t, err, http.StatusNotFound)

	st, err := s.SetLink(ctx, "A1", "alice", d.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	tok := tokenOf(t, st)
	if len(tok) < 22 {
		t.Errorf("token %q is shorter than 128 bits", tok)
	}
	if again, _ := s.SetLink(ctx, "A1", "alice", d.ID, true); tokenOf(t, again) != tok {
		t.Error("turning an on link on again changed its token")
	}

	t.Run("an owner on the same system sees their campaigns on it", func(t *testing.T) {
		p, err := s.PreviewLink(ctx, "bob", tok)
		if err != nil {
			t.Fatal(err)
		}
		if !p.SameSystem || len(p.Targets) != 1 || p.Targets[0].ID != "B1" || len(p.Targets[0].Lacks) != 1 {
			t.Errorf("preview = %+v", p)
		}
		if p, _ := s.PreviewLink(ctx, "alice", tok); len(p.Targets) != 1 || p.Targets[0].ID != "A1" {
			t.Errorf("alice has A1 and A2 but only A1 is draw steel: %+v", p.Targets)
		}
	})
	t.Run("an owner on another system sees no target", func(t *testing.T) {
		p, err := s.PreviewLink(ctx, "cara", tok)
		if err != nil || p.SameSystem || len(p.Targets) != 0 {
			t.Errorf("preview = %+v, %v", p, err)
		}
		_, err = s.TakeLink(ctx, "cara", tok, "C1")
		wantCode(t, err, http.StatusBadRequest)
	})
	t.Run("someone who owns no campaign learns nothing", func(t *testing.T) {
		for _, u := range []string{"dan", "pat", "stranger", ""} {
			_, err := s.PreviewLink(ctx, u, tok)
			wantCode(t, err, http.StatusForbidden)
			_, err = s.PreviewLink(ctx, u, "nothing")
			wantCode(t, err, http.StatusForbidden)
		}
	})
	t.Run("taking needs the target to be the reader's own campaign", func(t *testing.T) {
		_, err := s.TakeLink(ctx, "bob", tok, "C1")
		wantCode(t, err, http.StatusForbidden)
		_, err = s.TakeLink(ctx, "bob", tok, "missing")
		wantCode(t, err, http.StatusNotFound)
		got, err := s.TakeLink(ctx, "bob", tok, "B1")
		if err != nil {
			t.Fatal(err)
		}
		if got.CampaignID != "B1" || got.Origin != OriginShared || got.OriginRef != "Ashgate" || got.MakerUserID != "alice" {
			t.Errorf("taken = %+v", got)
		}
	})

	// Off means not found, and on again is a new link; the old one stays dead.
	if _, err := s.SetLink(ctx, "A1", "alice", d.ID, false); err != nil {
		t.Fatal(err)
	}
	_, err = s.PreviewLink(ctx, "bob", tok)
	wantCode(t, err, http.StatusNotFound)
	_, err = s.TakeLink(ctx, "bob", tok, "B1")
	wantCode(t, err, http.StatusNotFound)
	st, _ = s.SetLink(ctx, "A1", "alice", d.ID, true)
	if tokenOf(t, st) == tok {
		t.Error("a link turned on again reused its old token")
	}
	_, err = s.PreviewLink(ctx, "bob", tok)
	wantCode(t, err, http.StatusNotFound)
}

func TestNamedOwners(t *testing.T) {
	s, repo, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "oak")
	repo.search = []OwnerMatch{{UserID: "bob", DisplayName: "Bob Marsh", Campaigns: []OwnerCampaign{{Name: "Bramble", SystemID: "drawsteel"}}}, {UserID: "alice", DisplayName: "Alice Bo"}}

	if got, _ := s.SearchOwners(ctx, "A1", "alice", "b"); len(got) != 0 {
		t.Errorf("a one-letter search found %v", got)
	}
	if got, _ := s.SearchOwners(ctx, "A1", "alice", "bo"); len(got) != 1 || got[0].DisplayName != "Bob Marsh" {
		t.Errorf("search = %+v (the searcher is never listed)", got)
	}

	t.Run("adding refuses oneself, non-owners and owners on another system", func(t *testing.T) {
		for _, tt := range []struct{ who, msg string }{
			{"alice", "another owner"}, {"", "another owner"},
			{"pat", "does not own a campaign"},
			{"cara", "Cara has no campaign on this game system"},
		} {
			_, err := s.AddOwner(ctx, "A1", "alice", d.ID, tt.who)
			wantCode(t, err, http.StatusBadRequest)
			if !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("%q: error %q does not mention %q", tt.who, err, tt.msg)
			}
		}
		if len(repo.owners[d.ID]) != 0 {
			t.Error("a refused owner was stored")
		}
	})

	st, err := s.AddOwner(ctx, "A1", "alice", d.ID, "bob")
	if err != nil || len(st.Owners) != 1 {
		t.Fatalf("add = %+v, %v", st, err)
	}
	if l, _ := s.List(ctx, "A1", "alice", ""); !l[0].Shared {
		t.Error("a design with a named owner is not marked shared")
	}

	t.Run("Shared with me lists per target campaign of the same system", func(t *testing.T) {
		if got, _ := s.SharedWithMe(ctx, "B1", "bob", KindRoomTheme); len(got) != 1 || got[0].ID != d.ID {
			t.Errorf("bob in B1 = %+v", got)
		}
		if got, _ := s.SharedWithMe(ctx, "A1", "alice", ""); len(got) != 0 {
			t.Errorf("a campaign's own designs are not shared with it: %+v", got)
		}
		if got, _ := s.SharedWithMe(ctx, "C1", "cara", ""); len(got) != 0 {
			t.Errorf("cara was not named: %+v", got)
		}
	})
	t.Run("only a named owner reaches it, in a campaign they own on that system", func(t *testing.T) {
		_, err := s.PreviewShared(ctx, "C1", "cara", d.ID)
		wantCode(t, err, http.StatusNotFound)
		_, err = s.TakeShared(ctx, "C1", "cara", d.ID)
		wantCode(t, err, http.StatusNotFound)
		_, err = s.TakeShared(ctx, "A1", "bob", d.ID)
		wantCode(t, err, http.StatusForbidden)
		p, err := s.PreviewShared(ctx, "B1", "bob", d.ID)
		if err != nil || p.Name != "Ward" || !p.SameSystem {
			t.Fatalf("preview = %+v, %v", p, err)
		}
		got, err := s.TakeShared(ctx, "B1", "bob", d.ID)
		if err != nil || got.CampaignID != "B1" || got.Origin != OriginShared {
			t.Fatalf("take = %+v, %v", got, err)
		}
	})
	t.Run("a named owner whose campaign moved to another system is refused at take", func(t *testing.T) {
		repo.designs[d.ID].SystemID = "dnd5e"
		defer func() { repo.designs[d.ID].SystemID = "drawsteel" }()
		_, err := s.TakeShared(ctx, "B1", "bob", d.ID)
		wantCode(t, err, http.StatusBadRequest)
	})

	t.Run("removing an owner closes their access", func(t *testing.T) {
		if _, err := s.RemoveOwner(ctx, "A1", "alice", d.ID, "bob"); err != nil {
			t.Fatal(err)
		}
		_, err := s.TakeShared(ctx, "B1", "bob", d.ID)
		wantCode(t, err, http.StatusNotFound)
	})
}

func TestStopSharing(t *testing.T) {
	s, repo, _ := newWorld()
	ctx := context.Background()
	d := mustSave(t, s, "Ward", "oak")
	st, _ := s.SetLink(ctx, "A1", "alice", d.ID, true)
	tok := tokenOf(t, st)
	_, _ = s.AddOwner(ctx, "A1", "alice", d.ID, "bob")
	taken, _ := s.TakeLink(ctx, "bob", tok, "B1")

	st, err := s.StopSharing(ctx, "A1", "alice", d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.LinkOn || st.LinkURL != "" || len(st.Owners) != 0 {
		t.Errorf("state after stopping = %+v", st)
	}
	_, err = s.PreviewLink(ctx, "bob", tok)
	wantCode(t, err, http.StatusNotFound)
	_, err = s.TakeShared(ctx, "B1", "bob", d.ID)
	wantCode(t, err, http.StatusNotFound)
	if got, _ := s.SharedWithMe(ctx, "B1", "bob", ""); len(got) != 0 {
		t.Errorf("still shared: %+v", got)
	}
	if l, _ := s.List(ctx, "A1", "alice", ""); l[0].Shared {
		t.Error("the design still reads as shared")
	}
	// Copies already taken stay with their owners.
	if _, err := s.Get(ctx, "B1", "bob", taken.ID); err != nil {
		t.Errorf("the copy went with the share: %v", err)
	}
	_ = repo
}
