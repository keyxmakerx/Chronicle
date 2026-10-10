# quest_board.js, notice_boards.js, quest_board_kit.js

## Purpose

The two cork-board blocks of the quests plugin. **Quest board** sits on a
quest page: the notice players read, the map scrap, the reward tag, and the
DM's Quest Ledger (steps, rewards, foes, links, looks). **Notice boards** sits
on a place page (a tavern) or on a category dashboard (the same boards, keyed to the category): one or more boards where the DM posts quest
notices and players pin pages, maps, notes and string between them, with the
DM's Board Ledger (boards, quests, players, looks).

`quest_board_kit.js` (`window.QuestBoardKit`) is what both share: the icons,
the overlay layer (toast, reading sheet), inline editing, the ledger shell and
its list controls, board dragging, the picker and the motion helpers. It must
load before the two widgets.

## Mount

Source: `internal/plugins/quests/static/js/`, loaded on sight by the quests
plugin's `Widgets` registration (ADR-063): each widget's list puts the kit
first, and a page with both boards fetches the kit once. The
blocks `quest_board` and `notice_boards` are registered in
`internal/app/routes.go`; their markup is `internal/plugins/quests/block.templ`,
which also links `static/css/quest_board.css` and the period fonts.

| Attribute | Meaning |
|---|---|
| `data-endpoint` | `/campaigns/:id/quests/:eid`, `/campaigns/:id/notice-boards/:eid` or `/campaigns/:id/category-boards/:tid`; every sub-URL is built from it |
| `data-campaign-id` | Base for picker, map, page and Armory URLs |
| `data-csrf-token` | Sent on every write; the category mount omits it and `Chronicle.apiFetch` uses the page token |
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
- The ledger's due row is a day picked on the campaign calendar when the DM
  view has a `dueCalendar` (sent as `dueDate`; the server returns the `due` label
  and days left and keeps a calendar event in step); without one it is the free
  text `notice.due`. Notices show `daysLeft` on the board and in the reading
  sheet, and an unfinished one turns late when it is negative.
- Ledgers open by clicking the bookmark and fold away on a click outside or
  Escape, unless pinned. While the hand-out panel is open a fold-away asks
  first if something was picked.
- Every ledger list has add (Enter), rename (double-click or F2), remove (×
  then "Remove?") and drag-to-reorder by the grip.
- Hand out splits a money reward between the ticked characters and pays each
  share through `POST /armory/pay`; items go through `POST /armory/give`. Steps
  run one at a time and finished ones are remembered, so pressing Hand out
  again after a failure never pays or gives twice.
- Both widgets stay current: `QuestBoardKit.live` keeps one socket per
  campaign, and a `quest.updated` or `notice_boards.updated` for what is shown
  (or a reconnect, or coming back to the tab) refetches through the widget's
  own route. A refetch waits while the viewer is typing, dragging, tying
  string, reading a notice or handing out, so nothing in progress is lost.
- Looping animations slow to rest through `MotionRest`, and stop under
  reduced motion.
