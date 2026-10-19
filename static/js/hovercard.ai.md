# Hover card (`hovercard.js`, `static/css/paper.css`)

The one card every content hover-over opens: linked pages
(`widgets/entity_tooltip.js`), rule words (`widgets/rulebook.js`), and any
package widget that previews content. Callers describe the content; the card
owns behaviour and look. Never style a hover card of your own.

## Using it

```js
var unbind = Chronicle.hovercard.bind(root, '.my-term', function (el) {
  return { kind: 'Condition', title: 'Grabbed', text: '…',
           link: { href: '/campaigns/1/rules#grabbed', label: 'Open in the rulebook' } };
  // or a Promise of that, or null for no card
}, { pinOnClick: true });   // words that are not links: a click pins the card
```

`{ corner: true }` draws the card as the fading corner instead of the
campaign's look; time lines pass it when the owner picked the corner
(`<html data-cz-tlpop="corner">`). `{ kit: true }` draws the card in the
page's style kit (look `kit`) instead of the campaign's look while
`<html data-cz-kit>` is set; time lines pass it, so the classic card on a
time line wears the kit and every other hover card keeps its own look.

Content fields: `kind`, `kindIcon` (Font Awesome name), `title`, `locked`,
`pic`, `badge` (a second, round picture on the corner of `pic`: a creature's
token beside its art), `facts` (`[[label, value]]`), `text`, `foot`,
`link {href,label}`, `extraHTML`. All but `extraHTML` are escaped; `extraHTML` must already be
safe markup. Links are followed only for same-site paths, `#` fragments and http(s).
`Chronicle.hovercard.html(content)` returns the markup for a still preview
(the Customize tiles). `open`, `update`, `close`, `current` drive it by hand.

## Behaviour

- Opens after the pointer rests 250 ms; closes 180 ms after it leaves, unless
  it moved onto the card.
- Click (`pinOnClick`) or a long press on a link (touch) pins it with a close
  button; a click elsewhere closes it.
- Keyboard focus on the trigger opens it; Escape closes it and returns focus.
- One card at a time. Kept 8 px inside the window; flips above the trigger
  near the bottom. Scrolling closes an unpinned card and moves a pinned one.
- The fading corner sits above the trigger, its edge on the trigger's
  centre and its 7 px stem ending 8 px from it (room for a bead grown on
  hover); it flips to the trigger's left near the right edge (`chc--cr`) and
  below it near the top of the window (`chc--cd`).
- Motion follows the device setting and the campaign's reduce-motion switch.
- A card with a `badge` opens with its art settling into its frame (320 ms)
  and the badge popping onto the corner a beat later (260 ms, 140 ms in);
  reduced motion, Calm and Off show both still.

## Looks

`<html data-cz-hover>` from Customize › Hover cards (`Appearance.HoverCard`,
`layouts.AppearanceAttrs`): absent = **Paper**, or `plain`, `night`,
`compact`. Paper stays paper in dark mode. `corner` (the fading corner) is
set per card by the `corner` option, never by `data-cz-hover`; it draws in
the site's own colours with the campaign colour for its edge. `kit` is set
per card by the `kit` option. Under a style kit both take the kit's palette
and faces, and their corners, edges, fill and shadow from the `--kit-tl-*`
tokens on `#chc` (`style_kits.css`, "Time lines"); the Customizer's sample
popups are not `#chc`, so they keep Material.

## Shared paper

`paper.css` also defines the paper every paper-styled screen uses: tokens
`--paper`, `--paper-cut`, `--paper-under`, `--paper-edge`, `--paper-ink`,
`--paper-ink-soft`, `--paper-accent`, `--paper-burn`, `--paper-font`,
`--paper-lift`, `--paper-grain`, `--paper-fibre`, `--paper-ui-font`; classes `.paper` (a sheet
with aged edges, grain and a lifted shadow), `.paper-stack` (wrapper that
tucks a turned sheet behind its `.paper` child; must not clip),
`.paper-kind` and `.paper-title`. Colours match the Rules book.

Tests: `test/js/hovercard.test.mjs`, `test/js/timeline_popup_browser.test.mjs` (the corner), `test/js/timeline_kits_browser.test.mjs` (the kits).
