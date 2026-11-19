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

async function open(editable, ownerFields) {
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
      return route.fulfill({ contentType: 'application/json', body: '{}' });
    }
    if (p === EP) {
      const fields = [
        { key: 'race', label: 'Race', type: 'text' },
        { key: 'background', label: 'Background', type: 'text' },
        { key: 'str', label: 'STR', type: 'number' },
      ];
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({
        fields, type_fields: fields, fields_data: { race: 'Elf', background: 'Sage', str: 14 },
        field_overrides: {}, shown: {}, owner_fields: ownerFields,
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
