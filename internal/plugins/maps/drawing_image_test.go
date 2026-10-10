package maps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// imageRepo stores one drawing in memory; the embedded nil interface makes any
// method the tests do not expect panic instead of silently passing.
type imageRepo struct {
	DrawingRepository
	stored  *Drawing
	created *Drawing
	updated *Drawing
}

func (r *imageRepo) CreateDrawing(_ context.Context, d *Drawing) error { r.created = d; return nil }
func (r *imageRepo) GetDrawing(context.Context, string) (*Drawing, error) {
	c := *r.stored
	return &c, nil
}
func (r *imageRepo) UpdateDrawing(_ context.Context, d *Drawing) error      { r.updated = d; return nil }
func (r *imageRepo) ListShadows(context.Context, string) ([]Drawing, error) { return nil, nil }

// fakeMedia knows which media ids belong to which campaign and counts lookups.
type fakeMedia struct {
	files map[string]string // media id -> campaign id
	err   error
	calls int
}

func (m *fakeMedia) ImageInCampaign(_ context.Context, mediaID, campaignID string) (bool, error) {
	m.calls++
	if m.err != nil {
		return false, m.err
	}
	return m.files[mediaID] == campaignID && campaignID != "", nil
}

func newImageService(repo DrawingRepository, media MediaVerifier) DrawingService {
	s := NewDrawingService(repo)
	s.SetMapLookup(func(context.Context, string) (string, error) { return "camp-1", nil })
	if media != nil {
		s.SetMediaVerifier(media)
	}
	return s
}

func strp(s string) *string { return &s }

const twoCorners = `[{"x":10,"y":10},{"x":40,"y":30}]`

func TestCreateDrawing_Image(t *testing.T) {
	media := &fakeMedia{files: map[string]string{"mine": "camp-1", "theirs": "camp-2"}}
	cases := []struct {
		name    string
		in      CreateDrawingInput
		wantErr bool
		alpha   float64
	}{
		{"valid, opacity unset means opaque", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine")}, false, 1.0},
		{"opacity below the floor is raised", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), FillAlpha: 0.05}, false, 0.2},
		{"opacity above 1 is capped", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), FillAlpha: 7}, false, 1.0},
		{"opacity inside the range is kept", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), FillAlpha: 0.6}, false, 0.6},
		{"crop at the limit", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Crop: json.RawMessage(`{"t":45,"r":0,"b":45,"l":10}`)}, false, 1.0},
		{"crop sent as null is no crop", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Crop: json.RawMessage(`null`)}, false, 1.0},
		{"missing image id", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners)}, true, 0},
		{"blank image id", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("  ")}, true, 0},
		{"another campaign's file", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("theirs")}, true, 0},
		{"unknown file", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("nope")}, true, 0},
		{"one point", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(`[{"x":1,"y":1}]`), ImageID: strp("mine")}, true, 0},
		{"three points", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(`[{"x":1,"y":1},{"x":2,"y":2},{"x":3,"y":3}]`), ImageID: strp("mine")}, true, 0},
		{"crop side over 45", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Crop: json.RawMessage(`{"t":45.5,"r":0,"b":0,"l":0}`)}, true, 0},
		{"negative crop side", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Crop: json.RawMessage(`{"t":-1,"r":0,"b":0,"l":0}`)}, true, 0},
		{"crop is not an object", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Crop: json.RawMessage(`[1,2]`)}, true, 0},
		{"turned past 45 degrees", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Rotation: 46}, true, 0},
		{"turned the full 45 is fine", CreateDrawingInput{DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"), Rotation: -45}, false, 1.0},
		{"rectangle carrying an image id", CreateDrawingInput{DrawingType: "rectangle", Points: json.RawMessage(twoCorners), ImageID: strp("mine")}, true, 0},
		{"rectangle carrying a crop", CreateDrawingInput{DrawingType: "rectangle", Points: json.RawMessage(twoCorners), Crop: json.RawMessage(`{"t":1,"r":1,"b":1,"l":1}`)}, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &imageRepo{}
			tc.in.MapID, tc.in.CallerRole = "map-1", permissions.RoleScribe
			d, err := newImageService(repo, media).CreateDrawing(context.Background(), tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
					t.Errorf("want a 400 domain error, got %v", err)
				}
				if repo.created != nil {
					t.Error("a refused picture reached persistence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if d.FillAlpha != tc.alpha {
				t.Errorf("fill_alpha = %v, want %v", d.FillAlpha, tc.alpha)
			}
			if d.ImageID == nil || *d.ImageID != "mine" {
				t.Errorf("image id not stored: %v", d.ImageID)
			}
		})
	}
}

