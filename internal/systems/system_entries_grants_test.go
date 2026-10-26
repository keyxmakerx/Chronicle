package systems

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

func (f *fakeEntryRepo) ListGrants(_ context.Context, camp, sys, user string) ([]EntryGrant, error) {
	out := []EntryGrant{}
	for _, g := range f.grants {
		if g.CampaignID == camp && g.SystemID == sys && (user == "" || g.UserID == user) {
			out = append(out, g)
		}
	}
	return out, nil
}
func (f *fakeEntryRepo) FindGrant(_ context.Context, camp, sys, field, user string) (*EntryGrant, error) {
	for _, g := range f.grants {
		if g.CampaignID == camp && g.SystemID == sys && g.FieldKey == field && g.UserID == user {
			c := g
			return &c, nil
		}
	}
	return nil, nil
}
func (f *fakeEntryRepo) PutGrant(_ context.Context, g *EntryGrant) error {
	for i := range f.grants {
		o := &f.grants[i]
		if o.CampaignID == g.CampaignID && o.SystemID == g.SystemID && o.FieldKey == g.FieldKey && o.UserID == g.UserID {
			o.Budget, o.EntryID, o.GrantedBy = g.Budget, 0, g.GrantedBy
			return nil
		}
	}
	f.nextID++
	g.ID = f.nextID
	f.grants = append(f.grants, *g)
	return nil
}
func (f *fakeEntryRepo) DeleteGrant(_ context.Context, camp string, id int64) (bool, error) {
	for i := range f.grants {
		if f.grants[i].ID == id && f.grants[i].CampaignID == camp {
			f.grants = append(f.grants[:i], f.grants[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeEntryRepo) ClaimGrant(_ context.Context, camp string, id int64) (bool, error) {
	for i := range f.grants {
		if f.grants[i].ID == id && f.grants[i].CampaignID == camp && f.grants[i].EntryID == 0 {
			f.grants[i].EntryID = -1
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeEntryRepo) SetGrantEntry(_ context.Context, camp string, id, entryID int64) error {
	for i := range f.grants {
		if f.grants[i].ID == id && f.grants[i].CampaignID == camp {
			f.grants[i].EntryID = entryID
		}
	}
	return nil
}

func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	if code == 0 {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	ae, ok := err.(*apperror.AppError)
	if !ok || ae.Code != code {
		t.Fatalf("want %d, got %v", code, err)
	}
}

func fairyIn() CreateSystemEntryInput {
	return CreateSystemEntryInput{FieldKey: "ancestry", Name: "Fairy", Properties: map[string]any{
		"ancestry_points": 9.0,
		"purchased_traits": []any{map[string]any{"name": "Wings", "cost": 2.0}, map[string]any{"name": "Glow", "cost": 1.0}},
	}}
}

// The permission is the only way a player writes an entry: none refuses,
// one lets them write exactly one, and renewing it lets them write another.
func TestSystemEntryService_PlayerNeedsGrant(t *testing.T) {
	ctx := context.Background()
	svc, repo := newEntrySvc(entryTestManifest())
	_, err := svc.Create(ctx, "c1", playerActor, fairyIn())
	wantCode(t, err, 403)

	_, err = svc.Grant(ctx, "c1", playerActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 3})
	wantCode(t, err, 403)
	_, err = svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "level", UserID: "pl"})
	wantCode(t, err, 422)
	_, err = svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 21})
	wantCode(t, err, 422)
	g, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 3})
	wantCode(t, err, 0)
	if ok, _ := svc.CanMake(ctx, "c1", "ancestry", "pl"); !ok {
		t.Fatal("an unspent permission should offer make-your-own")
	}
	if ok, _ := svc.CanMake(ctx, "c1", "ancestry", "other"); ok {
		t.Fatal("another player has no permission")
	}

	e, err := svc.Create(ctx, "c1", playerActor, fairyIn())
	wantCode(t, err, 0)
	if e.Status != EntryStatusDraft || e.Visibility != EntryVisibilityDirectors || e.CreatedBy != "pl" {
		t.Fatalf("a player's entry is a draft kept from others: %+v", e)
	}
	if e.Properties["ancestry_points"] != 3 {
		t.Fatalf("the budget is the Director's, got %v", e.Properties["ancestry_points"])
	}
	if repo.grants[0].EntryID != e.ID {
		t.Fatalf("the permission should be spent on entry %d, got %d", e.ID, repo.grants[0].EntryID)
	}
	if ok, _ := svc.CanMake(ctx, "c1", "ancestry", "pl"); ok {
		t.Fatal("a spent permission offers nothing")
	}
	second := fairyIn()
	second.Name = "Pixie"
	_, err = svc.Create(ctx, "c1", playerActor, second)
	wantCode(t, err, 403)

	if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 4}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Create(ctx, "c1", playerActor, second)
	wantCode(t, err, 0)
	if p.Properties["ancestry_points"] != 4 {
		t.Fatalf("a renewed permission carries its new budget, got %v", p.Properties["ancestry_points"])
	}

	wantCode(t, svc.RevokeGrant(ctx, "c1", playerActor, g.ID), 403)
	wantCode(t, svc.RevokeGrant(ctx, "c1", dirActor, g.ID), 0)
	wantCode(t, svc.RevokeGrant(ctx, "c1", dirActor, g.ID), 404)
}

