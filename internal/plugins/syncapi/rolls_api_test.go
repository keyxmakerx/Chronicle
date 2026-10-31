package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type fakeRolls struct {
	gotCampaign, gotMember string
	got                    *RollReport
	dup                    bool
}

func (f *fakeRolls) Report(_ context.Context, campaignID, memberID string, in RollReport) (string, bool, error) {
	f.gotCampaign, f.gotMember, f.got = campaignID, memberID, &in
	return "roll-1", f.dup, nil
}

type fakeRollPlayers struct{ FoundryPlayerRepository }

func (fakeRollPlayers) List(context.Context, string) ([]FoundryPlayer, error) {
	return []FoundryPlayer{
		{FoundryUserID: "fu-linked", MemberUserID: "member-7", ReportedAt: time.Now()},
		{FoundryUserID: "fu-unlinked"},
	}, nil
}

const rollBody = `{"originId":"msg-1","foundryUserId":"%s","rollerName":"Jun","formula":"2d10+1","dice":[{"sides":10,"value":3},{"sides":10,"value":4}],"total":8,"mode":"public"}`

func rollRequest(t *testing.T, h *APIHandler, keyID int, foundryUser string) (int, map[string]any, error) {
	t.Helper()
	c, rec := dmScreenContext(http.MethodPost, fmt.Sprintf(rollBody, foundryUser), keyID)
	err := h.ReportRoll(c)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, err
}

func TestReportRoll(t *testing.T) {
	t.Run("not wired is 404", func(t *testing.T) {
		h := NewAPIHandler(nil, nil, nil, nil)
		_, _, err := rollRequest(t, h, 5, "")
		if got := dmStatusOf(t, err); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})
	t.Run("a signed-in browser cannot report a roll", func(t *testing.T) {
		h, rolls := NewAPIHandler(nil, nil, nil, nil), &fakeRolls{}
		h.SetRolls(rolls, fakeRollPlayers{})
		_, _, err := rollRequest(t, h, synthKeySessionID, "")
		if got := dmStatusOf(t, err); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
		if rolls.got != nil {
			t.Fatal("the roll reached the log")
		}
	})
	t.Run("a linked Foundry user rolls as their member", func(t *testing.T) {
		h, rolls := NewAPIHandler(nil, nil, nil, nil), &fakeRolls{}
		h.SetRolls(rolls, fakeRollPlayers{})
		code, out, err := rollRequest(t, h, 5, "fu-linked")
		if err != nil || code != http.StatusCreated || out["id"] != "roll-1" || out["duplicate"] != false {
			t.Fatalf("code %d out %v err %v", code, out, err)
		}
		if rolls.gotCampaign != "camp-1" || rolls.gotMember != "member-7" || rolls.got.Total != 8 || len(rolls.got.Dice) != 2 {
			t.Fatalf("reported %+v as %q", rolls.got, rolls.gotMember)
		}
	})
	t.Run("an unlinked Foundry user rolls as a name", func(t *testing.T) {
		h, rolls := NewAPIHandler(nil, nil, nil, nil), &fakeRolls{}
		h.SetRolls(rolls, fakeRollPlayers{})
		if _, _, err := rollRequest(t, h, 5, "fu-unlinked"); err != nil {
			t.Fatal(err)
		}
		if rolls.gotMember != "" {
			t.Fatalf("member = %q", rolls.gotMember)
		}
	})
	t.Run("a replay answers 200", func(t *testing.T) {
		h, rolls := NewAPIHandler(nil, nil, nil, nil), &fakeRolls{dup: true}
		h.SetRolls(rolls, fakeRollPlayers{})
		code, out, err := rollRequest(t, h, 5, "")
		if err != nil || code != http.StatusOK || out["duplicate"] != true {
			t.Fatalf("code %d out %v err %v", code, out, err)
		}
	})
}
