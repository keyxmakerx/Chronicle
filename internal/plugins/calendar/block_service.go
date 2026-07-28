// block_service.go — C-CALV4-SPINE-P2 (calendar-v4 wave 1, W-A).
//
// The calendar Block's service spine: NARROW interfaces, the batched eager
// loader, the cross-calendar NEXT UP index, and the sync pill.
//
// NARROW ON PURPOSE. Nothing here widens CalendarRepository (~60 methods,
// implemented by a hand-written mockCalendarRepo in service_test.go) or
// CalendarService. Widening either churns the mock and breaks the package's
// test build for every parallel calendar-v4 slice at once. BlockRepository is
// the Block's own read surface, constructed by NewBlockRepository over the same
// *sql.DB; the two cross-plugin facts the Block needs (whether a module is
// connected, and the real-time wall-clock seam) arrive through one-method
// interfaces installed at the composition root.
package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// --- narrow read surface ---------------------------------------------------

// BlockRepository is everything the Block spine reads. Every list-shaped method
// is BATCHED across calendars: the Bench renders four calendars on the page the
// nav Calendar tab lands on, for every player, and eagerLoad's nine sequential
// per-calendar queries do not survive that.
type BlockRepository interface {
	GetByID(ctx context.Context, id string) (*Calendar, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]Calendar, error)
	ListEventsForMonth(ctx context.Context, calendarID string, year, month int, role int) ([]Event, error)

	MonthsForCalendars(ctx context.Context, calIDs []string) (map[string][]Month, error)
	WeekdaysForCalendars(ctx context.Context, calIDs []string) (map[string][]Weekday, error)
	MoonsForCalendars(ctx context.Context, calIDs []string) (map[string][]Moon, error)
	SeasonsForCalendars(ctx context.Context, calIDs []string) (map[string][]Season, error)
	ErasForCalendars(ctx context.Context, calIDs []string) (map[string][]Era, error)
	EventCategoriesForCalendars(ctx context.Context, calIDs []string) (map[string][]EventCategory, error)
	FestivalsForCalendars(ctx context.Context, calIDs []string) (map[string][]Festival, error)
	CyclesForCalendars(ctx context.Context, calIDs []string) (map[string][]Cycle, error)

	EntitiesForEventsBatch(ctx context.Context, eventIDs []string, role int, userID string) (map[string][]EntityTieRef, error)
	TiedEventIDsForEntity(ctx context.Context, entityID string, eventIDs []string) (map[string]bool, error)
	UpcomingEventsForCalendars(ctx context.Context, calIDs []string, role int) (map[string][]Event, error)
}

// RealTimeSeam applies the wall-clock seam to a calendar loaded outside the
// main service loader. A real-time calendar's Current* fields are COMPUTED from
// the clock in its anchor zone, not stored (C-REAL-CALENDAR-P2 F1), so a
// batch-loaded calendar that skips this seam prints a stale date.
//
// It is a one-method interface rather than a dependency on CalendarService so
// the Block never holds the 60-method aggregate. *calendarService satisfies it
// through the additive ApplyRealTime method in service.go.
type RealTimeSeam interface {
	ApplyRealTime(cal *Calendar)
}

// BlockSyncSnapshot is the transport-agnostic answer to "is anything synced to
// this campaign, and what did it last see". It is deliberately not a syncapi
// type: internal/plugins/calendar must not import internal/plugins/syncapi
// (TestPluginImportGuard), so the composition root adapts.
type BlockSyncSnapshot struct {
	// Connected is true when a module has ever talked to this campaign.
	Connected bool
	// Transport names it for the pill ("Foundry"). "" omits the segment.
	Transport string
	// LastSeen is when the module last read the campaign's date. Zero = never.
	LastSeen time.Time
	// Applied* is the date the module reported it actually applied, when it has
	// confirmed one. Nil = never confirmed.
	AppliedYear, AppliedMonth, AppliedDay *int
}

// SyncLinkProbe answers the sync pill's question without the calendar plugin
// knowing what a Foundry module is.
type SyncLinkProbe interface {
	CampaignSyncSnapshot(ctx context.Context, campaignID string) (BlockSyncSnapshot, error)
}

// --- the service -----------------------------------------------------------

// BlockService produces BlockData and the reads the Block's hosts need.
type BlockService struct {
	repo      BlockRepository
	realTime  RealTimeSeam
	syncProbe SyncLinkProbe
}

