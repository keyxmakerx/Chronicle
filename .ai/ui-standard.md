# UI standard

How Chronicle looks and behaves, as the operator has decided it. Every UI
change is checked against this page; a change that departs from it needs a
signed mockup that says so. Tokens and helpers named here are the shared
pieces to reuse, not copy.

This is the **Material** look: things on screen behave like physical sheets
(they have an edge, sit at a height, cast a shadow from one light and move
only the way a real sheet could), and the page is as minimal as it needs to
be, not as minimal as possible. It grew from the Annals page look and is the
only look new work is designed in. Two older looks are still on parts of the
site and are replaced when a change touches them (under **Older looks**).

## How it should feel

Eight rules every screen is judged against.

- **Minimal as needed, not as possible.** Type, spacing and thin rules carry
  the structure. A box, edge or fill is there when it means something (a fact
  panel, a field, a selection, a tool, a sheet that lifts above the page), so
  a page is neither boxed all over nor bare. One colour, the campaign's
  accent, says where you are and what you can act on.
- **Everything is physical.** Every surface is a sheet with an edge and a
  height, lit from one light above. Every move has a physical reason: it
  shows where the thing was, where it goes, and what it slid over or out
  from under. A sheet that opens over content lies on top of it; it never
  shoves the content aside unless the content is making room on purpose.
- **It answers at once.** Every click or key shows a change within a tenth of
  a second: a press, a colour, a spinner.
- **Things come from somewhere and go back there.** A menu grows out of its
  button, a card peeks out of the name it describes, a linked page pulls out
  from under the block that holds the link, an editor slides out from behind
  the part it edits. Closing reverses the same path. Nothing pops into
  existence and then moves; only things that live at an edge, like the phone
  menu, slide in from that edge.
- **Weight matches what's at stake.** Browsing is light and quick. Powerful
  actions (entering Site admin, Publish, Delete, Send invites) press deeper
  and let go slower; nothing else gets the heavy press unless the owner picks
  Keystone buttons in Customize.
- **Nothing bounces, jumps or waits on show.** Motion slows to a stop, with
  no wobble or overshoot, nothing moves before its transition starts, and
  nothing runs past half a second unless it marks a big moment. Three signed
  exceptions: entering Site admin and the Customizer's layout tiles, which jiggle
  while the pointer is over their section (both under Motion), and a style
  kit's own opening and pointer response, which may settle or sway as its
  row under **Style kits** says.
- **Calm is always respected.** Under Calm nothing travels; under Off, or on
  a device set to reduce motion, things simply appear.
- **One look, many campaigns.** The frame is the same everywhere; a
  campaign's colour, pictures, heading face and style kit bring the
  character, and the owner's Customize choices go further (below).

## Two kinds of screen

Every screen is one of two kinds, and is designed in that kind's register.

- **Pages** are what people read: an entry's page, the campaign home, a
  category's list, the time line, a character's sheet, the Handbook. They use
  the quiet look under **Look**: no cards around prose or lists, headings in
  the campaign's heading face, hairline rules between rows, the accent on
  labels and "you are here". A new page block or package widget is drawn
  this way first and marks the shared block parts (under Customize) so every
  page look can style it.
- **Workshops** are where the owner arranges and configures: the
  Customizer, the calendar, map and book editors, settings, Site admin.
  They are framed tools on the theme tokens (`customize_shell.css` is the
  model: a rail on the left, the work in the middle, an inspector on the
  right, one save bar). A workshop shows the page it changes as the real
  page, never an abstract grid or a form describing it.
- **Paper is an optional extra, mostly for players.** Every surface already
  behaves like a sheet (The material rules); paper is only the optional
  texture and colours: a look some in-world
  things can wear (hover cards, a character sheet, the Handbook) where a
  signed mockup gives it to them, never a default for a new feature and
  never a reason to restyle a tool. Where it exists it builds on the shared
  paper tokens and classes in `static/css/paper.css`, never its own colours,
  and moves with the same moves as everything else. The owner's Customize ›
  Depth sets how far paper lifts; Flat drops the tucked sheet behind it.
- **The tools are never paper**: both sidebars, the header, lists, the
  calendar grid, maps, game nights, settings, Customize, admin, dialogs and
  forms use the theme tokens.

## The material rules

How the physical idea turns into pixels. Each rule names what a real sheet
would do.

- **One light, from above.** Shadows fall straight down and a little
  forward, grow softer and wider the higher a sheet sits, and are tinted by
  the surface under them, never a grey blob or a sideways offset. A sheet
  lifted on hover casts the next level's shadow (`--elev-*`) and nothing
  else changes shape.
- **Sheets stack, they don't shove.** A pull-down, a peek, a hover card or a
  slip lies over the content below it, with its shadow on that content, and
  takes its space back when it closes. Content moves aside only when it is
  making room on purpose: a fold opening in a list, a docked panel, a row
  joining.
- **Pull-downs drape from what holds them.** A title's drawer, a "last seen"
  list or a widget's settings unrolls downward from the edge it hangs from,
  like a blind, over the page beneath; it rolls back up the same way.
