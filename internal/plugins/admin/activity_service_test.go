package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeActivityRepo struct {
	inserted            []ActivityEntry
	insertErr           error
	gotLimit, gotOffset int
}

func (f *fakeActivityRepo) Insert(_ context.Context, e *ActivityEntry) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.inserted = append(f.inserted, *e)
	return nil
}

func (f *fakeActivityRepo) List(_ context.Context, limit, offset int) ([]ActivityEntry, int, error) {
	f.gotLimit, f.gotOffset = limit, offset
	return nil, 0, nil
}

func TestActivityService_Record(t *testing.T) {
	tests := []struct {
		name    string
		entry   ActivityEntry
		repoErr error
		wantErr bool
		wantN   int
	}{
		{"stores a valid entry", ActivityEntry{Action: "user.admin_granted"}, nil, false, 1},
		{"rejects empty action", ActivityEntry{Action: "  "}, nil, true, 0},
		{"surfaces storage error", ActivityEntry{Action: "campaign.deleted"}, errors.New("db down"), true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeActivityRepo{insertErr: tc.repoErr}
			err := NewActivityService(repo).Record(context.Background(), tc.entry)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if len(repo.inserted) != tc.wantN {
				t.Errorf("inserted %d, want %d", len(repo.inserted), tc.wantN)
			}
		})
	}
}

// RecordActivity must never panic or return anything when storage fails.
func TestActivityService_RecordActivityNeverFails(t *testing.T) {
	repo := &fakeActivityRepo{insertErr: errors.New("db down")}
	NewActivityService(repo).RecordActivity(context.Background(), "u1", "media.deleted", "media", "f1", "map.png")
	repo.insertErr = nil
	NewActivityService(repo).RecordActivity(context.Background(), "u1", "media.deleted", "media", "f1", "map.png")
	if len(repo.inserted) != 1 || repo.inserted[0].TargetLabel != "map.png" {
		t.Errorf("unexpected inserted: %+v", repo.inserted)
	}
}

func TestActivityService_ListPaging(t *testing.T) {
	tests := []struct {
		name                  string
		page, perPage         int
		wantLimit, wantOffset int
	}{
		{"first page", 1, 10, 10, 0},
		{"third page", 3, 10, 10, 20},
		{"bad page clamps", 0, 10, 10, 0},
		{"bad perPage defaults", 1, 0, activityPerPage, 0},
		{"huge perPage capped", 1, 5000, 100, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeActivityRepo{}
			if _, _, err := NewActivityService(repo).List(context.Background(), tc.page, tc.perPage); err != nil {
				t.Fatal(err)
			}
			if repo.gotLimit != tc.wantLimit || repo.gotOffset != tc.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", repo.gotLimit, repo.gotOffset, tc.wantLimit, tc.wantOffset)
			}
		})
	}
}

func TestActivityEntry_Sentence(t *testing.T) {
	tests := []struct {
		name string
		e    ActivityEntry
		want string
	}{
		{"admin granted", ActivityEntry{ActorName: "Mara", Action: "user.admin_granted", TargetLabel: "Theo"}, "Mara made Theo an admin"},
		{"no target needed", ActivityEntry{ActorName: "Mara", Action: "backup.run"}, "Mara started a backup"},
		{"missing label", ActivityEntry{ActorName: "Mara", Action: "campaign.deleted"}, "Mara deleted the campaign an item"},
		{"deleted actor", ActivityEntry{Action: "media.deleted", TargetLabel: "map.png"}, "A former admin deleted the file map.png"},
		{"unknown action", ActivityEntry{ActorName: "Mara", Action: "thing.did_stuff"}, "Mara made a change (thing.did stuff)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.e.Sentence(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestActivityService_RecordClampsToColumns(t *testing.T) {
	repo := &fakeActivityRepo{}
	svc := NewActivityService(repo)
	long := strings.Repeat("é", 300)
	if err := svc.Record(context.Background(), ActivityEntry{Action: "campaign.delete", TargetLabel: long, TargetID: long}); err != nil {
		t.Fatal(err)
	}
	got := repo.inserted[0]
	if n := utf8.RuneCountInString(got.TargetLabel); n != 255 {
		t.Errorf("label has %d characters, want 255", n)
	}
	if n := utf8.RuneCountInString(got.TargetID); n != 64 {
		t.Errorf("target id has %d characters, want 64", n)
	}
	if !utf8.ValidString(got.TargetLabel) {
		t.Error("clamping split a character")
	}
}

// TestActivityPhrases_CoverRecordedActions pins that every action the
// recorders emit has a readable sentence, and that a label placeholder is
// used at most once so Sentence never prints a format error.
func TestActivityPhrases_CoverRecordedActions(t *testing.T) {
	actions := []string{
		"extension.updated", "extension.rescanned",
		"extension.plugin_reloaded", "extension.plugin_stopped",
		"storage.user_limit_set", "storage.user_limit_removed", "storage.campaign_limit_set",
		"storage.campaign_limit_removed", "storage.user_bypass_set", "storage.user_bypass_cleared",
		"storage.campaign_bypass_set", "storage.campaign_bypass_cleared", "apialert.resolved",
		"foundry.campaign_notified", "foundry.campaign_force_pinned", "foundry.older_notified",
		"foundry.older_force_pinned", "restore.started",
	}
	for _, a := range actions {
		t.Run(a, func(t *testing.T) {
			phrase, ok := activityPhrases[a]
			if !ok {
				t.Fatalf("no phrase for %q", a)
			}
			if strings.Count(phrase, "%s") > 1 {
				t.Errorf("phrase %q has more than one %%s", phrase)
			}
		})
	}
}
