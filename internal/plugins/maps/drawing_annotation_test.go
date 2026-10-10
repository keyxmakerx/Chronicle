package maps

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// annotationInput is a valid create input for typ; cases change one thing.
func annotationInput(typ string) CreateDrawingInput {
	in := CreateDrawingInput{
		MapID: "map-1", DrawingType: typ, StrokeColor: "#2563eb", StrokeWidth: 4,
		CallerRole: permissions.RoleScribe, CreatedBy: "u-1",
	}
	switch typ {
	case DrawingTypeArrow:
		in.Points = pts(`[{"x":10,"y":10},{"x":40,"y":30}]`)
	case DrawingTypeHighlight:
		in.Points = pts(`[{"x":10,"y":10},{"x":12,"y":11},{"x":20,"y":12}]`)
		in.StrokeWidth = 18
		in.FillAlpha = 0.35
	case DrawingTypeStep:
		in.Points = pts(`[{"x":50,"y":50}]`)
		in.TextContent = strp("3")
	case DrawingTypeCallout:
		in.Points = pts(`[{"x":50,"y":50}]`)
		in.TextContent = strp("Rings at low tide")
	}
	return in
}

func TestCreateDrawing_Annotations(t *testing.T) {
	manyPts := func(n int) json.RawMessage {
		var b strings.Builder
		b.WriteString("[")
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`{"x":1,"y":1}`)
		}
		b.WriteString("]")
		return json.RawMessage(b.String())
	}
	cases := []struct {
		name   string
		typ    string
		modify func(*CreateDrawingInput)
		ok     bool
		check  func(*testing.T, *Drawing)
	}{
		{"arrow: tail to tip", DrawingTypeArrow, nil, true, nil},
		{"arrow: one point", DrawingTypeArrow, func(in *CreateDrawingInput) { in.Points = pts(`[{"x":1,"y":1}]`) }, false, nil},
		{"arrow: three points", DrawingTypeArrow, func(in *CreateDrawingInput) { in.Points = pts(`[{"x":1,"y":1},{"x":2,"y":2},{"x":3,"y":3}]`) }, false, nil},
		{"arrow: tail equals tip", DrawingTypeArrow, func(in *CreateDrawingInput) { in.Points = pts(`[{"x":1,"y":1},{"x":1,"y":1}]`) }, false, nil},
		{"arrow: off the map", DrawingTypeArrow, func(in *CreateDrawingInput) { in.Points = pts(`[{"x":-5,"y":1},{"x":2,"y":2}]`) }, false, nil},
		{"arrow: malformed points", DrawingTypeArrow, func(in *CreateDrawingInput) { in.Points = pts(`[[1,1],[2,2]]`) }, false, nil},
		{"arrow: carries text", DrawingTypeArrow, func(in *CreateDrawingInput) { in.TextContent = strp("hi") }, false, nil},
		{"arrow: too wide", DrawingTypeArrow, func(in *CreateDrawingInput) { in.StrokeWidth = 61 }, false, nil},
		{"arrow: turned", DrawingTypeArrow, func(in *CreateDrawingInput) { in.Rotation = 10 }, false, nil},
		{"arrow: colour that is not hex", DrawingTypeArrow, func(in *CreateDrawingInput) { in.StrokeColor = "red;x" }, false, nil},
		{"arrow: fill colour that is not hex", DrawingTypeArrow, func(in *CreateDrawingInput) { in.FillColor = strp("url(#a)") }, false, nil},
		{"arrow: carries a picture", DrawingTypeArrow, func(in *CreateDrawingInput) { in.ImageID = strp("m-1") }, false, nil},

		{"highlight: stroke", DrawingTypeHighlight, nil, true, nil},
		{"highlight: unset opacity gets the default", DrawingTypeHighlight, func(in *CreateDrawingInput) { in.FillAlpha = 0 }, true,
			func(t *testing.T, d *Drawing) {
				if d.FillAlpha != defaultHighlightAlpha {
					t.Errorf("fill_alpha = %v, want %v", d.FillAlpha, defaultHighlightAlpha)
				}
			}},
		{"highlight: opaque", DrawingTypeHighlight, func(in *CreateDrawingInput) { in.FillAlpha = 1 }, false, nil},
		{"highlight: one point", DrawingTypeHighlight, func(in *CreateDrawingInput) { in.Points = pts(`[{"x":1,"y":1}]`) }, false, nil},
		{"highlight: a long stroke under the shared ceiling", DrawingTypeHighlight, func(in *CreateDrawingInput) { in.Points = manyPts(600) }, true, nil},
		{"highlight: over the shared ceiling", DrawingTypeHighlight, func(in *CreateDrawingInput) { in.Points = manyPts(1000) }, false, nil},

		{"step: number", DrawingTypeStep, nil, true, nil},
		{"step: number is stored canonical", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = strp(" 007 ") }, true,
			func(t *testing.T, d *Drawing) {
				if d.TextContent == nil || *d.TextContent != "7" {
					t.Errorf("text_content = %v, want 7", d.TextContent)
				}
			}},
		{"step: the cap", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = strp("999") }, true, nil},
		{"step: over the cap", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = strp("1000") }, false, nil},
		{"step: zero", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = strp("0") }, false, nil},
		{"step: negative", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = strp("-2") }, false, nil},
		{"step: not a number", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = strp("<b>") }, false, nil},
		{"step: no number", DrawingTypeStep, func(in *CreateDrawingInput) { in.TextContent = nil }, false, nil},
		{"step: two points", DrawingTypeStep, func(in *CreateDrawingInput) { in.Points = pts(`[{"x":1,"y":1},{"x":2,"y":2}]`) }, false, nil},
		{"step: text size in range", DrawingTypeStep, func(in *CreateDrawingInput) { in.FontSize = intp(16) }, true, nil},
		{"step: text size too big", DrawingTypeStep, func(in *CreateDrawingInput) { in.FontSize = intp(400) }, false, nil},

		{"callout: text", DrawingTypeCallout, nil, true, nil},
		{"callout: text is trimmed", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = strp("  two\nlines  ") }, true,
			func(t *testing.T, d *Drawing) {
				if *d.TextContent != "two\nlines" {
					t.Errorf("text_content = %q", *d.TextContent)
				}
			}},
		{"callout: markup is kept as text", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = strp(`<img src=x onerror=alert(1)>`) }, true, nil},
		{"callout: at the limit", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = strp(strings.Repeat("é", MaxDrawingTextRunes)) }, true, nil},
		{"callout: over the limit", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = strp(strings.Repeat("a", MaxDrawingTextRunes+1)) }, false, nil},
		{"callout: blank", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = strp("   ") }, false, nil},
		{"callout: no text", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = nil }, false, nil},
		{"callout: control character", DrawingTypeCallout, func(in *CreateDrawingInput) { in.TextContent = strp("a\x00b") }, false, nil},
		{"callout: no point", DrawingTypeCallout, func(in *CreateDrawingInput) { in.Points = pts(`[]`) }, false, nil},

		{"text label: over the shared limit", "text", func(in *CreateDrawingInput) {
			in.Points = pts(`[{"x":1,"y":1}]`)
			in.TextContent = strp(strings.Repeat("a", MaxDrawingTextRunes+1))
		}, false, nil},
		{"text label: at the shared limit", "text", func(in *CreateDrawingInput) {
			in.Points = pts(`[{"x":1,"y":1}]`)
			in.TextContent = strp(strings.Repeat("a", MaxDrawingTextRunes))
		}, true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := annotationInput(tc.typ)
			if tc.modify != nil {
				tc.modify(&in)
			}
			repo := &shadowRepo{}
			d, err := NewDrawingService(repo).CreateDrawing(context.Background(), in)
			if tc.ok {
				if err != nil {
					t.Fatalf("want accepted, got %v", err)
				}
				if repo.created == nil || repo.created.DrawingType != tc.typ {
					t.Fatalf("not stored as %s: %+v", tc.typ, repo.created)
				}
				if tc.check != nil {
					tc.check(t, d)
				}
				return
			}
			if err == nil {
				t.Fatal("want refused, got accepted")
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != 400 {
				t.Errorf("want a 400, got %v", err)
			}
			if repo.created != nil {
				t.Error("a refused drawing must not be stored")
			}
		})
	}
}

