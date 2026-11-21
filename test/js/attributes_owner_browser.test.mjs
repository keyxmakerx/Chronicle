// attributes_owner_browser.test.mjs — the claimed owner's Attributes box: the
// GET lists owner_fields, the form shows only those, and Done sends a patch of
// just what changed. A Scribe's box still sends the whole map.
//
// Runs boot.js and attributes.js in a real Chromium against a stubbed server.
// Needs playwright-core and a Chromium (see _browser_env.mjs); skips locally,
// and fails under CI, when either is missing.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { pw, chromePath, skip } from './_browser_env.mjs';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const EP = '/campaigns/c1/entities/e1/fields';

function page(editable) {
  return `<!doctype html><html data-gm-term="GM"><head>
<script src="/htmx.js"></script><script src="/boot.js"></script><script src="/attributes.js"></script></head><body>
<div data-widget="attributes" data-endpoint="${EP}" data-campaign-id="c1"${editable ? ' data-editable="true"' : ''}></div>
</body></html>`;
}

async function open(editable, ownerFields, extra = {}, putReply = '{}', putStatus = 200) {
  const browser = await pw.chromium.launch({ executablePath: chromePath });
  const pg = await browser.newPage();
  const puts = [];
  await pg.route('http://t.test/**', (route) => {
    const req = route.request();
    const p = new URL(req.url()).pathname;
    if (p === '/htmx.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/vendor/htmx.min.js')) });
    if (p === '/boot.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/boot.js')) });
    if (p === '/attributes.js') return route.fulfill({ contentType: 'text/javascript', body: readFileSync(join(root, 'static/js/widgets/attributes.js')) });
    if (p === EP && req.method() === 'PUT') {
      puts.push(JSON.parse(req.postData()));
      return route.fulfill({ status: putStatus, contentType: 'application/json', body: putReply });
    }
    if (p === EP) {
      const fields = [
        { key: 'race', label: 'Race', type: 'text' },
        { key: 'background', label: 'Background', type: 'text' },
        { key: 'str', label: 'STR', type: 'number' },
      ];
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({
        fields, type_fields: fields, fields_data: { race: 'Elf', background: 'Sage', str: 14 },
        field_overrides: {}, shown: {}, owner_fields: ownerFields, ...extra,
      }) });
    }
    if (p.includes('/choices') || p.includes('/systems/')) return route.fulfill({ status: 404, body: '' });
    return route.fulfill({ contentType: 'text/html', body: page(editable) });
  });
  await pg.goto('http://t.test/');
  await pg.waitForSelector('[data-widget="attributes"] .card');
  return { browser, pg, puts };
}

async function edit(pg) {
  await pg.getByRole('button', { name: 'Edit' }).click();
  await pg.waitForSelector('[data-field-key]');
}

test('a claimed owner edits only their fields and sends a patch of the change', { skip }, async () => {
  const { browser, pg, puts } = await open(false, ['race', 'background']);
  try {
    assert.equal(await pg.locator('button[title="Customize fields for this page"]').count(), 0, 'no gear for the owner');
    await edit(pg);
    const keys = await pg.$$eval('[data-field-key]', (els) => els.map((e) => e.getAttribute('data-field-key')));
    assert.deepEqual(keys.sort(), ['background', 'race']);
    assert.match(await pg.textContent('[data-widget="attributes"]'), /The rest of your hero is your GM/);
    await pg.fill('[data-field-key="race"]', 'Dwarf');
    await pg.fill('[data-field-key="background"]', '');
    await pg.getByRole('button', { name: 'Done' }).click();
    await pg.waitForFunction(() => !document.querySelector('[data-field-key]'));
    assert.deepEqual(puts, [{ fields_patch: { race: 'Dwarf', background: null } }]);
  } finally {
    await browser.close();
  }
});

test('a page with no owner fields has no Edit for a player', { skip }, async () => {
  const { browser, pg } = await open(false, undefined);
  try {
    assert.equal(await pg.getByRole('button', { name: 'Edit' }).count(), 0);
  } finally {
    await browser.close();
  }
});

test('a Scribe still saves the whole map', { skip }, async () => {
  const { browser, pg, puts } = await open(true, undefined);
  try {
    await edit(pg);
    assert.equal(await pg.locator('[data-field-key]').count(), 3);
    await pg.fill('[data-field-key="race"]', 'Gnome');
    await pg.getByRole('button', { name: 'Done' }).click();
    await pg.waitForFunction(() => !document.querySelector('[data-field-key]'));
    assert.equal(puts.length, 1);
    assert.deepEqual(Object.keys(puts[0]), ['fields_data']);
    assert.equal(puts[0].fields_data.race, 'Gnome');
  } finally {
    await browser.close();
  }
});

test('a hero that holds a pick the rules now exclude says so, and a waiting pick is named', { skip }, async () => {
  const { browser, pg } = await open(false, ['race'], {
    choice_breaks: [{ fieldKey: 'race', value: 'Fury', kind: 'not_offered', reason: 'No Furies this time' }],
    choice_waiting: [{ id: 3, fieldKey: 'background', entryName: 'Noble' }],
  });
  try {
    const text = await pg.textContent('.ag-rules');
    assert.match(text, /Fury isn.t offered any more\. You keep it; a new pick must be on the list\./);
    assert.match(text, /No Furies this time/);
    assert.match(text, /Noble is waiting for your Director, for background\./);
  } finally {
    await browser.close();
  }
});

test('a refused save shows the server sentence and keeps the form open', { skip }, async () => {
  const { browser, pg } = await open(false, ['race'], {}, JSON.stringify({ message: '"Fury" is not on the race list' }), 400);
  try {
    await edit(pg);
    await pg.fill('[data-field-key="race"]', 'Fury');
    await pg.getByRole('button', { name: 'Done' }).click();
    await pg.waitForFunction(() => /not on the race list/.test(document.body.textContent));
  } finally {
    await browser.close();
  }
});

test('a url field links only to a web address; a script address stays words', { skip }, async () => {
  const fields = [
    { key: 'site', label: 'Site', type: 'url' },
    { key: 'trap', label: 'Trap', type: 'url' },
  ];
  const { browser, pg } = await open(false, null, {
    fields, type_fields: fields,
    fields_data: { site: 'https://example.test/a', trap: 'javascript:window.hit=1' },
  });
  try {
    const links = await pg.$$eval('[data-widget="attributes"] a', (as) => as.map((a) => a.getAttribute('href')));
    assert.deepEqual(links, ['https://example.test/a']);
    assert.match(await pg.textContent('[data-widget="attributes"]'), /javascript:window\.hit=1/);
  } finally {
    await browser.close();
  }
});
