// builder_handler.go — THE WIZARD'S THREE HANDLERS, and the security reviews
// they answer (C-CALV4-WIZARD-P13 §§7.1–7.4).
//
// ── EVERY VIEWER OF THIS SURFACE IS AN OWNER, AND THAT IS THE CHIP GATE ────
//
// All three routes are registered on the authed `cg` group with
// campaigns.RequireRole(campaigns.RoleOwner) — the identical stack and the
// identical floor the three existing creation doors ride. The `needs backend`
// chip is GM-FACING HONESTY COPY that never renders to a player
// (decisions/2026-07-27-needs-backend-audience.md), and on this surface that
// rule is satisfied BY CONSTRUCTION: there is no player render of the builder.
//
// THE FAILURE MODE THIS COMMENT PREVENTS is a later slice lowering the floor to
// Player+ "so co-DMs can look", silently shipping those chips into a player's
// markup. THE CHIP'S AUDIENCE GATE HERE IS THE ROLE FLOOR. If the floor moves,
// every chip in builder.templ must be re-audited in the same commit.
//
// ── §7.2  GET /campaigns/:id/calendars/builder ─────────────────────────────
//
// EXPOSES: a form. It reads NO campaign row — the mockup has no "you already
// have N calendars" line and this handler adds no such read. If one is ever
// wanted it goes through ListVisibleCalendars, never a raw repo call.
// WHO: authenticated · campaign access · calendar addon · Owner, all enforced
// by the group stack rather than by this file.
// WRITES: nothing. It is a GET and it is idempotent.
// ACCEPTS: ?step= only, validated against the nine station keys, and an unknown
// value is a 400 rather than a silent clamp ([LYR-4]'s reject-don't-drop).
// Prior answers ride hidden inputs, never the query.
// W5a: nothing per-calendar is resolved, so the visibility split is untouched
// and no new read path to any calendar exists.
// CSRF / EGRESS / ABUSE: a GET with no state and no fan-out. Nothing to add.
//
// ── §7.3  POST /campaigns/:id/calendars/builder/preview ────────────────────
//
// EXPOSES: only what the caller sent, re-rendered. IT PERFORMS NO READ OF ANY
// CAMPAIGN ROW AND WRITES NOTHING — see the doc comment on BuilderPreviewAPI,
// which says so in as many words because the next hand will want to "just look
// up the campaign's default calendar for a nicer preview".
// WHO: Owner, via the group stack.
// WRITES: NOTHING. It persists nothing at all. A POST that persists nothing is
// honest only if the code says so, so the code says so.
// WHY POST: a multi-month structure is a body, not a querystring — a
// querystring would put authored campaign content in access logs and a long
// custom calendar would not fit — and a POST gets CSRF for free from the header
// boot.js already attaches.
// XSS: templ escapes by default and every month/weekday/moon/season/era name is
// an operator-authored string rendered as TEXT. Asserted, not assumed:
// builder_handler_test.go round-trips a name containing a script tag.
// RESOURCE BOUNDS — THE REAL RISK: there is no ceiling on grid size anywhere in
// the geometry (blockRowCount computes from lead/days/weekLen with no cap), so
// every bound in builderLimits is enforced on the way IN, REJECT-DON'T-TRUNCATE,
// and each is cross-checked against what SetMonths/SetWeekdays/SetMoons already
// accept so the wizard cannot preview a calendar the terminal submit rejects.
// W5a: THE ROUTE TAKES NO calendar_id, NO entity_id AND NO HOST DESCRIPTOR, so
// it has no IDOR surface and cannot be asked to render an existing calendar.
// UPLOADS: the shipped 10 MB io.LimitReader cap is kept on BOTH transports.
// EGRESS: a draft is not campaign content and never becomes a row, so it enters
// no export and no AI-workspace DTO by construction.
// ABUSE: unauthenticated callers cannot reach it; an Owner can spend their own
// CPU. No fan-out, no email, no notification, no cross-user write. Rate limiting
// is NOT required and is deliberately not invented — the cell cap is the bound.
// PLUGIN ISOLATION: this handler lives in the plugin; the widget package gains
// no Echo import, no router knowledge and no campaign id.
//
// ── §7.4  POST /campaigns/:id/calendars/builder ────────────────────────────
//
// EXPOSES: a redirect — to the 10-tab editor, which is the surface L6 says the
// wizard is a front door TO, or to the V2 calendar for a real-life calendar,
// matching what CreateCalendar's own caller already does.
// WHO: Owner, via the group stack.
// WRITES: one calendars row plus its sub-resources, all in the campaign :id
// that RequireCampaignAccess already authorised. owner/created_by come from the
// SESSION, never from the body.
// ACCEPTS: the same draft as §7.3, RE-VALIDATED here — the preview's validation
// is not an authorisation and a caller can skip the preview entirely.
// ATOMICITY AND ROLLBACK: CreateCalendar then ApplyImport is two calls and
// ApplyImport is itself atomic. If the sub-resource apply fails, the
// half-created calendar is DELETED. A wizard that leaves a nameless
// half-calendar behind breaks its own "Create is one act" promise on the first
// failure, and that promise is the product.
// CSRF: rides global CSRF plus the header boot.js attaches.
// AUDIT: logged twice — created, then imported — matching ImportFromSetupAPI's
// shape exactly.
// EGRESS / ABUSE: creating a calendar is an existing capability at an existing
// role floor. Nothing new.
package calendar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/audit"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// builderDefaultPreset is what a fresh wizard opens on. It is the campaign
// demo calendar's own shape, and it is a CHOICE rather than a default: opening
// on Blank would make the first thing an author sees an empty form, which is
// the surface the wizard exists to replace.
const builderDefaultPreset = "harptos"

