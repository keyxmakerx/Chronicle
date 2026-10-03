package maps

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// --- Mock Repository ---

type mockMapRepo struct {
	createMapFn    func(ctx context.Context, m *Map) error
	getMapFn       func(ctx context.Context, id string) (*Map, error)
	updateMapFn    func(ctx context.Context, m *Map) error
	deleteMapFn    func(ctx context.Context, id string) error
	listMapsFn     func(ctx context.Context, campaignID string) ([]Map, error)
	searchMapsFn   func(ctx context.Context, campaignID, query string) ([]Map, error)
	createMarkerFn func(ctx context.Context, mk *Marker) error
	getMarkerFn    func(ctx context.Context, id string) (*Marker, error)
	updateMarkerFn func(ctx context.Context, mk *Marker) error
	deleteMarkerFn func(ctx context.Context, id string) error
	listMarkersFn  func(ctx context.Context, mapID string, role int) ([]Marker, error)

	campaignFrame    string
	getFrameErr      error
	setCampaignFrame func(ctx context.Context, campaignID, frame string) error
}

func (m *mockMapRepo) GetCampaignFrame(context.Context, string) (string, error) {
	return m.campaignFrame, m.getFrameErr
}

func (m *mockMapRepo) SetCampaignFrame(ctx context.Context, campaignID, frame string) error {
	if m.setCampaignFrame != nil {
		return m.setCampaignFrame(ctx, campaignID, frame)
	}
	return nil
}

func (m *mockMapRepo) CreateMap(ctx context.Context, mp *Map) error {
	if m.createMapFn != nil {
		return m.createMapFn(ctx, mp)
	}
	return nil
}

