# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC — thin index, not a session log                        -->
<!-- Purpose: Cross-cutting project state + index of per-plugin .ai.md files. -->
<!-- Update: When release status / branch state / cross-cutting work changes. -->
<!-- ====================================================================== -->

## For humans

### What this file is

A thin index. It documents Chronicle's current high-level state (release version, active phase, cross-cutting items) and points at where per-plugin status lives — each plugin owns its own `.ai.md` per the convention in `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D2=(c)` + `2026-05-23-c-plugin-isolation-audit.md §2.3`.

### What this file is NOT

It is no longer a chronological session-recap log. Pre-2026-05-23 session recaps (51 numbered entries spanning ~135 KB) live in `.ai/archive/status-2026-04-25-pre-shrink.md`. Going forward, per-session deliverables are tracked by the dispatch workflow (`cordinator/decisions/2026-05-19-dispatch-workflow.md`): one report per dispatch in `cordinator/reports/chronicle/`, plus the PR itself.

If you're an AI session looking for "what shipped last week", read the Cordinator repo's `main` branch and grep `reports/chronicle/` by date — books and reports live on cordinator `main` now, not a working branch. If you're looking for plugin-specific architecture, footguns, or recent work on plugin X, read `internal/plugins/<X>/.ai.md`.

## For AI sessions

### calendar-v4 — R2-2b SHIPPED: the editor earns its chrome (2026-08-02)

**The operator's complaint, closed at last.** 2026-07-29, on the live client:
*"I'm unable to click and do anything… It should be a small card with a nice
unfolding animation, and then a bigger editor that forms via some animation."*
DAYCARD answered the first two thirds and gave one honest reason for the rest —
[DC-7] refused to invent a motion signature and escalated it. The operator
signed the carve-out on 2026-08-01, and `C-CALV4-EDITOR-R2b` is the answer to
the last four words.

**Three things and no fourth.** THE CHROME: the stage-2 mechanism keeps every
`data-de-*` handle and every request body while its controls are replaced — the
locked (hue · pattern · glyph) type rail, a real month grid whose week length is
DERIVED from the payload's own weekday names, the intercalary day as a full-width
row and a real date, the recurrence block, the visibility cards as real radios
with an Owner-only allow/deny roster, the tie pill with an entity picker, and a
live preview. THE MORPH: geometric, ONE named class, four properties
(`inline-size` · `block-size` · `translate` · `opacity`), the register's existing
tokens, no new duration, no scale, declared inside the ONE register section.
DRAG-CREATE: landed rather than severed, on all seven [DC-11] terms.

**THE RECURRENCE UNIT LIST WAS WRONG IN THREE DIRECTIONS AND ALL THREE ARE
CORRECTED.** The week unit is NOT invention and its chip came off — week-based
recurrence strides `WeekLength() × recurrenceWeeks(...)`, so `weekly` MEANS
every tenday on a ten-day calendar, and DAYCARD §5 and the mockup are both
wrong. `year` IS invention, the drawing offers it UNCHIPPED, and it does not
ship at all. And new to this slice: `every N months` degrades exactly the same
way, because `OccursOn`'s monthly branch ignores `RecurrenceInterval` entirely —
so that control is ABSENT for the month unit rather than chipped. There is
nothing there for a backend to add.

**[ER-5] WAS MEASURED AND THE MEASUREMENT DISAGREED WITH THE PREDICTION.** The
ruling expected a wide editor to re-create DAYCARD's occlusion blockers. It does
not: 61 day cells × 11 viewport widths × 6 candidate box widths, Ledger stacked
and docked, and **every** candidate holds 0 px² of overlap. What moves with width
is how often the popover falls to the signed desktop SHEET. The shipped 760px is
the largest that never sheets at or above the editor's own two-column
breakpoint, and the divergence from the drawing's ~1008px is arithmetic: a
1008px box cannot sit beside a 300px docked Ledger inside an 1180px measure.
`placeCard` was not re-opened.

**AND THE MEASUREMENT WENT STALE TWO STAGES LATER — the fix round's first
finding.** It was taken before the morph existed and not re-run. From the morph
stage the same probe FAILED at every width, up to 70,906 px² over a docked
Ledger, `clear=true`. Cause: `edClose` writes the reverse morph geometry as an
INLINE `inline-size` and `edHide` — the only thing that clears it — runs on a
timer `edShow` cancels, so a reopen inside 160ms measured the CARD's width for a
box the sheet sizes at 760. `edShow` now clears it before the box is measured;
`placeCard` is still untouched. The probe's own accounting was separated at the
same time (popover overlap gates; the signed desktop sheet is recorded; the
CROSS-BLOCK case is reported with a card control arm and booked as
`C-CALV4-CARD-CROSSBLOCK-LEDGER`). **A measurement is scoped to the commit it
was taken on.**

