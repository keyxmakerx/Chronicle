package app

import (
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

// The production wiring must give both media entry points the card-picture
// guard; without it a card's picture is one id away from any member. The test
// fails if either handler is left without it.
func TestWireCardPictures(t *testing.T) {
	svc := decks.NewService(nil, nil, nil)
	mediaHandler := media.NewHandler(nil)
	apiHandler := syncapi.NewMediaAPIHandler(nil, nil)
	mediaSvc := media.NewMediaService(nil, "", 0)
	if mediaHandler.CardPictureGuardWired() || apiHandler.CardPictureGuardWired() || mediaSvc.CardPictureGuardWired() {
		t.Fatal("a fresh handler already has the guard; the test would prove nothing")
	}

	wireCardPictures(svc, mediaSvc, mediaHandler, apiHandler)

	if !mediaHandler.CardPictureGuardWired() {
		t.Error("the media server has no card picture guard; a card's picture would be served to any member")
	}
	if !apiHandler.CardPictureGuardWired() {
		t.Error("the sync media API has no card picture guard; it would list and link a card's picture")
	}
	if !mediaSvc.CardPictureGuardWired() {
		t.Error("the media service has no card picture guard; an upload of a card picture's bytes would be merged into it")
	}
}

// RegisterRoutes is too large to run in a test, so this pins that it calls the
// helper above with both handlers it builds.
func TestRegisterRoutesWiresCardPictures(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "wireCardPictures(decksSvc, mediaService, mediaHandler, mediaAPIHandler)") {
		t.Error("RegisterRoutes no longer wires the card picture guard onto the media handlers")
	}
}
