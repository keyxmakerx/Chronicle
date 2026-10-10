package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pagedEntitySvc serves a campaign of `total` entities in pages, counting calls.
type pagedEntitySvc struct {
	entities.EntityService
	total int
	calls int
}

func (s *pagedEntitySvc) List(_ context.Context, _ string, _ int, _ int, _ string, o entities.ListOptions) ([]entities.Entity, int, error) {
	s.calls++
	var out []entities.Entity
	for i := (o.Page - 1) * o.PerPage; i < o.Page*o.PerPage && i < s.total; i++ {
		out = append(out, entities.Entity{ID: fmt.Sprintf("e%d", i)})
	}
	return out, s.total, nil
}

// The party walk must go past the old 20-page (2000 entity) stop, and only
// report truncation when the safety bound itself is reached.
func TestListByTypes_WalksPastOldCapAndReportsBound(t *testing.T) {
	tests := []struct {
		name          string
		total         int
		wantLen       int
		wantTruncated bool
	}{
		{"well past the old 2000 stop", 2500, 2500, false},
		{"exactly the bound is complete", listPageBound * 100, listPageBound * 100, false},
		{"beyond the bound is reported", listPageBound*100 + 1, listPageBound * 100, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &armoryStashDirectoryAdapter{svc: &pagedEntitySvc{total: tt.total}}
			got, truncated, err := a.listByTypes(context.Background(), &stashTypeInfo{}, "c1", []int{1}, 3, "u", 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantLen || truncated != tt.wantTruncated {
				t.Errorf("len=%d truncated=%v, want %d %v", len(got), truncated, tt.wantLen, tt.wantTruncated)
			}
		})
	}
}
