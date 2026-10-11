// editor_image.test.mjs — pins how a picture inside editor text is saved.
// The saved shape must match what the Go sanitizer keeps and what
// sanitize.StripSecretsHTML drops for players (internal/sanitize/picture_test.go):
// a figure whose classes carry width, side and GM-only, and a plain
// /media/<id> src (never a signed, expiring link, never another website).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const src = readFileSync(join(root, 'static/js/widgets/editor_image.js'), 'utf8');
const ID = '0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70';

function load() {
  const added = [];
  const window = {
    TipTap: { Node: { create: (spec) => ({ spec }) } },
    Chronicle: { SlashCommands: { addCommand: (c) => added.push(c) } },
  };
  vm.runInNewContext(src, { window, Chronicle: window.Chronicle, TipTap: window.TipTap });
  return { E: window.Chronicle.EditorImage, added };
}

test('widths snap to 5% steps between 10% and 100%', () => {
  const { E } = load();
  for (const [inp, want] of [[42, 40], [43, 45], [3, 10], [140, 100], ['x', 100]]) {
    assert.equal(E._clampWidth(inp), want, `clampWidth(${inp})`);
  }
});

test('only this site\'s own media path is accepted as a picture', () => {
  const { E } = load();
  assert.equal(E._mediaIdFromSrc('/media/' + ID), ID);
  assert.equal(E._mediaIdFromSrc('/media/' + ID + '?sig=abc&exp=1'), ID, 'a signed link reads back to the plain id');
  for (const bad of ['https://evil.example/x.png', '/media/../etc', '/media/not-a-uuid', 'javascript:alert(1)', '']) {
    assert.equal(E._mediaIdFromSrc(bad), null, bad);
  }
});

test('saved HTML is a figure with classes and a plain /media src', () => {
  const { E } = load();
  const spec = E._renderSpec({ mediaId: ID, alt: 'Mira', caption: 'Mira Kell', width: 40, align: 'right', gmOnly: true });
  assert.deepEqual(JSON.parse(JSON.stringify(spec)), [
    'figure', { class: 'ce-img ce-img--w40 ce-img--right ce-img--gm' },
    ['img', { src: '/media/' + ID, alt: 'Mira' }],
    ['figcaption', {}, 'Mira Kell'],
  ]);
  const plain = E._renderSpec({ mediaId: ID, alt: '', caption: '', width: 100, align: 'bogus', gmOnly: false });
  assert.equal(plain[1].class, 'ce-img ce-img--w100 ce-img--center');
  assert.equal(plain.length, 3, 'no empty figcaption');
});

test('the CSS defines every width class the script can produce', () => {
  const css = readFileSync(join(root, 'static/css/input.css'), 'utf8');
  for (let w = 10; w <= 100; w += 5) {
    assert.match(css, new RegExp(`\\.ce-img--w${w} \\{ width: ${w}%; \\}`), `missing .ce-img--w${w}`);
  }
});

test('adds a Picture command to the slash menu and is wired into the page editor', () => {
  const { added } = load();
  assert.equal(added.length, 1);
  assert.equal(added[0].id, 'picture');
  const editorSrc = readFileSync(join(root, 'static/js/widgets/editor.js'), 'utf8');
  assert.match(editorSrc, /extensions\.push\(Chronicle\.EditorImage\.extension\)/);
  const base = readFileSync(join(root, 'internal/templates/layouts/base.templ'), 'utf8');
  assert.ok(base.indexOf('editor_slash.js') < base.indexOf('editor_image.js') &&
            base.indexOf('editor_image.js') < base.indexOf('widgets/editor.js'),
            'editor_image.js loads after editor_slash.js and before editor.js');
});

// --- Pictures in notes -------------------------------------------------------
// Players write notes, and /media/upload needs Scribe, so notes send pictures
// to their own route; the server keeps the file private to the note's readers.

function loadWithApi(fetchLog, reply) {
  const added = [];
  const window = {
    TipTap: { Node: { create: (spec) => ({ spec }) } },
    Chronicle: {
      SlashCommands: { addCommand: (c) => added.push(c) },
      apiFetch: (url, opts) => {
        fetchLog.push({ url, opts });
        return Promise.resolve({ ok: true, json: () => Promise.resolve(reply) });
      },
    },
  };
  vm.runInNewContext(src, { window, Chronicle: window.Chronicle, TipTap: window.TipTap, FormData, Promise });
  return window.Chronicle.EditorImage;
}

