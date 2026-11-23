# Card borders for a system's pick-list entries

An entry in a system package's `data/*.json` (an ancestry, a class, a kit...) can
ask for a decorative border around its card in the hero creator. Two details do it:

| Detail | What it is |
|---|---|
| `card_border` | The border's key, from the list below. |
| `card_colour` | The colour the border's coloured parts take, written as `#` and six hex digits, for example `#c4243a`. Optional. |

```json
{
  "slug": "stoneborn",
  "name": "Stoneborn",
  "source": "custom",
  "properties": {
    "card_border": "carved-stone",
    "card_colour": "#e0a030"
  }
}
```

Both are plain text details, so they go in `properties` like any other scalar.
The hero creator never lists them as facts on the card or in the entry's sheet.

## The borders

| Key | Shown name | Colour used when `card_colour` is left out |
|---|---|---|
| `carved-stone` | Carved stone | `#e0a030` |
| `earth` | Earth | `#b08a5a` |
| `fire` | Fire | `#d9622b` |
| `ice` | Ice | `#8fd8ff` |
| `storm` | Storm | `#6fa8dc` |
| `glass` | Glass | `#58c4d8` |
| `glass-smoke` | Smoke glass | `#9aa3ad` |
| `glass-sea` | Sea glass | `#4fb8a8` |
| `glass-rose` | Rose glass | `#e58fb0` |
| `glass-midnight` | Midnight glass | `#a78bfa` |
| `rune-metal` | Rune metal | `#3fc8d4` |
| `great-rune` | Great rune | `#b48cf2` |
| `mechanical` | Boiler works | `#6fa8dc` |
| `thorn-glass` | Thorn glass | `#c4243a` |
| `scales` | Scales | `#2fb5a4` |
| `vine` | Vine and wood | `#6fae4f` |

The key is matched exactly (lower-case). Chronicle owns the pictures; a package
cannot add a border or supply its own picture.

## What a wrong value does

- A `card_border` that is not text, or not one of the keys above, gives a plain
  card. The entry still works; nothing is shown to the player.
- A `card_colour` that is missing, not text, or not `#` plus six hex digits
  (`#fff` and `red` are both wrong) takes the border's own colour from the table.
  Upper-case digits are fine and are read as lower-case.
- A `card_colour` with no `card_border` does nothing.

An entry written in the campaign itself (rather than shipped in a package) is
checked when it is saved instead: a key outside the list, or a
colour that is not six hex digits, is refused with a message that lists the
valid keys.

## Where borders show

Only the hero creator's stage look draws borders. It is the default look; the
creator opened with `?look=classic` on its address is the classic one, which never draws them, and neither
creator lists the two details as facts.
The stage look draws the border on the entry's card on a phone and on a computer;
the parts of a border made to take colour (carved lines, runes, glowing cracks, a
vine, the dye of the scales) take the entry's colour, and the rest of the picture
stays as drawn. The "make your own" card and the list view stay plain.

## The writer of an entry picks its border

Whoever writes a campaign's own entry (a player with permission, or a Director)
picks the border its card wears in the entry form, in the "Card border" row
under the short description: a small sample, the border's name and Change. It
unfolds to a live card and the same colours and samples the Customizer uses.
The choice is saved with the entry, and a Director can change it afterwards.

## The campaign's owner can change a card's border

A campaign's owner can pick a different border, or none, for any card, a game
system's own entry included, in the Customizer. Their choice wins over the
package's `card_border` and `card_colour` for everyone who sees the card, and
choosing the package's own look again, or resetting, hands the card back to the
package.

The owner does this on the Customizer's Hero creator page: the real creator
stands in a frame with the campaign's cards, and pressing a card opens a panel
with the samples and colours. "The game system's" sample is the look the package
gives that card; "None" shows the card with no border even when the package gives
one. The change waits in the Customizer's save bar like any other. A campaign's
own entry keeps its border in the entry itself, not in this list.
