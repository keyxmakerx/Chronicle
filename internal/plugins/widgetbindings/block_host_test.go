// block_host_test.go — the re-render seam's own pins (C-CALV4-HOST-P3 §4/§5).
//
// Three properties, each of which has already cost this repo once:
//
//   - BlockHost must generate a BOX. display:contents made the entity column's
//     `space-y-*` margins no-ops and butted blocks together (QA1 Bug 3), and it
//     would additionally give a container-query-sized block nothing to measure.
//   - the container declaration is PER WIDGET TYPE. Exactly one of the four
//     widgets this wrapper hosts is sized by container queries; the other three
//     must keep today's plain box (block_host.go records what was measured
//     about containment and what was retracted).
//   - BindingAffordance's signature is frozen and delegates; only the new
//     BindingAffordanceFor may name a host type.
package widgetbindings

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// fakeWidgetType stands in for a real widget slug. The tests deliberately do
// NOT name calendar / timeline / maps / worldstate: a quoted plugin slug outside
// its owning plugin directory is what tools/check-plugin-isolation.sh exists to
// catch, and none of these assertions is about a particular plugin.
const fakeWidgetType = "widget-under-test"

func renderComp(t *testing.T, c templ.Component) string {
	t.Helper()
	var sb strings.Builder
	if err := c.Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	return sb.String()
}

// TestBlockHost_IsARealBox. The wrapper is a plain <div> carrying the stable
// swap-target id — never display:contents, which is the QA1 Bug 3 regression.
func TestBlockHost_IsARealBox(t *testing.T) {
	html := renderComp(t, BlockHost(fakeWidgetType, "ent-1", templ.NopComponent))
	if !strings.Contains(html, `<div id="`+BlockHostID(fakeWidgetType, "ent-1")+`"`) {
		t.Errorf("BlockHost must wrap the block in a real <div> box; got %q", html)
	}
	if strings.Contains(html, "display:contents") || strings.Contains(html, "display: contents") {
		t.Error("BlockHost must generate a box — display:contents makes the entity column's " +
			"between-block margins no-ops and leaves container queries nothing to measure")
	}
}

// TestBlockHost_ContainerDeclarationIsOptIn is the guard on §4's ⚠.
//
// An undeclared widget type must carry NO container declaration — three of the
// four widgets this wrapper hosts asked for nothing, and a shared wrapper is
// the wrong place to change layout semantics for all of them at once. A
// declared one must carry it, or the calendar Block's size-class queries
// measure something other than its host.
func TestBlockHost_ContainerDeclarationIsOptIn(t *testing.T) {
	const undeclared = fakeWidgetType + "-undeclared"
	if s := blockHostStyle(undeclared); s != "" {
		t.Errorf("an undeclared widget type must carry no inline style; got %q", s)
	}
	html := renderComp(t, BlockHost(undeclared, "ent-1", templ.NopComponent))
	if strings.Contains(html, "container-type") {
		t.Errorf("an undeclared host must not become a containment context; got %q", html)
	}

	const declared = fakeWidgetType + "-declared"
	DeclareInlineSizeHost(declared)
	if got, want := blockHostStyle(declared), "container-type:inline-size"; got != want {
		t.Errorf("declared host style = %q, want %q", got, want)
	}
	html = renderComp(t, BlockHost(declared, "ent-2", templ.NopComponent))
	if !strings.Contains(html, "container-type:inline-size") {
		t.Errorf("a declared host must be a measured inline-size container; got %q", html)
	}
	// Declaring one type must not leak onto its neighbours.
	if blockHostStyle(undeclared) != "" {
		t.Error("the container declaration leaked to an undeclared widget type")
	}
}

// TestBindingAffordanceFor_NamesItsHostType. The old signature hardcoded
// host_type=entity (the deferred P3b gap). The new one carries whatever host it
// is rendered on, so a dashboard-hosted block's picker reads and writes the
// dashboard's binding rather than an entity's.
func TestBindingAffordanceFor_NamesItsHostType(t *testing.T) {
	dash := renderComp(t, BindingAffordanceFor("camp-1", HostTypeDashboard, fakeWidgetType, "camp-1:player", "widget", SourceDefault))
	if !strings.Contains(dash, "host_type=dashboard") {
		t.Errorf("dashboard affordance must query host_type=dashboard; got %q", dash)
	}
	if strings.Contains(dash, "host_type=entity") {
		t.Error("dashboard affordance must not fall back to the entity host type")
	}

	// An unknown host type degrades to the entity host the affordance always
	// used — never to an empty host_type, which would read no binding at all.
	junk := renderComp(t, BindingAffordanceFor("camp-1", "not-a-host-type", fakeWidgetType, "ent-1", "widget", SourceDefault))
	if !strings.Contains(junk, "host_type=entity") {
		t.Errorf("an unknown host type must degrade to the entity host; got %q", junk)
	}
}

// TestBindingAffordance_DelegatesUnchanged. The four widget blocks call the old
// signature; its output must stay byte-identical to the entity form of the new
// one, or this refactor silently re-cuts four surfaces.
func TestBindingAffordance_DelegatesUnchanged(t *testing.T) {
	for _, src := range []string{SourceOwn, SourceEntityType, SourceDefault} {
		old := renderComp(t, BindingAffordance("camp-1", fakeWidgetType, "ent-9", "widget", src))
		neu := renderComp(t, BindingAffordanceFor("camp-1", HostTypeEntity, fakeWidgetType, "ent-9", "widget", src))
		if old != neu {
			t.Errorf("source %q: BindingAffordance must delegate byte-identically\nold: %s\nnew: %s", src, old, neu)
		}
		if !strings.Contains(old, "host_type=entity") {
			t.Errorf("source %q: the frozen signature must keep the entity host type", src)
		}
	}
}
