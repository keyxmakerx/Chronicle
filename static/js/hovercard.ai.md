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

Content fields: `kind`, `kindIcon` (Font Awesome name), `title`, `locked`,
`pic`, `facts` (`[[label, value]]`), `text`, `foot`, `link {href,label}`,
`extraHTML`. All but `extraHTML` are escaped; `extraHTML` must already be
safe markup. Links are followed only for same-site paths and http(s).
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
- Motion follows the device setting and the campaign's reduce-motion switch.

## Looks

`<html data-cz-hover>` from Customize › Hover cards (`Appearance.HoverCard`,
`layouts.AppearanceAttrs`): absent = **Paper**, or `plain`, `night`,
`compact`. Paper stays paper in dark mode.

## Shared paper

`paper.css` also defines the paper every paper-styled screen uses: tokens
`--paper`, `--paper-cut`, `--paper-under`, `--paper-edge`, `--paper-ink`,
`--paper-ink-soft`, `--paper-accent`, `--paper-burn`, `--paper-font`,
`--paper-lift`, `--paper-grain`, `--paper-fibre`; classes `.paper` (a sheet
with aged edges, grain and a lifted shadow), `.paper-stack` (wrapper that
tucks a turned sheet behind its `.paper` child; must not clip),
`.paper-kind` and `.paper-title`. Colours match the Rules book.

Tests: `test/js/hovercard.test.mjs`.
