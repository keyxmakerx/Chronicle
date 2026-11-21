// On a touch screen the Edit drawer's buttons and the chosen days' actions are
// at least 44 tall (.ai/ui-standard.md, Phones and tablets); the mouse sizes
// stay. Read from the stylesheet since the rule is a media query.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const css = readFileSync(new URL('../../static/css/calendar-view.css', import.meta.url), 'utf8');

// coarseBlocks returns the body of every (pointer:coarse) media block.
function coarseBlocks(src) {
  const out = [];
  const re = /@media \(pointer:coarse\)\{/g;
  let m;
  while ((m = re.exec(src))) {
    let depth = 1, i = re.lastIndex;
    while (depth && i < src.length) { const c = src[i++]; if (c === '{') depth++; else if (c === '}') depth--; }
    out.push(src.slice(re.lastIndex, i - 1));
  }
  return out;
}

test('touch raises the Edit drawer buttons and the chosen days actions to 44', () => {
  const body = coarseBlocks(css).join('\n');
  for (const sel of ['.cal-v5 .dkr .btn', '.cal-v5 .bbb']) {
    const re = new RegExp('(^|[,\\n}])\\s*' + sel.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*(,[^{]*)?\\{[^}]*min-height:44px');
    assert.match(body, re, sel + ' needs min-height:44px on touch');
  }
});

test('the mouse sizes are unchanged', () => {
  assert.match(css, /\.cal-v5 \.btn\.sm\{height:26px;/);
  assert.match(css, /\.cal-v5 \.bbb\{[^}]*height:34px/);
});
