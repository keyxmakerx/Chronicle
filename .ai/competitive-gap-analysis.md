# Competitive Gap Analysis — Chronicle vs Competitors

**Created:** 2026-03-09
**Purpose:** Identify features Chronicle is missing compared to World Anvil, LegendKeeper,
Kanka, and Obsidian (TTRPG plugin ecosystem). Inform phase V/W priority and discover
new sprint candidates.

---

## Feature Matrix

Legend: ✅ = Has it | 🟡 = Partial | ❌ = Missing | N/A = Not applicable

| Feature | Chronicle | World Anvil | LegendKeeper | Kanka | Obsidian |
|---------|-----------|-------------|--------------|-------|----------|
| **WORLDBUILDING** | | | | | |
| Wiki articles / entities | ✅ | ✅ | ✅ | ✅ (20 types) | ✅ (notes) |
| Custom entity types / categories | ✅ | ✅ (25+ templates) | 🟡 (flat pages) | ✅ | ✅ (folders) |
| Custom fields / attributes | ✅ + overrides | ✅ | ✅ (sidebar properties) | ✅ | ✅ (properties) |
| Guided worldbuilding prompts | ❌ | ✅ (meta tool) | ❌ | ❌ | ❌ |
| Genre presets / templates | ❌ | ✅ (themes) | ✅ (community templates) | ❌ | ✅ (vault templates) |
| Entity cloning | ✅ | ✅ | ❌ | ✅ | N/A |
| Entity sub-notes / posts | ✅ | ✅ (secrets) | ✅ (tabs) | ✅ (posts/secrets) | ✅ (nested notes) |
| **EDITOR** | | | | | |
| Rich text (WYSIWYG) | ✅ (TipTap) | 🟡 (BBCode) | ✅ (block editor) | 🟡 (Summernote) | ✅ (markdown) |
| @mentions / linking | ✅ | ✅ | ✅ (auto-detect) | ✅ | ✅ ([[wikilinks]]) |
| Auto-linking (name detection) | ✅ | ❌ | ✅ (best-in-class) | ❌ | ❌ |
| Slash commands (/) | ❌ | ❌ | ✅ | ❌ | ✅ |
| Tables | ✅ | ✅ | ✅ | ✅ | ✅ |
| Code blocks w/ syntax highlight | ✅ | ❌ | ❌ | ❌ | ✅ |
| Find & replace | ✅ | ❌ | ❌ | ❌ | ✅ |
| Callout / info blocks | ✅ | ✅ | ✅ (secrets block) | ❌ | ✅ (callouts) |
| Embed media / video | 🟡 (images only) | ✅ (video, audio, SFX) | ✅ (embed blocks) | 🟡 | ✅ |
| Markdown import/export | ❌ | ❌ | ✅ (YAML frontmatter) | ❌ | ✅ (native) |
| **MAPS** | | | | | |
| Interactive maps | ✅ (Leaflet) | ✅ | ✅ (WebGL, best) | ✅ | ✅ (Leaflet plugin) |
| Map markers / pins | ✅ (39 icons) | ✅ | ✅ (drag-drop, multi-select) | ✅ | ✅ |
| Map layers | ✅ | ✅ | ✅ (zoom-dynamic) | ✅ | ❌ |
| Map regions / territories | ❌ | ❌ | ✅ (polygons, fills) | ❌ | ❌ |
| Map paths / roads | ❌ | ❌ | ✅ (labeled, measured) | ❌ | ❌ |
| Map drawing tools | 🟡 (backend only) | ❌ | ✅ | ❌ | ❌ |
| Map measurement / distance | ❌ | ❌ | ✅ (calibration) | ❌ | ❌ |
| Map fog of war | 🟡 (Foundry sync) | ❌ | ❌ | ❌ | ❌ |
| Nested maps (world→city→dungeon) | ❌ | ✅ (linked maps) | ✅ (nested) | ✅ | ❌ |
| Map navigation / routing | ❌ | ❌ | ✅ (travel time calc) | ❌ | ❌ |
| Map embed in wiki pages | ❌ | ✅ | ✅ (auto-center on pin) | ❌ | ❌ |
| Map marker search | ✅ | ✅ | ✅ | ✅ | N/A |
| Map marker clustering | ✅ | ❌ | ✅ (WebGL) | ❌ | ❌ |
| **TIMELINE** | | | | | |
| Visual timeline | ✅ (D3, eras, minimap) | ✅ | ✅ (list/gantt/calendar) | ✅ | 🟡 (plugin) |
| Event connections | ✅ (SVG arrows) | ❌ | ❌ | ❌ | ❌ |
| Multiple timelines | ✅ | ✅ (parallel) | ✅ (multiple + simultaneous) | ✅ | ❌ |
| Custom time systems | ❌ | ✅ | ✅ | ✅ | ✅ (Fantasy Calendar) |
| Timeline views (list/gantt) | 🟡 (D3 only) | ✅ | ✅ (3 views) | 🟡 | ❌ |
| **CALENDAR** | | | | | |
| Interactive calendar | ✅ | ✅ | 🟡 (via timeline) | ✅ (best) | ✅ (plugin) |
| Month/week/day views | ✅ | ✅ | N/A | ✅ | ✅ (plugin) |
| Event drag-and-drop | ✅ | ❌ | N/A | ❌ | ❌ |
| Custom calendars / months | ✅ | ✅ | ✅ | ✅ (best: -2B to +2B) | ✅ (plugin) |
| Event categories | ✅ (custom) | ✅ | ✅ | ✅ | ❌ |
| Moons / seasons / weather | ❌ | ✅ | ❌ | ✅ | ✅ (plugin) |
| **COLLABORATION** | | | | | |
| Multi-user campaigns | ✅ | ✅ | ✅ | ✅ | ❌ |
| Role-based permissions | ✅ (Owner/Scribe/Player) | ✅ | ✅ | ✅ (best: per-role CRUD) | N/A |
| Per-entity permissions | ✅ (custom grants) | ✅ | 🟡 (admin-only secrets) | ✅ (per-role per-user) | N/A |
| Group-based visibility | ✅ | ✅ (subscriber groups) | ❌ | ✅ (multiple roles) | N/A |
| Real-time co-editing | ❌ | ❌ | ✅ (Yjs) | ❌ | ❌ |
| Invite system (email) | ❌ | ✅ | ✅ (link sharing) | ✅ | N/A |
| Secrets / DM-only content | ✅ (visibility modes) | ✅ (inline secrets) | ✅ (block-level secrets) | ✅ (secret posts) | N/A |
| "View as player" preview | ✅ | ✅ | ❌ | ✅ | N/A |
| **ORGANIZATION** | | | | | |
| Quick search (Ctrl+K) | ✅ | ✅ | ✅ | ✅ | ✅ (Cmd+O) |
| Tags | ✅ | ✅ | ✅ (tag index blocks) | ✅ (best: nested tags) | ✅ |
| Relations / connections | ✅ (graph viz) | ✅ (family trees, diplomacy) | 🟡 (boards arrows) | ✅ | ✅ (Dataview) |
| Backlinks / "Referenced By" | ❌ | ❌ | ✅ (+ aliases) | ❌ | ✅ (core feature) |
| Graph view | ✅ (relations) | 🟡 (diplomacy webs) | ❌ | 🟡 (premium relations) | ✅ (best: core) |
| Entity hierarchy (parent/child) | ✅ | ✅ | ✅ (nesting) | ✅ | ✅ (folders) |
| Saved filters / views | ❌ | ❌ | ❌ | ❌ | ✅ (Dataview) |
| Favorites / bookmarks | ✅ | ✅ | ❌ | ❌ | ✅ (bookmarks) |
| Recent entities | ✅ | ❌ | ❌ | ✅ | ✅ |
| **SESSION MANAGEMENT** | | | | | |
| Session tracking | ✅ | ✅ | ❌ | ✅ (journals) | ❌ |
| RSVP / attendance | ✅ | ❌ | ❌ | ❌ | ❌ |
| Session recurrence | ✅ | ❌ | ❌ | ❌ | ❌ |
| Session recaps | ✅ | ✅ | ❌ | ✅ (journals) | ❌ |
| Session prep checklist | ❌ | ❌ | ❌ | ❌ | ✅ (tasks) |
| Character assignment | ✅ | ✅ | ❌ | ✅ | ❌ |
| **GAME SYSTEMS** | | | | | |
| D&D 5e reference data | ✅ (SRD) | ✅ (45+ systems) | ❌ | ❌ | ✅ (Fantasy Statblocks) |
| Pathfinder 2e reference | ✅ (ORC data) | ✅ | ❌ | ❌ | ✅ (plugin) |
| Character sheets | ❌ | ✅ (100+ systems) | ❌ | 🟡 (plugins) | ✅ (plugins) |
| Combat / initiative tracker | ❌ | ✅ | ❌ | ❌ | ✅ (plugin) |
| Dice roller | ❌ | ✅ (in-sheet) | ❌ | ❌ | ✅ (plugin) |
| Stat blocks / encounter builder | ❌ | ✅ | ❌ | ❌ | ✅ (plugin) |
| **DM TOOLS** | | | | | |
| Customizable dashboards | ✅ (best) | 🟡 (upcoming overhaul) | ❌ | ❌ | ❌ |
| Layout / template editor | ✅ (best: drag-drop) | ❌ | 🟡 (page columns) | ❌ | ❌ |
| Family tree / genealogy | ❌ | ✅ | 🟡 (boards) | ✅ (families module) | ❌ |
| Diplomacy / faction web | ❌ | ✅ (visual) | 🟡 (boards) | ❌ | ❌ |
| NPC generator | ❌ | ❌ | ❌ | ❌ | ❌ |
| **PLATFORM** | | | | | |
| Self-hosted | ✅ (primary) | ❌ | ❌ | 🟡 (source-available) | ✅ (local) |
| REST API | ✅ | ✅ | ❌ | ✅ | ❌ |
| VTT integration | ✅ (Foundry) | ✅ (various) | ❌ | ❌ | ❌ |
| Extension / addon system | ✅ (3-layer) | ❌ | ❌ | ✅ (plugins) | ✅ (1000+ plugins) |
| Export / import | ✅ (JSON) | ✅ | ✅ (HTML/JSON/LK) | ✅ (JSON) | ✅ (markdown) |
| Audit logging | ✅ | ❌ | ❌ | ❌ | ❌ |
| Offline support | ❌ | ❌ | ✅ (local storage) | ❌ | ✅ (native) |
| Whiteboards / canvas | ❌ | ✅ | ✅ (boards, best) | ❌ | ✅ (canvas) |
| **UI/UX** | | | | | |
| Dark mode | ✅ | ✅ | ✅ | ✅ | ✅ |
| Custom themes | 🟡 (dark/light) | ✅ (20 themes) | ❌ | ✅ (custom CSS) | ✅ (community) |
| Mobile responsive | ✅ | ✅ | ✅ | ✅ | 🟡 (mobile app) |
| Keyboard shortcuts | ✅ (Ctrl+K/N/E/S) | ❌ | ✅ (Adobe-style) | ❌ | ✅ (extensive) |
| Meter / tracker blocks | ❌ | ❌ | ✅ (6 styles) | ❌ | ❌ |
| Cover images on pages | ❌ | ✅ | ✅ | ❌ | ❌ |

