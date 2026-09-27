# Header customization and nav polish — build plan

**APPROVED overall by the operator 2026-09-12 after six render rounds; the
addenda below supersede the July layout where they conflict. Order of work:
`2026-09-12-build-order.md`.**

**The sidebar's structure is #739's, not this file's:** the Apps drawer
(round-2 ruling 3, slice N1) is superseded by the signed "Pinned + folding
sections" sidebar, where apps and categories stay on screen as folding
sections. The living-ring ruling at the foot of this file stands.

**Ruling:** the customizable header was designed in July, signed by the
operator, and never built. **Revive it; do not redesign it.** The nav's
structural work (one item model, one reorder mechanic — C-NAV-V3) shipped and
matches the code; it needs a polish slice, not a rethink.
**Executor:** the `go-dev` agent; design questions go back to the lead,
never guessed. **Owner-facing summary:** the top bar becomes yours — your
name or logo, editable in place; a row of small live widgets you pick; the
campaign switcher folded into the name. Two pieces (a moving sky background,
a mini calendar) wait for the calendar rebuild.

## Where the design lives (read in this order — it is the spec)

1. `Cordinator/plans/2026-06-03-topbar-customization-vision.md` — the ask.
2. `Cordinator/plans/2026-07-07-campaign-chrome-v2-design.md` — decisions
   D1–D19. **Binding.**
3. `Cordinator/mockups/campaign-chrome-v2.html` + `mockups/chrome-directions/
   e-final-mix.png` — the signed layout and the operator's visual pick.
4. `Cordinator/dispatches/chronicle/C-CHROME-{P1,P2,P3,BG}.md` — the four
   slices as written in July. **Stale anchors; re-baseline before use.**

## Step 0 — re-baseline (one agent, read-only, before any code)
For each of D1–D19 and each dispatch: does the file/line it names still
exist, and does the design still apply? Three drifts are already known:
- **D-"living sky" background mode:** the sky engine was deleted with the
  calendar (2026-08-21). **Defer to the skypane standalone project.** Nothing
  in H1–H3 may depend on it.
- **Compact-calendar and weather widgets:** the calendar plugin is domain-
  only until V5. **Their registry slots are declared; their bodies wait.**
- **The accent-trio rename (D14):** already shipped under different names as
  Site / Action / App accent (`C-ACCENT-SLOTS`, PR #541,
  `campaigns/branding.templ:557-634`). **Done; do not redo.**
Output: a table D1–D19 → keep / done / defer, with fresh `file:line`.

## Slices

### N0 — nav polish (independent of Chrome V2; can go first)
1. **Hover ghosting.** `C-NAV-HOVER-ANIM-FIX` was dispatched for "~6 boxes
   flash mid-sweep" on `sidebar-nav-glow` (`static/css/input.css:344-438`);
   no completion report exists and the CSS carries no fix comment. **Verify
   in headless Chromium first** (the sandbox has it); fix only if reproduced.
   The glow itself is operator-approved and must survive identically.
2. **Retired-tab deep link.** A dashboard card links to the retired Settings
   "Features" tab (blank page). Find it (`grep -rn "tab=features"`), point it
   at `/campaigns/:id/extensions`.
3. **The live inline-script bug.** `campaigns/branding.templ:675-710` (the
   Surface Accents card) carries an inline `<script>`; `boot.js:212` strips
   it on a boosted swap, so reaching Customize → Appearance by clicking the
   sidebar likely lands with dead handlers. Convert to inline IIFE `onclick`
   per STANDING_ORDERS §3 and add the file to the ratchet
   (`tools/check-page-scripts.sh`, `.ai/todo.md` item H).
4. **One save model.** That same card saves by direct PUT per click while
   every other Appearance control uses the staged draft/save bar
   (`branding.templ:365-373`, `appearance_editor.js`). Fold it into the
   staged flow. Two save models on one page is how settings get lost.
Mobile: **keep the off-canvas drawer.** No bottom bar, no icon rail — nothing
in the books asks for one and inventing one is the trap.

