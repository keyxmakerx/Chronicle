// event_repository_integration_test.go exercises EventRepository against a
// real MariaDB: CRUD, every list/search read, entity ties in both
// directions, tenant isolation between calendars/campaigns, and cascade
// behavior on delete. Skipped under `-short`.
package calendar

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestEventRepository_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	t.Cleanup(func() { db.Close() }) // after fixture cleanups (LIFO), not before

	ctx := context.Background()
	eventRepo := NewEventRepository(db)
	calRepo := NewCalendarRepository(db)
	kindRepo := NewEventKindRepository(db)

	fix := newTestCampaign(t, db, "event")
	cal := newTestCalendar(testUUID(t), fix.CampaignID, "Event Calendar")
	if err := calRepo.Create(ctx, cal); err != nil {
		t.Fatalf("Create calendar: %v", err)
	}
	// A real 12-month year, so every fixture event below (months 1,2,5,6)
	// lands within range: a calendar with NO months defined would make
	// StrandedEventCounts' "no months, no calendar can beat that" false
	// positive false for every event, which is a fixture bug, not a schema one.
	twelveMonths := make([]MonthInput, 12)
	for i := range twelveMonths {
		twelveMonths[i] = MonthInput{Name: "Month", Days: 30, SortOrder: i}
	}
	if err := calRepo.SetMonths(ctx, cal.ID, twelveMonths); err != nil {
		t.Fatalf("SetMonths: %v", err)
	}
	kind, err := kindRepo.Create(ctx, fix.CampaignID, EventKindInput{Slug: "festival", Name: "Festival", Icon: "\U0001f389", Color: "#10b981", DefaultAnnounced: AnnouncedAhead})
	if err != nil {
		t.Fatalf("Create kind: %v", err)
	}
	entityID := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Tied Entity")
	era, err := calRepo.CreateEra(ctx, cal.ID, EraInput{Name: "Founding Era", StartYear: 1, StartMonth: 1, StartDay: 1, Color: "#123456"})
	if err != nil {
		t.Fatalf("CreateEra: %v", err)
	}

	newEvent := func(id, name string, year, month, day int) *Event {
		return &Event{
			ID: id, CalendarID: cal.ID, Name: name, Year: year, Month: month, Day: day,
			Visibility: "everyone", AllDay: true,
		}
	}

	t.Run("CreateEvent and GetEvent round-trip every V5 field, including the kind join", func(t *testing.T) {
		announced := AnnouncedOnDay
		payload := `{"type":"blood","moons":[1]}`
		evt := newEvent(testUUID(t), "Round Trip Event", 10, 5, 20)
		evt.KindID = &kind.ID
		evt.Announced = &announced
		evt.Payload = &payload
		evt.EntityID = &entityID
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}

		got, err := eventRepo.GetEvent(ctx, evt.ID)
		if err != nil || got == nil {
			t.Fatalf("GetEvent = %+v, %v", got, err)
		}
		if got.KindID == nil || *got.KindID != kind.ID {
			t.Errorf("KindID = %v, want %d", got.KindID, kind.ID)
		}
		if got.KindName != kind.Name || got.KindSlug != kind.Slug || got.KindColor != kind.Color {
			t.Errorf("joined kind display fields = name=%q slug=%q color=%q, want %q/%q/%q",
				got.KindName, got.KindSlug, got.KindColor, kind.Name, kind.Slug, kind.Color)
		}
		if got.Announced == nil || *got.Announced != AnnouncedOnDay {
			t.Errorf("Announced = %v, want %q", got.Announced, AnnouncedOnDay)
		}
		if got.Payload == nil || *got.Payload != payload {
			t.Errorf("Payload = %v, want %q", got.Payload, payload)
		}
		if parsed := got.ParsePayload(); parsed == nil || parsed.Type != MoonNightBlood || len(parsed.Moons) != 1 {
			t.Errorf("ParsePayload() = %+v", parsed)
		}
		if got.EntityName == "" {
			t.Error("EntityName should be joined from the linked entity")
		}
	})

	t.Run("CreateEvent with no kind: EffectiveAnnounced falls back to on_day", func(t *testing.T) {
		evt := newEvent(testUUID(t), "Kindless Event", 10, 6, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		got, err := eventRepo.GetEvent(ctx, evt.ID)
		if err != nil || got == nil {
			t.Fatalf("GetEvent: %+v, %v", got, err)
		}
		if got.KindID != nil {
			t.Errorf("KindID = %v, want nil", got.KindID)
		}
		if eff := got.EffectiveAnnounced(nil); eff != AnnouncedOnDay {
			t.Errorf("EffectiveAnnounced(nil) = %q, want %q", eff, AnnouncedOnDay)
		}
		// The event's own KindID is nil, so an unrelated kind passed in must
		// never supply its default: only kind.ID == *e.KindID may do that.
		if eff := got.EffectiveAnnounced(kind); eff != AnnouncedOnDay {
			t.Errorf("EffectiveAnnounced(unrelated kind) = %q, want %q", eff, AnnouncedOnDay)
		}
	})

	t.Run("UpdateEvent persists kind_id/announced/payload changes", func(t *testing.T) {
		evt := newEvent(testUUID(t), "Mutable Event", 11, 1, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		ahead := AnnouncedAhead
		evt.Name = "Renamed Event"
		evt.KindID = &kind.ID
		evt.Announced = &ahead
		if err := eventRepo.UpdateEvent(ctx, evt); err != nil {
			t.Fatalf("UpdateEvent: %v", err)
		}
		got, _ := eventRepo.GetEvent(ctx, evt.ID)
		if got.Name != "Renamed Event" || got.KindID == nil || *got.KindID != kind.ID || got.Announced == nil || *got.Announced != AnnouncedAhead {
			t.Errorf("UpdateEvent did not persist: %+v", got)
		}
	})

	t.Run("deleting an event kind SETs NULL on events, never deletes them", func(t *testing.T) {
		disposableKind, err := kindRepo.Create(ctx, fix.CampaignID, EventKindInput{Slug: "disposable", Name: "Disposable", DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create disposable kind: %v", err)
		}
		evt := newEvent(testUUID(t), "Survives Kind Deletion", 12, 1, 1)
		evt.KindID = &disposableKind.ID
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		if err := kindRepo.Delete(ctx, disposableKind.ID, fix.CampaignID); err != nil {
			t.Fatalf("Delete kind: %v", err)
		}
		got, err := eventRepo.GetEvent(ctx, evt.ID)
		if err != nil || got == nil {
			t.Fatalf("event should survive its kind's deletion, got %+v, %v", got, err)
		}
		if got.KindID != nil {
			t.Errorf("KindID = %v after the kind was deleted, want nil (ON DELETE SET NULL)", got.KindID)
		}
	})

	t.Run("DeleteEvent removes the row", func(t *testing.T) {
		evt := newEvent(testUUID(t), "Doomed Event", 13, 1, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		if err := eventRepo.DeleteEvent(ctx, evt.ID); err != nil {
			t.Fatalf("DeleteEvent: %v", err)
		}
		got, err := eventRepo.GetEvent(ctx, evt.ID)
		if err != nil || got != nil {
			t.Errorf("GetEvent after DeleteEvent = %+v, %v; want (nil, nil)", got, err)
		}
	})

	t.Run("UpdateEventVisibility sets visibility and rules", func(t *testing.T) {
		evt := newEvent(testUUID(t), "Visibility Event", 14, 1, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		rules := `{"denied_users":["u2"]}`
		if err := eventRepo.UpdateEventVisibility(ctx, evt.ID, "dm_only", &rules); err != nil {
			t.Fatalf("UpdateEventVisibility: %v", err)
		}
		got, _ := eventRepo.GetEvent(ctx, evt.ID)
		if got.Visibility != "dm_only" || got.VisibilityRules == nil || *got.VisibilityRules != rules {
			t.Errorf("UpdateEventVisibility did not persist: %+v", got)
		}
	})

	t.Run("CreateEvent and UpdateEvent reject an invalid JSON payload", func(t *testing.T) {
		bad := "{not json"
		evt := newEvent(testUUID(t), "Bad Payload Event", 15, 1, 1)
		evt.Payload = &bad
		if err := eventRepo.CreateEvent(ctx, evt); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
			t.Errorf("CreateEvent(invalid JSON payload) err = %v, want a validation error", err)
		}
		if got, _ := eventRepo.GetEvent(ctx, evt.ID); got != nil {
			t.Error("CreateEvent should not have inserted the event at all")
		}

		evt.Payload = nil
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent (valid, no payload): %v", err)
		}
		evt.Payload = &bad
		if err := eventRepo.UpdateEvent(ctx, evt); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
			t.Errorf("UpdateEvent(invalid JSON payload) err = %v, want a validation error", err)
		}
	})

	t.Run("list methods: month, year, date range, upcoming, search, all", func(t *testing.T) {
		mustExec(t, db, `DELETE FROM calendar_events WHERE calendar_id = ?`, cal.ID) // isolate this subtest's counts
		e1 := newEvent(testUUID(t), "January Fair", 500, 1, 10)
		e2 := newEvent(testUUID(t), "January Duel", 500, 1, 20)
		e3 := newEvent(testUUID(t), "February Feast", 500, 2, 1)
		for _, e := range []*Event{e1, e2, e3} {
			if err := eventRepo.CreateEvent(ctx, e); err != nil {
				t.Fatalf("CreateEvent %s: %v", e.Name, err)
			}
		}

		jan, err := eventRepo.ListEventsForMonth(ctx, cal.ID, 500, 1, permissions.RolePlayer)
		if err != nil || len(jan) != 2 {
			t.Fatalf("ListEventsForMonth = %+v, %v; want 2", jan, err)
		}

		year, err := eventRepo.ListEventsForYear(ctx, cal.ID, 500, permissions.RolePlayer)
		if err != nil || len(year) != 3 {
			t.Fatalf("ListEventsForYear = %d events, %v; want 3", len(year), err)
		}

		all, err := eventRepo.ListAllEvents(ctx, cal.ID)
		if err != nil || len(all) != 3 {
			t.Fatalf("ListAllEvents = %d events, %v; want 3", len(all), err)
		}

		ranged, err := eventRepo.ListEventsForDateRange(ctx, cal.ID, 500, 1, 15, 2, 28, permissions.RolePlayer)
		if err != nil || len(ranged) != 2 {
			t.Fatalf("ListEventsForDateRange(Jan15..Feb28) = %d events, %v; want 2 (Jan Duel + Feb Feast)", len(ranged), err)
		}

		upcoming, err := eventRepo.ListUpcomingEvents(ctx, cal.ID, 500, 1, 15, permissions.RolePlayer, 10)
		if err != nil || len(upcoming) != 2 {
			t.Fatalf("ListUpcomingEvents(from Jan 15) = %d events, %v; want 2", len(upcoming), err)
		}

		found, err := eventRepo.SearchEvents(ctx, cal.ID, "Fair", permissions.RolePlayer)
		if err != nil || len(found) != 1 || found[0].ID != e1.ID {
			t.Fatalf("SearchEvents(\"Fair\") = %+v, %v", found, err)
		}
	})

	t.Run("visibility filtering: a player never sees a dm_only event, an owner does", func(t *testing.T) {
		mustExec(t, db, `DELETE FROM calendar_events WHERE calendar_id = ?`, cal.ID)
		secret := newEvent(testUUID(t), "Secret Plot", 600, 1, 1)
		secret.Visibility = "dm_only"
		if err := eventRepo.CreateEvent(ctx, secret); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		asPlayer, err := eventRepo.ListEventsForYear(ctx, cal.ID, 600, permissions.RolePlayer)
		if err != nil || len(asPlayer) != 0 {
			t.Fatalf("player sees %d dm_only events, %v; want 0", len(asPlayer), err)
		}
		asOwner, err := eventRepo.ListEventsForYear(ctx, cal.ID, 600, permissions.RoleOwner)
		if err != nil || len(asOwner) != 1 {
			t.Fatalf("owner sees %d events, %v; want 1", len(asOwner), err)
		}
	})

	t.Run("tenant isolation: a calendar's events never appear under another calendar's id", func(t *testing.T) {
		otherCal := newTestCalendar(testUUID(t), fix.CampaignID, "Other Calendar")
		if err := calRepo.Create(ctx, otherCal); err != nil {
			t.Fatalf("Create otherCal: %v", err)
		}
		mustExec(t, db, `DELETE FROM calendar_events WHERE calendar_id IN (?, ?)`, cal.ID, otherCal.ID)
		mine := newEvent(testUUID(t), "Mine", 700, 1, 1)
		if err := eventRepo.CreateEvent(ctx, mine); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		leaked, err := eventRepo.ListEventsForYear(ctx, otherCal.ID, 700, permissions.RoleOwner)
		if err != nil {
			t.Fatalf("ListEventsForYear(otherCal): %v", err)
		}
		if len(leaked) != 0 {
			t.Errorf("ListEventsForYear(otherCal.ID) returned %d events that belong to a different calendar", len(leaked))
		}
	})

	t.Run("StrandedEventCounts finds an event whose month no longer exists", func(t *testing.T) {
		strandedCal := newTestCalendar(testUUID(t), fix.CampaignID, "Stranding Calendar")
		if err := calRepo.Create(ctx, strandedCal); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := calRepo.SetMonths(ctx, strandedCal.ID, []MonthInput{{Name: "Only Month", Days: 30}}); err != nil {
			t.Fatalf("SetMonths: %v", err)
		}
		stranded := newEvent(testUUID(t), "Stranded", 1, 5, 1) // month 5 doesn't exist (only 1 month)
		stranded.CalendarID = strandedCal.ID
		if err := eventRepo.CreateEvent(ctx, stranded); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		counts, err := eventRepo.StrandedEventCounts(ctx, fix.CampaignID)
		if err != nil {
			t.Fatalf("StrandedEventCounts: %v", err)
		}
		if counts[strandedCal.ID] != 1 {
			t.Errorf("StrandedEventCounts[%s] = %d, want 1", strandedCal.ID, counts[strandedCal.ID])
		}
		if _, ok := counts[cal.ID]; ok {
			t.Errorf("StrandedEventCounts should omit calendars with zero stranded events, found an entry for %s", cal.ID)
		}
	})

	t.Run("EventDatesForCalendars batches multiple calendars in one read", func(t *testing.T) {
		mustExec(t, db, `DELETE FROM calendar_events WHERE calendar_id = ?`, cal.ID)
		e := newEvent(testUUID(t), "Batched", 800, 1, 1)
		if err := eventRepo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		out, err := eventRepo.EventDatesForCalendars(ctx, []string{cal.ID}, permissions.RolePlayer)
		if err != nil {
			t.Fatalf("EventDatesForCalendars: %v", err)
		}
		if len(out[cal.ID]) != 1 || out[cal.ID][0].Name != "Batched" {
			t.Errorf("EventDatesForCalendars[%s] = %+v", cal.ID, out[cal.ID])
		}
	})

	t.Run("entity ties: link/unlink both directions, visibility-gated reads, cascades", func(t *testing.T) {
		evt := newEvent(testUUID(t), "Tied Event", 900, 1, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}

		if err := eventRepo.LinkEntityEvent(ctx, entityID, evt.ID, string(RoleInvolved)); err != nil {
			t.Fatalf("LinkEntityEvent: %v", err)
		}
		// Re-linking the same pair updates the role rather than erroring
		// (the unique key on (entity_id, event_id) makes this an upsert).
		if err := eventRepo.LinkEntityEvent(ctx, entityID, evt.ID, string(RolePresent)); err != nil {
			t.Fatalf("re-LinkEntityEvent: %v", err)
		}
		if err := eventRepo.LinkEntityEra(ctx, entityID, era.ID, nil); err != nil {
			t.Fatalf("LinkEntityEra: %v", err)
		}

		refs, err := eventRepo.EntitiesForEvent(ctx, evt.ID, permissions.RoleOwner, fix.UserID)
		if err != nil || len(refs) != 1 || refs[0].EntityID != entityID {
			t.Fatalf("EntitiesForEvent = %+v, %v", refs, err)
		}
		if refs[0].ParticipationRole == nil || *refs[0].ParticipationRole != string(RolePresent) {
			t.Errorf("EntitiesForEvent role = %v, want %q (the updated role)", refs[0].ParticipationRole, RolePresent)
		}

		eraRefs, err := eventRepo.EntitiesForEra(ctx, era.ID, permissions.RoleOwner, fix.UserID)
		if err != nil || len(eraRefs) != 1 {
			t.Fatalf("EntitiesForEra = %+v, %v", eraRefs, err)
		}

		calRefs, err := eventRepo.EntitiesForCalendar(ctx, cal.ID, permissions.RoleOwner, fix.UserID)
		if err != nil {
			t.Fatalf("EntitiesForCalendar: %v", err)
		}
		found := false
		for _, ref := range calRefs {
			if ref.EntityID == entityID {
				found = true
			}
		}
		if !found {
			t.Errorf("EntitiesForCalendar did not include the tied entity: %+v", calRefs)
		}

		eventTies, err := eventRepo.EventsForEntity(ctx, fix.CampaignID, entityID, permissions.RoleOwner)
		if err != nil {
			t.Fatalf("EventsForEntity: %v", err)
		}
		foundEvt := false
		for _, tie := range eventTies {
			if tie.Event.ID == evt.ID {
				foundEvt = true
				if tie.ParticipationRole != string(RolePresent) {
					t.Errorf("EventsForEntity role = %q, want %q", tie.ParticipationRole, RolePresent)
				}
			}
		}
		if !foundEvt {
			t.Errorf("EventsForEntity did not include the tied event: %+v", eventTies)
		}

		eraTies, err := eventRepo.ErasForEntity(ctx, fix.CampaignID, entityID)
		if err != nil {
			t.Fatalf("ErasForEntity: %v", err)
		}
		foundEra := false
		for _, tie := range eraTies {
			if tie.Era.ID == era.ID {
				foundEra = true
				if tie.ParticipationRole != nil {
					t.Errorf("ErasForEntity role = %v, want nil (era ties may carry no role)", tie.ParticipationRole)
				}
			}
		}
		if !foundEra {
			t.Errorf("ErasForEntity did not include the tied era: %+v", eraTies)
		}

		if err := eventRepo.UnlinkEntityEvent(ctx, entityID, evt.ID); err != nil {
			t.Fatalf("UnlinkEntityEvent: %v", err)
		}
		refs, _ = eventRepo.EntitiesForEvent(ctx, evt.ID, permissions.RoleOwner, fix.UserID)
		if len(refs) != 0 {
			t.Errorf("EntitiesForEvent after Unlink = %+v, want empty", refs)
		}

		if err := eventRepo.UnlinkEntityEra(ctx, entityID, era.ID); err != nil {
			t.Fatalf("UnlinkEntityEra: %v", err)
		}
		eraRefs, _ = eventRepo.EntitiesForEra(ctx, era.ID, permissions.RoleOwner, fix.UserID)
		if len(eraRefs) != 0 {
			t.Errorf("EntitiesForEra after Unlink = %+v, want empty", eraRefs)
		}
	})

	t.Run("security: kind_id/entity_id, links and entity-scoped ties never cross a campaign boundary", func(t *testing.T) {
		other := newTestCampaign(t, db, "event-other")
		otherCal := newTestCalendar(testUUID(t), other.CampaignID, "Other Campaign Calendar")
		if err := calRepo.Create(ctx, otherCal); err != nil {
			t.Fatalf("Create otherCal: %v", err)
		}
		otherKind, err := kindRepo.Create(ctx, other.CampaignID, EventKindInput{Slug: "other-kind", Name: "Other Kind", DefaultAnnounced: AnnouncedOnDay})
		if err != nil {
			t.Fatalf("Create otherKind: %v", err)
		}
		otherEntity := newTestEntity(t, db, other.CampaignID, other.UserID, "Other Campaign Entity")

		t.Run("CreateEvent rejects a kind_id from another campaign", func(t *testing.T) {
			evt := newEvent(testUUID(t), "Cross-campaign kind", 300, 1, 1)
			evt.KindID = &otherKind.ID
			if err := eventRepo.CreateEvent(ctx, evt); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
				t.Errorf("CreateEvent(kind from another campaign) err = %v, want a validation error", err)
			}
			if got, _ := eventRepo.GetEvent(ctx, evt.ID); got != nil {
				t.Error("CreateEvent should not have inserted the event at all")
			}
		})

		t.Run("CreateEvent rejects an entity_id from another campaign", func(t *testing.T) {
			evt := newEvent(testUUID(t), "Cross-campaign entity", 300, 1, 2)
			evt.EntityID = &otherEntity
			if err := eventRepo.CreateEvent(ctx, evt); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
				t.Errorf("CreateEvent(entity from another campaign) err = %v, want a validation error", err)
			}
		})

		t.Run("UpdateEvent rejects a kind_id from another campaign", func(t *testing.T) {
			evt := newEvent(testUUID(t), "Update Target", 300, 1, 3)
			if err := eventRepo.CreateEvent(ctx, evt); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
			evt.KindID = &otherKind.ID
			if err := eventRepo.UpdateEvent(ctx, evt); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
				t.Errorf("UpdateEvent(kind from another campaign) err = %v, want a validation error", err)
			}
			got, _ := eventRepo.GetEvent(ctx, evt.ID)
			if got == nil || got.KindID != nil {
				t.Errorf("UpdateEvent should not have set the cross-campaign kind_id: %+v", got)
			}
		})

		t.Run("UpdateEvent rejects an entity_id from another campaign", func(t *testing.T) {
			evt := newEvent(testUUID(t), "Update Target 2", 300, 1, 4)
			if err := eventRepo.CreateEvent(ctx, evt); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
			evt.EntityID = &otherEntity
			if err := eventRepo.UpdateEvent(ctx, evt); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
				t.Errorf("UpdateEvent(entity from another campaign) err = %v, want a validation error", err)
			}
		})

		t.Run("LinkEntityEvent rejects an entity from another campaign", func(t *testing.T) {
			evt := newEvent(testUUID(t), "Link Target", 300, 1, 5)
			if err := eventRepo.CreateEvent(ctx, evt); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
			if err := eventRepo.LinkEntityEvent(ctx, otherEntity, evt.ID, string(RoleInvolved)); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
				t.Errorf("LinkEntityEvent(cross-campaign) err = %v, want a validation error", err)
			}
			refs, _ := eventRepo.EntitiesForEvent(ctx, evt.ID, permissions.RoleOwner, fix.UserID)
			if len(refs) != 0 {
				t.Errorf("LinkEntityEvent should not have created the tie: %+v", refs)
			}
		})

		t.Run("LinkEntityEra rejects an entity from another campaign", func(t *testing.T) {
			if err := eventRepo.LinkEntityEra(ctx, otherEntity, era.ID, nil); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
				t.Errorf("LinkEntityEra(cross-campaign) err = %v, want a validation error", err)
			}
		})

		t.Run("EntitiesForEvent/EntitiesForEra/EntitiesForCalendar never surface an entity from another campaign", func(t *testing.T) {
			// Simulates a tie that predates this guard (or a bug elsewhere):
			// insert it directly, bypassing LinkEntityEvent's own check, so
			// the READ side is proven to defend independently of the write side.
			evt := newEvent(testUUID(t), "Stale Tie Event", 300, 1, 6)
			if err := eventRepo.CreateEvent(ctx, evt); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
			mustExec(t, db, `INSERT INTO entity_event_links (entity_id, event_id, participation_role) VALUES (?, ?, ?)`,
				otherEntity, evt.ID, string(RoleInvolved))

			refs, err := eventRepo.EntitiesForEvent(ctx, evt.ID, permissions.RoleOwner, fix.UserID)
			if err != nil {
				t.Fatalf("EntitiesForEvent: %v", err)
			}
			for _, ref := range refs {
				if ref.EntityID == otherEntity {
					t.Error("EntitiesForEvent surfaced an entity from another campaign")
				}
			}

			calRefs, err := eventRepo.EntitiesForCalendar(ctx, cal.ID, permissions.RoleOwner, fix.UserID)
			if err != nil {
				t.Fatalf("EntitiesForCalendar: %v", err)
			}
			for _, ref := range calRefs {
				if ref.EntityID == otherEntity {
					t.Error("EntitiesForCalendar surfaced an entity from another campaign")
				}
			}
		})

		t.Run("EventsForEntity and ErasForEntity are scoped to the given campaign", func(t *testing.T) {
			evt := newEvent(testUUID(t), "Scoped Tie Event", 300, 1, 7)
			if err := eventRepo.CreateEvent(ctx, evt); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
			if err := eventRepo.LinkEntityEvent(ctx, entityID, evt.ID, string(RoleInvolved)); err != nil {
				t.Fatalf("LinkEntityEvent: %v", err)
			}
			if err := eventRepo.LinkEntityEra(ctx, entityID, era.ID, nil); err != nil {
				t.Fatalf("LinkEntityEra: %v", err)
			}

			ties, err := eventRepo.EventsForEntity(ctx, fix.CampaignID, entityID, permissions.RoleOwner)
			if err != nil {
				t.Fatalf("EventsForEntity(own campaign): %v", err)
			}
			foundOwn := false
			for _, tie := range ties {
				if tie.Event.ID == evt.ID {
					foundOwn = true
				}
			}
			if !foundOwn {
				t.Error("EventsForEntity(own campaign) should include the tied event")
			}

			ties, err = eventRepo.EventsForEntity(ctx, other.CampaignID, entityID, permissions.RoleOwner)
			if err != nil {
				t.Fatalf("EventsForEntity(other campaign): %v", err)
			}
			if len(ties) != 0 {
				t.Errorf("EventsForEntity(wrong campaign) = %+v, want empty", ties)
			}

			eraTies, err := eventRepo.ErasForEntity(ctx, fix.CampaignID, entityID)
			if err != nil {
				t.Fatalf("ErasForEntity(own campaign): %v", err)
			}
			foundEra := false
			for _, tie := range eraTies {
				if tie.Era.ID == era.ID {
					foundEra = true
				}
			}
			if !foundEra {
				t.Error("ErasForEntity(own campaign) should include the tied era")
			}

			eraTies, err = eventRepo.ErasForEntity(ctx, other.CampaignID, entityID)
			if err != nil {
				t.Fatalf("ErasForEntity(other campaign): %v", err)
			}
			if len(eraTies) != 0 {
				t.Errorf("ErasForEntity(wrong campaign) = %+v, want empty", eraTies)
			}
		})

		t.Run("EventsForEntity hides a dm_only tied event from a player", func(t *testing.T) {
			secret := newEvent(testUUID(t), "Secret Tie Event", 301, 1, 7)
			secret.Visibility = "dm_only"
			if err := eventRepo.CreateEvent(ctx, secret); err != nil {
				t.Fatalf("CreateEvent: %v", err)
			}
			if err := eventRepo.LinkEntityEvent(ctx, entityID, secret.ID, string(RoleInvolved)); err != nil {
				t.Fatalf("LinkEntityEvent: %v", err)
			}
			has := func(ties []EntityEventTie) bool {
				for _, tie := range ties {
					if tie.Event.ID == secret.ID {
						return true
					}
				}
				return false
			}

			asPlayer, err := eventRepo.EventsForEntity(ctx, fix.CampaignID, entityID, permissions.RolePlayer)
			if err != nil {
				t.Fatalf("EventsForEntity(player): %v", err)
			}
			if has(asPlayer) {
				t.Error("EventsForEntity(player) surfaced a dm_only event")
			}
			asOwner, err := eventRepo.EventsForEntity(ctx, fix.CampaignID, entityID, permissions.RoleOwner)
			if err != nil {
				t.Fatalf("EventsForEntity(owner): %v", err)
			}
			if !has(asOwner) {
				t.Error("EventsForEntity(owner) should include the dm_only event")
			}
		})
	})

	t.Run("deleting an event cascades entity_event_links", func(t *testing.T) {
		evt := newEvent(testUUID(t), "Cascade Source Event", 901, 1, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		if err := eventRepo.LinkEntityEvent(ctx, entityID, evt.ID, string(RoleInvolved)); err != nil {
			t.Fatalf("LinkEntityEvent: %v", err)
		}
		if err := eventRepo.DeleteEvent(ctx, evt.ID); err != nil {
			t.Fatalf("DeleteEvent: %v", err)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity_event_links WHERE event_id = ?`, evt.ID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Error("deleting the event should cascade entity_event_links, but the row survived")
		}
	})

	t.Run("deleting an era cascades entity_era_links", func(t *testing.T) {
		disposableEra, err := calRepo.CreateEra(ctx, cal.ID, EraInput{Name: "Disposable Era", StartYear: 900, Color: "#abcdef"})
		if err != nil {
			t.Fatalf("CreateEra: %v", err)
		}
		if err := eventRepo.LinkEntityEra(ctx, entityID, disposableEra.ID, nil); err != nil {
			t.Fatalf("LinkEntityEra: %v", err)
		}
		if err := calRepo.DeleteEra(ctx, cal.ID, disposableEra.ID); err != nil {
			t.Fatalf("DeleteEra: %v", err)
		}
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity_era_links WHERE era_id = ?`, disposableEra.ID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Error("deleting the era should cascade entity_era_links, but the row survived")
		}
	})

	t.Run("deleting an entity cascades both link tables", func(t *testing.T) {
		disposableEntity := newTestEntity(t, db, fix.CampaignID, fix.UserID, "Disposable Entity")
		evt := newEvent(testUUID(t), "For Disposable Entity", 902, 1, 1)
		if err := eventRepo.CreateEvent(ctx, evt); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		if err := eventRepo.LinkEntityEvent(ctx, disposableEntity, evt.ID, string(RoleInvolved)); err != nil {
			t.Fatalf("LinkEntityEvent: %v", err)
		}
		if err := eventRepo.LinkEntityEra(ctx, disposableEntity, era.ID, nil); err != nil {
			t.Fatalf("LinkEntityEra: %v", err)
		}
		mustExec(t, db, `DELETE FROM entities WHERE id = ?`, disposableEntity)

		var nEvt, nEra int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity_event_links WHERE entity_id = ?`, disposableEntity).Scan(&nEvt); err != nil {
			t.Fatalf("count event links: %v", err)
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entity_era_links WHERE entity_id = ?`, disposableEntity).Scan(&nEra); err != nil {
			t.Fatalf("count era links: %v", err)
		}
		if nEvt != 0 || nEra != 0 {
			t.Errorf("deleting the entity should cascade both link tables; event links=%d era links=%d", nEvt, nEra)
		}
	})
}
