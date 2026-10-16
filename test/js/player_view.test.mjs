// player_view.test.mjs — pins player_view.js: the page after the switch
// plays only a fresh mark for this page that matches the view the server
// rendered, and the motion level is read the way the rest of the site
// reads it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import vm from 'node:vm';

const here = path.dirname(fileURLToPath(import.meta.url));
const src = readFileSync(path.join(here, '..', '..', 'static', 'js', 'widgets', 'player_view.js'), 'utf8');

function attrs(map) { return { getAttribute: (k) => (k in map ? map[k] : null), hasAttribute: (k) => k in map, removeAttribute: (k) => { delete map[k]; } }; }

// Outside the Annals look the script only exposes its helpers.
const listeners = [];
const sandbox = {
  document: { documentElement: attrs({}), addEventListener: (t) => listeners.push(t) },
};
sandbox.window = sandbox;
vm.runInNewContext(src, sandbox);
const { arrival, motionOf } = sandbox.Chronicle._playerView;

test('the Standard look keeps the plain switch', () => {
  assert.deepEqual(listeners, []);
});

const now = 1_000_000;
const mark = (o) => JSON.stringify({ to: 'player', path: '/campaigns/c/entities/e', t: now - 500, ...o });

test('arrival plays a fresh mark for this page and this view', () => {
  const cases = [
    ['to player, now viewing as player', mark(), true, 'player'],
    ['to director, now the Director', mark({ to: 'director' }), false, 'director'],
    ['to player, but the server rendered the Director', mark(), false, ''],
    ['to director, but still viewing as player', mark({ to: 'director' }), true, ''],
    ['another page', mark({ path: '/campaigns/c/entities/x' }), true, ''],
    ['too old', mark({ t: now - 16000 }), true, ''],
    ['from the future', mark({ t: now + 5000 }), true, ''],
    ['unknown direction', mark({ to: 'owner' }), true, ''],
    ['no time', mark({ t: 'soon' }), true, ''],
    ['nothing stored', null, true, ''],
    ['not JSON', '{oops', true, ''],
  ];
  for (const [name, raw, viewing, want] of cases) {
    assert.equal(arrival(raw, now, '/campaigns/c/entities/e', viewing), want, name);
  }
});

test('motion follows html[data-motion] and the device setting', () => {
  assert.equal(motionOf(attrs({}), false), 'full');
  assert.equal(motionOf(attrs({ 'data-motion': 'calm' }), false), 'calm');
  assert.equal(motionOf(attrs({ 'data-motion': 'off' }), false), 'off');
  assert.equal(motionOf(attrs({ 'data-motion': 'wild' }), false), 'full');
  assert.equal(motionOf(attrs({ 'data-motion': 'calm' }), true), 'off');
});