---

## Key Gaps — Prioritized for V/W Phases

### HIGH PRIORITY (DM workflow differentiators)

1. **Backlinks / "Referenced By" panel** — LegendKeeper and Obsidian's killer feature.
   Chronicle already has @mention data in `entry_html`. Just needs a reverse index query.
   → **Sprint V-2** (already planned)

2. **Secrets / DM-only content blocks in editor** — World Anvil and LegendKeeper let you
   embed hidden content inline within articles. Chronicle has entity-level visibility and
   DM-only posts, but no inline secret blocks within the editor itself.
   → **New: Sprint V-1.5 or integrate into existing editor**

3. **Slash commands in editor** — LegendKeeper's `/secret`, `/layout`, `/help` pattern.
   TipTap supports this natively. Huge DX improvement.
   → **New: Sprint V-1 addition or standalone**

4. **Quick capture / session journal** — No competitor has this well. Obsidian's daily notes
   are the closest. This is a differentiator.
   → **Sprint V-1** (already planned)

5. **Markdown import/export** — LegendKeeper supports it, Obsidian is native markdown.
   Critical for users migrating from Obsidian vaults.
   → **Sprint W-5** (already planned)

### MEDIUM PRIORITY (competitive parity)

6. **Map regions / territory outlines** — LegendKeeper's biggest differentiator. Polygon
   drawing on maps with fills, strokes, labels. Chronicle has the `map_drawings` table
   backend but no native drawing UI.
   → **Sprint W-2** (already planned, expand scope)

