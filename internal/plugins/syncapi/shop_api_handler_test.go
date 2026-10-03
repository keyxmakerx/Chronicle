package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

type stubShopRoomReader struct {
	layout   json.RawMessage
	err      error
	gotRole  int
	gotShop  string
	gotCamp  string
	gotCalls int
}

func (s *stubShopRoomReader) GetRoom(_ context.Context, campaignID, shopEntityID string, role int, _ string) (json.RawMessage, error) {
	s.gotCalls++
	s.gotRole, s.gotShop, s.gotCamp = role, shopEntityID, campaignID
	return s.layout, s.err
}

type stubRelationSvcForShop struct {
	relations.RelationService
	rels []relations.Relation
}

func (s *stubRelationSvcForShop) ListByEntity(_ context.Context, _, _ string) ([]relations.Relation, error) {
	return s.rels, nil
}

// stubEntitySvcForShop answers the batched visibility check from a fixed set
// of player-visible ids, recording the role and user it was asked about.
type stubEntitySvcForShop struct {
	entities.EntityService
	visible map[string]bool
	gotRole int
	gotUser string
}

func (s *stubEntitySvcForShop) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, role int, userID string) (map[string]bool, error) {
	s.gotRole, s.gotUser = role, userID
	out := map[string]bool{}
	for _, id := range ids {
		if s.visible[id] {
			out[id] = true
		}
	}
	return out, nil
}

func newShopRoomContext() (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/armory/shops/shop-1/room", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("camp-1", "shop-1")
	// A stored Bearer key, which resolves at Owner level.
	c.Set(apiKeyContextKey, &APIKey{ID: 42, CampaignID: "camp-1", UserID: "owner-1", IsActive: true})
	return c, rec
}

func TestGetShopRoom(t *testing.T) {
	rels := []relations.Relation{
		{ID: 1, TargetEntityID: "sword", RelationType: "sells", TargetEntityName: "Sword"},
		{ID: 2, TargetEntityID: "sword", RelationType: "sells", DmOnly: true},
		{ID: 3, TargetEntityID: "secret-item", RelationType: "sells"},
		{ID: 4, TargetEntityID: "keeper", RelationType: "employs"},
	}
	cases := []struct {
		name      string
		reader    *stubShopRoomReader
		rels      []relations.Relation
		wantCode  int
		wantGoods []int
		wantNull  bool
	}{
		{"saved layout, goods filtered for players", &stubShopRoomReader{layout: json.RawMessage(`{"v":1}`)}, rels, http.StatusOK, []int{1}, false},
		{"no saved layout is null, empty goods is []", &stubShopRoomReader{}, nil, http.StatusOK, []int{}, true},
		{"hidden or missing shop is not found", &stubShopRoomReader{err: apperror.NewNotFound("shop")}, rels, http.StatusNotFound, nil, false},
		{"no reader wired is not found", nil, rels, http.StatusNotFound, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := &stubEntitySvcForShop{visible: map[string]bool{"sword": true}}
			h := NewAPIHandler(nil, ents,
				&stubCampaignSvcForDmGrant{role: campaigns.RoleOwner},
				&stubRelationSvcForShop{rels: tc.rels})
			if tc.reader != nil {
				h.SetShopRoomReader(tc.reader, "armory")
			}
			c, rec := newShopRoomContext()

			err := h.GetShopRoom(c)
			if tc.wantCode != http.StatusOK {
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) || appErr.Code != tc.wantCode {
					t.Fatalf("want %d AppError, got %#v", tc.wantCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetShopRoom: %v", err)
			}
			if tc.reader.gotRole != int(campaigns.RoleOwner) || tc.reader.gotShop != "shop-1" || tc.reader.gotCamp != "camp-1" {
				t.Errorf("reader asked role=%d shop=%q camp=%q, want Owner shop-1 camp-1", tc.reader.gotRole, tc.reader.gotShop, tc.reader.gotCamp)
			}
			var resp struct {
				Layout json.RawMessage      `json:"layout"`
				Goods  []relations.Relation `json:"goods"`
			}
			if jerr := json.Unmarshal(rec.Body.Bytes(), &resp); jerr != nil {
				t.Fatalf("decode: %v (body=%s)", jerr, rec.Body)
			}
			if tc.wantNull != (string(resp.Layout) == "null") {
				t.Errorf("layout = %s, wantNull=%v", resp.Layout, tc.wantNull)
			}
			if resp.Goods == nil {
				t.Fatalf("goods must be an array, never null; body=%s", rec.Body)
			}
			got := make([]int, 0, len(resp.Goods))
			for _, g := range resp.Goods {
				got = append(got, g.ID)
			}
			if len(got) != len(tc.wantGoods) {
				t.Fatalf("goods ids = %v, want %v", got, tc.wantGoods)
			}
			for i := range got {
				if got[i] != tc.wantGoods[i] {
					t.Fatalf("goods ids = %v, want %v", got, tc.wantGoods)
				}
			}
			if len(tc.rels) > 0 && (ents.gotRole != int(campaigns.RolePlayer) || ents.gotUser != "") {
				t.Errorf("goods visibility asked role=%d user=%q, want a plain Player with no user", ents.gotRole, ents.gotUser)
			}
		})
	}
}
