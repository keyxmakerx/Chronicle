# Build order — chrome, permissions, Customize (approved 2026-09-12)

**Operator, 2026-09-12: "Overall I approve."** Canvas:
https://claude.ai/code/artifact/d554c339-512f-4f01-9ead-06cafd69df90
(three pages: Permissions, Chrome, Customize). No further renders are needed
to start. Specs live in `2026-09-12-header-and-nav.md` (all addenda),
ADR-057 as amended, and the permissions-indicator and security plans (deleted
once their open slices became issues; full text at
https://github.com/keyxmakerx/Chronicle/tree/dfc73c78/.ai/designs). This file
only orders the work.

**Standing rules.** One slice = one PR, branched from main. The `go-dev`
agent implements; every PR gets the `reviewer` pass before push; the
`migration-safety` gate on anything touching schema. `make verify` +
`make test-js` green. Screenshots at desktop and 390px in headless Chromium
against the canvas board the slice implements.

**Defaults assumed where the operator did not pick** (say so in the PR):
nav highlight style ships as **D living ring, Calm**, still fallback **J rail
and tint**; page name In the row.

## Order

1. **P-1 Permissions glance + slide-out** — ADR-057 slices 1–5 as amended:
   fix the Co-DM gate and the Scribe wrong-glance first; one
   `visibilityGlance` component beside the name (Owner/Scribe/co-DM only,
   nothing for Players); hover = the key; click = the widget's existing
   right-edge card, Owner edits, Scribe sees the badge; retire the edit
   form's inline mount and the bottom-of-Details icon. Small, isolated,
   highest value per token. **Start here.**
2. **N1 Sidebar: switcher flyout + brand in place** — the Apps drawer first
   planned here is superseded by the sidebar signed on #739 (Pinned +
   folding sections). The switcher is a slide-over from the sidebar's edge
   (search, folded filters incl. Public, current first); header
   picker and Campaigns link retire; Owner edits name/logo in the header.
   One `items` model, `SidebarItem` extended, never paralleled. The
   header's left slot shows the path (direction C).
3. **N2 Nav highlight styles** — CSS-only styles D/G/H/I moving, J/K/L/M
   still; `prefers-reduced-motion` → the still twin; Owner setting on
   Customize (Strength, Page name); per-user off switch in the account
   menu. Can ride with N1 or follow it.
4. **T-A/T-B Customize cleanup** (C-THEME-V2 A + B, `BACKLOG.md:930-973`):
   bug batch, one save model (fold Surface Accents into the staged flow,
   N0 item 4), no JS-injected UI, the CI guard. Cheap; unblocks everything
   on that surface. First task inside it: open the Appearance tab in a live
   session and record what the demo site actually moves.
5. **H1–H3 Header** — widget host with the no-calendar widgets, then the
   Customize → Header section (brand, five background modes incl. image
   ≤1.5 MB with scrim, widget rows, one save). Sky stays deferred.
6. **T-C + Preview rebuild** — the colour tones, more presets + custom hex,
   elevation/motion presets, heading font; the demo site fed by the draft
   at three zoom levels; one Customize page with sections down the left.
7. **In parallel, other chat:** security plan S1–S6 and fog/layers/map-write
   API parity (unblocked). They touch none of the above.

Deferred, on purpose: calendar V5 (see `.ai/todo.md`), living sky
(skypane), timeline V2, search V2.
