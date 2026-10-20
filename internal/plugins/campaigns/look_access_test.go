package campaigns

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

// viewer builds a campaign context for one kind of viewer.
func viewer(role Role, granted bool, ap *Appearance) *CampaignContext {
	s, _ := json.Marshal(CampaignSettings{Appearance: ap})
	return &CampaignContext{
		Campaign:    &Campaign{ID: "c1", Settings: string(s)},
		MemberRole:  role,
		IsDmGranted: granted,
		IsMember:    role > RoleNone,
	}
}

func TestLookAccessAllows(t *testing.T) {
	cases := []struct {
		name    string
		role    Role
		granted bool
		member  bool
		setting string
		want    bool
	}{
		{"owner only: owner", RoleOwner, false, true, LookAccessOwner, true},
		{"owner only: scribe", RoleScribe, false, true, LookAccessOwner, false},
		{"owner only: player", RolePlayer, false, true, LookAccessOwner, false},
		{"owner only: dm-granted player", RolePlayer, true, true, LookAccessOwner, false},
		{"co-dm: owner", RoleOwner, false, true, LookAccessCoDM, true},
		{"co-dm: scribe", RoleScribe, false, true, LookAccessCoDM, true},
		{"co-dm: dm-granted player", RolePlayer, true, true, LookAccessCoDM, true},
		{"co-dm: plain player", RolePlayer, false, true, LookAccessCoDM, false},
		{"everyone: owner", RoleOwner, false, true, LookAccessEveryone, true},
		{"everyone: scribe", RoleScribe, false, true, LookAccessEveryone, true},
		{"everyone: player", RolePlayer, false, true, LookAccessEveryone, true},
		{"everyone: non-member", RoleNone, false, false, LookAccessEveryone, false},
		{"unknown setting", RolePlayer, false, true, "nobody", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := &CampaignContext{MemberRole: tc.role, IsDmGranted: tc.granted, IsMember: tc.member}
			if got := LookAccessAllows(tc.setting, cc); got != tc.want {
				t.Errorf("LookAccessAllows(%q) = %v, want %v", tc.setting, got, tc.want)
			}
		})
	}
}

