// media_uploader_load.test.mjs — the Media page's upload zone uses
// x-data="mediaUploader(...)". htmx strips <script> tags from a boosted swap,
// so the component must come from the layout, and before Alpine, or Alpine
// throws "mediaUploader is not defined" when the page is reached from the
// sidebar or the Manage tabs.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const read = (p) => readFileSync(join(root, p), 'utf8');

test('the layout loads media_uploader.js before Alpine', () => {
  const base = read('internal/templates/layouts/base.templ');
  const uploader = base.indexOf('/static/js/media_uploader.js');
  const alpine = base.indexOf('/static/vendor/alpine.min.js');
  assert.ok(uploader > 0, 'base.templ does not load media_uploader.js');
  assert.ok(uploader < alpine, 'media_uploader.js must load before alpine.min.js');
});

test('the Media page carries no inline uploader script', () => {
  assert.doesNotMatch(read('internal/plugins/media/media_browser.templ'), /<script\b/i);
});

test('mediaUploader returns the state the template binds', () => {
  const sandbox = {};
  vm.runInNewContext(read('static/js/media_uploader.js'), sandbox);
  const c = sandbox.mediaUploader('c1', 'tok');
  assert.equal(c.dragOver, false);
  assert.deepEqual(Array.from(c.queue), []);
  assert.equal(typeof c.handleDrop, 'function');
  assert.equal(typeof c.handleFiles, 'function');
});
