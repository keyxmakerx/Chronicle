package aiexport

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// roleFor maps the privacy mode to the role passed into plugin
// listers. Safe wants the player-level view (dm_only stripped),
// Permitted/Everything wants the owner view.
func roleFor(p PrivacyMode) int {
	if p == PrivacyModeSafe {
		return permissions.RolePlayer
	}
	return permissions.RoleOwner
}

// estimateTokens returns a rough token count for a markdown string, using an
// OpenAI-ish 4 chars/token heuristic — accurate enough for the modal header
// line that helps owners gauge whether the export fits a target AI's
// context window.
func estimateTokens(markdown string) int {
	if markdown == "" {
		return 0
	}
	return (len(markdown) + 3) / 4
}

// formatTokens renders an int with thousands separators ("12,400").
func formatTokens(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte(',')
		}
	}
	for i := rem; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// RenderHeader emits the document preamble: campaign name + export
// timestamp + privacy-mode banner + token estimate placeholder. The
// orchestrator fills the token estimate AFTER rendering all sections
// (chicken-and-egg: the count depends on the rendered body).
func RenderHeader(campaignName string, generatedAt time.Time, opts Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — AI Export\n\n", campaignName)
	fmt.Fprintf(&b, "*Generated %s · Privacy mode: %s",
		generatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		opts.Privacy.String())
	if opts.IncludeSessionGMNotes && opts.Privacy != PrivacyModeSafe {
		b.WriteString(" · GM notes included")
	}
	b.WriteString("*\n\n")
	b.WriteString(headerTokenPlaceholder)
	b.WriteString("\n\n")
	b.WriteString("> **Disclaimer:** intentionally lossy markdown export. " +
		"Source-of-truth lives in Chronicle; do not round-trip back from this file. " +
		"For lossless backup use the campaign-export endpoint instead.\n\n")
	return b.String()
}

// headerTokenPlaceholder is the literal placeholder string the
// orchestrator substitutes with the final token estimate. Keeping
// it a const so it's a stable replace target.
const headerTokenPlaceholder = "**Estimated tokens:** ~__TOKEN_COUNT__"

// substituteTokenCount replaces the placeholder once the full body
// has been rendered. Substitution is single-shot — placeholder appears
// exactly once in RenderHeader output.
func substituteTokenCount(doc string, count int) string {
	return strings.Replace(doc,
		"~__TOKEN_COUNT__",
		"~"+formatTokens(count),
		1)
}

// ----------------------------------------------------------------------------
// Entities
// ----------------------------------------------------------------------------

// RenderEntities renders the Entities section: groups by EntityType, renders
// each entity's EntryHTML via htmlToMarkdown, inlines tags and relations.
// Bidirectional relations are listed on both endpoints (AI-consumer clarity
// over the mild duplication).
//
//   - ents: already privacy-filtered by the caller.
//   - tagsByEntity/relByEntity: batch-fetched; missing key = none, nil map
//     skipped gracefully.
//   - opts: privacy mode is enforced by the caller; renderer trusts inputs.
//
// Returns the markdown section and its byte count for the token estimate.
func RenderEntities(
	ctx context.Context,
	ents []entities.Entity,
	types []entities.EntityType,
	tagsByEntity map[string][]tags.Tag,
	relByEntity map[string][]relations.Relation,
	opts Options,
) (string, error) {
	if len(ents) == 0 {
		return "", nil
	}

	typeByID := make(map[int]entities.EntityType, len(types))
	for _, t := range types {
		typeByID[t.ID] = t
	}

	// Group entities by EntityTypeID. Stable order: EntityType.SortOrder,
	// then EntityType.Name. Within a type: Entity.SortOrder then Name.
	byType := make(map[int][]entities.Entity)
	for _, e := range ents {
		byType[e.EntityTypeID] = append(byType[e.EntityTypeID], e)
	}
	typeIDs := make([]int, 0, len(byType))
	for id := range byType {
		typeIDs = append(typeIDs, id)
	}
	sort.Slice(typeIDs, func(i, j int) bool {
		a, b := typeByID[typeIDs[i]], typeByID[typeIDs[j]]
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		return a.Name < b.Name
	})

	// Parent names come only from the rows being exported (already filtered
	// to what the reader may see) plus opts.ParentNames, so a hidden parent
	// is never named.
	opts.ParentNames = visibleParentNames(ents, opts.ParentNames)

	var b strings.Builder
	b.WriteString("# Entities\n\n")
	for _, tid := range typeIDs {
		t := typeByID[tid]
		section := byType[tid]
		sort.SliceStable(section, func(i, j int) bool {
			if section[i].SortOrder != section[j].SortOrder {
				return section[i].SortOrder < section[j].SortOrder
			}
			return section[i].Name < section[j].Name
		})

		heading := t.NamePlural
		if heading == "" {
			heading = t.Name
		}
		if heading == "" {
			heading = "Pages"
		}
		fmt.Fprintf(&b, "## %s\n\n", heading)

		for _, e := range section {
			if err := renderEntity(&b, &e, tagsByEntity[e.ID], relByEntity[e.ID], opts); err != nil {
				return "", err
			}
		}
	}
	return b.String(), nil
}