func (m *mockMapRepo) GetMap(ctx context.Context, id string) (*Map, error) {
	if m.getMapFn != nil {
		return m.getMapFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMapRepo) UpdateMap(ctx context.Context, mp *Map) error {
	if m.updateMapFn != nil {
		return m.updateMapFn(ctx, mp)
	}
	return nil
}

func (m *mockMapRepo) DeleteMap(ctx context.Context, id string) error {
	if m.deleteMapFn != nil {
		return m.deleteMapFn(ctx, id)
	}
	return nil
}

func (m *mockMapRepo) ListMaps(ctx context.Context, campaignID string) ([]Map, error) {
	if m.listMapsFn != nil {
		return m.listMapsFn(ctx, campaignID)
	}
	return nil, nil
}

func (m *mockMapRepo) SearchMaps(ctx context.Context, campaignID, query string) ([]Map, error) {
	if m.searchMapsFn != nil {
		return m.searchMapsFn(ctx, campaignID, query)
	}
	return nil, nil
}

func (m *mockMapRepo) CreateMarker(ctx context.Context, mk *Marker) error {
	if m.createMarkerFn != nil {
		return m.createMarkerFn(ctx, mk)
	}
	return nil
}

func (m *mockMapRepo) GetMarker(ctx context.Context, id string) (*Marker, error) {
	if m.getMarkerFn != nil {
		return m.getMarkerFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMapRepo) UpdateMarker(ctx context.Context, mk *Marker) error {
	if m.updateMarkerFn != nil {
		return m.updateMarkerFn(ctx, mk)
	}
	return nil
}

func (m *mockMapRepo) DeleteMarker(ctx context.Context, id string) error {
	if m.deleteMarkerFn != nil {
		return m.deleteMarkerFn(ctx, id)
	}
	return nil
}

func (m *mockMapRepo) ListMarkers(ctx context.Context, mapID string, role int, userID string) ([]Marker, error) {
	if m.listMarkersFn != nil {
		return m.listMarkersFn(ctx, mapID, role)
	}
	return nil, nil
}

// --- Test Helpers ---

func newTestMapService(repo *mockMapRepo) MapService {
	return NewMapService(repo)
}

func assertAppError(t *testing.T, err error, expectedCode int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != expectedCode {
		t.Errorf("expected status code %d, got %d (message: %s)", expectedCode, appErr.Code, appErr.Message)
	}
}

// --- CreateMap Tests ---

func TestCreateMap_Success(t *testing.T) {
	repo := &mockMapRepo{}
	svc := newTestMapService(repo)

	m, err := svc.CreateMap(context.Background(), CreateMapInput{
		CampaignID: "camp-1",
		Name:       "World Map",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected map, got nil")
	}
	if m.Name != "World Map" {
		t.Errorf("expected name 'World Map', got %q", m.Name)
	}
	if m.CampaignID != "camp-1" {
		t.Errorf("expected campaign_id 'camp-1', got %q", m.CampaignID)
	}
	if m.ID == "" {
		t.Error("expected a generated UUID, got empty string")
	}
}

func TestCreateMap_EmptyName(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	_, err := svc.CreateMap(context.Background(), CreateMapInput{
		CampaignID: "camp-1",
		Name:       "",
	})
	assertAppError(t, err, 422)
}

func TestCreateMap_EmptyCampaignID(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	_, err := svc.CreateMap(context.Background(), CreateMapInput{
		CampaignID: "",
		Name:       "World Map",
	})
	assertAppError(t, err, 422)
}

func TestCreateMap_WithDescription(t *testing.T) {
	repo := &mockMapRepo{}
	svc := newTestMapService(repo)

	desc := "A detailed world map"
	m, err := svc.CreateMap(context.Background(), CreateMapInput{
		CampaignID:  "camp-1",
		Name:        "World Map",
		Description: &desc,
		ImageWidth:  1920,
		ImageHeight: 1080,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Description == nil || *m.Description != desc {
		t.Errorf("expected description %q, got %v", desc, m.Description)
	}
	if m.ImageWidth != 1920 {
		t.Errorf("expected width 1920, got %d", m.ImageWidth)
	}
	if m.ImageHeight != 1080 {
		t.Errorf("expected height 1080, got %d", m.ImageHeight)
	}
}

func TestCreateMap_RepoError(t *testing.T) {
	repo := &mockMapRepo{
		createMapFn: func(_ context.Context, _ *Map) error {
			return errors.New("db error")
		},
	}
	svc := newTestMapService(repo)
	_, err := svc.CreateMap(context.Background(), CreateMapInput{
		CampaignID: "camp-1",
		Name:       "World Map",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetMap Tests ---

func TestGetMap_Success(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, id string) (*Map, error) {
			return &Map{ID: id, Name: "World Map", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestMapService(repo)

	m, err := svc.GetMap(context.Background(), "map-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "map-1" {
		t.Errorf("expected ID 'map-1', got %q", m.ID)
	}
}

func TestGetMap_NotFound(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return nil, nil
		},
	}
	svc := newTestMapService(repo)
	_, err := svc.GetMap(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

func TestGetMap_RepoError(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestMapService(repo)
	_, err := svc.GetMap(context.Background(), "map-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- UpdateMap Tests ---

func TestUpdateMap_Success(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return &Map{ID: "map-1", Name: "Old Name", CampaignID: "camp-1"}, nil
		},
		updateMapFn: func(_ context.Context, m *Map) error {
			if m.Name != "New Name" {
				t.Errorf("expected updated name 'New Name', got %q", m.Name)
			}
			return nil
		},
	}
	svc := newTestMapService(repo)

	err := svc.UpdateMap(context.Background(), "map-1", UpdateMapInput{
		Name: "New Name",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMap_NotFound(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return nil, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMap(context.Background(), "nonexistent", UpdateMapInput{Name: "New"})
	assertAppError(t, err, 404)
}

func TestUpdateMap_EmptyName(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return &Map{ID: "map-1", Name: "Old", CampaignID: "camp-1"}, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMap(context.Background(), "map-1", UpdateMapInput{Name: ""})
	assertAppError(t, err, 422)
}

// --- DeleteMap Tests ---

func TestDeleteMap_Success(t *testing.T) {
	deleted := false
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return &Map{ID: "map-1", CampaignID: "camp-1"}, nil
		},
		deleteMapFn: func(_ context.Context, id string) error {
			deleted = true
			if id != "map-1" {
				t.Errorf("expected delete ID 'map-1', got %q", id)
			}
			return nil
		},
	}
	svc := newTestMapService(repo)

	err := svc.DeleteMap(context.Background(), "map-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected DeleteMap to be called on repo")
	}
}

func TestDeleteMap_NotFound(t *testing.T) {
	repo := &mockMapRepo{
		getMapFn: func(_ context.Context, _ string) (*Map, error) {
			return nil, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.DeleteMap(context.Background(), "nonexistent", nil)
	assertAppError(t, err, 404)
}

// --- ListMaps Tests ---

func TestListMaps_Success(t *testing.T) {
	repo := &mockMapRepo{
		listMapsFn: func(_ context.Context, _ string) ([]Map, error) {
			return []Map{
				{ID: "map-1", Name: "World"},
				{ID: "map-2", Name: "Dungeon"},
			}, nil
		},
	}
	svc := newTestMapService(repo)

	maps, err := svc.ListMaps(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maps) != 2 {
		t.Errorf("expected 2 maps, got %d", len(maps))
	}
}

func TestListMaps_Empty(t *testing.T) {
	repo := &mockMapRepo{
		listMapsFn: func(_ context.Context, _ string) ([]Map, error) {
			return []Map{}, nil
		},
	}
	svc := newTestMapService(repo)

	maps, err := svc.ListMaps(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maps) != 0 {
		t.Errorf("expected 0 maps, got %d", len(maps))
	}
}

// --- CreateMarker Tests ---

func TestCreateMarker_Success(t *testing.T) {
	repo := &mockMapRepo{}
	svc := newTestMapService(repo)

	mk, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Town Square",
		X:         50.5,
		Y:         25.0,
		Icon:      "fa-map-pin",
		Color:     "#3b82f6",
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk == nil {
		t.Fatal("expected marker, got nil")
	}
	if mk.Name != "Town Square" {
		t.Errorf("expected name 'Town Square', got %q", mk.Name)
	}
	if mk.MapID != "map-1" {
		t.Errorf("expected map_id 'map-1', got %q", mk.MapID)
	}
	if mk.X != 50.5 {
		t.Errorf("expected X 50.5, got %f", mk.X)
	}
	if mk.Y != 25.0 {
		t.Errorf("expected Y 25.0, got %f", mk.Y)
	}
}

func TestCreateMarker_EmptyName(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	_, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID: "map-1",
		Name:  "",
		X:     50,
		Y:     50,
	})
	assertAppError(t, err, 422)
}

func TestCreateMarker_EmptyMapID(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	_, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID: "",
		Name:  "Town",
		X:     50,
		Y:     50,
	})
	assertAppError(t, err, 422)
}

func TestCreateMarker_CoordinatesOutOfRange(t *testing.T) {
	tests := []struct {
		name string
		x, y float64
	}{
		{"X negative", -1, 50},
		{"X over 100", 101, 50},
		{"Y negative", 50, -1},
		{"Y over 100", 50, 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestMapService(&mockMapRepo{})
			_, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
				MapID: "map-1",
				Name:  "Marker",
				X:     tt.x,
				Y:     tt.y,
			})
			assertAppError(t, err, 422)
		})
	}
}

func TestCreateMarker_BoundaryCoordinates(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})

	// 0 and 100 are valid boundary values.
	mk, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Corner",
		X:         0,
		Y:         100,
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error at boundary coordinates: %v", err)
	}
	if mk.X != 0 || mk.Y != 100 {
		t.Errorf("expected (0, 100), got (%f, %f)", mk.X, mk.Y)
	}
}

func TestCreateMarker_DefaultsApplied(t *testing.T) {
	repo := &mockMapRepo{}
	svc := newTestMapService(repo)

	mk, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Default Marker",
		X:         50,
		Y:         50,
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk.Icon != "fa-map-pin" {
		t.Errorf("expected default icon 'fa-map-pin', got %q", mk.Icon)
	}
	if mk.Color != "#3b82f6" {
		t.Errorf("expected default color '#3b82f6', got %q", mk.Color)
	}
	if mk.Visibility != "everyone" {
		t.Errorf("expected default visibility 'everyone', got %q", mk.Visibility)
	}
}

func TestCreateMarker_InvalidIcon(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	_, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Bad Icon",
		X:         50,
		Y:         50,
		Icon:      "<script>alert(1)</script>",
		Color:     "#ff0000",
		CreatedBy: "user-1",
	})
	assertAppError(t, err, 400)
}

