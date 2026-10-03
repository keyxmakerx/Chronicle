package foundry_vtt

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// TestUpdateModeFromLegacy pins the mapping that makes every existing
// campaign keep behaving as it does today.
func TestUpdateModeFromLegacy(t *testing.T) {
	cases := []struct {
		name        string
		pin, mode   string
		wantMode    packages.UpdateMode
		wantVersion string
	}{
		{"nothing stored", "", "", packages.UpdateModeAutomatic, ""},
		{"promote with no pin", "", PinModePromote, packages.UpdateModeAutomatic, ""},
		{"a pin is pinned", "0.1.5", "", packages.UpdateModePinned, "0.1.5"},
		{"a pin wins over promote", "0.1.5", PinModePromote, packages.UpdateModePinned, "0.1.5"},
		{"preserve with a pin", "0.1.5", PinModePreserve, packages.UpdateModePinned, "0.1.5"},
		{"preserve with no pin yet", "", PinModePreserve, packages.UpdateModePinned, ""},
		{"stored pinned with a pin", "0.1.5", PinModePinned, packages.UpdateModePinned, "0.1.5"},
		// Today a bare "pinned" with no pin still follows the installed
		// version, so it must stay automatic.
		{"stored pinned with no pin", "", PinModePinned, packages.UpdateModeAutomatic, ""},
		{"ask first with a pin", "0.1.5", PinModeApproveFirst, packages.UpdateModeApproveFirst, "0.1.5"},
		{"ask first with no pin", "", PinModeApproveFirst, packages.UpdateModeApproveFirst, ""},
		{"unknown mode, no pin", "", "weird", packages.UpdateModeAutomatic, ""},
	}
	for _, tc := range cases {
		gotMode, gotVersion := UpdateModeFromLegacy(tc.pin, tc.mode)
		if gotMode != tc.wantMode || gotVersion != tc.wantVersion {
			t.Errorf("%s: got (%q,%q) want (%q,%q)", tc.name, gotMode, gotVersion, tc.wantMode, tc.wantVersion)
		}
	}
}

// Whatever a campaign is served today (its pin, or the installed version
// when it has none) must be unchanged by the mapping: only pin != "" means a
// pinned serve, and only approve_first adds anything new.
func TestUpdateModeFromLegacyNeverChangesWhatIsServed(t *testing.T) {
	for _, pin := range []string{"", "0.1.5"} {
		for _, mode := range []string{"", PinModePreserve, PinModePromote, PinModePinned} {
			_, version := UpdateModeFromLegacy(pin, mode)
			if version != pin {
				t.Errorf("pin %q mode %q: reported version %q, serving uses %q", pin, mode, version, pin)
			}
		}
	}
}

func TestIsValidPinModeAcceptsApproveFirst(t *testing.T) {
	if !IsValidPinMode(PinModeApproveFirst) {
		t.Error("approve_first must be a valid stored mode")
	}
}

// bindingSvc records the pin writes the binding makes through the Service.
type bindingSvc struct {
	Service
	pinned map[string]string
	forced []string
	err    error
}

func (s *bindingSvc) SetPinnedVersion(_ context.Context, id, version string) error {
	if s.err != nil {
		return s.err
	}
	s.pinned[id] = version
	return nil
}

func (s *bindingSvc) ForcePinCampaign(_ context.Context, id, version, _, _, _ string) error {
	if s.err != nil {
		return s.err
	}
	s.pinned[id] = version
	s.forced = append(s.forced, id)
	return nil
}

type fakePinLister struct{ rows []CampaignPinRow }

func (f fakePinLister) AllCampaignPins(context.Context) ([]CampaignPinRow, error) { return f.rows, nil }

func newBinding(pin, mode string) (packages.CampaignBinding, *bindingSvc, *fakeSettings) {
	st := &fakeSettings{pin: pin, pinMode: mode}
	svc := &bindingSvc{pinned: map[string]string{}}
	// Mirror the real Service: a pin write lands in the settings.
	return NewCampaignBinding(&settingsBackedSvc{svc, st}, st, fakePinLister{rows: []CampaignPinRow{
		{CampaignID: "a", CampaignName: "A", Pin: "0.1.0", PinMode: ""},
		{CampaignID: "b", CampaignName: "B", Pin: "", PinMode: PinModePreserve},
		{CampaignID: "c", CampaignName: "C", Pin: "", PinMode: PinModePromote},
		{CampaignID: "d", CampaignName: "D", Pin: "0.1.0", PinMode: PinModeApproveFirst},
	}}), svc, st
}