// NewBlockService builds the spine. The two seams are optional and installed
// post-construction (the SetTimelineLister / SetMailSender idiom): a nil
// RealTimeSeam simply means a real-time calendar shows its stored date, and a
// nil SyncLinkProbe means the pill reports "Not linked" — both are honest
// degradations rather than a nil dereference.
func NewBlockService(repo BlockRepository) *BlockService {
	return &BlockService{repo: repo}
}

// SetRealTimeSeam installs the wall-clock seam.
func (s *BlockService) SetRealTimeSeam(rt RealTimeSeam) { s.realTime = rt }

// SetSyncLinkProbe installs the sync-pill probe.
func (s *BlockService) SetSyncLinkProbe(p SyncLinkProbe) { s.syncProbe = p }

// --- the composition-root singleton ----------------------------------------

// blockSpine holds the process-wide Block spine.
//
// WHY A PACKAGE-LEVEL PROVIDER. internal/app/routes.go belongs to exactly ONE
// calendar-v4 slice for the whole wave (COMMON §5 rule 2) — this one — and the
// surfaces that CONSUME the spine (C-CALV4-HOST-P3's entity-page hosting,
// C-CALV4-BENCH-P4's Bench) land afterwards and must not have to re-edit it.
// The alternative, a field on calendar.Handler, would mean editing handler.go,
// which is one of the repo's source-text-pinned files (COMMON §3). This is the
// same composition-root provider idiom systems.SetSyncMappingProvider already
// uses from routes.go (~:2516).
//
// atomic.Pointer, not a bare var: it is written once at boot and read on every
// request, and the race detector runs in CI.
var blockSpine atomic.Pointer[BlockService]

// InstallBlockSpine wires the Block spine from the composition root. Safe to
// call once at boot; later calls replace it.
func InstallBlockSpine(s *BlockService) { blockSpine.Store(s) }

// BlockSpine returns the installed spine, or nil when the calendar plugin is
// degraded. Callers must nil-check — a degraded plugin renders no Block, which
// is the same honesty state the rest of the plugin uses.
func BlockSpine() *BlockService { return blockSpine.Load() }

// --- BlockData -------------------------------------------------------------

// BlockDate is a calendar date. Month is 1-BASED, matching Event.Month and
// Calendar.CurrentMonth (Calendar.MonthDays and Calendar.Months are 0-based —
// the conversion happens once, here, rather than at every call site).
type BlockDate struct {
	Year, Month, Day int
}

// BlockRequest is one Block render.
type BlockRequest struct {
	CalendarID string
	CampaignID string
	Viewer     BlockViewer
	// View is the month being displayed. A zero Year falls back to the
	// calendar's own current month, which is what an unnavigated Block shows.
	View BlockDate
	// IsActive marks the viewer's currently-active calendar.
	IsActive bool
	// LedgerHidden / ShelfHidden dock the zones but hide them (the real-world
	// Block on the Bench renders with noShelf).
	LedgerHidden bool
	ShelfHidden  bool
	// MoonCap bounds the per-day discs; <= 0 draws every declared moon.
	MoonCap int
}

