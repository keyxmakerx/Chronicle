package systems

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type stubEntityProvider struct {
	dump  EntityFieldDump
	cov   FieldCoverage
	types []EntityTypeInfo
	err   error
}

func (s stubEntityProvider) EntityFields(context.Context, string, string) (EntityFieldDump, error) {
	return s.dump, s.err
}
func (s stubEntityProvider) TypeFieldCoverage(context.Context, string, string) (FieldCoverage, error) {
	return s.cov, s.err
}
func (s stubEntityProvider) EntityTypes(context.Context, string) ([]EntityTypeInfo, error) {
	return s.types, s.err
}

func TestRenderEntityFields(t *testing.T) {
	old := entityDiagProvider
	defer func() { entityDiagProvider = old }()

	entityDiagProvider = nil
	if !strings.Contains(renderEntityFields("c:e"), "not wired") {
		t.Error("nil provider should say not wired")
	}
	if !strings.Contains(renderEntityFields("oops"), "Usage") {
		t.Error("bad arg should show usage")
	}

	entityDiagProvider = stubEntityProvider{dump: EntityFieldDump{
		Found: true, ID: "e1", Name: "Tyne", TypeName: "Hero",
		Fields: map[string]any{"might": 2, "class": "Fury"},
	}}
	out := renderEntityFields("camp:tyne")
	for _, w := range []string{"Tyne", "Hero", "might", "class", "Fury"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}

	entityDiagProvider = stubEntityProvider{dump: EntityFieldDump{Found: true, ID: "e2", Name: "Orrin", Fields: map[string]any{}}}
	if !strings.Contains(renderEntityFields("camp:orrin"), "renders blank") {
		t.Error("empty fields should flag the blank signature")
	}

	entityDiagProvider = stubEntityProvider{dump: EntityFieldDump{Found: false}}
	if !strings.Contains(renderEntityFields("camp:ghost"), "No entity") {
		t.Error("not found should say so")
	}
}

func TestRenderFieldCoverage(t *testing.T) {
	old := entityDiagProvider
	defer func() { entityDiagProvider = old }()

	entityDiagProvider = stubEntityProvider{cov: FieldCoverage{
		Found: true, TypeName: "Hero", EntityCount: 4,
		Declared: []FieldCoverageRow{
			{Key: "might", Label: "Might", NonEmpty: 4},
			{Key: "backstory", Label: "Backstory", NonEmpty: 0},
		},
	}}
	out := renderFieldCoverage("camp:Hero")
	if !strings.Contains(out, "✗ `Backstory` — 0/4") {
		t.Errorf("zero-coverage row missing:\n%s", out)
	}
	if !strings.Contains(out, "✓ `Might` — 4/4 (100%)") {
		t.Errorf("full-coverage row missing:\n%s", out)
	}
	if strings.Index(out, "Backstory") > strings.Index(out, "Might") {
		t.Error("emptiest field should sort first")
	}
}

func TestRenderEntityTypes(t *testing.T) {
	old := entityDiagProvider
	defer func() { entityDiagProvider = old }()

	entityDiagProvider = nil
	if !strings.Contains(renderEntityTypes("c"), "not wired") {
		t.Error("nil provider should say not wired")
	}
	if !strings.Contains(renderEntityTypes(""), "Usage") {
		t.Error("empty arg should show usage")
	}

	entityDiagProvider = stubEntityProvider{types: []EntityTypeInfo{
		{ID: 7, Name: "Heroes", Slug: "heroes", PresetCategory: "character", Count: 3},
	}}
	out := renderEntityTypes("camp")
	for _, w := range []string{"`7`", "Heroes", "heroes", "preset:character", "3 entit"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}

	entityDiagProvider = stubEntityProvider{types: nil}
	if !strings.Contains(renderEntityTypes("camp"), "No entity types") {
		t.Error("empty should say so")
	}
}

func TestRenderCampaignList(t *testing.T) {
	old := campaignListFn
	defer func() { campaignListFn = old }()

	campaignListFn = nil
	if !strings.Contains(renderCampaignList(""), "not wired") {
		t.Error("nil provider should say not wired")
	}

	campaignListFn = func(context.Context) ([]CampaignInfo, error) {
		return []CampaignInfo{{ID: "c1", Name: "Mistale", Slug: "mistale"}}, nil
	}
	out := renderCampaignList("")
	for _, w := range []string{"c1", "Mistale", "mistale"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}

	campaignListFn = func(context.Context) ([]CampaignInfo, error) { return nil, nil }
	if !strings.Contains(renderCampaignList(""), "No campaigns") {
		t.Error("empty should say so")
	}
}

