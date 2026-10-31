package app

import (
	"reflect"
	"testing"
)

type recordingRevoker struct{ calls []string }

func (r *recordingRevoker) RevokeUser(campaignID, userID string) {
	r.calls = append(r.calls, "user:"+campaignID+":"+userID)
}

func (r *recordingRevoker) RevokeCampaign(campaignID string) {
	r.calls = append(r.calls, "campaign:"+campaignID)
}

// Every revoker hears every campaign access change: the live sockets and the
// AI keys must both end on Trash and on a handover.
func TestCampaignRevokers_FanOut(t *testing.T) {
	a, b := &recordingRevoker{}, &recordingRevoker{}
	rs := campaignRevokers{a, &aiKeyKeeper{}, b}
	rs.RevokeUser("c1", "u1")
	rs.RevokeCampaign("c2")
	want := []string{"user:c1:u1", "campaign:c2"}
	if !reflect.DeepEqual(a.calls, want) || !reflect.DeepEqual(b.calls, want) {
		t.Errorf("calls = %v and %v, want %v each", a.calls, b.calls, want)
	}
}
