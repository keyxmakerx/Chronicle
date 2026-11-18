#!/usr/bin/env node
// build-signin-scenes.mjs bakes the sign-in scenes' drawn layers
// (tools/signin-scenes/*.svg) into the blurred WebP layers the page serves
// (static/img/signin/). The scenes stay readable: only the back layer gets a
// light blur, for depth, baked in so the animated layers need no filter at
// run time.
//
// Usage: NODE_PATH="$(npm root -g)" node tools/build-signin-scenes.mjs
// Needs global playwright + chromium and python3 with Pillow (WebP encode).
import { createRequire } from 'node:module';
import { readdirSync, readFileSync, mkdirSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

// playwright is a global install, found through NODE_PATH, which ES imports ignore.
const { chromium } = createRequire(import.meta.url)('playwright');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const src = path.join(root, 'tools/signin-scenes');
const out = path.join(root, 'static/img/signin');
mkdirSync(out, { recursive: true });

// 1920x1080 keeps the drawing crisp on a large screen, where the layer is
// shown wider than the viewport. The drawing is laid 140px past each edge so
// the back layer's blur never fades to transparent at the rim.
const W = 1920, H = 1080, PAD = 140;
const TONE = 'brightness(1.08) saturate(1.1)';
const filterFor = (f) => (f.includes('-1-back') ? `blur(3px) ${TONE}` : TONE);

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: W, height: H } });
const pngs = [];
for (const f of readdirSync(src).filter((n) => n.endsWith('.svg'))) {
  const b64 = readFileSync(path.join(src, f)).toString('base64');
  await page.setContent(
    `<body style="margin:0;overflow:hidden;background:transparent"><img src="data:image/svg+xml;base64,${b64}" ` +
    `style="position:absolute;left:-${PAD}px;top:-${Math.round(PAD * H / W)}px;width:${W + 2 * PAD}px;` +
    `height:${Math.round((W + 2 * PAD) * H / W)}px;filter:${filterFor(f)}"></body>`);
  await page.waitForTimeout(300);
  const png = path.join(out, f.replace('.svg', '.png'));
  await page.screenshot({ path: png, omitBackground: true });
  pngs.push(png);
}
await browser.close();

execFileSync('python3', ['-I', '-c', `
import sys, os
from PIL import Image
for p in sys.argv[1:]:
    Image.open(p).save(p[:-4] + '.webp', 'WEBP', quality=86, method=6)
    os.remove(p)
`, ...pngs]);
