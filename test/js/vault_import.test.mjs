// vault_import.test.mjs -- pins the pure helpers of the Manage > Import drop
// zone: what it refuses before spending an upload, and the progress wording.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/vault_import.js'), 'utf8');

function load() {
  const registered = {};
  const window = { Chronicle: { register: (name, def) => { registered[name] = def; } } };
  vm.runInNewContext(src, { window, Chronicle: window.Chronicle });
  return { V: window.Chronicle.VaultImport, registered };
}

test('only .zip names are accepted, in any case', () => {
  const { V } = load();
  for (const [name, want] of [['a.zip', true], ['Vault.ZIP', true], ['a.zip.exe', false], ['a.md', false], ['zip', false], ['', false], [null, false]]) {
    assert.equal(V.isZipName(name), want, String(name));
  }
});

test('checkFile explains each refusal and accepts a good zip', () => {
  const { V } = load();
  assert.match(V.checkFile(null, 100), /Choose a \.zip/);
  assert.match(V.checkFile({ name: 'v.tar', size: 1 }, 100), /isn't a \.zip/);
  assert.match(V.checkFile({ name: 'v.zip', size: 300 * 1048576 }, 200 * 1048576), /over 200 MB/);
  assert.equal(V.checkFile({ name: 'v.zip', size: 5 }, 200 * 1048576), '');
  // No limit configured: the server still enforces its own.
  assert.equal(V.checkFile({ name: 'v.zip', size: 5e9 }, 0), '');
});

test('uploadPercent stays between 0 and 100 and survives bad input', () => {
  const { V } = load();
  for (const [loaded, total, want] of [[0, 100, 0], [50, 100, 50], [1, 3, 33], [200, 100, 100], [-5, 100, 0], [5, 0, 0], ['x', 100, 0], [5, NaN, 0]]) {
    assert.equal(V.uploadPercent(loaded, total), want, `${loaded}/${total}`);
  }
});

test('the label says the zip is being read once every byte is sent', () => {
  const { V } = load();
  assert.equal(V.uploadLabel(40), 'Uploading…');
  assert.equal(V.uploadLabel(100), 'Reading the zip…');
});

test('formatBytes uses the units the site uses', () => {
  const { V } = load();
  assert.equal(V.formatBytes(512), '512 B');
  assert.equal(V.formatBytes(2048), '2.0 KB');
  assert.equal(V.formatBytes(5 * 1048576), '5.0 MB');
  assert.equal(V.formatBytes(-1), '');
  assert.equal(V.formatBytes('x'), '');
});

test('the widget registers under the name the drop zone mounts', () => {
  const { registered } = load();
  assert.equal(typeof registered['vault-import'].init, 'function');
  assert.equal(typeof registered['vault-import'].destroy, 'function');
});
