package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The folder list follows the category rule of /entity-types: a viewer below
// the DM team never learns a DM-only category's folders exist.

type stubTypesSvc struct {
	entities.EntityService
	types []entities.EntityType
}

func (s *stubTypesSvc) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return s.types, nil
}

type stubFolderLister struct{ nodes []entities.SidebarNode }

func (s *stubFolderLister) ListAllFolders(_ context.Context, _ string) ([]entities.SidebarNode, error) {
	return s.nodes, nil
}

func TestListFolders(t *testing.T) {
	adventures := entities.PresetCategoryAdventures
	types := []entities.EntityType{{ID: 1, Name: "Locations"}, {ID: 2, Name: "Adventures", PresetCategory: &adventures}}
	outer := "n1"
	nodes := []entities.SidebarNode{
		{ID: "n1", EntityTypeID: 1, Name: "Sword Coast"},
		{ID: "n2", EntityTypeID: 1, Name: "Waterdeep", ParentID: &outer, SortOrder: 3},
		{ID: "n3", EntityTypeID: 2, Name: "Act Two"},
	}
	cases := []struct {
		name   string
		role   campaigns.Role
		bearer bool
		want   []string
	}{
		{"player session skips DM-only category folders", campaigns.RolePlayer, false, []string{"n1", "n2"}},
		{"scribe session sees every folder", campaigns.RoleScribe, false, []string{"n1", "n2", "n3"}},
		{"foundry bearer sees every folder", campaigns.RoleOwner, true, []string{"n1", "n2", "n3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewAPIHandler(nil, &stubTypesSvc{types: types}, &stubCampaignSvcForGM{role: tc.role}, nil)
			h.SetFolderLister(&stubFolderLister{nodes: nodes})
			c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/folders", "", tc.bearer)
			if err := h.ListFolders(c); err != nil {
				t.Fatalf("ListFolders: %v", err)
			}
			var resp struct {
				Data []struct {
					ID           string  `json:"id"`
					EntityTypeID int     `json:"entity_type_id"`
					Name         string  `json:"name"`
					ParentID     *string `json:"parent_id"`
					SortOrder    int     `json:"sort_order"`
				} `json:"data"`
				Total int `json:"total"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v (body=%s)", err, rec.Body)
			}
			var got []string
			for _, f := range resp.Data {
				got = append(got, f.ID)
				if f.ID == "n2" && (f.ParentID == nil || *f.ParentID != "n1" || f.SortOrder != 3 || f.Name != "Waterdeep") {
					t.Errorf("nested folder lost its shape: %+v", f)
				}
			}
			if len(got) != len(tc.want) || resp.Total != len(tc.want) {
				t.Fatalf("got %v (total %d), want %v", got, resp.Total, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestListFolders_NotWiredIs404(t *testing.T) {
	h := NewAPIHandler(nil, &stubTypesSvc{}, &stubCampaignSvcForGM{role: campaigns.RoleOwner}, nil)
	c, _ := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/folders", "", true)
	err := h.ListFolders(c)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("want a 404 app error, got %v", err)
	}
}
