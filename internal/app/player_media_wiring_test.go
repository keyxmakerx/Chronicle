package app

import (
	"os"
	"strings"
	"testing"
)

// The sync media API answers ?audience=players only through the media server's
// player check; unwired, every such read is refused, so the Foundry module
// would never show a room's picture to players. RegisterRoutes is too large to
// run in a test, so this pins the call.
func TestRegisterRoutesWiresPlayerMediaCheck(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "mediaAPIHandler.SetPlayerMediaCheck(mediaHandler)") {
		t.Error("RegisterRoutes no longer wires the player audience check onto the sync media API")
	}
}