// ShowBuilder renders the wizard. §7.2.
//
// IT READS NOTHING. No calendar row, no campaign row beyond the context the
// middleware already resolved. A GET that reads nothing cannot leak anything.
func (h *Handler) ShowBuilder(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)

	step, ok := builderStationIndex(c.QueryParam("step"))
	if !ok {
		// REJECT, DO NOT CLAMP. A ?step= that quietly slid to Start would turn a
		// wrong bookmark into a wrong page with no complaint.
		return apperror.NewBadRequest("unknown wizard step")
	}

	draft, err := builderPresetDraft(builderDefaultPreset)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("load default preset: %w", err))
	}
	data := builderView(cc.Campaign.ID, middleware.GetCSRFToken(c), draft, step, false, 0)
	return middleware.Render(c, http.StatusOK, BuilderPage(cc, data))
}

// BuilderPreviewAPI re-renders the wizard from the posted draft. §7.3.
//
// IT PERFORMS NO READ OF ANY CAMPAIGN ROW AND IT WRITES NOTHING. Not "not yet"
// — by design. It takes no calendar_id, no entity_id and no host descriptor, so
// it cannot be asked to render an existing calendar and it has no IDOR surface
// at all. If you are here to "just look up the campaign's default calendar for
// a nicer preview": that is the change this sentence exists to stop.
//
// It answers in one of two shapes, chosen by what the caller did:
//
//	a STATION CHANGE, a PRESET PICK, a PANEL ACTION or a FILE → the whole shell,
//	    because the rail, the panel and the preview all change;
//	a DATA CHANGE (the debounced form `change`) → the preview surface alone, so
//	    the input being typed into keeps its focus and its caret.
//
// One route, two fragments, and NOT a fourth route — which is what keeps the
// wizard's whole route budget at three.
func (h *Handler) BuilderPreviewAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)

	draft, step, importer, pvMonth, wholeShell, err := builderReadForm(c)
	if err != nil {
		return err
	}
	if err := validateBuilderDraft(draft); err != nil {
		return err
	}

	data := builderView(cc.Campaign.ID, middleware.GetCSRFToken(c), draft, step, importer, pvMonth)

	// An uploaded file carries its own detection line and mapping table. It is
	// read through the SAME DetectAndParse an existing calendar's import uses,
	// under the SAME 10 MB cap, on both transports.
	if name, res, ok := builderReadUpload(c); ok {
		data.Detected = fmt.Sprintf("%s — %s", name, builderFormatLabel(res.Format))
		data.Mapping = builderMappingFor(res, data.Draft)
	}

	if wholeShell {
		return middleware.Render(c, http.StatusOK, BuilderShellFragment(data))
	}
	return middleware.Render(c, http.StatusOK, BuilderPreviewFragment(data))
}

