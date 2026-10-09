# UI standard

How Chronicle looks and behaves, as the operator has decided it. Every UI
change is checked against this page; a change that departs from it needs a
signed mockup that says so. Tokens and helpers named here are the shared
pieces to reuse, not copy.

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
- **Admin stays on admin pages.** Inside a campaign, a site admin gets one
  "Site admin" link, not the admin menu.
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

- **Folds measure their content.** A fold, roll or unfurl animates to the
  content's real height (measure `scrollHeight`, animate, then release to
  `auto`), so it never clips, jumps, or expands after a cut-off frame. No
  fixed `max-height` guesses.
- **Looping animations rest when the person steps away.** Every loop takes
  its clock from `window.MotionRest` (`static/js/motion_rest.js`) and stops
  requesting frames once `still()`. One-shot transitions don't need it.
- **Reduce switches are honoured**: `prefers-reduced-motion`, the campaign's
  `html[data-cz-reduce]` and a person's Calmer choice
  `html[data-view-motion="calm"]` all show the end state without the motion.
- **Durations and easing come from the tokens** in `static/css/input.css`
  (`--dur-micro` 120 ms feedback, `--dur-standard` 200 ms panels,
  `--dur-large` 280 ms views; `--ease-out` arrivals, `--ease-in` exits).
- **Effects have depth and stay light**: no flat pastel fills, no gimmicky
  bounces, and nothing that loads a weak device. Heavy effects check what the
  device can do first.
- **Zoom is gentle**: one wheel tick is a small step, never a jump from far to
  close.

## Look

- Chronicle's current look, on the real page as it is on `main`: theme
  tokens (`bg-surface`, `text-fg`, `text-fg-muted`, `border-edge`,
  `text-accent`), never hard-coded colours, and both light and dark.
- Icons are monochrome Font Awesome in the text colour; colour is for state,
  not decoration.
- Buttons and pills size to their content; nothing stretches full width after
  a state change.
- Words on screen are plain and short. No developer terms, IDs, or version
  strings that aren't real.

## Mockups and sign-off

- A visible change gets a mockup first, on the real page as it is on `main`,
  at shipped quality: never placeholders or a lean out-of-context fragment.
- Motion decisions are shown as playable clips, never stills.
- A signed mockup is the contract until a new one is signed. Small
  differences found while building go back as a "small differences" card.
