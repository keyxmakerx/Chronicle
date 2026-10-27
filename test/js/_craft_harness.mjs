// _craft_harness.mjs — a page with the crafting bench widget on it, for the
// craft_bench browser test and tools/craft-thumbs.mjs. Serves the repo's own
// /static files to a real Chromium and fakes everything else.

import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath } from './_browser_env.mjs';

export const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

const SKY = ['sky_world', 'sky_looks', 'sky_moon', 'sky_events', 'sky_2d', 'sky_shaders', 'sky_gl', 'sky_pane'];
const CRAFT = ['craft_scene', 'craft_desk', 'craft_bench_editor', 'craft_bench'];
const TYPES = { '.css': 'text/css', '.js': 'application/javascript', '.jpg': 'image/jpeg', '.png': 'image/png', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' };

// mounts: the craft_bench elements' attributes, one string per mount.
// paintedSky lets the window paint its sky on a software GPU, which the tray
// pictures want and the tests don't: it costs a CPU-bound runner seconds a frame.
export function html(mounts, width = 1200, paintedSky = false) {
  const scripts = SKY.concat(CRAFT).map((f) => `<script src="/static/js/widgets/${f}.js"></script>`).join('\n');
  return `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/static/css/app.css">
<style>html,body{margin:0;background:#111}main{max-width:${width}px}</style></head><body><main>
${mounts.map((a) => `<div data-widget="craft_bench" ${a}></div>`).join('\n')}
</main>
<script>window.htmx = { on: function () {}, process: function () {}, onLoad: function () {}, config: {} }; window.SkyGLAllowSoftware = ${paintedSky};</script>
<script src="/static/js/boot.js"></script>
${scripts}
</body></html>`;
}

export async function open(opts = {}) {
  const browser = await pw.chromium.launch({ executablePath: chromePath, args: ['--no-sandbox'] });
  // A browser left open keeps the test process alive after a failure.
  try {
    return await load(browser, opts);
  } catch (e) {
    await browser.close();
    throw e;
  }
}

async function load(browser, { mounts = ['data-mode="play"'], width = 1200, height = 900, scale = 1, paintedSky = false }) {
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: scale });
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.route('**/*', (route) => {
    const u = new URL(route.request().url());
    if (u.pathname === '/') return route.fulfill({ contentType: 'text/html', body: html(mounts, width, paintedSky) });
    if (u.pathname.startsWith('/static/')) {
      const file = join(root, u.pathname);
      if (!existsSync(file)) return route.fulfill({ status: 404, body: '' });
      const ext = u.pathname.slice(u.pathname.lastIndexOf('.'));
      return route.fulfill({ contentType: TYPES[ext] || 'application/octet-stream', body: readFileSync(file) });
    }
    return route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
  });
  // A busy runner, several browsers at once on a software GPU, can take far
  // longer than Playwright's defaults; each caller then waits for what it needs.
  page.setDefaultTimeout(60000);
  await page.goto('http://bench.test/', { waitUntil: 'domcontentloaded', timeout: 90000 });
  return { browser, page, errors };
}

// Resolves once every baked layer of the scene in mount n is a picture.
export async function baked(page, n = 0, timeout = 90000) {
  await page.waitForFunction((n) => {
    const svg = document.querySelectorAll('[data-widget="craft_bench"] .cr-svg')[n];
    if (!svg) return false;
    const gs = svg.querySelectorAll('[data-bake]');
    return gs.length > 0 && Array.prototype.every.call(gs, (g) => g.children.length === 1 && g.firstElementChild.tagName === 'image');
  }, n, { timeout });
}
