// Package syncapi — sync_cursor.go encodes the pull cursor for
// POST /api/v1/campaigns/:id/sync.
//
// The pull walks the campaign's entity list internally, page by page, and
// stops after syncMaxPullPages so one request cannot hold a connection open
// across an unbounded table scan. That stop was originally a hard ceiling:
// the response set has_more, but the request had no way to say "resume where
// you stopped", and since is a *filter* over the list rather than a position
// in it. So a campaign with more than syncMaxPullPages*syncPageSize entities
// had a tail that no sync request could ever reach — every pull re-scanned
// the same first thousand and stopped.
//
// The cursor turns that ceiling into a page size. It carries the next
// internal page number, and it is opaque on the wire on purpose: the client's
// only contract is "send back what you were given", so the server can later
// switch from offset paging to a keyset without breaking a client that
// hard-coded the arithmetic.
//
// Offset paging is safe here only because entity list ordering is a TOTAL
// order — every ORDER BY in the entities repository ends in e.id ASC (sweep
// R3 stage 4). Without that tiebreaker a page walk duplicates and skips rows,
// which is exactly the bug this endpoint would otherwise re-import.
package syncapi

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// syncCursorPrefix versions the cursor payload so a future keyset cursor can
// be told apart from this one rather than silently misparsed.
const syncCursorPrefix = "sync-v1:"

// syncMaxCursorPage bounds the decoded page number. At syncPageSize=100 this
// is ten million entities, far past any real campaign, and it stops a hostile
// or corrupt cursor from asking the database for an absurd OFFSET.
const syncMaxCursorPage = 100000

// encodeSyncCursor renders the next internal page number as an opaque token.
func encodeSyncCursor(page int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(syncCursorPrefix + strconv.Itoa(page)))
}

// decodeSyncCursor reads a cursor back into an internal page number. An empty
// cursor means "start at the beginning" and yields page 1, so a client that
// has never paged does not have to send the field at all.
//
// A malformed cursor is a 400 rather than a silent reset to page 1: resetting
// would restart the walk from the top and look like it worked, which is the
// same class of quiet lie the cursor exists to fix.
func decodeSyncCursor(cursor string) (int, error) {
	if cursor == "" {
		return 1, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, apperror.NewBadRequest("invalid sync cursor; send back the next_cursor value from the previous response")
	}
	payload := string(raw)
	if !strings.HasPrefix(payload, syncCursorPrefix) {
		return 0, apperror.NewBadRequest("unrecognized sync cursor version")
	}
	page, err := strconv.Atoi(strings.TrimPrefix(payload, syncCursorPrefix))
	if err != nil || page < 1 {
		return 0, apperror.NewBadRequest("invalid sync cursor page")
	}
	if page > syncMaxCursorPage {
		return 0, apperror.NewBadRequest("sync cursor is out of range")
	}
	return page, nil
}
