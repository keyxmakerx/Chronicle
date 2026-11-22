package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// fakeCardGuard is the decks side of the sync media API: which files are a
// card's picture.
type fakeCardGuard struct {
	pics   map[string]bool
	ids    []string
	idsErr error
	isErr  error
}

func (f *fakeCardGuard) IsCardPicture(_ context.Context, _, id string) (bool, error) {
	if f.isErr != nil {
		return true, f.isErr
	}
	return f.pics[id], nil
}
func (f *fakeCardGuard) CardPictureSeenBy(context.Context, string, string, media.CardPictureViewer) (bool, error) {
	return false, nil
}
func (f *fakeCardGuard) CardPictureIDs(context.Context, string) ([]string, error) {
	return f.ids, f.idsErr
}
func (f *fakeCardGuard) CardPictureUses(context.Context, string, string, media.CardPictureViewer) ([]media.CardPictureUse, error) {
	return nil, nil
}

// listSvc records how the API asked for its page.
type listSvc struct {
	media.MediaService
	files    []media.MediaFile
	plain    int
	excluded []string
	byID     map[string]*media.MediaFile
}

func (l *listSvc) ListCampaignMedia(context.Context, string, int, int) ([]media.MediaFile, int, error) {
	l.plain++
	return l.files, len(l.files), nil
}

func (l *listSvc) ListCampaignMediaExcluding(_ context.Context, _ string, ids []string, _, _ int) ([]media.MediaFile, int, error) {
	l.excluded = ids
	var out []media.MediaFile
	skip := map[string]bool{}
	for _, id := range ids {
		skip[id] = true
	}
	for _, f := range l.files {
		if !skip[f.ID] {
			out = append(out, f)
		}
	}
	return out, len(out), nil
}

func (l *listSvc) GetByID(_ context.Context, id string) (*media.MediaFile, error) {
	return l.byID[id], nil
}

func cardCampaign() *string { c := "camp-1"; return &c }

func cardFixture(role campaigns.Role, g media.CardPictureGuard) (*MediaAPIHandler, *listSvc, *APIKey) {
	svc := &listSvc{
		files: []media.MediaFile{
			{ID: "plain", CampaignID: cardCampaign(), MimeType: "image/png"},
			{ID: "card-pic", CampaignID: cardCampaign(), MimeType: "image/png"},
		},
		byID: map[string]*media.MediaFile{
			"plain":    {ID: "plain", CampaignID: cardCampaign(), MimeType: "image/png"},
			"card-pic": {ID: "card-pic", CampaignID: cardCampaign(), MimeType: "image/png"},
		},
	}
	h := NewMediaAPIHandler(&stubSyncSvcForRole{}, svc)
	h.SetCampaignService(&stubCampaignSvcForRole{getMemberFn: memberWithRole(role)})
	h.SetCardPictureGuard(g)
	return h, svc, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "u"}
}

func listedIDs(t *testing.T, body []byte) (ids []string, total int) {
	t.Helper()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	return ids, out.Total
}

// A Scribe's sync listing leaves a card's picture out in the query, so the
// total and the page agree; the owner's lists everything.
func TestListMedia_LeavesOutCardPicturesBelowOwner(t *testing.T) {
	g := &fakeCardGuard{pics: map[string]bool{"card-pic": true}, ids: []string{"card-pic"}}

	h, svc, key := cardFixture(campaigns.RoleScribe, g)
	c, rec := newRoleContext(key)
	if err := h.ListMedia(c); err != nil {
		t.Fatal(err)
	}
	ids, total := listedIDs(t, rec.Body.Bytes())
	if len(ids) != 1 || ids[0] != "plain" || total != 1 || len(svc.excluded) != 1 {
		t.Errorf("Scribe listing = %v total %d excluded %v, want only the plain file", ids, total, svc.excluded)
	}

	h, svc, key = cardFixture(campaigns.RoleScribe, g)
	key.ID = 42 // a stored key is the owner's
	c, rec = newRoleContext(key)
	if err := h.ListMedia(c); err != nil {
		t.Fatal(err)
	}
	ids, total = listedIDs(t, rec.Body.Bytes())
	if len(ids) != 2 || total != 2 || svc.plain != 1 {
		t.Errorf("owner listing = %v total %d plain calls %d, want both files", ids, total, svc.plain)
	}

	t.Run("a failing guard refuses the listing", func(t *testing.T) {
		h, _, key := cardFixture(campaigns.RoleScribe, &fakeCardGuard{idsErr: errors.New("db")})
		c, _ := newRoleContext(key)
		if err := h.ListMedia(c); err == nil {
			t.Error("the listing was served although the card pictures could not be found")
		}
	})
}

// A Scribe cannot look a card's picture up by id, and no caller below owner is
// handed a link to one; the owner still is.
func TestGetMedia_CardPicture(t *testing.T) {
	g := &fakeCardGuard{pics: map[string]bool{"card-pic": true}}
	get := func(h *MediaAPIHandler, key *APIKey, id string) (int, string) {
		c, rec := newRoleContext(key)
		c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "mediaID", Value: id}})
		err := h.GetMedia(c)
		if err != nil {
			return http.StatusNotFound, ""
		}
		return rec.Code, rec.Body.String()
	}

	h, _, key := cardFixture(campaigns.RoleScribe, g)
	if code, _ := get(h, key, "card-pic"); code != http.StatusNotFound {
		t.Errorf("a Scribe's lookup of a card picture answered %d, want 404", code)
	}
	if code, _ := get(h, key, "plain"); code != http.StatusOK {
		t.Errorf("a Scribe's lookup of an ordinary file answered %d, want 200", code)
	}
	failing, _, fkey := cardFixture(campaigns.RoleScribe, &fakeCardGuard{isErr: errors.New("db")})
	if code, _ := get(failing, fkey, "plain"); code != http.StatusNotFound {
		t.Errorf("a failing check answered %d, want 404", code)
	}
	ownerH, _, okey := cardFixture(campaigns.RoleScribe, g)
	okey.ID = 42
	if code, _ := get(ownerH, okey, "card-pic"); code != http.StatusOK {
		t.Errorf("the owner's lookup answered %d, want 200", code)
	}
}

func TestMediaAPICardPictureGuardWiredFlag(t *testing.T) {
	h := NewMediaAPIHandler(&stubSyncSvcForRole{}, &listSvc{})
	if h.CardPictureGuardWired() {
		t.Error("CardPictureGuardWired must be false when unset")
	}
	h.SetCardPictureGuard(&fakeCardGuard{})
	if !h.CardPictureGuardWired() {
		t.Error("CardPictureGuardWired must be true once set")
	}
}