func TestCreateMarker_InvalidColor(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	_, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Bad Color",
		X:         50,
		Y:         50,
		Icon:      "fa-map-pin",
		Color:     "red; background: url(evil)",
		CreatedBy: "user-1",
	})
	assertAppError(t, err, 422)
}

func TestCreateMarker_ValidShortHexColor(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	mk, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Short Hex",
		X:         50,
		Y:         50,
		Icon:      "fa-map-pin",
		Color:     "#f00",
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk.Color != "#f00" {
		t.Errorf("expected color '#f00', got %q", mk.Color)
	}
}

func TestCreateMarker_DMOnlyVisibility(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	mk, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:      "map-1",
		Name:       "Secret",
		X:          10,
		Y:          20,
		Visibility: "dm_only",
		CreatedBy:  "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk.Visibility != "dm_only" {
		t.Errorf("expected visibility 'dm_only', got %q", mk.Visibility)
	}
	if !mk.IsDMOnly() {
		t.Error("expected IsDMOnly() to return true")
	}
}

func TestCreateMarker_WithEntityLink(t *testing.T) {
	svc := newTestMapService(&mockMapRepo{})
	entityID := "entity-123"
	mk, err := svc.CreateMarker(context.Background(), CreateMarkerInput{
		MapID:     "map-1",
		Name:      "Linked Marker",
		X:         50,
		Y:         50,
		EntityID:  &entityID,
		CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk.EntityID == nil || *mk.EntityID != entityID {
		t.Errorf("expected entity_id %q, got %v", entityID, mk.EntityID)
	}
}

// --- GetMarker Tests ---

func TestGetMarker_Success(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, id string) (*Marker, error) {
			return &Marker{ID: id, Name: "Pin", MapID: "map-1"}, nil
		},
	}
	svc := newTestMapService(repo)

	mk, err := svc.GetMarker(context.Background(), "mk-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mk.ID != "mk-1" {
		t.Errorf("expected ID 'mk-1', got %q", mk.ID)
	}
}

