// notice_board_frame.spec.mjs: the wide notice board's cork stays inside its
// wooden frame however narrow the column it sits in.
//
// The wide cork has an aspect ratio and a minimum height. Without a definite
// width, the browser carries the minimum height through the ratio into a
// minimum width, so in a column narrower than about 620px the cork ran past
// the frame's right edge. This loads the real static/css/quest_board.css
// and markup shaped like quest_board_kit.js's boardFrame('wide multi', ...),
// then measures both boxes at several widths, empty and with notices pinned.
//
// RUN: NODE_PATH="$(npm root -g)" node --test test/e2e/notice_board_frame.spec.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const here = dirname(fileURLToPath(import.meta.url));
const css = readFileSync(join(here, '..', '..', 'static', 'css', 'quest_board.css'), 'utf8');

function note(i) {
  return '<div class="note" style="left:' + (3 + i * 24.5) + '%;top:5%;width:22%;--r:0deg"><span class="pin"></span>' +
    '<div class="pp"><div class="k">Notice</div><div class="t">Quest ' + i + '</div></div></div>';
}

function page(width, inner) {
  return '<!doctype html><html><head><style>' + css + '</style><style>body{margin:0}.col{margin:20px;width:' + width + 'px}</style></head><body>' +
    '<div class="col"><div class="qbk" data-bl="lit"><div class="bw wide"><div class="board wide multi"><span class="bplate">Notices</span>' +
    '<div class="cork"><div class="face">' + inner + '</div></div></div></div></div></div></body></html>';
}

test('wide notice board cork stays inside its frame', async () => {
  const browser = await chromium.launch();
  try {
    const p = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    const fills = { empty: '<div class="empty">No boards yet.</div>', pinned: [0, 1, 2, 3].map(note).join('') };
    for (const [label, inner] of Object.entries(fills)) {
      for (const width of [300, 370, 500, 620, 700, 900]) {
        await p.setContent(page(width, inner));
        const r = await p.evaluate(() => {
          const b = document.querySelector('.board').getBoundingClientRect();
          const c = document.querySelector('.cork').getBoundingClientRect();
          return { bl: b.left, br: b.right, bb: b.bottom, cl: c.left, cr: c.right, cb: c.bottom };
        });
        assert.ok(r.cl >= r.bl && r.cr <= r.br, `${label} at ${width}px: cork ${r.cl}-${r.cr} runs past frame ${r.bl}-${r.br}`);
        assert.ok(r.cb <= r.bb, `${label} at ${width}px: cork bottom ${r.cb} runs past frame bottom ${r.bb}`);
      }
    }
  } finally {
    await browser.close();
  }
});