// requireVisibleCalendar loads one calendar eagerly and applies the
// calendar-level visibility gate (C-CALV4-SEAM-P5 §4): a calendar the viewer
// may not see answers with the SAME not-found shape a nonexistent one does —
// the apperror.NewNotFound("calendar not found") handler_v2.go's
// requireVisibleCalendar returns — so a hidden calendar's existence never
// leaks through a distinguishable error. ONE return site covers both branches
// on purpose: two constructors would eventually drift into an oracle.
//
// The gate lives on the spine because calendarVisibleTo is unexported: a
// phase-B host outside this package cannot reproduce it, and its only
// alternative is the 60-method service the spine exists to avoid. Same
// predicate family as UpcomingAcrossCalendars' filterCalendarsByUser — the
// owner/co-DM and system-context (empty UserID) bypasses are the predicate's,
// not re-decided here.
func (s *BlockService) requireVisibleCalendar(ctx context.Context, calendarID string, viewer BlockViewer) (*Calendar, error) {
	cals, err := s.EagerLoadCalendars(ctx, []string{calendarID})
	if err != nil {
		return nil, err
	}
	cal := cals[calendarID]
	if cal == nil || !calendarVisibleTo(cal, viewer.Role, viewer.UserID) {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return cal, nil
}

// Block builds one BlockData.
//
// The order is load-bearing: resolve the calendar (eager, real-time-seamed,
// visibility-GATED — see requireVisibleCalendar), read the month's candidate
// events plus any intercalary months hanging off it, run the viewer filter,
// resolve ties in ONE batched query over the FILTERED set — the tie read
// applies no visibility filter of its own and relies on this ordering
// (TiedEventIDsForEntity's contract) — then hand everything to projectBlock,
// which derives both counts and every cell from one pass. projectBlock's own
// filterEventsByUser is idempotent over the already-filtered slice, so direct
// projectBlock callers (the seam tests) still get the single authoritative
// filter.
func (s *BlockService) Block(ctx context.Context, req BlockRequest) (calblock.BlockData, error) {
	cal, err := s.requireVisibleCalendar(ctx, req.CalendarID, req.Viewer)
	if err != nil {
		return calblock.BlockData{}, err
	}

	monthIdx, year := s.resolveView(cal, req.View)
	events, err := s.candidateEvents(ctx, cal, monthIdx, year, req.Viewer.Role)
	if err != nil {
		return calblock.BlockData{}, err
	}
	// Filter BEFORE the tie read. TiedEventIDsForEntity deliberately applies no
	// visibility filter of its own on the promise that every event id it is
	// handed is already viewer-visible; handing it raw candidates would ask it
	// about events this viewer may not see. `events` is compacted in place and
	// only the filtered prefix is read from here on (COMMON §7).
	events = filterEventsByUser(events, req.Viewer.Role, req.Viewer.UserID)

	tied, err := s.tiedEventIDs(ctx, req.Viewer.HostEntity, events)
	if err != nil {
		return calblock.BlockData{}, err
	}

	pill, err := s.CalendarLinkStatus(ctx, campaignOf(cal, req.CampaignID))
	if err != nil {
		// A degraded sync probe must not blank a calendar. "Not linked" with the
		// real denominator is the honest fallback; the denominator NEVER drops.
		pill = blockSyncPill(blockSyncFacts{State: blockSyncStateNone, Total: pill.Total})
	}

	return projectBlock(BlockProjectionInput{
		Calendar:     cal,
		Events:       events,
		Viewer:       req.Viewer,
		MonthIndex:   monthIdx,
		Year:         year,
		TiedEventIDs: tied,
		Sync:         pill,
		IsActive:     req.IsActive,
		LedgerHidden: req.LedgerHidden,
		ShelfHidden:  req.ShelfHidden,
		MoonCap:      req.MoonCap,
	}), nil
}

// campaignOf prefers the calendar's own campaign over a caller-supplied one —
// the calendar row is the authority, and a mismatched campaign id would compute
// the pill's denominator against the wrong campaign.
func campaignOf(cal *Calendar, fallback string) string {
	if cal != nil && cal.CampaignID != "" {
		return cal.CampaignID
	}
	return fallback
}

// resolveView clamps the requested view to a real month. Returns a 0-BASED
// month index.
//
// A fully unset view means "the Block has not been navigated", which is the
// calendar's own current month. A PARTIALLY set view keeps what the caller did
// supply — a bad month with a real year clamps into that year rather than
// silently jumping back to today, because losing a navigated year is the kind
// of bug that reads as "the calendar keeps snapping back".
func (s *BlockService) resolveView(cal *Calendar, v BlockDate) (monthIdx, year int) {
	clamp := func(i int) int {
		if i < 0 {
			return 0
		}
		if n := len(cal.Months); n > 0 && i >= n {
			return n - 1
		}
		return i
	}
	if v.Year == 0 && v.Month < 1 {
		return clamp(cal.CurrentMonth - 1), cal.CurrentYear
	}
	year = v.Year
	if year == 0 {
		year = cal.CurrentYear
	}
	return clamp(v.Month - 1), year
}

// candidateEvents reads the rendered month PLUS every intercalary month hanging
// off it, as one candidate slice holding each event ONCE.
//
// Concatenating before the viewer filter (rather than filtering each month) is
// what keeps the one-pass rule intact: projectBlock filters the union exactly
// once, so the counts and the intercalary marks come from the same pass as the
// grid cells.
//
// The union DEDUPES on event id, and the dedupe lives HERE, at the source
// (C-CALV4-SEAM-P5 §5 ruling — not a downstream marks filter). Every
// ListEventsForMonth call returns every recurring candidate regardless of the
// month asked for (the C-CAL-EDITOR-EXPANSION PR2 widening; OccursOn decides
// placement in Go), so querying the rendered month plus N intercalary months
// yields N+1 copies of each recurring row. Downstream assumes the slice holds
// each event once: blockCountEvents happens to dedupe on id, but
// blockMarksForDate emits one mark per row, so an undeduped union draws
// duplicate chips and a doubled foot total while the counts stay right.
func (s *BlockService) candidateEvents(ctx context.Context, cal *Calendar, monthIdx, year, role int) ([]Event, error) {
	months := append([]int{monthIdx}, blockIntercalaryMonths(cal, monthIdx)...)
	var out []Event
	seen := make(map[string]bool)
	for _, mi := range months {
		evs, err := s.repo.ListEventsForMonth(ctx, cal.ID, year, mi+1, role)
		if err != nil {
			return nil, fmt.Errorf("block events for month %d: %w", mi+1, err)
		}
		for i := range evs {
			if seen[evs[i].ID] {
				continue
			}
			seen[evs[i].ID] = true
			out = append(out, evs[i])
		}
	}
	return out, nil
}

// tiedEventIDs resolves the tie set in ONE batched query. Off an entity page
// there is nothing to tie to and no query is issued.
//
// PRECONDITION: `events` is already viewer-filtered (Block runs
// filterEventsByUser before calling this). TiedEventIDsForEntity applies no
// visibility filter of its own on the strength of that invariant — see its
// doc comment in block_repository.go.
func (s *BlockService) tiedEventIDs(ctx context.Context, hostEntity string, events []Event) (map[string]bool, error) {
	if strings.TrimSpace(hostEntity) == "" || len(events) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].ID)
	}
	tied, err := s.repo.TiedEventIDsForEntity(ctx, hostEntity, ids)
	if err != nil {
		return nil, fmt.Errorf("block tie read: %w", err)
	}
	return tied, nil
}