7. **Map measurement / distance tool** — LegendKeeper has map calibration + distance calc.
   Leaflet supports this via plugins.
   → **Sprint W-2 addition**

8. **Nested / linked maps** — World Anvil and LegendKeeper let you click a marker to open
   a sub-map (world → region → city → dungeon). Chronicle maps are flat.
   → **New: Sprint W-2.5 or W-6**

9. **Map embed in entity pages** — LegendKeeper auto-centers map embeds on the entity's pin.
   Could be a layout block type that shows a mini-map centered on the entity's marker.
   → **New: Sprint W-2 addition**

10. **Content templates / page templates** — LegendKeeper has community templates, Obsidian
    has Templater. Chronicle has layout templates but not content pre-fill templates.
    → **Sprint V-3** (already planned)

11. **Family tree / genealogy view** — World Anvil has dedicated family trees, Kanka has a
    families module. Chronicle has relations but no genealogy visualization.
    → **Deferred** (already in backlog)

12. **Cover images on entity pages** — World Anvil and LegendKeeper show hero/banner images.
    Chronicle shows entity image as avatar only.
    → **New: Quick win, layout block type**

13. **Whiteboards / canvas** — LegendKeeper (boards), World Anvil (whiteboards), Obsidian
    (canvas). Useful for relationship mapping, session prep, DM screens.
    → **Deferred** (already in backlog, high effort)