// BuilderCreateAPI is the terminal act. §7.4.
//
// CREATE IS ONE ACT, and this function is what makes that true rather than
// advertised: on a sub-resource failure the half-created calendar is deleted,
// so a failed Create leaves the campaign exactly as it found it.
func (h *Handler) BuilderCreateAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()

	draft, _, _, _, _, err := builderReadForm(c)
	if err != nil {
		return err
	}
	// RE-VALIDATED FROM SCRATCH. The preview's validation is not an
	// authorisation and a caller can skip the preview entirely.
	if err := validateBuilderDraft(draft); err != nil {
		return err
	}
	if blocked := builderCreateBlocked(draft); blocked != "" {
		return apperror.NewValidation(blocked)
	}

	input := CreateCalendarInput{
		Mode:             draft.Mode,
		Name:             draft.Name,
		CurrentYear:      draft.Year,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
		LeapYearEvery:    draft.LeapEvery,
	}
	if epoch := strings.TrimSpace(draft.EpochName); epoch != "" {
		input.EpochName = &epoch
	}

	cal, err := h.svc.CreateCalendar(ctx, cc.Campaign.ID, input)
	if err != nil {
		return err
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarCreated, "calendar", cal.ID, cal.Name,
		map[string]any{"mode": draft.Mode, "via": "builder_wizard", "preset": draft.Preset})

	// A REAL-LIFE CALENDAR IS SEEDED BY CreateCalendar AND IS NOT IMPORTED
	// ONTO. Its structure comes from the wall clock — twelve Gregorian months,
	// seven weekdays, the AD epoch, year/hours/leap taken from `now` — and
	// ApplyImport would rewrite exactly those and force the date to 1/1, which
	// on a wall-clock-authoritative calendar is the freeze W8 exists to
	// prevent. So the real-life path applies no import at all; what the author
	// chose is the name and the timezone, and only the timezone is written.
	if draft.Mode == ModeRealLife {
		if zone := strings.TrimSpace(draft.TimeZone); zone != "" {
			enable := true
			if err := h.svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{
				Name:             cal.Name,
				EpochName:        cal.EpochName,
				CurrentYear:      cal.CurrentYear,
				CurrentMonth:     cal.CurrentMonth,
				CurrentDay:       cal.CurrentDay,
				HoursPerDay:      cal.HoursPerDay,
				MinutesPerHour:   cal.MinutesPerHour,
				SecondsPerMinute: cal.SecondsPerMinute,
				LeapYearEvery:    cal.LeapYearEvery,
				LeapYearOffset:   cal.LeapYearOffset,
				SetRealTime:      &enable,
				RealTimeZone:     &zone,
			}); err != nil {
				return builderRollback(ctx, h, cal, err)
			}
		}
		return c.Redirect(http.StatusSeeOther,
			fmt.Sprintf("/campaigns/%s/calendar/v2/%s", cc.Campaign.ID, cal.ID))
	}

	// THE SAME ApplyImport THE IMPORTER USES, from the same *ImportResult shape
	// the four parsers produce. One apply path, two front doors.
	res := builderImportResult(draft)
	if err := h.svc.ApplyImport(ctx, cal.ID, res); err != nil {
		return builderRollback(ctx, h, cal, err)
	}
	h.logCalendarAudit(c, cc.Campaign.ID, audit.ActionCalendarImported, "calendar", cal.ID, cal.Name,
		map[string]any{
			"format":   res.Format,
			"months":   len(res.Months),
			"weekdays": len(res.Weekdays),
			"moons":    len(res.Moons),
			"seasons":  len(res.Seasons),
			"eras":     len(res.Eras),
			"via":      "builder_wizard",
		})

	// The front door is the 10-tab structure editor — what L6 names literally,
	// what CreateCalendar's own caller already redirects to, and what the
	// Bench's `Builder →` already points at.
	return c.Redirect(http.StatusSeeOther,
		fmt.Sprintf("/campaigns/%s/calendars/%s/settings", cc.Campaign.ID, cal.ID))
}

