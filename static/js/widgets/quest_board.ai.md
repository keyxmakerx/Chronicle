# quest_board.js, notice_boards.js, quest_board_kit.js

## Purpose

The two cork-board blocks of the quests plugin. **Quest board** sits on a
quest page: the notice players read, the map scrap, the reward tag, and the
DM's Quest Ledger (steps, rewards, foes, links, looks). **Notice boards** sits
on a place page (a tavern): one or more boards where the DM posts quest
notices and players pin pages, maps, notes and string between them, with the
DM's Board Ledger (boards, quests, players, looks).

`quest_board_kit.js` (`window.QuestBoardKit`) is what both share: the icons,
the overlay layer (toast, reading sheet), inline editing, the ledger shell and
its list controls, board dragging, the picker and the motion helpers. It must
load before the two widgets.

## Mount

Loaded by `<script defer>` tags in `layouts/base.templ` (kit first). The
blocks `quest_board` and `notice_boards` are registered in
`internal/app/routes.go`; their markup is `internal/plugins/quests/block.templ`,
which also links `static/css/quest_board.css` and the period fonts.

| Attribute | Meaning |
|---|---|
| `data-endpoint` | `/campaigns/:id/quests/:eid` or `/campaigns/:id/notice-boards/:eid` |
| `data-campaign-id` | Base for picker, map, page and Armory URLs |
| `data-csrf-token` | Sent on every write |
| `data-can-edit` / `data-can-manage` | Draw the DM controls; the routes enforce the real rules |
| `data-member-role` | Notice boards only: which add-menu entries to draw |

## Data it calls

- Quest board: `GET`/`PUT <endpoint>` (partial PUT carrying `version`; a 409
  reloads), `GET /quests/picker?kind=page|map|character`, and for Hand out
  rewards `POST /armory/give` (form `character_id`, `item_id`, `quantity`,
  with `HX-Request` so the Armory answers 204), one linked item at a time.
- Notice boards: `GET <endpoint>`, the boards, items, order, looks and
  player-items routes under it (see the plugin's `.ai.md`), and the quest
  `GET`/`PUT` for stamping a notice done.

## Behaviour worth knowing

- What a viewer sees is decided by the server. A player's quest view has no
  foes, no hidden steps and no hidden pieces; a page a player cannot open
  arrives on a board as `concealed`, with no name or id.
- Ledgers open by clicking the bookmark and fold away on a click outside or
  Escape, unless pinned. While the hand-out panel is open a fold-away asks
  first if something was picked.
- Every ledger list has add (Enter), rename (double-click or F2), remove (×
  then "Remove?") and drag-to-reorder by the grip.
- Money rewards are shown split between the ticked characters; Chronicle has
  no coin payout, so the DM writes the shares on the sheets.
- Looping animations slow to rest through `MotionRest`, and stop under
  reduced motion.
