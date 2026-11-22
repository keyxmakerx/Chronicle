// _card_border_fixture.mjs — what a page hands the entry form's "Card border"
// row: the border library (data-borders), the picker's lists
// (data-border-picker), and the scripts, stylesheets and pictures behind them,
// served from the real files. Shared by the three browser files that open the
// entry form (the hero creator, the hero builder, the Own entries page).

import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
export const STATIC = join(root, 'internal/plugins/entities/static');
export const file = (rel) => readFileSync(join(STATIC, rel));

const mk = (img, extra = {}) => ({ img, ...extra });
const LIB = {
  'carved-stone': mk('earth.webp', { corner: 'carved-stone-corner.webp', cornerMask: 'carved-stone-corner-mask.webp', peak: 0.93, gain: 1.3 }),
  earth: mk('earth.webp'),
  fire: mk('fire.webp', { mask: 'fire-mask.webp', peak: 1, gain: 1.3 }),
  ice: mk('ice.webp'),
  storm: mk('storm.webp', { mask: 'storm-mask.webp', peak: 1, gain: 1.3 }),
  glass: mk('glass.webp'),
  'glass-smoke': mk('glass-smoke.webp'),
  'glass-sea': mk('glass-sea.webp'),
  'glass-rose': mk('glass-rose.webp'),
  'glass-midnight': mk('glass-midnight.webp'),
  'rune-metal': mk('rune-metal.webp', { mask: 'rune-metal-mask.webp', peak: 0.96, gain: 1.3 }),
  'great-rune': mk('great-rune.webp', { mask: 'great-rune-mask.webp', peak: 0, gain: 0.85 }),
  mechanical: mk('mechanical.webp', { mask: 'mechanical-mask.webp', corner: 'mechanical-corners.webp', cornerMask: 'mechanical-corners-mask.webp', ground: 'mechanical-steam.webp', peak: 0, gain: 1, glow: 1.4 }),
  'thorn-glass': mk('thorn-glass.webp', { mask: 'thorn-glass-mask.webp', peak: 0.92, gain: 1.3 }),
  scales: mk('scales.webp', { over: 'scales-boss.webp', dye: true }),
  vine: mk('vine.webp', { mask: 'vine-mask.webp', peak: 0.74, gain: 1.3 }),
};
const URLS = ['img', 'mask', 'over', 'corner', 'cornerMask', 'ground'];
export const libJSON = JSON.stringify(Object.fromEntries(Object.entries(LIB).map(([k, v]) => [k, Object.fromEntries(Object.entries(v).map(([p, x]) => [p, URLS.includes(p) ? '/borders/' + x + '?v=abc123' : x]))])));

const GROUPS = [
  { name: 'Elements', keys: ['earth', 'fire', 'ice', 'storm'] },
  { name: 'Stone and metal', keys: ['carved-stone', 'rune-metal', 'great-rune', 'mechanical'] },
  { name: 'Glass', keys: ['glass', 'glass-smoke', 'glass-sea', 'glass-rose', 'glass-midnight'] },
  { name: 'Wild', keys: ['thorn-glass', 'scales', 'vine'] },
];
export const INFO = {
  'carved-stone': ['Carved stone', '#e0a030'], earth: ['Earth', '#b08a5a'], fire: ['Fire', '#d9622b'], ice: ['Ice', '#8fd8ff'], storm: ['Storm', '#6fa8dc'],
  glass: ['Glass', '#58c4d8'], 'glass-smoke': ['Smoke glass', '#9aa3ad'], 'glass-sea': ['Sea glass', '#4fb8a8'], 'glass-rose': ['Rose glass', '#e58fb0'], 'glass-midnight': ['Midnight glass', '#a78bfa'],
  'rune-metal': ['Rune metal', '#3fc8d4'], 'great-rune': ['Great rune', '#b48cf2'], mechanical: ['Boiler works', '#6fa8dc'],
  'thorn-glass': ['Thorn glass', '#c4243a'], scales: ['Scales', '#2fb5a4'], vine: ['Vine and wood', '#6fae4f'],
};
export const pickJSON = JSON.stringify({ groups: GROUPS, borders: Object.fromEntries(Object.keys(INFO).map((k) => [k, { name: INFO[k][0], colour: INFO[k][1], tints: !!(LIB[k].mask || LIB[k].cornerMask || LIB[k].dye) }])) });

// The two attributes as they go into a mount's opening tag (single-quoted).
export const attrEsc = (s) => s.replace(/&/g, '&amp;').replace(/'/g, '&#39;');
export const bordersAttrs = ({ lib = true, pick = true } = {}) =>
  (lib ? ` data-borders='${attrEsc(libJSON)}'` : '') + (pick ? ` data-border-picker='${attrEsc(pickJSON)}'` : '');
// The scripts the widget registry lists ahead of the form.
export const pickerScripts = '<script src="/hero_borders.js"></script><script src="/hero_border_picker.js"></script>';

// Answers the picker's own files and the pictures; false for anything else.
export function serveBorders(route, path) {
  const js = { '/hero_borders.js': 'js/hero_borders.js', '/hero_border_picker.js': 'js/hero_border_picker.js' }[path];
  if (js) { route.fulfill({ contentType: 'text/javascript', body: file(js) }); return true; }
  const css = { '/hero_borders.css': 'css/hero_borders.css', '/hero_border_picker.css': 'css/hero_border_picker.css' }[path];
  if (css) { route.fulfill({ contentType: 'text/css', body: file(css) }); return true; }
  if (path.startsWith('/borders/')) {
    const f = join(STATIC, 'img/borders', path.slice('/borders/'.length));
    if (existsSync(f)) { route.fulfill({ contentType: 'image/webp', body: readFileSync(f) }); return true; }
  }
  return false;
}

// Relative luminance contrast of two [r,g,b] colours.
export function contrast(a, b) {
  const lum = (c) => { const [r, g, bl] = c.map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * bl; };
  const x = lum(a), y = lum(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}
