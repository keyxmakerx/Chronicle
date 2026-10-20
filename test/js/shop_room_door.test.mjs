// shop_room_door.test.mjs — how the open shop room closes
// (static/js/widgets/shop_room.js): only through its own Leave control, so
// using the inventory list or anything else on the page keeps it open.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const file = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static', 'js', 'widgets', 'shop_room.js');
const src = readFileSync(file, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');

test('the open shop has a Leave control in its top bar', () => {
  assert.match(src, /class="shr-out" data-out="1" aria-label="Leave the shop"/);
});

test('only the Leave control closes the shop', () => {
  const calls = [...src.matchAll(/(?<![\w.]|function )leave\(\)/g)];
  assert.equal(calls.length, 1, 'leave() must have exactly one caller');
  const line = src.slice(src.lastIndexOf('\n', calls[0].index), src.indexOf('\n', calls[0].index));
  assert.match(line, /b\.dataset\.out/);
});

test('no press outside the room closes the shop', () => {
  assert.doesNotMatch(src, /onOutsideShop|outsideClose/);
});
