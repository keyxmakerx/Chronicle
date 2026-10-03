// customize_look_pictures.test.mjs — tidyPictures() deletes any picture
// uploaded on the Customize page that pictureNames() does not list, so every
// picture slot that can be uploaded must be listed, or a fresh upload is
// deleted straight away (the menu banner once was).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'customize_look.js'), 'utf8');

function slice(start, end) {
  const a = src.indexOf(start);
  const b = src.indexOf(end, a);
  assert.ok(a > 0 && b > a, `${start} not found`);
  return src.slice(a, b + end.length);
}

test('every uploadable picture slot is kept by pictureNames', () => {
  const code = slice('function pictureNames(d)', '}') + slice('var UPLOAD_KIND = {', '};') +
    'var d = { brand: {}, header: {}, sidebar: {} };' +
    'Object.keys(UPLOAD_KIND).forEach(function (k) { var p = k.split("."); d[p[0]][p[1]] = k; });' +
    'JSON.stringify({ kept: pictureNames(d), slots: Object.keys(UPLOAD_KIND) })';
  const { kept, slots } = JSON.parse(vm.runInNewContext(code));
  for (const slot of slots) assert.ok(kept.includes(slot), `${slot} is uploadable but not kept`);
});
