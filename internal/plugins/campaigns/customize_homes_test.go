package campaigns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// stubHomesSvc records what the homes section saves. Only the calls the
// section makes are implemented; any other would nil-panic.
type stubHomesSvc struct {
	CampaignService
	campaign *Campaign
	owner    *DashboardLayout
}

func (s *stubHomesSvc) GetByID(context.Context, string) (*Campaign, error) { return s.campaign, nil }

func (s *stubHomesSvc) UpdateDashboardLayoutRaw(_ context.Context, _ string, j *string) error {
	s.campaign.DashboardLayout = j
	return nil
}

func (s *stubHomesSvc) UpdateOwnerDashboardLayout(_ context.Context, _ string, l *DashboardLayout) error {
	s.owner = l
	return nil
}

func TestApplyHomesSection(t *testing.T) {
	good := `{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"welcome_banner"}]}]}]}`
	bad := `{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[{"id":"b1","type":"no_such_block"}]}]}]}`
	tests := []struct {
		name      string
		raw       string
		role      Role
		wantErr   bool
		wantLeft  []string // homes still in the draft afterwards
		wantSaved []string // audiences saved into the campaign home column
		wantOwner bool
	}{
		{name: "one audience", raw: `{"items":{"player":{"layout":` + good + `}}}`, role: RoleOwner, wantSaved: []string{"player"}},
		{name: "dm dashboard", raw: `{"items":{"dm":{"layout":` + good + `}}}`, role: RoleOwner, wantOwner: true},
		{name: "no layout is no change", raw: `{"items":{"guest":{}}}`, role: RoleOwner},
		{name: "a bad home stays, the good one saves", raw: `{"items":{"guest":{"layout":` + bad + `},"default":{"layout":` + good + `}},"_changes":[{"label":"x"}]}`,
			role: RoleOwner, wantErr: true, wantLeft: []string{"guest"}, wantSaved: []string{"default"}},
		{name: "unknown home refused", raw: `{"items":{"owner":{"layout":` + good + `}}}`, role: RoleOwner, wantErr: true},
		{name: "not the owner", raw: `{"items":{"player":{"layout":` + good + `}}}`, role: RolePlayer, wantErr: true},
		{name: "not json", raw: `[`, role: RoleOwner, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubHomesSvc{campaign: &Campaign{ID: "c1"}}
			h := &Handler{service: svc}
			cc := &CampaignContext{Campaign: svc.campaign, MemberRole: tt.role, IsMember: true}
			left, err := h.applyHomesSection(context.Background(), cc, "u1", json.RawMessage(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			var rest homesSectionValue
			if left != nil {
				if err := json.Unmarshal(left, &rest); err != nil {
					t.Fatalf("remaining is not a section: %s", left)
				}
				if !strings.Contains(string(left), `"_changes"`) {
					t.Errorf("remaining lost the save bar's labels: %s", left)
				}
			}
			if len(rest.Items) != len(tt.wantLeft) {
				t.Errorf("remaining = %s, want %v", left, tt.wantLeft)
			}
			for _, k := range tt.wantLeft {
				if _, ok := rest.Items[k]; !ok {
					t.Errorf("remaining is missing %q", k)
				}
			}
			for _, k := range tt.wantSaved {
				if svc.campaign.GetRoleDashboardJSON(k) == nil {
					t.Errorf("%s home was not saved", k)
				}
			}
			if (svc.owner != nil) != tt.wantOwner {
				t.Errorf("owner dashboard saved = %v, want %v", svc.owner != nil, tt.wantOwner)
			}
		})
	}
}

func TestHomeEditorState(t *testing.T) {
	h := &Handler{}
	saved := `{"rows":[{"id":"own","columns":[]}]}`
	tests := []struct {
		name        string
		column      *string
		owner       *string
		key         string
		wantUnsaved bool
		wantRow     string
	}{
		{name: "fresh default starts from the built-in home", key: HomeDefault, wantUnsaved: true, wantRow: "row-welcome"},
		{name: "fresh guest starts from the starter guest home", key: HomeGuest, wantUnsaved: true, wantRow: "row-categories"},
		{name: "fresh dm dashboard", key: HomeDM, wantUnsaved: true, wantRow: "row-recent"},
		{name: "saved dm dashboard", key: HomeDM, owner: &saved, wantRow: "own"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &CampaignContext{Campaign: &Campaign{ID: "c1", DashboardLayout: tt.column, OwnerDashboardLayout: tt.owner}}
			var st struct {
				Scope   string          `json:"scope"`
				Unsaved bool            `json:"unsaved"`
				Key     string          `json:"key"`
				Layout  DashboardLayout `json:"layout"`
				Preview string          `json:"preview"`
			}
			if err := json.Unmarshal([]byte(h.homeEditorState(cc, tt.key)), &st); err != nil {
				t.Fatal(err)
			}
			if st.Scope != "home" || st.Key != tt.key || st.Unsaved != tt.wantUnsaved || st.Preview != "/campaigns/c1/customize/home-preview" {
				t.Errorf("state = %+v", st)
			}
			found := false
			for _, r := range st.Layout.Rows {
				found = found || r.ID == tt.wantRow
			}
			if !found {
				t.Errorf("layout has no row %q: %+v", tt.wantRow, st.Layout.Rows)
			}
		})
	}
}

func TestAdoptMaterialPageLook(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		changed  bool
		wantSave string // a substring the saved settings must hold; "" when nothing is saved
	}{
		{"no settings", "{}", true, `"page_look":"annals"`},
		{"appearance without a page look", `{"appearance":{"nav_style":"comet"}}`, true, `"nav_style":"comet"`},
		{"already material", `{"appearance":{"page_look":"annals"}}`, false, ""},
		{"keeps other settings", `{"accent_color":"#123456"}`, true, `"accent_color":"#123456"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved string
			repo := &mockCampaignRepo{
				findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
					return &Campaign{ID: id, Settings: tt.settings}, nil
				},
				updateSettingsFn: func(_ context.Context, _ string, j string) error { saved = j; return nil },
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})
			changed, err := svc.AdoptMaterialPageLook(context.Background(), "c1")
			if err != nil || changed != tt.changed {
				t.Fatalf("got %v, %v; want %v", changed, err, tt.changed)
			}
			if tt.wantSave == "" {
				if saved != "" {
					t.Errorf("saved %s, want nothing", saved)
				}
				return
			}
			if !strings.Contains(saved, tt.wantSave) || !strings.Contains(saved, `"page_look":"annals"`) {
				t.Errorf("saved %s, want %s and the Material page look", saved, tt.wantSave)
			}
		})
	}
}

func TestHomesPageLookChoice(t *testing.T) {
	material := &Appearance{PageLook: "annals"}
	tests := []struct {
		name    string
		v       string
		prev    *Appearance
		want    string
		wantErr bool
	}{
		{name: "standard is stored by name", v: "standard", prev: material, want: "standard"},
		{name: "material", v: "annals", want: "annals"},
		{name: "not sent keeps the stored look", v: "", prev: material, want: "annals"},
		{name: "not sent and never chosen", v: "", want: ""},
		{name: "unknown look", v: "paper", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pageLookChoice(tt.v, tt.prev)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("got %q, %v; want %q (err %v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// Every widget Home pages offers must be one a save accepts.
func TestHomeBlockTypesAreSaveable(t *testing.T) {
	for _, ty := range homeBlockTypes {
		if !ValidBlockTypes[ty] {
			t.Errorf("%s is offered but a save refuses it", ty)
		}
	}
}