// The draw gate applies to annotations like any drawing.
func TestCreateDrawing_AnnotationsNeedDrawAccess(t *testing.T) {
	for _, typ := range []string{DrawingTypeArrow, DrawingTypeHighlight, DrawingTypeStep, DrawingTypeCallout} {
		in := annotationInput(typ)
		in.CallerRole = permissions.RolePlayer
		repo := &shadowRepo{}
		if _, err := NewDrawingService(repo).CreateDrawing(context.Background(), in); err == nil || repo.created != nil {
			t.Errorf("%s: a player must not draw (err %v)", typ, err)
		}
	}
}

// annotationRepo hands back one stored drawing and records the write.
type annotationRepo struct {
	idorRepo
	stored  Drawing
	written *Drawing
}

func (r *annotationRepo) GetDrawing(context.Context, string) (*Drawing, error) {
	d := r.stored
	return &d, nil
}
func (r *annotationRepo) UpdateDrawing(_ context.Context, d *Drawing) error {
	r.written = d
	return nil
}

func TestUpdateDrawing_AnnotationsRecheckTheMergedRow(t *testing.T) {
	stored := func(typ string) Drawing {
		in := annotationInput(typ)
		return Drawing{ID: "d-1", MapID: "map-1", DrawingType: typ, Points: in.Points, StrokeColor: in.StrokeColor,
			StrokeWidth: in.StrokeWidth, FillAlpha: in.FillAlpha, TextContent: in.TextContent, Visibility: "everyone"}
	}
	cases := []struct {
		name  string
		typ   string
		input UpdateDrawingInput
		ok    bool
	}{
		{"callout: new text", DrawingTypeCallout, UpdateDrawingInput{TextContent: patch.Of("Wights rise here")}, true},
		{"callout: text cleared", DrawingTypeCallout, UpdateDrawingInput{TextContent: patch.Null[string]()}, false},
		{"callout: text too long", DrawingTypeCallout, UpdateDrawingInput{TextContent: patch.Of(strings.Repeat("a", MaxDrawingTextRunes+1))}, false},
		{"callout: move keeps the text", DrawingTypeCallout, UpdateDrawingInput{Points: patch.Of(pts(`[{"x":70,"y":20}]`))}, true},
		{"step: renumber", DrawingTypeStep, UpdateDrawingInput{TextContent: patch.Of("12")}, true},
		{"step: renumber past the cap", DrawingTypeStep, UpdateDrawingInput{TextContent: patch.Of("1000")}, false},
		{"arrow: visibility only", DrawingTypeArrow, UpdateDrawingInput{Visibility: patch.Of("dm_only")}, true},
		{"arrow: collapsed to a point", DrawingTypeArrow, UpdateDrawingInput{Points: patch.Of(pts(`[{"x":5,"y":5},{"x":5,"y":5}]`))}, false},
		{"arrow: given text", DrawingTypeArrow, UpdateDrawingInput{TextContent: patch.Of("x")}, false},
		{"highlight: opacity too high", DrawingTypeHighlight, UpdateDrawingInput{FillAlpha: patch.Of(0.95)}, false},
		{"highlight: colour that is not hex", DrawingTypeHighlight, UpdateDrawingInput{StrokeColor: patch.Of("javascript:")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &annotationRepo{stored: stored(tc.typ)}
			err := NewDrawingService(repo).UpdateDrawing(context.Background(), "d-1", "map-1", permissions.RoleOwner, true, tc.input)
			if tc.ok && err != nil {
				t.Fatalf("want accepted, got %v", err)
			}
			if !tc.ok && (err == nil || repo.written != nil) {
				t.Fatalf("want refused without a write, got err=%v written=%v", err, repo.written != nil)
			}
			if tc.name == "callout: move keeps the text" && (repo.written.TextContent == nil || *repo.written.TextContent != "Rings at low tide") {
				t.Error("a points-only update must keep the bubble's text")
			}
		})
	}
}