// A player cannot choose who sees their entry, and can keep it only to a
// character they play.
func TestSystemEntryService_PlayerKeep(t *testing.T) {
	ctx := context.Background()
	m := entryTestManifest()
	svc := NewSystemEntryService(&fakeEntryRepo{}, func(context.Context, string) *SystemManifest { return m }, WithEntryCharacters(testCharacters))
	if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl"}); err != nil {
		t.Fatal(err)
	}
	in := fairyIn()
	in.Visibility = EntryVisibilityEveryone
	_, err := svc.Create(ctx, "c1", playerActor, in)
	wantCode(t, err, 403)

	in = fairyIn()
	in.ForEntityID = "wren" // Tyne's character, not pl's
	_, err = svc.Create(ctx, "c1", playerActor, in)
	wantCode(t, err, 403)

	in.ForEntityID = "ash"
	in.Status = EntryStatusSent
	e, err := svc.Create(ctx, "c1", playerActor, in)
	wantCode(t, err, 0)
	if e.Visibility != EntryVisibilityCharacter || e.ForEntityID != "ash" || e.Status != EntryStatusSent {
		t.Fatalf("kept to Ash and handed in: %+v", e)
	}
	if e.Properties["ancestry_points"] != 9.0 {
		t.Fatalf("a permission without a budget leaves the points as written, got %v", e.Properties["ancestry_points"])
	}
}

// Until a Director accepts it, a draft is seen by its writer and the
// Directors only, even when it is kept to the writer's character; after,
// the visibility the Director chose decides.
func TestSystemEntryService_DraftVisibility(t *testing.T) {
	ctx := context.Background()
	m := entryTestManifest()
	svc := NewSystemEntryService(&fakeEntryRepo{}, func(context.Context, string) *SystemManifest { return m }, WithEntryCharacters(testCharacters))
	if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl"}); err != nil {
		t.Fatal(err)
	}
	e, err := svc.Create(ctx, "c1", playerActor, fairyIn())
	if err != nil {
		t.Fatal(err)
	}
	names := func(actor EntryActor) []string {
		l, err := svc.List(ctx, "c1", "ancestry", actor)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, x := range l {
			out = append(out, x.Name)
		}
		return out
	}
	tyne := EntryActor{UserID: "tyne"}
	for _, tt := range []struct {
		actor EntryActor
		want  int
	}{{dirActor, 1}, {playerActor, 1}, {tyne, 0}, {EntryActor{}, 0}} {
		if got := names(tt.actor); len(got) != tt.want {
			t.Fatalf("%+v sees %v, want %d", tt.actor, got, tt.want)
		}
	}
	// The writer's pick list marks it as theirs and still a draft.
	ch, err := svc.Choices(WithChoiceViewerUser(context.Background(), "pl"), "c1", "ancestry")
	if err != nil || len(ch) != 1 || !ch[0].Mine || ch[0].Status != EntryStatusDraft || ch[0].EntryID != e.ID {
		t.Fatalf("writer's choice: %+v %v", ch, err)
	}
	if ch, _ := svc.Choices(WithChoiceViewerUser(context.Background(), "tyne"), "c1", "ancestry"); len(ch) != 0 {
		t.Fatalf("another player's pick list holds no draft: %+v", ch)
	}

	if _, err := svc.Update(ctx, "c1", dirActor, e.ID, UpdateSystemEntryInput{
		Status: patch.Of(EntryStatusAccepted), Visibility: patch.Of(EntryVisibilityEveryone),
	}); err != nil {
		t.Fatal(err)
	}
	if got := names(tyne); len(got) != 1 {
		t.Fatalf("an accepted entry for everyone reaches every player, got %v", got)
	}
}