// settingsBackedSvc makes the fake Service write the pin into the fake
// settings the way the real one does.
type settingsBackedSvc struct {
	*bindingSvc
	st *fakeSettings
}

func (s *settingsBackedSvc) SetPinnedVersion(ctx context.Context, id, v string) error {
	if err := s.bindingSvc.SetPinnedVersion(ctx, id, v); err != nil {
		return err
	}
	s.st.pin = v
	return nil
}

func (s *settingsBackedSvc) ForcePinCampaign(ctx context.Context, id, v, a, ip, ua string) error {
	if err := s.bindingSvc.ForcePinCampaign(ctx, id, v, a, ip, ua); err != nil {
		return err
	}
	s.st.pin = v
	return nil
}

func TestCampaignBindingListsEveryCampaignWithItsMode(t *testing.T) {
	b, _, _ := newBinding("", "")
	got, err := b.Campaigns(context.Background(), &packages.Package{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]packages.UpdateMode{
		"a": packages.UpdateModePinned, "b": packages.UpdateModePinned,
		"c": packages.UpdateModeAutomatic, "d": packages.UpdateModeApproveFirst,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d campaigns: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Mode != want[s.CampaignID] || s.CampaignName == "" {
			t.Errorf("%s: %+v", s.CampaignID, s)
		}
	}
}

func TestCampaignBindingApply(t *testing.T) {
	cases := []struct {
		name         string
		startPin     string
		startMode    string
		mode         packages.UpdateMode
		version      string
		admin        bool
		wantPin      string
		wantMode     string
		wantForced   bool
		wantStateTo  packages.UpdateMode
		wantStateVer string
	}{
		{"automatic clears the pin", "0.1.0", PinModePinned, packages.UpdateModeAutomatic, "", false,
			"", PinModePromote, false, packages.UpdateModeAutomatic, ""},
		{"pinned stores pin and mode", "", "", packages.UpdateModePinned, "0.1.0", false,
			"0.1.0", PinModePinned, false, packages.UpdateModePinned, "0.1.0"},
		{"ask first stores pin and mode", "", "", packages.UpdateModeApproveFirst, "0.1.0", false,
			"0.1.0", PinModeApproveFirst, false, packages.UpdateModeApproveFirst, "0.1.0"},
		{"admin on their behalf goes through force-pin", "0.1.0", PinModeApproveFirst, packages.UpdateModeApproveFirst, "0.2.0", true,
			"0.2.0", PinModeApproveFirst, true, packages.UpdateModeApproveFirst, "0.2.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, svc, st := newBinding(tc.startPin, tc.startMode)
			err := b.Apply(context.Background(), "x", &packages.Package{}, tc.mode, tc.version, packages.ActorInfo{UserID: "u", Admin: tc.admin})
			if err != nil {
				t.Fatal(err)
			}
			if st.pin != tc.wantPin || st.pinMode != tc.wantMode {
				t.Errorf("stored pin=%q mode=%q, want pin=%q mode=%q", st.pin, st.pinMode, tc.wantPin, tc.wantMode)
			}
			if (len(svc.forced) == 1) != tc.wantForced {
				t.Errorf("force-pin used = %v, want %v", len(svc.forced) == 1, tc.wantForced)
			}
			state, err := b.State(context.Background(), "x", &packages.Package{})
			if err != nil || state.Mode != tc.wantStateTo || state.Version != tc.wantStateVer {
				t.Errorf("state = %+v, %v; want %q %q", state, err, tc.wantStateTo, tc.wantStateVer)
			}
		})
	}
}

func TestCampaignBindingApplyPinFailureWritesNoMode(t *testing.T) {
	b, svc, st := newBinding("", "")
	svc.err = errors.New("version not installed")
	if err := b.Apply(context.Background(), "x", &packages.Package{}, packages.UpdateModePinned, "9.9.9", packages.ActorInfo{}); err == nil {
		t.Fatal("expected the pin error")
	}
	if st.pin != "" || st.pinMode != "" {
		t.Errorf("a failed pin must leave the settings alone, got pin=%q mode=%q", st.pin, st.pinMode)
	}
}
