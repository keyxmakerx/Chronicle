// live-pictures.mjs — gathers the pictures the live checks took into one
// page for a person to look at, and decides whether any need approval.
//
// Each picture is "same" (matches the last run on main), "changed" (differs)
// or "new" (main has none yet). The report is one self-contained HTML file,
// pictures.html in LIVE_OUT, showing main's picture, this run's, and the
// difference painted red, side by side. In CI it is uploaded as the
// live-pictures-report artifact and listed in the job summary.
//
// Exit code: 1 when a picture changed and LIVE_PICTURES_APPROVED is not "1"
// (CI sets it when the pull request carries the pictures-approved label).
// New pictures never fail: there is nothing to compare them with.
//
// RUN: node tools/live-pictures.mjs   (tools/live-checks.sh runs it)

import { readdirSync, readFileSync, writeFileSync, existsSync, appendFileSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const OUT = process.env.LIVE_OUT || join(repoRoot, 'test-results', 'live');
const BASELINE = process.env.LIVE_BASELINE || '';
const APPROVED = process.env.LIVE_PICTURES_APPROVED === '1';
const dir = join(OUT, 'pictures');

const records = existsSync(dir)
  ? readdirSync(dir).filter((f) => f.endsWith('.json')).map((f) => JSON.parse(readFileSync(join(dir, f), 'utf8')))
  : [];
const order = { changed: 0, new: 1, same: 2 };
records.sort((a, b) => order[a.status] - order[b.status] || a.name.localeCompare(b.name));

const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
const img = (path) => existsSync(path) ? `<img src="data:image/png;base64,${readFileSync(path).toString('base64')}" alt="">` : '<p class="none">none</p>';
const count = (s) => records.filter((r) => r.status === s).length;
const changed = count('changed');

const cards = records.map((r) => `
<section class="card ${r.status}">
  <h2>${esc(r.name)} <span class="tag">${r.status === 'changed' ? `changed (${(r.diff * 100).toFixed(2)}% of the picture)` : r.status}</span></h2>
  <p>${esc(r.about)}${r.check ? ` · stands in for ${esc(r.check)}` : ''}</p>
  ${r.status === 'same' ? '' : `<div class="row">
    <figure><figcaption>On main</figcaption>${img(BASELINE ? join(BASELINE, r.name + '.png') : '')}</figure>
    <figure><figcaption>This change</figcaption>${img(join(dir, r.name + '.png'))}</figure>
    ${r.status === 'changed' ? `<figure><figcaption>What moved, in red</figcaption>${img(join(dir, r.name + '.diff.png'))}</figure>` : ''}
  </div>`}
</section>`).join('\n');

writeFileSync(join(OUT, 'pictures.html'), `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Live check pictures</title>
<style>
  :root { --bg: #f7f6f3; --fg: #1d1c1a; --muted: #6b6760; --line: #dcd8d0; --warn: #b4232c; --new: #2f5fb3; }
  @media (prefers-color-scheme: dark) { :root { --bg: #17181a; --fg: #ecebe8; --muted: #a19d95; --line: #34363a; --warn: #ff6b72; --new: #8fb4ff; } }
  body { background: var(--bg); color: var(--fg); font: 15px/1.5 system-ui, sans-serif; margin: 0; padding: 24px 16px; }
  main { max-width: 1400px; margin: 0 auto; }
  h1 { font-size: 1.4rem; margin: 0 0 4px; } h2 { font-size: 1rem; margin: 0; }
  .lead { color: var(--muted); margin: 0 0 24px; }
  .card { border-top: 1px solid var(--line); padding: 16px 0; }
  .card p { color: var(--muted); margin: 2px 0 10px; }
  .tag { font-size: .75rem; letter-spacing: .06em; text-transform: uppercase; color: var(--muted); margin-left: 8px; }
  .changed .tag { color: var(--warn); } .new .tag { color: var(--new); }
  .row { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
  figure { margin: 0; } figcaption { font-size: .8rem; color: var(--muted); margin-bottom: 4px; }
  img { width: 100%; height: auto; border: 1px solid var(--line); display: block; }
  .none { color: var(--muted); font-style: italic; }
</style></head><body><main>
<h1>Live check pictures</h1>
<p class="lead">${changed} changed, ${count('new')} new, ${count('same')} the same as on main.
${changed ? (APPROVED ? 'The pull request is labelled pictures-approved, so these changes are accepted.' : 'If every change below is intended, add the <b>pictures-approved</b> label to the pull request.') : ''}</p>
${cards || '<p class="lead">No pictures were taken.</p>'}
</main></body></html>
`);

const summary = [
  `### Live check pictures`,
  `${changed} changed, ${count('new')} new, ${count('same')} unchanged. Open the **live-pictures-report** artifact to see them side by side.`,
  ...records.filter((r) => r.status !== 'same').map((r) => `- **${r.name}** ${r.status}${r.status === 'changed' ? ` (${(r.diff * 100).toFixed(2)}%)` : ''}: ${r.about}`),
  changed && !APPROVED ? `\nIf the changes are intended, add the \`pictures-approved\` label to the pull request.` : '',
].join('\n');
console.log(summary);
if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, summary + '\n');

process.exitCode = changed && !APPROVED ? 1 : 0;
