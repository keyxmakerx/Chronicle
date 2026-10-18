package rumours

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// render draws the block for a viewer from the real service over the seeded
// page, which is the markup that viewer's browser receives.
func render(t *testing.T, repo *fakeRepo, v Viewer) string {
	t.Helper()
	view, err := newTestService(repo).View(context.Background(), tCamp, tPage, v)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if err := BlockRumours(tCamp, tPage, view).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// TestBlock_WhatEachViewerIsSent checks the markup, not the JSON API: the
// Director's controls and truth exist only in the Director's block.
func TestBlock_WhatEachViewerIsSent(t *testing.T) {
	cases := []struct {
		name    string
		v       Viewer
		present []string
		absent  []string
	}{
		{
			"director",
			vDM,
			[]string{`data-dm="1"`, `data-pair`, `data-dm-only`, `rm-dmtag cb-dm`, `data-truth`, `data-pbel`, `data-rm-add`, `data-more`, `"truth":`},
			[]string{`data-belief `},
		},
		{
			"scribe",
			vMira,
			[]string{`data-dm="0"`, `data-add="1"`, `data-mark="1"`, `data-belief`, `data-rm-add`, `data-more`},
			[]string{`data-pair`, `data-dm-only`, `rm-dmtag`, `data-truth`, `data-pbel`, `"truth"`, `≠`},
		},
		{
			"player",
			vJo,
			[]string{`data-dm="0"`, `data-add="0"`, `data-mark="0"`, `data-bel-static`},
			[]string{`data-pair`, `data-dm-only`, `rm-dmtag`, `data-truth`, `data-pbel`, `"truth"`, `≠`, `data-rm-add`, `data-more`, `data-rm-pop`, `data-belief `, `<textarea`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := render(t, seeded(), tc.v)
			for _, s := range tc.present {
				if !strings.Contains(html, s) {
					t.Errorf("missing %q", s)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(html, s) {
					t.Errorf("must not contain %q", s)
				}
			}
		})
	}
}

// TestBlock_NoTruthWordsReachANonDirector is the leak check on the rendered
// text: the verdicts in the seeded data are true/false/open, and a viewer who
// may not hear them gets markup where none of the Director's words appear as a
// verdict anywhere, including through a repository that hands the truth over.
func TestBlock_NoTruthWordsReachANonDirector(t *testing.T) {
	for _, leaky := range []bool{false, true} {
		repo := seeded()
		repo.leaky = leaky
		for _, v := range []Viewer{vMira, vJo} {
			html := render(t, repo, v)
			for _, s := range []string{`data-s=`, `Still open`, `"truth"`, `data-truth`} {
				if strings.Contains(html, s) {
					t.Errorf("leaky=%v role=%d: block contains %q", leaky, v.Role, s)
				}
			}
		}
	}
}

// TestBlock_InlineHandlersOnly proves the swap-safety rule: every control is
// an inline IIFE built server-side, and the only <script> in the block is the
// data one, so nothing depends on a script sibling running after an HTMX swap.
func TestBlock_InlineHandlersOnly(t *testing.T) {
	scripts := regexp.MustCompile(`<script[^>]*>`)
	for _, v := range []Viewer{vDM, vMira, vJo} {
		html := render(t, seeded(), v)
		found := scripts.FindAllString(html, -1)
		if len(found) != 1 || !strings.Contains(found[0], `application/json`) {
			t.Errorf("role %d: want exactly the JSON data script, got %v", v.Role, found)
		}
		if strings.Contains(html, "addEventListener") {
			t.Errorf("role %d: block must not register listeners itself", v.Role)
		}
		if v.Role >= 2 && !strings.Contains(html, "ChronicleRumours") {
			t.Errorf("role %d: handlers should call window.ChronicleRumours", v.Role)
		}
	}
}

// TestBlock_FoldAndBound checks the rest state: the newest two rows, the
// earlier tail, and the notice when the bound cut the list.
func TestBlock_FoldAndBound(t *testing.T) {
	html := render(t, seeded(), vDM)
	if got := strings.Count(html, `<article class="rm-rum" data-id="`+idFavour); got != 1 {
		t.Errorf("newest rumour rendered %d times", got)
	}
	if strings.Count(html, `data-id="6f0a7c52`) != 2 {
		t.Errorf("want only the newest two rows drawn at rest")
	}
	if !strings.Contains(html, "2 earlier rumours") {
		t.Errorf("want the earlier tail for the other two")
	}
	if strings.Contains(html, "data-rm-cut") {
		t.Errorf("no cut notice when nothing was cut")
	}

	view := &View{IsDM: true, CanAdd: true, CanMark: true, Total: 350, Rumours: make([]RumourView, MaxRumours)}
	var sb strings.Builder
	if err := BlockRumours(tCamp, tPage, view).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "Showing the newest 200 of 350 rumours on this page.") {
		t.Errorf("a cut list must say so")
	}
}

// TestBlock_NoGapForAPlayerWithNothing: a player with no rumours is given a
// hidden view, so the page has nothing to draw there.
func TestBlock_NoGapForAPlayerWithNothing(t *testing.T) {
	repo := newFakeRepo()
	view, err := newTestService(repo).View(context.Background(), tCamp, tPage, vJo)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Hidden {
		t.Fatal("a player with no rumours should get a hidden block")
	}
	for _, v := range []Viewer{vDM, vMira} {
		view, _ := newTestService(repo).View(context.Background(), tCamp, tPage, v)
		if view.Hidden {
			t.Errorf("role %d can add, so an empty block is their way to the first rumour", v.Role)
		}
	}
}

// TestBlock_EscapesWhatWasSaid: a rumour is a player's words; markup in it is
// text, in the rows and in the JSON.
func TestBlock_EscapesWhatWasSaid(t *testing.T) {
	repo := seeded()
	repo.rows[3].Quote = `<img src=x onerror=alert(1)></script><b>`
	html := render(t, repo, vDM)
	if strings.Contains(html, `<img src=x`) || strings.Contains(html, `</script><b>`) {
		t.Errorf("a rumour's text reached the page unescaped")
	}
}
