package media

import (
	"context"
	"testing"
)

// tracingMemberChecker records each membership lookup into a shared trace.
type tracingMemberChecker struct {
	stubMemberChecker
	trace *[]string
}

func (c *tracingMemberChecker) IsUserDmGranted(campaignID, userID string) bool {
	*c.trace = append(*c.trace, "dm-grant")
	return c.stubMemberChecker.IsUserDmGranted(campaignID, userID)
}

func (c *tracingMemberChecker) MemberRole(campaignID, userID string) int {
	*c.trace = append(*c.trace, "role")
	return c.stubMemberChecker.MemberRole(campaignID, userID)
}

type tracingVisibility struct {
	fakeEntityVisibilityFilter
	trace *[]string
}

func (v *tracingVisibility) FilterViewableEntityIDs(ctx context.Context, campaignID string, ids []string, role int, userID string) (map[string]bool, error) {
	*v.trace = append(*v.trace, "visibility")
	return v.fakeEntityVisibilityFilter.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
}

// uploadTrace runs one upload and returns the lookups it made. matched says
// whether the bytes match a file already used by a page the uploader can't
// see (a refused merge) or match nothing.
func uploadTrace(t *testing.T, matched bool) []string {
	t.Helper()
	var trace []string
	repo := &mockMediaRepo{
		findByContentHashFn: func(context.Context, string, string) (*MediaFile, error) {
			trace = append(trace, "hash")
			if matched {
				return &MediaFile{ID: "existing-hidden", MimeType: "image/png"}, nil
			}
			return nil, nil
		},
		findReferencesFn: func(context.Context, string, string) ([]MediaRef, error) {
			trace = append(trace, "references")
			if matched {
				return []MediaRef{{EntityID: "ent-hidden", RefType: "image"}}, nil
			}
			return nil, nil
		},
		createFn: func(context.Context, *MediaFile) error { return nil },
	}
	checker := &tracingMemberChecker{stubMemberChecker: stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"player-1": true}}}, trace: &trace}
	vis := &tracingVisibility{fakeEntityVisibilityFilter: fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}}, trace: &trace}
	svc := newMergeTestService(repo, checker, vis, t.TempDir())
	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1", UploadedBy: "player-1", MimeType: "image/png", FileSize: 100, FileBytes: testPNGBytes(t),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.MatchedExisting {
		t.Fatal("the upload merged; this test needs a refused merge")
	}
	return trace
}

// A refused merge and an upload that matched nothing make the same lookups in
// the same order, so the response time can't tell the uploader that a page
// they can't see holds the same picture.
func TestUpload_NoMatchMakesTheRefusedMergeLookups(t *testing.T) {
	refused := uploadTrace(t, true)
	noMatch := uploadTrace(t, false)
	if len(refused) < 4 {
		t.Fatalf("refused merge made only %v; the trace is not seeing the merge check", refused)
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
