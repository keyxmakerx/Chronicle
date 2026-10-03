#!/usr/bin/env bash
# Regenerates static/fonts/customize/ (woff2 files + fonts.json) from Google
# Fonts so Chronicle serves the Customize page fonts itself. Needs bash, curl,
# python3. Only the latin and latin-ext subsets are kept.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=static/fonts/customize
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
mkdir -p "$OUT"
rm -f "$OUT"/*.woff2 "$OUT"/fonts.json

# id|Google family (URL-encoded)|axis spec
SPECS='sourcesans|Source+Sans+3|ital,wght@0,400;0,500;0,600;0,700;1,400
atkinson|Atkinson+Hyperlegible|ital,wght@0,400;0,700;1,400;1,700
literata|Literata|ital,wght@0,400;0,500;0,600;0,700;1,400
sourceserif|Source+Serif+4|ital,wght@0,400;0,500;0,600;0,700;1,400
lora|Lora|ital,wght@0,400;0,500;0,600;0,700;1,400
alegreya|Alegreya|ital,wght@0,400;0,500;0,600;0,700;1,400
merriweather|Merriweather|ital,wght@0,400;0,700;1,400
cinzel|Cinzel|wght@600
marcellus|Marcellus|wght@400
imfell|IM+Fell+English|wght@400
cormorant|Cormorant+Garamond|wght@600
fraunces|Fraunces|wght@600
playfair|Playfair+Display|wght@600
josefin|Josefin+Sans|wght@600
chakra|Chakra+Petch|wght@600'

CSS_DIR=$(mktemp -d); trap 'rm -rf "$CSS_DIR"' EXIT
while IFS='|' read -r id fam axes; do
  curl -fsS -A "$UA" "https://fonts.googleapis.com/css2?family=${fam}:${axes}&display=swap" -o "$CSS_DIR/$id.css"
done <<<"$SPECS"

CSS_DIR="$CSS_DIR" OUT="$OUT" UA="$UA" python3 - <<'PY'
import json, os, re, subprocess
css_dir, out, ua = os.environ["CSS_DIR"], os.environ["OUT"], os.environ["UA"]
KEEP = {"latin", "latin-ext"}
manifest, downloaded = {}, {}
for fn in sorted(os.listdir(css_dir)):
    fid = fn[:-4]
    text = open(os.path.join(css_dir, fn)).read()
    # group faces sharing (style, subset, url): a variable font served per weight
    groups = {}
    for m in re.finditer(r"/\* ([\w-]+) \*/\s*@font-face \{(.*?)\}", text, re.S):
        subset, body = m.group(1), m.group(2)
        if subset not in KEEP:
            continue
        g = lambda k: re.search(k + r":\s*([^;]+);", body).group(1).strip()
        family = g("font-family").strip("'\"")
        weights = [int(w) for w in g("font-weight").split()]
        url = re.search(r"url\(([^)]+)\)", body).group(1)
        key = (g("font-style"), subset, url)
        d = groups.setdefault(key, {"family": family, "w": [], "ur": g("unicode-range")})
        d["w"] += weights
    faces = []
    for (style, subset, url), d in groups.items():
        lo, hi = min(d["w"]), max(d["w"])
        wt = str(lo) if lo == hi else f"{lo} {hi}"
        suffix = "-italic" if style == "italic" else ""
        name = f"{fid}-{subset}-{'var' if lo != hi else lo}{suffix}.woff2"
        if url not in downloaded:
            subprocess.run(["curl", "-fsS", "-A", ua, url, "-o", os.path.join(out, name)], check=True)
            downloaded[url] = name
        faces.append({"family": d["family"], "style": style, "weight": wt,
                      "file": name, "unicodeRange": d["ur"]})
    faces.sort(key=lambda f: (f["style"], f["file"]))
    manifest[fid] = faces
with open(os.path.join(out, "fonts.json"), "w") as f:
    json.dump(manifest, f, indent=2, sort_keys=True)
    f.write("\n")
PY
echo "wrote $OUT"
