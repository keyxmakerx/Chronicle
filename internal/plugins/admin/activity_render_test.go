package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestAdminDashboard_NeedsYouAndRecent(t *testing.T) {
	tests := []struct {
		name     string
		needs    []NeedsItem
		recent   []ActivityEntry
		contains []string
	}{
		{
			name:     "empty states",
			contains: []string{"Nothing needs you right now.", "No admin changes have been recorded yet.", "/admin/activity"},
		},
		{
			name:  "rows and sentence",
			needs: buildNeedsYou(needsInput{PendingSubmissions: 2}),
			recent: []ActivityEntry{{
				ActorName: "Mara", Action: "user.admin_granted", TargetLabel: "Theo",
				CreatedAt: time.Now().Add(-5 * time.Minute),
			}},
			contains: []string{"/admin/packages/pending", "Review packages", "Mara made Theo an admin", "5 minutes ago"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			c := AdminDashboardPage(buildHomeGroups(homeInput{Users: 1, Now: time.Now()}), tc.needs, tc.recent)
			if err := c.Render(context.Background(), &buf); err != nil {
				t.Fatalf("render failed: %v", err)
			}
			html := buf.String()
			for _, want := range tc.contains {
				if !strings.Contains(html, want) {
					t.Errorf("missing %q in dashboard", want)
				}
			}
		})
	}
}

func TestAdminActivityPage_FiltersAndPagerKeepQuery(t *testing.T) {
	var buf bytes.Buffer
	d := ActivityPageData{
		Entries: []ActivityEntry{{ActorName: "Mara", Action: "smtp.saved", CreatedAt: time.Now()}},
		Actors:  []ActivityActor{{ID: "u1", Name: "Mara"}},
		Query:   ActivityQuery{Actor: "u1", Area: AreaSite, When: When30},
		Total:   60, Page: 1, PerPage: 25,
	}
	if err := AdminActivityPage(d).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{
		`method="get"`, `name="actor"`, `name="area"`, `name="when"`,
		"fa-hard-drive", "actor=u1", "area=site", "when=30d",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in page", want)
		}
	}
}

func TestAdminActivityPage_UsesSharedPagination(t *testing.T) {
	var buf bytes.Buffer
	entries := []ActivityEntry{{ActorName: "Mara", Action: "backup.run", CreatedAt: time.Now()}}
	if err := AdminActivityPage(ActivityPageData{Entries: entries, Total: 60, Page: 1, PerPage: 25, Query: ActivityQuery{When: When7Days}}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Page 1 of 3") || !strings.Contains(html, "Mara started a backup") {
		t.Errorf("expected shared pager and entry; got: %s", html)
	}
}
