# craft_rules -- a recipe's "When… then…" rules, and how they look

`data-widget="craft_rules"` mounts the crafting rules editor: one recipe's
rules over a calendar ("when the Lantern is full, the project roll has an
edge"), a strip of the calendar's next 30 days showing when each rule is on,
and a **How it looks** tab that plays each moment players see on an alchemy
bench, dressed by dragging the shared item-look effects onto spots. The
crafting app (`internal/plugins/crafting`) mounts it on one stored recipe and
saves its rules, looks and extras with the recipe; the admin Design Lab
(`/admin/design-lab`) runs it against demo recipes and saves nothing. The
project roll carries the rules out on the server
(`internal/plugins/crafting/verdict.go` and `roll_extras.go`).

## The parts

Seven scripts, loaded together on sight (`coreWidgets` in `internal/app/routes.go`):

| File | Owns | Global |
|---|---|---|
| `item_look_fx.js` | The shared effects engine (looks, particles, sound, light, one clock), from the item Look block | `ItemLookFX` |
| `craft_rules_logic.js` | What a rule means: cleaning, the calendar read into days, on/next/never, the verdict, the sentence, the AI brief and paste. Pure | `CraftRules` |
| `craft_rules_stage.js` | The bench a moment plays on: `ItemLookFX.Stage` with the alchemy bench drawn in place of the item stage | `CraftRulesStage` |
| `craft_rules_editor.js` | The "When it can be made" tab: timeline, day panel, rule cards, the editor in each card, common rules, copy and paste for an AI | `CraftRulesEditor` |
| `craft_rules_more.js` | The "If it goes wrong" tab (botch cost, quirk table, mishaps) and the "Extras" tab | `CraftRulesMore` |
| `craft_rules_looks.js` | The How it looks tab: moments, stage, spot markers, the effects shelf, drag and tap-to-place | `CraftRulesLooks` |
| `craft_rules.js` | The glue: reads the attributes, injects the CSS once (prefix `crr-`), reads the calendar or uses the demo, header, recipe chips, tabs | registers `craft_rules` |

`sky_world.js` (loaded on every page by the base layout) gives the day count
and moon phases, so a rule's "full" is the day the calendar page shows full.

Attributes: `data-campaign-id` + `data-calendar-id`. Without both, the demo
calendar. The Design Lab passes the bench's `?campaign_id=&calendar_id=`
through, so an admin can write rules against a real calendar.

The crafting app sets `el._craftApp = {recipe, lists, save}` before mounting.
`recipe` is `{id, name, ings: [[qty, name]], goal, roll, liquid, rules, looks, extras}`
with the rules as the AI writes them; `lists` fills the pickers (benches,
marks, skills, items, recipes, tables, and each table's rows); `save({rules, looks, extras})`
is called after every change to any of them. The editor then shows that one recipe and no sample rules.
With no calendar the Director can see (or one that fails to read) it starts
with `noCal`: no day strip, no moon, event, weather or season kinds, and the
rules about the bench, the maker and every time still work.

## A rule

As the AI writes it, and as `CraftRules.ruleData` emits it:

```json
{"when": [{"kind": "moon", "moon": "The Lantern", "phase": "full"}],
 "then": {"kind": "edge"},
 "chance": "1 in 4",
 "players": "book" | "whenItHappens" | "secret",
 "bookLine": "Brewed under a full Lantern…",
 "look": {"spots": {"window": {"look": "dawn"}, "heat": {"move": "dim"}}, "words": "Not tonight"},
 "draft": true}
```

- **when**: up to 3 conditions, all must hold ("or" is two rules). Kinds:
  `moon` (`moon`, `phase`: one of the eight phase names, `waxing`, `waning`,
  or a hand-set moon's state `full|half|crescent|dark|vanished`), `event`
  (`event`), `rolled` (`event`, `result`), `weather` (`weather`), `season`
  (`season`), `time` (`dawn|day|dusk|night`), `bench`, `ingredient` (`mark`),
  `maker` (`skill`), `always`. The last four have no day, so the timeline can't
  place them and says so.
- **then**: `only`, `cannot`, `edge`, `bane`, `points` (`amount` -10..10),
  `extra` (`amount` 1..3), `fine`, `becomes` (`item`), `mark` (`mark`),
  `quirk`/`mishap` (`table`), `discover` (`recipe`), `say` (`text`).
- **chance**: `always` (left out) or `1 in 2|3|4|6|10|20`. An `only` rule is always.
- **players**: in the book (with an optional `bookLine`), shown on the bench
  when it fires, or the Director's alone. A secret rule has no look.

`cleanRule` reads that or the editor's own form (`players: 'shown'`, `line`)
and returns the editor's form, or null when no condition or effect is known.
Strings are trimmed and capped (`LIMITS`); a recipe holds at most 40 rules.

Names are kept as the calendar spells them and never guessed: a name the
calendar doesn't have flags the rule ("isn't on this calendar") and keeps it
off. Rules from an AI arrive with `draft: true` and count for nothing until the
Director presses Approve.

## What the editor says

- **Status chip** (`CraftRules.status`): "On today", "Next: Gloaming 10–12 · in 2
  days", "Waits for you to set The Veil to Open" (a hand-set moon's future is
  unknown, so it never predicts), "Doesn't line up before …" or "No Clear in
  the weather before …" (weather and events are known only so far ahead), or
  "Never happens" (two cycling moons that are never in those phases together;
  the warning offers to switch one to the hand-set moon).
- **Verdict** for a day (`verdict`): a certain "can't be made" wins; with any
  "only then" rule at least one must be on; an edge and a bane cancel, as in
  any Draw Steel roll. Chance rules show as "maybe".
- **Cancelled**: a rule whose every day is also a certain "can't be made" day
  says which rule cancels it.

## Reading a real calendar

All reads are the calendar API the calendar page uses; any part that fails is
left empty rather than failing the whole editor, and an unreadable calendar
falls back to the demo with a banner saying so.

| Read | Gives |
|---|---|
| `GET /campaigns/:id/calendars/:calid` | months, moons, seasons, today |
| `…/events?year=&month=` (each month the 30 days cross) | events; a repeating one's `occurrences`, minus `skipped` |
| `…/events/index` | every event's name for the picker (`truncated` is surfaced) |
| `…/rolls?year=&month=` | rolled events' results by day |
| `…/events/:eid/roll?year=` | a rolled event's possible results |
| `…/weather/days?year=&month=` | the day's weather (`preset_label`, `icon`) |
| `…/weather/settings` | the campaign's weather kinds (Director only; may fail) |

Weather pictures follow the calendar's six (`clear`, `cloud`, `rain`, `snow`,
`storm`, `fog`); `WEATHER_TABLE` is the built-in kinds with theirs, pinned to
the generator's presets by the logic test.

## If it goes wrong, and Extras

A recipe's extras are cleaned by `CraftRules.cleanExtras`, the same as the
server's `CleanExtras` (`test/js/fixtures/craft_extras_clean.json` pins both).
"If it goes wrong" sets what botched work costs, the table a quirk is rolled
on, and a chance of a mishap from a table on every roll; a table's rows are
shown read-only (they are edited on the Rolling Tables page), with "Try a
roll" and "Watch" for each mishap's moment. "Extras" holds breakthroughs,
mastery, substitutes and the settings that need an item to remember
something of its own (quality, shelf life, the maker's mark) or a player
action that doesn't exist yet (experimenting, salvage, others helping);
those are saved and labelled "Not yet carried out at the roll".

## How it looks

Moments: every brew's success, progress, failure and breakthrough, then each
rule players see (not drafts, not secret). A moment's look is
`{spots: {spot: {look, move}}, words}` over `ItemLookFX.MOMENT_SPOTS`
(`window` the whole time, `heat` while it heats, `work` while it brews, `bench`
at the moment, `result` when it's done), with `ItemLookFX.LOOKS` and two moves
(`shake`, `dim`). A recipe with mishaps gets a Mishaps group, one moment per
table row, played from the "If it goes wrong" tab's Watch. Drag an effect onto a spot, or tap it and then a spot; tap a
filled spot or its chip's × to empty it (with Undo). "Back to the default"
appears once a moment differs from where it started. A new rule gets a look
from its tone (`defLook`).

The stage subclasses `ItemLookFX.Stage`, so it shares the one clock, sleeps off
screen, and follows `html[data-motion]`: under Calm no shake and a lighter
trickle; under Off it draws still until Play is pressed. Better effects added
to `ItemLookFX.LOOKS` appear on the shelf with no change here.

## The AI

The ask bar at the top takes what the Director wants in words. "Draft it"
copies the brief with that request (`request`, `answerWith`) and opens the
paste box; Chronicle runs no AI of its own, so the answer comes back by
paste.
"Copy what it needs" copies `CraftRules.aiBrief`: how rules work, this
calendar's names (moons with their phases or states, events, rolled events and
results, weather, seasons), the campaign lists, every shape, and the recipe's
rules as data. Without a clipboard (a page not served over HTTPS) the brief is
shown to select by hand. "Paste what it wrote" takes one rule, a list, or
`{"rules": […]}` (code fences are stripped) through `parseAI`, and adds what it
can read as drafts. "Show what the AI writes" shows each rule's data under it.

## Tests

- `test/js/craft_rules_logic.test.mjs`: the logic in a vm with `sky_world.js`.
  Phases match the calendar page's, the built-in weathers are the generator's,
  cleaning, status, verdict, sentences, events and rolls, starters, the brief
  round trip.
- `test/js/craft_rules_browser.test.mjs`: the widget mounted by `boot.js` in
  Chromium: the timeline's verdicts, a card opening and changing, common rules,
  pasted drafts and Approve, the copied brief reading back as the same rules,
  placing an effect, motion off, a real calendar from a faked API, the demo
  fallback, destroy.

## Not here yet

- TODO(#1883): ingredient marks at a project roll. The server's reading
  (`internal/plugins/crafting/verdict.go`) leaves them "maybe", as the
  preview does.
- TODO(#1824): the recipe editor's fold-out over the crafting widget.