func TestGetMarker_NotFound(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return nil, nil
		},
	}
	svc := newTestMapService(repo)
	_, err := svc.GetMarker(context.Background(), "nonexistent")
	assertAppError(t, err, 404)
}

// --- UpdateMarker Tests ---

func TestUpdateMarker_Success(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", Name: "Old", MapID: "map-1", X: 10, Y: 20}, nil
		},
		updateMarkerFn: func(_ context.Context, mk *Marker) error {
			if mk.Name != "New Name" {
				t.Errorf("expected name 'New Name', got %q", mk.Name)
			}
			if mk.X != 75 {
				t.Errorf("expected X 75, got %f", mk.X)
			}
			return nil
		},
	}
	svc := newTestMapService(repo)

	err := svc.UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{
		Name:  patch.Of("New Name"),
		X:     patch.Of(75.0),
		Y:     patch.Of(30.0),
		Icon:  patch.Of("fa-castle"),
		Color: patch.Of("#ff0000"),
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMarker_NotFound(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return nil, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMarker(context.Background(), "nonexistent", UpdateMarkerInput{Name: patch.Of("X"), X: patch.Of(50.0), Y: patch.Of(50.0)}, true)
	assertAppError(t, err, 404)
}

func TestUpdateMarker_EmptyName(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", Name: "Old", MapID: "map-1"}, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{Name: patch.Of(""), X: patch.Of(50.0), Y: patch.Of(50.0)}, true)
	assertAppError(t, err, 422)
}

func TestUpdateMarker_InvalidCoordinates(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", Name: "Pin", MapID: "map-1"}, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{
		Name: patch.Of("Pin"),
		X:    patch.Of(150.0),
		Y:    patch.Of(50.0),
	}, true)
	assertAppError(t, err, 422)
}