// --- EventsForDay ----------------------------------------------------------

// EventsForDay returns the viewer-visible events on one date, recurrence
// expanded through the single OccursOn predicate.
//
// Consumed by W-B (the docked Ledger answering a day) and the ledger surfaces.
// The calendar itself is visibility-gated (requireVisibleCalendar) — a hidden
// calendar's day must be as unanswerable as a nonexistent one's. The event
// filter runs ONCE over the month's candidate rows; the day selection is
// applied to the filtered result, never to a second filtering pass.
func (s *BlockService) EventsForDay(ctx context.Context, calendarID string, viewer BlockViewer, date BlockDate) ([]Event, error) {
	cal, err := s.requireVisibleCalendar(ctx, calendarID, viewer)
	if err != nil {
		return nil, err
	}
	candidates, err := s.repo.ListEventsForMonth(ctx, calendarID, date.Year, date.Month, viewer.Role)
	if err != nil {
		return nil, fmt.Errorf("events for day: %w", err)
	}
	// THE ONE PASS — `candidates` is compacted in place and must not be reread.
	visible := filterEventsByUser(candidates, viewer.Role, viewer.UserID)
	out := make([]Event, 0, len(visible))
	for i := range visible {
		if visible[i].OccursOn(cal, date.Year, date.Month, date.Day) {
			out = append(out, visible[i])
		}
	}
	blockSortEventsStable(out)
	return out, nil
}

// --- EntitiesForEvents -----------------------------------------------------

// EntitiesForEvents returns the entities tied to each of the given events, in
// ONE query — the batched, not-N+1 read behind tie rendering. The entity set is
// gated by the same entityVisibilityFilter EntitiesForEvent uses, so a player
// cannot learn a hidden entity's name through a tie list.
func (s *BlockService) EntitiesForEvents(ctx context.Context, eventIDs []string, viewer BlockViewer) (map[string][]EntityTieRef, error) {
	if len(eventIDs) == 0 {
		return map[string][]EntityTieRef{}, nil
	}
	out, err := s.repo.EntitiesForEventsBatch(ctx, eventIDs, viewer.Role, viewer.UserID)
	if err != nil {
		return nil, fmt.Errorf("entities for events: %w", err)
	}
	return out, nil
}

// --- EagerLoadCalendars ----------------------------------------------------

