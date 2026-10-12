// move_history_test.go pins the Withdraw control on a waiting purchase line:
// it appears only when the service says the viewer may withdraw, posts to the
// withdraw route, and confirms inline rather than with a browser dialog.
package armory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMoveHistoryWithdrawControl(t *testing.T) {
	line := func(purchase, can bool) MoveLine {
		return MoveLine{
			Move:     Move{ID: 7, CampaignID: "camp", Status: MovePending, CreatedAt: time.Now()},
			Summary:  "Kaela asked to buy 1 item at The Cup. Waiting for the GM.",
			Purchase: purchase, CanWithdraw: can,
		}
	}
	tests := []struct {
		name    string
		line    MoveLine
		want    []string
		notWant []string
	}{
		{
			name: "requester sees the control with an inline confirm",
			line: line(true, true),
			want: []string{
				"data-withdraw-open", "Withdraw this request?", ">Keep it<",
				`hx-post="/campaigns/camp/armory/purchase-requests/7/withdraw"`,
			},
			notWant: []string{"confirm(", "window.confirm"},
		},
		{
			name:    "viewer who may not withdraw gets no control",
			line:    line(true, false),
			notWant: []string{"data-withdraw", "/withdraw"},
		},
		{
			name:    "a move line never gets one",
			line:    line(false, true),
			notWant: []string{"data-withdraw", "/withdraw"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			if err := MoveHistoryList([]MoveLine{tc.line}).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			got := sb.String()
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in %s", w, got)
				}
			}
		})
	}
}
