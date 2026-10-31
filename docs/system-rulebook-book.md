# Writing a system's Rulebook book

A system package can ship a **book**: the Rules page then opens as a
full-screen, page-turning rulebook instead of the category browser. Chronicle
draws it (pages, turns, contents, hover cards, Player and Director views); the
package supplies only YAML files saying what is on each page. No programming.

## Where the files go

```
your-package/
  manifest.json
  book/
    book.yaml            the cover: title, look, and the list of chapters
    chapters/
      basics.yaml        one file per chapter
      combat.yaml
      ...
    art/
      cover.svg          optional: the drawn cover
    fonts/
      display.woff2      optional: the book's own fonts
```

A package has a book when `book/book.yaml` exists. Files are
[YAML](https://yaml.org): `name: value` lines, `- ` for list items, `|` for
multi-line text indented underneath. Quote a value that is a bare `Null`,
`true` or `false`, or starts with `*`, `[` or `{`.

`book/` is never served as-is: Chronicle reads and checks it, strips what the
reader may not see, and sends the rest. The exception is the cover and font
files `book.yaml` names: exactly those are sent to readers who may open the
book.

## book.yaml

```yaml
title: Rulebook
mark: DS                 # up to 3 letters, shown as the book's badge
turn: flip               # flip (default), slide, or fade
glossary: rules-glossary.json   # optional: hover definitions from a data file
theme:                   # optional: colours and fonts, all optional
  paper: "#0d1017"
  paper-alt: "#131722"
  ink: "#f2f4fa"
  ink-soft: "#c9d0e0"
  muted: "#8a93a8"
  edge: "#262c3d"
  accent: "#a78bfa"
  heading-font: "Inter, system-ui, sans-serif"
  body-font: "Inter, system-ui, sans-serif"
  ui-font: "Inter, system-ui, sans-serif"   # labels, buttons and the bar
  cover: "#2e2240"       # the leather around the pages (paper look only)
look: paper              # optional: paper, bold, or left out
cover: art/cover.svg     # optional: a drawn cover the book opens from
fonts:                   # optional: the book's own fonts, at most 8
  - family: "Big Shoulders Display"
    file: fonts/big-shoulders-display-latin-900-normal.woff2
    weight: 900          # optional: 100 to 900, default 400
    style: normal        # optional: normal (default) or italic
tabs: [players-screen, how-to-play, combat]   # optional: chapters with a tab
start: players-screen    # optional: the chapter the book opens on
first: how-to-play       # optional: where a first-time reader opens instead
screen: players-screen   # optional: a chapter that drops over any page
terms:                   # optional: extra hover definitions
  edge: A bonus on a power roll.
parts:
  - title: Player's book
    chapters: [basics, making-a-hero, combat]
  - title: Director's book
    director: true       # only Directors see this part
    chapters: [running-the-game, monsters]
```

- `chapters` names files in `book/chapters/` without the `.yaml` ending.
  Names use lower-case letters, digits and `-`.
- Colours are `#` hex values. Fonts are font names separated by commas.
- `glossary` names a file in the package's `data/` folder: a list of entries
  with `name` and `summary`. Each becomes a hover definition.
- `look: paper` draws pages as paper inside a cover with a crease between
  them; set the paper colours in `theme` (light `paper`, dark `ink`).
- `look: bold` draws a poster-style book: dark chapter banners, heavy headings,
  coloured result tiers, coloured from `theme` (`accent` colours banner edges
  and tiers). A book is `paper` or `bold`, not both. Navigation is the Sections
  ribbon and the bar under the book, so a bold book has no contents drawer or
  tabs. Each chapter gets a colour from a fixed set, in book order, for its
  banner edge, page numbers and the intro's first letter. A chapter that is a
  single `wide` contents page takes the left page, with the next chapter
  opening beside it.
- A book's `look`, colours and fonts belong to its system; the site's Customize
  settings never change them.
- `cover` is a drawn cover. On first open in a visit the book shows closed; a
  click, Enter or Space opens it, and **Close the book** in the top bar closes
  it. Draw the whole cover, title lettering included; Chronicle adds only an
  "Open" label. SVG at 400 by 516, cropped to fill, shown as a picture (scripts
  never run), path `art/<name>.svg`. With reduced motion it is a still that a
  click dismisses.
- `fonts` loads font files the book ships. Each entry has `family`, a `file`
  of the form `fonts/<name>.woff2`, and optionally `weight` and `style`; list
  one entry per weight or style. Use faces whose licence allows
  redistribution (SIL OFL does). Reference the family in `theme`'s
  `heading-font`, `body-font` or `ui-font`.
- Cover and font file names use lower-case letters, digits and `-`. Files must
  be in the package, at most 1 MB each. A missing or oversized file, or a name
  outside these folders, is reported like any other `book.yaml` mistake.
- `tabs` puts a row of tabs above a paper book, one per named chapter in that
  order (at most 12). A House rules tab is appended when the campaign has
  house rules.
- The book opens on `start`; a first-time reader opens on `first`; a link
  ending in `#<chapter>` (e.g. `#making-a-hero`) opens that chapter. Without
  `start` the book reopens where the reader left it.
- `screen` names a chapter of quick-reference pages (e.g. a player screen),
  left out of the page order and tabs. A ribbon hangs from the top bar on every
  page (or press P); its pages drop down side by side over the book, and Escape,
  the ribbon or a click on the book folds them away. Contents entries, links
  and search results for that chapter open it the same way. If `start` names it,
  the book opens with the screen down. Eight or more cards on a screen page
  get a find-by-name box.
- `tabs`, `start`, `first` and `screen` must name chapters listed in `parts`. A
  player never receives a tab, start, screen or link to a Director-only
  chapter.

## A chapter file

```yaml
title: Combat
intro: How a fight runs, from the first turn to the last.
pages:
  - title: Your turn
    blocks:
      - text: |
          On your turn you get a **main action**, a **maneuver**, and a
          **move action**. A hit can leave a target [[bleeding]].

          A blank line starts a new paragraph.
          - A line starting with a dash is a list item.
      - type: flaps
        items:
          - title: Main action
            text: Usually an ability.
          - title: Maneuver
            text: Something quick, like standing up.
```

Chapters, pages and blocks can all carry `director: true` to show them to
Directors only. A page with `wide: true` spans both pages of the spread, and
`columns: true` flows its blocks into two columns (useful for a reference
screen).

### A rules-index chapter

Instead of pages, a chapter can name one of the system's reference data
categories (a `categories` entry in `manifest.json`). Chronicle then makes the
pages itself from the data file, so the book lists every entry and stays in
step when the data changes:

```yaml
title: Abilities
intro: Every hero ability, one page per class.
index:
  category: abilities      # a manifest category; its data/<category>.json
  group: class             # optional: one page per value of this property
  other: Common abilities  # optional: the page for entries without one
```

- With `group`, each value gets its own page, in order (numbers count up, so
  a `level` group reads Level 1, Level 2, Level 10). Without it, the entries
  run A to Z, `per-page` to a page (40 unless set, at most 100).
- Each entry folds down to show the category's manifest fields (the same
  columns the category list shows) and its description. Rule markup such as
  `{@condition taunted|taunts}` becomes a hover word.