// A stored label longer than the bound still accepts edits that leave its text
// alone; only writing new text is held to the bound.
func TestUpdateDrawing_LongStoredLabelKeepsWorking(t *testing.T) {
	long := strings.Repeat("a", MaxDrawingTextRunes+50)
	repo := &annotationRepo{stored: Drawing{ID: "d-1", MapID: "map-1", DrawingType: "text", Points: pts(`[{"x":1,"y":1}]`), TextContent: &long}}
	svc := NewDrawingService(repo)
	if err := svc.UpdateDrawing(context.Background(), "d-1", "map-1", permissions.RoleOwner, true, UpdateDrawingInput{StrokeColor: patch.Of("#c0392b")}); err != nil {
		t.Errorf("a colour change must not trip the text bound: %v", err)
	}
	repo.written = nil
	if err := svc.UpdateDrawing(context.Background(), "d-1", "map-1", permissions.RoleOwner, true, UpdateDrawingInput{TextContent: patch.Of(long)}); err == nil {
		t.Error("writing over-long text must be refused")
	}
}

// Every annotation is withheld from players and scribes under a shadow, by
// the same rule as any drawing, and the owner still gets it.
func TestListDrawings_AnnotationsUnderShadow(t *testing.T) {
	under := []Drawing{
		{ID: "arrow-in", MapID: "map-1", DrawingType: DrawingTypeArrow, Points: pts(`[{"x":12,"y":12},{"x":28,"y":20}]`)},
		{ID: "hl-in", MapID: "map-1", DrawingType: DrawingTypeHighlight, Points: pts(`[{"x":12,"y":12},{"x":14,"y":13},{"x":20,"y":15}]`)},
		{ID: "step-in", MapID: "map-1", DrawingType: DrawingTypeStep, Points: pts(`[{"x":20,"y":20}]`), TextContent: strp("1")},
		{ID: "bubble-in", MapID: "map-1", DrawingType: DrawingTypeCallout, Points: pts(`[{"x":25,"y":25}]`), TextContent: strp("secret")},
	}
	clear := []Drawing{
		{ID: "arrow-half", MapID: "map-1", DrawingType: DrawingTypeArrow, Points: pts(`[{"x":12,"y":12},{"x":60,"y":60}]`)},
		{ID: "step-out", MapID: "map-1", DrawingType: DrawingTypeStep, Points: pts(`[{"x":70,"y":70}]`), TextContent: strp("2")},
		{ID: "bubble-out", MapID: "map-1", DrawingType: DrawingTypeCallout, Points: pts(`[{"x":80,"y":10}]`), TextContent: strp("open")},
	}
	all := append(append([]Drawing{testShadow}, under...), clear...)
	svc := NewDrawingService(&shadowRepo{drawings: all, shadows: []Drawing{testShadow}})

	for _, tc := range []struct {
		name string
		role int
		want int
	}{
		{"player", permissions.RolePlayer, 1 + len(clear)},
		{"scribe", permissions.RoleScribe, 1 + len(clear)},
		{"owner / co-DM", permissions.RoleOwner, len(all)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ListDrawings(context.Background(), "map-1", tc.role, "u")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("sees %d drawings, want %d", len(got), tc.want)
			}
			if tc.role < permissions.RoleOwner {
				for _, d := range got {
					if strings.HasSuffix(d.ID, "-in") {
						t.Errorf("%s must be withheld under the shadow", d.ID)
					}
				}
			}
		})
	}
	for _, d := range under {
		d := d
		hidden, err := svc.IsDrawingShadowed(context.Background(), &d, permissions.RolePlayer)
		if err != nil || !hidden {
			t.Errorf("by id: %s must read as hidden to a player (got %v, %v)", d.ID, hidden, err)
		}
	}
}

