package posts

import (
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// stripPostSecrets removes GM-only content (secret text, GM pictures,
// rollers) from each post's body. A post a player may see can still carry
// GM-only parts inside it, so hiding private posts alone is not enough.
func stripPostSecrets(posts []Post) {
	for i := range posts {
		if len(posts[i].Entry) > 0 {
			posts[i].Entry = json.RawMessage(sanitize.StripSecretsJSON(string(posts[i].Entry)))
		}
		if posts[i].EntryHTML != nil {
			stripped := sanitize.StripSecretsHTML(*posts[i].EntryHTML)
			posts[i].EntryHTML = &stripped
		}
	}
}
