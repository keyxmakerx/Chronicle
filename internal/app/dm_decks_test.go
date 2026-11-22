package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
)

type decksAddonStub struct {
	addons.AddonService
	on  bool
	err error
}

func (s decksAddonStub) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return s.on, s.err
}

type decksScreenStub struct {
	decks.Service
	got  decks.Viewer
	view *decks.ScreenView
	err  error
}

func (s *decksScreenStub) Screen(_ context.Context, _ string, v decks.Viewer) (*decks.ScreenView, error) {
	s.got = v
	return s.view, s.err
}

func TestDMDecksAdapter(t *testing.T) {
	view := &decks.ScreenView{CampaignID: "c1", Total: 3, Shown: []decks.ScreenDeck{
		{ID: "d1", Drawn: []decks.ScreenShown{{CopyID: "x"}, {CopyID: "y"}}}, {ID: "d2"}, {ID: "d3"}}}
	tests := []struct {
		name    string
		addon   decksAddonStub
		svcErr  error
		viewer  dmscreen.Viewer
		wantOK  bool
		wantErr bool
		asked   bool
	}{
		{"on, for the owner", decksAddonStub{on: true}, nil, dmscreen.Viewer{UserID: "u", Role: 3}, true, false, true},
		{"the add-on off has no section and asks nothing of decks", decksAddonStub{on: false}, nil, dmscreen.Viewer{UserID: "u", Role: 3}, false, false, false},
		{"an add-on lookup that fails is an error", decksAddonStub{err: errors.New("db down")}, nil, dmscreen.Viewer{UserID: "u", Role: 3}, false, true, false},
		{"a refusal from decks is passed on", decksAddonStub{on: true}, errors.New("no"), dmscreen.Viewer{UserID: "u", Role: 3}, false, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &decksScreenStub{view: view, err: tt.svcErr}
			a := &dmDecksAdapter{svc: svc, addons: tt.addon}
			body, summary, ok, err := a.DecksSection(context.Background(), "c1", tt.viewer)
			if ok != tt.wantOK || (err != nil) != tt.wantErr {
				t.Fatalf("ok %v err %v, want ok %v err %v", ok, err, tt.wantOK, tt.wantErr)
			}
			if asked := svc.got.UserID != ""; asked != tt.asked {
				t.Errorf("decks asked = %v, want %v", asked, tt.asked)
			}
			if tt.wantOK {
				if body == nil || summary != "3 decks · 2 drawn" {
					t.Errorf("body %v summary %q", body, summary)
				}
				if !svc.got.Director || svc.got.UserID != "u" {
					t.Errorf("decks saw viewer %+v", svc.got)
				}
			}
		})
	}
}