// The writer edits and hands in their draft; a Director hands it back with a
// note or accepts it, after which only Directors change it.
func TestSystemEntryService_DraftReview(t *testing.T) {
	ctx := context.Background()
	svc, _ := newEntrySvc(entryTestManifest())
	if _, err := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl", Budget: 3}); err != nil {
		t.Fatal(err)
	}
	e, err := svc.Create(ctx, "c1", playerActor, fairyIn())
	if err != nil {
		t.Fatal(err)
	}
	other := EntryActor{UserID: "tyne"}
	tests := []struct {
		name   string
		actor  EntryActor
		in     UpdateSystemEntryInput
		code   int
		status string
	}{
		{"another player refused", other, UpdateSystemEntryInput{Summary: patch.Of("x")}, 403, ""},
		{"writer cannot choose who sees it", playerActor, UpdateSystemEntryInput{Visibility: patch.Of(EntryVisibilityEveryone)}, 403, ""},
		{"writer cannot write the note", playerActor, UpdateSystemEntryInput{ReviewNote: patch.Of("fine")}, 403, ""},
		{"writer cannot accept", playerActor, UpdateSystemEntryInput{Status: patch.Of(EntryStatusAccepted)}, 403, ""},
		{"writer cannot raise the budget", playerActor, UpdateSystemEntryInput{Properties: patch.Of(map[string]any{"ancestry_points": 12.0})}, 0, EntryStatusDraft},
		{"writer hands it in", playerActor, UpdateSystemEntryInput{Status: patch.Of(EntryStatusSent)}, 0, EntryStatusSent},
		{"bad status", dirActor, UpdateSystemEntryInput{Status: patch.Of("done")}, 422, ""},
		{"director asks for changes", dirActor, UpdateSystemEntryInput{Status: patch.Of(EntryStatusDraft), ReviewNote: patch.Of("Wings cost 3")}, 0, EntryStatusDraft},
		{"writer edits again", playerActor, UpdateSystemEntryInput{Summary: patch.Of("Tiny folk"), Status: patch.Of(EntryStatusSent)}, 0, EntryStatusSent},
		{"director accepts", dirActor, UpdateSystemEntryInput{Status: patch.Of(EntryStatusAccepted)}, 0, EntryStatusAccepted},
		{"writer locked out once accepted", playerActor, UpdateSystemEntryInput{Summary: patch.Of("y")}, 403, ""},
		{"accepted cannot go back to draft", dirActor, UpdateSystemEntryInput{Status: patch.Of(EntryStatusDraft)}, 422, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.Update(ctx, "c1", tt.actor, e.ID, tt.in)
			wantCode(t, err, tt.code)
			if tt.code != 0 {
				return
			}
			if got.Status != tt.status {
				t.Fatalf("status %q, want %q", got.Status, tt.status)
			}
			if got.Properties["ancestry_points"] != nil && got.Properties["ancestry_points"] != 3 && got.Properties["ancestry_points"] != 3.0 {
				t.Fatalf("budget drifted: %v", got.Properties["ancestry_points"])
			}
			if tt.name == "director asks for changes" && got.ReviewNote != "Wings cost 3" {
				t.Fatalf("note %q", got.ReviewNote)
			}
			if tt.status == EntryStatusAccepted && got.ReviewNote != "" {
				t.Fatalf("accepting clears the note, got %q", got.ReviewNote)
			}
		})
	}
}

// Taking the permission back stops the writer editing their draft.
func TestSystemEntryService_RevokedGrantLocksDraft(t *testing.T) {
	ctx := context.Background()
	svc, repo := newEntrySvc(entryTestManifest())
	g, _ := svc.Grant(ctx, "c1", dirActor, GrantInput{FieldKey: "ancestry", UserID: "pl"})
	e, err := svc.Create(ctx, "c1", playerActor, fairyIn())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeGrant(ctx, "c1", dirActor, g.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(ctx, "c1", playerActor, e.ID, UpdateSystemEntryInput{Summary: patch.Of("x")})
	wantCode(t, err, 403)
	if len(repo.grants) != 0 {
		t.Fatal("revoked")
	}
	if l, _ := svc.Grants(ctx, "c1", playerActor); len(l) != 0 {
		t.Fatalf("no permissions left: %+v", l)
	}
}
