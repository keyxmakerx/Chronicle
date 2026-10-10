# UI standard

How Chronicle looks and behaves, as the operator has decided it. Every UI
change is checked against this page; a change that departs from it needs a
signed mockup that says so. Tokens and helpers named here are the shared
pieces to reuse, not copy.

## How it should feel

Six rules every screen is judged against.

- **It answers at once.** Every click or key shows a change within a tenth of
  a second: a press, a colour, a spinner.
- **Things come from where you clicked.** A menu grows out of its button; a
  card peeks out of the name it describes. Only things that live at an edge,
  like the phone menu, slide in from that edge.
- **Weight matches what's at stake.** Browsing is light and quick. Powerful
  actions (entering Site admin, Publish, Delete, Send invites) press deeper
  and let go slower; nothing else gets the heavy press.
- **Nothing bounces or waits on show.** Motion slows to a stop, with no wobble
  or overshoot, and nothing runs past half a second unless it marks a big
  moment. Entering Site admin is the one signed exception (under Motion).
- **Calm is always respected.** Under Calm nothing travels; under Off, or on
  a device set to reduce motion, things simply appear.
- **One look, many campaigns.** The frame is quiet and the same everywhere;
  a campaign's colour, pictures and fonts bring the character, and the
  owner's Customize choices go further (below).

## What is on a page

- **Nothing is on a page by default except player characters.** Everything
  else (sub-pages, posts, backlinks, inventory, game-system panels, player
  notes, RSVP) is a widget the owner places in the layout or on the
  dashboard. A feature switch makes a widget *available*; it never draws
  one. `internal/plugins/entities/page_extras.go` is the pattern for moving
  an auto-drawn panel into a placed block without changing existing pages.
- **One copy.** A panel that can be placed is never also drawn automatically,
  so placing it never shows it twice.
- **A switch is named for what you see**, and turns on one thing. Feature
  descriptions say what the owner gets, never a library or a protocol.
- **Two sidebars exist**: inside a campaign (`campaignNavTop`,
  `campaignNavCategories` in `internal/templates/layouts/nav.templ`) and
  outside one (`app.templ`). A navigation fix covers both.
- **Site admin is its own place.** On an admin page a site admin's sidebar is
  the admin menu (`AdminSidebarNav`): a shade darker, under a "Site admin"
  band, with "Back to Chronicle" returning to the page they came from.
  Everywhere else the only trace is the footer's Site admin button
  (`SiteAdminEntry`, the tools icon `SiteAdminIcon`), in both sidebars.
- **Who sees what follows one check.** DM-only content is visible to everyone
  who can write it: the owner and members with DM access
  (`CampaignContext.VisibilityRole()` to see it, `CanAuthorDmOnly()` to write
  it), never `MemberRole` compared to `RoleOwner`.

## Lists an owner edits

Every list a DM edits (calendar months, weather kinds, shop stock, sidebar
rows, tags, quests, tables) has **add, rename, remove with a confirm, and
reorder**. Lists are built for hundreds of rows: search or filter, paging or
folding, counts in a pill sized to its number.

## Opening and closing

- **Fold-outs and panels open on click and close on a click off them**, or
  Escape. Hover never opens something you act in.
- **Hover is for a peek**: a preview of who or what, appearing where the
  pointer is (the sidebar peek, availability bars). A click then opens the
  full panel.
- **Content previews use the one hover card**, `Chronicle.hovercard`
  (`static/js/hovercard.js`): linked pages, rule words, and any package widget
  preview. A caller describes the content; the card owns behaviour and look,
  so no feature draws its own tooltip. It opens after the pointer rests
  250 ms and stays while the pointer is on it; keyboard focus opens it and
  Escape closes it, returning focus; one card at a time, kept inside the
  window. A click on a link still follows it (a long press pins it on
  touch); a click on a rule word pins the card with a close button.
- **The hover card's look is the owner's choice** (Customize › Hover cards:
  Paper, Plain, Night, Compact; Paper by default). Paper stays paper in dark
  mode, and its tokens and classes (`static/css/paper.css`) are the shared
  paper look for every paper-styled page.
- **Panels open with an animation inside their own widget**: they grow out of
  the thing clicked, not slide in from the window edge or from under another
  element.
- **A click off a panel with unsaved changes warns first.** Register edits
  with `Chronicle.markDirty(id)` / `markClean(id)` (`static/js/boot.js`); a
  closing panel checks `Chronicle.isDirty()`.
- **Big editors pop out** over the page, as the calendar editor does, rather
  than living as a long form in a settings tab.
- **Actions that change a lot ask first** (downtime, removals, resets). A
  removal says what goes and whether Trash can bring it back.

## Motion

Every animation is one of six moves. Each has a Calm version that fades in
place, and Off shows the end state.

