# Customize page fonts

Self-hosted woff2 files for the web fonts the Customize page offers, so
Chronicle serves them itself instead of loading them from Google. Only the
`latin` and `latin-ext` subsets are kept. `fonts.json` lists every face per
font id (family, style, weight or weight range, file, unicode-range); a file
named `-var` is a variable font covering the weight range recorded there.

Every family here is licensed under the SIL Open Font License 1.1.

Inter is not included; Chronicle loads it separately.

## Regenerate

Run `tools/fetch-customize-fonts.sh` from anywhere (needs bash, curl, python3).
It fetches the Google Fonts CSS with a Chrome user agent, downloads the woff2
files and rewrites this directory's `.woff2` files and `fonts.json`. To add or
change a family or weight, edit the `SPECS` list in the script.
