package app

import (
	"context"
	"strings"
	"testing"
)

func TestParseCompare(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		known  bool
		behind int
	}{
		{"behind main", 200, `{"status":"behind","ahead_by":12,"behind_by":0}`, true, 12},
		{"up to date", 200, `{"status":"identical","ahead_by":0}`, true, 0},
		{"a build GitHub doesn't know", 404, `{"message":"Not Found"}`, false, 0},
		{"rate limited", 403, `{}`, false, 0},
		{"unreadable", 200, `not json`, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCompare(tt.status, strings.NewReader(tt.body), "abc1234")
			if got.Known != tt.known || got.Behind != tt.behind {
				t.Fatalf("got %+v", got)
			}
			if !got.Known && got.Problem == "" {
				t.Fatal("an unknown result must say why")
			}
			if (got.CompareURL != "") != (tt.behind > 0) {
				t.Fatalf("compare URL %q for behind=%d", got.CompareURL, tt.behind)
			}
		})
	}
}

// Only a commit id ever reaches the request URL.
func TestChronicleVersionChecker_UnstampedBuild(t *testing.T) {
	for _, rev := range []string{"", "(devel)", "abc/../x"} {
		v := &chronicleVersionChecker{revision: rev, apiBase: "http://127.0.0.1:1"}
		if got := v.CheckChronicle(context.Background()); got.Known || got.Problem == "" {
			t.Errorf("revision %q: %+v", rev, got)
		}
	}
}
