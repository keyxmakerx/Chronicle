# Made-item pictures

The Dark painted pictures `static/js/widgets/made_items.js` shows for made
items and shop goods, and how they are made. Run `./build.sh` (or
`./build.sh sword candle` for some) to rebuild them into `static/img/made/`.

- `src/` builds each item as a solid in three.js: `kit.js` the shape helpers,
  `mats.js` the materials (each driven by one colour), `stage.js` the picture
  room, `items/` the items, `items/demo.js` their names and own colours.
- `render.js` renders jobs in headless Chromium; `jobs.js` writes the jobs.
- Every item is rendered twice: in neutral greys (`greys.js` picks one grey
  per colour role, as light as the item's own colour), and as a mask that
  draws each surface in its role's channel, as strongly as its colour comes
  from the recipe (`maskItem` in `stage.js`).
- `pack.py` lays both through the paint pass (`paintm.py`, which moves the
  mask with the same brush strokes) and writes four-tile atlases: the neutral
  picture, then the body, trim and detail masks. The browser multiplies the
  greys by a recipe's colours.
- Items listed with a 1 at the end of their row in `made_items.js` move: 64
  frames, periodic, so the strip loops.
- `internal/madeitems` and `made_items.js` list the same items in the same
  order with the same colours (`TestKeysMatchTheScript`); `gen.mjs` prints
  both tables from `items/demo.js` when an item is added.

A part that should keep its own colour whatever the recipe (the orb's wooden
base, the crossbow's prod) is built with a fixed colour, never one that
depends on the recipe matching a particular value.