### LOWER PRIORITY (nice-to-haves)

14. **Multiple timeline views** — LegendKeeper has list/gantt/calendar views. Chronicle has
    only the D3 visualization. A simple list view would be easy to add.
    → **New: Quick sprint**

15. **Meter / tracker blocks** — LegendKeeper's unique feature for HP bars, spell slots.
    Could be a widget block type in the layout editor.
    → **New: Sprint W-6 or layout block**

16. **Entity aliases** — LegendKeeper lets entities have multiple canonical names for auto-
    linking and search. Chronicle auto-link uses exact names only.
    → **New: Sprint V-2 addition**

17. **Moons / seasons / weather** — World Anvil and Kanka calendars support moons and
    weather effects. Chronicle calendars don't.
    → **Deferred** (calendar enhancement)

18. **Custom themes / accent colors** — World Anvil has 20 themes, Kanka allows custom CSS.
    → **Sprint W-5** (partially planned)

19. **Embed media blocks** — World Anvil embeds video/audio/SFX in articles. Chronicle only
    supports images.
    → **New: editor enhancement**

20. **Entity image gallery** — Multiple images per entity instead of single avatar.
    → **Backlog** (already noted)

---

## Revised Phase Execution Order

Based on this analysis, the recommended order is:

```
V (Notes & Discovery) → W (Polish & Power Tools) → T (Game Systems) → U (Collaboration)
```

With the following additions integrated:

### Phase V Additions (from gap analysis)
- V-1: Quick Capture + **Slash Commands in Editor**
- V-1.5 (NEW): **Inline Secrets/DM Blocks in Editor**
- V-2: Backlinks + **Entity Aliases**
- V-3: Content Templates (unchanged)
- V-4: Enhanced Graph + **Cover Images** (layout block)

### Phase W Additions (from gap analysis)
- W-2: Map Drawing Tools + **Regions, Measurement, Map Embeds**
- W-2.5 (NEW): **Nested/Linked Maps**
- W-6 (NEW): **Timeline List View + Meter Blocks**

---

## Chronicle's Unique Strengths (No Competitor Has)

These features are Chronicle exclusives — lean into them:

1. **Drag-and-drop page layout editor** with visual block placement
2. **Per-category dashboard editor** with configurable block types
3. **Per-entity field overrides** (entities customize their own schema)
4. **3-layer extension system** (content/widget/WASM) with per-campaign toggle
5. **Session RSVP with recurrence** (weekly/biweekly/monthly)
6. **Timeline event connections** with SVG arrows and 4 styles
7. **Audit logging** with full action history
8. **Foundry VTT bidirectional sync** (journals, maps, calendar, fog)
9. **Self-hosted as primary target** with no paywall or forced public content
