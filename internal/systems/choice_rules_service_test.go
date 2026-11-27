package systems

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeRuleRepo is an in-memory ChoiceRuleRepository.
type fakeRuleRepo struct {
	rules    []ChoiceRule
	requests []ChoiceRequest
	nextID   int64
	failList bool
}

func (f *fakeRuleRepo) List(_ context.Context, camp, sys, field string) ([]ChoiceRule, error) {
	out := []ChoiceRule{}
	for _, r := range f.rules {
		if r.CampaignID == camp && r.SystemID == sys && (field == "" || r.FieldKey == field) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRuleRepo) ListForViewer(ctx context.Context, camp, sys, field, user string) ([]ChoiceRule, error) {
	if f.failList {
		return nil, apperror.NewInternal(fmt.Errorf("db down"))
	}
	all, _ := f.List(ctx, camp, sys, field)
	out := []ChoiceRule{}
	for _, r := range all {
		if r.Target == "" || r.Target == user {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRuleRepo) Apply(_ context.Context, ups, dels []ChoiceRule) error {
	same := func(a, b ChoiceRule) bool {
		return a.CampaignID == b.CampaignID && a.SystemID == b.SystemID && a.FieldKey == b.FieldKey &&
			a.Source == b.Source && a.Key == b.Key && a.Target == b.Target
	}
	for _, d := range dels {
		for i := range f.rules {
			if same(f.rules[i], d) {
				f.rules = append(f.rules[:i], f.rules[i+1:]...)
				break
			}
		}
	}
	for _, u := range ups {
		found := false
		for i := range f.rules {
			if same(f.rules[i], u) {
				u.ID = f.rules[i].ID
				f.rules[i] = u
				found = true
			}
		}
		if !found {
			f.nextID++
			u.ID = f.nextID
			f.rules = append(f.rules, u)
		}
	}
	return nil
}

func (f *fakeRuleRepo) CountForEntry(_ context.Context, camp, sys, field, src, key string) (int, error) {
	n := 0
	for _, r := range f.rules {
		if r.CampaignID == camp && r.SystemID == sys && r.FieldKey == field && r.Source == src && r.Key == key {
			n++
		}
	}
	return n, nil
}

func (f *fakeRuleRepo) DeleteForEntry(_ context.Context, camp, sys, field, src, key string) (int, error) {
	n := 0
	kept := f.rules[:0]
	for _, r := range f.rules {
		if r.CampaignID == camp && r.SystemID == sys && r.FieldKey == field && r.Source == src && r.Key == key {
			n++
			continue
		}
		kept = append(kept, r)
	}
	f.rules = kept
	for i := range f.requests {
		q := &f.requests[i]
		if q.CampaignID == camp && q.FieldKey == field && q.Source == src && q.Key == key && q.Status == RequestWaiting {
			q.Status = RequestWithdrawn
		}
	}
	return n, nil
}

func (f *fakeRuleRepo) UpsertWaiting(_ context.Context, r *ChoiceRequest) error {
	for i := range f.requests {
		q := &f.requests[i]
		if q.Status == RequestWaiting && q.CampaignID == r.CampaignID && q.UserID == r.UserID && q.FieldKey == r.FieldKey && q.EntityID == r.EntityID {
			q.Source, q.Key, q.Name = r.Source, r.Key, r.Name
			r.ID = q.ID
			return nil
		}
	}
	f.nextID++
	r.ID, r.Status, r.CreatedAt = f.nextID, RequestWaiting, time.Now()
	f.requests = append(f.requests, *r)
	return nil
}

func (f *fakeRuleRepo) GetRequest(_ context.Context, camp string, id int64) (*ChoiceRequest, error) {
	for i := range f.requests {
		if f.requests[i].ID == id && f.requests[i].CampaignID == camp {
			c := f.requests[i]
			return &c, nil
		}
	}
	return nil, apperror.NewNotFound("request not found")
}

func (f *fakeRuleRepo) ListRequests(_ context.Context, camp, sys, status string) ([]ChoiceRequest, error) {
	out := []ChoiceRequest{}
	for _, q := range f.requests {
		if q.CampaignID == camp && q.SystemID == sys && q.Status == status {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f *fakeRuleRepo) UserRequests(_ context.Context, camp, sys, user, field string, since time.Time) ([]ChoiceRequest, error) {
	out := []ChoiceRequest{}
	for i := len(f.requests) - 1; i >= 0; i-- {
		q := f.requests[i]
		if q.CampaignID != camp || q.UserID != user || q.FieldKey != field {
			continue
		}
		if q.Status == RequestWaiting || (q.Status == RequestSuggested && q.DecidedAt != nil && !q.DecidedAt.Before(since)) {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f *fakeRuleRepo) HeroWaiting(_ context.Context, camp, user, entity string) ([]ChoiceRequest, error) {
	out := []ChoiceRequest{}
	for _, q := range f.requests {
		if q.CampaignID == camp && q.UserID == user && q.EntityID == entity && q.Status == RequestWaiting {
			out = append(out, q)
		}
	}
	return out, nil
}

func (f *fakeRuleRepo) Close(_ context.Context, camp string, id int64, status, msg, by string) (bool, error) {
	for i := range f.requests {
		q := &f.requests[i]
		if q.ID == id && q.CampaignID == camp && q.Status == RequestWaiting {
			now := time.Now()
			q.Status, q.Message, q.DecidedBy, q.DecidedAt = status, msg, by, &now
			return true, nil
		}
	}
	return false, nil
}

// testChoices is a CharacterChoiceService over a fixed package list plus the
// real entry service's own entries, so visibility is the production rule, and
// the limiter is applied the way the real service applies it.
type testChoices struct {
	pkg     []Choice
	entries SystemEntryService
	limiter ChoiceLimiter
}

func (t *testChoices) CharacterChoices(ctx context.Context, c, k string) ([]Choice, error) {
	l, err := t.CharacterChoiceList(ctx, c, k)
	if err != nil {
		return nil, err
	}
	return l.Choices, nil
}

func (t *testChoices) CharacterChoiceList(ctx context.Context, campaignID, field string) (*ChoiceList, error) {
	l := &ChoiceList{FieldKey: field, SystemID: "sys", SystemName: "Test System", Choices: append([]Choice{}, t.pkg...)}
	extra, err := t.entries.Choices(ctx, campaignID, field)
	if err != nil {
		return nil, err
	}
	l.Choices = append(l.Choices, extra...)
	if t.limiter != nil {
		if err := t.limiter.Limit(ctx, campaignID, l); err != nil {
			return nil, err
		}
	}
	return l, nil
}

func (t *testChoices) CreatorPlan(context.Context, string) (*CreatorPlan, error) {
	return &CreatorPlan{}, nil
}

func (t *testChoices) RulesTerms(context.Context, string) (*RulesTerms, error) {
	return &RulesTerms{Terms: map[string]RulesTerm{}}, nil
}

func (t *testChoices) SetChoiceLimiter(l ChoiceLimiter) { t.limiter = l }
func (t *testChoices) SetChoiceLooks(CardLookReader) {}
func (t *testChoices) SetChoicePins(ChoicePinReader) {}

type fakeHeroes struct {
	heroes  []ChoiceHero
	members []ChoiceMember
	set     []string
	setOK   bool
}

func (f *fakeHeroes) Heroes(context.Context, string) ([]ChoiceHero, error) { return f.heroes, nil }
func (f *fakeHeroes) Members(context.Context, string) ([]ChoiceMember, error) {
	return f.members, nil
}
func (f *fakeHeroes) SetField(_ context.Context, _, entity, owner, field, value string) (bool, error) {
	f.set = append(f.set, entity+"|"+owner+"|"+field+"|"+value)
	return f.setOK, nil
}

type ruleEnv struct {
	svc     ChoiceRuleService
	repo    *fakeRuleRepo
	entries SystemEntryService
	erepo   *fakeEntryRepo
	heroes  *fakeHeroes
	choices *testChoices
}

const camp = "camp1"

func newRuleEnv(t *testing.T) *ruleEnv {
	t.Helper()
	m := entryTestManifest()
	erepo := &fakeEntryRepo{}
	resolver := EntryManifestResolver(func(context.Context, string) *SystemManifest { return m })
	rrepo := &fakeRuleRepo{}
	heroes := &fakeHeroes{setOK: true, members: []ChoiceMember{{UserID: "pat", Name: "Pat"}, {UserID: "lee", Name: "Lee"}, {UserID: "dm", Name: "DM"}}}
	choices := &testChoices{pkg: []Choice{
		{Key: "fury", Slug: "fury", Name: "Fury", Source: ChoiceSourcePackage},
		{Key: "devil", Slug: "devil", Name: "Devil", Source: ChoiceSourcePackage},
		{Key: "human", Slug: "human", Name: "Human", Source: ChoiceSourcePackage},
	}}
	svc := NewChoiceRuleService(rrepo, erepo, choices, resolver, WithChoiceHeroes(heroes))
	entries := NewSystemEntryService(erepo, resolver, WithEntryRules(svc.(EntryRuleForgetter)))
	choices.entries = entries
	choices.limiter = svc
	return &ruleEnv{svc: svc, repo: rrepo, entries: entries, erepo: erepo, heroes: heroes, choices: choices}
}

func (e *ruleEnv) homebrew(t *testing.T, name, visibility string) *SystemEntry {
	t.Helper()
	en, err := e.entries.Create(context.Background(), camp, dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: name, Visibility: visibility})
	if err != nil {
		t.Fatal(err)
	}
	return en
}

func (e *ruleEnv) ctxFor(user string) context.Context {
	return WithChoiceFor(WithChoiceViewerUser(WithChoiceViewer(context.Background(), false), user), user)
}

func (e *ruleEnv) mark(t *testing.T, source, key, target, state string) {
	t.Helper()
	_, err := e.svc.SetMarks(context.Background(), camp, dirActor, ChoiceMarksInput{FieldKey: "ancestry",
		Marks: []ChoiceMark{{Source: source, Key: key, Target: target, State: state}}})
	if err != nil {
		t.Fatalf("mark %s/%s for %q = %s: %v", source, key, target, state, err)
	}
}

func listNames(l *ChoiceList) []string {
	out := []string{}
	for _, c := range l.Choices {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

func TestChoiceRuleService_LimitsEveryReader(t *testing.T) {
	env := newRuleEnv(t)
	moth := env.homebrew(t, "Mothfolk", EntryVisibilityEveryone)
	env.homebrew(t, "Secret", EntryVisibilityDirectors)
	env.mark(t, "package", "fury", "", RuleNotOffered)
	env.mark(t, "package", "fury", "pat", RuleOpen)
	env.mark(t, "campaign", "*", "", RuleNotOffered)
	env.mark(t, "campaign", fmt.Sprint(moth.ID), "pat", RuleAskFirst)

	ctx := context.Background()
	pat, err := env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(listNames(pat), ","); got != "Devil,Fury,Human,Mothfolk" {
		t.Fatalf("Pat gets %s, want Furies back and Mothfolk, asking first", got)
	}
	for _, c := range pat.Choices {
		if (c.Name == "Mothfolk") != (c.State == ChoiceStateAskFirst) {
			t.Errorf("%s state = %q", c.Name, c.State)
		}
	}

	lee, err := env.choices.CharacterChoiceList(env.ctxFor("lee"), camp, "ancestry")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(listNames(lee), ","); got != "Devil,Human" {
		t.Fatalf("Lee gets %s, want no Fury and no homebrew", got)
	}
	// The Directors-only entry is never named or counted for a player.
	if lee.Rules == nil || lee.Rules.NotHereCount != 2 {
		t.Fatalf("Lee's rules = %+v, want 2 not here (Fury, Mothfolk) and never the hidden entry", lee.Rules)
	}
	for _, n := range lee.Rules.NotHere {
		if n.Name == "Secret" {
			t.Fatal("a Directors-only entry was named to a player")
		}
	}

	// Directors, and a context that asked for the list as it is, are not limited.
	dm, err := env.choices.CharacterChoiceList(WithChoiceViewer(ctx, true), camp, "ancestry")
	if err != nil || len(dm.Choices) != 5 || dm.Rules != nil {
		t.Fatalf("Director list = %d entries, rules %+v, err %v; want all 5, unlimited", len(dm.Choices), dm.Rules, err)
	}
	staff, err := env.choices.CharacterChoiceList(WithChoiceUnlimited(env.ctxFor("scribe")), camp, "ancestry")
	if err != nil || len(staff.Choices) != 4 {
		t.Fatalf("unlimited reader = %d entries, err %v; want the 4 a player can read, rules aside", len(staff.Choices), err)
	}
}

func TestChoiceRuleService_FailsClosed(t *testing.T) {
	env := newRuleEnv(t)
	env.repo.failList = true
	if _, err := env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry"); err == nil {
		t.Fatal("a rules read that fails must fail the list, not serve it unlimited")
	}
}

func TestChoiceRuleService_GrantIsOpenForItsPlayer(t *testing.T) {
	env := newRuleEnv(t)
	env.mark(t, "package", "human", "", RuleRequired)
	// A player's own entry (made with a Director's permission) stays on a
	// short list for them alone.
	if _, err := env.entries.Grant(context.Background(), camp, dirActor, GrantInput{FieldKey: "ancestry", UserID: "pat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.entries.Create(context.Background(), camp, EntryActor{UserID: "pat"}, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Pat's Own"}); err != nil {
		t.Fatal(err)
	}
	pat, _ := env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry")
	if got := strings.Join(listNames(pat), ","); got != "Human,Pat's Own" || pat.Rules.Prefilled != "" {
		t.Fatalf("Pat gets %s (prefilled %q), want the short list plus their own entry, nothing filled in", got, pat.Rules.Prefilled)
	}
	lee, _ := env.choices.CharacterChoiceList(env.ctxFor("lee"), camp, "ancestry")
	if got := strings.Join(listNames(lee), ","); got != "Human" || lee.Rules.Prefilled != "Human" {
		t.Fatalf("Lee gets %s (prefilled %q), want Human filled in", got, lee.Rules.Prefilled)
	}
}

func TestChoiceRuleService_DirectorsOnly(t *testing.T) {
	env := newRuleEnv(t)
	scribe := EntryActor{UserID: "scribe"}
	player := EntryActor{UserID: "pat"}
	ctx := context.Background()
	marks := ChoiceMarksInput{FieldKey: "ancestry", Marks: []ChoiceMark{{Source: "package", Key: "fury", State: RuleNotOffered}}}
	calls := map[string]func(a EntryActor) error{
		"rules":     func(a EntryActor) error { _, err := env.svc.Rules(ctx, camp, "", a); return err },
		"set marks": func(a EntryActor) error { _, err := env.svc.SetMarks(ctx, camp, a, marks); return err },
		"check":     func(a EntryActor) error { _, err := env.svc.CheckMarks(ctx, camp, a, marks); return err },
		"entries":   func(a EntryActor) error { _, err := env.svc.DirectorEntries(ctx, camp, a, "ancestry", ""); return err },
		"count":     func(a EntryActor) error { _, err := env.svc.RuleCountForEntry(ctx, camp, a, 1); return err },
		"requests":  func(a EntryActor) error { _, err := env.svc.WaitingRequests(ctx, camp, a); return err },
		"allow":     func(a EntryActor) error { _, err := env.svc.AllowRequest(ctx, camp, a, 1); return err },
		"suggest":   func(a EntryActor) error { return env.svc.SuggestAnother(ctx, camp, a, 1, "x") },
	}
	for name, call := range calls {
		for who, actor := range map[string]EntryActor{"scribe": scribe, "player": player} {
			t.Run(name+" refused to a "+who, func(t *testing.T) {
				err := call(actor)
				ae, ok := err.(*apperror.AppError)
				if !ok || ae.Code != 403 {
					t.Fatalf("err = %v, want 403", err)
				}
			})
		}
	}
	if len(env.repo.rules) != 0 {
		t.Fatal("a refused call wrote a rule")
	}
}

func TestChoiceRuleService_SetMarksValidation(t *testing.T) {
	env := newRuleEnv(t)
	secret := env.homebrew(t, "Secret", EntryVisibilityDirectors)
	open := env.homebrew(t, "Open Homebrew", EntryVisibilityEveryone)
	ctx := context.Background()
	tests := []struct {
		name    string
		marks   []ChoiceMark
		wantErr string
	}{
		{"ok", []ChoiceMark{{Source: "package", Key: "fury", State: RuleNotOffered}}, ""},
		{"unknown entry", []ChoiceMark{{Source: "package", Key: "nope", State: RuleNotOffered}}, "isn't on this list"},
		{"a package key under the campaign source", []ChoiceMark{{Source: "campaign", Key: "fury", State: RuleNotOffered}}, "isn't on this list"},
		{"bad state", []ChoiceMark{{Source: "package", Key: "fury", State: "maybe"}}, "open, ask first"},
		{"bad source", []ChoiceMark{{Source: "web", Key: "fury", State: RuleNotOffered}}, "of the game or of this campaign"},
		{"not a member", []ChoiceMark{{Source: "package", Key: "fury", Target: "stranger", State: RuleNotOffered}}, "isn't a member"},
		{"required for players can't name a Directors-only entry",
			[]ChoiceMark{{Source: "campaign", Key: fmt.Sprint(secret.ID), State: RuleRequired}}, "Players can't see Secret yet"},
		{"required for one player can't name an entry they can't read",
			[]ChoiceMark{{Source: "campaign", Key: fmt.Sprint(secret.ID), Target: "pat", State: RuleRequired}}, "can't see Secret yet"},
		{"required on a readable homebrew entry", []ChoiceMark{{Source: "campaign", Key: fmt.Sprint(open.ID), State: RuleRequired}}, ""},
		{"not offered may name a hidden entry", []ChoiceMark{{Source: "campaign", Key: fmt.Sprint(secret.ID), State: RuleNotOffered}}, ""},
		{"a group can be required", []ChoiceMark{{Source: "campaign", Key: "*", State: RuleRequired}}, ""},
		{"no marks", nil, "between 1 and"},
		{"a reason that is too long", []ChoiceMark{{Source: "package", Key: "fury", State: RuleNotOffered, Reason: strp(strings.Repeat("x", maxRuleReason+1))}}, "at most"},
		{"one bad mark applies none", []ChoiceMark{{Source: "package", Key: "devil", State: RuleNotOffered}, {Source: "package", Key: "nope", State: RuleNotOffered}}, "isn't on this list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env.repo.rules = nil
			_, err := env.svc.SetMarks(ctx, camp, dirActor, ChoiceMarksInput{FieldKey: "ancestry", Marks: tt.marks})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
			if len(env.repo.rules) != 0 {
				t.Fatalf("a refused batch left rules behind: %+v", env.repo.rules)
			}
		})
	}
	if _, err := env.svc.SetMarks(ctx, camp, dirActor, ChoiceMarksInput{FieldKey: "level", Marks: []ChoiceMark{{Source: "package", Key: "fury", State: RuleNotOffered}}}); err == nil {
		t.Fatal("a field with no pick list was accepted")
	}
}

func strp(s string) *string { return &s }

func TestChoiceRuleService_MarksReplaceAndClear(t *testing.T) {
	env := newRuleEnv(t)
	ctx := context.Background()
	set := func(m ChoiceMark) []ChoiceRule {
		rules, err := env.svc.SetMarks(ctx, camp, dirActor, ChoiceMarksInput{FieldKey: "ancestry", Marks: []ChoiceMark{m}})
		if err != nil {
			t.Fatal(err)
		}
		return rules
	}
	rules := set(ChoiceMark{Source: "package", Key: "fury", State: RuleNotOffered, Reason: strp("too strong")})
	if len(rules) != 1 || rules[0].Reason != "too strong" || rules[0].Name != "Fury" {
		t.Fatalf("rules = %+v", rules)
	}
	// A save that says nothing of the reason keeps it; saying it replaces it.
	rules = set(ChoiceMark{Source: "package", Key: "fury", State: RuleAskFirst})
	if len(rules) != 1 || rules[0].State != RuleAskFirst || rules[0].Reason != "too strong" {
		t.Fatalf("a state change lost the reason: %+v", rules)
	}
	rules = set(ChoiceMark{Source: "package", Key: "fury", State: RuleAskFirst, Reason: strp("")})
	if rules[0].Reason != "" {
		t.Fatalf("reason = %q, want cleared", rules[0].Reason)
	}
	// Open for everyone is the default, so it removes the mark; for a player
	// it is an exception and is stored.
	set(ChoiceMark{Source: "package", Key: "fury", Target: "pat", State: RuleOpen})
	rules = set(ChoiceMark{Source: "package", Key: "fury", State: RuleOpen})
	if len(rules) != 1 || rules[0].Target != "pat" || rules[0].State != RuleOpen {
		t.Fatalf("rules = %+v, want only Pat's exception", rules)
	}
	rules = set(ChoiceMark{Source: "package", Key: "fury", Target: "pat", State: "clear"})
	if len(rules) != 0 {
		t.Fatalf("rules = %+v, want none", rules)
	}
	// A mark for an entry that has since left the package can still be cleared.
	env.repo.rules = []ChoiceRule{{CampaignID: camp, SystemID: "sys", FieldKey: "ancestry", Source: "package", Key: "gone", State: RuleNotOffered}}
	if rules = set(ChoiceMark{Source: "package", Key: "gone", State: "clear"}); len(rules) != 0 {
		t.Fatalf("an orphaned rule could not be cleared: %+v", rules)
	}
}

func TestChoiceRuleService_DeleteEntryDeletesItsRules(t *testing.T) {
	env := newRuleEnv(t)
	moth := env.homebrew(t, "Mothfolk", EntryVisibilityEveryone)
	other := env.homebrew(t, "Other", EntryVisibilityEveryone)
	key := fmt.Sprint(moth.ID)
	env.mark(t, "campaign", key, "", RuleAskFirst)
	env.mark(t, "campaign", key, "pat", RuleOpen)
	env.mark(t, "campaign", fmt.Sprint(other.ID), "", RuleNotOffered)
	env.mark(t, "package", "fury", "", RuleNotOffered)
	ctx := context.Background()

	n, err := env.svc.RuleCountForEntry(ctx, camp, dirActor, moth.ID)
	if err != nil || n != 2 {
		t.Fatalf("count = %d, %v; want 2 (\"in 2 hero rules\")", n, err)
	}
	if _, err := env.svc.RequestChoice(env.ctxFor("lee"), ChoiceRequestInput{CampaignID: camp, UserID: "lee", FieldKey: "ancestry", EntryName: "Mothfolk"}); err != nil {
		t.Fatal(err)
	}
	if err := env.entries.Delete(ctx, camp, dirActor, moth.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := env.repo.CountForEntry(ctx, camp, "sys", "ancestry", "campaign", key); n != 0 {
		t.Fatalf("%d rules left for the deleted entry", n)
	}
	if len(env.repo.rules) != 2 {
		t.Fatalf("rules = %+v, want the other entry's and the package's left alone", env.repo.rules)
	}
	if env.repo.requests[0].Status != RequestWithdrawn {
		t.Fatalf("the request waiting on the deleted entry is %s, want withdrawn", env.repo.requests[0].Status)
	}
	// A new entry of the same name starts clean: rules follow the id, not the name.
	again := env.homebrew(t, "Mothfolk", EntryVisibilityEveryone)
	if n, _ := env.repo.CountForEntry(ctx, camp, "sys", "ancestry", "campaign", fmt.Sprint(again.ID)); n != 0 {
		t.Fatal("a new entry inherited the old entry's rules")
	}
}

func TestChoiceRuleService_AskFirstRequests(t *testing.T) {
	env := newRuleEnv(t)
	env.mark(t, "package", "fury", "", RuleAskFirst)
	ctx := context.Background()
	ask := func(user, name string) (*ChoiceRequestOutcome, error) {
		return env.svc.RequestChoice(env.ctxFor(user), ChoiceRequestInput{CampaignID: camp, UserID: user, EntityID: "hero1", EntityName: "Tess", FieldKey: "ancestry", EntryName: name})
	}
	if _, err := ask("pat", "Human"); err == nil {
		t.Fatal("an open entry was recorded as a request")
	}
	if _, err := ask("pat", "No Such"); err == nil {
		t.Fatal("an unknown entry was recorded as a request")
	}
	out, err := ask("pat", "fury")
	if err != nil || out.Status != RequestWaiting || out.EntryName != "Fury" {
		t.Fatalf("outcome = %+v, %v", out, err)
	}
	if len(env.heroes.set) != 0 {
		t.Fatal("asking wrote the hero's field")
	}
	// Asking again for the same hero and field replaces the request.
	if _, err := ask("pat", "Fury"); err != nil || len(env.repo.requests) != 1 {
		t.Fatalf("requests = %d, %v; want one", len(env.repo.requests), err)
	}

	// The player sees their own waiting pick on the list; others do not.
	pat, _ := env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry")
	if pat.Rules == nil || pat.Rules.Waiting == nil || pat.Rules.Waiting.EntryName != "Fury" {
		t.Fatalf("Pat's rules = %+v, want the waiting pick", pat.Rules)
	}
	lee, _ := env.choices.CharacterChoiceList(env.ctxFor("lee"), camp, "ancestry")
	if lee.Rules.Waiting != nil {
		t.Fatal("another player saw Pat's waiting pick")
	}

	waiting, err := env.svc.WaitingRequests(ctx, camp, dirActor)
	if err != nil || len(waiting) != 1 || waiting[0].PlayerName != "Pat" || waiting[0].EntityName != "Tess" || waiting[0].FieldLabel != "ancestry" {
		t.Fatalf("waiting = %+v, %v", waiting, err)
	}

	// Allow: an open mark for that player, the hero's field set, request closed.
	res, err := env.svc.AllowRequest(ctx, camp, dirActor, waiting[0].ID)
	if err != nil || !res.HeroUpdated {
		t.Fatalf("allow = %+v, %v", res, err)
	}
	if got := strings.Join(env.heroes.set, ";"); got != "hero1|pat|ancestry|Fury" {
		t.Fatalf("hero writes = %q", got)
	}
	var open bool
	for _, r := range env.repo.rules {
		if r.Target == "pat" && r.Key == "fury" && r.State == RuleOpen {
			open = true
		}
	}
	if !open {
		t.Fatalf("rules = %+v, want Pat's open mark for Fury", env.repo.rules)
	}
	patAfter, _ := env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry")
	for _, c := range patAfter.Choices {
		if c.Name == "Fury" && c.State != "" {
			t.Fatal("Fury still asks first for Pat after being allowed")
		}
	}
	if _, err := env.svc.AllowRequest(ctx, camp, dirActor, waiting[0].ID); err == nil {
		t.Fatal("an answered request was allowed twice")
	}
}

func TestChoiceRuleService_SuggestAnother(t *testing.T) {
	env := newRuleEnv(t)
	env.mark(t, "package", "fury", "", RuleAskFirst)
	ctx := context.Background()
	out, err := env.svc.RequestChoice(env.ctxFor("pat"), ChoiceRequestInput{CampaignID: camp, UserID: "pat", EntityID: "h", FieldKey: "ancestry", EntryName: "Fury"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.svc.SuggestAnother(ctx, camp, dirActor, out.ID, "  Try Devil  "); err != nil {
		t.Fatal(err)
	}
	if len(env.heroes.set) != 0 {
		t.Fatal("suggesting another touched the hero")
	}
	pat, _ := env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry")
	if pat.Rules.Waiting != nil || pat.Rules.Suggested == nil || pat.Rules.Suggested.Message != "Try Devil" {
		t.Fatalf("Pat's rules = %+v, want the Director's note and nothing waiting", pat.Rules)
	}
	if err := env.svc.SuggestAnother(ctx, camp, dirActor, out.ID, "again"); err == nil {
		t.Fatal("an answered request was answered twice")
	}
	if left, _ := env.svc.WaitingRequests(ctx, camp, dirActor); len(left) != 0 {
		t.Fatalf("waiting = %+v, want none", left)
	}
	// A note kept too long is not shown for ever.
	env.repo.requests[0].DecidedAt = timePtr(time.Now().Add(-2 * suggestedShownFor))
	pat, _ = env.choices.CharacterChoiceList(env.ctxFor("pat"), camp, "ancestry")
	if pat.Rules.Suggested != nil {
		t.Fatal("an old suggestion is still shown")
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestChoiceRuleService_AllowWhenTheHeroIsGone(t *testing.T) {
	env := newRuleEnv(t)
	env.heroes.setOK = false
	env.mark(t, "package", "fury", "", RuleAskFirst)
	out, _ := env.svc.RequestChoice(env.ctxFor("pat"), ChoiceRequestInput{CampaignID: camp, UserID: "pat", EntityID: "h", FieldKey: "ancestry", EntryName: "Fury"})
	res, err := env.svc.AllowRequest(context.Background(), camp, dirActor, out.ID)
	if err != nil || res.HeroUpdated {
		t.Fatalf("allow = %+v, %v; want success with the hero left alone", res, err)
	}
}

func TestChoiceRuleService_CheckMarksCountsHeroesThatWouldBreak(t *testing.T) {
	env := newRuleEnv(t)
	env.heroes.heroes = []ChoiceHero{
		{ID: "h1", Name: "Tess", OwnerUserID: "pat", Fields: map[string]string{"ancestry": "Fury"}},
		{ID: "h2", Name: "Bram", OwnerUserID: "lee", Fields: map[string]string{"ancestry": "fury"}},
		{ID: "h3", Name: "Dax", OwnerUserID: "lee", Fields: map[string]string{"ancestry": "Human"}},
		{ID: "h4", Name: "Nobody", OwnerUserID: "", Fields: map[string]string{"ancestry": "Fury"}},
		{ID: "h5", Name: "Blank", OwnerUserID: "pat", Fields: map[string]string{}},
	}
	ctx := context.Background()
	check := func(marks ...ChoiceMark) *MarksCheck {
		res, err := env.svc.CheckMarks(ctx, camp, dirActor, ChoiceMarksInput{FieldKey: "ancestry", Marks: marks})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := check(ChoiceMark{Source: "package", Key: "fury", State: RuleNotOffered})
	if res.Count != 2 || len(res.Heroes) != 2 || res.Heroes[0].HeroName != "Tess" || res.Heroes[0].PlayerName != "Pat" || res.Heroes[1].PlayerName != "Lee" {
		t.Fatalf("check = %+v, want Tess (Pat) and Bram (Lee)", res)
	}
	if res.Heroes[0].Kind != BreakNotOffered {
		t.Fatalf("kind = %s", res.Heroes[0].Kind)
	}
	res = check(ChoiceMark{Source: "package", Key: "fury", State: RuleNotOffered}, ChoiceMark{Source: "package", Key: "fury", Target: "pat", State: RuleOpen})
	if res.Count != 1 || res.Heroes[0].HeroName != "Bram" {
		t.Fatalf("check = %+v, want only Bram once Pat is excepted", res)
	}
	if res = check(ChoiceMark{Source: "package", Key: "fury", State: RuleAskFirst}); res.Count != 0 {
		t.Fatalf("an ask-first mark breaks %d heroes, want none: a hero holding it counts as allowed", res.Count)
	}
	res = check(ChoiceMark{Source: "package", Key: "human", State: RuleRequired})
	if res.Count != 2 {
		t.Fatalf("a short list of Human breaks %d heroes, want the two that hold Fury", res.Count)
	}
	// Nothing was saved by asking.
	if len(env.repo.rules) != 0 {
		t.Fatal("the dry run wrote a rule")
	}
	// A hero that already breaks the stored rule is not counted again.
	env.mark(t, "package", "fury", "", RuleNotOffered)
	if res = check(ChoiceMark{Source: "package", Key: "fury", Target: "lee", State: RuleNotOffered, Reason: strp("x")}); res.Count != 0 {
		t.Fatalf("a hero that already broke the rule was counted: %+v", res)
	}
}

func TestChoiceRuleService_HeroBreaks(t *testing.T) {
	env := newRuleEnv(t)
	env.mark(t, "package", "fury", "", RuleNotOffered)
	ctx := context.Background()
	hero := ChoiceHero{ID: "h", Name: "Tess", OwnerUserID: "pat", Fields: map[string]string{"ancestry": "Fury"}}
	got, err := env.svc.HeroBreaks(ctx, camp, hero)
	if err != nil || len(got) != 1 || got[0].FieldKey != "ancestry" || got[0].Kind != BreakNotOffered {
		t.Fatalf("breaks = %+v, %v", got, err)
	}
	hero.Fields["ancestry"] = "Human"
	if got, _ = env.svc.HeroBreaks(ctx, camp, hero); len(got) != 0 {
		t.Fatalf("breaks = %+v, want none", got)
	}
	if got, _ = env.svc.HeroBreaks(ctx, camp, ChoiceHero{Fields: map[string]string{"ancestry": "Fury"}}); len(got) != 0 {
		t.Fatal("a hero nobody plays was flagged")
	}
}

func TestChoiceRuleService_DirectorEntries(t *testing.T) {
	env := newRuleEnv(t)
	moth := env.homebrew(t, "Mothfolk", EntryVisibilityEveryone)
	env.homebrew(t, "Secret", EntryVisibilityDirectors)
	env.homebrew(t, "Devil", EntryVisibilityEveryone) // shares a name with the game's own
	env.mark(t, "package", "fury", "", RuleNotOffered)
	env.mark(t, "package", "fury", "pat", RuleOpen)
	env.mark(t, "campaign", "*", "", RuleAskFirst)
	env.repo.rules = append(env.repo.rules, ChoiceRule{CampaignID: camp, SystemID: "sys", FieldKey: "ancestry", Source: "package", Key: "vanished", Name: "Old", State: RuleNotOffered})
	ctx := context.Background()

	all, err := env.svc.DirectorEntries(ctx, camp, dirActor, "ancestry", "")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]DirectorEntry{}
	for _, e := range all.Entries {
		byName[e.Source+"/"+e.Name] = e
	}
	if len(all.Entries) != 6 {
		t.Fatalf("entries = %d, want the game's 3 and the campaign's 3, hidden included", len(all.Entries))
	}
	fury := byName["package/Fury"]
	if fury.State != RuleNotOffered || fury.Everyone == nil || len(fury.Players) != 1 || fury.Players[0].PlayerName != "Pat" || fury.RuleCount != 2 {
		t.Fatalf("Fury = %+v", fury)
	}
	if m := byName["campaign/Mothfolk"]; m.State != RuleAskFirst || !m.Inherited || m.EntryID != moth.ID {
		t.Fatalf("Mothfolk = %+v, want ask first inherited from the group", m)
	}
	if len(all.Groups) != 2 || all.Groups[0].Source != "campaign" || all.Groups[0].Everyone == nil || all.Groups[0].Everyone.State != RuleAskFirst {
		t.Fatalf("groups = %+v", all.Groups)
	}
	if len(all.Orphans) != 1 || all.Orphans[0].Name != "Old" {
		t.Fatalf("orphans = %+v", all.Orphans)
	}
	if len(all.Warnings) != 1 || !strings.Contains(all.Warnings[0], "Devil") {
		t.Fatalf("warnings = %v, want the name clash", all.Warnings)
	}

	// As Pat: only what Pat can read, in the state Pat gets it.
	asPat, err := env.svc.DirectorEntries(ctx, camp, dirActor, "ancestry", "pat")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range asPat.Entries {
		if e.Name == "Secret" {
			t.Fatal("the as-player list shows an entry that player can't read")
		}
		if e.Name == "Fury" && e.State != RuleOpen {
			t.Fatalf("Fury as Pat = %s, want open", e.State)
		}
	}
	if len(asPat.Entries) != 5 {
		t.Fatalf("as Pat = %d entries, want 5", len(asPat.Entries))
	}
}

// A rule on a standard entry holds for the campaign's rewrite of it, in the
// player's list and in the Directors' view, where it is not an orphan.
func TestChoiceRuleService_RuleFollowsRewrite(t *testing.T) {
	env := newRuleEnv(t)
	env.mark(t, "package", "fury", "", RuleNotOffered)
	rewrite := Choice{Key: "40", Name: "Fury (our words)", Source: ChoiceSourceCampaign, Origin: OriginRewritten, StandsFor: "fury"}
	env.choices.pkg = []Choice{rewrite, env.choices.pkg[1], env.choices.pkg[2]}

	l, err := env.choices.CharacterChoiceList(env.ctxFor("lee"), camp, "ancestry")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range l.Choices {
		if c.Name == rewrite.Name {
			t.Fatal("the rewrite of a not-here entry is offered to a player")
		}
	}

	all, err := env.svc.DirectorEntries(context.Background(), camp, dirActor, "ancestry", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Orphans) != 0 {
		t.Fatalf("orphans = %+v, want none: the rule holds for the rewrite", all.Orphans)
	}
	for _, e := range all.Entries {
		if e.Name == rewrite.Name && (e.State != RuleNotOffered || e.Everyone == nil) {
			t.Fatalf("rewrite = %+v, want not here with the everyone mark", e)
		}
	}
}
