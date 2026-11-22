package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// seenSvc records the viewer and patch SetSeen sends and answers a page view
// that, like the real service, carries chosen players for a Director only.
type seenSvc struct {
	trackers.Service
	id     string
	viewer trackers.Viewer
	patch  trackers.TrackerPatch
}

func (s *seenSvc) Patch(_ context.Context, _, id string, v trackers.Viewer, in trackers.TrackerPatch) (*trackers.TrackerView, error) {
	s.id, s.viewer, s.patch = id, v, in
	return &trackers.TrackerView{}, nil
}

func (s *seenSvc) Page(_ context.Context, _, _ string, v trackers.Viewer) (*trackers.PageView, error) {
	tv := trackers.TrackerView{ID: "t1", KindID: "k1", Name: "Hunger", Visibility: trackers.SeePlayers}
	if v.Director {
		tv.Players = []trackers.PlayerView{{UserID: "u1", Name: "Pat", CanChange: true}}
	}
	return &trackers.PageView{Trackers: []trackers.TrackerView{tv}}, nil
}

type fakeDirectory []trackers.Member

func (f fakeDirectory) Members(context.Context, string) ([]trackers.Member, error) { return f, nil }

func TestAITrackersAdapter_SetSeen(t *testing.T) {
	tests := []struct {
		name    string
		actor   records.Actor
		in      records.TrackerSeen
		wantVis string
		picks   []trackers.PlayerPick
	}{
		{"chosen players", records.Actor{UserID: "own", Role: 3},
			records.TrackerSeen{Visibility: "players", Players: []records.TrackerPlayer{{UserID: "u1", CanChange: true}, {UserID: "u2"}}},
			"players", []trackers.PlayerPick{{UserID: "u1", CanChange: true}, {UserID: "u2"}}},
		{"everyone sends no list", records.Actor{UserID: "own", Role: 3},
			records.TrackerSeen{Visibility: "everyone", Players: []records.TrackerPlayer{{UserID: "u1"}}}, "everyone", nil},
		{"directors sends no list", records.Actor{UserID: "own", Role: 3},
			records.TrackerSeen{Visibility: "directors"}, "directors", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &seenSvc{}
			ad := aiTrackersAdapter{svc: svc}
			if err := ad.SetSeen(context.Background(), "c", "t1", tc.actor, tc.in); err != nil {
				t.Fatal(err)
			}
			if svc.id != "t1" || svc.patch.Visibility == nil || *svc.patch.Visibility != tc.wantVis {
				t.Fatalf("patch %+v for %q", svc.patch, svc.id)
			}
			if (svc.patch.Players == nil) != (tc.picks == nil) {
				t.Fatalf("players %+v", svc.patch.Players)
			}
			if tc.picks != nil {
				got := *svc.patch.Players
				if len(got) != len(tc.picks) || got[0] != tc.picks[0] || got[1] != tc.picks[1] {
					t.Fatalf("picks %+v", got)
				}
			}
			// The actor goes through, so the service's Director check decides.
			if svc.viewer.UserID != "own" || !svc.viewer.Director {
				t.Fatalf("viewer %+v", svc.viewer)
			}
		})
	}
}

// A player's key reaches the service as a non-Director, so the service refuses
// it, and no chosen player's name or the member list comes back for it.
func TestAITrackersAdapter_NotADirector(t *testing.T) {
	player := records.Actor{UserID: "pl", Role: 1}
	svc := &seenSvc{}
	ad := aiTrackersAdapter{svc: svc, members: fakeDirectory{{UserID: "u1", Name: "Pat"}}}
	_ = ad.SetSeen(context.Background(), "c", "t1", player, records.TrackerSeen{Visibility: "everyone"})
	if svc.viewer.Director {
		t.Fatalf("viewer %+v", svc.viewer)
	}
	pv, err := ad.Page(context.Background(), "c", "p", player)
	if err != nil || len(pv.Trackers) != 1 || len(pv.Trackers[0].Players) != 0 {
		t.Fatalf("page %+v err %v", pv, err)
	}
	if ms, err := ad.Members(context.Background(), "c", player); err != nil || len(ms) != 0 {
		t.Fatalf("members %+v err %v", ms, err)
	}
	owner := records.Actor{UserID: "own", Role: 3}
	pv, _ = ad.Page(context.Background(), "c", "p", owner)
	if len(pv.Trackers[0].Players) != 1 || pv.Trackers[0].Players[0] != (records.TrackerPlayer{UserID: "u1", Name: "Pat", CanChange: true}) {
		t.Fatalf("director page %+v", pv.Trackers[0].Players)
	}
}

func TestAITrackersAdapter_Members(t *testing.T) {
	ad := aiTrackersAdapter{members: fakeDirectory{
		{UserID: "u1", Name: "Pat", Character: "Sivin"}, {UserID: "u2", Name: "Dee", Director: true}}}
	got, err := ad.Members(context.Background(), "c", records.Actor{UserID: "own", Role: 3})
	if err != nil || len(got) != 2 || got[0] != (records.TrackerMember{UserID: "u1", Name: "Pat", Character: "Sivin"}) || !got[1].Director {
		t.Fatalf("members %+v err %v", got, err)
	}
}
