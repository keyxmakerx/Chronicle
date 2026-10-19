package layouts

import (
	"context"
	"testing"
)

func TestGMTerm(t *testing.T) {
	ctx := context.Background()
	if got := GMTerm(ctx); got != DefaultGMTerm {
		t.Fatalf("unset: GMTerm = %q, want %q", got, DefaultGMTerm)
	}
	if got := GMTerm(SetGMTerm(ctx, "Director")); got != "Director" {
		t.Fatalf("set: GMTerm = %q, want Director", got)
	}
	if got := GMTerm(SetGMTerm(ctx, "")); got != DefaultGMTerm {
		t.Fatalf("empty: GMTerm = %q, want %q", got, DefaultGMTerm)
	}
}
