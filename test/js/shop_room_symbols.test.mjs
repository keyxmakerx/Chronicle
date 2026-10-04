// shop_room_symbols.test.mjs — every icon the shop room draws (goods and
// decorations, static/js/widgets/shop_room.js) points at a symbol the same
// drawing defines; a dangling <use> renders as nothing.

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
const SR = sandbox.ShopRoom;

test('every icon <use> in the room has its symbol defined', () => {
  for (const roomType of SR.ROOM_TYPES.map((t) => t[0])) {
    for (const setting of ['room', 'tower', 'tree', 'burrow', 'cave']) {
      const S = { roomType, pal: 'oak', setting, fx: 'full', size: 'm', full: 'normal', deco: 'lots', keep: true,
        seeds: { room: 1, goods: 1, deco: 1 }, pieces: [], ov: {}, portrait: null, lines: [], mode: 'shop', key: 7, name: 'Shop', dark: false,
        its: [{ id: 'r1', n: 'Thing', ic: 'gem', c: '#d4a72c', price: '1 gp' }] };
      const room = SR.createRoom(S);
      room.generate();
      const svg = room.draw().svg;
      const used = new Set([...svg.matchAll(/href="#(i-[a-z0-9-]+)"/g)].map((m) => m[1]));
      const defined = new Set([...svg.matchAll(/<symbol id="(i-[a-z0-9-]+)"/g)].map((m) => m[1]));
      assert.ok(used.size > 1, `${roomType}/${setting} draws icons`);
      for (const u of used) assert.ok(defined.has(u), `${roomType}/${setting}: ${u} has no symbol`);
    }
  }
});
