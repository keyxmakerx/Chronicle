# Credits & Third-Party Assets

## Icons

### game-icons.net (CC-BY 3.0)

The Almanac calendar showcase (`/demo/calendar/almanac`) vendors the following
icons from [game-icons.net](https://game-icons.net) (mirror:
[github.com/game-icons/icons](https://github.com/game-icons/icons)), licensed
under [Creative Commons Attribution 3.0](https://creativecommons.org/licenses/by/3.0/):

| Icon | Author | File |
|------|--------|------|
| Sun | Lorc | `static/vendor/game-icons/lorc/sun.svg` |
| Eclipse | Lorc | `static/vendor/game-icons/lorc/eclipse.svg` |

Vendored locally (no runtime CDN) and inlined into the sky-band sun renderer,
recolored per time-of-day / event state via CSS. Originals are kept verbatim
under `static/vendor/game-icons/` to satisfy the attribution requirement.

### Google Noto Emoji (OFL 1.1)

The Almanac moon library vendors the lunar-phase emoji set from
[Google Noto Emoji](https://github.com/googlefonts/noto-emoji), licensed under
the [SIL Open Font License 1.1](https://scripts.sil.org/OFL):

- 8 lunar phases `U+1F311`–`U+1F318` + crescent `U+1F319` + 4 face variants
  `U+1F31A`–`U+1F31D` → `static/vendor/noto-emoji/moons/`

### Twitter Twemoji (CC-BY 4.0)

The Almanac moon library also vendors the same lunar Unicode set from
[Twemoji](https://github.com/jdecked/twemoji) (maintained fork), licensed under
[Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/):

- Same `U+1F311`–`U+1F31D` set → `static/vendor/twemoji/moons/`

Both sets are vendored locally (no runtime CDN); the owner picks a moon's
`phaseSource` (`noto` / `twemoji` / `css-clip`) per moon.

### Procedural moon designs (coordinator-authored)

The 12 procedural moon SVGs under `static/vendor/cal-moons/` (watercolor,
holographic, etched, constellation + 8 realistic shaded variants) are extracted
verbatim from the design-session prototypes
(`docs/design/world-state-effects/prototypes/moon-procedurals-batch2.html` +
`moon-realistic-iterations.html`) — coordinator-authored, no third-party
attribution required.

