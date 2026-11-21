# shop_stock_fill.js and shop_stock_gen.js

## Purpose

Fill the shelves: a Director's draft over a shop's stock list. Pick the kind of
shop, how many goods, how dear, and which sources (everyday goods, the
campaign's items, the system's goods, a rolling table); pin, leave out or edit
goods, roll again, then Keep. Nothing is saved until Keep.

## Pieces

- `shop_stock_gen.js`: no DOM. `ShopStockGen.run(opts)` returns
  `{goods, summary, stats}`; the same seed gives the same draft. Priced items
  and system goods keep their own price; only the everyday list scales, by a
  day's food. A good with no known price has `price: null` and is never given
  a made-up one. Rare finds (system goods, unpriced items, table lines, flagged
  everyday goods) are marked `today` and go on today's shelf.
- `shop_stock_fill.js`: `Chronicle.shopFill.open(host, cfg)` draws the sheet in
  the stock block; `cfg.onKept` receives `{batch, relations, skipped, unpriced}`
  for the Undo line.
- `static/shop-goods/everyday.json`: the everyday list, original names and
  prices in days of food, with counts and the shop kinds that carry each.

## Calls

`GET armory/shops/:eid/fill-sources`, `GET armory/shops/:eid/room`,
`GET roll-tables`, `POST armory/shops/:eid/stock` (Keep, all or nothing),
`POST armory/shops/:eid/stock/undo`, `PUT armory/food-price`. Server side:
`internal/plugins/armory/shop_stock_service.go`; every route is Director only.

## Gotchas

- A good with no page becomes a page marked `entities.shop_good` (kept out of
  lists, search, Auto-link and sync until "Make it a full page"). Undo removes
  the listings of that Keep only; the pages stay and are reused.
- `render()` is re-entrancy safe: clearing the sheet drops the focused field,
  whose blur fires a change that renders again.
- Tests: `test/js/shop_stock_gen.test.mjs`, `shop_stock_fill_browser.test.mjs`.
