package media

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PlainPlayerCanOpen admits only a file any player could open with their own
// session, and refuses whenever a check is missing or fails.
func TestPlainPlayerCanOpen(t *testing.T) {
	refs := func(ids ...string) func(context.Context, string, string) ([]MediaRef, error) {
		return func(context.Context, string, string) ([]MediaRef, error) {
			out := make([]MediaRef, 0, len(ids))
			for _, id := range ids {
				out = append(out, MediaRef{EntityID: id})
			}
			return out, nil
		}
	}
	tests := []struct {
		name    string
		file    func() *MediaFile
		refs    func(context.Context, string, string) ([]MediaRef, error)
		visible map[string]bool
		visErr  error
		maps    *fakeMapImageGuard
		cards   *fakeCardPictureGuard
		unwired bool
		want    bool
		wantErr bool
	}{
		{name: "no page shows it", file: adr058File, want: true},
		{name: "a page every player sees", file: adr058File, refs: refs("hidden", "open"), visible: map[string]bool{"open": true}, want: true},
		{name: "only GM-only pages", file: adr058File, refs: refs("hidden"), want: false},
		{name: "page filter fails", file: adr058File, refs: refs("open"), visErr: errors.New("db"), wantErr: true},
		{name: "reference lookup fails", file: adr058File, refs: func(context.Context, string, string) ([]MediaRef, error) { return nil, errors.New("db") }, wantErr: true},
		{name: "a map's picture", file: adr058File, maps: &fakeMapImageGuard{isPic: true}, want: false},
		{name: "map check fails", file: adr058File, maps: &fakeMapImageGuard{err: errors.New("db")}, wantErr: true},
		{name: "a card's picture", file: adr058File, cards: &fakeCardPictureGuard{isPic: true}, want: false},
		{name: "card check fails", file: adr058File, cards: &fakeCardPictureGuard{isPic: true, isErr: errors.New("db")}, wantErr: true},
		{name: "a note picture", file: func() *MediaFile { f := adr058File(); f.UsageType = UsageNoteImage; return f }, want: false},
		{name: "a page file", file: func() *MediaFile { f := adr058File(); f.UsageType = UsagePageFile; return f }, want: false},
		{name: "no campaign", file: func() *MediaFile { f := adr058File(); f.CampaignID = nil; return f }, want: false},
		{name: "unwired", file: adr058File, unwired: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAccessMediaService{findReferencesFn: tt.refs}
			vis := &fakeEntityVisibilityFilter{viewableIDs: tt.visible, err: tt.visErr}
			h := NewHandler(svc)
			if !tt.unwired {
				maps, cards := tt.maps, tt.cards
				if maps == nil {
					maps = &fakeMapImageGuard{}
				}
				if cards == nil {
					cards = &fakeCardPictureGuard{}
				}
				h.SetMapImageGuard(maps)
				h.SetCardPictureGuard(cards)
				h.SetEntityVisibilityFilter(vis)
			}
			got, err := h.PlainPlayerCanOpen(context.Background(), tt.file())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("PlainPlayerCanOpen = %v, want %v", got, tt.want)
			}
			if vis.callCount > 0 && (vis.lastRole != int(campaigns.RolePlayer) || vis.lastUserID != "") {
				t.Errorf("pages filtered as role %d user %q, want a plain player with no user", vis.lastRole, vis.lastUserID)
			}
		})
	}
}