| Move | Used for | Full | Calm |
|---|---|---|---|
| Peek | hover cards, the sidebar peek, availability bars | fade and a 4px rise, `--dur-micro`, `--ease-out` | fade only |
| Grow | menus, panels, pop-out editors, out of the thing clicked | scale from the clicked edge, `--dur-standard`, `--ease-out` | fade only |
| Fold | sections, calendar months, long lists | to the content's real height, `--dur-standard` | opens at once |
| Slide-out | sheet tabs, creator steps, slips under a value | out from under its surface, `--dur-slide`, `--ease-slide` | appears in place with a fade |
| Page turn | the Handbook only | a turn with a soft crease, `--dur-turn`, `--ease-turn` | the new spread fades in |
| Settle | moving to a new page or tab | fade and a 6px rise, `--dur-large`, `--ease-out` | fade only |

- **Entering Site admin rises and bounces**, the one bounce in Chronicle.
  Only after the footer button, the admin menu comes up from the bottom of
  the screen, gathering a little speed, and bounces off the top three times,
  each smaller, over `--dur-arrive`; then the band's accent rule draws
  across. The button presses deeper and lets go slower than other footer
  buttons (`static/js/site_admin.js`, `.admin-arrive`). Moving between admin
  pages plays nothing. Calm and Off show the menu in place.
- **How much motion is one attribute**: `html[data-motion]` is `calm` or `off`
  (absent means full), from `MotionLevel` in
  `internal/templates/layouts/appearance.go`. A device asking for reduced
  motion is always Off (the first-paint script in `base.templ`); the owner's
  "Calmer for everyone" (`data-cz-reduce`) makes the campaign Calm; a person's
  My view choice can lower that to Calm or Off, never raise it. Calm and Off
  both also write `data-view-motion="calm"`, the "wants less motion" flag
  older scripts read. After a change on the page, `Chronicle.syncMotion()`
  (`static/js/theme.js`) works it out again.
- **Calm is enforced in `input.css`**: under Calm only fades and colour
  changes transition, and keyframe animations jump to their end, so a move
  built from `opacity` plus `transform` becomes its Calm version by itself.
  Script-driven motion checks `data-motion` (or the older flags) and does the
  same.
- **Folds measure their content.** A fold, roll or unfurl animates to the
  content's real height (measure `scrollHeight`, animate, then release to
  `auto`), so it never clips, jumps, or expands after a cut-off frame. No
  fixed `max-height` guesses.
- **Looping animations rest when the person steps away.** Every loop takes
  its clock from `window.MotionRest` (`static/js/motion_rest.js`) and stops
  requesting frames once `still()`. One-shot transitions don't need it.
- **Durations and easing come from the tokens** in `static/css/input.css`;
  the owner's Motion speed retimes all of them, the paper moves included.
- **Effects have depth and stay light**: no flat pastel fills, no bounces or
  overshoot outside Site admin's entrance, and nothing that loads a weak
  device. Heavy effects check what
  the device can do first.
- **Zoom is gentle**: one wheel tick is a small step, never a jump from far to
  close.

### Every interaction

Each interaction maps onto the moves above with these timings. Calm is the
column's version; Off shows the end state.

| Interaction | When | Full | Timing | Calm |
|---|---|---|---|---|
| Hover | pointer over a row, card or button | colour fill; cards lift one depth level | 120ms colour, 200ms lift | colour only |
| Press | any button | sinks 4% with an inner shadow | 60ms down, 200ms up | colour only |
| Heavy press | Site admin, Publish, Delete, Send invites only | sinks 10%, lets go slowly | 70ms down, 280ms up on `--ease-slide` | colour only |
| Keyboard focus | tabbing onto anything | accent ring | 80ms, no movement | same |
| Switch | a setting on or off | knob slides, track fills | 160ms in-out | knob jumps, colour fades |
| Menu or picker | dropdowns, a row's ⋯ menu | Grow out of its button | opens 200ms ease-out, closes 120ms ease-in | fade |
| Dialog | confirming, a small form on top | backdrop fades; dialog grows from 96% and rises 6px | opens 200ms, closes 150ms | fade |
| Drawer | the phone menu, an edge panel | slides in from its own edge | 280ms ease-out | fade |
| Hover card | resting on a name or rule word | Peek after 250ms | 120ms | fade |
| Fold | sections, groups, more details | to its real height; arrow turns | 200ms ease-out | opens at once |
| Tab switch | changing tab or view | Settle | 280ms ease-out | fade |
| Another page | any link to a new page | thin accent progress line, then Settle; the menu stays | 280ms settle | line and fade |
| Adding to a list | a new page, a tag | the row opens its space, fades in, glows briefly | 200ms, glow fades over 1.2s | fade and glow |
| Removing from a list | deleting, unlinking | the row fades and its space closes | 160ms ease-in | fade, space closes at once |
| Dragging | reordering, moving between groups | lifts to depth 3 with a slight tilt; drops into place | 120ms pick up, 200ms drop | lift shadow only |
| Count changes | a badge or number updating | the new number grows in from 70% | 280ms | number swaps |

## Look

