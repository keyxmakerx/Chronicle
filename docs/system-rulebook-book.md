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
```

A package has a book when `book/book.yaml` exists. Files are
[YAML](https://yaml.org): indented `name: value` lines, with `- ` starting
each item of a list. Text that runs over several lines starts with `|` and is
indented underneath. Put a value in quotes if it is a bare `Null`, `true` or
`false`, or starts with a symbol like `*`, `[` or `{`; otherwise YAML reads it
as something other than text.

The `book/` folder is never served to browsers as-is. Chronicle reads it, checks
it, removes anything the reader isn't allowed to see, and sends only the rest.

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
Directors only. A page with `wide: true` spans both pages of the spread.

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

## Player and Director

A Director is the campaign owner, or a member the owner has granted Director
visibility. Directors see everything and can switch to the player view to
check what players get. Players are sent only the player view: Director-only
parts, chapters, pages, blocks, creature numbers and notes never leave the
server.

## When something is wrong

Chronicle checks every file when the Rules page opens. A chapter with a mistake
is replaced by a page saying so; Directors see where the mistake is (file, page
and block) and what is wrong, players see that the chapter couldn't load. The
rest of the book still works.

Limits: 200 chapters, 200 pages per chapter, 60 blocks per page, 1 MB per file.
