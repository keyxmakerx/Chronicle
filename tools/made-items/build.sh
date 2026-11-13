#!/bin/bash
# build.sh [items] : renders and packs the painted made-item pictures into
# static/img/made/. Needs node, Playwright's Chromium, python3 with numpy and
# Pillow. Renders run four at a time in software WebGL; the whole set takes
# about an hour.
set -euo pipefail
cd "$(dirname "$0")"
[ -d node_modules/three ] || npm i --no-save three@0.170.0 esbuild@0.24.0
npx esbuild src/index.js --bundle --format=iife --global-name=RealItems --outfile=dist/set.js --log-level=warning
node greys.js
ALL=${*:-$(node -e "console.log(require('./order.json').join(' '))")}
MOV=$(node -e "const m=require('fs').readFileSync('../../static/js/widgets/made_items.js','utf8');const all=process.argv.slice(1);console.log(all.filter(k=>new RegExp('^\\\\s+'+k+': \\\\[.*, 1\\\\]','m').test(m)).join(' '))" $ALL)
mkdir -p out/still out/shelf out/clip
node jobs.js still out/js.json $ALL; node jobs.js shelf out/jsh.json $ALL; node jobs.js clip out/jc.json $MOV
# Slow and fast jobs alternate in the lists, so each queue takes a mix.
node -e "const fs=require('fs');const a=[].concat(...['out/js.json','out/jsh.json','out/jc.json'].map(f=>JSON.parse(fs.readFileSync(f))));for(let i=0;i<4;i++)fs.writeFileSync('out/q'+i+'.json',JSON.stringify(a.filter((_,j)=>Math.floor(j/2)%4===i)))"
for i in 0 1 2 3; do node render.js out/q$i.json > out/log$i.txt 2>&1 & done; wait
grep -h -E "FAIL|PAGEERR" out/log*.txt && exit 1 || true
python3 pack.py stills $ALL && python3 pack.py shelf $ALL && { [ -z "$MOV" ] || python3 pack.py clips $MOV; }
for d in s l shelf move; do mkdir -p ../../static/img/made/$d; cp pub2/$d/*.webp ../../static/img/made/$d/ 2>/dev/null || true; done