- **Edges are a vocabulary, not a habit.** A page uses only these, each for
  its meaning:

  | Edge | Means | Used for |
  |---|---|---|
  | none | it is the page itself | prose, headings, lists of rows |
  | hairline rule | the next thing starts here | between rows, under a block heading |
  | full hairline edge | a sheet laid on the page | fact panels, pictures, a lifted card, menus, hover cards |
  | field edge | you can type here | text fields, pickers |
  | leading accent edge (2px) | selected, or where you are | a selected row, the current section |
  | leading amber edge (3px) | only the Director sees this | `data-dm-only` rows and blocks |
  | dashed edge | a place for something, not a thing yet | drop targets, an empty slot, "Add a widget" |

  A sheet that already lifts with a shadow needs no heavier edge than a
  hairline; two edges never stack on one thing.
- **Small by default, bigger on request.** A widget on a reading page opens
  at its smallest useful size (one line, a count, the latest item) and
  unfolds or pulls down for the rest; nothing is drawn at full size just
  because it can be. Design every list for one item, a few and many (hundreds),
  and show all three in its mockup.
- **Hit areas are generous.** Anything that answers to the pointer reacts
  over at least 32 by 32 pixels (44 on touch), even where the mark you see is
  smaller, such as a rail tick, a dot on a line or a spot on a picture.
- **Every open thing shows its way out.** A panel, drawer, pinned card or
  pull-down carries a visible close (or Done) control in the same place on
  each, as well as Escape; nobody has to guess where to click to leave.
- **Floating tools can be moved, pinned and resized, and step aside.** A
  tool that floats over the page (Jot notes, the dice tray, the AI helper on
  a reading page) can be dragged by its top edge, resized from its corner and
  pinned to a side, remembers where it was, and slides out of the way when
  something it would cover needs attention (a dialog, the part being
  edited), coming back after.
- **Long pages show their contents.** A page with several sections offers a
  contents rail or list that is easy to find without hunting (at the top or
  in the margin) and follows the reader.
- **Controls come in four weights, each for its place.** Like edges, a
  control's weight says what it is, and the lightest one that fits wins. What
  opens always grows out of what was clicked.

  | Weight | Looks like | For |
  |---|---|---|
  | Quiet word | text in the accent, no border, fill or shadow; underline on hover | revealing more inside reading content: "Show all 6", "why" |
  | Tab | small capitals with a hairline under them and up one side, like an index label; no fill, box or lift | a drawer or fold that hangs below it on that same side line, such as a title's pull-down |
  | Icon | a monochrome icon, no frame until hover | a margin, a rail, a row's end, or anywhere a word won't fit; always labelled for screen readers |
  | Button | border, fill, resting shadow | a real command ("Publish", "Add reason") or a value picker (a standing, "Sort by") |

  A fold or a reason can also open from the thing it belongs to (its name,
  its row, its heading) with no control at all. A full button is never the
  default way to reveal more, and one view never stacks several buttons where
  quiet words would do.
- **No emoji, anywhere.** Icons are monochrome Font Awesome; a symbol on
  screen is never an emoji, in templates, scripts or messages.

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
- **Site admin is its own place, the control room.** On an admin page a site
  admin's sidebar is the admin menu (`AdminSidebarNav`), under a "Site admin"
  band, with "Back to Chronicle" returning to the page they came from and
  "Leave Site admin" doing the same from the top bar. Admin pages are always
  dark, on a deeper blue-black with red as the accent (`html[data-site-admin]`
  in `input.css`), so they never pass for the everyday site; labels and
  figures are set in mono. A status strip runs along the foot of every admin
  page (`AdminStatusStrip`): the site is live, sessions signed in, the last
  backup's age, and that every change is on the record in Admin activity.
  Everywhere else the only trace is the Site admin row at the very bottom of
  both sidebars (`SiteAdminEntry`, the tools icon `SiteAdminIcon`): a dark
  plate edged in admin red with a red icon, whose hover brightens the edge
  rather than the everyday glow; its icon alone when the menu is folded.
