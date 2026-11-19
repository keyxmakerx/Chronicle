# put_on_page.js

The "Put on a page" chooser (widget `put-on-page`, an on-sight script in
`coreWidgets`). A `[data-put-block="<block type>"]` button inside the widget's
element opens a list of the campaign's categories under it; choosing one
appends the block to the end of that category's page layouts.

- Host: any element with `data-widget="put-on-page"` and `data-campaign-id`.
  The Game & features page's "For your pages" section and the DM Screen's
  Trackers section (`homebrew_drawer.js`) both use it, so the chooser exists once.
- Blocks it knows how to name: `quest_board`, `notice_boards`, `trackers`,
  `page_tests` (see `NOUN`).
- Reads `GET /campaigns/:id/entity-types/placement?block=` for the categories,
  the suggested one first (the server owns that rule) and whether each already
  holds the block; writes `POST /campaigns/:id/entity-types/:etid/layout/blocks`
  with `{type}`. The answer is `added` or `already_there`; the toast says which.
  Both routes are owner and co-DM only.
- Shows three categories, then "Another category…" for the rest.
- Motion: the menus' `.mn-enter/.mn-leave/.mn-from/.mn-to` classes; Off shows it
  at once. Styles are `.put-pop` in `static/css/input.css`.
- Pure helpers are on `window.PutOnPage` for `test/js/put_on_page.test.mjs`.