- A field marked `gm_only` in the manifest is shown to Directors only.
- `director: true` on the chapter keeps the whole index for Directors.
- A chapter has `index` or `pages`, never both. Generated chapters can't be
  changed in a campaign's book editor; they follow the package.
- The book's search box searches written pages and every rules-index entry.

### Writing text

- `**bold**` and `*italic*`.
- `[[term]]` underlines a word and shows its definition on hover, tap or
  keyboard focus. `[[term|what to show]]` shows different wording.
- A blank line starts a new paragraph; a line starting with `- ` is a list item.

## Blocks

Every block is one item under `blocks:`. Plain text needs no `type`.

| Block | What the reader sees | What you write |
|---|---|---|
| text | Paragraphs | `text:` |
| callout | A highlighted box | `title:`, `text:` |
| roll | Dice they can roll, with the result's band highlighted | `dice:` (`NdM`, 1-10 dice of 2-100 sides, like `2d10`), `label:`, `modifier: true` to offer a bonus picker, `bands:` |
| cards | A row of cards that unfold for detail | `items:` each with `title:`, `summary:`, `text:` |
| flaps | Headings that open to explain | `items:` each with `title:`, `text:` |
| example | A worked example, stepped through line by line | `title:`, `steps:` (a list of text) |
| creature | A creature as heroes see it; Directors also see its numbers | `name:`, `tagline:`, `look:`, `notice:` (list), `stats:` (list of `label:`/`value:`), `note:` |
| note | A Director's note | `text:` (always Director-only) |
| links | Buttons that turn to other chapters | `title:`, `items:` each with `title:`, `summary:`, `chapter:` (a chapter name) |
| contents | Every chapter the reader can see except the one it is on, numbered, with the page each starts on; a pick turns to it. On a `wide` page it runs in two columns | `title:` (optional) |
| widget | One of the package's own widgets, for things no block covers | `widget:` (a slug from `manifest.json`'s `widgets`) |

A roll's `bands` go lowest first. Every band but the last has `max:`, the
highest total it covers:

```yaml
- type: roll
  dice: 2d10
  label: Power roll
  modifier: true
  bands:
    - max: 11
      label: Tier 1
      text: A weak result.
    - max: 16
      label: Tier 2
      text: A solid result.
    - label: Tier 3
      text: A strong result.
```

A creature's `stats` and `note` are Director-only: players see the creature's
look and what they notice, and a `?` where the numbers would be.

## The Sections bookmark

Every book has a **Sections** bookmark at the top of the right-hand page. It
drops a list of the book's chapters grouped by part, with a multi-page
chapter's pages beneath it; picking one turns to that page, and Escape or a
click elsewhere closes it. Directors see Director-only parts and chapters;
players do not. The `screen` chapter is left out. The list is built from
`parts`; nothing to write.

## Player and Director

A Director is the campaign owner or a member granted Director visibility.
Directors see everything and can switch to the player view. Players receive
only the player view: Director-only parts, chapters, pages, blocks, creature
numbers and notes never leave the server.

## Editing a campaign's copy

Directors see an **Edit** link on the Rules page to change their campaign's
copy: rewrite a page, add pages to a chapter, add a house-rules chapter. The
package files are never touched, so a package update does not overwrite
campaign edits; a page edited by the campaign and since changed by the package
is flagged so the Director picks which text to keep.

The owner can also have an AI write house rules: on the campaign's Your AI
page, ask for something with **House rules** as its content. The AI answers
with `kind: house-rule` blocks whose front matter is a chapter file (`intro`,
`director`, `pages`); the owner reads the pages, drawn in the Handbook's look,
before anything is saved.

The editor downloads the campaign's copy as a zip of these same files
(`book/book.yaml`, `book/chapters/*.yaml`), house-rules chapters as
`house-<name>` files in a "House rules" part, so an author can fold changes
back into the package.

## When something is wrong

Chronicle checks every file when the Rules page opens. A chapter with a
mistake is replaced by a page saying so: Directors see the file, page, block
and problem; players see only that the chapter could not load. The rest of the
book still works.

Limits: 200 chapters, 200 pages per chapter, 60 blocks per page, 60 items per
block list, 1 MB per file.