// EagerLoadCalendars loads several calendars WITH their sub-resources in a
// fixed number of queries — nine total, whatever the calendar count, against
// eagerLoad's nine PER CALENDAR.
//
// WHAT IT LOADS: months, weekdays, moons, seasons, eras, event categories,
// festivals, cycles (+ their entries). WHAT IT DOES NOT: Weather. No BlockData
// field consumes weather, the single-calendar eagerLoad still serves the
// settings page that does, and adding it would cost a tenth query plus the
// widest scan in the repository for data the Block never draws. Calendar.Weather
// is therefore nil on every calendar this returns — stated rather than silently
// absent.
//
// The real-time seam is applied per calendar (C-REAL-CALENDAR-P2 F1): without
// it, a batch-loaded real-time calendar prints its stale stored date.
func (s *BlockService) EagerLoadCalendars(ctx context.Context, ids []string) (map[string]*Calendar, error) {
	out := map[string]*Calendar{}
	ids = blockDedupeIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		cal, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("eager load calendars: %w", err)
		}
		if cal != nil {
			out[cal.ID] = cal
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	return out, s.hydrate(ctx, out)
}

// EagerLoadCampaignCalendars is the Bench's entry point: every calendar in a
// campaign, hydrated, in ListByCampaignID's order (sort_order, name) — ONE
// listing query plus the eight batched sub-resource queries, instead of
// 1 + 9×N.
func (s *BlockService) EagerLoadCampaignCalendars(ctx context.Context, campaignID string) ([]Calendar, error) {
	cals, err := s.repo.ListByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("eager load campaign calendars: %w", err)
	}
	if len(cals) == 0 {
		return nil, nil
	}
	byID := make(map[string]*Calendar, len(cals))
	for i := range cals {
		byID[cals[i].ID] = &cals[i]
	}
	if err := s.hydrate(ctx, byID); err != nil {
		return nil, err
	}
	return cals, nil
}

// hydrate fills the sub-resources of an already-loaded calendar set. It is the
// single place the batched loaders are composed, so EagerLoadCalendars and
// EagerLoadCampaignCalendars can never drift in what "eager" means.
func (s *BlockService) hydrate(ctx context.Context, byID map[string]*Calendar) error {
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic arg order — easier query logs, stable tests

	months, err := s.repo.MonthsForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch months: %w", err)
	}
	weekdays, err := s.repo.WeekdaysForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch weekdays: %w", err)
	}
	moons, err := s.repo.MoonsForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch moons: %w", err)
	}
	seasons, err := s.repo.SeasonsForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch seasons: %w", err)
	}
	eras, err := s.repo.ErasForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch eras: %w", err)
	}
	cats, err := s.repo.EventCategoriesForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch event categories: %w", err)
	}
	festivals, err := s.repo.FestivalsForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch festivals: %w", err)
	}
	cycles, err := s.repo.CyclesForCalendars(ctx, ids)
	if err != nil {
		return fmt.Errorf("batch cycles: %w", err)
	}

	for id, cal := range byID {
		cal.Months = months[id]
		cal.Weekdays = weekdays[id]
		cal.Moons = moons[id]
		cal.Seasons = seasons[id]
		cal.Eras = eras[id]
		cal.EventCategories = cats[id]
		cal.Festivals = festivals[id]
		cal.Cycles = cycles[id]
		if s.realTime != nil {
			s.realTime.ApplyRealTime(cal)
		}
	}
	return nil
}

// blockDedupeIDs drops blanks and duplicates while preserving first-seen order.
func blockDedupeIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// --- UpcomingAcrossCalendars -----------------------------------------------

// BlockUpcoming is one row of the Bench's cross-calendar NEXT UP index.
type BlockUpcoming struct {
	Event Event
	// Calendar is the calendar the event belongs to, hydrated.
	Calendar *Calendar
	// Date is the occurrence date (recurrence expanded), in that calendar.
	Date BlockDate
	// DaysUntil is how many of THAT CALENDAR'S OWN days separate the occurrence
	// from that calendar's current date. It is the ordering key — see
	// UpcomingAcrossCalendars.
	DaysUntil int
}

// blockUpcomingHorizonYears bounds how far forward a recurring event is
// expanded when looking for its next occurrence. Two of the calendar's own
// years is far past any plausible NEXT UP row and keeps the scan finite for a
// recurrence whose end date has already passed.
const blockUpcomingHorizonYears = 2