func TestUpdateMarker_InvalidIcon(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", Name: "Pin", MapID: "map-1"}, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{
		Name: patch.Of("Pin"),
		X:    patch.Of(50.0),
		Y:    patch.Of(50.0),
		Icon: patch.Of("javascript:alert(1)"),
	}, true)
	assertAppError(t, err, 400)
}

func TestUpdateMarker_InvalidColor(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", Name: "Pin", MapID: "map-1"}, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.UpdateMarker(context.Background(), "mk-1", UpdateMarkerInput{
		Name:  patch.Of("Pin"),
		X:     patch.Of(50.0),
		Y:     patch.Of(50.0),
		Icon:  patch.Of("fa-map-pin"),
		Color: patch.Of("not-a-color"),
	}, true)
	assertAppError(t, err, 422)
}

// --- DeleteMarker Tests ---

func TestDeleteMarker_Success(t *testing.T) {
	deleted := false
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return &Marker{ID: "mk-1", MapID: "map-1"}, nil
		},
		deleteMarkerFn: func(_ context.Context, id string) error {
			deleted = true
			if id != "mk-1" {
				t.Errorf("expected delete ID 'mk-1', got %q", id)
			}
			return nil
		},
	}
	svc := newTestMapService(repo)

	err := svc.DeleteMarker(context.Background(), "mk-1", nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected DeleteMarker to be called on repo")
	}
}

func TestDeleteMarker_NotFound(t *testing.T) {
	repo := &mockMapRepo{
		getMarkerFn: func(_ context.Context, _ string) (*Marker, error) {
			return nil, nil
		},
	}
	svc := newTestMapService(repo)
	err := svc.DeleteMarker(context.Background(), "nonexistent", nil, true)
	assertAppError(t, err, 404)
}

// --- ListMarkers Tests ---

func TestListMarkers_Success(t *testing.T) {
	repo := &mockMapRepo{
		listMarkersFn: func(_ context.Context, _ string, _ int) ([]Marker, error) {
			return []Marker{
				{ID: "mk-1", Name: "Pin 1"},
				{ID: "mk-2", Name: "Pin 2"},
			}, nil
		},
	}
	svc := newTestMapService(repo)

	markers, err := svc.ListMarkers(context.Background(), "camp-1", "map-1", 3, "owner-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(markers) != 2 {
		t.Errorf("expected 2 markers, got %d", len(markers))
	}
}

// fakeEntityVisibilityGate is a stub EntityVisibilityGate for ListMarkers
// tests: it reports exactly the entity IDs in `viewable` as visible and
// records the arguments it was called with.
type fakeEntityVisibilityGate struct {
	viewable map[string]bool
	err      error
	called   bool
	gotIDs   []string
}

func (g *fakeEntityVisibilityGate) FilterViewableEntityIDs(_ context.Context, _ string, entityIDs []string, _ int, _ string) (map[string]bool, error) {
	g.called = true
	g.gotIDs = entityIDs
	if g.err != nil {
		return nil, g.err
	}
	return g.viewable, nil
}

