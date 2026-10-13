# Writing a system's Rulebook book

A game system package can ship a **book**: the Rules page then opens as a
full-screen, page-turning rulebook instead of the category browser. Chronicle
draws the book (pages, page turns, contents, hover cards, the Player and
Director views); the package only supplies plain text files saying what is on
each page. Nothing here needs programming.

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
[YAML](https://yaml.org): indented `name: value` lines, with `- ` starting
each item of a list. Text that runs over several lines starts with `|` and is
indented underneath. Put a value in quotes if it is a bare `Null`, `true` or
`false`, or starts with a symbol like `*`, `[` or `{`; otherwise YAML reads it
as something other than text.

The `book/` folder is never served to browsers as-is. Chronicle reads it, checks
it, removes anything the reader isn't allowed to see, and sends only the rest.
The one exception is the cover and font files that `book.yaml` names (below):
those exact files, and no others, are sent to readers who may open the book.

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
- `look: paper` draws pages as paper inside a cover, with a soft crease
  between them. Set the paper colours in `theme` (light `paper`, dark `ink`).
  A book's `look`, colours and fonts belong to its game system: the site's
  Customize settings never change them.
- `look: bold` draws a poster-style book: dark chapter banners, heavy
  headings and coloured result tiers. It takes its colours from `theme`
  (`accent` colours the banners' edge and the tiers), so a book chooses
  `paper` or `bold`, not both.
- `cover` is a drawn cover: the first time a reader opens the book in a
  visit it shows closed, and a click (or Enter or Space) swings it open. A
  **Close the book** button in the top bar swings it shut again. Draw the
  whole cover in the file, title lettering included; Chronicle adds only a
  small "Open" label. The art is an SVG at 400 by 516, filling a closed book
  and cropped to fit. It is shown as a picture, so scripts in it never run,
  and the path must be `art/<name>.svg`. Readers who ask for reduced motion
  see the cover as a still that a click dismisses.
- `fonts` loads font files the book ships so it does not depend on a font
  being installed. Each entry has the `family` name, a `file` that must be
  `fonts/<name>.woff2`, and optionally a `weight` and `style`. Use faces whose
  licence allows redistribution (the SIL Open Font License does). Name the
  family in `theme`'s `heading-font` or `body-font` to use it. List one entry
  per weight or style.
- Cover and font file names use lower-case letters, digits and `-` and the
  files must be in the package, at most 1 MB each. A missing or oversized
  file, or a name outside these folders, is reported like any other mistake in
  `book.yaml`.
- `tabs` puts a row of tabs above the book, one per named chapter, in that
  order (at most 12). When the campaign has written house rules, a House
  rules tab is added at the end.
- The book opens on `start`. A reader who has never opened it opens on
  `first`. A link to the Rules page ending in `#<chapter>` (for example
  `#making-a-hero`) opens that chapter instead. Without `start` the book
  reopens where the reader left it.
- `screen` names a chapter of quick-reference pages, such as a player
  screen. It is left out of the page order and the tabs. Instead a ribbon
  hangs from the top bar on every page (or the reader presses P), and its
  pages drop down side by side over the book; Escape, the ribbon or a click
  on the book folds them away. Contents entries, links and search results
  for that chapter open it the same way. When `start` names the same
  chapter, the book opens with the screen down. A list of eight or more
  cards on a screen page gets a box to find a card by name.
- `tabs`, `start`, `first` and `screen` must name chapters listed in
  `parts`. A player never receives a tab, start page, screen or link that
  leads to a Director-only chapter.

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
| roll | Dice they can roll, with the result's band highlighted | `dice:` (like `2d10`), `label:`, `modifier: true` to offer a bonus picker, `bands:` |
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

Every book has a crimson bookmark at the top of the right-hand page, labelled
**Sections**. Pressing it pulls the ribbon down into a list of the book's
chapters, grouped by part; a chapter with more than one page lists its pages
beneath it. Picking an entry rolls the ribbon back up and then turns to that
page. Escape or a click elsewhere rolls it up too. Directors see their
Director-only parts and chapters in the list; players do not. The `screen`
chapter is left out, since it has its own ribbon. Nothing needs writing for
it: the list is made from `parts`.

## Player and Director

A Director is the campaign owner, or a member the owner has granted Director
visibility. Directors see everything and can switch to the player view to
check what players get. Players are sent only the player view: Director-only
parts, chapters, pages, blocks, creature numbers and notes never leave the
server.

## Editing a campaign's copy

The files above are the package's book. A campaign's Directors (the owner and
any member given Director visibility) can change their own campaign's copy
inside Chronicle: the Rules page shows an **Edit** link to them. They can
rewrite a page, add pages to a chapter and add a chapter of house rules. The
package files are never touched, so updating the package does not overwrite
anything a campaign wrote; a page a campaign edited that the package has since
changed is flagged so the Director can choose which text to keep.

The campaign owner can also have an AI write house rules: Settings, Data & AI,
then the AI tool's prompt with **House rules** as its content. The AI answers
with `kind: house-rule` blocks whose front matter is a chapter file like the
one above (`intro`, `director`, `pages`), and the owner reads the pages, drawn
in the Handbook's own look, before anything is saved.

A campaign's copy can be downloaded from the editor as a zip of these same
files (`book/book.yaml` and `book/chapters/*.yaml`), with house-rules chapters
added as `house-<name>` files in a "House rules" part, so a package author can
fold a campaign's changes back into the package.

## When something is wrong

Chronicle checks every file when the Rules page opens. A chapter with a mistake
is replaced by a page saying so; Directors see where the mistake is (file, page
and block) and what is wrong, players see that the chapter couldn't load. The
rest of the book still works.

Limits: 200 chapters, 200 pages per chapter, 60 blocks per page, 1 MB per file.
