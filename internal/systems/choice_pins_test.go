package systems

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func pinNames(cs []Choice) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
		if c.Pinned {
			out[i] += "*"
		}
	}
	return strings.Join(out, ",")
}

func TestOrderPinned(t *testing.T) {
	list := []Choice{
		{Name: "Devil", Key: "devil", Source: ChoiceSourcePackage},
		{Name: "Dwarf", Key: "dwarf", Source: ChoiceSourcePackage},
		{Name: "Human", Key: "human", Source: ChoiceSourcePackage},
		{Name: "Khepri", Key: "7", Source: ChoiceSourceCampaign},
		{Name: "Drakari", Key: "9", Source: ChoiceSourceCampaign},
	}
	cases := []struct {
		name string
		pins []string
		want string
	}{
		{"no pins keeps the list's order", nil, "Devil,Dwarf,Human,Khepri,Drakari"},
		{"pins lead in pin order", []string{"package:human", "campaign:7"}, "Human*,Khepri*,Devil,Dwarf,Drakari"},
		{"pin order wins over list order", []string{"campaign:9", "package:devil"}, "Drakari*,Devil*,Dwarf,Human,Khepri"},
		{"a pin naming no entry is skipped", []string{"package:memonek", "package:dwarf"}, "Dwarf*,Devil,Human,Khepri,Drakari"},
		{"a slug never matches a campaign entry's id", []string{"package:7", "campaign:human"}, "Devil,Dwarf,Human,Khepri,Drakari"},
		{"a repeated pin counts once", []string{"package:human", "package:human"}, "Human*,Devil,Dwarf,Khepri,Drakari"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := append([]Choice(nil), list...)
			if got := pinNames(orderPinned(in, tc.pins)); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// pinRepo is an in-memory ChoicePinRepository.
type pinRepo struct {
	keys    map[string][]string
	failGet bool
}

func (r *pinRepo) List(_ context.Context, c, s, f string) ([]string, error) {
	if r.failGet {
		return nil, errors.New("down")
	}
	return append([]string(nil), r.keys[c+"/"+s+"/"+f]...), nil
}

func (r *pinRepo) Replace(_ context.Context, c, s, f string, keys []string, _ string) error {
	if r.keys == nil {
		r.keys = map[string][]string{}
	}
	r.keys[c+"/"+s+"/"+f] = append([]string(nil), keys...)
	return nil
}

// pinChoices serves one fixed list from one system.
type pinChoices struct{ stubChoiceSvc }

func (*pinChoices) CharacterChoiceList(_ context.Context, _, k string) (*ChoiceList, error) {
	return &ChoiceList{FieldKey: k, SystemID: "drawsteel", Choices: []Choice{
		{Name: "Human", Key: "human", Source: ChoiceSourcePackage},
		{Name: "Dwarf", Key: "dwarf", Source: ChoiceSourcePackage},
		{Name: "Khepri", Key: "7", Source: ChoiceSourceCampaign},
	}}, nil
}

func TestSetPinned(t *testing.T) {
	cases := []struct {
		name     string
		director bool
		start    []string
		source   string
		key      string
		pinned   bool
		want     string
		wantErr  int
	}{
		{"a Director pins: it goes last among the pins", true, []string{"package:human"}, ChoiceSourceCampaign, "7", true, "package:human,campaign:7", 0},
		{"pinning twice keeps one pin, moved last", true, []string{"campaign:7", "package:human"}, ChoiceSourceCampaign, "7", true, "package:human,campaign:7", 0},
		{"a Director unpins", true, []string{"package:human", "campaign:7"}, ChoiceSourcePackage, "human", false, "campaign:7", 0},
		{"below the Director team is refused", false, nil, ChoiceSourcePackage, "human", true, "", http.StatusForbidden},
		{"an entry off the list cannot be pinned", true, nil, ChoiceSourcePackage, "memonek", true, "", http.StatusNotFound},
		{"an unknown source is refused", true, nil, "other", "human", true, "", http.StatusBadRequest},
		{"an empty key is refused", true, nil, ChoiceSourcePackage, " ", true, "", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &pinRepo{keys: map[string][]string{"c1/drawsteel/ancestry": tc.start}}
			svc := NewChoicePinService(repo, &pinChoices{})
			got, err := svc.SetPinned(context.Background(), "c1", "u1", tc.director, "ancestry", tc.source, tc.key, tc.pinned)
			if tc.wantErr != 0 {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tc.wantErr {
					t.Fatalf("err = %v, want code %d", err, tc.wantErr)
				}
				if strings.Join(repo.keys["c1/drawsteel/ancestry"], ",") != strings.Join(tc.start, ",") {
					t.Error("a refused change must not write")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, ",") != tc.want {
				t.Errorf("pins = %v, want %s", got, tc.want)
			}
		})
	}
}

// Who may pin is decided on the server: the owner and a member given DM
// access may; a Scribe, a player and a guest are refused.
func TestSetPinAPI_Roles(t *testing.T) {
	cases := []struct {
		name string
		cc   *campaigns.CampaignContext
		code int
	}{
		{"owner", &campaigns.CampaignContext{MemberRole: campaigns.RoleOwner, IsMember: true}, http.StatusOK},
		{"co-Director (DM access)", &campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsMember: true, IsDmGranted: true}, http.StatusOK},
		{"Scribe", &campaigns.CampaignContext{MemberRole: campaigns.RoleScribe, IsMember: true}, http.StatusForbidden},
		{"player", &campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsMember: true}, http.StatusForbidden},
		{"guest", &campaigns.CampaignContext{MemberRole: campaigns.RoleNone}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &pinRepo{}
			h := NewCharacterChoiceHandler(&stubChoiceSvc{})
			h.SetPins(NewChoicePinService(repo, &pinChoices{}))
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"source":"package","key":"dwarf","pinned":true}`))
			req.Header.Set("Content-Type", "application/json")
			c := e.NewContext(req, httptest.NewRecorder())
			c.SetPathValues(echo.PathValues{{Name: "fieldKey", Value: "ancestry"}})
			tc.cc.Campaign = &campaigns.Campaign{ID: "c1"}
			c.Set("campaign_context", tc.cc)
			err := h.SetPinAPI(c)
			code := http.StatusOK
			if err != nil {
				var ae *apperror.AppError
				if !errors.As(err, &ae) {
					t.Fatalf("unexpected error %v", err)
				}
				code = ae.Code
			}
			if code != tc.code {
				t.Fatalf("code = %d, want %d", code, tc.code)
			}
			pinned := len(repo.keys["c1/drawsteel/ancestry"]) == 1
			if pinned != (tc.code == http.StatusOK) {
				t.Errorf("stored pin = %v, want %v", pinned, tc.code == http.StatusOK)
			}
		})
	}
}

// A pin is an arrangement: when the pins cannot be read the list is served
// in its own order rather than not at all.
func TestApplyPins_ReadFailureKeepsList(t *testing.T) {
	s := &characterChoiceService{pins: NewChoicePinService(&pinRepo{failGet: true}, &pinChoices{})}
	l := &ChoiceList{FieldKey: "ancestry", SystemID: "drawsteel", Choices: []Choice{{Name: "B", Key: "b", Source: ChoiceSourcePackage}, {Name: "A", Key: "a", Source: ChoiceSourcePackage}}}
	s.applyPins(context.Background(), "c1", l)
	if pinNames(l.Choices) != "B,A" {
		t.Errorf("got %s", pinNames(l.Choices))
	}
	s.pins = NewChoicePinService(&pinRepo{keys: map[string][]string{"c1/drawsteel/ancestry": {"package:a"}}}, &pinChoices{})
	s.applyPins(context.Background(), "c1", l)
	if pinNames(l.Choices) != "A*,B" {
		t.Errorf("got %s", pinNames(l.Choices))
	}
}
