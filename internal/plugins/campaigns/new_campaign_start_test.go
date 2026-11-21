package campaigns

import (
	"context"
	"reflect"
	"testing"
)

// fakeFeatures records the campaigns it was asked to start features for.
type fakeFeatures struct{ asked []string }

func (f *fakeFeatures) EnableStarterFeatures(_ context.Context, campaignID, _ string) {
	f.asked = append(f.asked, campaignID)
}

// A campaign made now is handed to the feature enabler once; an import is not,
// because its file carries its own switches; without an enabler creation still
// works and turns nothing on.
func TestCreate_StartsTheUsualFeatures(t *testing.T) {
	tests := []struct {
		name      string
		input     CreateCampaignInput
		noEnabler bool
		want      int
	}{
		{"new campaign", CreateCampaignInput{Name: "A"}, false, 1},
		{"import leaves features to the file", CreateCampaignInput{Name: "A", SkipDefaultFeatures: true}, false, 0},
		{"no enabler wired", CreateCampaignInput{Name: "A"}, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestCampaignServiceFull(&mockCampaignRepo{}, &mockUserFinder{}, nil, &mockSeeder{})
			f := &fakeFeatures{}
			if !tt.noEnabler {
				svc.SetFeatureEnabler(f)
			}
			if _, err := svc.Create(context.Background(), "user-1", tt.input); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if len(f.asked) != tt.want {
				t.Errorf("asked %d times, want %d", len(f.asked), tt.want)
			}
		})
	}
}

// Each step is worked out from what exists, two can be passed on, and the
// count and the next step follow.
func TestBuildSetupView(t *testing.T) {
	tests := []struct {
		name     string
		facts    setupFacts
		done     int
		next     string
		stepDone map[string]bool
		values   map[string]string
	}{
		{"nothing yet", setupFacts{}, 0, "Choose your game", map[string]bool{}, nil},
		{"one invite sent", setupFacts{Invites: 1}, 1, "Choose your game", map[string]bool{setupInvite: true}, map[string]string{setupInvite: "1 invite sent"}},
		{"a join link counts as an invite", setupFacts{HasJoinLink: true}, 1, "Choose your game", map[string]bool{setupInvite: true}, map[string]string{setupInvite: "Invite link made"}},
		{"three invites", setupFacts{Invites: 3}, 1, "Choose your game", map[string]bool{setupInvite: true}, map[string]string{setupInvite: "3 invites sent"}},
		{"game chosen", setupFacts{HasSystem: true, SystemName: "Draw Steel"}, 1, "Invite your players", map[string]bool{setupGame: true}, map[string]string{setupGame: "Draw Steel"}},
		{"no game system kept", setupFacts{Kept: []string{setupGame}}, 1, "Invite your players", map[string]bool{setupGame: true}, map[string]string{setupGame: "No game system"}},
		{"look kept", setupFacts{Kept: []string{setupLook}}, 1, "Choose your game", map[string]bool{setupLook: true}, nil},
		{"two done", setupFacts{HasSystem: true, SystemName: "X", Invites: 1}, 2, "Pick a look", map[string]bool{setupGame: true, setupInvite: true}, nil},
		{"all four", setupFacts{HasSystem: true, SystemName: "X", Invites: 1, LookSaved: true, HasPage: true}, 4, "", map[string]bool{setupGame: true, setupInvite: true, setupLook: true, setupPage: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := buildSetupView("c1", tt.facts)
			if v.DoneCount != tt.done {
				t.Errorf("done = %d, want %d", v.DoneCount, tt.done)
			}
			switch {
			case tt.next == "" && v.Next != nil:
				t.Errorf("next = %q, want none", v.Next.Title)
			case tt.next != "" && (v.Next == nil || v.Next.Title != tt.next):
				t.Errorf("next = %+v, want %q", v.Next, tt.next)
			}
			for _, s := range v.Steps {
				if s.Done != tt.stepDone[s.Key] {
					t.Errorf("step %s done = %v, want %v", s.Key, s.Done, tt.stepDone[s.Key])
				}
				if want, ok := tt.values[s.Key]; ok && s.Value != want {
					t.Errorf("step %s value = %q, want %q", s.Key, s.Value, want)
				}
				if s.Done && s.KeepWord != "" {
					t.Errorf("step %s is done but still offers %q", s.Key, s.KeepWord)
				}
			}
		})
	}
}

// A look counts as chosen only once it holds more than the Annals page look
// and timeline popup every new campaign starts with.
func TestLookChosen(t *testing.T) {
	tests := []struct {
		name string
		a    *Appearance
		want bool
	}{
		{"none", nil, false},
		{"starting defaults", &Appearance{PageLook: "annals", TimelinePopup: "corner"}, false},
		{"another page look", &Appearance{PageLook: "classic", TimelinePopup: "corner"}, true},
		{"a menu colour", &Appearance{PageLook: "annals", SidebarColour: "ink"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lookChosen(tt.a); got != tt.want {
				t.Errorf("lookChosen = %v, want %v", got, tt.want)
			}
		})
	}
}

// Passing a step on stores its key once; other keys are refused.
func TestKeepSetupStep(t *testing.T) {
	var stored string
	c := &Campaign{ID: "c1", Settings: "{}"}
	repo := &mockCampaignRepo{
		findByIDFn:       func(context.Context, string) (*Campaign, error) { return c, nil },
		updateSettingsFn: func(_ context.Context, _ string, s string) error { stored = s; c.Settings = s; return nil },
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	ctx := context.Background()
	if err := svc.KeepSetupStep(ctx, "c1", "look"); err != nil {
		t.Fatal(err)
	}
	if err := svc.KeepSetupStep(ctx, "c1", "look"); err != nil {
		t.Fatal(err)
	}
	if got := c.ParseSettings().SetupKept; !reflect.DeepEqual(got, []string{"look"}) {
		t.Errorf("kept = %v (stored %s), want [look] once", got, stored)
	}
	for _, bad := range []string{"invite", "page", "", "../x"} {
		assertAppError(t, svc.KeepSetupStep(ctx, "c1", bad), 400)
	}
}