// Annotations wholly in unexplored hexes are withheld like any drawing.
func TestListDrawings_AnnotationsUnderHexFog(t *testing.T) {
	dx, dy, lx, ly := fogPositions()
	pt := func(x, y float64) json.RawMessage { return json.RawMessage(`[{"x":` + fl(x) + `,"y":` + fl(y) + `}]`) }
	two := func(a, b, c, d float64) json.RawMessage {
		return json.RawMessage(`[{"x":` + fl(a) + `,"y":` + fl(b) + `},{"x":` + fl(c) + `,"y":` + fl(d) + `}]`)
	}
	ds := []Drawing{
		{ID: "step-dark", MapID: "map-1", DrawingType: DrawingTypeStep, Points: pt(dx, dy), TextContent: strp("1")},
		{ID: "bubble-dark", MapID: "map-1", DrawingType: DrawingTypeCallout, Points: pt(dx, dy), TextContent: strp("x")},
		{ID: "arrow-dark", MapID: "map-1", DrawingType: DrawingTypeArrow, Points: two(dx, dy, dx+0.5, dy)},
		{ID: "hl-dark", MapID: "map-1", DrawingType: DrawingTypeHighlight, Points: two(dx, dy, dx+0.5, dy)},
		{ID: "step-lit", MapID: "map-1", DrawingType: DrawingTypeStep, Points: pt(lx, ly), TextContent: strp("2")},
		{ID: "arrow-into-dark", MapID: "map-1", DrawingType: DrawingTypeArrow, Points: two(lx, ly, dx, dy)},
	}
	svc := NewDrawingService(&shadowRepo{drawings: ds})
	svc.SetHexFogLookup(fakeFogLookup{mask: fogMaskFixture()})
	ids := func(list []Drawing) string {
		var out []string
		for _, d := range list {
			out = append(out, d.ID)
		}
		return strings.Join(out, ",")
	}
	for _, tc := range []struct {
		name string
		role int
		want string
	}{
		{"public visitor", permissions.RoleNone, "step-lit,arrow-into-dark"},
		{"player", permissions.RolePlayer, "step-lit,arrow-into-dark"},
		{"scribe", permissions.RoleScribe, "step-lit,arrow-into-dark"},
		{"owner / co-DM", permissions.RoleOwner, "step-dark,bubble-dark,arrow-dark,hl-dark,step-lit,arrow-into-dark"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ListDrawings(context.Background(), "map-1", tc.role, "u")
			if err != nil || ids(got) != tc.want {
				t.Errorf("sees %q (err %v), want %q", ids(got), err, tc.want)
			}
		})
	}
}

