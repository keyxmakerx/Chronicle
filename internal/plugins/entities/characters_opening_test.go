package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Characters opens on one line inside the frame; "Create hero" is there only
// for someone who may make one, and keeps the attribute the hero builder's
// script looks for.
func TestCharactersOpening(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Ashen"}}
	render := func(v CastView) string {
		var buf bytes.Buffer
		if err := CharactersContent(cc, v).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	player := render(CastView{})
	if !strings.Contains(player, `data-app-opening="full"`) || !strings.Contains(player, "The cast of Ashen.") || strings.Contains(player, "Create hero") {
		t.Errorf("a viewer who may not create should see the opening and no Create hero:\n%s", player)
	}
	maker := render(CastView{CanCreateHero: true})
	if !strings.Contains(maker, "Create hero") || !strings.Contains(maker, `data-create-hero="true"`) {
		t.Errorf("Create hero or its script hook is missing:\n%s", maker)
	}
	if i := strings.Index(maker, `id="cast-content"`); i < 0 || strings.Index(maker, "data-app-frame") < i {
		t.Errorf("the frame must sit inside #cast-content so its swaps keep it")
	}
}
