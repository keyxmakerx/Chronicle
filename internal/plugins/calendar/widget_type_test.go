// widget_type_test.go — C-WIDGET-BINDING-P2. The calendar + worldstate widget
// types (both back onto a calendar instance) and the calendar delete-hook that
// sweeps BOTH slugs.
package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

// wtCalStub overrides only the methods the calendar-instance backing uses.
type wtCalStub struct {
	CalendarService
	byID       map[string]*Calendar  // instanceID -> calendar (carries CampaignID)
	defaults   map[string]*Calendar  // campaignID -> default calendar
	list       map[string][]Calendar // campaignID -> calendars (ListInstances)
	listErr    error                 // forces a ListCalendars failure
	created    []CreateCalendarInput // captures CreateInstance → CreateCalendar
	createErr  error                 // forces a CreateCalendar failure
	getByIDErr error                 // non-404 error to exercise the "don't sweep on a blip" path
}

func (s *wtCalStub) GetCalendarByID(_ context.Context, id string) (*Calendar, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if c, ok := s.byID[id]; ok {
		return c, nil
	}
	return nil, apperror.NewNotFound("calendar not found")
}
func (s *wtCalStub) GetCalendar(_ context.Context, campaignID string) (*Calendar, error) {
	if c, ok := s.defaults[campaignID]; ok {
		return c, nil
	}
	return nil, apperror.NewNotFound("no calendar")
}
func (s *wtCalStub) ListCalendars(_ context.Context, campaignID string) ([]Calendar, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.list[campaignID], nil
}
func (s *wtCalStub) CreateCalendar(_ context.Context, campaignID string, input CreateCalendarInput) (*Calendar, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.created = append(s.created, input)
	return &Calendar{ID: "new-cal", CampaignID: campaignID, Name: input.Name}, nil
}

func wtHost(camp string) widgetbindings.HostRef {
	return widgetbindings.HostRef{CampaignID: camp, Type: widgetbindings.HostTypeEntity, ID: "ent-1"}
}

func TestCalendarAndWorldstate_Slugs(t *testing.T) {
	svc := &wtCalStub{}
	if got := NewCalendarWidgetType(svc).Slug(); got != WidgetTypeCalendar {
		t.Errorf("calendar slug = %q", got)
	}
	if got := NewWorldStateWidgetType(svc).Slug(); got != WidgetTypeWorldstate {
		t.Errorf("worldstate slug = %q", got)
	}
}

// Both widget types share the calendar-instance behavior: in-campaign instance
// validates, cross-campaign does not (security), not-found is sweepable, and a
// transient error does NOT report not-found (so the resolver won't sweep).
func TestCalendarInstanceBacking_InstanceExists(t *testing.T) {
	svc := &wtCalStub{byID: map[string]*Calendar{
		"cal-1": {ID: "cal-1", CampaignID: "camp-1"},
		"cal-x": {ID: "cal-x", CampaignID: "camp-OTHER"},
	}}
	for _, wt := range []widgetbindings.WidgetType{NewCalendarWidgetType(svc), NewWorldStateWidgetType(svc)} {
		ok, err := wt.InstanceExists(context.Background(), "camp-1", "cal-1")
		if err != nil || !ok {
			t.Errorf("%s: in-campaign instance should validate (ok=%v err=%v)", wt.Slug(), ok, err)
		}
		ok, err = wt.InstanceExists(context.Background(), "camp-1", "cal-x")
		if err != nil || ok {
			t.Errorf("%s: cross-campaign instance must NOT validate (ok=%v)", wt.Slug(), ok)
		}
		ok, err = wt.InstanceExists(context.Background(), "camp-1", "missing")
		if err != nil || ok {
			t.Errorf("%s: not-found should be (false,nil) so it's sweepable; got ok=%v err=%v", wt.Slug(), ok, err)
		}
	}
	// Transient (non-404) error → (false, err) so the resolver won't sweep.
	blip := &wtCalStub{getByIDErr: errors.New("db blip")}
	if ok, err := NewCalendarWidgetType(blip).InstanceExists(context.Background(), "camp-1", "cal-1"); ok || err == nil {
		t.Errorf("transient error must surface as (false,err); got ok=%v err=%v", ok, err)
	}
}

func TestCalendarInstanceBacking_DefaultInstance(t *testing.T) {
	svc := &wtCalStub{defaults: map[string]*Calendar{"camp-1": {ID: "cal-default", CampaignID: "camp-1"}}}
	for _, wt := range []widgetbindings.WidgetType{NewCalendarWidgetType(svc), NewWorldStateWidgetType(svc)} {
		id, ok, err := wt.DefaultInstance(context.Background(), wtHost("camp-1"))
		if err != nil || !ok || id != "cal-default" {
			t.Errorf("%s: default should be the campaign default calendar; got %q ok=%v err=%v", wt.Slug(), id, ok, err)
		}
		// No calendar in campaign → no default (not an error).
		if _, ok, err := wt.DefaultInstance(context.Background(), wtHost("camp-none")); ok || err != nil {
			t.Errorf("%s: no default when campaign has no calendar; ok=%v err=%v", wt.Slug(), ok, err)
		}
	}
}