// Through the real repository: each annotation round-trips what the viewer
// draws from (points, colour, width, opacity, number, text), and a dm_only
// one is absent from a player's list while the owner still gets it.
func TestAnnotations_RoundTripAndDMOnly_Integration(t *testing.T) {
	db := newMapsScratchDB(t)
	ctx := context.Background()
	svc := NewDrawingService(NewDrawingRepository(db))

	userID := newMapsDBID(t)
	campaignID := newMapsDBID(t)
	mapID := newMapsDBID(t)
	mustExecMaps(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, userID+"@example.test", "Maps Annotation Int Test", "x")
	mustExecMaps(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Maps Annotation Int Test", campaignID, userID)
	mustExecMaps(t, db, `INSERT INTO maps (id, campaign_id, name) VALUES (?, ?, ?)`,
		mapID, campaignID, "Test Map")

	made := map[string]*Drawing{}
	for _, typ := range []string{DrawingTypeArrow, DrawingTypeHighlight, DrawingTypeStep, DrawingTypeCallout} {
		in := annotationInput(typ)
		in.MapID = mapID
		in.CreatedBy = userID
		in.CallerRole = permissions.RoleOwner
		in.CallerIsDM = true
		d, err := svc.CreateDrawing(ctx, in)
		if err != nil {
			t.Fatalf("%s: CreateDrawing: %v", typ, err)
		}
		made[typ] = d
	}
	hiddenIn := annotationInput(DrawingTypeCallout)
	hiddenIn.MapID, hiddenIn.CreatedBy, hiddenIn.CallerRole, hiddenIn.CallerIsDM = mapID, userID, permissions.RoleOwner, true
	hiddenIn.Visibility = "dm_only"
	hiddenIn.TextContent = strp("Only the DM reads this")
	hidden, err := svc.CreateDrawing(ctx, hiddenIn)
	if err != nil {
		t.Fatal(err)
	}

	for typ, d := range made {
		got, err := svc.GetDrawing(ctx, d.ID)
		if err != nil {
			t.Fatalf("%s: GetDrawing: %v", typ, err)
		}
		if got.DrawingType != typ || got.StrokeColor != d.StrokeColor || got.StrokeWidth != d.StrokeWidth {
			t.Errorf("%s did not round-trip: %+v", typ, got)
		}
		var p []pointXY
		if err := json.Unmarshal(got.Points, &p); err != nil || len(p) == 0 {
			t.Errorf("%s: points did not round-trip: %s", typ, got.Points)
		}
	}
	if s := made[DrawingTypeStep]; s.TextContent == nil || *s.TextContent != "3" {
		t.Errorf("step number did not round-trip")
	}
	if b, _ := svc.GetDrawing(ctx, made[DrawingTypeCallout].ID); b.TextContent == nil || *b.TextContent != "Rings at low tide" {
		t.Errorf("bubble text did not round-trip")
	}
	if h, _ := svc.GetDrawing(ctx, made[DrawingTypeHighlight].ID); h.FillAlpha != 0.35 {
		t.Errorf("highlighter opacity = %v, want 0.35", h.FillAlpha)
	}

	player, err := svc.ListDrawings(ctx, mapID, permissions.RolePlayer, "u-player")
	if err != nil {
		t.Fatal(err)
	}
	if len(player) != 4 {
		t.Errorf("player sees %d drawings, want the 4 visible ones", len(player))
	}
	for _, d := range player {
		if d.ID == hidden.ID {
			t.Error("a dm_only bubble reached a player")
		}
	}
	owner, _ := svc.ListDrawings(ctx, mapID, permissions.RoleOwner, userID)
	if len(owner) != 5 {
		t.Errorf("owner sees %d drawings, want all 5", len(owner))
	}
}