- **Modern is the look.** Every page, panel and widget is built in
  Chronicle's modern look first: the theme tokens below, the six moves under
  Motion, light and dark. Mockups show that look first, always.
- **Paper is an optional extra, mostly for players**: a look that some
  in-world things can wear (hover cards, a character sheet, the Handbook)
  where a signed mockup gives it to them, never a default for a new feature
  and never a reason to restyle a tool. Where it exists it builds on the
  shared paper tokens and classes in `static/css/paper.css`, never its own
  colours, and moves with the same six moves as the modern look. The owner's
  Customize › Depth sets how far paper lifts; Flat drops the tucked sheet
  behind it. Further looks may come later, the same way.
- **The tools are never paper**: both sidebars, the header, ordinary pages,
  lists, the calendar grid, maps, game nights, settings, Customize, admin,
  dialogs and forms use the theme tokens below.
- Chronicle's current look, on the real page as it is on `main`: theme
  tokens (`bg-surface`, `text-fg`, `text-fg-muted`, `border-edge`,
  `text-accent`), never hard-coded colours, and both light and dark.
- **Colour roles**: page `--color-bg-primary`, card and panel
  `--color-bg-secondary`, hover and alternate rows `--color-bg-tertiary`, menu
  `--color-sidebar-bg`, headings `--color-text-primary`, labels
  `--color-text-secondary`. The accent (`--color-accent`, the campaign's
  colour) marks what you can act on and where you are, never decoration.
  Green, amber and red (`--color-ok-rgb`, `--color-warn-rgb`,
  `--color-bad-rgb`) only ever mean state.
- **Depth has four levels**: 0 flat (pages, lists, `--elev-static`), 1
  resting (cards, buttons, `--elev-resting`), 2 lifted (hover, menus, hover
  cards, `--elev-hover`), 3 above (dialogs, dragging, `--elev-dragged`). A
  thing rises one level when hovered or picked up and returns when let go; a
  pressed button sinks below level 1 with an inner shadow. Dark mode uses
  deeper shadows and lighter surfaces, not glows.
- **Corners**: 8 for controls and cards, 12 for dialogs, drawers and page
  panels, 4 for tags, 6 for small things, pills for counts and status labels.
  Nothing sharper than 4 or rounder than 12 except pills.
- **Type**: 24 bold page titles, 20 semibold section headings, 16 semibold
  card headings, 14 reading text and controls, 12.5 labels, hints and
  timestamps, 11 caps group labels. Inter unless the campaign picks its own
  fonts; reading text stays near 70 characters wide; numbers in columns are
  tabular.
- **Spacing is on steps of 4** (4, 8, 12, 16, 24, 32, 48): 8 inside a
  control, 12 to 16 between related things, 24 to 32 between sections. Gaps
  belong to layouts, not to each item.
- Icons are monochrome Font Awesome in the text colour; colour is for state,
  not decoration.
- Buttons and pills size to their content; nothing stretches full width after
  a state change.
- Words on screen are plain and short. No developer terms, IDs, or version
  strings that aren't real. A button says what happens ("Publish") and its
  message confirms it ("Published"); an error says what went wrong and how to
  fix it, without apologising.

## Every control has every state

A control isn't finished until it has rest, hover, pressed, keyboard focus,
working and off states.

| Control | Hover | Pressed | Focus | Working | Off |
|---|---|---|---|---|---|
| Button | fill, lifts to depth 2 | the press above | accent ring | spinner and "…ing" label, can't be pressed twice | faded |
| Powerful action | darkens | the heavy press | accent ring | spinner and "…ing" label | faded |
| Text field | no change | n/a | accent edge and soft ring | "Saving…" beside it | faded, not editable |
| Menu row | hover fill | fill, slight press | ring inside | n/a | faded, with the reason on hover |
| Switch | knob shadow grows | knob slides | accent ring | knob waits until saved | faded |

## Waiting and feedback

- **Under 0.3 seconds, show nothing extra**; a flash of a spinner reads as a
  glitch.
- **A button's action** shows a spinner in the button with what it's doing
  ("Saving…"), and it can't be pressed twice.
- **A page or panel loading** shows grey placeholder lines in the shape of the
  content after 0.3 seconds, then the content fades in over them. Under Calm
  the placeholders stay still.
- **Small saves** (typing, toggles) confirm in place with a green "Saved" tick
  that fades.
- **Bigger actions** (create, publish, invite) get a message at the bottom for
  three seconds, with Undo when it can be undone.
- **Errors stay until dismissed**, sit next to the thing that failed, and say
  how to fix it. Nothing shakes.
- **Deleting asks first** in a dialog naming exactly what goes, unless it can
  be undone.

## Mockups and sign-off

- A visible change gets a mockup first, on the real page as it is on `main`,
  at shipped quality: never placeholders or a lean out-of-context fragment.
- Motion decisions are shown as playable clips, never stills.
- A signed mockup is the contract until a new one is signed. Small
  differences found while building go back as a "small differences" card.