func TestRenderSyncInbound(t *testing.T) {
	old := syncInboundFn
	defer func() { syncInboundFn = old }()

	syncInboundFn = nil
	if !strings.Contains(renderSyncInbound("e1"), "not wired") {
		t.Error("nil provider should say not wired")
	}
	if !strings.Contains(renderSyncInbound(""), "Usage") {
		t.Error("empty arg should show usage")
	}

	syncInboundFn = func(string, int) []InboundSyncRecord {
		return []InboundSyncRecord{{EntityID: "e1", At: time.Unix(1000, 0), Source: "fields", Fields: map[string]any{"might": 2}}}
	}
	out := renderSyncInbound("e1")
	for _, w := range []string{"e1", "fields", "might"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}

	syncInboundFn = func(string, int) []InboundSyncRecord { return nil }
	if !strings.Contains(renderSyncInbound("e1"), "No inbound") {
		t.Error("no records should say so")
	}
}

func TestFileContains_Clamp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.js"), []byte("hello playEntrance world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := clampedPath(dir, "../escape"); ok {
		t.Error("traversal must be blocked")
	}
	if p, ok := clampedPath(dir, "a.js"); !ok || !strings.HasSuffix(p, "a.js") {
		t.Errorf("in-dir path should clamp ok, got %q %v", p, ok)
	}
	data, ok, tooLarge := readClampedFile(dir, "a.js")
	if !ok || tooLarge || !strings.Contains(string(data), "playEntrance") {
		t.Errorf("read failed: ok=%v tooLarge=%v", ok, tooLarge)
	}
	if _, ok, _ := readClampedFile(dir, "../escape"); ok {
		t.Error("traversal read must fail")
	}
}

func TestRenderFileContains_Messages(t *testing.T) {
	if !strings.Contains(renderFileContains("oops"), "Usage") {
		t.Error("bad arg should show usage")
	}
	if !strings.Contains(renderFileContains("nope:widgets/x.js:marker"), "No loaded system") {
		t.Error("unknown system id should say so")
	}
}

func TestPreviewValue(t *testing.T) {
	if got := previewValue("", 10); got != "_(empty)_" {
		t.Errorf("empty preview = %q", got)
	}
	if got := previewValue(strings.Repeat("x", 50), 10); len([]rune(got)) != 11 { // 10 + ellipsis
		t.Errorf("preview not capped: %d runes", len([]rune(got)))
	}
	if got := previewValue("a\nb", 10); strings.Contains(got, "\n") {
		t.Errorf("newline not collapsed: %q", got)
	}
}

func TestCampaignSlotSubstitution(t *testing.T) {
	// ambiguity detection
	if !CampaignSlotIsAmbiguous("entity.fields", "<campaignId>:tyne") {
		t.Error("placeholder campaign should be ambiguous")
	}
	if !CampaignSlotIsAmbiguous("entity.types", "") {
		t.Error("empty campaign should be ambiguous")
	}
	if CampaignSlotIsAmbiguous("entity.fields", "real-id:tyne") {
		t.Error("real campaign id should not be ambiguous")
	}
	if CampaignSlotIsAmbiguous("system.versions", "") {
		t.Error("non-campaign-scoped diagnostic should never be ambiguous")
	}
	// substitution preserves the rest of the arg
	if got := WithCampaign("entity.fields", "<campaignId>:tyne", "c9"); got != "c9:tyne" {
		t.Errorf("WithCampaign = %q, want c9:tyne", got)
	}
	if got := WithCampaign("entity.types", "<campaignId>", "c9"); got != "c9" {
		t.Errorf("WithCampaign whole = %q, want c9", got)
	}
	// ApplyCampaignPick only touches ambiguous, campaign-scoped calls
	plan := &BatchPlan{Calls: []PlannedCall{
		{Name: "entity.fields", Arg: "<campaignId>:tyne"},
		{Name: "entity.fields", Arg: "real:orrin"},
		{Name: "sync.inbound", Arg: "<tyneId>"},
	}}
	ApplyCampaignPick(plan, "c9")
	if plan.Calls[0].Arg != "c9:tyne" {
		t.Errorf("call0 = %q", plan.Calls[0].Arg)
	}
	if plan.Calls[1].Arg != "real:orrin" {
		t.Errorf("call1 should be untouched, got %q", plan.Calls[1].Arg)
	}
	if plan.Calls[2].Arg != "<tyneId>" {
		t.Errorf("sync.inbound (not campaign-scoped) should be untouched, got %q", plan.Calls[2].Arg)
	}
	// After substitution nothing should still need a campaign (call2 isn't scoped).
	if PlanNeedsCampaign(plan) {
		t.Error("after substitution no call should still need a campaign")
	}
}