// UpcomingAcrossCalendars is the Bench's NEXT UP index: the n soonest events
// across every calendar in a campaign, per viewer.
//
// THE ORDERING RULE (pinned by TestBlockUpcomingOrderingAcrossIncomparableCalendars).
// Two in-world calendars with unrelated epochs have NO natural cross-order:
// year 1523 of Harptos and year 218 of a Dwarven deep-count are not comparable,
// and neither is "month 3" against "month 3". The one quantity that is defined
// for both is HOW FAR AWAY the event is in its own calendar's days. So:
//
//	primary   : DaysUntil ascending — days counted in the event's OWN calendar
//	tie-break : the calendar's sort_order (the operator's own ordering)
//	then      : calendar name, then the occurrence date tuple, then event name,
//	            then event id
//
// The tail of that chain exists so the order is TOTAL and collation-independent:
// two servers, or one server and one MariaDB collation change, must not produce
// different NEXT UP lists. A day in a 10-day-week fantasy calendar is not a
// real-world day; the rule does not claim it is — it claims only that "sooner
// in its own world" is the honest cross-calendar proximity, which is exactly
// what the operator reads the index for.
//
// VISIBILITY. The existing UpcomingByCalendar path (service.go :914) filters
// base visibility in SQL ONLY and never calls filterEventsByUser, so an event
// carrying visibility_rules {allowed_users, denied_users} leaks its NAME to a
// player there. This does not repeat it: the batched read narrows in SQL, then
// the Go resolver runs once per calendar over that calendar's own rows.
func (s *BlockService) UpcomingAcrossCalendars(ctx context.Context, campaignID string, viewer BlockViewer, n int) ([]BlockUpcoming, error) {
	cals, err := s.EagerLoadCampaignCalendars(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if len(cals) == 0 {
		return nil, nil
	}
	// Calendar-level visibility gates the whole calendar before its events are
	// read — a dm_only CALENDAR must not contribute rows to a player's index.
	cals = filterCalendarsByUser(cals, viewer.Role, viewer.UserID)
	if len(cals) == 0 {
		return nil, nil
	}
	byID := make(map[string]*Calendar, len(cals))
	ids := make([]string, 0, len(cals))
	for i := range cals {
		byID[cals[i].ID] = &cals[i]
		ids = append(ids, cals[i].ID)
	}

	rows, err := s.repo.UpcomingEventsForCalendars(ctx, ids, viewer.Role)
	if err != nil {
		return nil, fmt.Errorf("upcoming across calendars: %w", err)
	}

	var out []BlockUpcoming
	for _, id := range ids {
		cal := byID[id]
		// THE ONE PASS, per calendar — rows[id] is compacted in place here and
		// is not read again.
		visible := filterEventsByUser(rows[id], viewer.Role, viewer.UserID)
		for i := range visible {
			if up, ok := s.nextOccurrence(cal, &visible[i]); ok {
				out = append(out, up)
			}
		}
	}
	blockSortUpcoming(out)
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// nextOccurrence finds an event's soonest occurrence at or after its calendar's
// current date, expanding recurrence through OccursOn.
func (s *BlockService) nextOccurrence(cal *Calendar, e *Event) (BlockUpcoming, bool) {
	if cal == nil || len(cal.Months) == 0 {
		return BlockUpcoming{}, false
	}
	base := cal.AbsoluteDay(cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
	year, month := cal.CurrentYear, cal.CurrentMonth
	limit := cal.CurrentYear + blockUpcomingHorizonYears
	for year <= limit {
		days := cal.MonthDays(month-1, year)
		start := 1
		if year == cal.CurrentYear && month == cal.CurrentMonth {
			start = cal.CurrentDay
		}
		for d := start; d <= days; d++ {
			if !e.OccursOn(cal, year, month, d) {
				continue
			}
			return BlockUpcoming{
				Event:     *e,
				Calendar:  cal,
				Date:      BlockDate{Year: year, Month: month, Day: d},
				DaysUntil: cal.AbsoluteDay(year, month, d) - base,
			}, true
		}
		month++
		if month > len(cal.Months) {
			month = 1
			year++
		}
	}
	return BlockUpcoming{}, false
}

// blockSortUpcoming applies the pinned cross-calendar ordering rule.
func blockSortUpcoming(rows []BlockUpcoming) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := &rows[i], &rows[j]
		if a.DaysUntil != b.DaysUntil {
			return a.DaysUntil < b.DaysUntil
		}
		as, bs := blockCalSortKey(a.Calendar), blockCalSortKey(b.Calendar)
		if as != bs {
			return as < bs
		}
		an, bn := blockCalName(a.Calendar), blockCalName(b.Calendar)
		if an != bn {
			return an < bn
		}
		if a.Date.Year != b.Date.Year {
			return a.Date.Year < b.Date.Year
		}
		if a.Date.Month != b.Date.Month {
			return a.Date.Month < b.Date.Month
		}
		if a.Date.Day != b.Date.Day {
			return a.Date.Day < b.Date.Day
		}
		if a.Event.Name != b.Event.Name {
			return a.Event.Name < b.Event.Name
		}
		return a.Event.ID < b.Event.ID
	})
}

