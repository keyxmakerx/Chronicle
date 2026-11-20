package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// withPinned adds the owner's Pinned section holding the given personal Manage
// rows to a test context.
func withPinned(ctx context.Context, keys ...string) context.Context {
	sec := NavSectionView{ID: "pinned", Kind: "pinned", Label: "Pinned"}
	for _, k := range keys {
		sec.Rows = append(sec.Rows, NavRowView{Key: k, Kind: "app", Label: k, URL: "/campaigns/c1/trash", Personal: true})
	}
	return ResolveNavState(SetNavSections(ctx, append([]NavSectionView{sec}, GetNavSections(ctx)...)))
}

func TestManageToolOfLine(t *testing.T) {
	tests := []struct {
		line, key, name string
	}{
		{"people", "manage:members", "People"},
		{"trash", "manage:trash", "Trash"},
		{"tables", "manage:roll-tables", "Rolling tables"},
		{"look", "manage:customize", "Customize"},
		{"changes", "manage:player-changes", "What players changed"},
		{"name", "", ""},
		{"end", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			key, name := ManageToolOfLine(tt.line)
			if key != tt.key || name != tt.name {
				t.Fatalf("ManageToolOfLine(%q) = (%q, %q), want (%q, %q)", tt.line, key, name, tt.key, tt.name)
			}
		})
	}
}

