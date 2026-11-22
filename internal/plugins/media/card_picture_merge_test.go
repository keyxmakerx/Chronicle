package media

import (
	"context"
	"errors"
	"testing"
)

// uploadOfCardPictureBytes uploads the bytes of a file that a card wears and
// no page references, as the given user, and returns what came back.
func uploadOfCardPictureBytes(t *testing.T, user string, g *fakeCardPictureGuard, wire bool) (got *MediaFile, created bool) {
	t.Helper()
	const existingID = "card-art"
	repo := &mockMediaRepo{
		findByContentHashFn: func(context.Context, string, string) (*MediaFile, error) {
			return &MediaFile{ID: existingID, MimeType: "image/png"}, nil
		},
		findReferencesFn: func(context.Context, string, string) ([]MediaRef, error) { return nil, nil },
		createFn:         func(context.Context, *MediaFile) error { created = true; return nil },
	}
	checker := &stubMemberChecker{
		members:   map[string]map[string]bool{"camp-1": {"player": true, "owner": true, "codm": true}},
		roles:     map[string]map[string]int{"camp-1": {"player": 1, "owner": 3, "codm": 1}},
		dmGranted: map[string]map[string]bool{"camp-1": {"codm": true}},
	}
	svc := newMergeTestService(repo, checker, &fakeEntityVisibilityFilter{}, t.TempDir())
	if wire {
		svc.SetCardPictureGuard(g)
	}
	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1", UploadedBy: user, MimeType: "image/png", FileSize: 100, FileBytes: testPNGBytes(t),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return got, created
}

// A player uploading the bytes of a card's picture, which no page references,
// must not be handed the existing file's id as a match: that would confirm to
// them that the image is in a hidden deck. The upload is stored as a separate
// file exactly like an ordinary one.
func TestUpload_BytesOfACardPictureAreNotMergedForAPlayer(t *testing.T) {
	g := &fakeCardPictureGuard{isPic: true}
	got, created := uploadOfCardPictureBytes(t, "player", g, true)
	if got.MatchedExisting || got.ID == "card-art" || !created || len(got.UsedBy) != 0 {
		t.Errorf("player upload merged into a card's picture: %+v (created=%v)", got, created)
	}
}

// Directors still merge, a file that is no card's picture still merges for a
// player, and a failing check takes the not-mergeable branch.
func TestUpload_CardPictureMergeCases(t *testing.T) {
	for _, tc := range []struct {
		name  string
		user  string
		guard *fakeCardPictureGuard
		merge bool
	}{
		{"player, a card picture", "player", &fakeCardPictureGuard{isPic: true}, false},
		{"player, not a card picture", "player", &fakeCardPictureGuard{}, true},
		{"owner, a card picture", "owner", &fakeCardPictureGuard{isPic: true}, true},
		{"co-Director, a card picture", "codm", &fakeCardPictureGuard{isPic: true}, true},
		{"player, the check failing", "player", &fakeCardPictureGuard{isErr: errors.New("db")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, created := uploadOfCardPictureBytes(t, tc.user, tc.guard, true)
			if got.MatchedExisting != tc.merge || created == tc.merge {
				t.Errorf("merged=%v created=%v, want merged=%v", got.MatchedExisting, created, tc.merge)
			}
		})
	}
	t.Run("the service reports the guard wired", func(t *testing.T) {
		svc := &mediaService{}
		if svc.CardPictureGuardWired() {
			t.Error("wired before it was set")
		}
		svc.SetCardPictureGuard(&fakeCardPictureGuard{})
		if !svc.CardPictureGuardWired() {
			t.Error("not wired after it was set")
		}
	})
}

type tracingCardGuard struct {
	fakeCardPictureGuard
	trace *[]string
}

func (g *tracingCardGuard) IsCardPicture(ctx context.Context, c, id string) (bool, error) {
	*g.trace = append(*g.trace, "card")
	return g.fakeCardPictureGuard.IsCardPicture(ctx, c, id)
}

// uploadCardTrace runs a player's upload and returns its lookups; matched says
// whether the bytes are a card's picture (a refused merge) or match nothing.
func uploadCardTrace(t *testing.T, matched bool) []string {
	t.Helper()
	var trace []string
	repo := &mockMediaRepo{
		findByContentHashFn: func(context.Context, string, string) (*MediaFile, error) {
			trace = append(trace, "hash")
			if matched {
				return &MediaFile{ID: "card-art", MimeType: "image/png"}, nil
			}
			return nil, nil
		},
		findReferencesFn: func(context.Context, string, string) ([]MediaRef, error) {
			trace = append(trace, "references")
			return nil, nil
		},
		createFn: func(context.Context, *MediaFile) error { return nil },
	}
	checker := &tracingMemberChecker{stubMemberChecker: stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"player-1": true}}}, trace: &trace}
	vis := &tracingVisibility{fakeEntityVisibilityFilter: fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}}, trace: &trace}
	svc := newMergeTestService(repo, checker, vis, t.TempDir())
	svc.SetCardPictureGuard(&tracingCardGuard{fakeCardPictureGuard: fakeCardPictureGuard{isPic: matched}, trace: &trace})
	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1", UploadedBy: "player-1", MimeType: "image/png", FileSize: 100, FileBytes: testPNGBytes(t),
	})
	if err != nil || got.MatchedExisting {
		t.Fatalf("Upload: %v, merged=%v; this test needs a refused merge", err, got != nil && got.MatchedExisting)
	}
	return trace
}

// A merge refused because the bytes are a card's picture makes the same
// lookups, in the same order, as an upload that matched nothing.
func TestUpload_CardPictureRefusalMakesTheNoMatchLookups(t *testing.T) {
	refused, noMatch := uploadCardTrace(t, true), uploadCardTrace(t, false)
	if len(refused) < 5 {
		t.Fatalf("refused merge made only %v; the trace is not seeing the checks", refused)
	}
	if len(refused) != len(noMatch) {
		t.Fatalf("lookups differ:\nrefused  %v\nno match %v", refused, noMatch)
	}
	for i := range refused {
		if refused[i] != noMatch[i] {
			t.Fatalf("lookups differ:\nrefused  %v\nno match %v", refused, noMatch)
		}
	}
}
