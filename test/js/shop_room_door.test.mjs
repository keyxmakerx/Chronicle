// shop_room_door.test.mjs — what a press outside the open shop room does
// (static/js/widgets/shop_room.js): it closes, unless the basket has items.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'static', 'js', 'widgets');
const sandbox = {};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(readFileSync(join(dir, 'shop_room_icons.js'), 'utf8'), sandbox);
vm.runInContext(readFileSync(join(dir, 'shop_room.js'), 'utf8'), sandbox);
const SR = sandbox.ShopRoomDoor;

test('a press outside the open shop closes it unless the basket has items', () => {
  assert.equal(SR.outsideClose({}), 'close');
  assert.equal(SR.outsideClose({ a: 0 }), 'close');
  assert.equal(SR.outsideClose({ a: 0, b: 2 }), 'warn');
});