### H1 — the widget host *(P1, re-baselined)*
Go-registered widget-type registry; `TopBarSnapshot` with a data budget;
per-widget error isolation; portal popovers. Ship with the widgets that need
no calendar: **search, quick-note, custom text, links, era pill** (era is
timeline data — confirm at re-baseline). Calendar/weather slots declared,
empty, labelled honestly ("after the calendar rebuild"). Extend
`CampaignSettings` (`campaigns/model.go:540-560`) with `TopBarWidgets`; do
NOT add a parallel settings model — the nav-v3 retrospective names "two data
models" as the root of three bugs.

### H2 — brand in place, switcher merged *(P2)*
Brand name editable inline for Owner (today: a field on the Appearance tab);
the campaign switcher (`app.templ:1190-1217`) folds into the brand name
per the mockup. Keep `GetDisplayName` (`data.go`) as the single source.

### H3 — the editor *(P3)*
The customization UI for H1/H2 per `campaign-chrome-v2.html`. Staged save,
same bar as Appearance.

### H-BG — living sky *(deferred; skypane project)*

## Constraints every slice inherits
- `isolate` on `<header>` (`app.templ:1152`) is load-bearing for the
  background layer; never remove. Popovers portal to `<body>`.
- No `templ script`, no inline `<script>` bodies in anything reachable by
  a boosted sidebar link. Inline IIFE `onclick` only.
- `sidebar-nav-glow` unchanged in effect.
- `SidebarItem`/`SidebarConfig` extended, never paralleled.

## Acceptance
`make verify` + `make test-js`; screenshots at desktop and 390px against the
signed mockup (headless Chromium); the `reviewer` pass. H1–H3 additionally:
the re-baseline table is linked in the PR and every D-number it keeps is
cited at the code that implements it.


## Addendum — operator rulings 2026-09-12 (round 2 of the renders)

Canvas: https://claude.ai/code/artifact/d554c339-512f-4f01-9ead-06cafd69df90
(Chrome page). Four rulings, in the operator's words, then what they change:

1. "the chronicle in the top left … needs to be able to be changed by the
   owner as well" → the brand in the **sidebar header** (logo + name) is
   Owner-editable **in place**: pencil on hover opens the name as an input,
   Enter saves through `UpdateBranding` (40-char cap unchanged), Esc
   cancels; click the logo to change it. D13's in-place editing moves from
   the header to the sidebar header. The Appearance field stays as the
   fallback.
2. "Wouldn't it be better to have campaigns in the navbar, vs the header?"
   → **one way to switch, in the nav.** The switcher folds into the sidebar
   brand (menu: current ✓, others, All campaigns = the old list page, New).
   The header picker (`app.templ:1190-1217`) and the sidebar "Campaigns"
   link both retire. **D16's brand card in the header is superseded**; the
   header's left slot shows the current path instead (see direction C).
3. "Journal, NPCs, Armory, Characters, etc should be in an apps drawer,
   kinda like how the parent categories work" → **N1 (new slice): Apps
   drawer.** Zone 2 becomes Dashboard · Apps ▸ · Categories ▸ · My
   Characters. Apps opens the same slide-over the categories use (Back row,
   then the addon shortcuts in owner order). The sidebar editor edits drawer
   items inside the drawer (reorder, hide) over the same unified `items`
   model — no second model. This supersedes N0's "no structural redesign".
4. "the ability to just have a page customized for the header, like if they
   wanted an image" → **H3 becomes its own Customize → Header page**: brand
   (name, logo), background (solid / gradient / animated gradient / image
   still-or-animated ≤1.5 MB with scrim / sky-deferred), widget list, one
   staged save. Today's Top Bar Style + Topbar Content cards fold into it.

**Open (operator picks):** how the current page is shown — A rail-and-tint,
B sliding marker (200 ms, jumps under reduced motion), C echo in the header
(the freed left slot reads the path), or a mix. **Lead's recommendation:
A + C.** The row lights in the sidebar; the header slot reads the path,
because inside a drawer the sidebar cannot show the leaf page. B adds
motion with nowhere to glide once drawers exist. N0's hover-glow verify still
stands; the glow itself is no longer sacred if the pick replaces it.