function fakeEditor(inserted) {
  const chain = { focus: () => chain, insertContent: (c) => { inserted.push(c); return chain; }, insertContentAt: (p, c) => { inserted.push(c); return chain; }, run: () => true };
  return { chain: () => chain };
}

test('a note editor uploads to the notes picture route, not the media library', async () => {
  const log = [];
  const E = loadWithApi(log, { id: ID });
  const inserted = [];
  const ed = fakeEditor(inserted);
  E.useNotePictures(ed, 'camp-1');
  const n = await E.uploadAndInsert(ed, 'camp-1', [{ name: 'map.png', type: 'image/png' }]);
  assert.equal(n, 1);
  assert.equal(log.length, 1);
  assert.equal(log[0].url, '/campaigns/camp-1/notes/pictures');
  assert.equal(log[0].opts.method, 'POST');
  assert.equal(log[0].opts.body.get('campaign_id'), null, 'the campaign is in the path, never a form field a client could change');
  assert.equal(log[0].opts.body.get('usage_type'), null, 'the server chooses how a note picture is stored');
  assert.equal(inserted[0].attrs.mediaId, ID, 'the node keeps the plain id, so the saved src is /media/<id>');
});

test('a page editor still uploads to the media library', async () => {
  const log = [];
  const E = loadWithApi(log, { id: ID });
  await E.uploadAndInsert(fakeEditor([]), 'camp-1', [{ name: 'x.png', type: 'image/png' }]);
  assert.equal(log[0].url, '/media/upload');
  assert.equal(log[0].opts.body.get('usage_type'), 'attachment');
});

test('pasted and dropped picture files are taken only while the note is editable', () => {
  const E = loadWithApi([], { id: ID });
  const ed = fakeEditor([]);
  E.useNotePictures(ed, 'camp-1');
  const props = E.pasteDropProps(() => ed, 'camp-1');
  const file = { name: 'a.png', type: 'image/png' };
  const clip = { clipboardData: { files: [file] } };
  assert.equal(props.handlePaste({ editable: false }, clip), false, 'read-only: left to the browser');
  assert.equal(props.handlePaste({ editable: true }, { clipboardData: { files: [{ type: 'text/plain' }] } }), false, 'not a picture');
  assert.equal(props.handlePaste({ editable: true }, clip), true);
  let prevented = false;
  const drop = { dataTransfer: { files: [file] }, clientX: 1, clientY: 2, preventDefault: () => { prevented = true; } };
  assert.equal(props.handleDrop({ editable: true, posAtCoords: () => ({ pos: 3 }) }, drop, null, true), false, 'moving existing content is not an upload');
  assert.equal(props.handleDrop({ editable: true, posAtCoords: () => ({ pos: 3 }) }, drop, null, false), true);
  assert.ok(prevented);
});

test('the Journal and the jot notes editors carry the picture node and the notes upload', () => {
  for (const f of ['static/js/widgets/journal.js', 'static/js/widgets/notes.js']) {
    const js = readFileSync(join(root, f), 'utf8');
    assert.match(js, /Chronicle\.EditorImage\.extension/, `${f} puts the picture node in its schema`);
    assert.match(js, /EditorImage\.useNotePictures\(/, `${f} sends pictures to the notes route`);
    assert.match(js, /EditorImage\.pasteDropProps\(/, `${f} takes pasted and dropped pictures`);
    assert.match(js, /EditorImage\.pickAndInsert\(/, `${f} has a way to choose a picture`);
  }
  const templ = readFileSync(join(root, 'internal/widgets/notes/journal.templ'), 'utf8');
  assert.match(templ, /data-cmd="picture"/);
});

test('the picture tools show in any editable editor, not only the page editor', () => {
  const css = readFileSync(join(root, 'static/css/input.css'), 'utf8');
  assert.match(css, /\.ProseMirror\[contenteditable="true"\]\) \.ce-img\.is-selected \.ce-img__handle/);
});