// The pin is offered to the owner alone, and never where a save would be
// refused.
func TestManagePin_OnlyTheOwnerGetsOne(t *testing.T) {
	tests := []struct {
		name   string
		c      navStateCase
		pinned bool
		want   string
	}{
		{"owner, not pinned", navStateCase{role: 3, path: "/campaigns/c1/trash"}, false, `aria-pressed="false"`},
		{"owner, pinned", navStateCase{role: 3, path: "/campaigns/c1/trash"}, true, `aria-pressed="true"`},
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1/trash"}, false, ""},
		{"player", navStateCase{role: 1, path: "/campaigns/c1/trash"}, false, ""},
		{"visitor", navStateCase{role: 0, path: "/campaigns/c1/trash"}, false, ""},
		{"owner previewing as a player", navStateCase{role: 3, player: true, path: "/campaigns/c1/trash"}, false, ""},
		{"owner on an archived campaign", navStateCase{role: 3, archived: true, path: "/campaigns/c1/trash"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.c.ctx()
			if tt.pinned {
				ctx = withPinned(ctx, "manage:trash")
			}
			var buf bytes.Buffer
			if err := ManagePin("manage:trash", "Trash").Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if tt.want == "" {
				if out != "" {
					t.Fatalf("a pin was drawn: %s", out)
				}
				return
			}
			for _, w := range []string{tt.want, `data-manage-pin="manage:trash"`, `onclick=`, `<button`} {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
			label := "Pin Trash to your sidebar"
			if tt.pinned {
				label = "Unpin Trash from your sidebar"
			}
			if !strings.Contains(out, label) {
				t.Errorf("missing label %q in %s", label, out)
			}
		})
	}
}

// A tool with no key draws nothing, so a line with nothing to pin has no pin.
func TestManagePin_NoKeyNoPin(t *testing.T) {
	var buf bytes.Buffer
	if err := ManagePin("", "").Render(navStateCase{role: 3, path: "/campaigns/c1"}.ctx(), &buf); err != nil || buf.Len() != 0 {
		t.Fatalf("err=%v out=%q", err, buf.String())
	}
}

// The owner unpins a Manage tool from the sidebar's Pinned row; nobody else's
// Pinned rows, and none of the owner's other rows, get a pin there.
func TestNavPinnable_OwnerOnlyForTheirManagePins(t *testing.T) {
	owner := navStateCase{role: 3, path: "/campaigns/c1/trash"}.ctx()
	pinned := NavSectionView{Kind: "pinned"}
	apps := NavSectionView{Kind: "apps"}
	tests := []struct {
		name string
		sec  NavSectionView
		row  NavRowView
		want bool
	}{
		{"their pinned tool", pinned, NavRowView{Key: "manage:trash", Personal: true}, true},
		{"a campaign pin is the editor's", pinned, NavRowView{Key: "app:maps"}, false},
		{"an app row is the editor's", apps, NavRowView{Key: "app:maps"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := navPinnable(owner, tt.sec, tt.row); got != tt.want {
				t.Fatalf("navPinnable = %v, want %v", got, tt.want)
			}
		})
	}
}

// With the tool pinned, its Pinned row holds the ring and Manage only reads as
// current.
func TestNavHasRing_PinnedToolTakesTheRing(t *testing.T) {
	plain := navStateCase{role: 3, path: "/campaigns/c1/trash"}.ctx()
	if !NavHasRing(plain, NavManageKey) {
		t.Errorf("the Manage row should hold the ring on a Manage page")
	}
	pinned := withPinned(plain, "manage:trash")
	if NavHasRing(pinned, NavManageKey) || !NavHasRing(pinned, "manage:trash") {
		t.Errorf("a pinned tool's row should hold the ring, not Manage")
	}
	if !NavIsCurrent(pinned, NavManageKey) || !NavIsCurrent(pinned, "manage:trash") {
		t.Errorf("both rows read as current")
	}
}

// Find is the owner's: nobody else is offered the button, and the frame
// carries it only when it is offered.
func TestManageFind_OnlyTheOwnerGetsOne(t *testing.T) {
	tests := []struct {
		name string
		c    navStateCase
		want bool
	}{
		{"owner", navStateCase{role: 3, path: "/campaigns/c1/trash"}, true},
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1/trash"}, false},
		{"player", navStateCase{role: 1, path: "/campaigns/c1/trash"}, false},
		{"visitor", navStateCase{role: 0, path: "/campaigns/c1/trash"}, false},
		{"owner previewing as a player", navStateCase{role: 3, player: true, path: "/campaigns/c1/trash"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := ManageFind().Render(tt.c.ctx(), &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if !tt.want {
				if out != "" {
					t.Fatalf("Find was drawn: %s", out)
				}
				return
			}
			for _, w := range []string{`data-manage-find`, `data-find-url="/campaigns/c1/dashboard/find"`, `aria-keyshortcuts="/"`, `onclick=`, `<button`, `find_palette.js`} {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
			var frame bytes.Buffer
			if err := ManageFrame(ManageFrameData{Title: "Trash"}).Render(tt.c.ctx(), &frame); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(frame.String(), "data-manage-find") {
				t.Errorf("the frame lacks Find for the owner")
			}
		})
	}
}

// A page that draws its own full-width Find tells the frame, which then leaves
// its header button out: one Find on the page, so "/" has one button to press.
// The field itself is the owner's alone, like the button.
func TestManageFindField_ReplacesTheFramesButton(t *testing.T) {
	tests := []struct {
		name string
		c    navStateCase
		want bool
	}{
		{"owner", navStateCase{role: 3, path: "/campaigns/c1/dashboard"}, true},
		{"scribe", navStateCase{role: 2, path: "/campaigns/c1/dashboard"}, false},
		{"owner previewing as a player", navStateCase{role: 3, player: true, path: "/campaigns/c1/dashboard"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithManageFindField(tt.c.ctx())
			var field, frame bytes.Buffer
			if err := ManageFindField().Render(ctx, &field); err != nil {
				t.Fatal(err)
			}
			if err := ManageFrame(ManageFrameData{Title: "Campaign settings", Home: true}).Render(ctx, &frame); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(field.String(), "data-manage-find"); got != tt.want {
				t.Errorf("field drawn = %v, want %v: %s", got, tt.want, field.String())
			}
			if tt.want && !strings.Contains(field.String(), "Find a setting") {
				t.Errorf("the field says what it finds: %s", field.String())
			}
			if strings.Contains(frame.String(), "data-manage-find") {
				t.Errorf("the frame still draws its own Find: %s", frame.String())
			}
		})
	}
}