// With the media seam unwired a picture write must fail closed, never store an
// unchecked id.
func TestCreateDrawing_ImageFailsClosedWithoutVerifier(t *testing.T) {
	repo := &imageRepo{}
	_, err := newImageService(repo, nil).CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-1", DrawingType: "image", Points: json.RawMessage(twoCorners),
		ImageID: strp("mine"), CallerRole: permissions.RoleOwner,
	})
	if err == nil || repo.created != nil {
		t.Fatalf("want a refusal with nothing stored, got err=%v created=%v", err, repo.created)
	}
}

func TestCreateDrawing_ImageMediaLookupErrorFailsClosed(t *testing.T) {
	repo := &imageRepo{}
	media := &fakeMedia{err: errors.New("db down")}
	_, err := newImageService(repo, media).CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-1", DrawingType: "image", Points: json.RawMessage(twoCorners),
		ImageID: strp("mine"), CallerRole: permissions.RoleOwner,
	})
	if err == nil || repo.created != nil {
		t.Fatalf("want a refusal with nothing stored, got err=%v", err)
	}
}

// The drawing gate still applies to pictures: a player cannot place one.
func TestCreateDrawing_ImageStillNeedsDrawAccess(t *testing.T) {
	repo := &imageRepo{}
	media := &fakeMedia{files: map[string]string{"mine": "camp-1"}}
	_, err := newImageService(repo, media).CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-1", DrawingType: "image", Points: json.RawMessage(twoCorners),
		ImageID: strp("mine"), CallerRole: permissions.RolePlayer,
	})
	if err == nil || repo.created != nil {
		t.Fatalf("a player placed a picture: err=%v", err)
	}
}

func storedPicture() *Drawing {
	return &Drawing{
		ID: "pic", MapID: "map-1", DrawingType: "image", Points: json.RawMessage(twoCorners),
		StrokeColor: "#000000", StrokeWidth: 2, FillAlpha: 0.8, Rotation: 10, Visibility: "everyone",
		ImageID: strp("mine"), Crop: json.RawMessage(`{"t":5,"r":5,"b":5,"l":5}`), SortOrder: 3,
		CreatedBy: strp(picAuthor),
	}
}

// picAuthor placed storedPicture; the scribe edits in these tests are theirs.
const picAuthor = "u-pic"