// builderRollback deletes a half-created calendar and returns the original
// failure.
//
// THE PROMISE IS THE PRODUCT. "Nothing is written until Create, and Create is
// one act" stops being true the first time a failed apply leaves a nameless
// calendar behind — and the author's only evidence would be a stray row in a
// list. Same shape as api_handler.go's create-then-apply rollback.
func builderRollback(ctx context.Context, h *Handler, cal *Calendar, cause error) error {
	slog.Error("builder: create failed after the calendar row existed; rolling back",
		slog.String("calendar_id", cal.ID), slog.Any("error", cause))
	if delErr := h.svc.DeleteCalendar(ctx, cal.ID); delErr != nil {
		// The rollback itself failed. Say so loudly: the campaign now has a row
		// the wizard did not intend, and a silent 500 would hide it.
		slog.Error("builder: ROLLBACK FAILED — a half-created calendar remains",
			slog.String("calendar_id", cal.ID), slog.Any("error", delErr))
	}
	return apperror.NewInternal(fmt.Errorf("create calendar: %w", cause))
}

// --- reading the form --------------------------------------------------------

// builderReadForm rebuilds the whole declaration from the posted body and
// applies whatever the caller just did to it.
//
// EVERY PREVIEW REBUILDS FROM THE BODY. There is no server-side draft, so this
// function is the only place draft state comes from, and a field that is not
// carried here is a field that silently resets — which is why
// builderCarryFields and this reader are written as one pair.
func builderReadForm(c echo.Context) (d *builderDraft, step int, importer bool, pvMonth int, wholeShell bool, err error) {
	form, ferr := c.FormParams()
	if ferr != nil {
		return nil, 0, false, 0, false, apperror.NewBadRequest("could not read the wizard form")
	}

	d = &builderDraft{
		Preset:    form.Get("preset"),
		Mode:      form.Get("mode"),
		Name:      form.Get("cal_name"),
		EpochName: form.Get("epoch"),
		Year:      builderAtoi(form.Get("year"), 1),
		TimeZone:  strings.TrimSpace(form.Get("tz")),
		Hue:       form.Get("hue"),
		Pattern:   form.Get("pattern"),
		Letter:    form.Get("letter"),
		LeapEvery: builderAtoi(form.Get("leap_every"), 0),
		LeapAdd:   builderAtoi(form.Get("leap_add"), 0),
		LeapName:  form.Get("leap_name"),
		LeapAfter: form.Get("leap_after"),
		LeapNote:  form.Get("leap_note"),
	}
	d.HollowSwatch = form.Get("hollow") == "1"
	if d.Mode == "" {
		d.Mode = ModeFantasy
	}

	// The month list is ONE ordered slice and the three parallel arrays are
	// index-aligned by construction: builderCarryFields emits exactly one entry
	// per month per family, and the owning station emits the name visibly.
	names, days := form["m_name"], form["m_days"]
	inter, leapDays := form["m_inter"], form["m_leapdays"]
	for i := range days {
		m := builderMonth{Days: builderAtoi(days[i], 0)}
		if i < len(names) {
			m.Name = names[i]
		}
		if i < len(inter) {
			m.Intercalary = inter[i] == "1"
		}
		if i < len(leapDays) {
			m.LeapDays = builderAtoi(leapDays[i], 0)
		}
		d.Months = append(d.Months, m)
	}
	d.Weekdays = append(d.Weekdays, form["wd"]...)

	mn, mp, ma := form["moon_name"], form["moon_period"], form["moon_newat"]
	for i := range mn {
		m := builderMoon{Name: mn[i]}
		if i < len(mp) {
			m.Period = builderAtof(mp[i], 0)
		}
		if i < len(ma) {
			m.NewAt = builderAtof(ma[i], 0)
		}
		d.Moons = append(d.Moons, m)
	}

	sn, sc, scn := form["season_name"], form["season_color"], form["season_cname"]
	sm, sd, sem, sed := form["season_sm"], form["season_sd"], form["season_em"], form["season_ed"]
	for i := range sc {
		s := builderSeason{}
		if i < len(sn) {
			s.Name = sn[i]
		}
		s.Color = sc[i]
		if i < len(scn) {
			s.ColorName = scn[i]
		}
		if i < len(sm) {
			s.StartMonth = builderAtoi(sm[i], 0)
		}
		if i < len(sd) {
			s.StartDay = builderAtoi(sd[i], 0)
		}
		if i < len(sem) {
			s.EndMonth = builderAtoi(sem[i], 0)
		}
		if i < len(sed) {
			s.EndDay = builderAtoi(sed[i], 0)
		}
		d.Seasons = append(d.Seasons, s)
	}

	en, ec, ecl, ecn, ey := form["era_name"], form["era_code"], form["era_color"], form["era_cname"], form["era_year"]
	for i := range ecl {
		e := builderEra{Color: ecl[i]}
		if i < len(en) {
			e.Name = en[i]
		}
		if i < len(ec) {
			e.Code = ec[i]
		}
		if i < len(ecn) {
			e.ColorName = ecn[i]
		}
		if i < len(ey) {
			e.StartYear = builderAtoi(ey[i], 1)
		}
		d.Eras = append(d.Eras, e)
	}

	step, ok := builderStationIndex(form.Get("step"))
	if !ok {
		return nil, 0, false, 0, false, apperror.NewBadRequest("unknown wizard step")
	}
	importer = form.Get("importer") == "1"
	pvMonth = builderAtoi(form.Get("pv_month"), 0)

	// A PRESET PICK REPLACES THE WHOLE DECLARATION, which is what a preset is.
	if key := form.Get("wz_preset"); key != "" {
		next, perr := builderPresetDraft(key)
		if perr != nil {
			return nil, 0, false, 0, false, apperror.NewBadRequest("unknown preset")
		}
		d, importer, pvMonth = next, false, 0
		if next.Mode == ModeRealLife {
			// The real-life card short-circuits stations 1–7 exactly as the
			// importer door does: the wall clock owns the structure, so the only
			// remaining questions are on Review.
			step = len(builderStations) - 1
		}
		return d, step, importer, pvMonth, true, nil
	}
	if form.Get("wz_importer") == "1" {
		return d, 0, true, pvMonth, true, nil
	}
	if key := form.Get("wz_step"); key != "" {
		next, sok := builderStationIndex(key)
		if !sok {
			return nil, 0, false, 0, false, apperror.NewBadRequest("unknown wizard step")
		}
		return d, next, false, pvMonth, true, nil
	}
	if act := form.Get("wz_act"); act != "" {
		pvMonth = builderApplyAct(d, act, pvMonth)
		return d, step, importer, pvMonth, true, nil
	}
	// No verb: this is the debounced data change, and only the preview moves.
	return d, step, importer, pvMonth, false, nil
}