func TestLookPartAccessDefaultsAndClamps(t *testing.T) {
	sheet, _ := LookPartByKey(LookPartSheet)
	dm, _ := LookPartByKey(LookPartDmScreen)
	cases := []struct {
		name string
		part LookPart
		ap   *Appearance
		want string
	}{
		{"sheet default", sheet, nil, LookAccessEveryone},
		{"dmscreen default", dm, nil, LookAccessCoDM},
		{"sheet stored", sheet, &Appearance{AccessSheet: LookAccessOwner}, LookAccessOwner},
		{"dmscreen stored owner", dm, &Appearance{AccessDmScreen: LookAccessOwner}, LookAccessOwner},
		// A hand-edited value must never widen the DM Screen.
		{"dmscreen never everyone", dm, &Appearance{AccessDmScreen: LookAccessEveryone}, LookAccessCoDM},
		{"garbage falls back", sheet, &Appearance{AccessSheet: "x"}, LookAccessEveryone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.part.Access(tc.ap); got != tc.want {
				t.Errorf("Access = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMayPickByRoleAndSetting(t *testing.T) {
	cases := []struct {
		name    string
		role    Role
		granted bool
		ap      *Appearance
		sheet   bool
		dm      bool
	}{
		{"player, defaults", RolePlayer, false, nil, true, false},
		{"scribe, defaults", RoleScribe, false, nil, true, true},
		{"dm-granted player, defaults", RolePlayer, true, nil, true, true},
		// The owner's pick is the campaign look itself.
		{"owner, defaults", RoleOwner, false, nil, false, false},
		{"player, sheet owner-only", RolePlayer, false, &Appearance{AccessSheet: LookAccessOwner}, false, false},
		{"scribe, both owner-only", RoleScribe, false, &Appearance{AccessSheet: LookAccessOwner, AccessDmScreen: LookAccessOwner}, false, false},
		{"player, sheet co-dm", RolePlayer, false, &Appearance{AccessSheet: LookAccessCoDM}, false, false},
		{"dm-granted, sheet co-dm", RolePlayer, true, &Appearance{AccessSheet: LookAccessCoDM}, true, true},
		{"player, dmscreen hand-set everyone", RolePlayer, false, &Appearance{AccessDmScreen: LookAccessEveryone}, true, false},
	}
	sheet, _ := LookPartByKey(LookPartSheet)
	dm, _ := LookPartByKey(LookPartDmScreen)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := viewer(tc.role, tc.granted, tc.ap)
			if got := sheet.MayPick(tc.ap, cc); got != tc.sheet {
				t.Errorf("sheet MayPick = %v, want %v", got, tc.sheet)
			}
			if got := dm.MayPick(tc.ap, cc); got != tc.dm {
				t.Errorf("dmscreen MayPick = %v, want %v", got, tc.dm)
			}
		})
	}
}

func TestAccessChoiceValidation(t *testing.T) {
	sheet, _ := LookPartByKey(LookPartSheet)
	dm, _ := LookPartByKey(LookPartDmScreen)
	cases := []struct {
		name    string
		part    LookPart
		in      string
		want    string
		wantErr bool
	}{
		{"empty is default", sheet, "", "", false},
		{"default stored as empty", sheet, LookAccessEveryone, "", false},
		{"sheet codm", sheet, LookAccessCoDM, LookAccessCoDM, false},
		{"sheet owner", sheet, LookAccessOwner, LookAccessOwner, false},
		{"dmscreen default stored as empty", dm, LookAccessCoDM, "", false},
		{"dmscreen owner", dm, LookAccessOwner, LookAccessOwner, false},
		{"dmscreen everyone refused", dm, LookAccessEveryone, "", true},
		{"unknown refused", sheet, "anyone", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := accessChoice(tc.part, tc.in)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("accessChoice(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestApplyLookAccessKeepsWhenAbsent(t *testing.T) {
	prev := &Appearance{AccessSheet: LookAccessOwner, AccessDmScreen: LookAccessOwner}
	var a Appearance
	if err := applyLookAccess(&a, nil, prev); err != nil {
		t.Fatal(err)
	}
	if a.AccessSheet != LookAccessOwner || a.AccessDmScreen != LookAccessOwner {
		t.Errorf("absent access should keep the stored setting, got %+v", a)
	}
	// Sent explicitly, it replaces; an empty part resets to its default.
	if err := applyLookAccess(&a, &LookAccessInput{Sheet: LookAccessCoDM}, prev); err != nil {
		t.Fatal(err)
	}
	if a.AccessSheet != LookAccessCoDM || a.AccessDmScreen != "" {
		t.Errorf("explicit access should replace, got %+v", a)
	}
	if err := applyLookAccess(&a, &LookAccessInput{DmScreen: LookAccessEveryone}, prev); err == nil {
		t.Error("DM Screen must refuse everyone")
	}
}

func TestApplyAppearanceStoresAccess(t *testing.T) {
	var s CampaignSettings
	in := AppearanceInput{Access: &LookAccessInput{Sheet: LookAccessOwner, DmScreen: LookAccessOwner}}
	if _, err := applyAppearance(&s, in); err != nil {
		t.Fatal(err)
	}
	if s.Appearance == nil || s.Appearance.AccessSheet != LookAccessOwner || s.Appearance.AccessDmScreen != LookAccessOwner {
		t.Fatalf("access not stored: %+v", s.Appearance)
	}
	// A later save that does not send access (a look preset) keeps it.
	if _, err := applyAppearance(&s, AppearanceInput{Look: "parchment"}); err != nil {
		t.Fatal(err)
	}
	if s.Appearance.AccessSheet != LookAccessOwner {
		t.Errorf("a save without access reset it: %+v", s.Appearance)
	}
}

func TestValidateLookPick(t *testing.T) {
	cases := []struct {
		name    string
		role    Role
		granted bool
		ap      *Appearance
		part    string
		value   string
		wantErr bool
	}{
		{"player picks sheet", RolePlayer, false, nil, "sheet", "parchment", false},
		{"player picks modern", RolePlayer, false, nil, "sheet", "modern", false},
		{"player resets sheet", RolePlayer, false, nil, "sheet", "", false},
		{"bad sheet value", RolePlayer, false, nil, "sheet", "gothic", true},
		{"unknown part", RolePlayer, false, nil, "colours", "x", true},
		{"player cannot pick dmscreen", RolePlayer, false, nil, "dmscreen", "sharp", true},
		{"scribe picks dmscreen", RoleScribe, false, nil, "dmscreen", "sharp", false},
		{"dm-granted player picks dmscreen", RolePlayer, true, nil, "dmscreen", "ornate", false},
		{"bad kit value", RoleScribe, false, nil, "dmscreen", "parchment", true},
		{"narrowed sheet refuses player", RolePlayer, false, &Appearance{AccessSheet: LookAccessOwner}, "sheet", "night", true},
		{"narrowed sheet still lets reset be refused too", RolePlayer, false, &Appearance{AccessSheet: LookAccessCoDM}, "sheet", "", true},
		{"owner uses Customize", RoleOwner, false, nil, "sheet", "night", true},
		{"non-member", RoleNone, false, nil, "sheet", "night", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cc := viewer(tc.role, tc.granted, tc.ap)
			_, err := ValidateLookPick(tc.ap, cc, tc.part, tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// A pick the owner has since narrowed away is ignored, not deleted, and comes
// back if access is widened again.
func TestAppliedLookPicksIgnoresNarrowedAccess(t *testing.T) {
	stored := map[string]string{"sheet": "night", "dmscreen": "sharp"}
	cases := []struct {
		name string
		role Role
		ap   *Appearance
		want map[string]string
	}{
		{"scribe, defaults: both count", RoleScribe, nil, map[string]string{"sheet": "night", "dmscreen": "sharp"}},
		{"player, defaults: only sheet", RolePlayer, nil, map[string]string{"sheet": "night"}},
		{"scribe, sheet narrowed to owner", RoleScribe, &Appearance{AccessSheet: LookAccessOwner}, map[string]string{"dmscreen": "sharp"}},
		{"scribe, both narrowed", RoleScribe, &Appearance{AccessSheet: LookAccessOwner, AccessDmScreen: LookAccessOwner}, nil},
		{"owner never uses a pick", RoleOwner, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AppliedLookPicks(tc.ap, viewer(tc.role, false, tc.ap), stored)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
	// A stored value that is no longer a valid choice is skipped too.
	got := AppliedLookPicks(nil, viewer(RolePlayer, false, nil), map[string]string{"sheet": "retired-style"})
	if len(got) != 0 {
		t.Errorf("an unknown stored value must not apply, got %v", got)
	}
}

func TestLookPartsMatchTheCampaignLookLists(t *testing.T) {
	for _, p := range LookParts {
		if len(p.Choices) == 0 || !contains(p.Settings, p.Default) {
			t.Errorf("%s: needs choices and a default among its settings", p.Key)
		}
		for _, ch := range p.Choices {
			if LookChoiceLabel(ch) == ch {
				t.Errorf("%s: choice %q has no display name", p.Key, ch)
			}
		}
	}
	if dm, _ := LookPartByKey(LookPartDmScreen); contains(dm.Settings, LookAccessEveryone) {
		t.Error("the DM Screen must never offer everyone")
	}
}

func TestMemberLookPickService(t *testing.T) {
	ctx := context.Background()
	repo := &mockCampaignRepo{}
	svc := newTestCampaignService(repo, &mockUserFinder{})
	player := viewer(RolePlayer, false, nil)

	if err := svc.SetMemberLookPick(ctx, player, "u1", "sheet", "night"); err != nil {
		t.Fatalf("player sets sheet: %v", err)
	}
	if err := svc.SetMemberLookPick(ctx, player, "u1", "dmscreen", "sharp"); err == nil {
		t.Error("a player must not set the DM Screen kit")
	}
	if err := svc.SetMemberLookPick(ctx, player, "u1", "sheet", "bogus"); err == nil {
		t.Error("an unknown value must be refused")
	}
	got, err := svc.MemberLookPicks(ctx, player, "u1")
	if err != nil || !reflect.DeepEqual(got, map[string]string{"sheet": "night"}) {
		t.Fatalf("picks = %v, %v", got, err)
	}
	// Another person's picks are not read from this person's key.
	if other, _ := svc.MemberLookPicks(ctx, player, "u2"); len(other) != 0 {
		t.Errorf("u2 must not see u1's pick, got %v", other)
	}
	// The owner narrows access: the pick stays stored but no longer applies.
	narrowed := viewer(RolePlayer, false, &Appearance{AccessSheet: LookAccessOwner})
	if got, _ := svc.MemberLookPicks(ctx, narrowed, "u1"); len(got) != 0 {
		t.Errorf("a narrowed pick must be ignored, got %v", got)
	}
	if stored := repo.lookPicks["c1/u1"]["sheet"]; stored != "night" {
		t.Errorf("narrowing must not delete the pick, stored = %q", stored)
	}
	// Reset clears it.
	if err := svc.SetMemberLookPick(ctx, player, "u1", "sheet", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.MemberLookPicks(ctx, player, "u1"); len(got) != 0 {
		t.Errorf("reset should clear, got %v", got)
	}
}