5. (round 3) "Can the campaign selector actually be a slide out from the
   right side … a search bar and filters inside of it?" → **yes, ruled.**
   **Corrected round 4:** the panel comes out of the NAV's own right edge
   (a 340px flyout in the sidebar palette, sliding from under the sidebar,
   ~220ms, lying over the page with a scrim), NOT the viewport's right edge.
   Contents: search box, role chips (All / Owner / Scribe / Player with
   counts), Active / Archived, current campaign first then by last played,
   New at the foot, Manage = the old list page. Search and filters render
   only past a handful of campaigns; on a phone it fills the drawer. This
   replaces the dropdown drawn in round 2. Build it as the sidebar's second
   slide-over (the categories panel is the first); it is not the
   permissions card.

**Where-am-I, round 3:** the operator leans C + A and asked for three more:
D traced ring (their idea: on click a 2px line runs the button's perimeter,
direction random, ~350ms, then settles into the lit row; reduced motion =
no trace), E the icon fills (ripple from the icon, icon stays solid), F
folder tab (the active row takes the page colour and joins it at the edge).
All drawn with C's header path alongside. **CLOSED 2026-09-13 — see the
ruling at the foot of this file.**

**Where-am-I, round 4:** the operator likes D and wants the page name kept in
the row (A's sub-label), and clarified D is CONTINUOUS: two traces that keep
circling and reversing, not a one-shot. Drawn moving on the canvas in three
strengths (lively / calm / still). Lead's recommendation: **calm at rest,
flare on hover and click, still under reduced motion**, plus C's path in the
header. Implementation: one SVG rect per active row, `pathLength="100"`,
two `stroke-dashoffset` keyframe tracks with uneven reversals, CSS only, no
script loop; `prefers-reduced-motion` disables it. Lively-at-rest is a
permanent repaint on every page and a permanent eye-pull; if the operator
wants it anyway, make the strength an Owner setting on Customize → Header
with the user's reduced-motion preference always winning. **CLOSED
2026-09-13 — see the ruling at the foot of this file.**

**D, corrected (round 5, operator):** the traces ride the FULL glow border
— the row's own box, `sidebar-nav-glow::before` inset 0, square corners —
not an inset ring. Three states: **rest** nothing; **hover** = today's
right-edge glow unchanged, with the two traces sitting on it and tugging a
few percent back and forth, going nowhere; **active** = the two traces on
the full border at a middle speed, each drifting in LENGTH and PACE on its
own long uneven cycle (14s / 19s, ease-in-out per segment), now and then
one closing the whole loop, never quick ("we don't want to freak out ADHD
users"). Page name kept in the row (A's sub-label). CSS only: one SVG rect
pair per active row, `pathLength="100"`, keyframes on `stroke-dasharray` +
`stroke-dashoffset`; `prefers-reduced-motion` → Still.

**Ruled: Owner settings for it, on Customize → Header** ("Navigation
highlight" card): Motion Still / Calm (default) / Lively · Traces One / Two
· Length Short / Long / Varies · On hover Quiet / Tug · Page name Hidden /
In the row. Colour follows the Chrome accent. Reduced-motion members always
get Still; a per-user "turn nav motion off" lives in the account menu.
Extend `CampaignSettings` (`campaigns/model.go`), never a parallel model.
**D + C is the pick in all but name; awaiting the operator's word.**
*(That word arrived 2026-09-13. See the ruling at the foot of this file.)*

## Addendum — round 6 (2026-09-12): styles, not knobs; the Customize page

- **Switcher:** filters fold by default (one "Filters ▾" row with the live
  summary chips); the filter set is role, Active/Archived, and **Public**
  (`Campaign.IsPublic`, already a model field).
- **D corrected again:** rest shows NO page name (you are not in that app).
  Hover = today's glow shape rendered as two traces touching end to end
  (top-right corner → right edge → two thirds of the bottom), tugging at each
  other, never parting. Click = they part and circle the full border at a
  middle speed with drifting length and pace; the page name appears in the
  row.
- **The setting is a choice of STYLE, not knobs on D** (the lead misread it
  in round 5). Owner picks one on the Customize page → Navigation: moving
  styles D living ring (operator's), G comet, H breathing, I tide; still
  styles J rail and tint, K folder tab, L edge-lit, M solid icon; plus
  Strength (Calm/Lively) for the moving ones and Page name (Hidden/In the
  row). Every moving style names its still fallback for reduced motion.
  Canvas page "Customize" holds the drawings, all live.
- **The Customize page today, measured:** seven cards on the Appearance tab
  (`branding.templ` 520-800: Backdrop Image, Brand Name, Site accent, Surface
  Accents, Top Bar Style, Topbar Content, Font) and one preview
  (`#appearance-preview-root`, `branding.templ:388-470`). The preview IS
  wired — `static/js/widgets/appearance_editor.js` (1006 lines) mounts on
  `data-widget="appearance-editor"` (`branding.templ:376`) and updates
  accent, brand, top bar style, font and backdrop — but each in one small
  spot, and nothing else on the demo site reacts (May audit §8.4:
  accent-weighted ~90%). That is why the operator calls it broken; a
  runtime break on top of that is NOT verified from here — first executor
  task: open it in a live session and record what actually moves.
- **"More colour options" = `C-THEME-V2` (`Cordinator/plans/BACKLOG.md:
  930-973`), scoped in May, never started.** Cleanup A (bugs), Cleanup B
  (one save model, no JS-injected UI — a CI guard), Cleanup C (the
  expansion: elevation presets, motion presets, heading font, and colours
  beyond the accents — drawn as tones: Sidebar Charcoal/Ink/Tinted, Page
  Cool/Warm/Paper, Text contrast Standard/High, plus more accent presets
  and custom hex on every slot), Preview rebuild (the demo site fed by the
  draft at three zoom levels: the faux site, a hover-me sample card for
  elevation + motion, swatches). **Proposed shape:** one Customize page,
  sections down the left (Brand · Header · Navigation · Colours · Type ·
  Motion and depth), the demo site always in view, one staged save.
  Header (H3) and Navigation (N1 + the highlight style) are sections of
  it, not separate tabs.


## RULED 2026-09-13 — the nav highlight is settled. Build it.

The operator confirmed **D, the living ring, at CALM strength**, with C's
header path alongside. This closes the question that stood open through
rounds 3, 4, 5 and 6, and it unblocks N1/N2 and the Customize page.

**Exactly what ships** (this is the spec; it is the operator's own round-5
and round-6 corrections, not a fresh interpretation):

- **Rest:** nothing. No traces, no page name. You are not in that app.
- **Hover:** today's right-edge glow, unchanged in effect, rendered as two
  traces touching end to end — top-right corner, down the right edge, two
  thirds along the bottom — tugging at each other a few percent and going
  nowhere. They never part on hover.
- **Active (click):** the two traces part and circle the FULL glow border —
  the row's own box, `sidebar-nav-glow::before` inset 0, square corners, not
  an inset ring — at a middle speed. Each drifts in LENGTH and PACE on its
  own long uneven cycle (14s / 19s, ease-in-out per segment); now and then
  one closes the whole loop. Never quick. The page name appears in the row.
- **Reduced motion:** style J (rail and tint), always, for any member whose
  system asks for it. A per-user "turn nav motion off" lives in the account
  menu and also wins.

**Implementation:** CSS only. One SVG rect pair per active row,
`pathLength="100"`, keyframes on `stroke-dasharray` + `stroke-dashoffset`.
No script loop — a JS animation loop on every page is the thing this design
is specifically avoiding. Colour follows the Chrome accent.

**Why Calm and not Lively.** Lively-at-rest is a permanent repaint and a
permanent eye-pull on every page in the app. The operator's own constraint
was "we don't want to freak out ADHD users." Lively remains selectable by an
Owner on Customize → Navigation; it is simply not what a new campaign gets.

**Still a STYLE choice, not knobs.** The Owner picks one style from the eight
drawn in round 6 — moving: D living ring, G comet, H breathing, I tide;
still: J rail and tint, K folder tab, L edge-lit, M solid icon — plus
Strength (Calm/Lively) for the moving ones and Page name (Hidden/In the row).
Every moving style names its still fallback. Extend `CampaignSettings`
(`campaigns/model.go`); never a parallel settings model — the nav-v3
retrospective names "two data models" as the root of three bugs.