// builderApplyAct applies one panel action. Every bound here is the SAME bound
// validateBuilderDraft enforces, so a control can never step past what the
// validator would then refuse.
func builderApplyAct(d *builderDraft, act string, pvMonth int) int {
	verb, idx, dir := builderParseAct(act)
	switch verb {
	case "mdays":
		if idx >= 0 && idx < len(d.Months) {
			d.Months[idx].Days = builderClamp(d.Months[idx].Days+dir, 0, builderLimits.MaxMonthDays)
		}
	case "month-add":
		if len(d.Months) < builderLimits.MaxMonths {
			d.Months = append(d.Months, builderMonth{Name: "New month", Days: 30})
		}
	case "interc-add":
		if len(d.Months) < builderLimits.MaxMonths {
			d.Months = append(d.Months, builderMonth{Name: "New festival", Days: 1, Intercalary: true})
		}
	case "month-del":
		// THE LAST MONTH IS NEVER REMOVED. A calendar with no months is a state
		// the Block draws honestly, but it is not a state a delete button should
		// be able to produce by accident.
		if idx >= 0 && idx < len(d.Months) && len(d.Months) > 1 {
			d.Months = append(d.Months[:idx], d.Months[idx+1:]...)
		}
	case "week":
		next := builderClamp(len(d.Weekdays)+dir, builderLimits.MinWeekdays, builderLimits.MaxWeekdays)
		for len(d.Weekdays) < next {
			d.Weekdays = append(d.Weekdays, fmt.Sprintf("D%d", len(d.Weekdays)+1))
		}
		if next < len(d.Weekdays) {
			d.Weekdays = d.Weekdays[:next]
		}
	case "leap-every":
		d.LeapEvery = builderClamp(d.LeapEvery+dir, 0, builderLimits.MaxLeapEvery)
	case "leap-add":
		d.LeapAdd = builderClamp(d.LeapAdd+dir, 0, builderLimits.MaxLeapAdd)
	case "leap-add-rule":
		d.LeapEvery, d.LeapAdd = 4, 1
	case "moon-add":
		if len(d.Moons) < builderLimits.MaxMoons {
			d.Moons = append(d.Moons, builderMoon{Name: "New moon", Period: 30, NewAt: 0})
		}
	case "season-add":
		if len(d.Seasons) < builderLimits.MaxSeasons {
			d.Seasons = append(d.Seasons, builderSeason{Name: "New season"})
		}
	case "season-del":
		if idx >= 0 && idx < len(d.Seasons) {
			d.Seasons = append(d.Seasons[:idx], d.Seasons[idx+1:]...)
		}
	case "era-add":
		if len(d.Eras) < builderLimits.MaxEras {
			d.Eras = append(d.Eras, builderEra{Name: "New era", StartYear: 1})
		}
	case "pv-prev", "pv-next":
		// THE PAGER WALKS MONTHS, NOT ROWS. A festival day is a Month with
		// IsIntercalary in the same ordered list; the Block draws it as a band
		// attached to the month it follows, so stopping on one would show the
		// same month with a different number beside it.
		if n := len(d.Months); n > 0 {
			delta := 1
			if verb == "pv-prev" {
				delta = -1
			}
			for i := 0; i < n; i++ {
				pvMonth = ((pvMonth+delta)%n + n) % n
				if !d.Months[pvMonth].Intercalary {
					break
				}
			}
		}
	}
	if pvMonth >= len(d.Months) {
		pvMonth = 0
	}
	return pvMonth
}

