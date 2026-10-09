package entities

import (
	"context"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCategoryContextPalette(t *testing.T) {
	reg := NewBlockRegistry()
	noop := func(BlockRenderContext) templ.Component { return templ.NopComponent }
	reg.Register(BlockMeta{Type: "calendar_full", Contexts: []string{"dashboard"}}, noop)
	reg.Register(BlockMeta{Type: "notice_boards", Contexts: []string{"template", "category"}}, noop)
	reg.Register(BlockMeta{Type: "title", Contexts: []string{"template"}}, noop)

	tests := []struct {
		ctx  string
		want map[string]bool
	}{
		{"category", map[string]bool{"calendar_full": true, "notice_boards": true, "title": false}},
		{"dashboard", map[string]bool{"calendar_full": true, "notice_boards": false, "title": false}},
		{"template", map[string]bool{"calendar_full": false, "notice_boards": true, "title": true}},
	}
	for _, tt := range tests {
		t.Run(tt.ctx, func(t *testing.T) {
			got := map[string]bool{}
			for _, m := range reg.TypesForCampaignAndContext(context.Background(), "c", nil, tt.ctx) {
				got[m.Type] = true
			}
			for typ, want := range tt.want {
				if got[typ] != want {
					t.Errorf("%s palette has %s = %v, want %v", tt.ctx, typ, got[typ], want)
				}
			}
		})
	}
}

func TestExtraCategoryBlocks(t *testing.T) {
	if IsExtraCategoryBlock("zz_test_block") {
		t.Fatal("unregistered block reported as registered")
	}
	if renderExtraCategoryBlock(nil, nil, "zz_test_block") == nil {
		t.Fatal("unknown block must render nothing, not nil")
	}
	RegisterCategoryBlock("zz_test_block", func(*campaigns.CampaignContext, *EntityType) templ.Component { return templ.NopComponent })
	if !IsExtraCategoryBlock("zz_test_block") {
		t.Fatal("registered block not reported")
	}
}