**THE FIX ROUND'S OTHER FIVE, IN ONE PARAGRAPH EACH, because every one of them
was GREEN and wrong rather than red and obvious.** (1) The greyscale proofs
declared `html{filter:grayscale(1)}` and the card and editor are `[popover]` —
top-layer, not painted as descendants of `<html>` — so the filter never reached
the surface under test; measured at 0.39% of editor pixels above chroma 20 with
a maximum of 255 while the caption said hue was removed. (2) The editor's
primary action shipped at **1.0:1**: `.cal-dayeditor .btn.fill` set `color:
var(--accent)` at the same specificity as the Bench's `background:
var(--accent)`, with the day card's sheet linked second, so `Create event` was
painted in its own fill. (3) The capture fixture seeded NO event categories, so
every shot photographed the module's `No type` fallback while its caption
claimed "the locked type rail" — and the producer was emitting no pattern
channel at all, making the type rail's three locked channels two. (4)
`TestDayCardCSS_EverySelectorIsScoped`'s comment claimed a literal check over a
`strings.Contains` prefix test; `.cal-daycard-drag` passed by accident and so
would a fourth root. (5) The carve-out monopoly guard iterated a hardcoded
eight-file list while claiming "product-wide"; adding `.edmorph` to
`gm_panel.js` left the whole package green. **Every one of them is now a
mechanical check with a named mutation that turns it red.**

**A GUARD THAT COULD NOT SEE ITS OWN SUBJECT.** The morph's close must remove
`.dcopen` before writing the reverse geometry or leaving takes as long as
arriving. Mutating that order left every end-state assertion green — it is an
ordering claim inside one task. The fixture's DOM stub grew an operation log and
the ordering is now a real assertion. A guard nudged until green stops proving
anything; a guard that was never able to see its subject never proved anything
at all.

**Carried, not closed:** `C-CALV4-DAYMENU` (the 10 `menus-*` stills travel with
it — a 22-of-32 fidelity split, never a shortfall); `C-CALV4-DAYPICK-A11Y`, now
also holding drag-create's missing keyboard equivalent; `C-CALV4-TOKENS-RESIGN`,
booked a **sixth** time, with two of the seven defects now visible in stills the
operator has signed; the live-authed CSRF case DAYCARD could not measure.

Books: ADR-048 §27 · `reports/chronicle/2026-08-02-C-CALV4-EDITOR-R2b.md` ·
`reports/chronicle/screenshots/2026-08-02-c-calv4-editor-r2b/`.

### calendar-v4 — W-H SHIPPED: the builder wizard, the last wave (2026-08-02)

**The remodel's last wave, and the only one whose gate was *finish*.**
`C-CALV4-WIZARD-P13` shipped `GET /campaigns/:id/calendars/builder` — nine
stations presented as Start plus eight steps, a five-card preset gallery, the
front door for the 4-format importer, and a live month preview that IS the
shipped Block rendered from an in-memory `*Calendar` that is never persisted.
Operator signature: `decisions/2026-08-01-operator-signatures-wz1-sky-editor.md`
("Signed — build P13"); the dispatch was 16/16 signed.

**Almost none of it is new capability, by design (L6).** The preset gallery is
the importer with four embedded payloads read through the SAME `DetectAndParse`
an upload meets — no preset table, no migration, no new parser, no second apply
path. The importer front door is the existing parser behind a route with no
`:calId`. What is new is the shell, the honesty states, and the motion.

**The three-card V1 setup chooser is RETIRED.** `GET /calendars/new` resolves to
the wizard, so every link across the product and every external bookmark lands
on the designed surface with no href edited in a file the wave did not own —
including the frozen `calendar_v2` helper. Three pins refreshed, none deleted.

**W-H's durable deliverable is a guard the motion policy asked for on
2026-07-27 and nobody had built**: a property/keyframe/reduced-motion/duration
budget over `static/css/calendar-builder.css`, landed BEFORE a line of that
sheet existed. Read ADR-048 §26 for what it can and cannot see — a comment
terminator inside prose silently disabled the entire motion register while all
six assertion families stayed green, and only a browser probe caught it.

**A FIX ROUND FOLLOWED, AND IT IS PART OF THE WAVE.** Adversarial verification
rejected the first pass on five blocking findings, every one of them found in a
browser against an entirely green suite: a moon-name input rendering 0px wide, a
"no turn this month" that was unconditional and false, an era name clipped
mid-word, a blank inside the leap station's flagship sentence, and three preview
features absent from the build while the copy asserted them. All five are fixed
and the evidence that missed them is widened — the narrow-lane probe now walks
all eleven station sheets at all seven widths and measures whether a control can
show its own value, and the `?step=` reject path is pinned on the GET as well as
the form. ADR-048 §26 records the three lessons.

**A SECOND FIX ROUND FOLLOWED IT, AND ITS FIRST FINDING IS THE WAVE'S WORST.**
An undisclosed edit had re-pointed `Index`'s zero-calendar branch at the wizard —
and `Index` is on the PUBLIC group behind `RequireViewAccess`, so a player AND an
anonymous visitor on a public campaign rendered the whole builder at 200, both
`needs backend` chips and the Create button included. §6.3's "every viewer of the
wizard is an owner, satisfied BY CONSTRUCTION" had only ever been true of the
three ROUTE REGISTRATIONS; a Go call does not read the route table. The floor now
travels with the handlers, a non-owner goes to V2's own empty state, and roles
are exercised by tests rather than asserted by a test's name. Also closed: the
fault sheet now draws the composition [WZ-15] item 5 ratified (the anchor stays,
the fault goes where the grid would be) instead of a headline claiming the date
cannot resolve above a Nameplate resolving it; the fidelity index lists
twenty-six differences instead of nine "exhaustive" ones (twenty-four when this
paragraph was first written; the third round added the one the second missed and
the one it created deliberately); §12.1(ii)'s five motion clips exist and the
stills are captured at rest and reproduce **content-identically, caption-glyph
rasterisation aside** — re-measured across two full regenerations, 39 of 42
byte-identical with `importer--dark`, `presets--mobile-light` and
`step-eras--dark` differing only inside rendered text runs, and WHICH three land
in that set varies between run pairs; and the importer, which had been parsing
the dropped file and then discarding it, now adopts it as the draft. ADR-048 §26
carries all of it.

*This sentence said "byte-for-byte" and "twenty-four" for one round longer than
it was true*, in the very entry whose round claimed every false evidence claim
had been corrected. The evidence index and the report were fixed and Chronicle's
own books were not, which is the same failure at a smaller scale: a correction
that does not travel to every place the claim was made has not been made.

**A THIRD ROUND CLOSED IT, ON FIVE COORDINATOR RULINGS.** Two of the five were
decisions rather than repairs, which is why they came from the coordinator and
not the executor. **The delay ladder [WZ-8] tabulates had never run**: the rule
sat before pass 2's `animation:` shorthand at equal specificity, the shorthand
reset `animation-delay` to `0s`, and every static guard passed because every
static guard asks whether the rule exists. It runs now (0 / 33.3 / 66.7 / 100 /
133.3ms, measured in Chromium — the dispatch's tabulated "≤132ms" is the same
rule with `--m-step` rounded to 33ms in prose, and the sheet now carries the
division rather than the rounding), pinned by a byte-ORDER assertion and a browser
probe, and it **deliberately diverges from the sealed mockup**, which carries the
identical defect — the written signed mechanism outranks the drawing's accident,
and the mockup's own one-line fix is booked, not taken. **The wizard's host layer
set gained `eras`** (ruling R3, a deliberate re-pin of [WZ-2c]): the preview
draws the era bands the signed stills draw, DEF is unchanged, and the preview's
disclosure note shrank to the two absences still true — pinned together, with the
stale phrases asserted gone by name. **The indigo-vs-amber CTA** is disclosed and
booked to `C-CALV4-TOKENS-RESIGN` rather than patched, the sibling wave's way.
**The preset ↔ `BuildExport` round trip** the acceptance list required exists and
found one asymmetry (a moon's colour), asserted exactly and booked. And the
evidence's own arithmetic now describes the photograph — plus one more defect the
regeneration surfaced: the two "reduced motion" stills were ordinary renders,
because the capture never asked Chromium for the preference the harness's media
query needed.

Books: ADR-048 §26 · `reports/chronicle/2026-08-02-C-CALV4-WIZARD-P13.md` ·
screenshots, clips + measurements under
`reports/chronicle/screenshots/2026-08-02-c-calv4-wizard-p13/`.
### HOTFIX — two independent defects the operator hit at once (2026-08-02)

Branch `claude/hotfix-boost-scripts-prefs-fk`, off the #584 merge. Both were
reported as one symptom ("the calendar page does nothing"), and both were
reproduced before anything was changed.

1. **Boosted navigation deleted the Bench's page scripts.** The App layout puts
   `{children...}` inside `<main id="main-content">`; every sidebar link is
   `hx-boost="true" hx-select="#main-content" hx-swap="innerHTML"`; `boot.js`
   sets `htmx.config.allowScriptTags = false`, at which point htmx's
   `makeFragment` REMOVES script tags from the swapped fragment rather than
   skipping them. `bench.templ` mounted `cal_visibility.js`,
   `calendar_permissions.js` and `calendar_daycard.js` at exactly that depth, so
   the Bench wired on a direct load and silently did not through the sidebar —
   dead day card, dead Permissions button, and nothing looking wrong, because
   `<link rel=stylesheet>` in the same region survives the same code path. Fixed
   by moving all three into the plugin body-script registry
   (`internal/app/routes.go` → `layouts.SetPluginBodyScripts` → `base.templ`),
   which emits outside the swapped region. **Class defect:** 29 more page-side
   `<script src>` tags across 16 templs remain; `tools/check-page-scripts.sh` +
   `tools/page-script-allowlist.txt` is a whole-tree ratchet (wired into CI,
   self-testing) that lets the count only shrink, and the sweep is booked as
   **C-HTMX-SCRIPT-SWEEP**.

2. **Every calendar preference write was a guaranteed FK violation.**
   `SetSidebarPinned` / `SetBlockLayers` / `SetBenchSections` inserted an empty
   `calendar_active.calendar_id`, which migration 006 declares NOT NULL and
   foreign-keyed to `calendars(id)` → errno 1452 → 500 → no `HX-Refresh` → a
   switch that visibly did nothing. InnoDB checks the FK on the attempted
   insert, so `ON DUPLICATE KEY UPDATE` never rescued it and this failed for
   every viewer. Invisible to CI because all existing tests mock the repository.
   The service now resolves a real calendar (reusing `resolveActiveCalendar`'s
   ladder) and passes it down; the conflict clause still touches only the
   preference column, so a preference write cannot move a chosen active
   calendar. Failures now map to a named `apperror` instead of an anonymous 500.
   **Real-DB proof is booked, not claimed** — no database runs in the build
   environment; see **C-PREFS-FK-INTEGRATION** in `.ai/todo.md`. The operator's
   instance is the live confirmation.

Both entries, with the full reasoning, are in `.ai/todo.md` §1 Critical; the FK
correction also amends `internal/plugins/calendar/.ai.md`'s C-CALV4-LAYERS-P9
section, which described the empty-string seed approvingly.

### calendar-v4 — W-G PART B SHIPPED: `/campaigns/:id/schedule` (2026-08-01)

**The operator's #1 feature has a page.** `C-CALV4-RSVP-P8` Part B was gated on
a drawing pass — the spec forbade building the Verdict, the Matrix, the Roster
or the Painter from its own prose — and the gate closed when the operator signed
`mockups/calendar-v4-schedule.html` on 2026-07-29 against the `wg-schedule-*`
stills. **Where that mockup and the W-G spec disagree, the mockup wins**; the
spec now carries a supersession note naming the sections it overrules.

- **Five surfaces, two role orders, one route.** Director: Verdict → Matrix →
  (Roster · Painter) → Answer. Player: (Answer · Painter) → Verdict → Matrix.
  SOURCE order, never CSS `order:`. `GET /campaigns/:id/schedule` at Player+ is
  the whole route budget — `routes_snapshot.txt` **723 → 724**, no migration.
- **No new write path.** The Painter writes through the scheduler's shipped
  availability PUTs (two of them, because `[This week only | Every week]` means
  two things and already has two tables), the Answer through P8A's event RSVP
  POST, the Nudge through P8B's `/calendar/ask`. Forking a second availability
  write would fork the composition invariant with it.
- **Rank 1 IS the Bench's derived window**, by construction: `benchRsvpPeakRun`
  is extracted and shared, and the oracle asserts both surfaces name the same
  day and the same hour.
- **Permission is absence, including in the prose.** A player's payload carries
  no lane, no other member's name, no chip — so there is no `if IsGM` in the
  markup at all. The one real bug the shots found was a SENTENCE: reason clauses
  computed over an absent lane map told a player that five of five people had
  ignored the question.
- **The door is the Bench's RSVP panel title**, which has read `RSVP · Schedule`
  since the signed contract drew it. Not the nav — WG-2's ruling keeps that
  pointing at `/availability` and books the retirement as its own slice. The
  Bench's 23 shot keys are byte-identical with the link in place.
- **The fidelity gate is pixels and it paid for itself five times**, catching
  twenty-two defects every string assertion had passed before the verdict round
  found three more (below). The third pass was mostly
  about SHAPE rather than words: every caption on the page shipped with the right
  sentences and none of the drawing's emphasis — five bold lead-ins gone, two of
  which ARE this surface's named honesty claims. Captions are a `[]ScheduleRun`
  model now, never a markup string. Also: the matrix caption was three paragraphs
  where the drawing returns one, the count lane ran its numeral beside its hour
  instead of over it, the matrix head named the week but not the visible band,
  and a member with no zone was drawn amber where the drawing draws it grey.
- **The fourth pass was not pixels at all — it was a PARSER**, and it found six
  the eye had signed off four times. Diffing the sealed `<style>` against the
  shipped sheets declaration by declaration (normalising the `.cal-schedule`
  scoping and whitespace) caught: every segmented control rendering its selected
  rung **PINK**, because `color-mix(in oklch, var(--surface-card) …, var(--accent))`
  mixes from an ACHROMATIC surface and oklch resolves the missing hue by the
  short arc — 349.2deg the wrong way round the wheel — where the drawing mixes
  into `transparent` precisely so it cannot; a `.say .badge` rule dropped from
  the MIDDLE of a run of narrow rules the sheet otherwise carries in order,
  leaving every chip ~27% oversized at 390; a drawn `width:auto` "tidied" into
  `min-width:0`, which is a no-op replaced by a real effect and collapsed the
  answer well until `19:00` and `in` collided; and a missing `gap`, a missing
  `:hover` and a `cursor: default` where the drawing writes `not-allowed`.
  **Screenshots show a page that looks plausible — a 27% oversized chip still
  looks like a chip, and a pink pill still looks like a selected pill.** Only a
  mechanical diff sees a rule that is simply absent.
- **A measuring instrument that differs from its subject reports its own
  defects as the subject's.** The harness padded 20px at every width where the
  product's `<main>` pads 12px below 768 — 8px, and the drawn narrow matrix has
  exactly 8px to spare, so the phone shot clipped a column the shipping page does
  not. Three harness lessons are now written down in
  `internal/plugins/calendar/.ai.md`: shoot with a driver (the Chrome CLI clamps
  `--window-size` to a 500px minimum), scroll a control into view before
  measuring its target, and **measure horizontal overflow per ELEMENT** — a
  nested `overflow-x:auto` container never contributes to
  `documentElement.scrollWidth`, so "the page does not drag sideways" was true
  while the matrix dragged inside its own panel.
- **The fifth round found a whole VIEW, and the parser found it again.**
  `.sc-ruler` and `.sc-head-row b` were absent from the shipped sheet; pulling
  that thread found that **`?zoom=day` was a live control that changed nothing** —
  it round-tripped the query and inked `aria-current` while `scheduleColumns` and
  `scheduleDayCount` never read it, so the Day rung returned the identical week
  matrix. At 390 the same rung is DISABLED in the drawing (`--text-muted`,
  `cursor: not-allowed`, `title="week zoom is forced at this width"`) and the
  build shipped a live link at every width, while `ScheduleToggle.Disabled` was
  set NOWHERE — leaving a doc comment, a templ branch and a CSS `:disabled`
  repair all describing a refusal that did not exist. All three are closed: the
  day view is built out of the week payload the Matrix already ships (no route,
  no query, no seam), and the refusal is a real disabled rung that a media query
  chooses between — the server cannot see a viewport, which is ADR-048 §13's own
  bound, so both rungs ship and exactly one is ever displayed. Two footnotes
  worth keeping: the sealed sheet DECLARES `.sc-ruler` and never renders it
  (`const ruler = DAYZOOM ? '' : '';`), so those 21 declarations are deliberately
  still not carried — a rule matching nothing is the defect that was just fixed,
  not its repair; and measuring the refused rung turned up four more drawn
  declarations the sheets never carried, growing every page-head control to a
  real touch target at 390.
- Report: cordinator `reports/chronicle/2026-08-01-C-CALV4-SCHEDULE-PARTB.md` ·
  ADR-048 gained a Part B section · shots in
  `reports/chronicle/screenshots/2026-08-01-c-calv4-schedule-partb/`.

### calendar-v4 — ROUND 2 OPEN · R2-2a (THE DAY ANSWERS ITS CLICK) SHIPPED (2026-07-31)

**`C-CALV4-DAYCARD` stages 1-2 closed the operator's loudest live-client
complaint:** *"I'm unable to click and do anything, it just selects the date and
nothing happens."* Both halves were mechanically true — the click sets a
visually-hidden radio and the CSS-only ANSWER ladder filters the docked Ledger
(quiet by design; where the `ledger` layer is off it emits no control at all),
and calendar-v4 had no way to create or edit an event anywhere.

- **The card** is the pointer-first answer ON TOP of the CSS-only one. One
  page-level `[popover]`, positioned from the clicked cell's rect, listing the
  day with the LEDGER ROW's own field set. **The shipped ladder is untouched.**
- **The agreement law is pinned in Go, at the producer, joined to the count
  oracle** (GM / Nissa / Bryn): the card's event-id set per day EQUALS the set
  the ladder leaves visible. One source, one viewer-filtered pass. A card
  showing one more event than the Ledger is a permission leak wearing a UI
  change's clothes.
- **The editor's MECHANISM** ships against the shipped, IDOR-closed event API:
  **zero new write routes.** Create/edit are `POST`/`PUT` (Scribe), delete is
  `DELETE` (Owner), all through `Chronicle.apiFetch`. Its full §5 chrome pass
  and drag-create split out as **C-CALV4-EDITOR-R2b** at a stage boundary,
  under the dispatch's pre-authorised split.
- **ONE new read route**, the whole budget: `GET
  /campaigns/:id/calendars/:calId/events/:eid`, authed `cg`, `RolePlayer` floor,
  literal path, the grid's own viewer filter inside. Hidden / filtered /
  `:calId`-mismatched / missing are **one branch, one body**.
  `routes_snapshot.txt` **722 → 723**, one addition, zero removals.
- **Motion CONSUMES R2-1's register rather than opening a second one**
  ([DC-6], first-lander clause). Two rules added by name inside
  `calendar-bench.css`'s existing register section and inside its single
  `prefers-reduced-motion: no-preference` wrapper, reusing all three `--disc-*`
  tokens. The allowlist was widened by zero bytes; what was added is the CLAIM
  that the card is inside it, failing in both directions.
- **The visibility mapper was EXTRACTED, not copied a third time** — `mode ↔
  {visibility, visibility_rules}` now lives once in
  `internal/plugins/calendar/static/js/cal_visibility.js`, and both the
  permissions modal and the day-card editor read it.
- **Permission is absence, mechanically.** Every role gate is markup-level from
  the producer; a player's Bench contains no editor scaffold, no field name and
  no route string. The Block's DOM is asserted byte-identical before and after
  open + close.

**Known gap, stated rather than papered over:** where the Ledger is NOT docked a
day has no focusable control at all, so the card is pointer-only for that
viewer. No `tabindex` was injected (that is a Block mutation and a control the
server never rendered). Booked as **C-CALV4-DAYPICK-A11Y**.

**Fix-forward round 1 (stages 3-5), against the slice's adversarial review:**

- **The occlusion report reached no consumer** (DC-CLEAR-1). `placeCard`
  computed a `clear` flag; two comments and a commit body promised an
  unclearable geometry would be "visible rather than silent"; nothing read it.
  The one condition [DC-3] signs as a STOP-AND-FLAG shipped as the quietest
  thing on the page — the "sentence promising a guard that never ran" class
  R2-1's stage 9 existed to kill. The mobile branch was additionally returning
  `clear: true` without ever consulting the Ledger's rect, while measuring
  8.5k-54k px² of overlap. `clear` is now MEASURED the same way in both
  branches, from the box that actually landed, and it has two readers:
  `data-dc-clear` on the card's root at every width (a report, guarded against
  ever being styled) and ONE console warning per session at desktop widths only.
- **The intercalary payload path was 0% covered** (DC-ICAL-2) while daycard.go
  claimed the mirrored key helpers were pinned "in either direction" — true of
  `slug-N`, false of `slug-iN`, because the signed fixture declares no
  intercalary month. Five tests against real intercalary months spliced into the
  oracle fixture, including two of UNEQUAL length, which is the shape the coords
  zip fails on. The shipped code was correct; the guard and the comment were not.
- **Two listeners reached `edSave` with no in-flight guard** (DC-SAVE-6). Single
  -fire rested entirely on `preventDefault` beating the submit button's
  activation; a capture-phase move or a branch reorder would have made every
  Save write twice, and a double-click did it already. Guarded at the WRITE, not
  the listener.

**Fix-forward round 2 (stages 7-8), against the second adversarial review:**

- **A rename un-repeated the event** (DC2-RECUR-DATALOSS, the blocker). The
  editor authors no recurrence in this stage and "preserved" it by saying
  nothing about it — but `is_recurring` is a VALUE-typed bool on the shipped
  `PUT` and the service assigns it unguarded ON PURPOSE ("false IS the value,
  not 'absent'"), so omission was a write of false. The nil-guarded
  `recurrence_type` / `interval` / `end_*` around it then survived, leaving the
  exact half-state C-CAL-RECURRING-PARTIAL-STATE-CLEANUP cleaned up once. The
  read route now HANDS BACK the three fields the write path clobbers and the
  editor sends them home; create mode still sends none, because for a new event
  false is the true value. The lesson is in ADR-048: *"the client round-trips
  what it does not offer" is true field-by-field, and its truth depends on the
  field's TYPE on the wire.* The remaining partial clients of that PUT are
  booked for the same sweep.
- **Two guards were green-but-blind, both by enumerating from a SAMPLE**
  (DC2-PAYLOAD-OMITEMPTY, DC2-SCOPEGUARD-LINEFORM). The payload law's key
  inventory came from marshalling a hand-written literal, so a ninth field
  tagged `omitempty` would have been invisible to "want exactly these eight
  keys"; the stylesheet's scope guard read only lines ENDING in `{`, so the
  sheet's 21 single-line rules were never examined. Both now derive from the
  definition — reflection over the type's json tags, a brace-scanner over the
  sheet — and both go red on the verifier's own mutations.
- **The W5a same-answer table was missing the ownership row**
  (DC2-W5A-OWNERSHIP). `GetEventAPI`'s header claims FOUR refusals are
  indistinguishable; only three were pinned, and the unpinned one is the branch
  that separates "this event is on a calendar you can see" from "one you
  cannot". Sixth row added, with both calendars resolvable inside the campaign
  so the ownership check is what actually fires.

**Fix-forward round 3 (stages 9-13), against the third adversarial review:**

- **The card covered the STACKED Ledger, deterministically, in the DESKTOP
  treatment** (DC3-STACKED-LEDGER-OCCLUSION-1, the blocker — [DC-3]'s own
  STOP-AND-FLAG). `placeCard`'s dodge was HORIZONTAL only, which is complete
  while the Ledger is a right-hand column and structurally impossible once the
  Bench stacks it full-width BELOW the grid: the clamp computed a negative limit
  and no-opped, and the vertical branch above it only ever flipped for viewport
  room. Between roughly **625px and 884px of `.cal-bench` content width** the
  card therefore landed on the band for every day and every viewer, measured at
  21,080 px² against the real render. The editor escaped only by accident — too
  tall to fit below, so its viewport flip happened to clear the band, which is
  the proof the missing dodge was available. `placeCard` now treats the Ledger's
  SETTLED RECT as an exclusion zone whatever its role in the layout, and tries
  below → above → the bottom sheet ([DC-3] bullet 4's own signed answer; no
  third geometry, no resizing). Same three widths now measure 0 px²,
  `data-dc-clear="1"`, zero warnings. The desktop sheet FALLBACK still warns,
  because the geometry running out is the signed condition even when the card no
  longer covers anything; the mobile sheet stays silent, as DC2-MOBILE-4 set it.
- **The suite had mislabelled the product's own layout as pathological.** The one
  negative placement case called the stacked-Ledger geometry "a pathological
  geometry: the column starts 40px in", which is why the hole survived two
  reviews. It is now positive regression coverage at the ~884px and ~944px
  boundaries, plus a case for the sheet fallback.
- **A driver was mounted without the global it reads** (PERM-JS-HARD-DEP-2).
  [DC-10]'s extraction left `calendar_permissions.js` depending hard on
  `window.ChronicleCalVisibility` with no fallback — correct, but it means the
  driver mounted alone wires nothing, and `app_dashboard.templ` mounted it alone
  on a page that is retained-but-unrouted. Mount dropped; absence pinned against
  the page that renders it; and a source-level guard now requires any template
  mounting the driver to mount `cal_visibility.js` FIRST.
- **The editor wrote to the id the SERVER echoed, not the door that was clicked**
  (EDIT-MODE-ID-FALLBACK-3). The same line decided PUT-vs-POST, so a record
  without an `id` would have turned an edit into a duplicate-creating POST.
  Edit mode now carries the door's id, Delete follows it, and one pure
  `writeTarget()` REFUSES an edit with no id rather than creating.
- **Guard B4 did not read the day card** (DC2-B4-GLOB-4, ruled). `*daycard*`
  added to the scope glob with two self-test rows — coverage widening only,
  mutation-verified in both directions.

**Fix-forward round 4 (stages 14-15), against the fourth adversarial review:**

- **The round-3 fix closed the short-box case and opened a larger one on the same
  code path** (DC3-DESKTOP-SHEET-OCCLUSION-R4, the blocker). The two-candidate
  dodge admitted the ABOVE position only when it fitted the viewport outright and
  sent everything else to a clamp that pins the box to the BOTTOM of the
  viewport — where the stacked band is. A box taller than the room above its day
  therefore never flipped, failed both candidates and took the DESKTOP SHEET,
  covering **100% of the Ledger** across the same 625-884px band, while warning
  that the geometry was impossible. It was not impossible, it was not attempted:
  a 481px box at top=8 ends at 489 and the band starts at 595. Reachable two
  ways with ordinary data — `+ New event` (a 420x400 editor: 107,604 px² at
  884px, a REGRESSION against the pre-round-3 module, which placed the same
  editor clear) and a day with 12+ events (≈379px). `above` is now CLAMPED into
  the viewport rather than dropped: the same clamping the below-candidate always
  had, in the direction this one prefers. Still two anchored candidates and one
  sheet; the card is still never resized; the sheet fallback is unweakened and
  is now the only thing its STOP-AND-FLAG speaks about. **The lesson is not
  round 3's:** the round-3 fix was verified on the case it was written for and
  not on the surface that case hands off to, and the module's own comment
  ("THERE IS NO THIRD GEOMETRY") foreclosed the placement that fixes both.
- **A harness fidelity gap hid the editor half for three rounds.** `daycard_dom`'s
  rect is a static all-zero and the card is the EDITOR's anchor, so every editor
  in the JS suite was placed at the viewport origin, where nothing can be
  occluded. The card's rect is now derived from the placement the module wrote,
  and the editor regression goes red without it.
- **The route record's field law was a deny-list**
  (GETEVENT-FIELD-BOUND-IS-A-DENYLIST). §8's "only fields the editor writes
  back" is pinned twice; the payload's half reads the type by REFLECTION, the
  route's half asserted six required keys and fifteen hand-written forbidden
  names, so an `owner_id` or an `attendees` would have passed in silence — the
  exact shape the payload's own guard was fix-forwarded away from one round
  earlier. Both halves now read the definition.

**The §12 screenshot gate is PART-EXECUTED** — the geometry rows are measured
headlessly against the real render (1232px clears the Ledger at 0 px²; the
625-884px stacked band clears it after rounds 3 and 4, for short boxes and tall
ones; 390px is the signed bottom sheet, recorded rather than mis-reported); the
rows that need a live authed session are still open. Both halves are in
`.ai/todo.md`. Round 3's disclosure lesson rides with it — **a geometry claim is
a claim about the widths it was taken at** — and round 4 adds the second axis:
it is also a claim about the BOX HEIGHTS it was taken at, and the surface the
card hands off to has a different one.

### calendar-v4 — ROUND 2 · R2-1 (THE REVEAL PASS) SHIPPED (2026-07-30)

**`C-CALV4-BENCH-R2` slice R2-1 closed the operator's three 2026-07-29 live-client
complaints.** Round 2 adds no data, no zone and no engine: it decides what a
viewer meets first, how wide it is allowed to be, and what a surface says when it
is closed.

- *"where are the menus" / "5-6 blocks of data before you get to the calendar"* →
  four Bench sections (`ribbon` · `rsvp` · `nextup` · `rows`) are now native
  `<details>`/`<summary>` disclosures, **closed by default at every width**, each
  stating one true line computed from the VIEWER's own payload. The two Blocks,
  `.phead`, `.sechead` and `.caption` never collapse — the subject of the page may
  not be inside a disclosure.
- *"so stretched out, especially the RSVP menu"* → `--bench-measure: 1180px` on a
  page that had no `max-width` at all, and the RSVP panel becomes a two-column
  grid at ≥1024px via `display: contents` with a **byte-identical DOM**. The
  `.benchblock` measures **1144px @1440 / 1180px @1920**, both clear of the
  Block's 900px full-tier floor: no silent demotion.
- *"on the entity it scrolls"* → the entity embed's seed drops from five keys to
  `["moons","eras","weeknums"]` — exactly the two keys that add a ZONE leave, the
  three inside the month stay. No widget file was edited; that is what [LYR-3]'s
  seed rule bought.

**Store:** migration 016 `bench_sections` on `calendar_active` — the THIRD
extension of that row, under the same PR #368 decision 007 and 014 both cite. It
holds the **CLOSED** set so `NULL` (never chosen) stays distinguishable from `''`
(closed nothing). **ZERO new routes:** the existing `POST
/campaigns/:id/calendar/prefs` grew one optional `section=` field and
`routes_snapshot.txt` is byte-identical. That branch answers 204 with **no**
`HX-Refresh` while `layers=` keeps it, and the asymmetry is documented where the
next hand will misread it.

**Motion:** the product's first signed animation family ships —
`cordinator decisions/2026-07-29-motion-disclosure-register.md`. Clip-reveal +
opacity on `::details-content`, 200ms open / 160ms close, easing READ from the
Block's own `--ease` ladder, and the whole rule block inside ONE
`@media (prefers-reduced-motion: no-preference)` so reduced motion is instant and
complete rather than shortened. `TestBenchCSS_NoMotionAtAll` was **inverted to an
allowlist, never deleted**, and now asserts `close < open` arithmetically.

**A three-wave-old warning was measured out.** The entity producer claimed
dropping `ledger` would break the full-tier column arithmetic. `ColWidth` /
`IsNamed` / `IsNamedCSS` have zero non-test callers, the density flip is a
`@container cal-cell` query, and the full-tier track is `minmax(0, 1fr) auto` —
an absent Ledger collapses its own track. The one real consequence is the
opposite of the fear: named columns now flip on at a **narrower** host
(1198px → 898px for a ten-day week). Pinned by
`entity_ledger_geometry_test.go`.

ADR-048 gained a **section** (there is no ADR-049) covering the disclosure
mechanism, §13 met for the SECOND time (a pattern, not an incident), the third
extension of `calendar_active`, and the three-way distinction between
permission-is-absence, needs-backend-omission and **compactness-is-a-choice** —
all three render as "less" and are indistinguishable in a screenshot.

**Verifier round, fixed forward (2026-07-30, stages 6–8; the history was
pushed, so nothing was amended).** Three findings, all in what the slice had
already shipped:

1. **The disclosure flip was speaking for the eight controls inside it.** htmx
   resolves `hx-vals` by WALKING ANCESTORS (`bn()` recurses to `parentElement`
   in `static/vendor/htmx.min.js`), and the flip has to ride on the `<details>`
   because `toggle` does not bubble — so `section=<key>` was silently appended
   to the player's RSVP trio, the owner's `Ask →` form and all five sort links.
   Benign (their handlers ignore unknown fields) and a live trap: a control
   inside a disclosure posting `layers=` would inherit `section=` and be
   rejected 400 by this slice's own "exactly one of" guard. **The key now rides
   the POST URL**, which nothing inherits; Echo reads it either way because
   `FormParams` → `ParseForm` merges query and body on a POST.
2. **The closed ribbon did not name the session it was holding.** A player
   receives three tiles and their ONLY RSVP answer control is inside that
   disclosure; the summary was built from the Today tile alone. A `session`
   clause now prints the tile's headline and the viewer's own standing
   ("Session 41 · You: in"), gated on the existing `NeedsBackend` marker so
   build status never becomes a sentence.
3. **A twisty comment described a rotation the sheet never shipped** — the same
   defect stage 4 existed to delete, arriving fresh. The mechanism is a content
   swap (▸ / ▾ on `[open]`); the comment now says so and `bench_test.go` asserts
   no rule naming `.disc`/`summary` declares `transform`.

**Second verifier round (2026-07-31, stage 9; also fixed forward).** The same
class a third and fourth time, which is why the fix is now a PIN over prose and
not another rewrite. `bench.templ`'s header still read "calendar-bench.css
defines no transition, no animation and no @keyframes, and its contract test
says so" — both clauses died in stage 3 — and `calendar-bench.css`'s RSVP header
read "the Bench sheet is under tools/check-v2-motion-discipline.sh". **That
second one is the dangerous half**: the guard scopes to
`internal/plugins/{calendar,timeline,ai_workspace,campaigns}` and filters to
`${scope}/*.templ` / `${scope}/*.css`, so it has never seen `static/css/` — and
line :50 of the SAME FILE says so correctly. The sheet contradicted itself, and
the false half named a CI net under the one allowlist [BR2-2] warned has no net.
`TestBenchProse_MotionClaimsMatchTheSheet` now derives both facts: it fails if
any sheet-wide motion denial survives while the sheet declares a `transition:`,
if the guard is named anywhere near this sheet without "does not police", or if
`bench.templ`'s header stops citing the register and the test that enforces it.

**⚠️ Operator, and it is flagged at the PR gate:** [BR2-4] diverges from the
literal instruction "desktop defaults open, mobile defaults closed". One
server-rendered `open` cannot vary by viewport (ADR-048 §13). What shipped is
closed-everywhere + CSS `order` putting the calendar above the ribbon at ≤640px.
It is cut-able at the gate.

**⚠️ No screenshots.** The §13 measurement gate could not be executed: this
environment has no browser and the Playwright chromium download failed. Every
geometric claim above is derived arithmetically from the shipped stylesheet and
shell, and pinned by tests. See the slice report for the full list of what a
live client should still confirm by eye.


### calendar-v4 — WAVE 3 TAIL · THE ASKING EMAIL SHIPPED (2026-07-29)

**`C-CALV4-RSVP-P8B` closed the operator's own 28 July directive and the last
`needs backend` chip inside the RSVP panel.** Chronicle can now ask a table for
its schedule.

**The route:** `POST /campaigns/:id/calendar/ask`, Scribe+, inside
`RegisterRSVPRoutes`' authed `cg` — **721 → 722, one addition, zero removals**.
Same role floor as the `rsvp-collection` toggle it is the re-send of, for that
toggle's own stated reason: enabling is the invite moment, so it must not be
reachable by the people being invited.

**One email, two sections.** The schedule ask leads — subject and primary CTA —
and needs no session, so it is valid in a campaign that has scheduled nothing.
The five EXISTING RSVP action links ride the same email when a collecting
session is resolvable AND the recipient may see it, re-minted through the
unchanged `MintActionTokens`. `renderScheduleAskEmail` is a sibling of
`renderInviteEmail` sharing lifted package-level helpers; the invite email's
output is byte-identical after the lift.

**The CTA is a plain deep link, and that is a correction to the booking, not a
reduction.** *"A tokened link to the availability grid"* is not buildable: the
grid is a 1,318-line client SPA over six authenticated JSON routes, so a token
would have to authenticate all six or mint a session from an emailed link. The
directive's one-time-token pattern is honoured by the RSVP links in the same
email. See ADR-048 §25(b) — it is the section a future slice will need most.

**Three auth defects repaired first, in their own commit** (stage 1, fenced to
destination preservation + sanitisation): `handleUnauthenticated` now carries
where you were going into `/login?redirect=…`; the login form carries it through
the POST as a hidden field exactly as register already did; and `Login` uses
`sanitizeRedirect` instead of a bare `HasPrefix(redir, "/")`, which accepted
`//evil.example`. The third was unreachable ONLY because of the second, and this
slice is what made it reachable.

**The rate limit is persisted.** Calendar migration `015_schedule_asks`: a 6h
per-campaign cooldown and a 24h per-recipient floor, plus a per-user 10/h
in-memory limiter on the route. The cooldown REFUSES the send; the floor SKIPS
that member, so a second ask after somebody joins mails the new member and
nobody else. Nothing is recorded when nothing was sent.

**The panel's `Nudge` is LIVE, in exactly one place.** Both session-tile Nudges
were RETIRED rather than lit — two live buttons mailing one roster from one page
is a double-send affordance. `Propose` keeps its chip and `ActionsWhy` shrank to
name it alone. The Bench's own chip enumeration went from four to three. The
`email not configured` state is a `.badge.warn`, never `.badge.need`.

**Still open and named:** the propose-from-window write path; ledger #3
(`HasPattern`) which stays Part B's; a plain Scribe has the ask capability and
no button, because the panel `.side` is GM-tier by WG-8; and **`C-NOTIFY-PREFS`
is newly booked** — this is the first Chronicle email a member can receive
repeatedly because somebody else pressed a button, and there are no notification
preferences anywhere in the product.

---

### calendar-v4 — WAVE 3 · W-F LANDED, THE LAYER SYSTEM IS REAL (2026-07-29)

**A default nobody can leave is not a default — and for three waves, nobody
could leave DEF.** `C-CALV4-LAYERS-P9` (W-F) built the per-viewer preference
store that `block_projection.go`, `entity_calendar_block.go`, `bench.go` and
`shelf.templ` had all been writing "does not exist" about, in the same words,
since wave 1.

**The store:** calendar migration `014` adds `block_layers VARCHAR(255) NULL` to
`calendar_active` — extending the row migration 007 already chose to extend
under PR #368 stop-and-flag #3, rather than minting a table. `NULL` means "never
chosen" and renders the HOST'S SEED, so every wave-1/2 screenshot stayed valid
on day one; `''` means "chose nothing" and renders a bare month. **The two are
different answers at every layer**, because collapsing them makes the bare month
unreachable and turns the default into a floor.

**The route:** `POST /campaigns/:id/calendar/prefs`, the calendar-v4 wave's ONLY
new route — **720 → 721, one addition, zero removals**. It answers **204 with
`HX-Refresh: true` and no body at all**, so the host page rebuilds every Block
through the handler that already owns `requireVisibleCalendar` and the W5a
split. Nothing about which calendar or whose entity is ever taken from a request
body, because there is no response to build. That single choice is why §12.1's
security review is short by construction rather than by argument.

**The switchboard is a top-layer `[popover]` with NO JS ANYWHERE** — opened
declaratively by `popovertarget`, eight rows, "of 8" for every role. Pin
amendment **r54** adds `LayerState.PersistURL` with
`HasSwitchboard == (PersistURL != "")` pinned rather than assumed, and both
failure modes are silent, which is why it is pinned.

**Two of the three chipped zones are FILLED, neither needing a new pin field:**
`legend` from r52's `Mark.AxisLabel` (type axis only; it JOINS the count oracle,
and a type whose every event is hidden from a viewer is absent from their legend
— not even a zero), and `moongraph` from r53's `MonthGeometry.Almanac` (one lane
per drawn body, no composite ever, the ceiling still declared once in the
Nameplate). `horizon` keeps its chip and the chip is now **role-gated** — a
player who enables the key gets nothing at all, because `needs backend` is
GM-facing copy and `display:none` is not absence.

**The screenshot gate fired twice and both were real.** The sheet anchored to
the ⋯ covered the docked Ledger's own "Event list" row (a control hiding its own
target — re-anchored to the instrument); and HOST-P3's predicted std collision
happened with a real legend and a real graph, answered per CTS-8 inside the
Block's own std geometry with the body scrolling and **no layer key dropped**.

**DEF is still `["moons"]` and neither host seed gained a key** ([LYR-7] — the
HOST-P3/BENCH-P4 bookings close as SUPERSEDED, because the switchboard IS the
reachability they wanted). **ADR-048 gains §20-§24**, and there is no ADR-049.

**Split out under named follow-ons:** the Filters engine
(`C-CALV4-FILTERS-P10`), the colour-by picker and the legend's other two axes
(`C-CALV4-AXIS-P11`, blocked on DATA — no `Mark.OwnerLabel`, no per-calendar
label), the horizon data and the `Reveal through` write
(`C-CALV4-HORIZON-P12`, with its own security review), and the Tonight retarget
plus the `.sp2`/`.almgrid` ladder extension (`C-CALV4-ANSWER-EXT`, W-B's files).
`moonstyle=words` is deferred un-numbered: three of L20's four sky values reduce
to layer keys, and `words` names a register that exists nowhere in Chronicle.

### calendar-v4 — WAVE 3 · W-G PART A LANDED (2026-07-28)

**The operator's #1 feature is backed, and the slice began by retracting a
claim we had signed.** `C-CALV4-RSVP-P8` **Part A** filled the Bench's signed
RSVP panel (`cv4:2205-2263`) from storage that had shipped long before wave 1
said it had not. Wave 1's honesty copy asserted, in code and in the page
caption, that *"there is no session entity, no RSVP table and no per-member time
zone"* — **all three false**, contradicted by calendar migration 013, sessions
migrations 002 and 004, and core migration 000001. A `needs backend` chip that
is wrong is a fabricated ABSENCE and is worse than no chip, because it reads as
a verified finding. **ADR-048 §15** records the miss and the rule it produces:
a chip is a preflight FINDING, and a finding names the migration or the route it
checked.

What Part A ships: per-member availability lanes (owner / co-DM only), the
anonymous density row (everyone's), a **derived** recommended window under a
permanent `derived · not stored` chip with a quorum refusal below three
members, the party-visible member table with roles, zone chips, per-member
local clocks and answers, and a **live** RSVP trio on the session tile. Its
gate is `bench_rsvp_oracle_test.go` — no stored aggregate reaches the surface,
and a departed member's stored row changes no number.

**Zero new routes, zero migrations, zero JS.** `internal/wire/routes_snapshot.txt`
is byte-identical at 720 lines: the trio posts form-encoded to the existing
Player+ RSVP endpoint and the handler branches on `middleware.IsHTMX`.
`014_` in the calendar plugin was still free for W-F, which took it.

**ADR-048 gains §15-§19**: the preflight miss, the permission law (answers are
party-visible, lanes are not), the retired two-value role vocabulary and
`.badge.gm`'s third signed string `co-DM`, the zone-abbreviation ruling and the
zone-less clock, and the no-stored-aggregate rule.

**Still open in wave 3:** W-F (`C-CALV4-LAYERS-P9`) took the route lane after
P8 sealed and is **DONE** — see the section above. **W-G Part B** (`/campaigns/:id/schedule` — the Verdict, the
Director's matrix, the Roster, the Painter) is GATED on a coordinator drawing
pass and is NOT built; `C-CALV4-RSVP-P8B` ("the asking email") is booked for the
reminder endpoint the Nudge control still lacks.

### calendar-v4 — WAVE 2 COMPLETE (2026-07-28)

**The Block's four zones are all REAL.** Wave 1 (P0, PB, P1, P2, P5, P3, P4) is
merged; wave 2 is done — W-B filled the docked Ledger (`C-CALV4-LEDGER-P6`) and
W-E filled the Shelf (`C-CALV4-SHELF-P7`) with Upcoming, Filters and the
Almanac. W-F (the layer system, the per-viewer preference store, the legend, the
horizon, the moongraph — and now the Filters engine) is wave 3.

`.ai/decisions.md` **ADR-048** records the whole architecture decision in ONE
place, by design: the widget/producer split, the CSS-only controls, the four
honesty idioms, the one-pass count rule, the motion budget, and — as §10-§14
rather than a competing ADR — the Almanac, why the grid's three-moon ceiling is
only legitimate because it exists, and the two bounds wave 2 discovered (a
server-rendered `checked` cannot vary by container query; a primitive shared
across zones needs every rule that names it to name its zone too).

The pinned render contract has been amended three times under numbered
decisions: r51 (tie/mark emission), r52 (the Ledger row's three fields) and r53
(the celestial register, **uncapped by MoonCap** — the one sanctioned place a
host-passed parameter is deliberately non-authoritative for a zone).

Two things want a coordinator eye rather than a fix, both booked in
`.ai/todo.md`: the std tier is tighter than either earlier reading (a filled
Shelf wants 166px in a 520px Block), and `.ltabs` still carries `Month` alone
because [S9]'s four-tab construction is not buildable while selection is
CSS-only.

### Current release + branch state

- **Release line:** 0.0.1 (Release Readiness completed 2026-04-25 — backup scripts + mariadb-client in image + deployment runbook)
- **Active phase:** Pre-launch cleanup — locked macro-sequence: Calendar → Docs-audit → Tech-debt → Bugs → Features. The ~2026-06-26 target launch date has elapsed; pre-launch cleanup continues (no new date set). PC-Claim all 4 stages complete (Foundry PR #64 merged). Sidebar/nav consolidation (C-NAV-V3, both PRs) shipped as the last major structural arc; see `cordinator/plans/2026-05-21-master-plan.md` for the prior phase definition, current dispatch queue drives day-to-day priorities.
- **Coordinator artifacts (books, reports, dispatches):** live on the Cordinator repo's `main` branch — no dedicated working branch anymore.
- **LANDED (2026-07-28), branch `claude/coordinator-handoff-stage-3-3d3s4w`:**
  `C-CALV4-SEAM-P5` is **complete** — stages 1–15, one commit per stage, each
  green on the full `-short` suite. The durable deliverable is the **seam
  suite** (`internal/plugins/calendar/block_seam_test.go`): real fixtures
  projected through `projectBlock` and rendered through `calblock.Block`,
  asserted on the HTML. The discipline it encodes: assertions about what the
  PRODUCER chose to emit live producer-side; widget-side tests only pin
  renderer behaviour on hand-written `BlockData`.
  - Stages 1–3 (`18a84b8`/`ae9405d`/`71bfc99`): the amended **r51** pin (four
    identity fields `int64`→`string`, `Mark.Tied`,
    `MonthGeometry.MoonsDeclared`), the tie toggle as **ink, not membership**
    (measured cost: 1 byte), the calendar **identity triple** reaching the DOM.
  - Stages 4–13 closed everything the RESUME listed open: §3.3 the dm_only
    **dogear** (`Restricted` is the discriminator: false→notch,
    true→diamond), §3.4 the overlapping `MoreCount` (both foot-line sites),
    §3.5 `.band.half`'s stray border-right, §3.6 `needs backend` chips gated
    on their flags, **layer gating** (3 of 8 `LayerState` keys gated nothing;
    the widget fixtures were pinning the defect), §4 calendar-level
    visibility on `Block()`/`EventsForDay()` (a hidden calendar answers
    byte-identical to a missing one), §5 recurring-event dedupe at
    `candidateEvents`, §6 the lint scanner's templ-conditional blind spot +
    guard B4's `data-cell`/`data-row` subjects, §7 the three review fold-ins
    (timeline `visibility_override` folded into `EventCount`,
    `nextOccurrence`'s loop-invariant base hoisted,
    `EventsForEntityFiltered` promoted onto the `CalendarService`
    interface). Dated detail: `internal/plugins/calendar/.ai.md` +
    `internal/widgets/calendar_block/.ai.md`.
  - **Stage 15 (2026-07-28) closed the booked residual:** `buildMonthGeometry`
    populates `MonthGeometry.MoonsDeclared` from `Calendar.Moons` (hydrated by
    the block path's `MoonsForCalendars` batch read), and the Nameplate renders
    the r51 "3 of 4 moons" badge IFF the declared total exceeds the grid's
    `moonCap` ceiling (3) — a calendar declaring three or fewer states nothing
    extra, per the signed acceptance line. Seam-pinned through the real
    `BlockService.Block` path (`TestSeam_DeclaredMoonTotalReachesTheNameplate`)
    plus a widget-side render pin on hand-written `BlockData`.
  - `0c5f5f4` (same branch, not P5) `C-PERM-DMGRANT-REVOKE`: a removed co-DM
    kept owner-level visibility on **public** campaigns. Both the stored
    grant and the resolve-time gate are fixed. No migration.
- **LANDED (2026-07-28), same branch — `C-CALV4-HOST-P3` (wave 1, phase B):**
  the calendar-v4 Block now has its **first production caller**. Entity pages
  are the primary consumption surface (round-1 delta L3), so the entity calendar
  embed's month surface is `calendar_block.Block` projected through the P2
  spine, replacing the compact `adaptiveCalendarWidget`. Three commits.
  - `dcaaa4b` **the block host seam.** `BindingAffordanceFor` is the
    host-type-agnostic affordance (the old signature is unchanged and
    delegates; four widget blocks embed it), closing the deferred P3b gap where
    the picker query hardcoded `host_type=entity`. `BlockHost` gains
    `container-type: inline-size` **per widget type**
    (`DeclareInlineSizeHost`), not unconditionally — see the measured note
    below.
  - `fc2fe9c` **the tie toggle is live and pure CSS.** A hidden radio pair plus
    `:has()` changes the untied ink level; zero JavaScript (a `<script>` in an
    HTMX-swapped fragment never runs, `boot.js:163`) and zero routes. TWO ink
    levels now, per the signed `setTie()`: 0.28 in tied mode, 0.70 in whole —
    the sheet previously had one branch at the wrong level.
  - `0437fad` **the entity page hosts the Block.** Degrade ladder kept and
    extended: a calendar the viewer may not see renders **byte-identically** to
    no calendar at all (P5 stage 9's not-found shape, held at the host
    boundary). The linked-events list moved onto `EventsForEntityFiltered` —
    the loop it replaced applied only the event-level half of the filter. The
    entity page passes its own **layer set** (`moons, eras, weeknums, ledger,
    moongraph, shelf`), which is the mechanism the DEF ruling named; producer
    DEF stays `["moons"]`.
- **MEASURED, and it retracts a wave-1 assumption:**
  `container-type: inline-size` does **NOT** make an element a containing block
  for `position: fixed` / `position: absolute` descendants — `contain: layout`
  does. Chromium 141, three identical hosts: plain `(0,0)`, `container-type`
  `(0,0)`, `contain: layout` `(82,450)`. The C-CALV4-HOST-P3 dispatch warned
  the opposite (it would have trapped the maps block's `fixed inset-0` modals),
  and the warning reads correct from the spec text. Do not re-derive it from
  the spec; the reproduction is in
  `cordinator/reports/chronicle/screenshots/2026-07-28-c-calv4-host-p3/`.
- **`data.go` is byte-identical to the coordinator's pin and must never be
  `gofmt`-ed** — the pin is not gofmt-clean and formatting it breaks the
  match. Verify with `cmp` against `C-CALV4-BLOCKDATA.go.txt`.
- **Recent cross-cutting decisions** (most recent first):
  - 2026-07-24 — **Entity-tie visibility leak fix (C-CAL-ENTITY-TIES-LEAK-FIX, cordinator#32 gap #1 follow-up)**: `GET /campaigns/:id/calendars/:calId/events/:eid/entities` (RolePlayer-gated) returned every tied entity's name/type/icon/color with NO entity-visibility filtering — `EntitiesForEvent`/`EntitiesForEra` (`entity_ties_repository.go`) took only an ID, so the WHERE clause had nothing to gate on; a Player could read a dm_only/custom-restricted entity's NAME via any event or era it was tied to. The sibling `EntitiesForCalendar` (Calendars-dashboard associations panel, C-APPS-CAL-DASH-W1) was already hardened with `entityVisibilityFilter(role, userID)` — the original cordinator#32 audit missed these two. Fix: both methods now take `role, userID` and apply the same `entityVisibilityFilter`, entities re-aliased `e` (was `ent`) to match the filter's hardcoded column prefix. Threaded through `CalendarRepository`/`CalendarService` interfaces + `entity_ties_service.go`; `ListEventEntitiesAPI` sources the viewer context via `cc.VisibilityRole()` (co-DM/`IsDmGranted` counts as Owner) + `auth.GetUserID(c)`. `EntitiesForEra` has no production HTTP route yet — the signature change is forward-looking, no behavior change today. Updated the syncapi cross-plugin `CalendarService` test stub (`calendar_api_handler_test.go`) — the one other implementor, confirmed via `grep -r "EntitiesForEvent"`. **No live MariaDB in this sandbox** (confirmed: no docker daemon) for a literal query-level red→green repro; matched the test depth the precedent fix shipped with (fragment-level `TestEntityVisibilityFilter`, unmodified) plus new service-delegation tests (`TestEntitiesForEvent_ServiceDelegates`/`Era`) and a Player/Owner/co-DM matrix at the handler layer (`TestListEventEntitiesAPI_RespectsEntityVisibility`) + an equivalent service-layer matrix for era (`TestEntitiesForEra_VisibilityMatrix`), each driving a mock repo that models the real `role >= RoleOwner` threshold to prove the fix's plumbing end-to-end. `go build ./...` / `go test ./...` (45 packages, 0 fail) / `golangci-lint run ./...` all green. See `internal/plugins/calendar/.ai.md` §"Entity-tie visibility leak fix". Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security first), §T-O1 (verify before claim — the no-live-DB gap is stated plainly, not papered over).
  - 2026-07-21 — **Owner-only field visibility (C-FIELDS-OWNER-FILTER)**: a player-visibility audit found that a claimed player character's fields (e.g. Draw Steel's `backstory`) were visible to the WHOLE party — Chronicle's only field-level tier was `GMOnly` (GM-tier vs. everyone, #517/audit M-1), with no way to keep a field private to (the claiming player + GM) while still showing the rest of the party that the character exists. Added the missing tier: `FieldDefinition.OwnerOnly` (`model.go`), enforced by the new `Entity.IsOwnedBy(userID)` + `FilterOwnerOnlyFields`/`FilterRestrictedFields` (`gm_fields.go`, composes with the existing `FilterGMOnlyFields` — a GM-only field stays hidden even from the entity's own owner; an owner-only field is shown to the owner but not to other players). Wired into every field-data egress point identified by the original M-1 audit: entities-plugin `GetFieldsAPI`/`PreviewAPI` (`handler.go`), `CharacterSurfaceSchemaJSON` (`character_surface.go` + `character_surface_block.templ`, threaded a `userID` through `BlockRenderContext`), and syncapi's `GetEntity`/`ListEntities` (`egress_sanitize.go`/`api_handler.go`, the same path Foundry sync AND the Draw Steel character-sheet widget's client-side fetch both use). Convergence for existing campaigns mirrors the GM-flag reconciler exactly: `EntityService.SyncFieldOwnerOnlyFlags` (refactored the shared per-type walk out of `SyncFieldGMFlags` into `syncFieldFlag`, both call it now), `systems.FieldDef.OwnerOnly` manifest annotation, `preset_applier.go`'s `buildOwnerOnlyFlagsByCategory`/`reconcileFieldOwnerOnlyFlags` run at boot and on package install/update alongside the existing GM ones. Also added `data-is-owner` to the manifest-renderer widget mount (`show_renderer_registry.go`, mirrors the existing `data-is-gm`) and threaded `UserID` onto `EntityShowRenderContext`/`EntityShowPage`, so a system-package widget can gate UI the same way the GM Lore box already does. **Draw Steel side** (companion repo): `manifest.json`'s `backstory` field now carries `"owner_only": true` (its `gm_notes`/director-notes sibling was already `gm_only`); `character-sheet.js`'s Background row is now scheduled only `if (data.isGm || data.isOwner)` — a teammate no longer sees a misleading "No backstory yet." placeholder for a hero that actually has one, they see no box at all (same rationale as the pre-existing GM Lore gate — a permission gate, not a data gate). Tests: new table-driven cases mirroring every existing GM-only test 1:1 (`gm_fields_test.go`, `gm_fields_handler_test.go`, `character_surface_test.go`, `gm_fields_egress_test.go`, `show_renderer_registry_test.go`) plus the Draw Steel `node --test tools/` suite (unaffected, still 162/162). `go build ./...` / `go test ./... -short` (whole repo) / `golangci-lint run` all green. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security first — server is the authority, not the widget's client-side box-hiding), §T-O4 (single shared filter/reconciler, extended not forked, so the two tiers can't drift).
  - 2026-07-19 — **3 residual JS-attribute XSS sinks closed (C-SEC-XSS-JSATTR-SWEEP-R1)**: the follow-up to #560's mandated 56-site sweep, which found these 3 and deliberately left them unfixed to hold scope (the other 53 are documented safe). All three interpolated free text into an Alpine `x-data` JS string literal unescaped; each now routes through a plugin-local `jsEsc` (re-declared per plugin, §T-B2, no cross-plugin import). **(1) entities/form.templ `parentSelector` (HIGH, cross-user, stored):** a parent entity's `Name` flowed into `selectedName: '%s'` — any Scribe+ renames an entity to `');<payload>//`, sets it as another entity's parent, and the child's edit form executes it for any member who opens it → wrapped in the entities plugin's existing `jsEsc`. **(2) sessions/sessions.templ `editSessionModal` (HIGH, cross-user, stored):** `RecurrenceType` flowed into `recType: '%s'`, and `UpdateSession` (`service.go`) never validated it (the JSON PUT binds the body unchecked) so the "enum" was attacker-writable → executes for every isScribe viewer. Fixed BOTH layers: `UpdateSession` now rejects any non-nil `RecurrenceType` outside `{weekly,biweekly,monthly,custom}` (mirrors CreateSession's switch but unconditional — the sink reads it regardless of `IsRecurring`), plus a new sessions-local `jsEsc` at the sink. **(3) campaigns/settings.templ `settingsGeneralTab` (LOW, self-XSS):** `SystemID` (`custom:<url>` remainder unvalidated) flowed into `_savedSystemId: '%s'`; owner-only to set AND to view, so injector == only viewer — sink hardened with a campaigns-local `jsEsc`, no new `custom:`-validation machinery (per the dispatch, not worth it for a self-XSS). Per-sink regression tests (`parent_selector_xss_test.go`, `recurrence_type_xss_test.go`, `system_id_xss_test.go`) assert the hostile `');alert(1)//` family renders backslash-escaped (`\&#39;`, survives templ's HTML round-trip) while legit values are unchanged, plus the service-level `UpdateSession`-refuses-unknown-type test. `make templ` / `go build ./...` / `go test ./internal/plugins/{entities,sessions,campaigns}/...` / `golangci-lint run` all green. No new routes, no migrations, no cross-plugin imports. **Overlap note:** #560 was still open at write time and also adds a byte-identical campaigns `jsEsc` at the same location (and prepends to this same list) — trivial keep-one-copy resolution when both land; this branch is self-contained and green on fresh `main`. Report: `cordinator reports/chronicle/2026-07-19-c-sec-xss-jsattr-sweep-r1.md`. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security first — this *is* the tenet), §T-B2 (plugin isolation — `jsEsc` re-declared locally per plugin).
  - 2026-07-19 — **Reflected XSS on campaign Settings `?tab` closed (C-SEC-XSS-SETTINGS-TAB, audit SEC-1)**: the audit's lead T-B1 finding. `campaigns.Settings` read `?tab=` and passed it unvalidated into `CampaignSettingsPage`, which interpolated it into an Alpine `x-data` expression (`settings.templ` `fmt.Sprintf("{ tab: '%s' }", …)`). templ HTML-escapes the attribute, but the browser HTML-decodes it before Alpine `evaluate()`s it as JS, so `?tab=%27%29%3Balert(1)%2F%2F` (`');alert(1)//`) broke out of the string literal and ran attacker JS; CSP (`middleware/security.go`, `script-src 'unsafe-inline' 'unsafe-eval'` for Alpine) can't stop it, Settings is owner-gated (`routes.go` `RequireRole(RoleOwner)`), and the CSRF token is JS-readable (`csrf.go` `HttpOnly:false`) → owner-targeted account takeover from one crafted link. **Two-layer fix:** (1) `sanitizeSettingsTab(requested, tabs)` (`settings_tabs.go`) resolves `?tab=` against the already-role-filtered visible-tab set, falling back to the constant `"general"` on any empty/unknown/hostile value — an allowlist derived from the live `tabs` slice, so a viewer also can't pre-select a tab their role hides, and it removes the blank-tab-body side effect (the #5 seed: an unknown tab matched no `x-show`); (2) a local `jsEsc()` at the templ sink (mirrors entities' helper; re-declared locally to avoid a cross-plugin import, §T-B2) escapes `' \ \n \r` as defense in depth. Tests: `settings_tab_sanitize_test.go` (known tabs pass through; the exploit payload + 8 hostile/unknown inputs → `"general"`, asserting the sanitized value not the source text; a role-hidden tab is rejected for a player but reachable by an owner; `jsEsc` neutralizes the quote-breakout). `templ generate` + `go build ./...` + `go test ./internal/plugins/campaigns/...` + `go vet` green. Branch `claude/campaigns-settings-xss-fix-ynnr8v`. Report: `cordinator/reports/chronicle/2026-07-19-c-sec-xss-settings-tab.md`. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security-first — this fix *is* the tenet), §T-B2 (local helper, no cross-plugin coupling).
  - 2026-07-18 — **Restore drill kit (C-BACKUP-RESTORE-KIT)**: closes the beta plan's single largest data-loss risk (`cordinator/plans/2026-07-10-beta-transition-plan.md` §2 item 0.6) — a restore had never been verified, even once. **Step-0 finding:** backup/restore tooling already exists and is thorough (`scripts/backup.sh` + `scripts/restore.sh` + `make backup`/`make restore`/`make backup-list`, ADR-035/036/037) — no `backup-now.sh` companion needed, out of scope per the dispatch's own rule. **New:** `tools/restore-drill.sh` — one command, no flags for the happy path. Finds the newest backup via `docker compose exec` against the live `chronicle` app container (read-only `ls`/`cp`, same place `make backup-list` looks), spins up a throwaway MariaDB container (no host port, no shared network/volume, machine-generated name asserted against a reserved-name list so it can never collide with `chronicle`/`chronicle-db`/`chronicle-redis`), loads the dump, and checks: `schema_migrations` present/plausible (version vs. `ExpectedCoreMigrationVersion`, `dirty` must be 0), core tables (`users`/`campaigns`/`entities`) have rows (`calendar_events` checked too but a missing plugin table only warns, doesn't fail), and a spot FK check (`entities.campaign_id -> campaigns.id`, 0 orphans). Prints green `RESTORE DRILL: PASS` / red `FAIL: <why>`; a `trap`-based cleanup always removes the throwaway container. `--file <manifest-or-dump>` tests a specific backup instead (sha256-verified when a manifest is given). **Docs:** `docs/RESTORE-DRILL.md` (when to run, the one command, PASS/FAIL meaning, a FAIL-reason table, a clearly ⚠️-marked "restore FOR REAL" section pointing at the existing `scripts/restore.sh`/`make restore`) + cross-links added in `docs/deployment.md` §1/§8. **CI self-test:** `tools/test-restore-drill.sh` runs the real script against fixtures in `testdata/restore-drill/` (`good.sql` plus three FAIL fixtures — dirty migration flag, orphaned FK, empty core table — each proving its specific check actually fires, not just the happy path) via real disposable MariaDB containers; wired into `.github/workflows/ci.yml`'s `test` job (ubuntu-latest ships Docker). **Verification note (T-O1):** this sandbox's Docker registry blob fetches are blocked by org egress policy, so the full container lifecycle couldn't be pulled/run directly here; validated instead by (a) loading all four fixtures + running the exact verify queries against a locally-installed MariaDB 10.11, which caught a real bug — MariaDB's non-interactive client does not expand `\t` inside SQL string literals passed via `-e`, so `CONCAT(version,'\t',dirty)` round-tripped as the literal two characters `\`+`t`, not a tab byte, silently breaking the migrations-check parser; fixed by switching the delimiter to a literal `|`; and (b) a throwaway shim standing in for `docker run`/`exec`/`stop` (routed to the same local MariaDB) to exercise the real, unmodified script end-to-end — arg parsing, `--help`, manifest+sha256 verification (happy path and induced mismatch), missing-file/unknown-arg/no-compose-file preconditions, and all three FAIL fixtures — confirming exit codes and messages match design. CI (which has real registry access) is the first environment to run the real `docker run mariadb:latest` path — and it immediately caught what the local substitute couldn't: the real image ships only a `mariadb`-named client binary, not the `mysql` compat symlink an older locally-installed MariaDB 10.11 still carries, so every hardcoded `mysql -uroot ...` call failed with `env: 'mysql': No such file or directory`. Fixed by resolving the client binary once right after the healthcheck-ready loop (`mariadb` preferred, `mysql` fallback) and threading that through every subsequent call — exactly the kind of environment-specific gap this verification note exists to surface honestly rather than paper over. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-O1 (verify before claim — this entry states exactly what was and wasn't executed), §T-B1 (the drill never widens the live DB's attack surface: no host port, no shared network/volume, no live-service name reuse).
  - 2026-07-18 — **Embed event chips now navigate to V2 (C-CAL-EMBED-CHIPS-TAP, wave-8 #549 gate follow-up)**: the dashboard/entity-page calendar embed (`calendar.templ` `dayCell`) rendered event chips as `<button>` with `cursor-pointer` + a hover ring but no click handler anywhere in the embed context — the V1 scripts that once wired them were correctly deleted by #549 (C-CAL-V1-SUNSET), leaving a silently inert affordance. **Chose Option A (tap → navigate)**: chips are now `<a href>` links to `/campaigns/:id/calendar/v2/:calId?year=Y&month=M&day=D` (the SAME calendar the embed is bound to), cursored to the tapped day via the `year`/`month`/`day` query params `ShowV2` already parses (`handler_v2.go`) — no new route, `routes_snapshot.txt` byte-unchanged. Uses the day CELL's year/month/day, not the event's own (a recurring event's stored fields are its original creation date, not the occurrence being displayed). V2 surfaces and `event_grid.js` untouched — `calendarGrid`/`dayCell` have exactly one caller (the embed). Test: `embed_chip_nav_test.go` pins no-`<button>` + the exact href. `go test ./... -short` + `make test-js` (266 JS tests) + `golangci-lint run ./...` all green. See `internal/plugins/calendar/.ai.md` §"Embed event chips now navigate to V2". Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B3 (an inert button is the production-grade-UI smell this closes), §T-O2 (consumer-verified: read `handler_v2.go` ShowV2's query-param parsing before picking the href shape).
  - 2026-07-18 — **Timezone-list consolidation + two booked riders (C-TZ-CONSOLIDATION, RC-16.3)**: three hand-curated IANA timezone dropdown lists had drifted independently — `auth.commonTimezones()` (account settings) and `calendar.commonTimeZones()` (real-time calendar anchor) were byte-identical (the latter's own header comment already flagged the duplication as an intentional-for-now mirror pending a DRY pass), but `static/js/availability.js`'s `COMMON_TZ` (scheduler zone picker) carried a literal `"UTC"` entry neither Go list had. Consolidated to ONE canonical home: **`internal/timeutil/zones.go`** (`Zone{Value,Label}` + `CommonZones()`), the **UNION** of all three old lists (63 + `UTC` = 64 entries) — pinned by `zones_test.go` encoding the three old lists verbatim and asserting exact-union membership both directions, so no zone any surface previously offered can silently vanish. All three consumers now render from it: `account.templ` + `calendar_settings.templ` loop `timeutil.CommonZones()` directly (old `commonTimezones()`/`commonTimeZones()` deleted, `internal/plugins/calendar/timezones.go` removed entirely); `availability.js` no longer hand-rolls a list — it reads a `data-common-tz` JSON attribute (`timeutil.CommonZonesJSON()`) added to the availability page's EXISTING root div (`data-availability-root`, same surface as `data-tz`/`data-campaign-id` etc.) — **no new route**, `routes_snapshot.txt` unchanged (`internal/wire` test green). 4 new `test/js/availability_tz.test.mjs` cases pin the JS wiring (renders from the attribute, a zone outside it is never offered, missing/malformed attribute degrades to `[]` without throwing). **Rider 1 (view-pill accent, #541 deviation-3 booked follow-up):** the calendar V2 active view pill's App-accent fallback chain (`calendarV2ViewPill` → `var(--color-accent-app, var(--color-accent-surface-1, var(--color-accent, #6366f1)))`) turned out to have ALREADY shipped as a drive-by of an unrelated dispatch (commit `9d3b7954`, C-CAL-SKY-STRIP) — this PR adds the missing byte-identity-style pin test it should have shipped with (`calendar_v2_accent_pill_test.go`: active pill uses the App slot, inactive carries no accent style, markup is unchanged across unrelated context values). **Rider 2 (docs-only):** `internal/plugins/campaigns/.ai.md` "Campaign Customization" section — stale since C-ACCENT-TRIO/C-ACCENT-SLOTS, still described the pre-trio single-`accent_color`-column model — refreshed with the full 4-slot table (Site/Action/App/legacy-surface-pair), storage (`CampaignSettings` JSON fields, no migration), and the render/save mechanism. **r27 check (hour12:false ban):** grepped the touched surfaces + repo-wide; only pre-existing explanatory comments in `calendar_v2_rt_hint.test.mjs` (already-shipped h23 fix context) — no live violation, nothing to flag. `go build`/`go vet`/`go test ./...` + `make test-js` all green. Branch `claude/tz-list-consolidation-anwh99`. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-O1 (verified the old lists' exact contents via a diff script before writing the union, not from memory), §T-O4 (single source of truth — this IS the mechanism), §T-B4 (the `.ai.md` refresh).
  - 2026-07-18 — **Skybox multi-instance fix (C-SKYBOX-MULTI-INSTANCE, booked r23 from #543)**: a second simultaneous `skybox` widget mount (e.g. a dashboard block + the calendar sky pane open at once, both showing the same campaign) rendered STATIC — only its server-rendered first paint, no animation. Root cause verified via a failing-first headless repro (`test/js/skybox_multi_instance.test.mjs`, booted the REAL engine in a node vm with two independent `[data-cal-sky]` trees): `cal-almanac.js`'s engine was a page-wide singleton — `document.querySelector('[data-cal-sky]')`/`'[data-cal-worldstate]'` always resolve to the FIRST match, `SKY_SURFACE`/`SKY_FRONT`/`LAYER_CACHE` were single module-level vars, and `band.__calInited` made every init() call after the first a no-op — so a second band never got a `CalParticleEngine` surface at all. **Not** a boot.js double-mount issue (skybox.js's own widget mount/destroy lifecycle was already correct and untouched; the bug was entirely inside the shared engine's own DOM-binding). Fix: `SKY_BANDS` (array, one entry per live sky root) replaces the singletons; `bindSkyBands()`/`paintSkyBands()` bind + paint every `[data-cal-sky]` root (pairing each with its own canvas by document order — every `Skybox()` render emits exactly one canvas per root); `feedSkyEngine`/`effectLayersForCache` give each band its OWN `layerCache` (a meteor closure's live streak state is per-canvas, sharing it across differently-sized canvases would desync it); `renderDayPipeline`/`renderTimePipeline`/`applySunState`/`applyMoonDesigns`/`applyMoodTint`/`setBandSizeVars` all loop every live band instead of touching just the first. Still ONE shared `CalParticleEngine` rAF loop (bands just register additional surfaces into its existing list) and ONE `worldState` (per the #543 doctrine: engine reuse, one loop, per-instance layer state) — `SKY_SURFACE`/`SKY_FRONT`/`window.__calSkyEngine` stay pointed at the first-bound band for back-compat with existing single-band callers/tests. `init()`'s band-binding gate now distinguishes "engine already live + a new band mounted alongside it" (bind the newcomer, don't touch the survivor's worldState) from "every previously-bound band is gone" (a real boosted-nav swap — full re-teardown, unchanged from before). Pins (all in the new test file): both instances tick; destroying one doesn't stop the other; reduced-motion freezes both identically; the existing single-band swap + provider-self-destroy tests (`worldstate_reinit.test.mjs`, `skybox_widget.test.mjs`) are unchanged and still green. `make test-js` — 270/270 pass (266 pre-existing + 4 new). Widget JS only — no calendar `.templ`/route/migration changes (Go build not runnable in this sandbox — missing `templ` codegen tool, pre-existing environment gap unrelated to this diff). See `internal/widgets/skybox/.ai.md` (footgun #1 updated) for the mechanism writeup. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B3 (reduced-motion still honored per-instance), §T-O4 (single worldState/rAF-loop source of truth preserved, not forked per instance); `cordinator/dispatches/chronicle/C-SKYBOX-MULTI-INSTANCE.md`; Chronicle PR #543 (doctrine cited).
  - 2026-07-18 — **Sync chip beacon upgraded from "saw" to "applied" (C-SYNC-APPLIED-BEACON)**: #548 (below) proved only what Foundry last SAW via a GET; a fetch can still fail to apply. This dispatch adds the Chronicle half of the upgrade (a separate agent codes the Foundry-module confirm call against the contract below). **New endpoint:** `POST /calendar/date/confirm` (`calendar_api_handler.go` `ConfirmDate`) in the same syncapi Bearer group as `GET /calendar/date` (`RequirePermission(PermRead)`) — body `{year,month,day}` → 204. Same dual-auth route shape as GetCurrentDate, but since this endpoint's entire purpose IS the write (no calendar payload to still serve), a session-synthesized key gets a synchronous 403 rather than #548's silent fire-and-forget no-op. **Storage:** ONE new append-only idempotent migration, syncapi chain `006_calendar_date_beacon_applied` (`ADD COLUMN IF NOT EXISTS applied_year/month/day/applied_at` on the EXISTING `sync_calendar_date_beacons` row, not a new table — extends #548's per-campaign beacon). **Create-or-update:** `repository.go` `ConfirmCalendarDateBeacon` handles a confirm landing before any served-date GET (fresh install) via `INSERT ... ON DUPLICATE KEY UPDATE` that only touches `applied_*` on the update path; the insert branch fills the pre-existing NOT-NULL `last_served_*` columns with a `0/0/0` sentinel (same "unset" convention as `calendar_api_handler.go`'s `defaultIfZero` — a real month/day is always 1-31) rather than faking a served date. `GetCalendarSyncBeacon` (handler.go) now treats `Month == 0` as "never served" and omits the served fields from the response instead of surfacing `0000-00-00`. **Chip preference:** `/calendar-sync-beacon` response gains `last_applied_date`/`last_applied_at` (no second fetch — rides the same beacon call). `computeSyncChipState` (`calendar_v2_shell.js`) gains an optional 7th param `fmAppliedDate`; internally `confirmedDate = fmAppliedDate || fmConfirmedDate` — omitting the arg (every pre-existing caller) collapses to exactly the pre-upgrade behavior, the graceful-fallback pin. `wireSkyStrip` computes `fmAppliedDate` from the beacon response gated by the same `SKY_STRIP_STALE_AFTER_MS` freshness bar as the served date. `routes_snapshot.txt` +1 line (`UPDATE_ROUTES_SNAPSHOT=1`). Tests: Go handler auth-gate + Bearer-only pin (`calendar_confirm_date_handler_test.go`), service no-throttle pin (`calendar_confirm_date_test.go`), response applied-fields + never-served-sentinel-omitted pins (`calendar_sync_beacon_handler_test.go`), 9 new JS pure-function + `wireSkyStrip` integration tests in `calendar_v2_sky_strip.test.mjs` (applied-overrides-served, applied-drives-drift, stale-applied-ignored, graceful-fallback-byte-identical). `go test ./... -short` + `make test-js` (275 JS tests, +9 new) + `golangci-lint run ./...` all green. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (the synthetic-session-key rejection is the security-critical piece — session callers never write), §T-B2 (extends the existing syncapi-owned endpoint, no new cross-plugin coupling), §T-O4 (single beacon row/response, not a parallel "applied" table).
  - 2026-07-17 — **Sync chip drift, wired (C-SYNC-DATE-BEACON, closes #545's flagged gap)**: #545 shipped the Calendaria sync chip's DRIFT state fully built and unit-tested but dormant — no source persisted what date Foundry last saw. This dispatch adds a **served-date beacon**: syncapi's `GET /calendar/date` (`calendar_api_handler.go` `GetCurrentDate`, the endpoint FM polls) now records `{last_served_year/month/day, last_served_at}` per campaign via `SyncAPIService.RecordCalendarDateBeacon` — named "served" not "applied"/"synced" (honest: what FM last SAW, never a claim about what it did). **Storage:** new syncapi-chain migration `005_calendar_date_beacon` (`sync_calendar_date_beacons`, PK `campaign_id`) — neither `api_keys` (per-key) nor `sync_mappings` (per-object) fit the per-campaign grain, so a new table was the correct call, not a column bolt-on. **Throttle:** write skipped when the existing beacon is <60s old AND the date is unchanged; a changed date always writes immediately regardless of age (`calendarDateBeaconThrottle`, service.go). Write itself is fire-and-forget on a background context (`recordCalendarDateBeaconIfModule`, matching the `UpdateKeyLastUsed`/`LogRequest` convention) so the GET hot path never blocks on the throttle SELECT + conditional UPSERT. **CRITICAL auth gate:** `GET /calendar/date` is dual-auth (`RequireAuthOrAPIKey` — session cookie OR Bearer key); only a REAL Bearer key (`key.ID != synthKeySessionID`) may beacon, so a member browsing Chronicle's own calendar over the session path never does — pinned by 3 handler tests. **Expose:** new syncapi-owned member-read endpoint `GET /campaigns/:id/calendar-sync-beacon` (`Handler.GetCalendarSyncBeacon`, no `RequireRole` — any campaign member, mirrors `foundry_vtt`'s `/foundry-presence`) rather than reaching into the `foundry-presence` payload — avoids a new foundry_vtt↔syncapi Go coupling neither plugin currently has (T-B2). `routes_snapshot.txt` +1 line (`UPDATE_ROUTES_SNAPSHOT=1`). **Chip:** `calendar_v2_shell.js`'s `wireSkyStrip` now fetches both `/foundry-presence` (connectivity) and the beacon (confirmed date) in parallel, gates the beacon by its OWN freshness (`SKY_STRIP_STALE_AFTER_MS`, same 15-min bar as presence staleness — an old beacon can't be trusted to represent what FM currently sees) before feeding `computeSyncChipState`'s previously-dormant `fmConfirmedDate` param; `data-cal-current-date` (new SSR'd attribute, `skyStripCurrentDateString` in `calendar_v2_sky_strip_helpers.go`, reads `cal.CurrentYear/Month/Day` — the same real-time-seamed fields `GetCurrentDate` serves — NOT the navigated view date) supplies `chronicleCurrentDate`. Beacon-fetch failure degrades independently (chip still paints from presence alone). Tests: Go throttle/auth-gate/endpoint unit tests (`calendar_date_beacon_test.go`, `calendar_date_beacon_handler_test.go`, `calendar_sync_beacon_handler_test.go`) + 6 new `wireSkyStrip` integration tests in `calendar_v2_sky_strip.test.mjs` driving the real fetch-and-paint chain end-to-end (drift/in-sync/stale-beacon-ignored/no-beacon/beacon-fetch-fails/presence-fetch-fails) — #545's dormant pure-function drift tests stay as-is (still valid) and are now backed by production reachability. `go test ./... -short` + `make test-js` (238 JS tests) + `golangci-lint run ./...` all green. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (auth-gate is the security-critical piece), §T-B2 (syncapi-owned endpoint over cross-plugin coupling), §T-O2 (consumer-verified: read #545's PR body + the actual `RequireAuthOrAPIKey`/`GetAPIKey` source before wiring).
  - 2026-07-12 — **Six arcs merged since the 2026-07-04 docs-drift audit (Chronicle #522–#534); this entry catches status.md up (C-DOC-DRIFT-REFRESH-R2).** Plugin count is unchanged at **24** — the two new feature arcs below (real-time calendar, availability scheduler) both landed inside their existing plugin dirs (`calendar`, `sessions`), not as new plugins.
    - **Real-time calendar (P1–P3, #526/#529/#533):** `reallife`-flagged calendars go computed-not-stored via an opt-in `tracks_real_time` seam; P2 added the owner enable flow + closed the P1 merge-gate findings; P3 fixed a leap-year weekday-drift bug in the shared grid math, hardened the real-time invariants, and added a live clock.
    - **Availability scheduler (P1–P2, #530/#534, C-SCHED):** new real-world session-scheduling surface inside the `sessions` plugin. P1 shipped recurring per-member availability (zone-local wall-clock, DST-correct via the new `internal/timeutil` package) + a DM density heatmap. P2 added slot proposals (UTC-instant options, own-tables responses/tokens/notifications — migration 003), per-option accept/decline/tentative, a topbar notification bell, and a month↔week calendar view. See `internal/plugins/sessions/.ai.md`.
    - **Sidebar/nav consolidation (C-NAV-V3, both PRs, #528/#532):** unified the sidebar onto one data model + one reorder mechanic (PR1); PR2 closed the PR1 gate findings (legacy custom-section preservation, a reconcile-failure write-window risk) and added a JS CI gate.
    - **Entity-visibility parity (#525):** four anon-reachable JSON widget endpoints (`GetEntry`, `GetFieldsAPI`, `PreviewAPI`, `GetAliasesAPI`) now honor custom per-entity visibility instead of the legacy default-mode-only check.
    - **Customization Hub rescue (#524, C-CUSTOMIZE-RESCUE):** fixed four real breaks in the owner Customization Hub, all rooted in the topbar controls (topbar brand name + a first-class Image mode).
    - **Systems ref-slug fixes (#527 + #531, C-SYSTEMS-REF-SLUG-FIX + R2):** `ReferenceItem` now normalizes its ID from `slug` at load time (blank IDs were breaking slug-keyed system data); R2 added a duplicate-ID guard, event aggregation, and ZIP-preview parity.
    - Also merged in this window: public-campaign view regression fixes (#522/#523), GM-only entity-field leak fix (#517, audit M-1), the Timeline Ledger as the calendar's 4th V2 view (#519/#520), and the surface-accent pair UI (#521, same day as the entry below).
    - **In flight now:** a four-agent wave — `C-SCHED-P3` (scheduler confirm-winner → session creation), `C-CAL-RT-RECUR-FIX` (real-time × recurrence interaction fix), `C-BETA-SEC-PACK` (beta security pass), plus a parallel Foundry-module lane.
  - 2026-07-07 — **Accent trio (C-ACCENT-TRIO rev 2 / cordinator design D14 rev)**: the single campaign accent becomes three slots. Slot 1 "Chrome" = the existing `accent_color` (header + nav + global interactive) — its CSS emission is BYTE-IDENTICAL to before, pinned by `TestAccentColorCSS_ChromeByteIdentical` against a legacy-formula oracle in `internal/templates/layouts/data_accent_test.go`. Slots 2+3 = the "surface pair" (`accent_surface_1/2` in `CampaignSettings`), consumed by themed content surfaces as primary/secondary via `--color-accent-surface-N{,-hover,-light,-rgb}` tokens emitted only when set (`accentSlotCSS`, one shared derivation — no forks); unset slots inherit chrome via `var(..., var(--color-accent))` fallback chains at consumers (zero CSS bytes). Save path EXTENDS `PUT /campaigns/:id/accent-color` with an optional `slot` form value (1|2) — no new route, `routes_snapshot.txt` untouched; service `UpdateAccentSurface` (load-merge-write, tested in `service_accent_test.go`). Hub: new "Surface Accents" card in the appearance tab (`surfaceAccentRow` ×2, direct-PUT precedent + local live preview) with a D19 sample character-header card. First consumer: the calendar V2 active view pill reads surface-1 (`calendar_v2.templ` `calendarV2ViewPill`). Character-page adoption is the named follow-up (the surface pair's primary consumer per the operator — lands with the entity/sheet theming pass). Branch `claude/calendar-audit-opus-prep-efr8r5`.
  - 2026-06-27 — **AI Workflow Support: data-tracing diagnostics (Phase 0 of the DS sheet-v3 arc)**: added 6 read-only diagnostics to the operator workspace catalog (`internal/systems/operator_diag.go` + new `operator_diag_entities.go`) to debug the "sheet renders blank / data not flowing" class. **`system.file-contains`** `<sys>:<relpath>:<markers>` — reads a served file (reusing the `fingerprintFiles` traversal clamp via new `clampedPath`/`readClampedFile`) and reports marker presence (live-build *content*, not just hash; closes the gap where I'd dropped to local `git show` for `playEntrance`). **`entity.fields`** `<camp>:<idOrSlug>` + **`entity.field-coverage`** `<camp>:<typeIdOrName>` — dump a hero's stored `fields_data` / declared-field population %, via injected `EntityDiagProvider` (impl `entityDiagAdapter` in new `internal/app/operator_diag_adapters.go`, wired like `SetInstalledPackagesProvider`; reads `entityService` GetByID/GetBySlug/List/GetEntityType*). **`sync.inbound`** `<entityId>` + **`sync.recent`** — what Foundry actually SENT, from a new bounded in-memory ring buffer in syncapi (`inbound_buffer.go`, captured in `UpdateEntityFields`; no DB/migration, mirrors the loader's DiagnosticEvents), wired via `SetSyncInboundProvider`. Together a **three-way trace** (sent→stored→declared) localizes where a value dies. All redacted/admin-gated/audited by the existing workspace; no new routes (wire snapshot unaffected). Tests: `operator_diag_entities_test.go`, `inbound_buffer_test.go`. Doc: `operator-diagnostics.md` catalog table. Branch `claude/chronicle-sheet-sync-j2m9s4`. **Next (Phase 1–3):** DS character-sheet **v3** (drop `hasX` gates → always-rendered scaffold + placeholders + responsive; new COMBAT/GM-LORE boxes + header actions) and findings-driven data/sync fixes — see plan `frolicking-foraging-candle.md`.
  - 2026-06-26 — **Operator AI Workspace (in-app diagnostics batch protocol)**: turned the operator diagnostics catalog into a single copy-paste round-trip with a **human-approval gate**, modeled on the campaign `ai_workspace` plugin's export→paste→review→commit flow but admin-scoped and read-only. New page `GET /admin/diagnostics/workspace` (`internal/plugins/admin/diagnostics_workspace.templ` + handler `DiagnosticsWorkspace`/`DiagnosticsWorkspaceParse`/`DiagnosticsWorkspaceRun`, routes in admin `routes.go`): **(1)** copy a machine-readable **functions list** (`systems.FunctionsSpecJSON` — every read-only diagnostic + the exact request shape) to feed an external AI; **(2)** paste back the ONE batch object the AI composes (`{v,note,full_dump,calls[]}`, fenced ```json accepted); **(3)** *Parse & review* validates against the live catalog and shows each call with unknown-name / full-dump flags — **nothing runs until the human clicks Approve**; **(4)** *Approve & run* executes the runnable read-only diagnostics and returns ONE compact, secret-redacted doc (manifest + results + byte-count footer) to copy back. Logic in new `internal/systems/operator_batch.go` (`Catalog`, `FunctionsSpecJSON`, `ParseBatch`→`BatchPlan`, `RunBatch`); added `FullDump bool` to `Diagnostic` (gates `system.health` — must set `full_dump:true` AND approve). Safety: bounded toolset (only catalog names run; 50-call cap; `DisallowUnknownFields`), **re-parse + re-derive runnability server-side on run** (never trusts a client plan), redaction belt-and-suspenders. Dashboard "Diagnostics" card repointed to the workspace; supersedes the standalone content-negotiated HTML console. Tests: `operator_batch_test.go` (parse/codefence/full-dump-gate/unknown/errors, manifest+footer, redaction). Wire snapshot updated for the 3 admin-gated routes (T-O2: admin-only read-only diagnostics UI). Doc: `docs/operator-diagnostics.md` §"The in-app AI Workspace". Branch `claude/chronicle-sheet-sync-j2m9s4`. **Hardening pass (parallel security + gap-analysis agents, same branch/PR #507):** security audit cleared ReDoS (RE2 linear), the full-dump gate (enforced in parse AND run), Templ XSS (all escaped), CSRF (global middleware + admin gate), and plan-tampering (run re-parses; trusts no client plan). Fixes applied: **(1) audit logging** — `DiagnosticsWorkspaceRun` now logs `admin.diagnostics_batch_run` via the already-wired `securityService.LogEvent` (Option A: site-admin event w/ actor+IP+UA, counts-only — the campaign `audit.Log` path rejects empty CampaignID so it doesn't fit; new event const/label/icon in `security_model.go`). **(2) output/CPU amplifier (MED)** — `RunBatch` does one authoritative classify+**dedup** pass (identical `(name,arg)` runs once → kills the 50×-rehash vector) and caps assembled output at 256 KB w/ truncation notice. **(3)** 64 KB paste cap + `sanitizeInline` on note/name/arg echoed into the result markdown (LOW manifest-integrity). Footer now reports bytes + ~tokens. Tests added: dedup (parse+run), byte cap, sanitize, note-sanitized. Deferred: per-route rate-limiting (spec §C2; low priority behind admin gate + bounded/deduped/capped toolset).
  - 2026-06-26 — **Operator-diagnostics: deploy/serve diagnostics batch**: three new named diagnostics + three SQL probes in `internal/systems/operator_diag.go`. **`packages.installed-vs-loaded`** — the smoking-gun check: compares each installed system package's DB version to what the loader actually serves (matched by install-path prefix → sidesteps slug-vs-manifest-id), flagging `NOT loaded` (registry never picked up the install) and version `MISMATCH`. Fed by a new injected `SetInstalledPackagesProvider` (dependency inversion; wired in `routes.go` from `pkgService.ListPackages`, system packages only). **`packages.on-disk-versions`** — lists every on-disk version folder per package tagging `[installed-db]`/`[LOADED]` to expose a shadowing leftover. **`systems.load-events`** — renders the loader's `DiagnosticEvents()` (discovered/skipped/failed, incl. WS-6 EventSkipped reasons). Probes added: `packages-db-state`, `entity-type-tree`, `sync-mapping-orphans` (SQL, paste-back). Tests: render logic for all three (NOT-loaded / MISMATCH / OK, load-events skip+reason) reusing the `withLoadedSystems` fixture; `go test -race` green. Doc tables updated. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **Operator-diagnostics hardening + docs (parallel-agent audit pass)**: a security/correctness audit of the new `internal/systems` diagnostics surfaced three real issues, all fixed: **(1) path traversal (HIGH)** — `fingerprintFiles` joined `dir`+manifest widget paths without clamping, so a hostile manifest `script_file:"../../etc/passwd"` could be stat/read; now `filepath.Clean`+`HasPrefix` clamp to the system dir (mirrors `WidgetScriptAPI`). **(2) unbounded read (MED)** — added an 8 MiB cap (`maxFingerprintBytes`); oversized files report `too-large` instead of being hashed (OOM guard). **(3) data race (MED)** — `globalLoader.modules`/`systemInstances` were accessed without sync while package installs re-`register()` concurrently; added `sync.RWMutex` to `SystemLoader` (writes in `register`/`RegisterSystem`, RLock in `Get/Dir/All/Count/GetSystem/AllSystems/Health`; `Health` snapshots under RLock then does disk I/O unlocked). Also widened `redactSecrets` to catch space-separated `Bearer <token>`. WS-5 `mergeNewFields` + WS-6 `preferCandidate` audited → PASS. `go test -race ./internal/systems` green. New: `docs/operator-diagnostics.md` (full feature doc) + `operator_diag_extra_test.go` (handler-level httptest, redaction/fingerprint edge cases) + traversal/size-cap tests. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **Extensions deployment-health diagnostic (read-only admin)**: `GET /admin/extensions/health` (`internal/systems/health.go` + `handler.go` `ExtensionsHealthAPI`, wired on the admin group in `routes.go`) returns, per LOADED system, the version + on-disk dir the loader actually serves from (`Dir(id)`, `source`) plus a content fingerprint (size + sha256[:16] + mtime) of each widget/manifest file. Purpose: diagnose the "Admin▸Packages reports version X but the stale file still renders" class of bug from the UI without host/SSH access — if `loaded_version` ≠ the installed version the in-memory registry never picked up the install; if it matches but a file hash is the old content the extraction is wrong. Read-only by construction (only stats/hashes files the loader already serves). Tests: `health_test.go` (fingerprint exists/size/hash/missing, content-sensitivity, dedupe, loader assembly). First slice of the operator-facing "extensions health" surface. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **Character-fields API: `foundry_item_single` (WS-3 single-item collections)**: added `FoundryItemSingle` to `FieldDef` + `CharacterFieldExport`, carried through both `CharacterFieldsForAPI`/`ItemFieldsForAPI` (`internal/systems/manifest.go`). Lets a system map an "exactly one X item" field (a hero's class/ancestry/kit/subclass, which live as embedded Foundry items, not a `system.*` path) to that item's name as a scalar string — the Foundry generic adapter collapses the collection when the flag is set. Mirrors the existing `foundry_collection`/`foundry_item_type`/`foundry_item_fields` passthrough (87c3bf7). Test: `manifest_test.go` `…_CollectionAnnotations`. Consumed by Chronicle-Draw-Steel's manifest (class/ancestry/kit/subclass) + Chronicle-Foundry-Module's adapter. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **System loader: version-aware duplicate resolution (WS-6; no silent downgrade)**: the loader keyed `modules[manifest.ID]` with unconditional last-wins, so two dirs declaring the same system ID (e.g. a leftover `<slug>-1`) resolved by scan order — a stale copy could shadow the current one. New `preferCandidate`/`register` (`internal/systems/loader.go`) apply a policy in BOTH discovery paths (`DiscoverAll` bundled + `loadSingleSystem` package): highest `version` wins (numeric `versionLess`), equal-version package overlays bundled (the intended override), and an older/stale duplicate is **skipped, not loaded** — gated so it also can't re-enter via `systemInstances` instantiation. Skips emit a new `EventSkipped` (`event_log.go`) so they surface in admin diagnostics instead of vanishing. Tests: `loader_dedup_test.go` (newest-wins regardless of scan order, stale-doesn't-shadow, `preferCandidate` table). The package-manager slug scan (`registry.go`) already picked the newest version *subdir*; this closes the same hazard at the ID-collision level. Branch `claude/chronicle-sheet-sync-j2m9s4`. **Remaining WS-6 piece:** the admin "Extensions Health — installed vs loaded" view (UI; surfaces the skip events) — deferred (needs a live look).
  - 2026-06-26 — **Entity-type field reconcile in place (WS-5; closes PC-PRESET-FIELDS)**: `ApplySystemPresets` (`internal/app/preset_applier.go`) no longer skips an existing type — it indexes existing types by preset-category then name and, on a match, calls the new **`entities.ReconcileEntityTypeFields(typeID, declared)`** to additively backfill any declared schema field the type is missing (else creates as before). The merge is the pure, unit-tested `mergeNewFields` (append-missing-by-Key; never removes/reorders/overwrites → idempotent, safe on every system enable/update). This fills EXISTING heroes' type schema (e.g. Draw Steel Tyne/Orrin/Saatraaol gain backstory/abilities/etc.) **without recreating the type** (which would orphan claimed entities). Type schema only — never entity data; trigger is the existing enable path (fires on package install/update). Tests: `internal/plugins/entities/reconcile_fields_test.go`. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-24 — **Migration robustness (post-`000030` incident; ADR-045)**: deleting an applied migration crash-looped prod. Durable fix in three layers. **Shipped in two PRs** (the original PR #498 had already merged at the pre-restore commit, stranding the fix — see ADR-044 incident-response lesson): the restore + runtime + CI guards landed on `main` via hotfix **PR #499**; the admin **visibility** layer (the unified Database page) follows in a second PR off `claude/gallant-johnson-kd574n`. **(1) Runtime** (`internal/database/migrate_state.go` `MigrateWithBackup`, replacing the unconditional backup→migrate in `cmd/server/main.go`): back up ONLY when a migration is pending (ends the every-restart backup storm); **DB AHEAD of build → log + boot anyway** (fixes the deletion case AND ordinary image rollbacks; additive-migration assumption, backstopped by health checks); **dirty DB fails fast** with restore guidance (the old `Force(v-1)` looped forever on non-idempotent DDL); `fatalBoot` sleeps `BOOT_FAIL_BACKOFF` (45s) so unrecoverable boots don't hot-loop. **(2) CI guards** (`internal/database/migrate_test.go` + `tools/check-migration-immutability.sh`): immutability (no delete/edit of an applied migration — the guard that blocks the incident), version-pin (`ExpectedCoreMigrationVersion == max`; fixed the live 29-vs-30 drift), idempotent-DDL lint (31 historical files grandfathered), gapless numbering, plugin up/down pairs. **(3) Admin visibility — unified tabbed `/admin/database` page** (Migrations · Health · Backups · Schema, behind an at-a-glance status strip): **Migrations** = core "Core schema" card (version/dirty/pending + DB-ahead banner) + per-plugin grid + history; **Health** = the SAME `RunStartupHealthChecks` boot runs, surfaced live (pass/warn/fail pills) + `GET /admin/database/status` JSON for monitoring — `database.RunHealthChecks` split out (structured result, no logging/exit) and the boot config extracted to `app.StartupHealthCheckConfig` so boot + admin never disagree; **Backups** = artifacts with an Auto (pre-migration) vs Manual badge, restorable snapshots w/ Chronicle+schema versions, last-auto-backup recency, and create/download/restore actions reusing the existing `backup`/`restore` plugin flows (no new engine); **Schema** = the D3 diagram, lazily mounted on first tab activation so it reads a real width. Admin stays decoupled: it defines `HealthChecker`/`BackupLister` interfaces, the app layer injects adapters (`internal/app/admin_db_adapters.go`). **Policy:** migrations are APPEND-ONLY + SCHEMA-ONLY; one-time DATA fixes are idempotent reconcilers (the `app/setup_pc.go` pattern), never migrations. Docs: ADR-045, CLAUDE.md §Migrations, `.ai/conventions.md` §Migration Safety Rules #8-11, `docs/deployment.md` §7.
  - 2026-06-24 — **Extension settings framework + owner-driven PC reconciliation (ADDITIVE to the retained migration)**: the prod duplicate (a generic "Player Characters" holding the characters next to Draw Steel's empty "Heroes" `drawsteel-character`) is auto-merged on deploy by the **retained** one-time guarded migration `000030` (unambiguous case), now COMPLEMENTED by an owner-driven path for the cases it skips. ⚠️ **PROD INCIDENT + lesson:** an initial draft DELETED `000030` and its test — that **crash-looped production** on boot (`no migration found for version 30: read down for version 30: file does not exist`; golang-migrate's `file://` source must contain every version up to the DB's recorded version). Reverted (`9990134`); **a migration any live DB has applied can never be deleted.** Each extension gets a reusable **settings/onboarding page** (overlay or full page) driven by a slug-keyed `SetupProvider` registry in the `addons` plugin (wired from `internal/app/routes.go` like `PresetApplier`; state in `campaign_addons.config_json.setup`, no migration). Enabling an addon nudges the owner (existing `extensions-hub-refresh` HX-Trigger + a `chronicle:notify` toast) and surfaces a "Setup" badge on the Extensions card. First provider = player-character setup: detects existing PC entities / sub-categories / the duplicate, asks "use the system's name ('Heroes') or your own?", and **merges on demand** via the new owner-triggered, single-campaign `entities.MergeDuplicatePlayerCharacterType` (+ repo `MoveEntitiesAndDeleteType`, one tx; human-readable `apperror` when ambiguous — this also closes PC-DUP-GUARD-2). Enabling stays safe (idempotent `EnsurePlayerCharacterType` still runs). **Kept** from the prior arc: the `nest` closure in `EnsurePlayerCharacterType` and the dead palette-filter removal. Branch `claude/gallant-johnson-kd574n`. Plan: `/root/.claude/plans/drifting-jingling-boole.md`. **Still deferred:** `ApplySystemPresets` drops `preset.Fields` (PC-PRESET-FIELDS).
  - 2026-06-23 — **PC-sheet system-binding (dynamic-surface arc, 4 shipped seams)**: a system pack now fills Chronicle's player-character category with **zero core changes** — the modularity seam for the dynamic surface. Design: Cordinator `plans/2026-06-23-pc-sheet-system-binding-design.md` (§11 is the binding shape — minimal-touch). Shipped (4 commits): **(1) renderer-by-preset** — `RendererDef.PresetCategory` (`internal/systems/manifest.go`); a renderer binds by entity-type slug **XOR** `preset_category` (validated against the manifest's own `entity_presets[].category`); the registry gains a `presetRenderers` map (`show_renderer_registry.go`) and `lookupEntityShowRenderer` resolves **slug-first-then-preset** so a pack's own slug-bound type is never shadowed; `registerManifestRenderers` (`routes.go`) routes each entry to the right map. **(2) nest the system char type** — `EnsurePlayerCharacterType` Case 2 now re-parents a system's own character type (e.g. `drawsteel-character`, detected generically via `isClaimableType`) under "Characters" as the PC sub-category — **no rename** (preserve the system's terms), **no field copy** (it already carries them + renders via its own slug renderer); no generic type is made. System-less campaigns still get the generic "Player Characters" (Case 3). **(3) duplicate guard** — `CreateEntityType` rejects a manual second PC type (one per preset-category) with a `Conflict` pointing at the addon. **(4) page-renderer ≠ palette block** — `GetSystemWidgetBlockMetas` (`internal/systems/handler.go`) excludes any widget that's also a `renderers[].widget`, killing the "drop the sheet into a layout → bare name" trap (principled rule replacing the earlier slug-string filter `9a1e4d6`). No DB change (manifest JSON only). Field-adoption / rename / `CreateEntityTypeInput.Fields` from the design's earlier drafts were **dropped** by §11. Docs: `systems/.ai.md` §Renderers, `entities/.ai.md` §`EnsurePlayerCharacterType`, `docs/system-package-rendering.md` §Player-character sheet. *Next:* Draw Steel `0.0.10` (optional `preset_category:"character"` renderer; not required since its slug renderer already covers its type); operator's prod-campaign stray-duplicate migration (separate from the boot path).
  - 2026-06-22 — **Dynamic-surface frame (Wave 1) + live demo**: `Chronicle.surface` (`static/js/widgets/dynamic_surface.js`; see its `.ai.md`) is the system-agnostic frame for dynamic sheets — a motion-preset menu, an overlay stack, an expand/collapse box, a memoized data provider, a mini→full launch, and a schema-driven `mount`. The admin **Design Lab** (`/admin/design-lab`) was repurposed from the old static component catalogue into the **surface demo**: a live, seeded character sheet, with `static/js/widgets/surface_demo.js` standing in as a sample "System" that registers box renderers + mounts the schema. Backend support: `entities.UpdateFields` now broadcasts an `updated` event (GAP-1, PR #490) so field pushes reach live web/Foundry consumers; a stored sync-key resolves to Owner visibility with a loud degrade signal (§1B, PR #489). Design: Cordinator `plans/2026-06-21-dynamic-widget-ui-framework-design.md` + the `2026-06-21-*-vision` / `-responsibilities-*` set. **First production adopter:** the new per-campaign **Characters ("Cast") page** (`GET /campaigns/:id/characters`) — the party (claimed PCs, viewer's own highlighted) + active NPCs (GM-curated `cast` tag), each card linking to the entity page and launching a mini→full preview via the frame. New `service.ListClaimed`; pure `assembleCastParty` (tested); per-plugin `embed.FS` for `characters.js`; sidebar "Characters" link. Zero migration. See `entities/.ai.md` §"Characters Page" + Cordinator `plans/2026-06-22-characters-cast-page-design.md`. **Second adopter:** the surface is now a real layout **block** (`character_surface`, editable in the layout editor) and the **default page for player-character types** (`CharacterLayout()`) — a seeded "big widget" sheet (portrait + fields + a description box that reuses the role-aware `editor` widget). Frame fix: `Provider.push` no longer clobbered by `load()`. See `entities/.ai.md` §`character_surface`. The Characters page is now **addon-gated + consolidated**: a Players section (`player-character-claiming`) + an NPCs/Monsters section contributed by the `npcs` plugin via an injected `NPCSectionProvider` (featured tag-row + revealed list + DM reveal/hide); page 404s if neither addon is on, and the old `/npcs` gallery redirects in. Enabling the `player-character-claiming` addon now **auto-premakes** the claimable "Player Characters" type (with the `character_surface` layout) via `PresetApplier.ApplyAddonEnableEffects` → `EnsurePlayerCharacterType` (idempotent); a **one-time idempotent startup backfill** (`internal/app/backfill.go`, wired in `routes.go` after the preset applier) replays that effect across campaigns that enabled the addon *before* it shipped — new `addons.ListCampaignsUsingAddon` + the existing service method, no SQL hand-rolling, safe every boot. Reference fix: client reference-renderers now resolve via `GET /campaigns/:id/systems/:mod/rules-glossary` (`SystemHandler.RulesGlossaryAPI`, serves the system's raw `data/rules-glossary.json` preserving authored `slug`). **Third adopter (shipped, Chronicle-Draw-Steel):** the Draw Steel `character-sheet` widget was refactored onto `Chronicle.surface` — 11 `ds-*` box renderers, schema omits empty boxes, headless identity banner, ability cards open a power-roll overlay; mount contract unchanged. **All of the above is migration-safe** (zero schema/`.sql` changes since the last green `main`; the one new query reads existing columns). **Fourth adopter (Flagship #2 — Rulebook) — PARKED** (built then removed from the build at the operator's request — "still a travesty"; code recoverable at commit `bbe6508` for a later rework): the Chronicle-core widget `static/js/widgets/rulebook.js` (`data-widget="rulebook"`; see its `.ai.md`) turns a system's `data/rules-glossary.json` into an interactive surface — a category nav + expanding rule boxes, debounced cross-category search, and stackable `{@term}` cross-ref overlays — reusing the frame for chrome/motion/overlays. Mounted above the category grid in `SystemIndexContent`; degrades invisibly with no glossary, so zero backend/route changes. **PC sub-category migration (prod):** `EnsurePlayerCharacterType` now ensures the claimable "Player Character" type is **nested under the default "Characters" category** (`ParentTypeID`) and **re-parents a stray top-level one** an earlier build premade — via `UpdateEntityType`, preserving claimed characters + layout; the startup backfill replays it, so deploying heals prod in place (the ADR-039 shape; no manual delete/SQL). Next: browser-verify the DS sheet + the prod PR to green `main`.
  - 2026-06-19 — **Player Character Claiming ALL 4 STAGES COMPLETE (Chronicle PR #480/#481/#482 + gate-enforce; Foundry-module PR #64)**: audit visibility + database layer + UI (claim banner, owner roster, per-type toggle) + Foundry actor-sync addon-aware PC sub-type routing/claiming. Security pass: `.ai/security-audit-2026-06-19-pc-claiming.md`. See `.ai/decisions.md` (ADR-039), `docs/player-character-claiming.md` (operator guide), `entities/.ai.md` §"Player Character Claiming". Cross-repo coordination record: `cordinator/decisions/2026-06-19-pc-claim-design.md` + `cordinator/plans/2026-06-19-pc-claim-arc.md`.
  - 2026-06-11 — **Worldstate/GM overhaul arc COMPLETE; calendar code-complete** pending the operator's final visual pass. Landed post-#443: GM console r3 single-writer state machine (#456), day mini-view (#457), drawer action set incl. cross-plugin `EntityCreator` seam (#458), fragment-401 error-handler class fix (#459), demo consolidation (#460), recurrence via single `Event.OccursOn` predicate + additive `weatherDate` on the world-state PUT (#461). Next arcs queued in `cordinator/plans/ACTIVE.md` (2026-06-11 block): sweep fixes in flight, Puzzles arc designed (`cordinator/plans/2026-06-11-puzzles-arc.md`), Timeline V2 awaiting design pick.
  - chronicle#442 (+ #443 r2) — worldstate render overhaul: cal-almanac-render.css render/chrome CSS split, back+front sky canvases with layered SKY_FX, strip+full-band-sheets GM console. See `internal/plugins/calendar/.ai.md` §"Worldstate render architecture (2026-06)".
  - `cordinator/decisions/2026-05-26-chronicle-production-safety-system.md` — `RunStartupHealthChecks` rubric + docker-unavailable substitute pattern
  - `cordinator/decisions/2026-05-26-ai-export-pipeline-design.md` — AI export pipeline design (future scope, scoping decisions locked)
  - `cordinator/decisions/2026-05-26-draw-steel-spin-up-strategy.md` — Draw Steel game system spin-up strategy (own security audit first)
  - `cordinator/decisions/2026-05-25-plugin-static-assets.md` — per-plugin `embed.FS` static-asset ownership (NW-2.2 Chunk F)
  - `cordinator/decisions/2026-05-23-plugin-registration.md` — lightweight `PluginRegistration` registry (NW-2.2 Chunk A)
  - `cordinator/decisions/2026-05-22-loadDescriptor-fallback.md` — Chronicle/Foundry-Module descriptor wire pin (locked, used by `internal/plugins/foundry_vtt/descriptor_fallback_test.go`)

### Bootstrap reads for a new session

In order:

1. `cordinator/decisions/2026-05-21-core-tenets.md` — binding tenets every session honors
2. `cordinator/decisions/2026-05-19-dispatch-workflow.md` — how dispatches + status reports flow
3. `cordinator/decisions/2026-05-23-decision-routing.md` — backend-vs-frontend question routing
4. This file (you're here) — high-level state + plugin index
5. `.ai/conventions.md` — code patterns
6. `.ai/architecture.md` — three-tier extension model + request flow
7. The relevant plugin's `.ai.md` if your work is plugin-scoped (see index below)

### Plugin .ai.md index (the canonical per-plugin docs)

#### Plugins (24)

| Plugin | `.ai.md` |
|---|---|
| addons | [internal/plugins/addons/.ai.md](../internal/plugins/addons/.ai.md) |
| admin | [internal/plugins/admin/.ai.md](../internal/plugins/admin/.ai.md) |
| ai_workspace | [internal/plugins/ai_workspace/.ai.md](../internal/plugins/ai_workspace/.ai.md) |
| armory | [internal/plugins/armory/.ai.md](../internal/plugins/armory/.ai.md) |
| audit | [internal/plugins/audit/.ai.md](../internal/plugins/audit/.ai.md) |
| auth | [internal/plugins/auth/.ai.md](../internal/plugins/auth/.ai.md) |
| backup | [internal/plugins/backup/.ai.md](../internal/plugins/backup/.ai.md) |
| bestiary | [internal/plugins/bestiary/.ai.md](../internal/plugins/bestiary/.ai.md) |
| calendar | [internal/plugins/calendar/.ai.md](../internal/plugins/calendar/.ai.md) |
| campaigns | [internal/plugins/campaigns/.ai.md](../internal/plugins/campaigns/.ai.md) |
| designlab | [internal/plugins/designlab/.ai.md](../internal/plugins/designlab/.ai.md) |
| entities | [internal/plugins/entities/.ai.md](../internal/plugins/entities/.ai.md) |
| foundry_vtt | [internal/plugins/foundry_vtt/.ai.md](../internal/plugins/foundry_vtt/.ai.md) |
| maps | [internal/plugins/maps/.ai.md](../internal/plugins/maps/.ai.md) |
| media | [internal/plugins/media/.ai.md](../internal/plugins/media/.ai.md) |
| npcs | [internal/plugins/npcs/.ai.md](../internal/plugins/npcs/.ai.md) |
| packages | [internal/plugins/packages/.ai.md](../internal/plugins/packages/.ai.md) |
| restore | [internal/plugins/restore/.ai.md](../internal/plugins/restore/.ai.md) |
| sessions | [internal/plugins/sessions/.ai.md](../internal/plugins/sessions/.ai.md) |
| settings | [internal/plugins/settings/.ai.md](../internal/plugins/settings/.ai.md) |
| smtp | [internal/plugins/smtp/.ai.md](../internal/plugins/smtp/.ai.md) |
| syncapi | [internal/plugins/syncapi/.ai.md](../internal/plugins/syncapi/.ai.md) |
| timeline | [internal/plugins/timeline/.ai.md](../internal/plugins/timeline/.ai.md) |
| widgetbindings | [internal/plugins/widgetbindings/.ai.md](../internal/plugins/widgetbindings/.ai.md) |

#### Widgets (11 of 11)

| Widget | `.ai.md` |
|---|---|
| attributes | [internal/widgets/attributes/.ai.md](../internal/widgets/attributes/.ai.md) |
| calendar_block | [internal/widgets/calendar_block/.ai.md](../internal/widgets/calendar_block/.ai.md) |
| calendar_v2 | [internal/widgets/calendar_v2/.ai.md](../internal/widgets/calendar_v2/.ai.md) — **READ-ONLY for the calendar-v4 wave** |
| editor | [internal/widgets/editor/.ai.md](../internal/widgets/editor/.ai.md) |
| entity_notes | [internal/widgets/entity_notes/.ai.md](../internal/widgets/entity_notes/.ai.md) |
| mentions | [internal/widgets/mentions/.ai.md](../internal/widgets/mentions/.ai.md) |
| notes | [internal/widgets/notes/.ai.md](../internal/widgets/notes/.ai.md) |
| posts | [internal/widgets/posts/.ai.md](../internal/widgets/posts/.ai.md) |
| relations | [internal/widgets/relations/.ai.md](../internal/widgets/relations/.ai.md) |
| tags | [internal/widgets/tags/.ai.md](../internal/widgets/tags/.ai.md) |
| title | [internal/widgets/title/.ai.md](../internal/widgets/title/.ai.md) |

`calendar_block` (new, `internal/widgets/calendar_block/`) currently holds only
the pinned cross-slice contract `data.go` + its reflection shape pin. Its
`.ai.md` lands with the renderer in C-CALV4-BLOCK-P1.

### Cross-cutting state (not plugin-scoped)

#### 2026-07-26 — C-CALV4-FOUNDATION-P0 (calendar-v4 wave 1, phase A)

The floor the other four wave-1 slices stand on. Additive only — no Go behaviour
change, no templ change, no route, no migration.

| Item | What |
|---|---|
| Data contract | `internal/widgets/calendar_block/data.go`, copied **byte-identically** from the coordinator pin (`cordinator/dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`). It is the ONE cross-slice artifact of the wave; three dispatches write against it. **Editing the struct is a STOP-AND-FLAG**, not a judgement call. `data_test.go` pins the field set (name + kind + composite type, in declaration order) by **reflection only** — it reads no source text, so restructuring `data.go` cannot red CI. |
| Motion tokens | `--motion-fast` / `--motion-standard` / `--motion-leisurely` were referenced fallback-only throughout the V2 animation library and **never defined**. Now defined in `static/css/input.css:146-148` at exactly their existing fallback values (150 / 200 / 300ms) — a no-op on shipped surfaces. Deliberately NOT aliased to `--dur-*` (`--dur-micro` is 120ms, not 150ms). |
| Guards B1–B4 | `tools/check-calendar-v4-lints.sh`, wired into CI. Diff-scoped against `origin/main` like the other guards. Self-tests itself on every run, so "OK" always means the rules can still fire. |
| Motion-discipline guard | `tools/check-v2-motion-discipline.sh` had existed since the V2 wave and was invoked by **nothing** — only a comment at `input.css:103` referenced it. Now wired into CI. It was already diff-scoped and passes on main; a non-diff-scoped run would flag 7 grandfathered lines across 5 files. |
| Deleted guard | `tools/check-templ-drift.sh` — diffed pathspec `'*.templ.go'` while templ emits `*_templ.go`, which are gitignored. Structurally incapable of failing; reported OK on a tree where 753 generated files had changed. **Deleted rather than repaired**: `templ generate` + `go build` in CI already catch real drift, and a guard that reads as coverage it never provided is worse than no guard. |
| `make verify` | New target chaining the full local CI sequence in CI order. Five parallel chats were each reconstructing it from `ci.yml` by hand. |
| `calendar_v2/.ai.md` | Filed at last (house rule 7). States the package is **read-only for the v4 wave** and why: its `data-visibility-*` DOM contract has two independent JS drivers in the calendar plugin, and nothing — compiler or test — catches an attribute rename. |

#### Active arc: Calendar remodel (cordinator `plans/2026-07-24-calendar-remodel-requirements.md`)

Wave 1. Source-of-record for scope + operator priorities is the cordinator plan; ADRs land here.

| Item | What | Status |
|---|---|---|
| C-CAL-ENTITY-TIES-LEAK-FIX | Entity-visibility leak on event/era tie lists | ✅ shipped PR #565 |
| C-CAL-RSVP-P1 | First-class RSVPs on calendar events + SMTP action emails (operator's #1 priority) — ADR-046, calendar migration 013 | ✅ this branch |
| C-CAL-RSVP-P2 | Players give TEMPORARY availability from the RSVP flow — "suggest another time" becomes structured windows that land in the scheduler overlay. No new tables/routes. ADR-046 amendment | ✅ this branch |
| C-CAL-MOBILE-VIEWS-FIX | Phone Month\|Agenda toggle: the Agenda pill was a dead control (linked to the URL it was already on). Step 0 **disproved** the "mobile layout never swaps" half — a 390px browser probe shows the swap always worked; the reported "7-column grid" is the shipped mini-month. Absorbs **C-ASSET-VERSIONING** (see below) | ✅ this branch |

**Operator gate still open:** RSVP *emails* need SMTP configured (Admin → `/admin/smtp` →
"Send test email"). In-app RSVP works without it — every mail seam is nil-safe.

##### calendar-v4 widgetization remodel (wave 1, 2026-07-26)

Operator directive: *"just go with a full redo of the calendar."* Source of record is
cordinator `plans/2026-07-26-calendar-v4-remodel-master-plan.md`; the design contract
(`mockups/calendar-v4.html` + 73 signed stills) is **immutable**, and the nine coordinator
rulings live in `dispatches/chronicle/C-CALV4-WAVE1-COMMON.md` §6.

| Slice | What | Status |
|---|---|---|
| C-CALV4-FOUNDATION-P0 | Phase A — the data contract + the guard rails: the pinned `BlockData` + its reflection shape pin, the three never-defined `--motion-*` tokens, guards B1–B4 and the dormant motion-discipline guard wired into CI, the vacuous templ-drift guard deleted, `make verify`, `calendar_v2/.ai.md`. Detail under its own dated heading above | ✅ PR #569 |
| C-CALV4-SPINE-P2 | Server spine: leap-aware geometry, one-pass per-viewer projection, batched eager loader, narrow interfaces, the wave's only `app/routes.go` touch | ✅ this branch |

**Two cross-slice facts a later slice must not re-derive:**

- `internal/widgets/calendar_block/data.go` is the ONE cross-slice contract, pinned
  verbatim at cordinator `dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`. It is copied
  byte-identically (including a struct-tag alignment `gofmt` would change — do NOT run
  `gofmt -w` on it; no gofmt linter is enabled in `.golangci.yml`, so it is safe).
  **Editing the struct is a stop-and-flag.**
- `calendar.BlockSpine()` is the process-wide accessor for the Block service, installed
  from `app/routes.go` via `calendar.InstallBlockSpine`. It exists because `routes.go`
  belongs to exactly one calendar-v4 slice for the whole wave, so the phase-B surfaces
  (entity-page hosting, the Bench) reach the spine without re-editing it.

**RESOLVED (r51 pin amendment, C-CALV4-SEAM-P5 stage 1, 2026-07-27):** the pin
originally typed `CalendarID` / `Mark.EventID` / `ViewerContext.UserID` /
`ViewerContext.HostEntity` as `int64` while all four are `VARCHAR(36)` UUIDs in
Chronicle, so the producer zeroed them and smuggled the calendar's identity
through `CalendarSlug`. The amended pin types them `string`; the producer now
carries the real ids and `CalendarSlug` means the slug again.

**Cross-plugin note:** `sessions.NotifyUsers` (generic notification fan-in) is new. The
notifications store was always documented as generic (T-B2); C-CAL-RSVP-P1 is its first
writer outside the scheduler. Future features should use `NotifyUsers` with their own type
constant rather than adding a bespoke `NotifyX` per feature.

#### 2026-07-26 · C-CALV4-BLOCK-P1 (calendar-v4 wave 1, W-A) — the Block, render tier

New plugin-agnostic widget package **`internal/widgets/calendar_block`** plus
**`static/css/calendar-block.css`**. Nothing outside those two paths changed;
no route, no migration, no edit to any frozen calendar file. Full detail lives in
[internal/widgets/calendar_block/.ai.md](../internal/widgets/calendar_block/.ai.md);
the load-bearing points for anyone else in this repo:

- **Sizing is CSS container queries, and it has to be.** `boot.js:163` sets
  `htmx.config.allowScriptTags = false`, so a `<script>` inside an HTMX-swapped
  fragment never executes — a JS-sized widget silently renders at the wrong
  density after any swap. Size class comes from the host wrapper
  (`container-name: cal-block`; full ≥900 · std ≥300 · mini ≥240 · else
  sub-mini), density from each cell (`container-name: cal-cell`; measured column
  ≥84px → names). **This is the pattern any future self-sizing widget should
  copy**; `internal/widgets/calendar_block/sizing.go` carries the arithmetic in
  Go for tests only and is never called at render time.
- **`static/css/calendar-block.css` is UNLAYERED and self-contained** per
  `cordinator decisions/2026-06-05-rendering-canvas-css-exemption.md`, and every
  selector is scoped under `.cal-block-host`. An unlayered sheet outranks the
  app's layered CSS, so an unscoped rule in it would restyle the whole product;
  `TestCSS_EverySelectorIsScoped` parses the sheet and enforces that.
- **A second real-browser probe exists.**
  `container_query_probe_test.go` joins
  `calendar_v2_mobile_breakpoint_probe_test.go` as a test CI never runs (both
  skip under `-short`). Measured columns at the signed host widths are tabulated
  in the widget's `.ai.md`.
- **`data.go` is a cross-slice PIN** (cordinator
  `dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`). Editing the struct is a
  stop-and-flag: C-CALV4-SPINE-P2 writes what this package reads.

#### Static asset versioning (C-ASSET-VERSIONING, shipped inside C-CAL-MOBILE-VIEWS-FIX, 2026-07-26)

Every template-emitted static URL now carries a cache-busting `?v=<digest>`.
**This is a cross-cutting convention, not a calendar detail** — a new template
that writes `src="/static/…"` by hand fails a contract test.

- **`layouts.AssetURL(path)`** (`internal/templates/layouts/assets.go`) is the
  single helper. It appends the first 10 hex of the asset's SHA-256 when the
  file resolves (on-disk `static/` root, or a plugin embed FS registered via
  `layouts.RegisterAssetFS` — wired in `app.mountPluginStatic`), and falls back
  to a per-BUILD token (executable size+mtime) when it can't. Non-`/static/`
  URLs and URLs already carrying `?`/`#` are returned untouched. Digests are
  memoized per path for the process lifetime.
- **`middleware.StaticCache(prefix)`** (registered globally in `app.New`) turns
  those tokens into policy: `immutable, max-age=1y` when `?v=` is present,
  `max-age=0, must-revalidate` otherwise. The second arm is the safety net — an
  unconverted asset degrades to "always revalidated", never to "silently stale".
  Echo's `e.Static` sets NO Cache-Control at all, so before this browsers
  applied heuristic freshness and could hold build-old assets for hours.
- **Why now:** C-CAL-MOBILE-VIEWS-FIX Step 0 reproduced a live consequence.
  `md:contents` was a NEW Tailwind utility in C-CAL-MOBILE-AGENDA; rendering
  post-deploy HTML against a pre-deploy `app.css` drops the Week/Day/Timeline
  pills from the **desktop** calendar entirely, with every DOM test green. That
  is the "calendar looks old after deploy" class, demonstrated rather than
  assumed.
- **Guard:** `TestTemplatesUseAssetURL` walks every `.templ` in the repo and
  fails on a bare `src="/static/`/`href="/static/`. All 101 existing call sites
  across 17 templates were converted.

#### Closed arc: NW-2.2 plugin-isolation refactor

Per `cordinator/reports/chronicle/2026-05-23-c-plugin-isolation-audit.md` §3 (7 chunks A-G + D2-cleanup):

| Chunk | What | Status |
|---|---|---|
| A | Lightweight `PluginRegistration` registry (`internal/plugins/registry.go`); foundry_vtt + smtp pilots | ✅ shipped PR #334 |
| B | Magic-string consolidation (4 code + 2 templ sites) | ✅ shipped PR #332 |
| C | Cross-plugin import discipline docs (this file's §Cross-plugin imports above) | ✅ shipped PR #333 |
| D | Plugin-specific UI back into owning plugin (4 sub-refactors: banner / dashboard sync block / settings guide / show-banner fragment) | ✅ shipped PR #338 |
| D2-cleanup | Drop unused `fmBanner` + `maps.FoundryPresence` chains exposed by Chunk D; preserves campaigns.FoundryPresenceLookup (live diagnostic) | ✅ shipped PR #342 |
| E | Per-plugin `.ai.md` split + `status.md` shrink via archive-and-thin-index | ✅ shipped PR #335 |
| F | Per-plugin static-asset ownership via `embed.FS` (calendar pilot; other plugins migrate opportunistically) | ✅ shipped PR #336 |
| G | Packages plugin per-row foundry UI fragment; pattern for D. Blocks 2-4 deferred to G2-wave per reshape pattern | ✅ shipped PR #337 |

#### Closed arc: Wave 2 security work (2026-05-22 → 2026-05-26)

| Chunk | What | Status |
|---|---|---|
| 1 + 5 | Password-reset log scrub (`auth.hashEmail`) + `database.SafeIdent` DDL helper | ✅ shipped PR #331 |
| 2 (Phase 2B) | Focused AST middleware-pin for the Foundry public rate-limit invariant (`internal/wire/foundry_public_ratelimit_test.go`) | ✅ shipped PR #339 |
| 3-AMENDED | `syncapi.RequireJSONContentType` middleware + `v1Multipart` sub-group skip pattern (D-C3.1) | ✅ shipped PR #344 |
| 4-AMENDED | `loadDescriptor` fallback snapshot test pinning Chronicle defaults against canonical `chronicle-package.json` from Foundry-Module | ✅ shipped PR #343 |
| 6-AMENDED | Egress HTML sanitization on the 6 `/api/v1/*` GET handlers via `internal/plugins/syncapi/egress_sanitize.go` (D-C6.1); D4=(c) backup/restore lossless preserved | ✅ shipped PR #345 |
| 7 | File-level sanitize-on-write invariant (`internal/sanitize/invariant_test.go` + snapshot) | ✅ shipped PR #340 |
| 8 | `.ai/conventions.md §Security` consolidated reference | ✅ shipped PR #341 |

Wiki All-Pages mobile-layout cosmetic fix (`data-entity-id` on the shared `EntityTableRow` to match the bulk-actions widget's contract): ✅ shipped PR #346.

#### Open work

- **C-SEC-CHUNK-2-PHASE-2C** — full middleware-chain capture for every route via `golang.org/x/tools/go/packages`. Deferred from PR #339's reshape.
- **C-SEC-CHUNK-7-PHASE-2** — method-level sanitize invariant with flow analysis + helper tracing. Deferred from PR #340's reshape.
- **G2-wave** — packages plugin per-row foundry UI Blocks 2-4 (deferred from PR #337).
- **F2-wave** — remaining plugins migrate to per-plugin `embed.FS` static assets (deferred from PR #336; calendar was the pilot).
- **NW-2.3** — move `/foundry-presence` endpoint into the foundry_vtt plugin (currently lives on campaigns; was preserved by D2-cleanup as a parallel structure).
- **Draw Steel spin-up** — pending its own security audit per `cordinator/decisions/2026-05-26-draw-steel-spin-up-strategy.md`.
- **AI Export Pipeline** — scoping locked per `cordinator/decisions/2026-05-26-ai-export-pipeline-design.md`; implementation pending.
- **Plugin Host interface design pass** — tracked in `cordinator/decisions/2026-05-23-plugin-registration.md`. Deferred from Chunk A.
- **C-CAL-WORLDSTATE-EFFECTS-SYSTEM** — synced world-state animation (Almanac sky-band + hourglass) on `/demo/calendar/almanac`, mock-data only. Spec in `docs/design/world-state-effects/`. **Wave 0 + Wave 1 + Wave 2a/2b merged to main; Wave 2c (mood-tint) merged (PR #395)** — this closes the Wave 2 MUST set. Shipped: `worldState` + `setWorldState` pub/sub spine + unified `EFFECTS` registry (PR #388); sun supersession to inline `lorc/sun.svg` + hourglass interior (heightmap sand + day/night + glass/wood materials, PR #389/#390); 10-effect weather/celestial bundle (PR #391); ~28-option moon library w/ vendored Noto/Twemoji + 12 procedurals (PR #394); mood-tint wash (PR #395, merged — Wave 2 MUST closed). **Wave 3 (time-control verb layer) shipped** — `timepieceFill`/`atmospherePaused` state + verbs (+1hr/+1day/long-rest/custom/set-time/step-back/atmosphere-pause) tweened on the shared rAF (`CalParticleEngine.addTick`), fill caps ~1/3 → reuse dawn/dusk flip; reusable mechanics in `window.__calTimeControl`. Tests: `test/js/*.test.mjs` (`make test-js`) + Go static guards in `internal/templates/demo/calendar_*_test.go`. Visual verification is the operator's local gate (build env has no headless browser). **After Wave 3:** Waves 4–5 incremental effect long-tail + the production GM Live Control Panel (post-deadline). Queue in `.ai/todo.md §2`.
- **C-TIMELINE-V2-DESIGN-1-TUNER** — the "FM Tuner" timeline showcase on `/demo/timeline/tuner`, mock-data only, page-separated (own `cal-timeline-tuner.{css,js}`), raw SVG + CSS transforms (NO D3). Lead of two candidate timeline designs (Ledger alternate not yet built). Radio-dial etched-metal time axis through the canvas middle with adaptive tick notches (7 zoom levels millennia→days); swim-lanes above/below (entity/category/tier grouping); era gradient bands + watermarks; hover-revealed entity-color-coded connection arcs + show-all toggle; self-contained effect registries with `timelineAxisRender` + `timelineBackdropRender` hooks; §J2 restrained atmospheric backdrop (weather + non-routine celestial always; sun+moons ONLY on special-moon days); §J1 cursor-sync DOM-event protocol (`cal:cursor-change`/`cal:event-create`/`cal:date-jump`, loop-prevented, 50ms drag throttle) — **Almanac amended to emit/listen too** (small `cal-almanac.js` delta + `window.__calCursorSync`). Exempt-OKLCH canvas CSS carries the rendering-canvas exemption marker. Tests: `test/js/tuner.test.mjs` + `test/js/cursor_sync.test.mjs` (+ shared-harness event-bus addition) + Go render/discipline guards in `internal/templates/demo/calendar_timeline_tuner_test.go`. Visual verification is the operator's local gate. **Merged (PR #414).**
- **C-CAL-WORLDSTATE-WIDGETS** — Phase 6 widgetization: graduates the showcase worldState renderers into a production widget + an entity-page block, completing "all three views entity-able" (calendar #411/#413, timeline Tuner #414, worldstate here). New `entity_worldstate` block (`internal/plugins/calendar/entity_worldstate_block.*`) renders the "mini shelf view" (sky band + hourglass-on-shelf) — campaign-level, `Contexts:["template","dashboard"]`, Singleton, friendly empty(Create-calendar CTA)/unavailable states mirroring #413. The `worldState` **provider singleton** (`static/js/widgets/worldstate_provider.js`) is the one source of truth per page: ONE `/calendar/world-state` fetch regardless of widget count (or ZERO when a server seed is embedded), `subscribe`/`onError`/`current`/`push`, shared rAF, reduced-motion, self-destroy on last unsubscribe. The `worldstate` **widget** (`static/js/widgets/worldstate.js`, `Chronicle.register`) drives the SHARED engine (`cal-almanac.js`) via `window.__calSetWorldState` — engine reused, not rebuilt. Rendering canvas reuses the already-exempt `cal-almanac.css` (did NOT duplicate the marked exempt CSS). Tests: `test/js/worldstate_provider.test.mjs` + `worldstate_widget.test.mjs` + Go block tests; widget docs in `static/js/widgets/worldstate.ai.md`. **Merged (PR #415).** Wave-4 per-entity configurable attachment remains OUT of scope (post-deadline widget framework).
- **C-ENTITY-PERMISSIONS-UX** — three entities-plugin presentation changes (one PR): (1) entity card's single **3-state visibility badge** (Everyone `fa-globe` / DM-Only `fa-lock` / Custom `fa-shield-halved`), Scribe+ gated, cards only — `entityVisibilityBadge` in `entity_card.templ`; (2) **inline permission editor** — `permissions.js` gains an opt-in `data-layout="inline"` (the edit-form mount uses it) that renders the widget as a compact summary row expanding in place via the `grid-template-rows 0fr↔1fr` animation (reduced-motion safe), reusing 100% of the grant/load/save path (C-PERMISSIONS-SAVE-FIX intact); slide-in card unchanged for other callers; (3) read-only **Category › Sub-category lineage** in the edit form (`entityTypeLineage`), with `ParentTypeName` now populated via a LEFT JOIN in `entityTypeRepository.FindByID`. Tests: `entity_permissions_ux_test.go` (badge states + player-hidden, inline-layout opt-in, lineage with/without parent) + `test/js/permissions_inline.test.mjs` (inline build/expand/collapse/disclosure + slide-in regression). **Merged (PR #416).** Visual feel of the inline expand is the operator's local gate.
- **C-MAPS-EDITOR-PIN-AND-ICON-PARITY** (operator priority #3) — Chronicle-side of a cross-repo dispatch. **Part A (icon parity):** `internal/plugins/maps/marker_icons.go` is now the canonical source of truth for the 39-icon marker vocabulary; the editor picker renders from `MarkerIconGroups()` and `GET /campaigns/:id/maps/marker-icons` exposes `{default,icons,groups}` as the contract the **Foundry** sync module aligns to (Chronicle authoritative — Option 1 / §A4). **Part B (pin editing in-editor):** double-clicking the map (Scribe+) drops a pin without the separate "Place Marker" toggle (`doubleClickZoom` disabled to avoid the zoom conflict; toggle + marker-list management preserved); shared `MapEditorBody` → applies to the full map page AND the entity-page embed. Tests: `marker_icons_test.go` (catalog integrity/validation/groups, select-from-catalog, inline-create affordances, player-gating, marker-icons API). **FLAGGED:** the Foundry-side translation table (`scripts/map-sync.mjs`) + the create→Foundry→edit→Chronicle round-trip are a **separate Foundry repo/PR** (out of this session's repo scope) — can't be built or round-trip-verified here. **Merged (PR #417).** Inline-pin gesture feel is the operator's local gate.
- **C-AUTH-LOGIN-CSRF-FIX** (login-blocking, HIGH) — fresh logins were failing the double-submit CSRF check with a raw "invalid CSRF token". **Root cause:** `internal/middleware/csrf.go` named the cookie by scheme (`__Host-chronicle_csrf` HTTPS / bare HTTP); behind a TLS-terminating proxy the scheme derived on the POST could differ from the GET that set the cookie, so the lookup missed it and compared the form token against a fresh value → 403. **Fix:** read the cookie under BOTH names (`readExistingCSRF`) + hardened `schemeIsSecure` (parses comma-list `X-Forwarded-Proto`). **Part B:** friendly no-jargon 403 (`middleware.CSRFFriendlyMessage`, flows through `ErrorPage`/HTMX toast) + login auto-recovery (stale-token login POST → `GET /login?expired=1` via HX-Redirect/303 → re-issues token + friendly banner). Tests: `internal/middleware/csrf_test.go` (set→submit, both scheme-flip directions, recovery HTMX+plain, friendly-403, API skip). **Shipped.** ⚠️ Operator confirmed a real proxied login post-merge (CI can't reproduce the proxy/scheme condition).
- **C-APPS-CAL-DASH-W1** (E1 Wave 1 of 4) — the Calendars management dashboard as a **dedicated page** (`GET /campaigns/:id/apps/calendar`, Owner), reached from the Extensions hub's per-app "Open dashboard" entry for calendar (now a dedicated-page link via `campaigns.ExtensionDashboardPageURL`; the inline-panel mechanism stays for apps without a dedicated page). **List + detail-pane**: left list via `ListCalendars`; right detail **composes** the existing CRUD (open / settings / setup-wizard / delete / active-switch — no new CRUD) + a **read-only associations panel**. Two new reads, **no migrations**: `EntitiesForCalendar` (entity-ties, joined through `calendar_events`/`calendar_eras` since the link tables carry no calendar_id — corrects the audit) + timeline `ListByCalendar`→`ListTimelinesForCalendar`, exposed to calendar via a **service-interface adapter** (`calendar.TimelineLister`, wired in `app/routes.go` — no repo cross-import, respects plugin isolation). Friendly empty/error states (#413 pattern). Files: `calendar/app_dashboard.{go,templ}`, `entity_ties_repository.go`, `timeline/{repository,service}.go`. Tests: `calendar/app_dashboard_test.go` (+ `EntitiesForCalendar` passthrough), `timeline/list_by_calendar_test.go`, updated hub tests. **Merged (PR #419).**
- **C-APPS-CAL-DASH-W2** (E1 Wave 2 of 4) — live "see in action" embeds in the W1 detail pane, reusing shipped surfaces (no new widgets): LIVE worldstate band (`worldStateBandV2` — the production sky+hourglass the #415 block also wraps) **only when the selected calendar is the campaign's ACTIVE one** (engine-singleton nuance DEFAULT — non-active shows a friendly "set active" note, no widget surgery, no stop-flag needed); the engine-free month grid lazy-loaded via the existing `/calendars/:calId/embed` route (any calendar); per-associated-timeline `timeline-viz` widget mounts (reusing the shipped widget; D3 loaded at page level when timelines exist). **Design call (flagged):** dashboard selection is now a **full navigation** (list rows are plain hrefs, not HTMX detail swaps) because `htmx.config.allowScriptTags=false` means engine/D3 `<script>`s in a swapped fragment won't run, and the engine inits from its seed at load — full-load makes "one live worldstate surface + clean teardown" automatic. No new routes/migrations. Tests: `calendar/app_dashboard_test.go` (active-vs-non-active worldstate branch, grid lazy-load, timeline previews, D3 gating, full-nav rows); reused-widget mount/teardown is covered by the existing `worldstate_*`/boot.js lifecycle. **Merged (PR #420).** Visual feel is the operator's local gate.
- **C-WIDGET-BINDING-P1-SPINE** (the real Wave-4, P1 of 4; supersedes E1 W3/W4 — W1/W2 stand) — generic **host ↔ widget-type ↔ data-instance** binding. New **`widgetbindings` plugin**: a polymorphic **FK-free** `widget_bindings` table (`host_type` ∈ entity/entity_type/dashboard from day one; `widget_type` = registry slug; unique per host+type), a declarative `WidgetType` **Registry** (the dynamic-not-hardcoded answer; namespace guarded in app code, no DB enum), and a `Service` whose **precedence resolver** runs own-binding → entity-type template → **default (= today's behavior)** and returns `{InstanceID, Source}`. **Integrity kit (AND, not OR):** per-plugin delete hook (`OnInstanceDeleted`) + always-on render-time orphan guard (in `Resolve`) + periodic `Sweep`; **campaign scope** is in the repo signature (unscoped read unrepresentable) and checked on **host AND resolved instance** (the leakage vector; MariaDB has no RLS). **Calendar retrofitted** (`calendar_widget_type.go` + `EntityCalendarBlock` takes a resolved `calendarID`; unbound resolves to the campaign default = identical to pre-framework → #411–#420 unchanged). Built-now-dormant: the **entity-type template inheritance** path (unit-tested, surfaced as data in P4). **ADR-038** records why polymorphic-FK-free (the core-before-plugin migration rule makes the integrity-preserving alternatives un-collectable + forces per-type schema churn we forbid). Tests: `widgetbindings/service_test.go` (CRUD, precedence, **directional cascade** type≠>own per Foundry #9818, orphan guard ×3, campaign-scope on bind+resolve, source layer, all-host-types-storable). Sanitize snapshot +1 line (no HTML surface). **Merged (PR #421).**
- **C-WIDGET-BINDING-P2-WORLDSTATE-TIMELINE** (P2 of 4) — retrofits **worldstate** + **timeline** onto the framework as WidgetTypes (no new tables). **Worldstate's instance is a calendar id** (it's a view over a calendar's clock) under a **distinct `"worldstate"` slug** (shares `calendarInstanceBacking` with calendar), so a host can point its hourglass at a different calendar than its calendar embed. **Timeline's instance is a timeline record**; because the entity timeline block is a campaign-level **preview list** (not a single timeline), the timeline type has **no single default** — unbound keeps the list, bound renders the one timeline (via the shipped `timeline-viz` mount). `EntityWorldStateBlock(svc,cc,userID,calendarID)` + `BlockTimeline(cc,timelineID)` take a resolved id; unbound = identical to today. **Delete hooks wired** (P1 left them unconnected): a `BindingCleaner` interface injected into the calendar + timeline services via a type-asserted `SetBindingCleaner` (interfaces unchanged) → `DeleteCalendar` sweeps `calendar`+`worldstate` (both reference calendar ids), `DeleteTimeline` sweeps `timeline`. `InstanceExists` hardened with `errors.As` (the services wrap not-found via `%w` and `apperror.SafeCode` doesn't unwrap → a wrapped 404 must still be sweepable). Two new slugs are append-only registry entries (no schema). Tests: `calendar/widget_type_test.go` + `timeline/widget_type_test.go` (slug, in/cross-campaign + not-found + transient-error guard, default-vs-no-default, delete-hook per type). **Merged (PR #422).** P3 maps + dashboard-as-host · P4 create-or-pick UI.
- **PC-CLAIM-2** (Player Character Claiming, Stage 2 of 4) — addon + claimable flag. Registered the `player-character-claiming` built-in addon (CategoryPlugin/StatusActive, `fa-user-check`; startup seeder upserts it). Migration `000029` adds `entity_types.claimable BOOLEAN NULL AFTER parent_type_id` (NULL=unset→legacy heuristic; TRUE/FALSE=explicit Owner choice). `Claimable *bool` plumbed through `EntityType` + create/update inputs & form DTOs. **Repo hardening:** collapsed the former six-way entity_type SELECT/scan duplication into a single `entityTypeColumns` const + `scanEntityType(rowScanner)` helper every read routes through (`FindByID` resolves `ParentTypeName` via a follow-up PK lookup so it shares the one scan path) — removes the scan-order drift footgun the dispatch flagged; `claimable` added to the Create INSERT + Update SET. `isClaimableType` honours the flag (explicit wins; NULL → preset/slug heuristic). `CreateEntityType` gates player-character sub-types (`PresetCategory=="player_character"` or `slug=="player-character"`) on the addon via a service-injected `AddonChecker` (`entityService.SetAddonChecker`, wired in `app/routes.go`), rejecting with an "enable the Player Character Claiming addon" apperror when off and defaulting `claimable=true` when on. Tests: table-driven `isClaimableType` (flag precedence) + `CreateEntityType` gate in `service_test.go`, and a live-DB `TestEntityTypeRepository_Integration` (all six reads + INSERT/UPDATE) so `make test-int` proves the new scans. Verified `templ generate` / `go build ./...` / `make test-unit` (43 pkgs) / `make test-int` / `migrate-up`+`down` round-trip. **Merged (PR #481).** Next: Stage 3 UI (per-type toggle, owner overview) · Stage 4 Foundry actor-sync mapping.
- **PC-CLAIM-3** (Player Character Claiming, Stage 3 of 4) — the UI for the addon + flag. **Owner on the character page:** the claim block is extracted to `claim_banner.templ` (`claimBanner`) and now shows "Claimed by &lt;DisplayName&gt;" when owned (Show handler resolves `owner_user_id` → `CampaignMember.DisplayName` via the existing `memberLister`, "a player" fallback for a stale owner). **GM owner overview:** new `category_owner_roster.templ` (`claimRosterPanel`) renders on a claimable category dashboard for **Scribe+** — every character with its owner, a reassign `<select>` (members) and an Unclaim button, both PUT `…/entities/:eid/owner` via `Chronicle.apiFetch`; the Index handler assembles a `ClaimRoster` (full type list capped 100 + members + owner-name map) only when Scribe+ AND `isClaimableType` AND addon-on, threaded through `CategoryDashboardContent` *inside* `#entity-list` so the existing search/sort outerHTML swap is preserved. **Per-type toggle:** `EntityTypeCard` + the Add-Category form gain a "Players can claim entities of this type" checkbox **only when the addon is on** — the quick-edit save rides `claimable` on its PUT (initialised from the effective `isClaimableType`), the create form posts `claimable=true` when checked; `EntityTypesPage`/`CreateEntityType` compute `claimingEnabled`. **Claim gating:** the unclaimed banner now requires addon-on **AND** `isClaimableType(type)`. Plugin isolation preserved (reused the in-package `AddonChecker` + `memberLister`; no new cross-plugin import). Tests: `claim_overview_test.go` (pure helpers + isolated component renders for the banner/roster/type-card gating). Verified `templ generate` / `go build ./...` / `make test-unit` (43 pkgs) / `go vet`. **Merged (PR #482).** Next: Stage 4 Foundry actor-sync addon-aware mapping.
- **C-WIDGET-BINDING-P3a-MAPS** (P3a of 4) — read-side maps retrofit; maps is the original `entity.map_id` precedent the framework generalizes. New `maps/map_widget_type.go` registers a `"map"` WidgetType (instance = a map id; `InstanceExists` = `GetMap` + campaign-scope, `errors.As` wrapped-404 guard; **no campaign default** — the legacy fallback lives in the closure). The `map_editor` block closure resolves the map id through the framework with a **legacy fallback**: default = today's `entity.map_id`; a `widget_bindings` row (widget_type="map") **wins** when present; **unbound = identical to today** (column drives the embed/picker/empty branches). `DeleteMap` delete-hook wired via the same type-asserted `SetBindingCleaner` seam (interfaces + mocks untouched); the legacy `entity.map_id` is independently SET-NULLed by `fk_entities_map_id` (maps migration 005). **No migration, no schema change** — the `entity.map_id` backfill→bindings + column drop is a **deferred** span-the-layers migration (after P3b). Tests: `maps/map_widget_type_test.go` (slug, in/cross-campaign + not-found + transient guard, no-default, binding-wins, delete-hook). **Shipped.** Deferred: **P3b** dashboard-as-host (unify `DashboardBlockSwitch` onto `BlockRegistry.Render` so `host_type='dashboard'` resolves) · the `entity.map_id` migration · **P4** create-or-pick UI.
- **C-CAL-WORLDSTATE-WIRE** (2026-07-26) — `calendar.worldstate.changed` + `calendar.weather.zones.changed` were **publisher-side dead letters**: live emitters (`worldstate_service.go` `SetWorldState`, `service.go` `SetWeatherZones`) but no `case` in `calendarEventPublisherAdapter.PublishCalendarEvent` (`internal/app/routes.go`), so both hit its `default: return` and were discarded before the bus. Every meteor, eclipse and weather-zone edit the operator authored had never once reached a WebSocket client — which is why "celestial events unfindable/unsyncable" had no trace to follow. Traced by the Foundry executor in Chronicle-Foundry-Module PR #82. **Three additive parts:** (1) new public types `ws.MsgCalendarWorldstateChanged` / `ws.MsgCalendarWeatherZonesChanged` + the adapter cases, with `internal/app/routes_calendar_ws_test.go` pinning the WHOLE calendar mapping (a `default: return` is a silent-drop machine — the next emitter added without a case must fail CI, not a session); (2) `WorldStateChangePayload` — a strict superset of the old `{date, moodTint}` (those paths preserved because the module shipped `formatWorldstateLine` against them) adding celestial `events[]` with the **stable `calendar_celestial_events` id**, `moons[]`, `weather` and an `audience` marker, **SPLIT BY AUDIENCE**: player-safe always, plus a full copy under the internal `calendar.worldstate.changed.dm` name that the adapter maps to the same public type with `RequiresDM` set (the hub gates per-message, so one message cannot be rich for the GM and redacted for the table); (3) `GET /api/v1/campaigns/:id/calendar/world-state` on the Bearer syncapi group, dm_only-gated by `resolveRole` exactly as every other syncapi calendar read. Enrichment is best-effort — a failed load degrades to the minimal payload rather than dropping the signal. `WorldStateEvent` gained `id` + `visibility`; the showcase parity test is unaffected (it pins top-level + moon keys). **ADR-047**; `internal/plugins/calendar/.ai.md` §"World State on the Wire"; `internal/plugins/syncapi/.ai.md`. Tests: `worldstate_wire_test.go`, `routes_calendar_ws_test.go`, `calendar_worldstate_handler_test.go`; `routes_snapshot.txt` +1.
- **C-CALV4-TIEFIX-PB** (2026-07-26, calendar-v4 wave 1 verify-then-fix carve-out) — two live bugs under the v4 remodel's entity-page calendar Block, both confirmed by source reading before fixing. **Bug 1:** `EventsForEntity` (`entity_ties_repository.go`) scanned 37 destinations against 39 selected columns (`eventCols`' 38 + `l.participation_role`) — missing `&evt.RecurrenceDayOfWeek`/`&evt.CollectRSVPs`, the same landmine class C-CAL-RSVP-P1 closed in `scanEvents` but never ported to this file — so every entity-side tie read failed at runtime and the entity-page calendar embed silently showed no tied events. Fixed + `event_scan_contract_test.go` widened (now file-parameterized) to cover this third consumer, proven to fail pre-fix / pass post-fix. Also added `EventsForEntityFiltered` (new method, existing signature untouched) closing a calendar-level visibility gap this path never enforced (a `dm_only` *calendar*'s event was reachable through an entity tie) — composed from existing repo methods rather than widening `CalendarRepository`, per COMMON's Bounds; not yet wired into `entity_calendar_block.go` (owned by a different slice). **Bug 2:** timeline `EventCount` (`internal/plugins/timeline/repository.go` `List`/`ListByCalendar`) used unconditional `COUNT(*)` subqueries while the row-returning `ListEventLinks`/`ListStandaloneEvents` filtered by visibility — a player-visible count/row mismatch that differenced into a count of hidden events. Fixed by folding the same predicate into both subqueries; **PARTIAL** — `visibility_rules` (Go-side `canUserView`) remains a residual gap SQL can't see, stated honestly per COMMON ruling #9, booked for the coordinator. **§18-C confirmed already closed** (PR #565) — not re-fixed. See `internal/plugins/calendar/.ai.md` §"Entity-side tie reads" and `internal/plugins/timeline/.ai.md` §"EventCount visibility fix".
- **C-CALV4-BENCH-P4** (2026-07-28, calendar-v4 wave 1 phase B) — the nav Calendar tab lands on **the Bench**. `GET /campaigns/:id/apps/calendar` keeps its path exactly (the nav target and the Extensions hub both carry it as bare strings, one pinned literally; a path change forces a `routes_snapshot.txt` regeneration) and `AppDashboard` becomes a thin handler delegating to new files: `calendar/bench.{go,templ}`, `bench_test.go`, and the unlayered self-contained `static/css/calendar-bench.css`. **The proportion rule is structural:** `benchClassify` promotes ONE primary calendar and AT MOST one real-world calendar to full Blocks; everything else is a subordinate row, at every width and under every sort key. **Permission is absence, and counted:** a player's ribbon is three tiles (Today / Next up / Session) — Sync, Needs attention and Horizon are never BUILT for them, so there is nothing in the DOM to grey or hide. **Honesty states:** the fault prints where the date would go with no date element (the warn TREATMENT is GM-only, the ROW is not — a player's own misconfigured calendar must not vanish silently); design-ahead surfaces carry the signed `needs backend` chip instead of the mockup's fabricated "RSVP 3 / 5", "9 days fogged" and hardcoded "all 11"; the sync denominator never drops in any state. **NEXT UP is viewer-filtered at the source** via P2's `UpcomingAcrossCalendars` — never `UpcomingByCalendar`, which filters base visibility in SQL only and leaks a `visibility_rules` event's NAME to a player — and `benchSortKeys` feeds the shipped `?sort=nextevent` control from that same filtered index rather than reopening the leak. **No N+1** on the page every player lands on: hydration goes through the spine's batched `EagerLoadCalendars` rather than the nine-queries-per-calendar `eagerLoad`. The owner/player list split (`ListCalendars` vs `ListVisibleCalendars`) is preserved verbatim. Host layer set = moons/eras/weeknums/ledger/shelf per the DEF ruling (`cordinator decisions/2026-07-28-calv4-def-and-zone-chips-ruling.md` §1); `moongraph`/`horizon` omitted and measured, not assumed. The four `app_dashboard_*_test.go` pin sets refreshed intent-first (route-page assertions now render the Bench; retained-but-unrouted card-grid component tests marked as such, never deleted). No new routes, no migrations, `data.go` byte-pin clean, `routes_snapshot.txt` byte-identical. Headless fidelity shots at 1232px and 358px, light and dark, GM and player. **Booked, not fixed:** `/apps/calendar` rides the authenticated group while `/calendar/v2` is public-capable, so an anonymous visitor to a PUBLIC campaign lands on `/login` — a route-group change plus a snapshot regeneration, i.e. a wave-2 slice. See `internal/plugins/calendar/.ai.md` §"The Bench".
- **C-CALV4-SHELF-P7** (2026-07-28, calendar-v4 wave 2, W-E) — **zone D is filled and the Block is architecturally complete.** `shelf_stub.templ` → `shelf.templ` (marker kept: the seam suite keys on `data-zone="shelf"`) plus a new `almanac.templ`. **Upcoming** reuses the Ledger's row primitive VERBATIM off the same `newLedgerView` pass — month-scoped exactly as drawn ([S1]), capped at four later rows and SAYING it capped — so the count oracle's new seventh reading is an equality rather than a hope. **Filters** ships the tab and not the engine ([S2]): nothing in Chronicle backs a filter and no preference store exists, so the panel is one `needs backend` chip with no controls and no pin field, and W-F is NAMED as the filling wave. A player gets neither the tab nor the panel ([S10]) — the needs-backend-audience ruling one level down, as absence rather than a disabled control, and the first per-role difference inside a chrome strip. **The Almanac** is the slice's gate and L21's other half: pin **r53** adds `MonthGeometry.Almanac`, a per-month celestial register produced **UNCAPPED by `MoonCap`** ([S5]) — the grid's three-moon ceiling is legitimate only because every declared body is carried here at full width, and before this the fourth moon existed in the database, was counted by `MoonsDeclared`, and was drawn nowhere. Three sub-tabs (Tonight · Month · Moons), every number computed from the month's REAL day count: the mockup's four thirty-day literals are FIXED and not ported, and `TestCSS_NoLiteralWeekLength` now covers the lane, which takes its day columns from `grid-auto-flow` so no month length exists as a literal or a variable `repeat()` anywhere in the sheet. No epithet ([S6]) and no node-window bracket — neither has a column. The nameplate badge's `— all of them are in the Almanac` tail is RESTORED and gated: a Block with the Shelf hidden (`noShelf`) or the shelf layer off still withholds it. **Three findings the string suites could not see**, all caught by a new browser probe written red-first: the §12 std collision came back with a filled Shelf and was answered inside the Block's own std geometry (the Shelf yields; no host layer key dropped, no pinned host file opened); W-B's day-pick ladder FILTER reached the Shelf's rows and made choosing a day reflow the Block 618→532px (one word, plus a guard that now checks the filter selector); and the strip clipped its last sub-tab at 358px instead of scrolling. Plus a harness defect: W-B's probe isolated only the day radio group, so with a third group on the page every box but the last measured a collapsed Shelf — the collision gate was measuring a state the product cannot reach. **DEF still `["moons"]`, both host layer sets unchanged, no new zone flags, no routes, no migrations, no JS**; `bench_test.go`'s chip pin re-stated as an ENUMERATION ([S12]) naming every surviving chip by file:line. ADR-048 §10-§14. See `internal/widgets/calendar_block/.ai.md` and `internal/plugins/calendar/.ai.md` §"calendar-v4 Shelf + the Almanac register".

### Archive

`.ai/archive/` holds historical docs that have served their purpose:

- `status-2026-04-25-pre-shrink.md` — the 1198-line chronological session log that lived at `.ai/status.md` until 2026-05-23. Pre-Phase-4 session recaps live here.
- `phase-d-plan.md` — Phase D sprint plan (Phase D shipped)
- `security-audit-2026-03-06.md` — the original security audit (superseded by `cordinator/reports/chronicle/2026-05-22-c-security-audit.md`)
- `plan.md` — Sprint V-2 (backlinks panel + entity aliases) implementation plan (work shipped)
- `plan-drawsteel-2026-03.md` — Draw Steel system module implementation plan (work shipped; moved here from a stray repo-root `plan.md` by C-DOC-DRIFT-REFRESH-R2)
- `todo-completed-2026-06-10.md` — completed-todo archive (moved 2026-06-10)

### IMPORTANT RULES (mirrored from CLAUDE.md)

Per `cordinator/decisions/2026-05-19-dispatch-workflow.md`:

1. Session-work deliverables → committed PR on the target repo + a Cordinator status report (`reports/chronicle/YYYY-MM-DD-<dispatch>.md` on the Cordinator repo's `main` branch).
2. Plugin-scoped status updates → append to the owning plugin's `.ai.md` "Recent Work" section. Don't bloat this file.
3. Cross-cutting decisions → new file in `cordinator/decisions/` + cite from code.
4. This file's "Cross-cutting state" section gets updated when an arc advances or a release ships.