// builderParseAct splits "verb:index:dir". The index is optional and empty
// means "not a row action".
func builderParseAct(act string) (verb string, idx, dir int) {
	parts := strings.Split(act, ":")
	verb = parts[0]
	idx = -1
	if len(parts) > 1 && parts[1] != "" {
		idx = builderAtoi(parts[1], -1)
	}
	if len(parts) > 2 {
		dir = builderAtoi(parts[2], 0)
	}
	return verb, idx, dir
}

// builderReadUpload reads an uploaded or pasted calendar under the SHIPPED
// 10 MB cap, on both transports.
//
// It returns ok=false when nothing was sent, which is the ordinary case: the
// importer station renders its pre-drop state and says so rather than showing
// an empty mapping table.
func builderReadUpload(c echo.Context) (name string, res *ImportResult, ok bool) {
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return "", nil, false
	}
	src, err := file.Open()
	if err != nil {
		return "", nil, false
	}
	defer func() { _ = src.Close() }()

	data, err := io.ReadAll(io.LimitReader(src, builderLimits.MaxUploadSize))
	if err != nil || len(data) == 0 {
		return "", nil, false
	}
	parsed, err := DetectAndParse(data)
	if err != nil {
		return "", nil, false
	}
	return file.Filename, parsed, true
}

// --- assembling one render ---------------------------------------------------

// builderView is the single place a BuilderViewData is built, so every render —
// page, shell fragment and preview fragment — computes the same facts from the
// same draft.
func builderView(campaignID, csrf string, d *builderDraft, step int, importer bool, pvMonth int) BuilderViewData {
	if pvMonth < 0 || pvMonth >= len(d.Months) {
		pvMonth = 0
	}
	data := BuilderViewData{
		CampaignID:   campaignID,
		CSRFToken:    csrf,
		Draft:        d,
		Step:         step,
		Importer:     importer,
		PreviewMonth: pvMonth,
		Presets:      builderPresets,
		Carry:        builderCarryFields(d, step, importer),
		Blocked:      builderCreateBlocked(d),
		Fault:        builderFaultFor(d),
		Checks:       builderChecksFor(d),
		Stats:        builderStatsFor(d),
		Block:        builderPreviewBlock(d, pvMonth),
	}
	if p, ok := builderPresetFor(d.Preset); ok {
		data.Identity = p
	}
	return data
}

// --- tiny readers ------------------------------------------------------------

func builderAtoi(s string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return n
}

func builderAtof(s string, fallback float64) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return fallback
	}
	return f
}

func builderClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