// TestListMarkers_BlanksEntityNameForUnviewableEntity pins ADR-055 rule 3
// for maps: an 'everyone' marker linked to a dm_only/private entity must not
// leak that entity's name/icon to a viewer who can't otherwise see it. The
// marker's own visibility is already repo-filtered; this is the separate
// linked-entity check.
func TestListMarkers_BlanksEntityNameForUnviewableEntity(t *testing.T) {
	repo := &mockMapRepo{
		listMarkersFn: func(_ context.Context, _ string, _ int) ([]Marker, error) {
			return []Marker{
				{ID: "mk-1", Name: "Pin 1", EntityID: strPtrMK("ent-secret"), EntityName: "Secret NPC", EntityIcon: "fa-user"},
			}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{}} // entity not viewable
	svc := newTestMapService(repo)
	svc.(*mapService).entityGate = gate

	markers, err := svc.ListMarkers(context.Background(), "camp-1", "map-1", 1 /* RolePlayer */, "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gate.called {
		t.Fatal("expected the entity visibility gate to be consulted")
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	if markers[0].EntityName != "" || markers[0].EntityIcon != "" {
		t.Errorf("expected EntityName/EntityIcon blanked for an unviewable entity, got name=%q icon=%q",
			markers[0].EntityName, markers[0].EntityIcon)
	}
	// The ID must be blanked too, not just the display fields: every
	// consumer of this struct (page HTML, both web JSON APIs, the sync API)
	// serializes EntityID verbatim, so a bare ID still tells the viewer a
	// specific hidden entity exists (ADR-055 rule 3).
	if markers[0].EntityID != nil {
		t.Errorf("expected EntityID blanked for an unviewable entity, got %q", *markers[0].EntityID)
	}
}

// TestListMarkers_KeepsEntityNameForViewableEntity is the positive
// counterpart: a linked entity the viewer IS permitted to see keeps its
// name/icon.
func TestListMarkers_KeepsEntityNameForViewableEntity(t *testing.T) {
	repo := &mockMapRepo{
		listMarkersFn: func(_ context.Context, _ string, _ int) ([]Marker, error) {
			return []Marker{
				{ID: "mk-1", Name: "Pin 1", EntityID: strPtrMK("ent-public"), EntityName: "Town Hall", EntityIcon: "fa-building"},
			}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{"ent-public": true}}
	svc := newTestMapService(repo)
	svc.(*mapService).entityGate = gate

	markers, err := svc.ListMarkers(context.Background(), "camp-1", "map-1", 1, "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if markers[0].EntityName != "Town Hall" || markers[0].EntityIcon != "fa-building" {
		t.Errorf("expected viewable entity's name/icon preserved, got name=%q icon=%q",
			markers[0].EntityName, markers[0].EntityIcon)
	}
}

// TestListMarkers_OwnerBypassesEntityGate pins that owners (who already see
// dm_only markers unfiltered) also see every linked entity's name, matching
// entities' own visibilityFilter bypass for the same role.
func TestListMarkers_OwnerBypassesEntityGate(t *testing.T) {
	repo := &mockMapRepo{
		listMarkersFn: func(_ context.Context, _ string, _ int) ([]Marker, error) {
			return []Marker{
				{ID: "mk-1", Name: "Pin 1", EntityID: strPtrMK("ent-secret"), EntityName: "Secret NPC", EntityIcon: "fa-user"},
			}, nil
		},
	}
	gate := &fakeEntityVisibilityGate{viewable: map[string]bool{}}
	svc := newTestMapService(repo)
	svc.(*mapService).entityGate = gate

	markers, err := svc.ListMarkers(context.Background(), "camp-1", "map-1", 3 /* RoleOwner */, "owner-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gate.called {
		t.Error("expected the entity visibility gate NOT to be consulted for an owner")
	}
	if markers[0].EntityName != "Secret NPC" {
		t.Errorf("expected owner to see the entity name unfiltered, got %q", markers[0].EntityName)
	}
}

// TestListMarkers_FailsClosedWhenGateUnwired pins the fail-closed default:
// an unconfigured (nil) EntityVisibilityGate must never be treated as
// "everything viewable" — that would silently reopen the leak this service
// exists to close if wiring is ever dropped.
func TestListMarkers_FailsClosedWhenGateUnwired(t *testing.T) {
	repo := &mockMapRepo{
		listMarkersFn: func(_ context.Context, _ string, _ int) ([]Marker, error) {
			return []Marker{
				{ID: "mk-1", Name: "Pin 1", EntityID: strPtrMK("ent-x"), EntityName: "Some NPC", EntityIcon: "fa-user"},
			}, nil
		},
	}
	svc := newTestMapService(repo) // no SetEntityVisibilityGate call

	markers, err := svc.ListMarkers(context.Background(), "camp-1", "map-1", 1, "player-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if markers[0].EntityName != "" || markers[0].EntityIcon != "" {
		t.Errorf("expected fail-closed blanking with no gate configured, got name=%q icon=%q",
			markers[0].EntityName, markers[0].EntityIcon)
	}
}

// --- SearchMaps Tests ---

func TestSearchMaps_Success(t *testing.T) {
	repo := &mockMapRepo{
		searchMapsFn: func(_ context.Context, _ string, _ string) ([]Map, error) {
			return []Map{
				{ID: "map-1", Name: "World Map"},
			}, nil
		},
	}
	svc := newTestMapService(repo)

	results, err := svc.SearchMaps(context.Background(), "camp-1", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["name"] != "World Map" {
		t.Errorf("expected name 'World Map', got %q", results[0]["name"])
	}
	if results[0]["type_name"] != "Map" {
		t.Errorf("expected type_name 'Map', got %q", results[0]["type_name"])
	}
	if results[0]["type_icon"] != "fa-map" {
		t.Errorf("expected type_icon 'fa-map', got %q", results[0]["type_icon"])
	}
}

func TestSearchMaps_Empty(t *testing.T) {
	repo := &mockMapRepo{
		searchMapsFn: func(_ context.Context, _ string, _ string) ([]Map, error) {
			return []Map{}, nil
		},
	}
	svc := newTestMapService(repo)

	results, err := svc.SearchMaps(context.Background(), "camp-1", "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- Model Method Tests ---

func TestMap_HasImage(t *testing.T) {
	t.Run("nil image", func(t *testing.T) {
		m := &Map{}
		if m.HasImage() {
			t.Error("expected HasImage() false for nil ImageID")
		}
	})

	t.Run("empty image", func(t *testing.T) {
		empty := ""
		m := &Map{ImageID: &empty}
		if m.HasImage() {
			t.Error("expected HasImage() false for empty ImageID")
		}
	})

	t.Run("with image", func(t *testing.T) {
		img := "img-123"
		m := &Map{ImageID: &img}
		if !m.HasImage() {
			t.Error("expected HasImage() true for set ImageID")
		}
	})
}

func TestMap_GetCampaignID(t *testing.T) {
	m := &Map{CampaignID: "camp-1"}
	if m.GetCampaignID() != "camp-1" {
		t.Errorf("expected 'camp-1', got %q", m.GetCampaignID())
	}
}

func TestMarker_IsDMOnly(t *testing.T) {
	t.Run("dm_only", func(t *testing.T) {
		mk := &Marker{Visibility: "dm_only"}
		if !mk.IsDMOnly() {
			t.Error("expected IsDMOnly() true")
		}
	})

	t.Run("everyone", func(t *testing.T) {
		mk := &Marker{Visibility: "everyone"}
		if mk.IsDMOnly() {
			t.Error("expected IsDMOnly() false")
		}
	})
}

// --- Icon and Color Pattern Tests ---

func TestIconPattern(t *testing.T) {
	valid := []string{"fa-map-pin", "fa-castle", "fa-dungeon", "fa-skull-crossbones"}
	invalid := []string{"<script>", "javascript:", "map-pin", "FA-MAP", "fa_map", ""}

	for _, icon := range valid {
		if !sanitize.IsIconName(icon) {
			t.Errorf("expected icon %q to be valid", icon)
		}
	}
	for _, icon := range invalid {
		if sanitize.IsIconName(icon) {
			t.Errorf("expected icon %q to be invalid", icon)
		}
	}
}

func TestColorPattern(t *testing.T) {
	valid := []string{"#fff", "#FFF", "#3b82f6", "#FF0000", "#abc", "#AABBCC"}
	invalid := []string{"red", "#gg0000", "#12345", "3b82f6", "#", "", "#1234567"}

	for _, color := range valid {
		if !colorPattern.MatchString(color) {
			t.Errorf("expected color %q to be valid", color)
		}
	}
	for _, color := range invalid {
		if colorPattern.MatchString(color) {
			t.Errorf("expected color %q to be invalid", color)
		}
	}
}
