package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// mergeCapture is an EntityService that only records the context MergeFields
// receives; embedding the interface leaves every other method nil.
type mergeCapture struct {
	entities.EntityService
	ctx context.Context
}

func (m *mergeCapture) MergeFields(ctx context.Context, _ string, _ map[string]any) error {
	m.ctx = ctx
	return nil
}

// A stash move writes the sheet under the stash source, so the money history
// does not add a second line for a change the move already records.
func TestArmoryFieldsAdapter_MarksStashSource(t *testing.T) {
	svc := &mergeCapture{}
	a := &armoryEntityFieldsAdapter{svc: svc}
	outer := changesource.With(context.Background(), changesource.Source{Kind: changesource.KindFoundry, UserID: "gm"})
	if err := a.UpdateEntityFields(outer, "c1", map[string]any{"gp": 3.0}); err != nil {
		t.Fatal(err)
	}
	got, ok := changesource.From(svc.ctx)
	if !ok || got.Kind != changesource.KindStash {
		t.Fatalf("source = %+v, %v; want stash", got, ok)
	}
}

// A shop purchase keeps its own mark, so the coins it spends show in the
// money history as spent at a shop.
func TestArmoryFieldsAdapter_KeepsShopSource(t *testing.T) {
	svc := &mergeCapture{}
	a := &armoryEntityFieldsAdapter{svc: svc}
	outer := armory.ShopPurchaseSource(context.Background(), "u1", "")
	if err := a.UpdateEntityFields(outer, "c1", map[string]any{"gp": 3.0}); err != nil {
		t.Fatal(err)
	}
	got, ok := changesource.From(svc.ctx)
	if !ok || got.Kind != changesource.KindShop || got.UserID != "u1" {
		t.Fatalf("source = %+v, %v; want shop by u1", got, ok)
	}
}

type stashCaptureBus struct{ msgs []*ws.Message }

func (b *stashCaptureBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

func TestArmoryStashEventAdapter(t *testing.T) {
	tests := []struct {
		name     string
		typ      string
		campaign string
		wantMsg  bool
	}{
		{"moved", armory.EventStashMoved, "c", true},
		{"requested", armory.EventStashRequested, "c", true},
		{"settled", armory.EventStashSettled, "c", true},
		{"money changed", armory.EventStashMoneyChanged, "c", true},
		{"downtime", armory.EventDowntimeChanged, "c", true},
		{"unknown type is dropped", "stash.exploded", "c", false},
		{"no campaign is dropped", armory.EventStashMoved, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &stashCaptureBus{}
			a := &armoryStashEventAdapter{bus: bus}
			a.PublishStashEvent(tt.typ, tt.campaign, "7", map[string]any{"moveId": 7})
			if (len(bus.msgs) == 1) != tt.wantMsg {
				t.Fatalf("messages = %d", len(bus.msgs))
			}
			if tt.wantMsg {
				m := bus.msgs[0]
				if string(m.Type) != tt.typ || m.CampaignID != "c" || m.ResourceID != "7" || !m.RequiresDM {
					t.Fatalf("message = %+v", m)
				}
			}
		})
	}
	// Before the bus is bound nothing is published and nothing panics.
	(&armoryStashEventAdapter{}).PublishStashEvent(armory.EventStashMoved, "c", "1", nil)
}

type memberStub struct {
	campaigns.CampaignService
	member *campaigns.CampaignMember
	err    error
}

func (m memberStub) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	return m.member, m.err
}

func TestArmoryMemberDirectory_MemberRole(t *testing.T) {
	tests := []struct {
		name     string
		stub     memberStub
		wantRole int
		wantOK   bool
		wantErr  bool
	}{
		{"member", memberStub{member: &campaigns.CampaignMember{Role: campaigns.RoleScribe}}, int(campaigns.RoleScribe), true, false},
		{"not a member", memberStub{err: apperror.NewNotFound("member not found")}, 0, false, false},
		{"lookup failure surfaces", memberStub{err: apperror.NewInternal(context.Canceled)}, 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, ok, err := (&armoryMemberDirectoryAdapter{svc: tt.stub}).MemberRole(context.Background(), "c", "u")
			if role != tt.wantRole || ok != tt.wantOK || (err != nil) != tt.wantErr {
				t.Fatalf("got %d %v %v", role, ok, err)
			}
		})
	}
}
