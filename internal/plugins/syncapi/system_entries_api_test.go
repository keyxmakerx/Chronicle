package syncapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

type stubSystemEntryLister struct {
	gotActor  systems.EntryActor
	gotField  string
	gotCampID string
	entries   []systems.SystemEntry
}

func (s *stubSystemEntryLister) List(_ context.Context, campaignID, fieldKey string, actor systems.EntryActor) ([]systems.SystemEntry, error) {
	s.gotCampID, s.gotField, s.gotActor = campaignID, fieldKey, actor
	return s.entries, nil
}

// Every caller, the Owner's stored key included, gets the player view: what
// this returns ends up in Foundry items players can open.
func TestListSystemEntries_AlwaysPlayerView(t *testing.T) {
	stub := &stubSystemEntryLister{entries: []systems.SystemEntry{
		{FieldKey: "ancestry", Slug: "fairy", Name: "Fairy", Visibility: systems.EntryVisibilityEveryone, CreatedBy: "u9",
			Properties: map[string]any{"size": "1T"}},
		{FieldKey: "ancestry", Slug: "draft", Name: "Draft", Visibility: systems.EntryVisibilityDirectors},
	}}
	h := &APIHandler{}
	h.SetSystemEntryLister(stub)
	c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/system-entries?fieldKey=ancestry", "", true)
	if err := h.ListSystemEntries(c); err != nil {
		t.Fatalf("ListSystemEntries: %v", err)
	}
	if stub.gotActor.IsDirector || stub.gotActor.UserID != "" {
		t.Errorf("actor = %+v, want the zero (player) actor", stub.gotActor)
	}
	if stub.gotCampID != "camp-1" || stub.gotField != "ancestry" {
		t.Errorf("listed %q/%q, want camp-1/ancestry", stub.gotCampID, stub.gotField)
	}
	body := rec.Body.String()
	for _, leak := range []string{"Draft", "u9", "visibility", "createdBy"} {
		if strings.Contains(body, leak) {
			t.Errorf("response carries %q: %s", leak, body)
		}
	}
	var got struct {
		Data  []apiSystemEntry `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Data) != 1 || got.Data[0].Name != "Fairy" || got.Data[0].Properties["size"] != "1T" {
		t.Errorf("got %+v", got)
	}
}

func TestListSystemEntries_NotWired(t *testing.T) {
	c, _ := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/system-entries", "", true)
	err := (&APIHandler{}).ListSystemEntries(c)
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Fatalf("err = %v, want 404", err)
	}
}
