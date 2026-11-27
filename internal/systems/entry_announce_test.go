package systems

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// heardEntries records each announced field.
type heardEntries struct{ fields []string }

func (h *heardEntries) EntryChanged(campaignID, fieldKey string) {
	h.fields = append(h.fields, campaignID+"/"+fieldKey)
}

func TestEntryWritesAreAnnounced(t *testing.T) {
	heard := &heardEntries{}
	repo := &fakeEntryRepo{}
	m := entryTestManifest()
	svc := NewSystemEntryService(repo, func(context.Context, string) *SystemManifest { return m }, WithEntryChanges(heard))
	ctx := context.Background()

	if _, err := svc.Create(ctx, "c1", playerActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Refused"}); err == nil {
		t.Fatal("a player's create should be refused")
	}
	if len(heard.fields) != 0 {
		t.Fatalf("a refused write announced %v", heard.fields)
	}
	e, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Stoneborn"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "c1", dirActor, e.ID, UpdateSystemEntryInput{Name: patch.Of("Stoneborn Elder")}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "c1", dirActor, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "c1", dirActor, e.ID); err == nil {
		t.Fatal("deleting a missing entry should fail")
	}
	want := []string{"c1/ancestry", "c1/ancestry", "c1/ancestry"}
	if len(heard.fields) != len(want) {
		t.Fatalf("announced %v, want %v", heard.fields, want)
	}
	for i := range want {
		if heard.fields[i] != want[i] {
			t.Fatalf("announced %v, want %v", heard.fields, want)
		}
	}
}

func TestNoAnnouncerLeavesTheRepositoryAsIs(t *testing.T) {
	repo := &fakeEntryRepo{}
	svc := NewSystemEntryService(repo, func(context.Context, string) *SystemManifest { return nil }, WithEntryChanges(nil)).(*systemEntryService)
	if svc.repo != SystemEntryRepository(repo) {
		t.Fatal("a nil announcer should not wrap the repository")
	}
}
