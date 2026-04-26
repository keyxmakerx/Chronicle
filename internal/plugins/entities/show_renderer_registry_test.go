package entities

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// markerComponent is a tiny templ.Component that writes a fixed string,
// used to assert "the registered renderer ran" without dragging in
// real templ output. Cheaper than a full templ test fixture.
type markerComponent struct{ tag string }

func (m markerComponent) Render(_ context.Context, w io.Writer) error {
	_, err := w.Write([]byte(m.tag))
	return err
}

func newMarkerRenderer(tag string) EntityShowRenderer {
	return func(_ EntityShowRenderContext) templ.Component {
		return markerComponent{tag: tag}
	}
}

// TestEntityShowRendererRegistry_RegisterAndLookup pins the basic
// happy path: register a renderer for a slug, look it up, get back
// the same renderer.
func TestEntityShowRendererRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	want := newMarkerRenderer("drawsteel-char")
	reg.Register("drawsteel-character", want)

	got, ok := reg.Lookup("drawsteel-character")
	if !ok {
		t.Fatal("expected lookup hit, got miss")
	}
	if got == nil {
		t.Fatal("expected non-nil renderer")
	}
}

// TestEntityShowRendererRegistry_LookupMiss confirms unregistered
// slugs return (nil, false). This is the *only* failure mode CH4
// promises — the caller branches on the bool to fall through to
// the existing block dispatch.
func TestEntityShowRendererRegistry_LookupMiss(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	reg.Register("drawsteel-character", newMarkerRenderer("a"))

	got, ok := reg.Lookup("location")
	if ok {
		t.Errorf("expected miss for unregistered slug, got hit")
	}
	if got != nil {
		t.Errorf("expected nil renderer on miss, got %v", got)
	}
}

// TestEntityShowRendererRegistry_RegisterReplaces guards the documented
// "last registration wins" behavior. System packages can override each
// other deterministically when their wiring order in routes.go is
// explicit; this test pins that contract so a future change to
// "first wins" or "panic on conflict" trips the test.
func TestEntityShowRendererRegistry_RegisterReplaces(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	reg.Register("char", newMarkerRenderer("first"))
	reg.Register("char", newMarkerRenderer("second"))

	got, ok := reg.Lookup("char")
	if !ok || got == nil {
		t.Fatal("expected hit after re-register")
	}

	// Render the renderer to confirm it's the second one.
	var buf bytesBufferLike
	if err := got(EntityShowRenderContext{}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if buf.String() != "second" {
		t.Errorf("expected last-registered renderer (%q), got %q", "second", buf.String())
	}
}

// TestEntityShowRendererRegistry_EmptyArgsAreNoops asserts that
// register-with-empty-slug and register-with-nil-renderer are both
// silent no-ops, not panics. A typo in a system package's wiring
// shouldn't crash the boot.
func TestEntityShowRendererRegistry_EmptyArgsAreNoops(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	reg.Register("", newMarkerRenderer("won't be reachable"))
	reg.Register("legit", nil)

	if _, ok := reg.Lookup(""); ok {
		t.Error("expected empty-slug Register to no-op; lookup was a hit")
	}
	if _, ok := reg.Lookup("legit"); ok {
		t.Error("expected nil-renderer Register to no-op; lookup was a hit")
	}
}

// TestLookupEntityShowRenderer_NilRegistry covers the boot-window
// case: globalEntityShowRendererRegistry is nil before
// SetGlobalEntityShowRendererRegistry runs. lookupEntityShowRenderer
// must return nil so show.templ falls through to block dispatch
// instead of panicking.
func TestLookupEntityShowRenderer_NilRegistry(t *testing.T) {
	prev := globalEntityShowRendererRegistry
	globalEntityShowRendererRegistry = nil
	t.Cleanup(func() { globalEntityShowRendererRegistry = prev })

	got := lookupEntityShowRenderer(EntityShowRenderContext{
		EntityType: &EntityType{Slug: "anything"},
	})
	if got != nil {
		t.Errorf("expected nil with nil registry, got %v", got)
	}
}

// TestLookupEntityShowRenderer_NilEntityType guards a defensive branch
// against handler bugs that pass a nil EntityType (shouldn't happen,
// but a panic in dispatch would 500 the entire entity show page).
func TestLookupEntityShowRenderer_NilEntityType(t *testing.T) {
	prev := globalEntityShowRendererRegistry
	globalEntityShowRendererRegistry = NewEntityShowRendererRegistry()
	t.Cleanup(func() { globalEntityShowRendererRegistry = prev })

	got := lookupEntityShowRenderer(EntityShowRenderContext{EntityType: nil})
	if got != nil {
		t.Errorf("expected nil with nil EntityType, got %v", got)
	}
}

// TestLookupEntityShowRenderer_SlugDispatch confirms the helper's full
// happy path: registry installed, EntityType present with slug, a
// matching renderer registered → the helper returns its component.
func TestLookupEntityShowRenderer_SlugDispatch(t *testing.T) {
	prev := globalEntityShowRendererRegistry
	reg := NewEntityShowRendererRegistry()
	reg.Register("drawsteel-character", newMarkerRenderer("rendered!"))
	globalEntityShowRendererRegistry = reg
	t.Cleanup(func() { globalEntityShowRendererRegistry = prev })

	got := lookupEntityShowRenderer(EntityShowRenderContext{
		EntityType: &EntityType{Slug: "drawsteel-character"},
	})
	if got == nil {
		t.Fatal("expected non-nil component on slug hit")
	}

	var buf bytesBufferLike
	if err := got.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if buf.String() != "rendered!" {
		t.Errorf("expected component to render its marker, got %q", buf.String())
	}
}

// TestEntityShowRendererRegistry_ConcurrentAccess is paranoia + race-
// detector check: in production the registry is written once at boot
// and read at request time, but the lock is documented as making
// concurrent access safe, so prove it under -race.
func TestEntityShowRendererRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewEntityShowRendererRegistry()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func(slug string) {
			defer wg.Done()
			reg.Register(slug, newMarkerRenderer(slug))
		}(uniqueSlug(i))
		go func(slug string) {
			defer wg.Done()
			_, _ = reg.Lookup(slug)
		}(uniqueSlug(i))
	}
	wg.Wait()
}

