# Crafting journal fonts

Self-hosted woff2 files for the handwriting choices of the crafting bench's
paper journal skin (`static/js/widgets/craft_journal.js`), so the journal
never loads fonts from Google. Only the `latin` subset is kept. The journal's
other hands use the Customize page's Kalam and IM Fell files.

| File | Family | Licence |
|---|---|---|
| `caveat-latin-400.woff2` | Caveat | SIL Open Font License 1.1 |
| `cedarville-latin-400.woff2` | Cedarville Cursive | SIL Open Font License 1.1 |
| `homemadeapple-latin-400.woff2` | Homemade Apple | Apache License 2.0 |

All three come from Google Fonts' `css2` API with a Chrome user agent, the
same way `tools/fetch-customize-fonts.sh` fetches the Customize fonts.
