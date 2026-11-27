# Sign-off cards

Every question for the operator is a card on the Chronicle Sign-offs page
(an Artifact; its link is in the project instructions). This is the one format
a card is written in. The page draws every part of a card from data, so cards
cannot drift into different layouts.

## What becomes a card

Only three things: a **look**, **what a feature does**, and a step that
**can't be undone**. Merging approved work, small differences from a signed
mockup, clean-ups and duplicates go ahead without a card and are listed once a
week. Choices inside a thread (which file, which test, order of work) are
never asked.

## Writing one

Write the card as a document in the page's `cards` collection with
`form: 2`. Do not write a `body` of hand-made HTML; the page builds the card.

| Field | Required | What goes in it |
|---|---|---|
| `form` | yes | `2` |
| `type` | yes | `look`, `does` or `undo` (shown as Look, What it does, Can't be undone) |
| `title` | yes | The question, in plain words, as the operator would ask it |
| `status`, `version`, `order`, `addedAt` | yes | `open`; `1` (raise it on every change, so an earlier answer shows Re-check); the next order number; the time |
| `issue`, `issueUrl` | if one exists | `Chronicle#123` and its link |
| `pills` | no | Two or three short tags, such as `mockup`, `3 options`, `maps` |
| `due` | only if it blocks something | Finishes "Needed …", such as `before the 1.0 release`. Leave it out when there is no rush |
| `context` | yes | One to four lines of what is already true: `{text, state}`, state `settled` (the operator ruled it), `penciled` (my suggestion, not yet theirs) or `open` |
| `what` | yes | One short paragraph: what changes for the operator or their players. No code names |
| `today` | for looks | HTML picture of the screen as it is now |
| `questions` | yes | Usually one. Each: `{id, q, help, today, options}` |
| `questions[].options` | yes | Two to four. Each: `{label, desc, pic, rec, fresh}` |
| `why` | yes | One or two sentences on why the pick is the pick |
| `more` | no | HTML for detail most readers skip (folded away) |

Options:

- `label` is a few words; the page adds the letter (A, B, C) in order.
- `desc` is one line on what happens if it is picked: the consequence, not a
  restatement of the label.
- `pic` is that option's own mockup. **Every option has one** on a look card;
  the page then shows Today and each letter in one box and animates what moves.
- `rec: true` on exactly one option. It is pre-selected and marked "my pick".
- When two options are not exclusive, add one that combines them ("Both",
  "A on desktop, B on phones"). The page adds "Neither of these" to every
  question by itself, so don't write a neither option; do write the real
  alternative if "neither" would mean something specific (such as "leave it
  as it is today").
- `fresh: true` on at least one option that is a genuinely new approach, not
  the familiar version redrawn. Hard questions get research outside TTRPG
  tools first (`help` is a good place for one line of what others do).

Mockups follow `.ai/ui-standard.md`: Material look, readable at the size shown,
light and dark, using the page's `--c-*` colours.

## After adding or changing a card

1. Check the page still loads, with the new card showing.
2. In the thread, reply in one line pointing at the card. The question itself
   lives only on the card.
3. To replace a card, give the new one `replaces: "<old key>"` and set the old
   one's `status: "archived"`, `archiveReason: "replaced"`, `replacedBy`. Never
   delete a card or an answer in `decisions`.

## Answered cards are archived

The page downloads every document in `cards` on each visit, so answered cards
keep only their small fields there. Their full content (`body`, `questions`,
`today`, `more`, `context`, `what`, `why`) is in the `archive` collection under
the same key, and the card carries `bodyIn: "archive"`. The page reads the
archive copy only when a card is opened from history or comes back to Waiting.

To change an archived card, read `archive/<key>`, edit it, and write the whole
card back to `cards/<key>` with `version` raised and without `bodyIn`. Pictures
are best kept small: every image inside a card is downloaded with it.

## Example

```json
{
  "form": 2, "type": "look", "status": "open", "version": 1,
  "title": "Calendar day card: where the weather sits",
  "issue": "Chronicle#123", "issueUrl": "https://github.com/keyxmakerx/Chronicle/issues/123",
  "pills": ["mockup", "3 options"],
  "context": [
    {"text": "Weather per day is built and live.", "state": "settled"},
    {"text": "Players see only revealed days.", "state": "settled"}
  ],
  "what": "When you open a day, the weather sits under the events, so players often miss it.",
  "questions": [{
    "id": "q0", "q": "Where should the weather go on the day card?",
    "help": "Apple Calendar puts it beside the date.",
    "today": "<div>…today's day card…</div>",
    "options": [
      {"label": "Beside the date", "desc": "Seen first, takes no extra room.", "rec": true, "pic": "<div>…</div>"},
      {"label": "A band across the top", "desc": "Louder; uses one line.", "pic": "<div>…</div>"},
      {"label": "Only when you hover the day", "desc": "Calmest; players may never find it.", "fresh": true, "pic": "<div>…</div>"}
    ]
  }],
  "why": "Players look at the date first, so the weather is read without anyone hunting for it."
}
```