// --- Tiny helpers ---

func uniqueSlug(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	return string(letters[i%len(letters)]) + "-test"
}

// bytesBufferLike is a minimal io.Writer that captures bytes for
// assertion. Avoids pulling in `bytes` just for tests.
type bytesBufferLike struct{ b []byte }

func (b *bytesBufferLike) Write(p []byte) (int, error) {
	b.b = append(b.b, p...)
	return len(p), nil
}

func (b *bytesBufferLike) String() string { return string(b.b) }

// TestMakeWidgetMountRenderer_EmitsBootJSMountPoint pins the contract
// CH4.5 promises to manifest authors: a renderer built from
// MakeWidgetMountRenderer renders one <div> with three data-* attributes
// that boot.js can find and mount the widget against.
func TestMakeWidgetMountRenderer_EmitsBootJSMountPoint(t *testing.T) {
	renderer := MakeWidgetMountRenderer("drawsteel-character-card")
	ctx := EntityShowRenderContext{
		CC:     &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-123"}},
		Entity: &Entity{ID: "ent-456"},
	}
	component := renderer(ctx)

	buf := &bytesBufferLike{}
	if err := component.Render(context.Background(), buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		`data-widget="drawsteel-character-card"`,
		`data-entity-id="ent-456"`,
		`data-campaign-id="camp-123"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered HTML missing %q:\n%s", want, got)
		}
	}
}

// TestMakeWidgetMountRenderer_HandlesMissingContext is paranoia for the
// boot path: if the registry is consulted before CC/Entity are populated
// (shouldn't happen in real flow, but cheap to defend), the renderer
// must not panic — it should emit empty data-* attributes instead.
func TestMakeWidgetMountRenderer_HandlesMissingContext(t *testing.T) {
	renderer := MakeWidgetMountRenderer("x")
	component := renderer(EntityShowRenderContext{})
	buf := &bytesBufferLike{}
	if err := component.Render(context.Background(), buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `data-widget="x"`) {
		t.Errorf("widget attr missing: %s", buf.String())
	}
}

// TestMakeWidgetMountRenderer_EscapesAttributes pins the XSS guard. The
// widget slug comes from the manifest (already validated), but
// entity/campaign IDs come from the database — defense-in-depth says
// any attempt to break out of the attribute value via raw quotes is
// neutralized by HTML escaping before it reaches the response.
func TestMakeWidgetMountRenderer_EscapesAttributes(t *testing.T) {
	renderer := MakeWidgetMountRenderer("ok")
	ctx := EntityShowRenderContext{
		Entity: &Entity{ID: `"><script>x</script>`},
	}
	buf := &bytesBufferLike{}
	if err := renderer(ctx).Render(context.Background(), buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(buf.String(), "<script>") {
		t.Errorf("script tag leaked through escaping: %s", buf.String())
	}
}
