// _browser_env.mjs — finds playwright-core and a Chromium for the *_browser
// tests. It looks in the usual places (NODE_PATH, /opt/node-tools, PW_CHROMIUM,
// /opt/pw-browsers, or the browser `playwright install` fetched). Locally a
// miss makes the tests skip, saying so; under CI it throws, so a browser
// guard is never silently unrun.

import { createRequire } from 'node:module';
import { readdirSync, existsSync } from 'node:fs';
import { join } from 'node:path';

function findPlaywright() {
  for (const base of [import.meta.url, '/opt/node-tools/', process.env.NODE_PATH ? process.env.NODE_PATH + '/' : null]) {
    if (!base) continue;
    try {
      const req = createRequire(base.startsWith('file:') || base.startsWith('/') ? (base.startsWith('/') ? base + 'x.js' : base) : base);
      return req('playwright-core');
    } catch (e) { /* try the next place */ }
  }
  return null;
}
function findChromium(pw) {
  if (process.env.PW_CHROMIUM && existsSync(process.env.PW_CHROMIUM)) return process.env.PW_CHROMIUM;
  try {
    for (const d of readdirSync('/opt/pw-browsers')) {
      const c = join('/opt/pw-browsers', d, 'chrome-linux', 'chrome');
      if (d.startsWith('chromium-') && existsSync(c)) return c;
    }
  } catch (e) { /* none */ }
  try {
    const c = pw && pw.chromium.executablePath();
    if (c && existsSync(c)) return c;
  } catch (e) { /* none */ }
  return null;
}

export const pw = findPlaywright();
export const chromePath = findChromium(pw);
const missing = !pw || !chromePath ? 'playwright-core or Chromium not found' : false;
if (missing && process.env.CI) throw new Error(missing + ': CI must run the browser tests, not skip them');
export const skip = missing;