func blockCalSortKey(c *Calendar) int {
	if c == nil {
		return 1 << 30
	}
	return c.SortOrder
}

func blockCalName(c *Calendar) string {
	if c == nil {
		return ""
	}
	return c.Name
}

// --- the sync pill ---------------------------------------------------------

// Sync pill states, from the signed contract's syncPill() map.
const (
	blockSyncStateOK    = "ok"
	blockSyncStateDrift = "drift"
	blockSyncStateBad   = "bad"
	blockSyncStatePause = "pause"
	blockSyncStateNone  = "none"
)

// blockSyncFacts is what CalendarLinkStatus resolved before it becomes strings.
type blockSyncFacts struct {
	State     string
	Linked    int
	Total     int
	Transport string
	PushedAgo string
	// Drift is the human magnitude of a date drift ("3 days"). Empty unless
	// State is drift.
	Drift string
}

// CalendarLinkStatus resolves the campaign's sync pill.
//
// WAVE-1 RULING (COMMON §6.3) — the NUMERATOR IS DEFINED, NOT QUERIED.
// sync_mappings has no `calendar` type and every syncapi calendar endpoint
// resolves the campaign DEFAULT calendar, so there is no per-calendar linkage
// to query. Linked is therefore 1 when a module is connected to the campaign
// (the campaign-default calendar is the one reachable) and 0 otherwise; Total
// is the number of calendars in the campaign. That is exactly what the signed
// "In sync · 1 of 4 linked" says. Inventing a per-calendar linkage here would
// be a fabricated number on a surface whose entire job is honesty.
//
// THE DENOMINATOR NEVER DROPS. Every failure path below keeps Total and lowers
// only the state — a pill that quietly shrinks from "1 of 4" to "1 of 1"
// because a probe timed out is worse than one that says "Not linked · 0 of 4".
//
// State "bad" (the signed "Incompatible structure · 12×10 ↔ 12×7") is NEVER
// emitted from here: Chronicle cannot see the module's own calendar structure,
// so it cannot know the two disagree. The module detects that and pauses on its
// side (Chronicle-Foundry-Module's structure comparison). The branch exists so
// a future probe that CAN report it has somewhere to land.
func (s *BlockService) CalendarLinkStatus(ctx context.Context, campaignID string) (calblock.SyncPill, error) {
	total := 0
	cals, err := s.repo.ListByCampaignID(ctx, campaignID)
	if err == nil {
		total = len(cals)
	}
	facts := blockSyncFacts{State: blockSyncStateNone, Total: total}
	if s.syncProbe == nil {
		return blockSyncPill(facts), nil
	}
	snap, perr := s.syncProbe.CampaignSyncSnapshot(ctx, campaignID)
	if perr != nil {
		return blockSyncPill(facts), fmt.Errorf("sync probe: %w", perr)
	}
	if !snap.Connected {
		return blockSyncPill(facts), nil
	}

	facts.Linked = 1
	facts.Transport = snap.Transport
	facts.State = blockSyncStateOK
	if !snap.LastSeen.IsZero() {
		facts.PushedAgo = "pushed " + blockHumanAgo(time.Since(snap.LastSeen)) + " ago"
	}
	if def, drifted := blockDateDrifted(cals, snap); drifted {
		facts.State = blockSyncStateDrift
		facts.Drift = s.driftMagnitude(ctx, def, snap)
	}
	return blockSyncPill(facts), nil
}

// driftMagnitude names the size of a drift ("3 days") in the default calendar's
// OWN days, or "" when it cannot be measured.
//
// ListByCampaignID returns SHALLOW rows with no Months, and AbsoluteDay needs
// them — so one batched months read happens here, and ONLY on the drift path.
// The common in-sync render pays nothing, and a failed read costs the magnitude
// rather than the state: "Drifted · 1 of 4 linked" is still true and still
// actionable, where a fabricated number would not be.
func (s *BlockService) driftMagnitude(ctx context.Context, def *Calendar, snap BlockSyncSnapshot) string {
	if def == nil || snap.AppliedYear == nil || snap.AppliedMonth == nil || snap.AppliedDay == nil {
		return ""
	}
	if len(def.Months) == 0 {
		months, err := s.repo.MonthsForCalendars(ctx, []string{def.ID})
		if err != nil {
			return ""
		}
		def.Months = months[def.ID]
	}
	if len(def.Months) == 0 {
		return ""
	}
	d := def.AbsoluteDay(def.CurrentYear, def.CurrentMonth, def.CurrentDay) -
		def.AbsoluteDay(*snap.AppliedYear, *snap.AppliedMonth, *snap.AppliedDay)
	if d < 0 {
		d = -d
	}
	if d == 0 {
		return ""
	}
	return blockHumanDays(d)
}

