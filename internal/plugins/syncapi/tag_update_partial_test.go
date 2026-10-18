package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// storedTagRepo serves one stored tag and records what Update writes.
type storedTagRepo struct {
	tags.TagRepository
	written *tags.Tag
}

func (r *storedTagRepo) FindByID(context.Context, int) (*tags.Tag, error) {
	return &tags.Tag{ID: 7, CampaignID: "camp-1", Name: "Villains", Slug: "villains", Color: "#112233", DmOnly: true}, nil
}

func (r *storedTagRepo) Update(_ context.Context, t *tags.Tag) error {
	r.written = t
	return nil
}

// A colour-only push (the Foundry module recolours tags this way) must keep
// the stored name instead of failing the service's required-name check.
func TestUpdateTag_NameIsOptional(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantName  string
		wantColor string
		wantErr   bool
	}{
		{"colour only keeps the name", `{"color":"#ffffff"}`, "Villains", "#ffffff", false},
		{"rename only keeps the colour", `{"name":"Foes"}`, "Foes", "#112233", false},
		{"blank name is still rejected", `{"name":""}`, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &storedTagRepo{}
			h := NewTagAPIHandler(nil, tags.NewTagService(repo), nil, nil)
			req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetParamNames("id", "tagId")
			c.SetParamValues("camp-1", "7")

			err := h.UpdateTag(c)
			if tt.wantErr {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
					t.Fatalf("err = %v, want 400", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if repo.written.Name != tt.wantName || repo.written.Color != tt.wantColor || !repo.written.DmOnly {
				t.Fatalf("wrote %+v", repo.written)
			}
		})
	}
}