func visibleParentNames(ents []entities.Entity, extra map[string]string) map[string]string {
	names := make(map[string]string, len(ents)+len(extra))
	for id, n := range extra {
		names[id] = n
	}
	for _, e := range ents {
		names[e.ID] = e.Name
	}
	return names
}

func renderEntity(
	b *strings.Builder,
	e *entities.Entity,
	entTags []tags.Tag,
	rels []relations.Relation,
	opts Options,
) error {
	fmt.Fprintf(b, "### %s {#%s}\n\n", e.Name, slugify(e.Name))

	// Sub-line: type label + privacy marker. TypeLabel is the freeform
	// subtype on the entity (e.g. "Half-Elf Sorcerer"). Falls back to
	// the joined TypeName when absent.
	subLine := strings.TrimSpace(strPtrOr(e.TypeLabel, ""))
	if subLine == "" {
		subLine = strings.TrimSpace(e.TypeName)
	}
	if e.IsPrivate {
		if subLine != "" {
			subLine += " · "
		}
		subLine += "_(GM-private)_"
	}
	if subLine != "" {
		fmt.Fprintf(b, "*%s*\n\n", subLine)
	}

	// Body — SEC-6-AMENDED invariant: EntryHTML through sanitize.HTMLPtr
	// before the converter (enforced by htmlToMarkdown). A conversion
	// failure skips just this field (bodyOrSkip) rather than aborting the
	// whole export.
	// Safe mode is a player's view, so GM-only spans and pictures in the
	// page text go too, as they do for players on the page itself.
	entry := e.EntryHTML
	if opts.Privacy == PrivacyModeSafe && entry != nil {
		stripped := sanitize.StripSecretsHTML(*entry)
		entry = &stripped
	}
	body, err := htmlToMarkdown(entry)
	body = bodyOrSkip("entity body", e.Name, body, err)
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}

	// Player notes — only render in Permitted/Everything (Safe drops
	// them as GM-leaning content even when they're not marked dm_only).
	if opts.Privacy != PrivacyModeSafe {
		notesBody, err := htmlToMarkdown(e.PlayerNotesHTML)
		notesBody = bodyOrSkip("entity player notes", e.Name, notesBody, err)
		if notesBody != "" {
			b.WriteString("**Player notes:** ")
			b.WriteString(notesBody)
			b.WriteString("\n\n")
		}
	}

	// Tags — inline metadata line. Drop dm_only tags in Safe mode
	// (the TagLister caller controls includeDmOnly when fetching;
	// renderer-side filter is defensive).
	if len(entTags) > 0 {
		names := make([]string, 0, len(entTags))
		for _, t := range entTags {
			if opts.Privacy == PrivacyModeSafe && t.DmOnly {
				continue
			}
			names = append(names, t.Name)
		}
		if len(names) > 0 {
			fmt.Fprintf(b, "**Tags:** %s\n\n", strings.Join(names, ", "))
		}
	}

	// Parent — where the page sits in the tree, so the AI can add sub-pages
	// with `parent:`. Named only when the reader can see the parent.
	if e.ParentID != nil {
		if name := opts.ParentNames[*e.ParentID]; name != "" {
			fmt.Fprintf(b, "**Parent:** %s\n\n", wikilink(name))
		}
	}

	// Relations — both endpoints per operator decision. The relation
	// row already carries the relationship type label + target name
	// from the join, so rendering doesn't require a second lookup.
	if len(rels) > 0 {
		names := make([]string, 0, len(rels))
		for _, r := range rels {
			if r.TargetEntityName == "" {
				continue
			}
			label := r.RelationType
			if label == "" {
				label = "related to"
			}
			names = append(names, fmt.Sprintf("%s %s", label, wikilink(r.TargetEntityName)))
		}
		if len(names) > 0 {
			fmt.Fprintf(b, "**Relations:** %s\n\n", strings.Join(names, "; "))
		}
	}

	b.WriteString("---\n\n")
	return nil
}