// blockDateDrifted reports whether the date the module says it APPLIED differs
// from the campaign-default calendar's current date, and returns that calendar
// so the magnitude can be measured separately.
//
// A module that has NEVER confirmed a date is connected, not drifted — calling
// it drift would flash a warning at every operator who has simply not pressed
// push yet. The comparison is an exact date-tuple match, which needs no month
// geometry and therefore works on the shallow rows ListByCampaignID returns.
func blockDateDrifted(cals []Calendar, snap BlockSyncSnapshot) (*Calendar, bool) {
	if snap.AppliedYear == nil || snap.AppliedMonth == nil || snap.AppliedDay == nil {
		return nil, false
	}
	var def *Calendar
	for i := range cals {
		if cals[i].IsDefault {
			def = &cals[i]
			break
		}
	}
	if def == nil {
		return nil, false
	}
	same := *snap.AppliedYear == def.CurrentYear &&
		*snap.AppliedMonth == def.CurrentMonth &&
		*snap.AppliedDay == def.CurrentDay
	return def, !same
}

// blockSyncPill turns resolved facts into the pinned SyncPill, emitting BOTH
// the full and compact strings. Tier is the renderer's business (CSS container
// queries choose which is visible: full tier → Full, std → Compact,
// mini/submini → neither), so the producer never decides which one to build.
func blockSyncPill(f blockSyncFacts) calblock.SyncPill {
	full, compact := blockSyncStrings(f)
	return calblock.SyncPill{
		State:     f.State,
		Full:      full,
		Compact:   compact,
		Linked:    f.Linked,
		Total:     f.Total,
		Transport: f.Transport,
		PushedAgo: f.PushedAgo,
	}
}

// blockSyncStrings builds the signed pill text.
//
// The signed strings (mockups/calendar-v4.html syncPill()) are:
//
//	ok    "In sync · Foundry · pushed 2m ago · 1 of 4 linked"  / "In sync · 1 of 4"
//	drift "Drifted · 3 days · 1 of 4 linked"                   / "Drifted · 1 of 4"
//	bad   "Incompatible structure · 12×10 ↔ 12×7"              / "Incompatible"
//	pause "Paused · date push paused — tracks real time"       / "Paused · tracks real time"
//	none  "Not linked · 0 of 4 linked"                         / "Not linked · 0 of 4"
//
// Emitted as PLAIN TEXT: the signed markup wraps parts in <b> and prefixes a
// status dot, and that is presentation the renderer owns. Segments whose data
// is absent (no transport name, no last-seen timestamp) are DROPPED rather than
// printed empty — "In sync · · pushed  ago · 1 of 4 linked" is the kind of
// half-rendered string that reads as a bug.
func blockSyncStrings(f blockSyncFacts) (full, compact string) {
	counts := fmt.Sprintf("%d of %d", f.Linked, f.Total)
	switch f.State {
	case blockSyncStatePause:
		return "Paused · date push paused — tracks real time", "Paused · tracks real time"
	case blockSyncStateBad:
		return "Incompatible structure", "Incompatible"
	case blockSyncStateDrift:
		parts := []string{"Drifted"}
		if f.Drift != "" {
			parts = append(parts, f.Drift)
		}
		parts = append(parts, counts+" linked")
		return strings.Join(parts, " · "), "Drifted · " + counts
	case blockSyncStateOK:
		parts := []string{"In sync"}
		if f.Transport != "" {
			parts = append(parts, f.Transport)
		}
		if f.PushedAgo != "" {
			parts = append(parts, f.PushedAgo)
		}
		parts = append(parts, counts+" linked")
		return strings.Join(parts, " · "), "In sync · " + counts
	default:
		return "Not linked · " + counts + " linked", "Not linked · " + counts
	}
}

// blockHumanAgo renders a duration the way the signed pill does ("2m", "3h",
// "5d"). Sub-minute resolves to "now" rather than "0m", which reads as broken.
func blockHumanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "moments"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// blockHumanDays renders a drift magnitude ("1 day" / "3 days").
func blockHumanDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}