// recordingCleaner captures OnInstanceDeleted calls.
type recordingCleaner struct{ calls [][2]string } // [widgetType, instanceID]

func (c *recordingCleaner) OnInstanceDeleted(_ context.Context, _ string, widgetType, instanceID string) (int, error) {
	c.calls = append(c.calls, [2]string{widgetType, instanceID})
	return 0, nil
}

// Deleting a calendar must sweep BOTH its calendar and worldstate bindings.
func TestDeleteCalendar_FiresBothDeleteHooks(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1"}, nil
		},
		deleteFn:           func(_ context.Context, _ string) error { return nil },
		listByCampaignIDFn: func(_ context.Context, _ string) ([]Calendar, error) { return nil, nil },
	}
	svc := NewCalendarService(repo)
	cleaner := &recordingCleaner{}
	svc.(interface{ SetBindingCleaner(BindingCleaner) }).SetBindingCleaner(cleaner)

	if err := svc.DeleteCalendar(context.Background(), "cal-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(cleaner.calls) != 2 {
		t.Fatalf("expected 2 delete-hook calls (calendar+worldstate); got %d (%v)", len(cleaner.calls), cleaner.calls)
	}
	want := map[string]bool{WidgetTypeCalendar: false, WidgetTypeWorldstate: false}
	for _, c := range cleaner.calls {
		if c[1] != "cal-1" {
			t.Errorf("hook fired for wrong instance %q", c[1])
		}
		want[c[0]] = true
	}
	if !want[WidgetTypeCalendar] || !want[WidgetTypeWorldstate] {
		t.Errorf("both calendar + worldstate slugs must be swept; got %v", cleaner.calls)
	}
}

// ListInstances (C-WIDGET-BINDING-P4a) maps the campaign's calendars to picker
// InstanceRefs — id + name + a mode-derived icon — for BOTH the calendar and
// worldstate widget types (they share the calendar-instance backing). An empty
// campaign yields an empty (non-nil) slice; a service error surfaces.
func TestCalendarInstanceBacking_ListInstances(t *testing.T) {
	svc := &wtCalStub{list: map[string][]Calendar{
		"camp-1": {
			{ID: "cal-f", CampaignID: "camp-1", Name: "Harptos", Mode: ModeFantasy},
			{ID: "cal-r", CampaignID: "camp-1", Name: "Earth", Mode: ModeRealLife},
		},
	}}
	for _, wt := range []widgetbindings.WidgetType{NewCalendarWidgetType(svc), NewWorldStateWidgetType(svc)} {
		refs, err := wt.ListInstances(context.Background(), "camp-1", 0)
		if err != nil {
			t.Fatalf("%s: ListInstances: %v", wt.Slug(), err)
		}
		if len(refs) != 2 {
			t.Fatalf("%s: want 2 refs; got %d", wt.Slug(), len(refs))
		}
		if refs[0].ID != "cal-f" || refs[0].Name != "Harptos" || refs[0].Icon != "fa-calendar-days" {
			t.Errorf("%s: fantasy ref = %+v", wt.Slug(), refs[0])
		}
		// Real-life calendars get the clock icon.
		if refs[1].ID != "cal-r" || refs[1].Icon != "fa-clock" {
			t.Errorf("%s: reallife ref = %+v", wt.Slug(), refs[1])
		}
		// Empty campaign → empty, non-nil slice (the picker renders "none yet").
		empty, err := wt.ListInstances(context.Background(), "camp-none", 0)
		if err != nil || empty == nil || len(empty) != 0 {
			t.Errorf("%s: empty campaign should give empty non-nil slice; got %v err=%v", wt.Slug(), empty, err)
		}
	}
	// A ListCalendars failure surfaces (not silently swallowed).
	if _, err := NewCalendarWidgetType(&wtCalStub{listErr: errors.New("db down")}).ListInstances(context.Background(), "camp-1", 0); err == nil {
		t.Errorf("ListInstances must surface a service error")
	}
}

// CreateInstance (C-WIDGET-BINDING-P4a) creates a named calendar via the service
// and returns its id — the "create new" half of the picker. The name is taken
// from the generic CreateInput (trimmed); a create error surfaces.
func TestCalendarInstanceBacking_CreateInstance(t *testing.T) {
	svc := &wtCalStub{}
	id, err := NewCalendarWidgetType(svc).CreateInstance(
		context.Background(), "camp-1", widgetbindings.CreateInput{Name: "  Harptos  "})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if id != "new-cal" {
		t.Errorf("want new calendar id; got %q", id)
	}
	if len(svc.created) != 1 || svc.created[0].Name != "Harptos" {
		t.Errorf("name should be trimmed and passed to CreateCalendar; got %+v", svc.created)
	}
	// A create failure surfaces.
	if _, err := NewCalendarWidgetType(&wtCalStub{createErr: errors.New("nope")}).
		CreateInstance(context.Background(), "camp-1", widgetbindings.CreateInput{Name: "x"}); err == nil {
		t.Errorf("CreateInstance must surface a service error")
	}
}
