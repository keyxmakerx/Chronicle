// The map picture, placed pictures, hexes and shadow outlines are stacked by
// pane z-index across several scripts. Leaflet's own overlay pane sits at 400,
// so the map picture must have its own pane below the layers drawn on it.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const read = (p) => readFileSync(new URL('../../internal/plugins/maps/static/js/' + p, import.meta.url), 'utf8');
const viewer = read('map_viewer.js');

function paneZ(src, name) {
  const re = new RegExp("(?:createPane|getPane)\\('" + name + "'\\)[^\\n]*?zIndex\\s*=\\s*(\\d+)|var pane = map\\.getPane\\('" + name + "'\\)[\\s\\S]*?pane\\.style\\.zIndex = (\\d+)");
  const m = src.match(re);
  assert.ok(m, 'no z-index found for pane ' + name);
  return Number(m[1] || m[2]);
}

test('the map picture is drawn in its own pane', () => {
  assert.match(viewer, /L\.imageOverlay\(mapImageURL, bounds, \{ pane: 'mpBase' \}\)/);
});

test('the map picture sits under pictures, hexes and shadow outlines', () => {
  const base = paneZ(viewer, 'mpBase');
  const pictures = paneZ(read('map_pictures.js'), 'mpPictures');
  const hexes = paneZ(read('map_hexes.js'), 'mpHexes');
  const staff = read('map_shadow.js').match(/staffPane\.style\.zIndex = (\d+)/);
  assert.ok(staff);
  for (const [name, z] of [['pictures', pictures], ['hexes', hexes], ['shadow outlines', Number(staff[1])]]) {
    assert.ok(base < z, 'map picture (' + base + ') must be under ' + name + ' (' + z + ')');
  }
});
