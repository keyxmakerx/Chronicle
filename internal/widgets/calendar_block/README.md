# calendar_block — the calendar Block widget

One component, four zones, any host width. Plugin-agnostic by construction: it
imports nothing from `internal/plugins/**`, performs no queries, and holds no
request state (bound blocks render with `context.Background()`).

```go
import "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"

// anywhere a templ component can go
@calendar_block.Block(data)
```

`data` is a fully-resolved `calendar_block.BlockData`. Build it in a service, not
in a handler. **`data.go` is a pinned cross-slice contract — do not edit the
struct.**

## The four zones

```
┌─────────────────────────────────────────────┬──────────────┐
│ A  NAMEPLATE  name · date-or-fault · sync   │              │
├─────────────────────────────────────────────┤  C  LEDGER   │
│                                             │              │
│ B  INSTRUMENT   the month                   │   docked,    │
│    ten-day weeks native                     │   never a    │
│    the era as a soft cell tint              │   drawer     │
│    intercalary row (full tier)              │              │
├─────────────────────────────────────────────┴──────────────┤
│ D  SHELF                                                    │
└─────────────────────────────────────────────────────────────┘
```

Zones C and D are `needs backend` stubs in wave 1 (W-B and W-E fill them). They
are still docked at their real size: the full-tier column arithmetic subtracts
the Ledger's 300px unconditionally, so a Block that skipped the zone would flip
density at the wrong host width.

## Sizing: you do not choose it, and neither does the widget

Size class follows **host** width; density follows **measured column** width.
Both are CSS container queries, and there is zero JavaScript in this package —
`htmx.config.allowScriptTags = false` (`boot.js:163`) means a `<script>` inside
an HTMX-swapped fragment never runs, so a JS-sized Block would silently render at
the wrong density after any swap.

| host width | size class | what changes |
|---|---|---|
| ≥ 900 | `full` | Ledger docks beside the month; intercalary row; wide sync string; 3-letter weekdays |
| ≥ 300 | `std` | Ledger docks below; compact sync string; 2-letter weekdays |
| ≥ 240 | `mini` | no header, no Ledger; a foot line instead |
| < 240 | `submini` | the grid is dropped honestly for a thirty-tick month rule |

| measured column | density |
|---|---|
| ≥ 84px | named chips |
| < 84px | one presence underline, max three segments |

To host a Block at a given width, just give its container that width — the
wrapper is the query container.

## Styling

`static/css/calendar-block.css` is unlayered, self-contained and scoped under
`.cal-block-host`. `Block()` emits its own `<link>` (via `layouts.AssetURL`), so
a fragment carries its own styling; hosts may also render
`calendar_block.Stylesheet()` in `<head>`.

The only token it inherits from the page is `--color-accent`.

## Before you write a producer

Read `.ai.md` §"Producer notes". The two that bite hardest:

- **Do not trim `Marks` to a density's cap** — the producer cannot know which
  subtree the container query will show.
- `TiedCount` and `WholeCount` must come from **the same viewer-filtered pass**,
  or the tie toggle becomes an oracle that differences hidden events out of the
  two numbers.

## Tests

```bash
go test ./internal/widgets/calendar_block/                 # render + CSS contract
go test ./internal/widgets/calendar_block/ -run Probe -v   # real browser (skips under -short)
BLOCK_SCREENSHOTS=/tmp/shots go test ./internal/widgets/calendar_block/ -run Screenshots
```

CI runs `-short`, so the browser probe never executes there. Container-query
density proven only by a Go unit test is not proven — run the probe locally
before claiming a sizing change works.