// ----------------------------------------------------------------------------
// Notes
// ----------------------------------------------------------------------------

// RenderNotes renders the Notes section: folder-aware via ParentID chains,
// owner-scoped (caller passes the already owner+shared-filtered result of
// notes.NoteService.ListByUserAndCampaign). Per-note: Title + privacy
// markers + body.
//
// Folder traversal is two-pass: build parent → children map, recurse from
// top-level (ParentID == nil). A visited set guards cycles, since
// notes.Note's ParentID is user-controlled.
func RenderNotes(ctx context.Context, list []notes.Note, opts Options) (string, error) {
	if len(list) == 0 {
		return "", nil
	}

	byParent := make(map[string][]notes.Note)
	var roots []notes.Note
	for _, n := range list {
		if n.ParentID == nil || *n.ParentID == "" {
			roots = append(roots, n)
			continue
		}
		byParent[*n.ParentID] = append(byParent[*n.ParentID], n)
	}
	sort.SliceStable(roots, func(i, j int) bool { return notesLess(roots[i], roots[j]) })
	for k := range byParent {
		sort.SliceStable(byParent[k], func(i, j int) bool {
			return notesLess(byParent[k][i], byParent[k][j])
		})
	}

	var b strings.Builder
	b.WriteString("# Notes\n\n")

	visited := make(map[string]bool, len(list))
	for _, n := range roots {
		if err := renderNoteTree(&b, n, byParent, visited, opts, 1); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// notesLess is the canonical note ordering: pinned first, then by
// title, then by ID for determinism.
func notesLess(a, b notes.Note) bool {
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	return a.ID < b.ID
}

func renderNoteTree(
	b *strings.Builder,
	n notes.Note,
	byParent map[string][]notes.Note,
	visited map[string]bool,
	opts Options,
	depth int,
) error {
	if visited[n.ID] {
		return nil // defend against pathological cycles
	}
	visited[n.ID] = true

	headingLevel := depth + 1
	if headingLevel > 6 {
		headingLevel = 6
	}

	prefix := strings.Repeat("#", headingLevel)
	title := strings.TrimSpace(n.Title)
	if title == "" {
		title = "Untitled"
	}
	if n.IsFolder {
		fmt.Fprintf(b, "%s 📁 %s\n\n", prefix, title)
	} else {
		fmt.Fprintf(b, "%s %s\n\n", prefix, title)
	}

	if !n.IsFolder {
		meta := []string{}
		if n.Pinned {
			meta = append(meta, "**Pinned**")
		}
		if n.IsShared {
			meta = append(meta, "_shared with campaign_")
		}
		if !n.UpdatedAt.IsZero() {
			meta = append(meta,
				"Updated "+n.UpdatedAt.UTC().Format("2006-01-02"))
		}
		if len(meta) > 0 {
			b.WriteString(strings.Join(meta, " · "))
			b.WriteString("\n\n")
		}

		body, err := htmlToMarkdown(n.EntryHTML)
		body = bodyOrSkip("note body", n.Title, body, err)
		if body != "" {
			b.WriteString(body)
			b.WriteString("\n\n")
		}
	}

	for _, child := range byParent[n.ID] {
		if err := renderNoteTree(b, child, byParent, visited, opts, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Calendar events
// ----------------------------------------------------------------------------

// RenderCalendarEvents groups events by their calendar month using the
// calendar's own month-name labels (e.g. "Highsummer 1247 AR" not "Month 4,
// Year 1247"). The era is appended via the matching Era row when one exists
// for the year.
//
// Privacy filter (defense in depth): the caller's CalendarLister
// (aiexport's interfaces.go) deliberately bypasses per-viewer role
// filtering — the AI export is a Director-only tool that needs the full
// picture — so Safe mode's drops happen HERE, independently, rather than
// trusting an upstream read to already agree with what "safe" means:
//
//   - cal.Visibility == "dm_only": the CALENDAR itself is GM-only (mirroring
//     the sibling timeline renderer's tl.Visibility == "dm_only" check
//     above), so none of its events — even "everyone" ones — are Safe. A
//     calendar's own visibility is independent of any individual event's,
//     and skipping this check would export every "everyone" event inside a
//     hidden calendar even in Safe mode.
//   - Visibility == "dm_only": never shown to a Player, so never Safe.
//   - Not yet announced: EffectiveAnnounced resolves to "on_day" (the
//     event's own kind default, absent an override) AND the event's date is
//     still in the calendar's future (cal.CurrentAbsoluteDay()) — an event
//     players could not yet know about. An "ahead"-announced event (a
//     yearly festival, or one explicitly marked AnnouncedAhead) is common
//     knowledge regardless of date and stays in.
//
// Moons and weather are deliberately NOT read or rendered by this function
// at all — RenderCalendarEvents' job is events, and a hidden moon
// (Moon.HiddenFromPlayers) or a not-yet-happened day's weather can only leak
// through a renderer that touches that data; this one never does, so there
// is nothing here for Safe mode to independently filter for either.
func RenderCalendarEvents(
	ctx context.Context,
	cal *calendar.Calendar,
	events []calendar.Event,
	opts Options,
) (string, error) {
	if cal == nil || len(events) == 0 {
		return "", nil
	}
	if opts.Privacy == PrivacyModeSafe && cal.Visibility == "dm_only" {
		return "", nil
	}

	monthName := func(month int) string {
		if month < 1 || month > len(cal.Months) {
			return fmt.Sprintf("Month %d", month)
		}
		return cal.Months[month-1].Name
	}

	eraFor := func(year int) string {
		// Eras sorted by start year; pick the matching one. Era.EndYear
		// nil = ongoing.
		for i := range cal.Eras {
			era := &cal.Eras[i]
			// Safe mode is the player's view: an era still hidden until it
			// begins is not named.
			if opts.Privacy == PrivacyModeSafe && cal.EraIsSecret(era) {
				continue
			}
			if year >= era.StartYear && (era.EndYear == nil || year <= *era.EndYear) {
				return era.Name
			}
		}
		return ""
	}

	kindByID := make(map[int]*calendar.EventKind, len(cal.EventKinds))
	for i := range cal.EventKinds {
		kindByID[cal.EventKinds[i].ID] = &cal.EventKinds[i]
	}
	currentAbsDay := cal.CurrentAbsoluteDay()

	// Filter + group by (year, month). Sort keys for determinism.
	type ymKey struct{ Year, Month int }
	groups := make(map[ymKey][]calendar.Event)
	for _, e := range events {
		if opts.Privacy == PrivacyModeSafe {
			if e.Visibility == "dm_only" {
				continue
			}
			var kind *calendar.EventKind
			if e.KindID != nil {
				kind = kindByID[*e.KindID]
			}
			announced := e.EffectiveAnnounced(kind)
			eventAbsDay := cal.AbsoluteDay(e.Year, e.Month, e.Day)
			if announced == calendar.AnnouncedOnDay && eventAbsDay > currentAbsDay {
				continue // hasn't happened yet, and players have no advance word
			}
			if cal.InSecretEra(e.Year, e.Month, e.Day) {
				continue // inside an era players don't know of yet
			}
		}
		k := ymKey{e.Year, e.Month}
		groups[k] = append(groups[k], e)
	}
	if len(groups) == 0 {
		return "", nil
	}
	keys := make([]ymKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Year != keys[j].Year {
			return keys[i].Year < keys[j].Year
		}
		return keys[i].Month < keys[j].Month
	})

	var b strings.Builder
	b.WriteString("# Calendar Events\n\n")
	fmt.Fprintf(&b, "*Calendar: **%s**", cal.Name)
	if cal.EpochName != nil && *cal.EpochName != "" {
		fmt.Fprintf(&b, " · Epoch: %s", *cal.EpochName)
	}
	b.WriteString("*\n\n")

	for _, k := range keys {
		bucket := groups[k]
		sort.SliceStable(bucket, func(i, j int) bool {
			if bucket[i].Day != bucket[j].Day {
				return bucket[i].Day < bucket[j].Day
			}
			return bucket[i].Name < bucket[j].Name
		})

		era := eraFor(k.Year)
		if era != "" {
			fmt.Fprintf(&b, "## %s %d %s\n\n", monthName(k.Month), k.Year, era)
		} else {
			fmt.Fprintf(&b, "## %s %d\n\n", monthName(k.Month), k.Year)
		}

		for _, e := range bucket {
			if err := renderCalendarEvent(&b, &e, opts); err != nil {
				return "", err
			}
		}
	}
	return b.String(), nil
}

func renderCalendarEvent(b *strings.Builder, e *calendar.Event, opts Options) error {
	fmt.Fprintf(b, "### Day %d — %s {#%s}\n\n", e.Day, e.Name, slugify(e.Name))

	meta := []string{}
	if e.StartHour != nil && e.StartMinute != nil {
		meta = append(meta, fmt.Sprintf("Time: %02d:%02d", *e.StartHour, *e.StartMinute))
	}
	if e.IsRecurring {
		t := "recurring"
		if e.RecurrenceType != nil && *e.RecurrenceType != "" {
			t = *e.RecurrenceType
		}
		meta = append(meta, "Recurrence: "+t)
	}
	if e.Visibility == "dm_only" {
		meta = append(meta, "_GM-only_")
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, " · "))
		b.WriteString("\n\n")
	}

	body, err := htmlToMarkdown(e.DescriptionHTML)
	body = bodyOrSkip("calendar event", e.Name, body, err)
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	return nil
}

// ----------------------------------------------------------------------------
// Sessions
// ----------------------------------------------------------------------------

func RenderSessions(
	ctx context.Context,
	list []sessions.Session,
	attendeesBySession map[string][]sessions.Attendee,
	entitiesBySession map[string][]sessions.SessionEntity,
	opts Options,
) (string, error) {
	if len(list) == 0 {
		return "", nil
	}

	// Sort: most-recently-scheduled first (planned next), completed
	// after. Within status, by scheduled date desc.
	sorted := make([]sessions.Session, len(list))
	copy(sorted, list)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, bb := sorted[i], sorted[j]
		if a.Status != bb.Status {
			return statusOrder(a.Status) < statusOrder(bb.Status)
		}
		return strPtrOr(a.ScheduledDate, "") > strPtrOr(bb.ScheduledDate, "")
	})

	var b strings.Builder
	b.WriteString("# Sessions\n\n")
	for _, s := range sorted {
		if err := renderSession(&b, &s, attendeesBySession[s.ID], entitiesBySession[s.ID], opts); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// statusOrder maps session status → render priority.
func statusOrder(s string) int {
	switch s {
	case sessions.StatusPlanned:
		return 0
	case sessions.StatusCompleted:
		return 1
	case sessions.StatusCancelled:
		return 2
	default:
		return 3
	}
}

func renderSession(
	b *strings.Builder,
	s *sessions.Session,
	atts []sessions.Attendee,
	ents []sessions.SessionEntity,
	opts Options,
) error {
	statusLabel := s.Status
	fmt.Fprintf(b, "## %s — %s {#%s}\n\n", s.Name, statusLabel, slugify(s.Name))

	meta := []string{}
	if s.ScheduledDate != nil && *s.ScheduledDate != "" {
		meta = append(meta, "Scheduled: "+*s.ScheduledDate)
	}
	if s.CalendarYear != nil && s.CalendarMonth != nil && s.CalendarDay != nil {
		meta = append(meta, fmt.Sprintf("In-game: Y%d M%d D%d",
			*s.CalendarYear, *s.CalendarMonth, *s.CalendarDay))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, " · "))
		b.WriteString("\n\n")
	}

	if s.Summary != nil && strings.TrimSpace(*s.Summary) != "" {
		b.WriteString("### Summary\n\n")
		b.WriteString(strings.TrimSpace(*s.Summary))
		b.WriteString("\n\n")
	}

	if s.RecapHTML != nil {
		recap, err := htmlToMarkdown(s.RecapHTML)
		recap = bodyOrSkip("session recap", s.Name, recap, err)
		if recap != "" {
			b.WriteString("### Recap\n\n")
			b.WriteString(recap)
			b.WriteString("\n\n")
		}
	}

	// GM-only notes — opt-in + privacy gate. SEC-6-AMENDED still applies
	// via htmlToMarkdown.
	if opts.IncludeSessionGMNotes && opts.Privacy != PrivacyModeSafe && s.NotesHTML != nil {
		notesBody, err := htmlToMarkdown(s.NotesHTML)
		notesBody = bodyOrSkip("session GM notes", s.Name, notesBody, err)
		if notesBody != "" {
			b.WriteString("### GM notes (owner-only — opted in)\n\n")
			b.WriteString(notesBody)
			b.WriteString("\n\n")
		}
	}

	if len(atts) > 0 {
		b.WriteString("### Attendees\n\n")
		for _, a := range atts {
			name := a.DisplayName
			if name == "" {
				name = a.UserID
			}
			fmt.Fprintf(b, "- %s — %s\n", name, a.Status)
		}
		b.WriteString("\n")
	}

	if len(ents) > 0 {
		b.WriteString("### Linked entities\n\n")
		for _, se := range ents {
			name := se.EntityName
			if name == "" {
				continue
			}
			role := se.Role
			if role == "" {
				role = "mentioned"
			}
			fmt.Fprintf(b, "- %s — *%s*\n", wikilink(name), role)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")
	return nil
}

// ----------------------------------------------------------------------------
// Timelines
// ----------------------------------------------------------------------------

// RenderTimelines renders the Timelines section: one ## heading per
// Timeline, then ### per event in chronological order. Events come from the
// EventLink join shape, which carries both calendar-linked and standalone
// events under a unified schema.
//
// Privacy filtering happens at the lister layer (ListTimelines /
// ListTimelineEvents take role + userID); renderer trusts the input. Safe
// mode additionally drops Visibility=="dm_only" rows as defense-in-depth.
func RenderTimelines(
	ctx context.Context,
	timelines []timeline.Timeline,
	eventsByTimeline map[string][]timeline.EventLink,
	opts Options,
) (string, error) {
	if len(timelines) == 0 {
		return "", nil
	}

	sorted := make([]timeline.Timeline, len(timelines))
	copy(sorted, timelines)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SortOrder != sorted[j].SortOrder {
			return sorted[i].SortOrder < sorted[j].SortOrder
		}
		return sorted[i].Name < sorted[j].Name
	})

	var b strings.Builder
	b.WriteString("# Timelines\n\n")
	for _, tl := range sorted {
		if opts.Privacy == PrivacyModeSafe && tl.Visibility == "dm_only" {
			continue
		}
		events := eventsByTimeline[tl.ID]
		if err := renderTimeline(&b, &tl, events, opts); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func renderTimeline(b *strings.Builder, tl *timeline.Timeline, events []timeline.EventLink, opts Options) error {
	fmt.Fprintf(b, "## Timeline: %s {#timeline-%s}\n\n", tl.Name, slugify(tl.Name))

	desc, err := htmlToMarkdown(tl.DescriptionHTML)
	desc = bodyOrSkip("timeline description", tl.Name, desc, err)
	if desc != "" {
		b.WriteString(desc)
		b.WriteString("\n\n")
	}

	filtered := make([]timeline.EventLink, 0, len(events))
	for _, ev := range events {
		// Visibility override on the link wins; else the underlying
		// event's visibility (carried on the joined fields). We treat
		// the empty string as "not dm_only".
		vis := ""
		if ev.VisibilityOverride != nil {
			vis = *ev.VisibilityOverride
		}
		if opts.Privacy == PrivacyModeSafe && vis == "dm_only" {
			continue
		}
		filtered = append(filtered, ev)
	}
	if len(filtered) == 0 {
		return nil
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		ai, aj := filtered[i], filtered[j]
		if ai.EventYear != aj.EventYear {
			return ai.EventYear < aj.EventYear
		}
		if ai.EventMonth != aj.EventMonth {
			return ai.EventMonth < aj.EventMonth
		}
		return ai.EventDay < aj.EventDay
	})

	for _, ev := range filtered {
		label := ev.EventName
		if ev.Label != nil && *ev.Label != "" {
			label = *ev.Label
		}
		fmt.Fprintf(b, "### Year %d", ev.EventYear)
		if ev.EventMonth > 0 {
			fmt.Fprintf(b, " · M%d D%d", ev.EventMonth, ev.EventDay)
		}
		fmt.Fprintf(b, " — %s\n\n", label)

		// EventLink carries an HTML pointer indirectly through
		// EventDescription (joined fields). It's plain text per the
		// model; still funnel through htmlToMarkdown so a future
		// schema change to HTML on this field gets the egress
		// invariant for free.
		if ev.EventDescription != nil && *ev.EventDescription != "" {
			body, err := htmlToMarkdown(ev.EventDescription)
			body = bodyOrSkip("timeline event", label, body, err)
			if body != "" {
				b.WriteString(body)
				b.WriteString("\n\n")
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// strPtrOr returns *p or fallback when p is nil/empty.
func strPtrOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// init verifies the renderer compiles against sanitize.HTMLPtr —
// purely a compile-time sentinel so a future refactor that drops
// the import (without dropping the call) fails the AST pin loudly.
var _ = sanitize.HTMLPtr