func TestUpdateDrawing_Image(t *testing.T) {
	cases := []struct {
		name    string
		in      UpdateDrawingInput
		wantErr bool
		check   func(t *testing.T, d *Drawing)
	}{
		{
			name: "opacity only preserves everything else",
			in:   UpdateDrawingInput{FillAlpha: patch.Of(0.5)},
			check: func(t *testing.T, d *Drawing) {
				if d.FillAlpha != 0.5 || *d.ImageID != "mine" || d.SortOrder != 3 || d.Rotation != 10 ||
					string(d.Crop) != `{"t":5,"r":5,"b":5,"l":5}` || d.Visibility != "everyone" {
					t.Errorf("unexpected row %+v", d)
				}
			},
		},
		{
			name: "opacity is clamped",
			in:   UpdateDrawingInput{FillAlpha: patch.Of(0.01)},
			check: func(t *testing.T, d *Drawing) {
				if d.FillAlpha != 0.2 {
					t.Errorf("fill_alpha = %v, want 0.2", d.FillAlpha)
				}
			},
		},
		{
			name: "explicit null clears the crop",
			in:   UpdateDrawingInput{Crop: patch.Null[json.RawMessage]()},
			check: func(t *testing.T, d *Drawing) {
				if len(d.Crop) != 0 {
					t.Errorf("crop = %s, want cleared", d.Crop)
				}
			},
		},
		{
			name: "crop replaced",
			in:   UpdateDrawingInput{Crop: patch.Of(json.RawMessage(`{"t":0,"r":10,"b":0,"l":0}`))},
			check: func(t *testing.T, d *Drawing) {
				if string(d.Crop) != `{"t":0,"r":10,"b":0,"l":0}` {
					t.Errorf("crop = %s", d.Crop)
				}
			},
		},
		{name: "crop over 45 refused", in: UpdateDrawingInput{Crop: patch.Of(json.RawMessage(`{"t":50,"r":0,"b":0,"l":0}`))}, wantErr: true},
		{
			name: "image changed to another campaign file",
			in:   UpdateDrawingInput{ImageID: patch.Of("other-mine")},
			check: func(t *testing.T, d *Drawing) {
				if *d.ImageID != "other-mine" {
					t.Errorf("image = %s", *d.ImageID)
				}
			},
		},
		{name: "image changed to a foreign file refused", in: UpdateDrawingInput{ImageID: patch.Of("theirs")}, wantErr: true},
		{name: "image cleared refused", in: UpdateDrawingInput{ImageID: patch.Null[string]()}, wantErr: true},
		{
			name: "stacking position replaced",
			in:   UpdateDrawingInput{SortOrder: patch.Of(9)},
			check: func(t *testing.T, d *Drawing) {
				if d.SortOrder != 9 {
					t.Errorf("sort_order = %d", d.SortOrder)
				}
			},
		},
		{
			name: "hidden from players",
			in:   UpdateDrawingInput{Visibility: patch.Of("dm_only")},
			check: func(t *testing.T, d *Drawing) {
				if d.Visibility != "dm_only" {
					t.Errorf("visibility = %s", d.Visibility)
				}
			},
		},
		{name: "corners reduced to one point refused", in: UpdateDrawingInput{Points: patch.Of(json.RawMessage(`[{"x":1,"y":1}]`))}, wantErr: true},
		{name: "turned past 45 refused", in: UpdateDrawingInput{Rotation: patch.Of(46.0)}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &imageRepo{stored: storedPicture()}
			media := &fakeMedia{files: map[string]string{"mine": "camp-1", "other-mine": "camp-1", "theirs": "camp-2"}}
			err := newImageService(repo, media).UpdateDrawing(context.Background(), "pic", "map-1", picAuthor, permissions.RoleScribe, false, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				if repo.updated != nil {
					t.Error("a refused edit reached persistence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, repo.updated)
		})
	}
}

// Re-sending the stored image id must not need the media lookup, so a picture
// whose file was removed can still be moved or resized.
func TestUpdateDrawing_ImageSameIDSkipsLookup(t *testing.T) {
	repo := &imageRepo{stored: storedPicture()}
	media := &fakeMedia{files: map[string]string{}}
	err := newImageService(repo, media).UpdateDrawing(context.Background(), "pic", "map-1", picAuthor, permissions.RoleScribe, false,
		UpdateDrawingInput{ImageID: patch.Of("mine"), Rotation: patch.Of(5.0)})
	if err != nil || media.calls != 0 {
		t.Fatalf("err=%v lookups=%d, want none", err, media.calls)
	}
}

func TestUpdateDrawing_NonImageRejectsImageFields(t *testing.T) {
	cases := []struct {
		name string
		in   UpdateDrawingInput
	}{
		{"image id", UpdateDrawingInput{ImageID: patch.Of("mine")}},
		{"crop", UpdateDrawingInput{Crop: patch.Of(json.RawMessage(`{"t":1,"r":1,"b":1,"l":1}`))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &imageRepo{stored: &Drawing{ID: "r", MapID: "map-1", DrawingType: "rectangle", Points: json.RawMessage(twoCorners), Visibility: "everyone"}}
			media := &fakeMedia{files: map[string]string{"mine": "camp-1"}}
			err := newImageService(repo, media).UpdateDrawing(context.Background(), "r", "map-1", "", permissions.RoleScribe, false, tc.in)
			if err == nil || repo.updated != nil {
				t.Fatalf("err=%v updated=%v, want a refusal", err, repo.updated)
			}
		})
	}
}

// A picture entirely under a shadow is withheld from players like any other
// drawing; one that only touches the shadow stays visible.
func TestImageDrawing_ShadowHiding(t *testing.T) {
	areas := []ShadowArea{{MinX: 0, MinY: 0, MaxX: 50, MaxY: 50}}
	under := Drawing{ID: "under", DrawingType: "image", Points: pts(`[{"x":5,"y":5},{"x":40,"y":40}]`)}
	touching := Drawing{ID: "touching", DrawingType: "image", Points: pts(`[{"x":5,"y":5},{"x":80,"y":80}]`)}
	got := filterDrawingsByShadow(areas, []Drawing{under, touching})
	if len(got) != 1 || got[0].ID != "touching" {
		t.Fatalf("player keeps %v, want only the picture that is not wholly under the shadow", got)
	}
}

// The picture fields reach the wire under the snake_case names the viewer reads.
func TestDrawingJSON_PictureFields(t *testing.T) {
	b, err := json.Marshal(storedPicture())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"image_id", "crop", "sort_order", "fill_alpha", "rotation"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q in %s", k, b)
		}
	}
	plain, _ := json.Marshal(Drawing{ID: "x", DrawingType: "rectangle", Points: json.RawMessage(`[]`)})
	var pm map[string]any
	_ = json.Unmarshal(plain, &pm)
	if _, ok := pm["crop"]; ok {
		t.Error("a non-picture must not carry a crop")
	}
}

// The add-a-picture tool, like the other drawing tools, is only in the markup
// for people who may draw; the pictures module loads for everyone because
// players need it to see pictures.
func TestMapEditorBody_PictureToolOnlyForDrawers(t *testing.T) {
	scribe := renderMapEditor(t, true)
	for _, want := range []string{`id="mp-add-picture"`, `data-widget="media-picker"`, `data-mime-prefix="image/"`, "Add a picture"} {
		if !strings.Contains(scribe, want) {
			t.Errorf("scribe markup missing %q", want)
		}
	}
	player := renderMapEditor(t, false)
	if strings.Contains(player, `id="mp-add-picture"`) {
		t.Error("players must not get the add-a-picture tool")
	}
	for _, isScribe := range []bool{true, false} {
		if !strings.Contains(renderMapEditor(t, isScribe), "map_pictures.js") {
			t.Errorf("pictures script missing for scribe=%v", isScribe)
		}
	}
}