- **The table is one tab away.** Every campaign page has one "Table" tab in
  the campaign header. It opens a drawer of three leaves, like a DM screen;
  each leaf picks its own section, and the arrangement is remembered for each
  person. The drawer is Material surfaces, never paper. A roll arrives as a
  slim island that slides out from behind the Table tab and back, styled by
  the style kit; clicking it, or a row of the roll log, drapes the roll's
  details down from it. T opens the table and R the rolls, never while
  typing; each person has quick 2D dice. The Handbook's Rules ribbon and its
  P key stay as they are (TODO(#1475)).
- **Who sees what follows one check.** DM-only content is visible to everyone
  who can write it: the owner and members with DM access
  (`CampaignContext.VisibilityRole()` to see it, `CanAuthorDmOnly()` to write
  it), never `MemberRole` compared to `RoleOwner`. It is marked with the
  Director's amber (under Look) wherever it is drawn, so the owner always
  knows what players won't see.

## Lists an owner edits

Every list a DM edits (calendar months, weather kinds, shop stock, sidebar
rows, tags, quests, tables) has **add, rename, remove with a confirm, and
reorder**. Lists are built for hundreds of rows: search or filter, paging or
folding, counts in a pill sized to its number. A long list opens a row in a
peek beside it (the list stays, the arrow keys move the peek) rather than
leaving the list for another page.

## Editing

- **Two names, two jobs.** The **page editor** is where you write a page's
  words. **The Customizer** is where you place widgets and arrange pages:
  a category's home, the campaign home and its versions for each role, and
  the campaign's look. Screens, buttons and docs use these two names and
  never call one by the other's.
- **Edit on the real page.** An owner changes a page, a layout or a home
  page where it is drawn, as players will see it; empty widgets fill with
  content marked "Example". A workshop's preview is the real page's own
  markup, made inert, never a sketch of it.
- **An editable part says so at rest, three ways.** While editing is on,
  each editable part has a pen beside its heading, an icon with no word, so
  it is findable without hovering, on touch and with the keyboard; hover and
  focus then outline the part with an accent hairline; and E (or the E in the
  editing hint) outlines every editable part on the page at once until
  pressed again. Hover alone is never the only way to discover it.
- **A part opens where it stands.** Its pen, or a click on the part, opens
  it: the pen becomes its tick in the same spot, its values become fields in
  place with no fill, only a line under them that turns the accent on focus
  (a title's own rule is its writing line), and what has no room drapes on a
  sheet below it, over the page. Esc closes the open thing first, then leaves
  editing. Each field saves as it is typed, and the page says Saving… then
  Saved.
- **An editor never covers what it edits.** It opens from the part (a
  popover for a few fields, a panel docked beside the page for a long form)
  and moves aside rather than sit on the part. A small part may edit in
  place; a part too big for that opens the docked panel.
- **One clear editing state.** When editing is on, the page says so (the
  edge in the campaign colour, Done where Edit page was) and nothing else
  about the page changes.
- **Arranging saves as one draft; a single choice saves at once.** Moving,
  adding and resizing many things (Customize, a page layout) collects into
  one draft with the save bar (`customize_shell.js`): what is waiting, Undo,
  Redo, Discard, Save changes, and Ctrl+Z, Ctrl+Shift+Z and Ctrl+Y. A single
  pick elsewhere (a field, a switch, a blank in a sentence) saves the moment
  it is made and confirms in place.
- **Find with "/".** A workshop with many settings has Find: "/" outside a
  text field, or the Find button, opens it, and it jumps to the setting by
  name.
- **Single-key shortcuts never take a typed letter.** A shortcut with no
  Ctrl, Cmd or Alt (E, "/", ?) does nothing while focus is in a text field,
  a text area, a select or anything editable (`isContentEditable`), and
  never calls `preventDefault` there, so the letter is typed. It works only
  in its own view (the page being edited, the open workshop), never on keys
  pressed anywhere else on the page. Each one also has a button, so the
  shortcut is never the only way.
- **Preview who and where.** A workshop that changes what people see offers
  Phone, Tablet and Computer, and "A player", so the owner sees what each
  gets before saving.
- **The AI helper never moves or covers the work.** In a workshop it docks
  beside the page, narrowing it; on a reading page or a sheet it is a
  floating tool (under The material rules) that sits beside the part it is
  helping with and can be moved, pinned or closed. It never slides in over
  the sheet it is working on.

## Opening and closing

- **Panels open on click and close only on purpose**: their visible close
  button, Escape, or a click on what opened them. A stray click elsewhere never
  closes a panel you are working in. A small menu or picker (a dropdown, a
  blank's list) closes on a click off it, losing nothing.
- **Hover is for a peek**: a preview of who or what, appearing where the
  pointer is (the sidebar peek, availability bars). A click then opens the
  full panel. Hover never opens something you act in.
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
- **The popup on a time line is the owner's choice** (Customize › Hover
  cards › Popups on time lines): the classic hover card, or the fading
  corner, which has no box. Its corner sits on the event's bead on a short
  stem, an edge in the campaign colour runs up the side and along the
  bottom, each fading from the corner, over a background solid behind the
  text that feathers out; it flips to the bead's left near the right edge.
  New campaigns start on the fading corner. It reaches the page as
  `data-cz-tlpop="corner"` on `<html>`, read by `page_timeline.js` (through
  the hover card's `corner` option) and `timeline_river.js` (its peek).
  In a style kit, the line under a page's title and both popups are drawn
  in the kit (signed in the time line mockup,
  https://claude.ai/artifact/9VLxhJvNYDooSeaByKfxt6): the classic card
  becomes a small box with the kit's edge and shadow, and the corner takes
  the kit's radius, edge and fill. Every other hover card keeps its look.
- **The character sheet style is the owner's choice** (Customize › Character
  sheets; Modern by default). The choice reaches the page as `data-cz-sheet`
  on `<html>`, which `static/css/sheet_styles.css` and `sheet_motion.js` read.
- **A rulebook always wears its game system's look.** Its colours, `look`
  and fonts come from the book, and Customize has no setting that changes
  them. The book editor's own panels always use the standard dark colours, so
  they stay readable whatever the book's theme; only its live page takes the
  book's.
- **Side panels stay beside the page.** The page peek
  (`static/js/peek_panel.js`) opens a linked page read-only in one side panel
  (a bottom sheet on a phone) on Shift-click, Shift+Enter or the peek icon
  beside a link, so two pages can be read together. A list's row peek, the
  AI helper and a docked editor work the same way: beside the page, closed by
  their close button or Escape.
- **Panels open with an animation inside their own widget**: they grow out
  of, or slide from behind, the thing clicked, never in from the window edge.
- **Leaving unsaved changes warns first.** Register edits with
  `Chronicle.markDirty(id)` / `markClean(id)` (`static/js/boot.js`); a
  closing panel or a page change checks `Chronicle.isDirty()`.
- **Big editors pop out** over the page, as the calendar editor does, rather
  than living as a long form in a settings tab.
- **Actions that change a lot ask first** (downtime, removals, resets). A
  removal says what goes and whether Trash can bring it back.

## Motion

Every animation is one of nine moves. Each has a Calm version that fades in
place, and Off shows the end state.

| Move | Used for | Full | Calm |
|---|---|---|---|
| Peek | hover cards, the sidebar peek, availability bars | fade and a 4px rise, `--dur-micro`, `--ease-out` | fade only |
| Grow | menus, pickers, dialogs, pop-out editors, out of the thing clicked | scale from the clicked edge, `--dur-standard`, `--ease-out` | fade only |
| Fold | sections, rows that join or leave a list, calendar months | to the content's real height, pushing what follows down, `--dur-standard` in, `--dur-micro` + 40ms out | opens at once |
| Drape | pull-downs: a title's drawer, a widget's longer list or settings | unrolls down from the edge it hangs from, over the page with its shadow, nothing below moves; rolls back up, `--dur-standard`, `--ease-out` | fade only |
| Slide-out | sheet tabs, creator steps, slips, an editor from behind its part | out from behind its surface, then in front of it, `--dur-slide`, `--ease-slide` | appears in place with a fade |
| Pull | following a connection, opening a list row's peek | the new content slides out from under the block that holds the link while the old stays and dims, `--dur-slide` × 1.22, `--ease-slide` | fade only |
| Page turn | the Handbook only | a turn with a soft crease, `--dur-turn`, `--ease-turn` | the new spread fades in |
| Settle | moving to a new page or tab | fade and a 6px rise, `--dur-large`, `--ease-out` | fade only |
| Glow | something new or something you came from | an accent glow that fades over about 1.2s | the same |

- **Slide-out means from behind, then in front.** The moving surface starts
  hidden behind its source, slides clear of it, then comes forward, quick but
  never a snap. A surface that appears first and slides afterwards is wrong.
- **Site admin rises in and falls away**, the one bounce in Chronicle.
  After the Site admin row, the whole admin page comes up from the bottom of
  the screen over the page you were on, gathering a little speed, and
  bounces off the top three times, each smaller, over `--dur-arrive`; the
  band's accent rule draws across as it lands. "Back to Chronicle" lets the
  admin page fall away down the screen over `--dur-leave`, and the page you
  came from is underneath. Both are cross-document view transitions, asked
  for only by those two clicks (`static/js/site_admin.js`) and by the next
  page before it paints (`static/js/site_admin_reveal.js`,
  `html[data-admin-vt]`); a browser without them shows the admin page rising
  by itself on the way in, and on the way out the admin page falls by itself
  over the everyday page colour before the next page loads. The row presses
  deeper and lets go slower than other menu rows. Moving between admin
  pages plays nothing; Calm fades, and Off plays nothing.
- **Annals: following a connection pulls the page out from under it.**
  Under the Annals page look, a click on a link in the connections block
  leaves the page being read where it is, scroll included, and dims it, while
  the linked page slides out from under the block, from the side the block
  sits on, over `--dur-slide` × 1.22 with `--ease-slide`. The block stays,
  opaque, casting a shadow onto the page beneath, and swaps its links in
  place: links both pages share move, links only the old page had fold away,
  and the new page's own links fold in with a glow, the page you came from in
  the accent. Following the link back to the page the last pull came from
  slides the page back under. It is a same-document view transition around
  the usual boosted swap of `#main-content` (`static/js/annals_pull.js`,
  `html[data-annals-mode]`); the scroll resets only after the old page is
  captured, so nothing jumps before the move. Calm fades; Off, reduced
  motion, unsaved changes, a modified click and a browser without view
  transitions keep the ordinary page load. The browser's Back button is the
  ordinary history restore and plays nothing.
- **The title's rule draws on arrival.** Under Annals the accent rule under
  a page title draws from the left over `--dur-large`, after 120ms, under
  full motion only.
- **A creature's pictures settle into its hover card.** A card with both art
  and a token (`badge`) opens with the art easing from slightly close into its
  frame over 320 ms, `--ease-out`, and the token popping onto the corner
  140 ms later over 260 ms with a small overshoot. Calm and Off show both
  still while the card fades.
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
- **A move is captured before anything changes.** A view transition takes
  its picture of the old state before the page scrolls, swaps or reflows, so
  nothing jumps somewhere else first.
- **Looping animations rest when the person steps away.** Every loop takes
  its clock from `window.MotionRest` (`static/js/motion_rest.js`) and stops
  requesting frames once `still()`. One-shot transitions don't need it.
- **The Customizer's layout tiles jiggle while you are over them.** In the
  Customizer's page layout, tiles rock by a fraction of a degree only while the pointer is
  over their section, so the loop rests the moment the person moves away; the
  tile under the pointer, a selected tile and a dragged one hold still. Calm,
  Off and reduced motion turn it off. It rocks 0.55° each way every 0.36 s,
  every third tile a little slower so they don't move in step.
- **Dragging a widget in the Customizer** opens a gap at its size where it
  will land. The gap moves once the pointer rests on a new spot for 110 ms
  and has moved on from the last one, so the widgets it pushes never move it
  back. The widgets either side of the gap glow, and every widget that moves
  glides to its place over 340 ms. Calm keeps them still.
- **Durations and easing come from the tokens** in `static/css/input.css`;
  the owner's Motion speed retimes all of them, the paper moves included.
- **Effects have depth and stay light**: no flat pastel fills, no bounces or
  overshoot outside Site admin's entrance, and nothing that loads a weak
  device. Heavy effects check what the device can do first.
- **Zoom is gentle**: one wheel tick is a small step, never a jump from far to
  close.

### Every interaction

Each interaction maps onto the moves above with these timings. Calm is the
column's version; Off shows the end state.

| Interaction | When | Full | Timing | Calm |
|---|---|---|---|---|
| Hover on a row | pointer over a link row, list row or connection | a soft fill; under Annals also the resting shadow, no movement | 120ms | colour only |
| Hover on a card or button | pointer over a card in a workshop, a button | fill; lifts one depth level | 120ms colour, 200ms lift | colour only |
| Press | any button | sinks 4% with an inner shadow | 60ms down, 200ms up | colour only |
| Heavy press | Site admin, Publish, Delete, Send invites only | sinks 10%, lets go slowly | 70ms down, 280ms up on `--ease-slide` | colour only |
| Keyboard focus | tabbing onto anything | accent ring | 80ms, no movement | same |
| Switch | a setting on or off | knob slides, track fills | 160ms in-out | knob jumps, colour fades |
| Menu or picker | dropdowns, a row's ⋯ menu | Grow out of its button | opens 200ms ease-out, closes 120ms ease-in | fade |
| Dialog | confirming, a small form on top | backdrop fades; dialog grows from 96% and rises 6px | opens 200ms, closes 150ms | fade |
| Drawer | the phone menu, an edge panel | slides in from its own edge | 280ms ease-out | fade |
| Docked panel | a peek, the AI helper, a docked editor | the page narrows as the panel slides out from behind its edge | 200ms ease-out | fade |
| Hover card | resting on a name or rule word | Peek after 250ms | 120ms | fade |
| Fold | sections, groups, more details | to its real height; arrow turns | 200ms ease-out | opens at once |
| Pull-down | a title's drawer, a small widget's full list | Drape from its edge over the page | 200ms ease-out, 150ms back up | fade |
| Tab switch | changing tab or view | Settle | 280ms ease-out | fade |
| Another page | any link to a new page | thin accent progress line, then Settle; the menu stays. Under Annals a connection Pulls instead | 280ms settle | line and fade |
| Adding to a list | a new page, a tag | the row Folds open, fades in, Glows | 200ms, glow fades over 1.2s | fade and glow |
| Removing from a list | deleting, unlinking | the row fades and Folds closed | 160ms ease-in | fade, space closes at once |
| Dragging | reordering, moving between groups | lifts to depth 3 with a slight tilt; drops into place | 120ms pick up, 200ms drop | lift shadow only |
| Count changes | a badge or number updating | the new number grows in from 70% | 280ms | number swaps |

## Look

- **Pages are quiet.** Prose, connections, link lists and lists of pages sit
  on the page with no card around them; rows are separated by hairline rules
  (`--color-border-light`) and lift with a soft fill on hover. An edge and a
  filled panel (`--color-bg-secondary`, radius 12, `--elev-resting`) are kept
  only where they carry meaning: fact panels, fields, pictures' frames, a
  selected row (a tint and a 2px accent edge on its leading side), and tools.
  `static/css/page_look_annals.css` is the reference.
- **Workshops are framed** on the same tokens: cards and panels on
  `--color-bg-secondary` with a 1px edge, a rail, an inspector and the save
  bar, as in `customize_shell.css`.
- The corner, depth, glow and face rules below describe the base kit,
  Material. Another style kit may replace them on the pages it dresses, only
  as its own row under **Style kits** says.
- Theme tokens (`bg-surface`, `text-fg`, `text-fg-muted`, `border-edge`,
  `text-accent`), never hard-coded colours, and both light and dark. Dark mode
  uses the same rules with lighter surfaces and a lighter accent, not glows
  (Lumen's glow is its kit's character and the one exception).
- **Colour roles**: page `--color-bg-primary`, panel `--color-bg-secondary`,
  hover and alternate rows `--color-bg-tertiary`, menu `--color-sidebar-bg`,
  headings `--color-text-primary`, labels `--color-text-secondary`.
- **One accent.** The accent (`--color-accent`, the campaign's colour) marks
  what you can act on and where you are: links (underlined at 40%), the
  selected row's edge, focus, the title's rule, group and section labels, and
  the Glow. It is never a fill behind content or a second decorative colour.
- **State colours mean state.** Green, amber and red (`--color-ok-rgb`,
  `--color-warn-rgb`, `--color-bad-rgb`) only ever mean state. Amber is also
  the **Director's mark**: DM-only content carries the `cb-dm` mark and, on a
  row or block, a 3px amber edge (`data-dm-only`), so no other meaning of
  amber shares a page with it. Site admin is the one place red is the accent,
  to say that changes there reach the whole site.
- **Depth has four levels**: 0 flat (pages, prose, lists, `--elev-static`), 1
  resting (fact panels, workshop cards, buttons, `--elev-resting`), 2 lifted
  (hover in a workshop, menus, hover cards, `--elev-hover`), 3 above
  (dialogs, dragging, `--elev-dragged`). A thing rises one level when hovered
  or picked up and returns when let go; a row on a page rises from 0 to
  resting on hover, with no movement; a pressed button sinks below level 1
  with an inner shadow. Dark mode uses deeper shadows and lighter surfaces,
  not glows.
- **Corners**: 8 for controls and rows, 12 for panels, dialogs, drawers and
  pictures, 4 for tags, 6 for small things, pills for counts and status
  labels. Nothing sharper than 4 or rounder than 12 except pills, the
  Customizer's own frames (tiles 14, the phone preview 28), which stand for
  a device, and the corners a style kit names (Sharp's square frames,
  Flowing's uneven rounded ones, Sketchbook's hand-drawn ones).
- **Type**: headings in the campaign's heading face; when the owner picked
  none, pages under Annals use tall capitals (Oswald, 600, set by
  `AppearanceCSS`) and everything else Inter. Page titles 24 to 40 (fluid)
  at a tight line height with the accent rule under them; 20 semibold section
  headings; 16 semibold block headings with a hairline under them; 14
  reading text and controls; 12.5 labels, hints and timestamps; 11 caps
  group labels, tracked, in the accent. Reading text stays near 62 to 70
  characters wide; numbers in columns are tabular.
- **Spacing is on steps of 4** (4, 8, 12, 16, 24, 32, 48): 8 inside a
  control, 12 to 16 between related things, 24 to 32 between sections. Gaps
  belong to layouts, not to each item. Space, not a box, groups things on a
  page.
- **The header is slim**: 48px under the quiet look unless the owner picked
  the tall one; every header function stays.
- Icons are monochrome Font Awesome in the text colour, or the accent beside
  a heading or in a tool's selected state; colour is never decoration, and
  an emoji is never an icon.
- Buttons and pills size to their content; nothing stretches full width after
  a state change.
- Words on screen are plain and short. No developer terms, IDs, or version
  strings that aren't real. A button says what happens ("Publish") and its
  message confirms it ("Published"); an error says what went wrong and how to
  fix it, without apologising.

## Customize goes further

The standard above is the base every campaign starts from. A campaign owner's
Customize choices (`AppearanceData`, emitted as `data-cz-*` on the page) can
make it richer, never break it: Calm and Off still win, and the tools keep
their tokens.

- **Page look** (`data-cz-page`, `static/css/page_look_annals.css`, chosen in
  Customize › Look › Pages, where Annals is named Material): Annals
  is the Material page look described under Look, and the one new pages and
  widgets are designed in. New campaigns start in Annals; Standard draws the
  same blocks with the older card around each, and stays a choice for
  campaigns that use it; neither moves a block. A
  look styles blocks only through the **shared block parts** every block
  marks: `cb-title`, `cb-head` and `cb-count`, `cb-card` (`cb-facts` keeps
  its edge), `cb-section` with `cb-section-h`, `cb-fact` with `cb-fact-l`
  and `cb-fact-v`, `cb-group`, `cb-link`, and `cb-dm` (the visible mark) with
  `data-dm-only` (the row or block players don't get) for what only the
  Director sees. A new block or package widget marks the same parts to
  follow the look. Standard draws them exactly as before (input.css,
  "Shared block parts"). Under Annals, following a connection pulls the new
  page out from under the connections block ("Motion").
- **Style kits** give a campaign a whole character, set out under **Style
  kits** below.
- **Buttons** (`data-cz-btn`): Lift is the base; Press, Glow and Ink restyle
  it; Keystone is soft, raised and rounded (12px), and gives every button the
  heavy press.
- **Depth** (`data-cz-elev`, shadows from `czElevation`): Standard is the
  base; Flat, Dramatic, and Ambient, whose shadows are tinted with the accent
  under a faint light from the page's top corner.
- **Motion speed** retimes the duration tokens; **menu highlight** and
  **hover card** looks are their own choices.
- **View as player** (the owner's eye button in the header) hides everything
  marked `data-dm-only`, and inline secrets (`data-secret`), while it is on
  (`html[data-view-as-player]`), in any look, and shows what is marked
  `data-player-only` (a count without the Director's rows). It is the owner's
  preview, since the server never sends players that content. Rows are
  marked where they are drawn: DM-only blocks, relations, notes for the DMs,
  DM-only posts and tags, page-table columns, and the sub-pages and
  Referenced by rows a player (no personal grants) wouldn't get. In Annals
  the switch shows what changes: Director-only rows fold away before the
  page reloads as a player sees it, the banner folds in, and coming back
  folds them in with the list-add glow
  (`static/js/widgets/player_view.js`). Calm fades, Off swaps. A secret
  never shows as a gap: a view built for players leaves out the link or row
  that would point at it.
- The editor's example site (`static/js/widgets/customize_look.js`,
  `static/css/customize.css`) shows each choice and keeps its numbers in step
  with the server's.

## Style kits

A style kit is a campaign's character: its top bar, dividers, boxes, faces,
palette, how things answer the pointer, and how a card opens and closes. The
owner picks one in the Customizer. There are six, and each one works in light
and dark mode, takes the campaign's colour, and behaves differently as well
as looking different. The signed mockup is the contract for each kit's
character: https://claude.ai/artifact/YMuDg5AgHP7V6L6G5rSDnc. A screen may
vary the details as long as it keeps that character. The owner picks the
kit in Customize › Look › Pages; it puts `data-cz-kit` on the page
(`static/css/style_kits.css`), and only pages in the Material page look wear
it. Each kit's opening is still to be built (#1548).

| Kit | Top bar | Dividers and rows | Boxes | Pointer | A card opens | Faces |
|---|---|---|---|---|---|---|
| Material (the base) | tinted surface lifted by a soft shadow; the current place is a raised pill | space and an accent label; hairlines in lists | filled, 12px corners, resting shadow, no edge | lifts (shadow) | grows out of what was clicked, its shape stretching into the card's | Inter |
| Sharp (broadsheet) | masthead: condensed capitals over a thick and a thin rule; places split by short rules, current one underlined | a heavy rule over each section head; hairlines between rows; a dateline | square ink frame, no fill or shadow; heading knocked out of the frame | inverts, nothing moves | unrolls straight down from the edge clicked, quick, no fade | Oswald, Source Serif 4 |
| Ornate (bound volume) | deep band, name centred, a double gilt line broken by a small ornament | a centred flourish between sections; dotted leaders across rows | double gilt frame on warm paper, curled corners | fills with the band colour | swings down from what was clicked on a hinge and settles | Cormorant Garamond, EB Garamond |
| Flowing (watercolour) | a wash between two tints, its lower edge a soft wave; the current place has a wavy underline | a drawn wave line; rows are separate soft "pebbles" | no edge, uneven rounded corners, a faint wash and a low shadow | outlines slowly change shape | spreads from what was clicked like ink in water, its edge settling into shape | Fraunces, Nunito |
| Lumen (soft light) | a night band with a fine line of light under it; a slow sheen crosses the name; the current place is a lit dot | a hairline fading at both ends, with an occasional travelling glint | a fine edge with a soft glow that breathes slowly | glow brightens | a ring of light spreads from what was clicked and the card blooms out of it into focus | Marcellus, Jost |
| Sketchbook (pen on paper) | no band: the name is handwritten over a pen stroke; the current place is circled by hand | a scribbled pen line; section names highlighted; dashed row underlines | hand-inked outline that wobbles slightly, on paper (cream in light, a chalkboard in dark) | gives a small wiggle | a taped note drops out of what was clicked and lands slightly askew | Kalam for headings and short labels, Atkinson Hyperlegible for text |

- **What every kit keeps.** What opens comes out of what was clicked and goes
  back into it, with a visible close; Escape and the rules under **Opening
  and closing** hold. The four control weights, the layout, the Director's
  amber and its 3px edge, state colours meaning state, and the accent marking
  what you can act on stay the same in every kit.
- **A kit's opening replaces Grow and Drape** for cards and panels that come
  out of something clicked on a page. The other moves under **Motion** keep
  their timing in every kit.
- **Calm and Off still win.** Under Calm or the device's reduced-motion
  setting, every kit's opening is a short fade in place, and every ambient or
  pointer movement stops at rest: Lumen's sheen, glint and breathing hold
  still with a steady glow, Flowing's outlines and Sketchbook's wiggle don't
  move. Off shows the end state. Ambient movement also runs only on what is
  on screen, and stops when the tab is hidden.
- **Faces are each kit's suggestion.** A heading face the owner picked wins
  over the kit's. A hand-drawn or display face is for headings and short
  labels only, never reading text. If a face fails to load, its fallback
  stack is a plain face of the same kind, never a script.
- **Kits dress pages, not tools.** The kit reaches the header and every page
  under the page look. Workshops (the Customizer, settings, site admin,
  editors' own panels) stay in Material so the tools stay predictable. Looks
  that are signed as their own (character sheets, the rulebook, the
  calendar, the quest board, map frames, encounters, the hover card) keep
  them in every kit, with two exceptions: the DM Screen wears the kit,
  since the Director keeps it open over the pages at the table, and so does
  the hover card on a time line (under **Opening and closing**).
- **Ornate's gilt is brass** (light `#8a8350`, dark `#b8b07f`), well apart
  from the Director's amber, and does not take the campaign's colour.

### The campaign's colour in a kit

- **The campaign's colour replaces each kit's accent** when the owner has
  picked one, and everything the kit derives from its accent follows it:
  Lumen's glow and line, Flowing's wash, Sketchbook's highlighter and tape.
  With no colour picked, each kit keeps the colour it was designed with. The
  contrast step below is built; the picker's refusals and swatch are
  TODO(#1549).
- **Lifted for dark mode.** The light-mode accent is the colour darkened
  slightly (mixed 88% with black). The dark-mode accent is the colour mixed
  55% with white, so one choice reads on both.
- **Colours near the Director's amber are refused** (hue 22 to 58 degrees,
  saturation above 45%, lightness above 30%). The colour picker says why in
  words and keeps the previous colour.

### Exceptions

Some colours will not read in some kits, and some blocks will not draw well
in some kits. Each has a named way out. None of them is a one-off override
in a template.

- **Contrast is checked per kit and per theme, when the page is drawn.**
  Each kit's accent must reach 4.5:1 against that kit's page and paper as
  link text, and 3:1 as an edge or a glow. A colour that falls short is
  stepped toward the kit's ink in lightness only, keeping its hue, until it
  passes, in that kit and that theme only. Light Lumen, the chalkboard and
  Ornate's warm paper each move a different colour, which is why the check
  is per kit. The Customizer's colour field shows the adjusted swatch beside
  the picked one when this happens.
- **A colour that would read as a state colour is moved off it.** Beside
  the green, amber and red of state, a picked colour within 12 degrees of
  red or green has its saturation lowered until the two are told apart.
  Amber is refused outright, as above.
- **Expensive effects step down when they cost too much.** Sketchbook's
  hand-drawn edge (the outline drawn twice, a little apart) and Lumen's
  breathing glow are only on boxes, which a page has few of. Rows in every
  kit use a plain edge in the kit's line colour and a glow that doesn't
  breathe.
  Printing uses plain edges and no glow.
- **A block that a kit draws badly can ask to be drawn plain.** A block or
  package widget that has been tried in a kit and found to break (a chart,
  an embedded map, a dense table) marks itself `data-kit-plain`, and is drawn
  with the Material parts inside that kit. Every such case is listed here
  with its kit, its reason and an issue to fix it. No block has needed it
  yet, so the marker is built with the first one.

| Block | Kit | Why it is plain | Issue |
|---|---|---|---|
| none yet | | | |

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
| Editable part | accent hairline and its name | opens its editor | the same hairline and name | "Saving…" in place | no mark |

## Waiting and feedback

- **Under 0.3 seconds, show nothing extra**; a flash of a spinner reads as a
  glitch.
- **A button's action** shows a spinner in the button with what it's doing
  ("Saving…"), and it can't be pressed twice.
- **A page or panel loading** shows grey placeholder lines in the shape of the
  content after 0.3 seconds, then the content fades in over them. Under Calm
  the placeholders stay still.
- **Small saves** (typing, toggles, a single pick) confirm in place with a
  green "Saved" tick that fades.
- **A draft saved from the save bar** shows "Saving…" in the bar, then a
  message at the bottom saying who now sees the change, with Undo.
- **Bigger actions** (create, publish, invite) get a message at the bottom for
  three seconds, with Undo when it can be undone.
- **Errors stay until dismissed**, sit next to the thing that failed, and say
  how to fix it. Nothing shakes.
- **Deleting asks first** in a dialog naming exactly what goes, unless it can
  be undone.

## Older looks

Two older looks are still on parts of the site. A change that touches a
screen in one brings that screen (not just the changed part) to the Material
look in the same piece of work, or files the issue to do it.

- **The boxed look (v1)**: raw Tailwind palette classes instead of tokens
  (`bg-white`, `bg-gray-*`, `text-gray-*`, `border-gray-*`, `dark:bg-gray-*`),
  a shadow and a rounded box on every section, emoji as icons. Replace with
  the tokens and the edge vocabulary.
- **The carded look (v2)**: the right tokens, but every section in a
  bordered `.card`, lists as grids of cards, and menus and popovers as boxes
  with a heavy shadow. On a page, take the box away and mark the shared
  block parts; in a workshop the frame stays, on the tokens.
- **Signed looks of their own** (the calendar, the quest board's looks, the
  rulebook's book, the map frames, character sheet styles) stay as signed
  until a new mockup is signed; the tools around them (headers, menus,
  dialogs, lists) follow this page.

## Designing a change

- **Hard design questions get real research**, including apps outside
  TTRPGs (design tools, writing and note apps, maps, games, print and
  museum conventions), and come back as several distinct options, at least
  one of them a new idea rather than the familiar version redrawn. The
  recommendation is marked, and each option is playable.
- **A new design starts from the Material look**, not from an older mockup
  or the Standard page look, and its motion says what physical thing each
  move is.
- A visible change gets a mockup first, on the real page as it is on `main`,
  at shipped quality: never placeholders or a lean out-of-context fragment.
- Motion decisions are shown as playable clips, never stills.
- A signed mockup is the contract until a new one is signed. Small
  differences found while building go back as a "small differences" card.