// Only the four crop sides are stored; any other key a caller sends is dropped.
func TestCrop_ReserialisedWithoutUnknownKeys(t *testing.T) {
	media := &fakeMedia{files: map[string]string{"mine": "camp-1"}}
	repo := &imageRepo{}
	d, err := newImageService(repo, media).CreateDrawing(context.Background(), CreateDrawingInput{
		MapID: "map-1", DrawingType: "image", Points: json.RawMessage(twoCorners), ImageID: strp("mine"),
		Crop: json.RawMessage(`{"t":5,"r":6,"b":7,"l":8,"evil":"<script>","x":1}`), CallerRole: permissions.RoleScribe,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(d.Crop, &got)
	if len(got) != 4 || got["t"] != 5.0 || got["l"] != 8.0 {
		t.Errorf("stored crop = %s, want only t,r,b,l", d.Crop)
	}

	up := &imageRepo{stored: storedPicture()}
	err = newImageService(up, media).UpdateDrawing(context.Background(), "pic", "map-1", picAuthor, permissions.RoleScribe, false,
		UpdateDrawingInput{Crop: patch.Of(json.RawMessage(`{"t":1,"junk":true}`))})
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	_ = json.Unmarshal(up.updated.Crop, &got)
	if len(got) != 4 || got["t"] != 1.0 || got["r"] != 0.0 {
		t.Errorf("updated crop = %s, want only t,r,b,l", up.updated.Crop)
	}
}

// pictureListRepo applies the same dm_only rule as the SQL list filter.
type pictureListRepo struct {
	idorRepo
	all []Drawing
}

func (r *pictureListRepo) ListShadows(context.Context, string) ([]Drawing, error) { return nil, nil }
func (r *pictureListRepo) ListDrawings(_ context.Context, _ string, role int, _ string) ([]Drawing, error) {
	var out []Drawing
	for _, d := range r.all {
		if d.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// Every picture the server sends carries a signed URL minted for the viewer
// asking, and a picture hidden from that viewer is not sent at all, so it has
// none. An anonymous visitor of a public campaign is a viewer too.
func TestListDrawings_PicturesCarrySignedURLForTheViewer(t *testing.T) {
	visible := Drawing{ID: "shown", MapID: "map-1", DrawingType: "image", Visibility: "everyone", Points: pts(twoCorners), ImageID: strp("m-shown")}
	hidden := Drawing{ID: "hid", MapID: "map-1", DrawingType: "image", Visibility: "dm_only", Points: pts(twoCorners), ImageID: strp("m-hidden")}
	rect := Drawing{ID: "r", MapID: "map-1", DrawingType: "rectangle", Visibility: "everyone", Points: pts(twoCorners)}
	h := NewDrawingHandler(guardMapSvc{}, NewDrawingService(&pictureListRepo{all: []Drawing{visible, hidden, rect}}))

	cases := []struct {
		name     string
		viewer   string
		role     campaigns.Role
		wantIDs  []string
		wantHide bool
	}{
		{"player", "user-p", campaigns.RolePlayer, []string{"shown", "r"}, false},
		{"anonymous public visitor", "anon", campaigns.RoleNone, []string{"shown", "r"}, false},
		{"owner sees the hidden one too", "user-o", campaigns.RoleOwner, []string{"shown", "hid", "r"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			viewer := tc.viewer
			req = req.WithContext(layouts.SetMediaURLFunc(req.Context(), func(id string) string {
				return "/media/" + id + "?sig=for-" + viewer
			}))
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "mid")
			c.SetParamValues("camp-1", "map-1")
			c.Set("campaign_context", dmWriteCampaignCtx(tc.role, false))
			if err := h.ListDrawings(c); err != nil {
				t.Fatal(err)
			}
			var got []map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, d := range got {
				id := d["id"].(string)
				ids = append(ids, id)
				url, has := d["image_url"].(string)
				switch id {
				case "shown":
					if want := "/media/m-shown?sig=for-" + tc.viewer; !has || url != want {
						t.Errorf("shown picture url = %q, want %q", url, want)
					}
				case "hid":
					if !tc.wantHide {
						t.Error("a hidden picture reached a viewer who may not see it")
					}
				case "r":
					if has {
						t.Error("a non-picture must not carry an image_url")
					}
				}
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("ids = %v, want %v", ids, tc.wantIDs)
			}
		})
	}
}
