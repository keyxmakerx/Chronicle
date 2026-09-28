/*
 * Chronicle calendar generators (Calendar V5 mockup engine).
 *
 * Plain JavaScript, no dependencies. Load it with a <script> tag, paste it inline, or require() it in Node:
 * it defines one global, ChronicleGen (and module.exports when there is a module system).
 *
 * Every generator runs from a RECIPE (what it makes, with sensible defaults) over a SCOPE (which days,
 * which moons, which categories) with a SEED, and keeps anything the caller marks as LOCKED. The same
 * inputs always give the same output. Every result carries a one-paragraph, plain-language `summary`.
 *
 *   ChronicleGen.run(generatorId, {calendar, recipe, scope, seed, locked, context}) -> result
 *
 * generatorId: 'weather' | 'sky' | 'events' | 'table' | 'names' | 'moons' | 'calendar' | 'history'
 *   calendar  Chronicle's preset/export shape ({format:'chronicle-calendar-v1', calendar:{...}}) or the bare
 *             calendar object (months, weekdays, moons, seasons, eras, leap_year_every, ...).
 *   recipe    a recipe object, or a built-in recipe id ('sky.classic'); omitted = the generator's default.
 *   scope     {year} | {month:{year,month}} | {week:{year,month,day}} | {day:{year,month,day}} |
 *             {season:{year,season}} | {range:{from,to}} | {days:[{year,month,day},...]}
 *             plus optional filters {moons:{only:[..], exclude:[..]}, categories:[..], zones:[..]}.
 *   seed      any string or number.
 *   locked    items to keep untouched (weather days, events, names, moons, eras) - see each generator.
 *             A weather day locked with only a preset gets a temperature that preset can happen at, and
 *             with fillLocked:true it comes back with the rest of its fields filled in.
 *   context   what the generator may read: {weather:[days], events:[..], climate, tables, history}.
 *   kinds       (weather) the owner's own weather: [{id, name, icon, color, like, seasons, climates, lasts, words,
 *               magic, look}]. Each adds a recipe switch makes['kind:<id>']; a kind that is off, off in every
 *               season, or kept to other climates leaves the run exactly as it would be without it.
 *   eventKinds  (events) the owner's own kinds of event: [{id, name, category, icon, examples, when, often, per,
 *               visibility, sky, look}], each with a switch makes['kind:<id>'] and the same guarantee.
 *   categories  (events, tables) the campaign's own categories [{id, name, icon, color}], beside the calendar's.
 *   look        (on a weather kind or a kind of event) {effect, accents, strength, tint}: the sky renderer's
 *               hand-tuned effects to lay over it. effect is the main one, one of the 41 ids in
 *               ChronicleGen.weather.effects().list (left out, the kind's `like` preset's effect is used);
 *               accents layers up to 2 more from the same list. strength and tint are unchanged.
 *
 * Results always include {generator, seed, recipe (normalised), scope:{label, days}, summary, stats,
 * warnings}. Generated items are Chronicle-shaped (the field names of the Go model and export) plus a `gen`
 * object for the editing UI (a stable key, the kind, the recipe, whether it was locked) and `announce`
 * ('ahead' | 'on-the-day'): 'ahead' for festivals, holidays and anything recurring (players may already
 * know it's coming, like a yearly meteor shower), 'on-the-day' otherwise unless a table entry, a table's
 * output or an owner's own kind of event sets it. Separate from `visibility` (who can see the event at
 * all, unchanged). Strip `gen` (and `sky` on sky events) before saving.
 *
 * Namespaces (each generator's function takes the same options as run()):
 *   ChronicleGen.cal       calendar maths: from(), scope(), validatePreset(), toPreset()
 *   ChronicleGen.recipes   builtIn(), get(), defaults(), copy(), validate(recipe, {kinds|eventKinds, categories}),
 *                          toJSON(), fromJSON(), describe(recipe, {kinds|eventKinds}), levels
 *   ChronicleGen.tables    builtIn(), validate(set, {categories}), toJSON(), fromJSON(text, {categories}), roll(),
 *                          generate(), quick(eventKind, {categories}) -> {set, recipe}, format
 *   ChronicleGen.weather   generate(), forecast(), climates(), climate(), checkClimate(), presets(kinds?), systems(),
 *                          guide(), zonePayload(), toWeatherInput(), validateKind(kind, otherKinds?), effects()
 *   ChronicleGen.sky       generate(), kinds()
 *   ChronicleGen.events    generate(), kinds(), categories, validateKind(eventKind, {categories, others}),
 *                          validateCategories(categories)
 *   ChronicleGen.names     generate(), themes()
 *   ChronicleGen.moons     generate(), phase(), moonEvents()
 *   ChronicleGen.calendar  generate(), templates()
 *   ChronicleGen.history   generate(), kinds()
 *   ChronicleGen.mass      reroll(), fillRestOfYear(), shiftEvents(), copyMonth(), everyMonth(), select()
 *   ChronicleGen.generators  the list of generators with their recipe schemas (for building forms)
 */
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module && module.exports) module.exports = api;
  if (root) root.ChronicleGen = api;
})(typeof globalThis !== 'undefined' ? globalThis : (typeof window !== 'undefined' ? window : this), function () {
'use strict';

var VERSION = '0.9.0';

/* ── Seeded randomness ──────────────────────────────────────────────────────────────────────────────
   Every draw comes from a stream named by its purpose (seed, generator, kind, day...), never from one
   shared sequence. That is what keeps results stable while the owner edits: switching blood moons off
   cannot reshuffle the meteor showers, and a week asked for on its own matches the same week of a year. */

function cyrb128(str) {
  var h1 = 1779033703, h2 = 3144134277, h3 = 1013904242, h4 = 2773480762;
  for (var i = 0, k; i < str.length; i++) {
    k = str.charCodeAt(i);
    h1 = h2 ^ Math.imul(h1 ^ k, 597399067);
    h2 = h3 ^ Math.imul(h2 ^ k, 2869860233);
    h3 = h4 ^ Math.imul(h3 ^ k, 951274213);
    h4 = h1 ^ Math.imul(h4 ^ k, 2716044179);
  }
  h1 = Math.imul(h3 ^ (h1 >>> 18), 597399067);
  h2 = Math.imul(h4 ^ (h2 >>> 22), 2869860233);
  h3 = Math.imul(h1 ^ (h3 >>> 17), 951274213);
  h4 = Math.imul(h2 ^ (h4 >>> 19), 2716044179);
  h1 ^= (h2 ^ h3 ^ h4); h2 ^= h1; h3 ^= h1; h4 ^= h1;
  return [h1 >>> 0, h2 >>> 0, h3 >>> 0, h4 >>> 0];
}

function hash32(str) { return cyrb128(String(str))[0]; }

function makeRng() {
  var parts = [];
  for (var i = 0; i < arguments.length; i++) parts.push(String(arguments[i]));
  var s = cyrb128(parts.join('␟')), a = s[0], b = s[1], c = s[2], d = s[3];
  function next() {
    a |= 0; b |= 0; c |= 0; d |= 0;
    var t = (a + b | 0) + d | 0;
    d = d + 1 | 0;
    a = b ^ b >>> 9;
    b = c + (c << 3) | 0;
    c = (c << 21 | c >>> 11);
    c = c + t | 0;
    return (t >>> 0) / 4294967296;
  }
  for (var w = 0; w < 12; w++) next();
  var spare = null;
  var r = {
    next: next,
    int: function (lo, hi) { return lo + Math.floor(next() * (hi - lo + 1)); },
    range: function (lo, hi) { return lo + next() * (hi - lo); },
    chance: function (p) { return next() < p; },
    pick: function (arr) { return arr.length ? arr[Math.floor(next() * arr.length)] : undefined; },
    normal: function () {
      if (spare !== null) { var v = spare; spare = null; return v; }
      var u = 0, q = 0;
      while (u === 0) u = next();
      q = next();
      var m = Math.sqrt(-2 * Math.log(u));
      spare = m * Math.sin(2 * Math.PI * q);
      return m * Math.cos(2 * Math.PI * q);
    },
    /* items: array; weightOf(item, index) -> weight >= 0. Returns undefined when every weight is zero. */
    weighted: function (items, weightOf) {
      var total = 0, ws = [];
      for (var i = 0; i < items.length; i++) { var w = Math.max(0, +weightOf(items[i], i) || 0); ws.push(w); total += w; }
      if (total <= 0) return undefined;
      var x = next() * total;
      for (var j = 0; j < items.length; j++) { x -= ws[j]; if (x < 0) return items[j]; }
      return items[items.length - 1];
    },
    shuffle: function (arr) {
      var out = arr.slice();
      for (var i = out.length - 1; i > 0; i--) { var j = Math.floor(next() * (i + 1)); var t = out[i]; out[i] = out[j]; out[j] = t; }
      return out;
    }
  };
  return r;
}

/* A stable short key for generated items, so the UI can select, lock and diff them across runs. */
function makeKey(prefix) {
  var parts = [];
  for (var i = 1; i < arguments.length; i++) parts.push(String(arguments[i]));
  return prefix + '-' + (hash32(parts.join('|')) >>> 0).toString(36);
}

/* A friendly seed for the dice button. Deliberately NOT deterministic: it is the one place chance enters. */
var SEED_ADJ = ['amber', 'ashen', 'bright', 'cinder', 'copper', 'dusky', 'ember', 'frost', 'gilded', 'hollow', 'iron', 'jade',
  'lantern', 'moss', 'north', 'pale', 'quiet', 'rook', 'salt', 'silver', 'tallow', 'thorn', 'umber', 'velvet', 'willow'];
var SEED_NOUN = ['badger', 'bell', 'comet', 'crow', 'ferry', 'fox', 'gate', 'hare', 'heron', 'kettle', 'lamp', 'marsh',
  'mill', 'moth', 'otter', 'owl', 'pike', 'quarry', 'reed', 'stag', 'tide', 'vane', 'wren', 'yarrow'];
function randomSeed() {
  function pick(a) { return a[Math.floor(Math.random() * a.length)]; }
  return pick(SEED_ADJ) + '-' + pick(SEED_NOUN) + '-' + (10 + Math.floor(Math.random() * 90));
}

/* ── Small maths ── */
function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }
function lerp(a, b, t) { return a + (b - a) * t; }
function smooth(t) { t = clamp(t, 0, 1); return t * t * (3 - 2 * t); }
function mod(n, m) { return ((n % m) + m) % m; }
function round1(x) { return Math.round(x * 10) / 10; }
function round2(x) { return Math.round(x * 100) / 100; }
function sum(arr) { var s = 0; for (var i = 0; i < arr.length; i++) s += arr[i]; return s; }
function angleLerp(a, b, t) { var d = mod(b - a + 540, 360) - 180; return mod(a + d * t, 360); }
function angleDiff(a, b) { return Math.abs(mod(b - a + 540, 360) - 180); }

/* ── Text ── */
function cap(s) { s = String(s == null ? '' : s); return s.charAt(0).toUpperCase() + s.slice(1); }
function lowerFirst(s) { s = String(s == null ? '' : s); return s.charAt(0).toLowerCase() + s.slice(1); }
function titleWords(s) {
  var small = { of: 1, the: 1, and: 1, a: 1, an: 1, in: 1, on: 1, at: 1, to: 1, by: 1, for: 1, under: 1, over: 1 };
  return String(s).split(' ').map(function (w, i) { return (i > 0 && small[w.toLowerCase()]) ? w.toLowerCase() : cap(w); }).join(' ');
}
/* "a" or "an" by sound, with the usual English exceptions. */
function article(word) {
  var w = String(word).toLowerCase().replace(/^[^a-z0-9]+/, '');
  if (/^(hour|honest|honou?r|heir)/.test(w)) return 'an';
  if (/^(uni|use|usu|ure|euro|one|once|ewe)/.test(w)) return 'a';
  return /^[aeiou]/.test(w) ? 'an' : 'a';
}
function withArticle(word) { return article(word) + ' ' + word; }
function plural(word) {
  var w = String(word);
  if (/(s|x|z|ch|sh)$/i.test(w)) return w + 'es';
  if (/[^aeiou]y$/i.test(w)) return w.slice(0, -1) + 'ies';
  if (/(f|fe)$/i.test(w) && !/(roof|chief|belief)$/i.test(w)) return w.replace(/(f|fe)$/i, 'ves');
  return w + 's';
}
function joinList(items) {
  var a = items.filter(function (x) { return x !== '' && x != null; });
  if (a.length <= 1) return a.join('');
  return a.slice(0, -1).join(', ') + ' and ' + a[a.length - 1];
}
var NUM_WORDS = ['no', 'one', 'two', 'three', 'four', 'five', 'six', 'seven', 'eight', 'nine', 'ten', 'eleven', 'twelve',
  'thirteen', 'fourteen', 'fifteen', 'sixteen', 'seventeen', 'eighteen', 'nineteen', 'twenty'];
function numWord(n) { return (n >= 0 && n < NUM_WORDS.length && n === Math.floor(n)) ? NUM_WORDS[n] : String(n); }
/* "3 storms", "one storm" - counts read as prose in summaries. */
function count(n, noun, nounPlural) { return numWord(n) + ' ' + (n === 1 ? noun : (nounPlural || plural(noun))); }
var ORD_WORDS = ['zeroth', 'first', 'second', 'third', 'fourth', 'fifth', 'sixth', 'seventh', 'eighth', 'ninth', 'tenth',
  'eleventh', 'twelfth', 'thirteenth', 'fourteenth', 'fifteenth', 'sixteenth'];
function ordWord(n) { return ORD_WORDS[n] || (n + 'th'); }
function roman(n) {
  var map = [[10, 'X'], [9, 'IX'], [5, 'V'], [4, 'IV'], [1, 'I']], out = '';
  for (var i = 0; i < map.length; i++) while (n >= map[i][0]) { out += map[i][1]; n -= map[i][0]; }
  return out;
}
/* Fills {name} slots from a map; unknown slots stay visible so a template bug shows up in review. */
function fill(tpl, vars) {
  return String(tpl).replace(/\{([A-Za-z_]+)\}/g, function (m, k) {
    if (vars[k] != null) return vars[k];
    var lk = lowerFirst(k);
    if (lk !== k && vars[lk] != null) return cap(vars[lk]);
    return m;
  });
}

/* ── Colour ── */
/* Colours are stored as short oklch() strings: Chronicle's old colour columns were VARCHAR(20), and
   "oklch(0.62 0.13 245)" is exactly 20 characters, so lightness and chroma keep two decimals and hue none. */
function oklch(L, C, H) { return 'oklch(' + clamp(L, 0, 1).toFixed(2) + ' ' + clamp(C, 0, 0.37).toFixed(2) + ' ' + Math.round(mod(H, 360)) + ')'; }
function hexToRgb(hex) {
  var h = String(hex).replace('#', '');
  if (h.length === 3) h = h.split('').map(function (c) { return c + c; }).join('');
  var n = parseInt(h, 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}
function rgbToHex(r, g, b) {
  return '#' + [r, g, b].map(function (v) { var s = Math.round(clamp(v, 0, 255)).toString(16); return s.length < 2 ? '0' + s : s; }).join('');
}
function hslToHex(h, s, l) {
  h = mod(h, 360) / 360; s = clamp(s, 0, 1); l = clamp(l, 0, 1);
  function f(n) { var k = (n + h * 12) % 12, a = s * Math.min(l, 1 - l); return l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1)); }
  return rgbToHex(f(0) * 255, f(8) * 255, f(4) * 255);
}
function mixHex(a, b, t) {
  var x = hexToRgb(a), y = hexToRgb(b);
  return rgbToHex(lerp(x[0], y[0], t), lerp(x[1], y[1], t), lerp(x[2], y[2], t));
}

/* ── Objects ── */
function clone(x) { return x == null ? x : JSON.parse(JSON.stringify(x)); }
function isObj(x) { return !!x && typeof x === 'object' && !Array.isArray(x); }
function deepFreeze(o) {
  if (o && typeof o === 'object' && !Object.isFrozen(o)) {
    Object.freeze(o);
    Object.keys(o).forEach(function (k) { deepFreeze(o[k]); });
  }
  return o;
}
function ownKeys(o) { return o ? Object.keys(o) : []; }
function uniq(arr) { var seen = {}, out = []; arr.forEach(function (x) { var k = typeof x + ':' + x; if (!seen[k]) { seen[k] = 1; out.push(x); } }); return out; }

/* ── Calendar maths ─────────────────────────────────────────────────────────────────────────────────
   Mirrors internal/plugins/calendar/model.go so generated dates, moon phases and weekdays agree with the
   server: AbsoluteDay is the same closed form (leap-aware), and the weekday column is the model's
   constant-length counter mod week length (constLenDayIndex). TODO(#741): V5 may reconcile the two; a
   caller can pass weekdayOf to follow whatever it decides. Months are 1-based everywhere, like the Event model. */

function GenError(message, details) {
  var e = new Error(message);
  e.name = 'GenError';
  e.friendly = true;
  if (details) e.details = details;
  return e;
}

var SEASON_TYPES = ['winter', 'spring', 'summer', 'autumn'];
var SEASON_THETA = { winter: 0, spring: 0.25, summer: 0.5, autumn: 0.75, wet: 0.5, dry: 0 };
/* Order matters: the first rule that matches a season's name wins. */
var SEASON_WORDS = [
  ['wet', /\b(the )?rains?\b|\bwet\b|monsoon|flood/i],
  ['dry', /\bdry\b|drought|\bdust/i],
  ['winter', /winter|frost|rime|snow|\bice|cold|long ?night|deep ?(cold|dark)|hoar|\bdark/i],
  ['spring', /spring|thaw|bud|seed|green|waking|\bsow|lamb|blossom|bloom/i],
  ['summer', /summer|high ?sun|zenith|bright|heat|\bsun\b|sunreign|midyear|\bburn/i],
  ['autumn', /autumn|\bfall\b|harvest|fad(e|ing)|wan(e|ing)|turning|leaf|reap|withering/i]
];
/* A season named by one of the themes is typed by the theme itself ("The Long Days" is summer); anything
   else by the words in its name. Filled in once the themes are loaded. */
var THEME_SEASON_TYPES = null;
function inferSeasonType(name) {
  var n = String(name || '');
  if (!THEME_SEASON_TYPES && typeof THEMES !== 'undefined') {
    THEME_SEASON_TYPES = {};
    ownKeys(THEMES).forEach(function (k) { ownKeys(THEMES[k].seasons).forEach(function (t) { String(THEMES[k].seasons[t]).split('|').forEach(function (ph) { THEME_SEASON_TYPES[ph.trim().toLowerCase()] = t; }); }); });
  }
  if (THEME_SEASON_TYPES && THEME_SEASON_TYPES[n.trim().toLowerCase()]) return THEME_SEASON_TYPES[n.trim().toLowerCase()];
  for (var i = 0; i < SEASON_WORDS.length; i++) if (SEASON_WORDS[i][1].test(n)) return SEASON_WORDS[i][0];
  return null;
}

function daysInGregorianMonth(y, m) { return new Date(Date.UTC(y, m, 0)).getUTCDate(); }
function gregorianJDN(y, m, d) {
  var a = Math.floor((14 - m) / 12), yy = y + 4800 - a, mm = m + 12 * a - 3;
  return d + Math.floor((153 * mm + 2) / 5) + 365 * yy + Math.floor(yy / 4) - Math.floor(yy / 100) + Math.floor(yy / 400) - 32045;
}

/* input: a preset/export envelope, a bare calendar object, or an existing Cal (returned as is). */
function makeCal(input, opts) {
  if (input && input.__cal) return input;
  opts = opts || {};
  if (!input || typeof input !== 'object') throw GenError('No calendar was given. Pass a Chronicle calendar (the preset or export shape).');
  var c = (input.format && input.calendar) ? input.calendar : (input.calendar && input.calendar.months ? input.calendar : input);
  if (!Array.isArray(c.months) || !c.months.length) throw GenError('This calendar has no months, so there are no days to fill. Add at least one month first.');

  var cal = { __cal: true, raw: c };
  cal.name = c.name || 'Calendar';
  cal.mode = c.mode || 'fantasy';
  cal.realTime = cal.mode === 'reallife' && !!c.tracks_real_time;
  cal.epoch = c.epoch_name || '';
  cal.hoursPerDay = c.hours_per_day > 0 ? c.hours_per_day : 24;
  cal.leapEvery = c.leap_year_every > 0 ? c.leap_year_every : 0;
  cal.leapOffset = c.leap_year_offset || 0;
  cal.currentYear = c.current_year != null ? c.current_year : 1;
  cal.currentMonth = c.current_month || 1;
  cal.currentDay = c.current_day || 1;

  var ms = c.months.map(function (m, i) { return { m: m, i: i }; });
  ms.sort(function (a, b) { var sa = a.m.sort_order != null ? a.m.sort_order : a.i, sb = b.m.sort_order != null ? b.m.sort_order : b.i; return sa - sb || a.i - b.i; });
  cal.months = ms.map(function (x, i) {
    return { index: i + 1, name: x.m.name || ('Month ' + (i + 1)), days: Math.max(0, x.m.days | 0), leapDays: Math.max(0, x.m.leap_year_days | 0), intercalary: !!x.m.is_intercalary };
  });
  var wds = (c.weekdays || []).map(function (w, i) { return { w: w, i: i }; });
  wds.sort(function (a, b) { var sa = a.w.sort_order != null ? a.w.sort_order : a.i, sb = b.w.sort_order != null ? b.w.sort_order : b.i; return sa - sb || a.i - b.i; });
  cal.weekdays = wds.map(function (x, i) { return { index: i, name: x.w.name || ('Day ' + (i + 1)), rest: !!x.w.is_rest_day }; });
  cal.weekLength = cal.weekdays.length;
  cal.moons = (c.moons || []).map(function (m, i) {
    return {
      index: i, name: m.name || ('Moon ' + (i + 1)), cycle: +m.cycle_days || 0, offset: +m.phase_offset || 0,
      color: m.color || '#d8dde6', size: m.size > 0 ? +m.size : (i === 0 ? 1 : 0.75),
      hidden: m.visibility === 'dm_only' || !!m.hidden, raw: m
    };
  });
  cal.eras = (c.eras || []).map(function (e, i) { return { index: i, name: e.name, start: e.start_year, end: e.end_year == null ? null : e.end_year, color: e.color, description: e.description || '' }; });

  var yl = 0, lx = 0;
  cal.months.forEach(function (m) { yl += m.days; lx += m.leapDays; });
  cal.baseYearLength = yl;
  cal.leapExtra = lx;
  if (yl <= 0 && lx <= 0) throw GenError('Every month in this calendar has zero days, so there are no days to fill.');

  cal.isLeap = function (y) {
    if (cal.realTime) return (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0;
    return cal.leapEvery > 0 && (y - cal.leapOffset) % cal.leapEvery === 0;
  };
  cal.monthDays = function (m, y) {
    if (m < 1 || m > cal.months.length) return 0;
    if (cal.realTime) return daysInGregorianMonth(y, m);
    var M = cal.months[m - 1];
    return M.days + (cal.isLeap(y) ? M.leapDays : 0);
  };
  cal.yearLength = function (y) { var t = 0; for (var m = 1; m <= cal.months.length; m++) t += cal.monthDays(m, y); return t; };
  function leapYearsBefore(y) {
    var e = cal.leapEvery;
    if (e <= 0 || y <= 0) return 0;
    var r = mod(cal.leapOffset, e);
    if (y <= r) return 0;
    return Math.floor((y - 1 - r) / e) + 1;
  }
  function yearStart(y) { // days before day 1 of year y (AbsoluteDay(y,1,1) - 1)
    if (cal.realTime) return gregorianJDN(y, 1, 1) - 1;
    if (y <= 0) return 0;
    return y * cal.baseYearLength + cal.leapExtra * leapYearsBefore(y);
  }
  /* Chronicle's AbsoluteDay. Years below 0 collapse onto year 0 there, so generators never use them. */
  cal.abs = function (y, m, d) {
    if (cal.realTime) return gregorianJDN(y, m, d);
    var t = yearStart(y);
    for (var i = 1; i < m && i <= cal.months.length; i++) t += cal.monthDays(i, y);
    return t + d;
  };
  cal.absOf = function (date) { return cal.abs(date.year, date.month, date.day); };
  cal.fromAbs = function (n) {
    if (cal.realTime) { var t = new Date((n - 2440588) * 864e5); return { year: t.getUTCFullYear(), month: t.getUTCMonth() + 1, day: t.getUTCDate() }; }
    var avg = cal.baseYearLength + (cal.leapEvery ? cal.leapExtra / cal.leapEvery : 0);
    var y = Math.max(0, Math.floor((n - 1) / avg));
    while (y > 0 && yearStart(y) >= n) y--;
    while (yearStart(y + 1) < n) y++;
    var rest = n - yearStart(y), m = 1, md;
    while (m <= cal.months.length && rest > (md = cal.monthDays(m, y))) { rest -= md; m++; }
    if (m > cal.months.length) { m = cal.months.length; rest = cal.monthDays(m, y); }
    return { year: y, month: m, day: rest };
  };
  cal.valid = function (y, m, d) { return m >= 1 && m <= cal.months.length && d >= 1 && d <= cal.monthDays(m, y) && y >= 0; };
  cal.validDate = function (date) { return !!date && cal.valid(date.year, date.month, date.day); };
  cal.isIntercalary = function (m) { return !!(cal.months[m - 1] && cal.months[m - 1].intercalary); };
  /* A day that exists only in leap years: a recurring event based there would skip three years in four. */
  cal.isLeapOnly = function (y, m, d) { var M = cal.months[m - 1]; return !cal.realTime && !!M && d > M.days; };

  /* The model's weekday column (constLenDayIndex mod week length), or the caller's own rule. */
  cal.constLenIndex = function (y, m, d) {
    var t = y * cal.baseYearLength;
    for (var i = 1; i < m && i <= cal.months.length; i++) t += cal.months[i - 1].days;
    return t + d;
  };
  cal.weekday = function (y, m, d) {
    var wl = cal.weekLength;
    if (!wl) return -1;
    if (typeof opts.weekdayOf === 'function') return mod(opts.weekdayOf(y, m, d), wl);
    if (cal.realTime) return mod(gregorianJDN(y, m, d) + 0, wl); // JDN%7: 0 = Monday
    return mod(cal.constLenIndex(y, m, d), wl);
  };
  cal.weekdayOfAbs = function (n) { var d = cal.fromAbs(n); return cal.weekday(d.year, d.month, d.day); };
  cal.isRestDay = function (y, m, d) { var w = cal.weekday(y, m, d); return w >= 0 && !!cal.weekdays[w] && cal.weekdays[w].rest; };
  cal.dayOfYear = function (y, m, d) { return cal.abs(y, m, d) - cal.abs(y, 1, 1); };

  cal.key = function (date) { return date.year + '-' + date.month + '-' + date.day; };
  cal.keyOfAbs = function (n) { return cal.key(cal.fromAbs(n)); };
  cal.fmt = function (date, withYear) {
    if (!date) return '';
    var M = cal.months[date.month - 1];
    var name = M ? M.name : ('month ' + date.month);
    var y = withYear === false ? '' : ' ' + date.year;
    if (M && !cal.realTime && M.intercalary && cal.monthDays(date.month, date.year) === 1) return name + y;
    if (cal.realTime) return date.day + ' ' + name + y;
    return date.day + ' ' + name + y;
  };
  cal.fmtAbs = function (n, withYear) { return cal.fmt(cal.fromAbs(n), withYear); };
  cal.fmtRange = function (a, b) {
    if (a === b) return cal.fmtAbs(a);
    var x = cal.fromAbs(a), y = cal.fromAbs(b);
    return cal.fmt(x, x.year !== y.year) + ' – ' + cal.fmt(y);
  };

  /* ── Seasons: Chronicle's ContainsDate semantics, plus an inferred type for weather and festivals. ── */
  var overrides = opts.seasonTypes || {};
  cal.seasons = (c.seasons || []).map(function (s, i) {
    var t = overrides[s.name] || overrides[i] || (SEASON_THETA[s.weather_effect] != null ? s.weather_effect : null) || inferSeasonType(s.name);
    return { index: i, name: s.name, sm: s.start_month, sd: s.start_day, em: s.end_month, ed: s.end_day, color: s.color, type: t, description: s.description || '' };
  });
  function containsDate(s, m, d) {
    var a = s.sm * 100 + s.sd, b = s.em * 100 + s.ed, v = m * 100 + d;
    return a <= b ? (v >= a && v <= b) : (v >= a || v <= b);
  }
  cal.seasonAt = function (m, d) {
    for (var i = 0; i < cal.seasons.length; i++) if (containsDate(cal.seasons[i], m, d)) return cal.seasons[i];
    return null;
  };
  /* Where each season sits in a common (non-leap) year, as day-of-year numbers 0..L-1. */
  var refYear = 1;
  if (!cal.realTime && cal.leapEvery) { while (cal.isLeap(refYear) && refYear < 50) refYear++; }
  if (cal.realTime) refYear = 2026; // any common Gregorian year does
  var L = cal.yearLength(refYear);
  cal.refYearLength = L;
  function doyOf(m, d) {
    m = clamp(m | 0, 1, cal.months.length);
    d = clamp(d | 0, 1, Math.max(1, cal.monthDays(m, refYear)));
    return cal.abs(refYear, m, d) - cal.abs(refYear, 1, 1);
  }
  cal.seasons.forEach(function (s) {
    s.startDoy = doyOf(s.sm, s.sd);
    s.endDoy = doyOf(s.em, s.ed);
    s.length = s.startDoy <= s.endDoy ? s.endDoy - s.startDoy + 1 : (L - s.startDoy) + s.endDoy + 1;
    s.midDoy = mod(s.startDoy + s.length / 2, L);
  });
  /* The annual phase theta: 0 = the heart of winter, 0.25 spring, 0.5 high summer, 0.75 autumn. Typed
     seasons pin it at their midpoints and it runs linearly between them, so a climate changes gradually
     through the year instead of stepping at each season boundary. */
  // Two seasons of one type (an early and a late winter, say) share their quarter of the year.
  var typed = cal.seasons.filter(function (s) { return s.type && SEASON_THETA[s.type] != null; });
  var anchors = typed.map(function (s) {
    var same = typed.filter(function (x) { return x.type === s.type; }).sort(function (a, b) { return a.startDoy - b.startDoy; });
    var k = same.indexOf(s), n = same.length;
    return { doy: s.midDoy, theta: mod(SEASON_THETA[s.type] + (n > 1 ? (k - (n - 1) / 2) * (0.2 / n) : 0), 1) };
  }).sort(function (a, b) { return a.doy - b.doy; });
  function consistent(list) {
    if (list.length < 2) return true;
    var total = 0;
    for (var i = 0; i < list.length; i++) {
      var a = list[i], b = list[(i + 1) % list.length];
      var dt = mod(b.theta - a.theta, 1);
      if (dt === 0 && list.length > 1) return false;
      total += dt;
    }
    return Math.abs(total - 1) < 1e-6;
  }
  if (!consistent(anchors)) anchors = [];
  cal.phaseSource = anchors.length >= 2 ? 'seasons' : anchors.length === 1 ? 'one season' : 'year start';
  var defaultShift = Math.round(L * 0.04);
  cal.thetaOfDoy = function (x) {
    if (!anchors.length) return mod((x - defaultShift) / L, 1);
    if (anchors.length === 1) return mod(anchors[0].theta + (x - anchors[0].doy) / L, 1);
    var i = 0;
    while (i < anchors.length && anchors[i].doy <= x) i++;
    var a = anchors[mod(i - 1, anchors.length)], b = anchors[mod(i, anchors.length)];
    var span = mod(b.doy - a.doy, L) || L, t = mod(x - a.doy, L) / span;
    return mod(a.theta + mod(b.theta - a.theta, 1) * t, 1);
  };
  cal.theta = function (y, m, d) {
    var doy = cal.dayOfYear(y, m, d), len = cal.yearLength(y);
    return cal.thetaOfDoy(len === L ? doy : Math.round(doy * L / len));
  };
  cal.thetaOfAbs = function (n) { var d = cal.fromAbs(n); return cal.theta(d.year, d.month, d.day); };
  /* The canonical season a day belongs to: its calendar season's type when known, else by theta. */
  cal.seasonTypeAt = function (y, m, d) {
    var s = cal.seasonAt(m, d);
    if (s && s.type) return s.type === 'wet' ? 'summer' : s.type === 'dry' ? 'winter' : s.type;
    var th = cal.theta(y, m, d);
    return SEASON_TYPES[Math.round(th * 4) % 4];
  };
  /* Calendaria's astronomical anchors: an equinox is the first day of its season, a solstice the middle. */
  cal.anchorDays = function (year) {
    var out = {};
    function firstOf(type) { return cal.seasons.filter(function (s) { return s.type === type; })[0]; }
    function dateAtDoy(doy) { var a = cal.abs(year, 1, 1) + clamp(Math.round(doy), 0, cal.yearLength(year) - 1); return cal.fromAbs(a); }
    function byTheta(target) {
      var best = 0, bd = 9;
      for (var x = 0; x < L; x++) { var dd = angleDiff(cal.thetaOfDoy(x) * 360, target * 360); if (dd < bd) { bd = dd; best = x; } }
      return best;
    }
    var sp = firstOf('spring'), su = firstOf('summer'), au = firstOf('autumn'), wi = firstOf('winter');
    out.springEquinox = dateAtDoy(sp ? sp.startDoy : byTheta(0.125));
    out.summerSolstice = dateAtDoy(su ? su.midDoy : byTheta(0.5));
    out.autumnEquinox = dateAtDoy(au ? au.startDoy : byTheta(0.625));
    out.winterSolstice = dateAtDoy(wi ? wi.midDoy : byTheta(0));
    var wet = firstOf('wet'), dry = firstOf('dry');
    if (wet) out.rainsBegin = dateAtDoy(wet.startDoy);
    if (dry) out.dryBegins = dateAtDoy(dry.startDoy);
    return out;
  };

  cal.monthSpan = function (y, m) { return [cal.abs(y, m, 1), cal.abs(y, m, Math.max(1, cal.monthDays(m, y)))]; };
  cal.yearSpan = function (y) { return [cal.abs(y, 1, 1), cal.abs(y, 1, 1) + cal.yearLength(y) - 1]; };
  cal.weekSpan = function (y, m, d) {
    var n = cal.abs(y, m, d), wl = cal.weekLength || 7, w = cal.weekLength ? cal.weekday(y, m, d) : 0;
    return [n - w, n - w + wl - 1];
  };
  /* The occurrence of a season that starts in year y. */
  cal.seasonSpan = function (y, s) {
    var start = cal.abs(y, s.sm, Math.min(s.sd, Math.max(1, cal.monthDays(s.sm, y))));
    var endYear = (s.sm * 100 + s.sd) <= (s.em * 100 + s.ed) ? y : y + 1;
    var end = cal.abs(endYear, s.em, Math.min(s.ed, Math.max(1, cal.monthDays(s.em, endYear))));
    return [start, end];
  };
  cal.findMoon = function (ref) {
    if (ref == null) return null;
    if (typeof ref === 'number') return cal.moons[ref] || null;
    var r = String(ref).toLowerCase();
    return cal.moons.filter(function (m) { return m.name.toLowerCase() === r; })[0] || null;
  };
  cal.findSeason = function (ref) {
    if (ref == null) return null;
    if (typeof ref === 'number') return cal.seasons[ref] || null;
    var r = String(ref).toLowerCase();
    return cal.seasons.filter(function (s) { return s.name.toLowerCase() === r; })[0] ||
      cal.seasons.filter(function (s) { return s.type === r; })[0] || null;
  };
  return cal;
}

/* ── Scope: which days a run may touch, and which moons/categories/zones it is about. ── */
function dateOf(x) {
  if (!x) return null;
  if (Array.isArray(x)) return { year: x[0], month: x[1], day: x[2] };
  return { year: +x.year, month: +x.month, day: +x.day };
}
function resolveScope(cal, scope) {
  scope = scope || {};
  var out = { days: [], label: '', kind: '' };
  function checkDate(d, what) {
    if (!d || !cal.valid(d.year, d.month, d.day)) {
      var M = d && cal.months[d.month - 1];
      var why = !d ? 'no date was given' : !M ? 'this calendar has no month ' + d.month
        : ('day ' + d.day + ' is not in ' + M.name + (d.year != null ? ' ' + d.year : '') + ', which has ' + cal.monthDays(d.month, d.year) + ' days');
      throw GenError('The ' + what + ' can’t be used: ' + why + '.');
    }
    return d;
  }
  function span(a, b) { var arr = []; for (var n = a; n <= b; n++) arr.push(n); return arr; }
  if (scope.days && scope.days.length) {
    var set = {};
    scope.days.forEach(function (x) { var d = checkDate(dateOf(x), 'chosen day'); set[cal.absOf(d)] = 1; });
    out.days = Object.keys(set).map(Number).sort(function (a, b) { return a - b; });
    out.kind = 'days';
    out.label = out.days.length === 1 ? cal.fmtAbs(out.days[0]) : count(out.days.length, 'chosen day');
  } else if (scope.day) {
    var d1 = checkDate(dateOf(scope.day), 'day');
    out.days = [cal.absOf(d1)]; out.kind = 'day'; out.label = cal.fmt(d1);
  } else if (scope.week) {
    var dw = checkDate(dateOf(scope.week), 'week’s date');
    var ws = cal.weekSpan(dw.year, dw.month, dw.day);
    out.days = span(ws[0], ws[1]); out.kind = 'week';
    out.label = 'the week of ' + cal.fmtAbs(ws[0]);
  } else if (scope.month) {
    var y = +scope.month.year, m = +scope.month.month;
    if (!(m >= 1 && m <= cal.months.length)) throw GenError('This calendar has no month ' + m + '; it has ' + cal.months.length + '.');
    if (cal.monthDays(m, y) === 0) throw GenError(cal.months[m - 1].name + ' has no days in ' + y + ' (it only appears in leap years).');
    var sp = cal.monthSpan(y, m);
    out.days = span(sp[0], sp[1]); out.kind = 'month'; out.label = cal.months[m - 1].name + ' ' + y;
  } else if (scope.season) {
    var s = cal.findSeason(scope.season.season != null ? scope.season.season : scope.season.name);
    if (!s) throw GenError('This calendar has no season called “' + (scope.season.season || scope.season.name) + '”.' + (cal.seasons.length ? ' Its seasons are ' + joinList(cal.seasons.map(function (x) { return x.name; })) + '.' : ' It has no seasons yet.'));
    var ss = cal.seasonSpan(+scope.season.year, s);
    out.days = span(ss[0], ss[1]); out.kind = 'season'; out.label = s.name + ' ' + scope.season.year + ' (' + cal.fmtRange(ss[0], ss[1]) + ')';
  } else if (scope.range) {
    var a = checkDate(dateOf(scope.range.from), 'first day of the range'), b = checkDate(dateOf(scope.range.to), 'last day of the range');
    var na = cal.absOf(a), nb = cal.absOf(b);
    if (nb < na) { var t = na; na = nb; nb = t; }
    if (nb - na > 366 * 12) throw GenError('That range is ' + (nb - na + 1) + ' days long; ask for at most about twelve years at a time.');
    out.days = span(na, nb); out.kind = 'range'; out.label = cal.fmtRange(na, nb);
  } else {
    var yr = scope.year != null ? +scope.year : cal.currentYear;
    if (!(yr >= 0)) throw GenError('Years before 0 can’t be used: Chronicle counts days from year 0.');
    var ys = cal.yearSpan(yr);
    out.days = span(ys[0], ys[1]); out.kind = 'year'; out.label = 'the year ' + yr + (cal.epoch ? ' ' + cal.epoch : '');
  }
  out.from = out.days[0];
  out.to = out.days[out.days.length - 1];
  out.set = {};
  out.days.forEach(function (n) { out.set[n] = 1; });
  out.years = uniq(out.days.map(function (n) { return cal.fromAbs(n).year; }));
  // Moons in scope: names or 0-based indexes; 'only' wins over 'exclude'.
  var mo = scope.moons || {};
  var only = (mo.only || []).map(function (r) { return cal.findMoon(r); }).filter(Boolean);
  var excl = (mo.exclude || []).map(function (r) { return cal.findMoon(r); }).filter(Boolean);
  var unknown = [].concat(mo.only || [], mo.exclude || []).filter(function (r) { return !cal.findMoon(r); });
  out.moons = (only.length ? only : cal.moons).filter(function (m) { return excl.indexOf(m) < 0; });
  out.moonWarnings = unknown.map(function (r) { return 'This calendar has no moon called “' + r + '”, so it was ignored.'; });
  out.categories = scope.categories && scope.categories.length ? scope.categories.slice() : null;
  out.zones = scope.zones && scope.zones.length ? scope.zones.slice() : null;
  return out;
}

/* ── Recipes ────────────────────────────────────────────────────────────────────────────────────────
   A recipe says what a generator makes (per-kind switches), which moons/categories it cares about by
   default, and the finer details. Every field has a default, so an empty recipe works; built-ins are
   frozen, and owners save copies. validate() speaks to the owner, not to a programmer. */

var RECIPE_FORMAT = 'chronicle-generator-recipe';
var FREQ = { off: 0, rare: 0.35, normal: 1, often: 2.2 };
var FREQ_LEVELS = ['off', 'rare', 'normal', 'often'];
var GENERATORS = {};
var GEN_ORDER = [];
var BUILTIN_RECIPES = {};

function defineGenerator(spec) {
  GENERATORS[spec.id] = spec;
  GEN_ORDER.push(spec.id);
  (spec.builtIns || []).forEach(function (b) {
    var r = normaliseRecipe(spec, b, []).recipe;
    r.id = b.id; r.name = b.name; r.description = b.description || ''; r.builtIn = true; r.basedOn = null;
    BUILTIN_RECIPES[b.id] = deepFreeze(r);
  });
}
function freqMult(level) { return FREQ[level] != null ? FREQ[level] : 1; }
/* How a generator is named inside a sentence: "the sky events generator", "the table generator". */
function genNoun(spec) { return spec.noun || spec.label.toLowerCase(); }

function fieldDefault(f) { return clone(f.default); }
function friendlyValue(v) { return typeof v === 'string' ? '“' + v + '”' : JSON.stringify(v); }

/* Checks one value against its field; returns an error message or null. */
function checkField(f, v, label) {
  var t = f.type || 'freq';
  if (t === 'freq') {
    if (FREQ_LEVELS.indexOf(v) < 0) return label + ' can be off, rare, normal or often — ' + friendlyValue(v) + ' isn’t one of those.';
  } else if (t === 'count' || t === 'number' || t === 'range') {
    if (typeof v !== 'number' || !isFinite(v)) return label + ' needs a number' + (t === 'count' ? ' (a whole number)' : '') + '; ' + friendlyValue(v) + ' isn’t one.';
    if (t === 'count' && v !== Math.floor(v)) return label + ' needs a whole number; ' + v + ' has a fraction.';
    if (f.min != null && v < f.min || f.max != null && v > f.max) return label + ' must be between ' + f.min + ' and ' + f.max + '; you gave ' + v + '.';
  } else if (t === 'bool') {
    if (typeof v !== 'boolean') return label + ' is a yes-or-no switch; use true or false, not ' + friendlyValue(v) + '.';
  } else if (t === 'select') {
    var ok = f.options.some(function (o) { return o.value === v; });
    if (!ok) return label + ' can be ' + joinList(f.options.map(function (o) { return friendlyValue(o.value); })).replace(/ and ([^ ]+)$/, ' or $1') + ' — ' + friendlyValue(v) + ' isn’t one of those.';
  } else if (t === 'text') {
    if (typeof v !== 'string') return label + ' should be text.';
    if (f.max && v.length > f.max) return label + ' is ' + v.length + ' characters long; keep it under ' + f.max + '.';
  } else if (t === 'list') {
    if (!Array.isArray(v) || v.some(function (x) { return typeof x !== 'string' && typeof x !== 'number'; })) return label + ' should be a list of names.';
  } else if (t === 'weekday') {
    if (v !== 'auto' && typeof v !== 'string' && typeof v !== 'number') return label + ' should be “auto”, a weekday’s name, or its position in the week.';
  } else if (t === 'custom') {
    if (f.check) return f.check(v, label);
  }
  return null;
}

/* Fills defaults and reports problems. Never throws: bad input comes back as errors. ctx carries what the run
   was given beside the recipe: the owner's own kinds (each adds a 'kind:<id>' switch) and campaign categories. */
function normaliseRecipe(spec, input, errors, warnings, ctx) {
  warnings = warnings || []; ctx = ctx || {};
  var r = {
    format: RECIPE_FORMAT, version: 1, generator: spec.id,
    id: input.id || null, name: input.name || spec.label, description: input.description || '',
    basedOn: input.basedOn || null, builtIn: !!input.builtIn,
    makes: {}, scope: {}, details: {}
  };
  var mk = input.makes || {};
  if (!isObj(mk)) { errors.push({ path: 'makes', message: '“What it makes” should be a list of switches, like {"comet": "rare"}.' }); mk = {}; }
  ownKeys(spec.makes).forEach(function (k) {
    var f = spec.makes[k];
    if (mk[k] === undefined) { r.makes[k] = fieldDefault(f); return; }
    var e = checkField(f, mk[k], cap(f.label));
    if (e) { errors.push({ path: 'makes.' + k, message: e }); r.makes[k] = fieldDefault(f); } else r.makes[k] = mk[k];
  });
  // Your own kinds each add a switch; one the run wasn't given is ignored, as an unknown table tag is.
  var kinds = (ctx.kinds || []).filter(function (k) { return isObj(k) && typeof k.id === 'string'; }), kindIds = kinds.map(function (k) { return k.id; });
  ownKeys(mk).forEach(function (k) {
    if (k.indexOf('kind:') !== 0) return;
    var id = k.slice(5), kn = kinds[kindIds.indexOf(id)];
    if (!kn) { warnings.push({ path: 'makes.' + k, message: '“' + id + '” isn’t one of the kinds given with this recipe, so its switch will be ignored.' + (kindIds.length ? ' The kinds here are ' + joinList(kindIds) + '.' : '') }); return; }
    if (FREQ_LEVELS.indexOf(mk[k]) < 0) errors.push({ path: 'makes.' + k, message: cap(kn.name || id) + ' can be off, rare, normal or often — ' + friendlyValue(mk[k]) + ' isn’t one of those.' });
  });
  kindIds.forEach(function (id) { var v = mk['kind:' + id]; r.makes['kind:' + id] = FREQ_LEVELS.indexOf(v) >= 0 ? v : 'normal'; });
  // A generator whose switches come from its content (the table generator's tags) checks them itself.
  if (ownKeys(spec.makes).length) ownKeys(mk).forEach(function (k) {
    if (!spec.makes[k] && k.indexOf('kind:') !== 0) {
      var known = ownKeys(spec.makes).map(function (x) { return spec.makes[x].label.toLowerCase(); });
      warnings.push({ path: 'makes.' + k, message: '“' + k + '” isn’t something the ' + genNoun(spec) + ' generator makes, so it will be ignored. It makes ' + joinList(known) + '.' });
    }
  });
  var dt = input.details || {};
  if (!isObj(dt)) { errors.push({ path: 'details', message: 'The details should be a set of named settings, like {"tone": "grim"}.' }); dt = {}; }
  ownKeys(spec.details).forEach(function (k) {
    var f = spec.details[k];
    if (dt[k] === undefined) { r.details[k] = fieldDefault(f); return; }
    var e = checkField(f, dt[k], cap(f.label));
    if (e) { errors.push({ path: 'details.' + k, message: e }); r.details[k] = fieldDefault(f); } else r.details[k] = clone(dt[k]);
  });
  ownKeys(dt).forEach(function (k) {
    if (!spec.details[k]) warnings.push({ path: 'details.' + k, message: 'The detail “' + k + '” isn’t used by this generator, so it will be ignored.' });
  });
  var sc = input.scope || {};
  if (sc.moons) {
    if (!isObj(sc.moons) || (sc.moons.only && !Array.isArray(sc.moons.only)) || (sc.moons.exclude && !Array.isArray(sc.moons.exclude))) {
      errors.push({ path: 'scope.moons', message: 'Moons should say which to use or leave out, like {"only": ["Vantre"]} or {"exclude": ["Ashka"]}.' });
    } else r.scope.moons = { only: (sc.moons.only || []).slice(), exclude: (sc.moons.exclude || []).slice() };
  }
  if (sc.categories) {
    if (!Array.isArray(sc.categories)) errors.push({ path: 'scope.categories', message: 'Categories should be a list, like ["sky", "quest"].' });
    else {
      var cats = categoryIds(ctx.categories), bad = sc.categories.filter(function (c) { return cats.indexOf(c) < 0; });
      if (bad.length) errors.push({ path: 'scope.categories', message: joinList(bad.map(friendlyValue)) + (bad.length > 1 ? ' aren’t categories' : ' isn’t a category') + ' the calendar uses. Use ' + joinList(cats) + '.' });
      else r.scope.categories = sc.categories.slice();
    }
  }
  if (spec.extraRecipe) spec.extraRecipe(r, input, errors, warnings, ctx);
  return { recipe: r, errors: errors, warnings: warnings };
}

/* The calendar's event categories; generated events only ever use these and the campaign's own. Past
   events have their own category, so history never mixes into the views of play. */
var EVENT_KINDS = ['session', 'quest', 'social', 'festival', 'sky', 'downtime', 'away', 'history'];
var CATEGORY_FIELDS = { id: 1, name: 1, icon: 1, color: 1 };
function categoryIds(cats) { return EVENT_KINDS.concat((cats || []).filter(function (c) { return isObj(c) && typeof c.id === 'string'; }).map(function (c) { return c.id; })); }

/* Whether players might already know an event is coming. 'ahead' covers festivals and holidays (category
   "festival") and anything recurring, yearly or not (a weekly market, a yearly meteor shower) - the whole
   world already expects those. Everything else is a surprise until the day. explicit, when it's one of
   the two values, always wins; it comes from a table entry, a table's output or an owner's own kind of
   event (the only sources allowed to set it) - never from a built-in sky kind or from history. */
var ANNOUNCE_VALUES = ['ahead', 'on-the-day'];
function announceOf(ev, explicit) {
  if (explicit === 'ahead' || explicit === 'on-the-day') return explicit;
  return (ev.category === 'festival' || ev.is_recurring) ? 'ahead' : 'on-the-day';
}
/* null when v is fine (unset or a valid value), else the tail of a plain-language error sentence. */
function announceProblem(v) {
  if (v == null || ANNOUNCE_VALUES.indexOf(v) >= 0) return null;
  return 'can be shown to players “ahead” of time or only “on-the-day”; ' + friendlyValue(v) + ' isn’t one of those.';
}
/* Plain-language checks for a campaign's own categories. */
function checkCategories(cats) {
  var errors = [], warnings = [], seen = {};
  if (cats == null) return { ok: true, errors: errors, warnings: warnings };
  if (!Array.isArray(cats)) return { ok: false, errors: [{ path: 'categories', message: 'Your categories should be a list, like [{"id": "heists", "name": "Heists"}].' }], warnings: warnings };
  cats.forEach(function (c, i) {
    var where = 'categories[' + i + ']', label = isObj(c) && c.name ? '“' + c.name + '”' : 'Category ' + (i + 1);
    if (!isObj(c)) { errors.push({ path: where, message: label + ' should have an id and a name, like {"id": "heists", "name": "Heists"}.' }); return; }
    if (typeof c.id !== 'string' || !/^[a-z][a-z0-9-]*$/.test(c.id)) errors.push({ path: where + '.id', message: label + ' needs an id in lowercase letters, numbers and dashes, starting with a letter, like "heists".' });
    else if (EVENT_KINDS.indexOf(c.id) >= 0) errors.push({ path: where + '.id', message: '“' + c.id + '” is already one of the calendar’s own categories; give yours another id.' });
    else if (seen[c.id]) errors.push({ path: where + '.id', message: 'Two of your categories use the id “' + c.id + '”.' });
    if (typeof c.name !== 'string' || !c.name.trim()) errors.push({ path: where + '.name', message: 'Give ' + (c.id ? '“' + c.id + '”' : label) + ' a name people will see, like "Heists".' });
    else if (c.name.length > 40) errors.push({ path: where + '.name', message: label + ' is ' + c.name.length + ' characters long; keep category names under 40.' });
    if (c.color != null && !(typeof c.color === 'string' && /^#[0-9a-fA-F]{6}$/.test(c.color))) errors.push({ path: where + '.color', message: label + '’s colour should be written like "#c0583a".' });
    if (c.icon != null && (typeof c.icon !== 'string' || c.icon.length > 40)) errors.push({ path: where + '.icon', message: label + '’s icon should be a short name.' });
    ownKeys(c).forEach(function (k) { if (!CATEGORY_FIELDS[k]) warnings.push({ path: where + '.' + k, message: '“' + k + '” isn’t a setting for a category, so it will be ignored.' }); });
    if (typeof c.id === 'string') seen[c.id] = 1;
  });
  return { ok: !errors.length, errors: errors, warnings: warnings };
}
function categoriesOf(opts) {
  var v = checkCategories(opts && opts.categories);
  if (!v.ok) throw GenError('Your categories have ' + count(v.errors.length, 'problem') + ': ' + v.errors.map(function (e) { return e.message; }).join(' '), v.errors);
  return (opts && opts.categories) || [];
}
/* What a recipe of this generator may refer to beyond itself: the run's own kinds and categories. */
function recipeContext(generatorId, opts) {
  opts = opts || {};
  return { kinds: generatorId === 'weather' ? opts.kinds : generatorId === 'events' ? (opts.eventKinds || opts.kinds) : null, categories: opts.categories };
}

function validateRecipe(input, opts) {
  var errors = [], warnings = [];
  if (!isObj(input)) return { ok: false, errors: [{ path: '', message: 'A recipe should be a set of settings (a JSON object), not ' + (Array.isArray(input) ? 'a list' : typeof input) + '.' }], warnings: [], recipe: null };
  if (input.format && input.format !== RECIPE_FORMAT) errors.push({ path: 'format', message: 'This file is a “' + input.format + '”, not a generator recipe.' });
  var spec = GENERATORS[input.generator];
  if (!input.generator) return { ok: false, errors: errors.concat([{ path: 'generator', message: 'This recipe doesn’t say which generator it is for. Add "generator": one of ' + joinList(GEN_ORDER) + '.' }]), warnings: warnings, recipe: null };
  if (!spec) return { ok: false, errors: errors.concat([{ path: 'generator', message: 'There’s no generator called “' + input.generator + '”. Try one of ' + joinList(GEN_ORDER) + '.' }]), warnings: warnings, recipe: null };
  if (input.name != null && (typeof input.name !== 'string' || !input.name.trim())) errors.push({ path: 'name', message: 'Give the recipe a name, so you can find it again.' });
  else if (input.name && input.name.length > 80) errors.push({ path: 'name', message: 'The name is ' + input.name.length + ' characters long; keep it under 80.' });
  if (!input.builtIn && input.id && BUILTIN_RECIPES[input.id]) errors.push({ path: 'id', message: '“' + input.id + '” is a built-in recipe’s id. Built-ins can’t be changed; save a copy with its own id instead.' });
  if (input.basedOn && !BUILTIN_RECIPES[input.basedOn]) warnings.push({ path: 'basedOn', message: 'This recipe says it is based on “' + input.basedOn + '”, which isn’t a built-in here; it still works on its own.' });
  var n = normaliseRecipe(spec, input, errors, warnings, recipeContext(input.generator, opts));
  return { ok: errors.length === 0, errors: errors, warnings: warnings, recipe: n.recipe };
}

/* Resolves what a run was given (id, object or nothing) into a normalised recipe; bad recipes throw.
   opts: the run's own kinds and categories, which a recipe's switches and scope may name. */
function resolveRecipe(generatorId, recipe, opts) {
  var spec = GENERATORS[generatorId];
  if (!spec) throw GenError('There’s no generator called “' + generatorId + '”. Try one of ' + joinList(GEN_ORDER) + '.');
  if (recipe == null) recipe = spec.defaultRecipe || (spec.builtIns && spec.builtIns[0] && spec.builtIns[0].id);
  if (typeof recipe === 'string') {
    var b = BUILTIN_RECIPES[recipe];
    if (!b) throw GenError('There’s no built-in recipe called “' + recipe + '”.');
    if (b.generator !== generatorId) throw GenError('“' + b.name + '” is a recipe for the ' + genNoun(GENERATORS[b.generator]) + ' generator, not the ' + genNoun(spec) + ' one.');
    var out = clone(b);
    (recipeContext(generatorId, opts).kinds || []).forEach(function (k) { if (isObj(k) && out.makes['kind:' + k.id] === undefined) out.makes['kind:' + k.id] = 'normal'; });
    return out;
  }
  var withGen = Object.assign({}, recipe);
  if (!withGen.generator) withGen.generator = generatorId;
  if (withGen.generator !== generatorId) throw GenError('This recipe is for the ' + (GENERATORS[withGen.generator] ? genNoun(GENERATORS[withGen.generator]) : withGen.generator) + ' generator, but you ran the ' + genNoun(spec) + ' one.');
  var v = validateRecipe(withGen, opts);
  if (!v.ok) throw GenError('The recipe has ' + count(v.errors.length, 'problem') + ': ' + v.errors.map(function (e) { return e.message; }).join(' '), v.errors);
  return v.recipe;
}

function recipeDefaults(generatorId) {
  var spec = GENERATORS[generatorId];
  return spec ? normaliseRecipe(spec, {}, []).recipe : null;
}
function slugify(s) { return String(s || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40) || 'recipe'; }
function copyRecipe(source, opts) {
  opts = opts || {};
  var src = typeof source === 'string' ? BUILTIN_RECIPES[source] : source;
  if (!src) throw GenError('There’s no recipe called “' + source + '” to copy.');
  var r = clone(src);
  r.builtIn = false;
  r.basedOn = src.builtIn ? src.id : (src.basedOn || null);
  r.name = opts.name || ('My ' + lowerFirst(src.name));
  r.id = opts.id || ('my-' + slugify(r.name).replace(/^my-/, '') + '-' + (hash32(r.name + '|' + (opts.salt || '')) >>> 0).toString(36).slice(0, 4));
  if (opts.description != null) r.description = opts.description;
  return r;
}
/* Stable key order, so saved recipes diff cleanly and share as readable text. */
function recipeToJSON(r) {
  var order = ['format', 'version', 'generator', 'id', 'name', 'description', 'basedOn', 'makes', 'scope', 'details', 'start', 'tables'];
  var o = {};
  order.forEach(function (k) { if (r[k] !== undefined && r[k] !== null && !(k === 'scope' && !ownKeys(r.scope).length)) o[k] = r[k]; });
  return JSON.stringify(o, null, 2);
}
function jsonErrorMessage(text, err, what) {
  var m = /position (\d+)/.exec(err.message || ''), where = '';
  if (m) {
    var pos = +m[1], before = text.slice(0, pos), line = before.split('\n').length, col = pos - before.lastIndexOf('\n');
    where = ' Something is wrong near line ' + line + ', column ' + col + ' — often a missing comma, an extra comma, or a quote that isn’t closed.';
  }
  return 'This isn’t valid ' + what + ' text.' + (where || ' Check for a missing comma or an unclosed quote.');
}
function recipeFromJSON(text) {
  var obj;
  try { obj = JSON.parse(text); } catch (e) { return { ok: false, errors: [{ path: '', message: jsonErrorMessage(String(text), e, 'recipe') }], warnings: [], recipe: null }; }
  if (obj && obj.builtIn) obj.builtIn = false; // a pasted recipe is always the owner's own copy
  return validateRecipe(obj);
}
/* "Based on Classic sky: blood moons off, auroras often." - what an owner changed, in words. opts.kinds (or
   eventKinds) name the owner's own kinds; without them a kind is named by its id. */
function describeRecipe(r, opts) {
  var spec = GENERATORS[r.generator];
  if (!spec) return '';
  var base = r.basedOn && BUILTIN_RECIPES[r.basedOn] ? BUILTIN_RECIPES[r.basedOn] : recipeDefaults(r.generator);
  var diffs = [];
  ownKeys(spec.makes).forEach(function (k) {
    if (JSON.stringify(r.makes[k]) !== JSON.stringify(base.makes[k])) {
      var f = spec.makes[k], v = r.makes[k];
      diffs.push(f.label.toLowerCase() + ' ' + ((f.type || 'freq') === 'freq' ? v : f.type === 'bool' ? (v ? 'on' : 'off') : v));
    }
  });
  var named = (recipeContext(r.generator, opts).kinds || []).filter(isObj);
  ownKeys(r.makes || {}).forEach(function (k) {
    if (k.indexOf('kind:') !== 0 || r.makes[k] === 'normal') return;
    var kn = named.filter(function (x) { return x.id === k.slice(5); })[0];
    diffs.push((kn && kn.name ? kn.name.toLowerCase() : k.slice(5)) + ' ' + r.makes[k]);
  });
  ownKeys(spec.details).forEach(function (k) {
    if (JSON.stringify(r.details[k]) !== JSON.stringify(base.details[k])) {
      var f = spec.details[k], v = r.details[k], shown = v;
      if (f.type === 'select') { var o = f.options.filter(function (x) { return x.value === v; })[0]; shown = o ? o.label.toLowerCase() : v; }
      else if (f.type === 'bool') shown = v ? 'on' : 'off';
      else if (isObj(v) || Array.isArray(v)) shown = 'customised';
      diffs.push(f.label.toLowerCase() + ': ' + shown);
    }
  });
  if (r.scope && r.scope.moons && ((r.scope.moons.only || []).length || (r.scope.moons.exclude || []).length)) {
    if (r.scope.moons.only.length) diffs.push('only ' + joinList(r.scope.moons.only.map(String)));
    if (r.scope.moons.exclude.length) diffs.push('never ' + joinList(r.scope.moons.exclude.map(String)));
  }
  var head = r.builtIn ? r.name : (r.basedOn && BUILTIN_RECIPES[r.basedOn] ? 'Based on ' + BUILTIN_RECIPES[r.basedOn].name : spec.label + ' defaults');
  return head + (diffs.length ? ': ' + diffs.join(', ') + '.' : r.builtIn ? '.' : ', unchanged.');
}

/* The public schema a UI builds its form from: switches up front, details under "More". */
function generatorList() {
  return GEN_ORDER.map(function (id) {
    var g = GENERATORS[id];
    return {
      id: id, label: g.label, blurb: g.blurb, needsCalendar: g.needsCalendar !== false,
      scope: clone(g.scope || {}), makes: clone(g.makes), details: clone(g.details),
      builtIns: (g.builtIns || []).map(function (b) { return b.id; }), defaultRecipe: g.defaultRecipe || null
    };
  });
}

/* ── Name themes ────────────────────────────────────────────────────────────────────────────────────
   Each theme has two voices: a TONGUE (syllable inventories for invented words, like the elven preset's
   Aevel and Sithrel) and WORDS (English compounds from curated banks, like the mockup's Seedtide and
   Driftfall). Month parts are sorted by season so a month's name fits the time of year it falls in.
   Token lists are "item:weight" strings; a bare item weighs 1. Everything here was written to be read
   aloud at the table: nothing borrowed from a published setting, nothing that is a real calendar. */

var THEMES = {
  pastoral: {
    label: 'Pastoral', blurb: 'Farms, hedgerows and market towns',
    tongue: {
      onsets: 'b:2 br d f g gr h:2 l:2 m:2 n p r:2 s:2 st t th w:2', vowels: 'a:3 e:3 i:2 o:2 u', codas: 'n:3 l:2 r:2 m d t s', finals: 'n l r m d t s',
      ends: { people: 'a:2 en in el on ett', places: 'by ton ley ham wick', moons: 'a:2 el en' }
    },
    monthFirst: {
      winter: 'Deep Long Rime Hoar Frost Snow Still Hearth Dark Ice Cold',
      spring: 'Thaw Seed Lamb Bud Plough Green Sow Furrow Mud Blossom Hare Rain',
      summer: 'High Hay Bright Sun Rose Honey Bloom Meadow Clover Swallow Midge',
      autumn: 'Harvest Sheaf Leaf Apple Reap Mist Barley Ember Hazel Fallow Rook Acorn'
    },
    monthLast: { any: 'tide month', winter: 'night frost wake fast mere moot', spring: 'march wake turn', summer: 'moot mere bloom', autumn: 'wane fall turn' },
    monthWhole: {
      winter: 'Deepwinter Longnight Rimefrost Frostwake Hoarmonth Stillmere Snowfast Hearthmoot Darkmonth',
      spring: 'Thawmarch Seedtide Lambing Ploughmonth Budwake Greenmarch Furrowtide Mudmonth Blossomtide',
      summer: 'Highsun Brightbloom Haymonth Rosetide Meadowmoot Honeymonth Swallowtide Clovermonth',
      autumn: 'Harvestwane Leaffall Sheaftide Applemonth Reapmoot Mistwane Emberwane Fallowmonth Barleyfall Rookwane'
    },
    weekdayWords: 'Plough Sow Hedge Mill Fold Well Byre Loft Churn Thresh Scythe Barrow Tithe Furrow', weekdayRest: 'Restday Hearthday Stillday',
    seasons: {
      winter: 'Wintertide|The Long Night|Hearthtime|The Still Months|Deep Winter', spring: 'The Thaw|Seedtime|The Greening|Lambing Season',
      summer: 'Highsun|High Summer|Haytime|The Long Days', autumn: 'The Turning|Harvesttime|Leaffall|The Fading',
      wet: 'The Rains|The Wet', dry: 'The Dry|The Dust Months'
    },
    festivals: {
      winterSolstice: 'Longnight Vigil|Festival of the Turning|Night of Fires|Hearthwatch|Night of the Long Candles',
      summerSolstice: 'Highsun Feast|Oath-day|Sunstanding|Brightfire|Festival of the Long Day',
      springEquinox: 'Thaw Fair|The Greening|First Furrow|Lambing Feast|Seed-blessing',
      autumnEquinox: 'Leaffall Feast|Apple Wake|Balance Day|The Turning of the Leaf',
      harvest: 'Feast of the First Sheaf|Harvest Home|Last Sheaf|Tithe-feast|Reaping Fair|The Gathering-in',
      newYear: "Year's Hinge|First Morning|Doorstep Day",
      remembrance: 'Night of Names|The Quiet Day|Moonfeast|Lantern Night',
      rains: 'Rain-welcome|The Wetting of the Fields', dry: 'The Last Watering',
      moon: "{moon}-feast|Night of {moon}|{moon} Watch|The {moon} Fair"
    },
    intercalary: 'Midwinter@winter|Midsummer@summer|Moonfeast@autumn|Greenday@spring|Sheafday@autumn|Hinge-day@winter|Lanternday@winter|The Quiet Day',
    eraNouns: 'Thorns Plenty Hedges Sheaves Wolves Barrows Mills Commons Bells Wards Lanterns Furrows',
    eraAdjs: 'Long Green Hungry Quiet Hollow Golden Grey Broken Fallow Lean Second',
    eras: {
      founding: 'The First Furrows|Age of Clearings|The Hedge-laying', golden: 'The Long Plenty|The Golden Sheaves|Age of Mills',
      strife: 'The Hedge Wars|The Sundered Shires|Age of Wolves', dark: 'The Lean Years|The Hungry Winters|The Grey Harvests',
      restoration: 'The Second Sowing|The Replanting|Age of Bells', faith: 'Age of Bells|The Chapel Years', discovery: 'Age of Roads|The Opening of the Downs',
      decline: 'The Fallow Years|The Long Decline', present: 'Reckoning of Wards|Age of Lanterns|The Long Reckoning|The Hearth Years'
    },
    moonWords: 'Sickle Lamp Wren Tallow Candle Barley Hob Hollow Ewe',
    placeFirst: 'Ash Oak Thorn Mill Barrow Harrow Kettle Brook Willow Bram Sedge Rook Hazel Crane Fern Apple Bell Nettle Cress Otter Badger Wold Holly Linden Marl',
    placeLast: 'ford stead field worth ham ley wick bury hollow cross well dale combe thorpe bridge green',
    given: 'Ada~f Alder Bram~m Bryony~f Colm~m Corin~m Dora~f Edda~f Elspeth~f Ferris~m Garrow~m Gilly Hal~m Hester~f Idony~f Jory~m Kit Linnet~f Maud~f Merritt Nell~f Oswin~m Pim Rowena~f Sorrel Tamsin~f Tobin~m Wat~m Wenna~f Wynn Yarrow',
    family: 'Ashby Bracken Cobb Dunmore Fallow Gorse Harrow Hedges Kettle Marl Millward Oakes Pennock Reed Sedge Tull Wolde Thatcher Fenwick',
    titles: 'Reeve|Goodwife~f|Master~m|Widow~f|Brother~m|Sister~f|Warden|Old|Mother~f|Miller'
  },

  nautical: {
    label: 'Nautical', blurb: 'Harbours, reefs, fishing fleets',
    tongue: {
      onsets: 's:2 v:2 l:2 m n r h k t th sh w', vowels: 'a:3 e:2 i:2 o u ae', codas: 'l:2 n:2 r s', finals: 'l n r s',
      ends: { people: 'a:2 en is el ka ra', places: 'el:2 ae is or', moons: 'u:2 el a i e' }
    },
    monthFirst: {
      winter: 'Gale Squall Wrack Deep Grey Black Ice Storm', spring: 'Herring Kelp Gull First Tern Rising Mackerel',
      summer: 'Bright Pearl Calm Long Salt Glass', autumn: 'Drift Seal Brine Fog Wreck Ebb'
    },
    monthLast: { any: 'tide month', winter: 'watch turn fall swell water moon', spring: 'wake tide water', summer: 'shoal water moon strand', autumn: 'fall mere watch moon swell' },
    monthWhole: {
      winter: 'Galeturn Squallmonth Wrackfall Deepmere Stormwatch Greywater Blackwater',
      spring: 'Herringrun Kelpmonth Firstwater Gullwake Ternmonth Risingtide',
      summer: 'Brightshoal Longtide Saltmoon Pearlwater Glasswater Calmmonth',
      autumn: 'Driftfall Sealmonth Lowtide Fogmere Wreckwatch Ebbmonth Stillwater'
    },
    weekdayWords: 'Moor Salt Reed Kelp Tallow Ember Net Keel Oar Rope Pitch Brine Hook Sail Cask Tar Line Spar', weekdayRest: 'Shoreday Harbourday Stillwater',
    seasons: {
      winter: 'The Gales|Stormtide|The Grey Months|Wrack Season', spring: 'The Herring Run|First Water|The Returning',
      summer: 'The Calms|Longtide|Bright Water', autumn: 'The Drift|Fog Season|The Turning Tide',
      wet: 'The Rains|Squall Season', dry: 'The Dry|The Calms'
    },
    festivals: {
      winterSolstice: 'Lantern Night|The Beacon Watch|Night of the Long Lamps', summerSolstice: 'Tidefeast|The Longest Tide|Festival of Sails',
      springEquinox: 'Nets-out|Blessing of the Hulls|First Launch', autumnEquinox: 'Hauling Day|Nets-in|The Turning Tide',
      harvest: 'The Silver Run|Salting Feast|Herring Feast', newYear: "Year's Hinge|First Tide|Slack Water",
      remembrance: 'Lantern Night|Drowned Bells|Night of the Drowned', rains: 'Squall-greeting|The Wet Launch', dry: 'The Calm Market',
      moon: "{moon}tide|Night of {moon}|{moon} Watch"
    },
    intercalary: "Year's Hinge@winter|Lantern Night@winter|Slack Water@summer|The Still Day|Beacon Day@autumn|Tideturn@spring",
    eraNouns: 'Lanterns Sails Beacons Tides Salt Wrecks Bells Harbours Oars Charts', eraAdjs: 'Long Low Drowned Salt Quiet Grey Bright Second',
    eras: {
      founding: 'The First Landing|Age of Oars|The Charting', golden: 'Age of Sails|The Bright Water|Age of Lanterns',
      strife: 'The Salt Wars|The Reef Feuds|The Chain Years', dark: 'The Drowning|The Wreck Years|The Low Tide',
      restoration: 'The Long Quiet|The Relighting of the Beacons', faith: 'Age of Bells|The Drowned Rites', discovery: 'Age of Charts|The Far Crossing',
      decline: 'The Ebb|The Silting', present: 'Age of Lanterns|The Tidereckoning|The Long Quiet|Years of the Beacon'
    },
    moonWords: 'Pearl Gull Beacon Lantern Tide Wrack Shell Buoy',
    placeFirst: 'Salt Gull Wrack Cod Brine Seal Reef Kelp Pearl Lantern Heron Skerry Tern Shell Cockle Sprat Oyster Rook Mussel',
    placeLast: 'mere haven mouth strand wick holm ness sound reach point cove quay sands',
    given: 'Ansel~m Brin Corra~f Dace~m Eilis~f Fen Grete~f Hallan~m Isa~f Kestrel Lorn~m Nessa~f Orla~f Quenna~f Rhys~m Selka~f Tove Ulla~f Vane Wenna~f Ysolde~f Marit~f Joss',
    family: 'Blackwater Cable Drift Fairwind Hawser Keel Netley Pike Rudd Tideman Wracke Spratt Coble Lantry',
    titles: 'Captain|Harbourmaster|Old|Mother~f|Pilot|Bosun|Widow~f|Net-warden'
  },

  dwarven: {
    label: 'Dwarven', blurb: 'Deep halls, forges and long memory',
    tongue: {
      onsets: 'b:2 br d:2 dr g:2 gr k:2 th z r m n t v', vowels: 'a:3 o:3 u:2 i e', codas: 'r:3 n:2 m k:2 d rn rk ld nd l', finals: 'r n m k d rn rk nd l',
      syl: '1:3 2:3', ends: { people: 'a:2 in ak or un ra', places: 'ak:2 um ar ul', moons: 'a:2 un ar' }
    },
    monthFirst: {
      winter: 'Hearth Cinder Still Deep Banked Coal', spring: 'Ore Lode Delve Vein Seam Shaft',
      summer: 'Forge Anvil Ember Bellows Iron Blaze', autumn: 'Gild Vault Copper Tally Rune Ingot'
    },
    monthLast: { any: 'moot tide', winter: 'rest hold watch', spring: 'fall wake delve', summer: 'fire wake hall', autumn: 'hall mark watch fall' },
    monthWhole: {
      winter: 'Hearthold Cinderrest Stillstone Deepwatch Coalmoot Bankedfire',
      spring: 'Orefall Deepvein Lodewake Seamtide Delvemark Shaftmoot',
      summer: 'Ironmoot Anviltide Emberwake Bellowsmoot Blazehall Forgewake',
      autumn: 'Gildhall Vaultwatch Tallymark Copperfall Runereckon Ingotmoot'
    },
    weekdayWords: 'Anvil Tong Bellows Ingot Lode Chisel Pick Hearth Vault Rivet Slag Kiln Seam', weekdayRest: 'Hearthday Stillday Ale-day',
    seasons: {
      winter: 'The Banked Fires|Stillrock|Deepcold', spring: 'The Opening|Oreseason|Seamtide', summer: 'Forgefire|The Great Heat|Anvilseason',
      autumn: 'The Tally|Gildseason|The Sealing', wet: 'The Floodings|The Seeping', dry: 'The Dry Seams|The Dust'
    },
    festivals: {
      winterSolstice: 'The Long Vigil|Night of the Banked Fires|Deepfire Night', summerSolstice: 'Forgefire|Anvil-day|The Great Pour',
      springEquinox: 'Opening of the Gates|Surface Day|First Light Above', autumnEquinox: 'Sealing of the Gates|The Tally',
      harvest: 'Tally-day|The Counting|Ingot Feast', newYear: 'First Strike|The Relighting',
      remembrance: 'Night of Carved Names|The Stone-Reading|The Doomsaying', rains: 'The Pumping', dry: 'The Dust Moot',
      moon: "{moon} Vigil|Night of {moon}"
    },
    intercalary: 'The Long Vigil@winter|The Relighting@winter|Tally-day@autumn|The Stone-Reading|Hearth-day@winter|First Strike@spring',
    eraNouns: 'Anvils Gates Seams Vaults Hammers Runes Hearths Forges Oaths', eraAdjs: 'Sealed Long Deep Hollow Iron Broken Silent Second',
    eras: {
      founding: 'The First Delving|Age of Carving|The Deep-count', golden: 'Age of Anvils|The Long Delve|Age of Vaults',
      strife: 'The Kinstrife|The Gate Wars|The Sundered Holds', dark: 'The Collapse|The Hollowing|The Silent Forges',
      restoration: 'The Relighting|The Second Delving', faith: 'Age of Runes|The Oath-years', discovery: 'The Deep Road|Age of Seams',
      decline: 'The Sealing|The Long Sleep', present: 'The Deep-count|Age of Sealed Gates|The Long Tally'
    },
    moonWords: 'Lamp Lantern Ingot Cinder Coin Shield Rivet Ember Whetstone',
    placeFirst: 'Iron Stone Deep Anvil Cinder Copper Granite Coal Silver Slate Flint Basalt Ember Brass Tin Quartz Garnet Hollow Bell Lantern',
    placeLast: 'hold delve deep forge hall gate mine shaft vault crag spire hearth seam fast home',
    given: 'Agna~f Baldra~f Dagna~f Dorn~m Grenna~f Gundar~m Hesk~m Kara~f Korrin~m Morga~f Orsik~m Rurik~m Skalla~f Thora~f Torvin~m Ulfar~m Vonda~f Yorra~f Brenna~f Hadrik~m',
    family: 'Anvilborn Coalbeard Deepdelver Flintfist Gravelback Ironvein Stonehelm Tinderbrow Underhill Cinderhand Seamwright',
    titles: 'Forgemaster|Deepwarden|Thane|Elder|Runesmith|Hearthmother~f|Tally-keeper'
  },

  elven: {
    label: 'Elven', blurb: 'Old forests, long songs, patient seasons',
    tongue: {
      onsets: 'l:3 s:2 th v:2 n:2 m r f c', vowels: 'a:4 e:4 i:3 ae ie o', codas: 'l:3 n:2 s th r', finals: 'l n s th r',
      first: 'ae:1 i:1 e:1 a:1', ends: { months: 'iel:2 el eth:2 ira essa ion anna ys ari ane is ara', people: 'ael iel ith ar an ys ie aen ira', places: 'ir eth:2 iel ara el ith aen orel', moons: 'ae iel ys ira el a' }
    },
    monthFirst: {
      winter: 'Star Frost Silver Snow Hush Moon', spring: 'Dawn Dew Bud Leaf Blossom Lark',
      summer: 'Sun Gold Song Light Bright Glade', autumn: 'Dusk Amber Mist Ember Russet Willow'
    },
    monthLast: { any: 'song tide', winter: 'veil fall mere', spring: 'bloom light turn', summer: 'song glow light', autumn: 'fall wane turn' },
    monthWhole: { winter: 'Starveil Frostsong Silverfall Hushlight', spring: 'Dawnsong Dewbloom Leafturn Larklight', summer: 'Sunglow Goldensong Gladelight Brightmere', autumn: 'Duskfall Amberwane Mistveil Russetfall' },
    weekdayWords: 'Leaf Dew Star Reed Song Ash Moss Wren Mist', weekdayRest: 'Stillsong',
    seasons: {
      winter: 'The Starwatch|Silverfrost|The Hush', spring: 'Budding|The Waking|Dawnsong', summer: 'Zenith|The Long Light|Goldenseason',
      autumn: 'Waning|The Amber|Dusksong', wet: 'The Rains|The Weeping', dry: 'The Dry|The Thirst'
    },
    festivals: {
      winterSolstice: 'The Starwatch|Night of Long Stars|The Silver Vigil', summerSolstice: 'The Long Light|Festival of the High Song|Sunrevel',
      springEquinox: 'The Waking|Bloomrise|Festival of Unfurling Leaves', autumnEquinox: 'Leafturn|The Amber Rite',
      harvest: 'Fruitfall|The Gathering of Seeds', newYear: 'The Return|First Dawn', remembrance: 'Night of Remembered Names|The Silent Song',
      rains: 'The Weeping Rite', dry: 'The Waiting', moon: "Night of {moon}|{moon}'s Song|The {moon} Revel"
    },
    intercalary: 'The Return@winter|First Dawn@spring|The Silent Song|Starwatch@winter|The Long Light@summer|Leafturn@autumn',
    eraNouns: 'Stars Songs Boughs Dawns Leaves Mirrors Seeds', eraAdjs: 'Long Silver Last Silent First Fading Second',
    eras: {
      founding: 'The First Dawn|Age of Stars|The Seeding', golden: 'The Long Song|Age of the Silver Boughs|The High Song',
      strife: 'The Sundering|The Thorn Wars|The Parting', dark: 'The Fading|The Silent Years|The Long Winter',
      restoration: 'Cycles of the Return|The Second Dawn', faith: 'Age of Mirrors|The Vigil Years', discovery: 'The Far Singing|Age of Paths',
      decline: 'The Dwindling|The Last Dawn', present: 'Cycles of the Return|The Waning Count|The Late Song'
    },
    moonWords: 'Silverlamp Dewdrop Wanderer Mirror Pearl Veil',
    placeFirst: 'Silver Star Dawn Moon Willow Mist Glimmer Leaf Swan Ash Elder Rowan Lark Dew', placeLast: 'glade dell mere fall wood reach bough vale spire song',
    given: '', family: '', titles: 'Lady|Lord|Speaker|Songkeeper|Warden|Seer|Elder'
  },

  desert: {
    label: 'Desert', blurb: 'Wells, caravans, glass and glare',
    tongue: {
      onsets: 'q kh:2 z:2 s:2 sh:2 r:2 m:2 n:2 h d t b j y', vowels: 'a:4 i:3 u e:2 aa', codas: 'r:2 n:2 m h z d l', finals: 'r n m h z d l',
      first: 'a:1', ends: { months: 'ah:2 ir un ez im ar aan iya uz et', people: 'a:2 ir an ez im ra ah ul', places: 'ah:2 ir an ez im ara ush et', moons: 'a u i' }
    },
    monthFirst: {
      winter: 'Star Well Palm Cold Lamp', spring: 'Wind Caravan Bloom Rain Green',
      summer: 'Scorch Glass Sun Glare Furnace Blaze', autumn: 'Amber Date Salt Dust Copper Harvest'
    },
    monthLast: { any: 'moon month', winter: 'well rest water', spring: 'wind road dawn', summer: 'fire glare road', autumn: 'dusk fall wind' },
    monthWhole: {
      winter: 'Starwell Palmrest Coldwater Wellmoon Lampmonth', spring: 'Windroad Caravanmoon Rainwell Greenwind Bloomdawn',
      summer: 'Scorchmonth Glassfire Sunglare Furnacemoon Blazeroad', autumn: 'Amberdusk Saltwind Dustfall Coppermoon Datefall'
    },
    weekdayWords: 'Well Palm Salt Dune Star Lamp Tent Spice Brass Tile Rope Jar', weekdayRest: 'Shadeday Wellday',
    seasons: {
      winter: 'The Cool|Starseason|The Lamp Months', spring: 'The Winds|Caravan Season', summer: 'The Burning|High Glare|The Furnace',
      autumn: 'The Amber|Datetime|The Salt Months', wet: 'The Rains|Flood Season', dry: 'The Dry|The Burning'
    },
    festivals: {
      winterSolstice: 'Night of the Cold Stars|The Long Watch|Festival of Lamps', summerSolstice: 'Day of No Shadow|The Glare|Noon of Noons',
      springEquinox: 'The Caravans Depart|Wind-greeting|Festival of the First Well', autumnEquinox: 'Return of the Caravans|The Salt Market',
      harvest: 'Date Harvest|The Gathering of Wells|Night of Full Granaries', newYear: 'First Water|The Opening of the Wells',
      remembrance: 'Night of Lamps|The Silent Dunes', rains: 'Rain-dancing|The Flooding of the Wadis', dry: 'The Sealing of the Cisterns',
      moon: "Night of {moon}|{moon}'s Lamp|{moon} Market"
    },
    intercalary: 'First Water@winter|Night of Lamps@winter|The Still Noon@summer|Well-day|The Silent Dunes@autumn',
    eraNouns: 'Wells Brass Caravans Lamps Salt Glass Dunes Spices', eraAdjs: 'Seven Long Burning Bitter Golden Dry Second',
    eras: {
      founding: 'Age of Wells|The First Caravan|The Digging', golden: 'Age of Brass|The Long Caravan|The Spice Years',
      strife: 'The Sand Kings|The Well Wars|The Salt Feuds', dark: 'The Drought|The Burning|The Glass Years',
      restoration: 'The Second Wells|The Refilling', faith: 'Age of Lamps|The Pilgrim Years', discovery: 'The Far Road|Age of Charts',
      decline: 'The Drying|The Long Thirst', present: 'Age of the Seven Wells|The Long Caravan|The Well-count'
    },
    moonWords: 'Lamp Pearl Brass Salt Dune Wanderer Coin',
    placeFirst: 'Salt Glass Dune Palm Star Bitter Sweet Red White Copper Brass Seven Thorn Scorpion Jackal', placeLast: ' Well| Wells| Spring| Gate| Rock| Oasis| Dunes| Crossing| Market',
    given: '', family: '', titles: 'Caravan-master|Well-keeper|Elder|Spice-merchant|Warden|Water-judge'
  },

  imperial: {
    label: 'Imperial', blurb: 'Legions, edicts and marble',
    tongue: {
      onsets: 'c:2 t:2 m:2 l:2 s:2 v:2 pr tr qu d n r g f br cl st', vowels: 'a:3 e:2 i:2 o:2 u au ae ia io', codas: 'n:2 r:2 s:2 x l m nt', finals: 'n r s x l',
      ends: { months: 'ian:2 ius ine ara ex ium or ana ent eus ora', people: 'us:2 ia a ius an ex o ina', places: 'ium:2 ia ara ona os um ica', moons: 'a:2 is us' }
    },
    monthFirst: { winter: 'Iron Pale Ice Watch', spring: 'Laurel Crown Banner Edict', summer: 'Triumph Sun Gold Eagle', autumn: 'Tribute Harvest Ash Census' },
    monthLast: { any: 'month tide', winter: 'watch moot', spring: 'march', summer: 'feast reign', autumn: 'tide moot' },
    monthWhole: { winter: 'Ironmonth Palewatch', spring: 'Laurelmarch Bannermonth Edicttide', summer: 'Triumphfeast Goldenreign Eaglemonth', autumn: 'Tributetide Censusmonth Ashmoot' },
    weekdayWords: 'Crown Law Coin Sword Scroll Temple Forum Shield Market Seal', weekdayRest: 'Hearthday Templeday', weekdaySuffix: 'day',
    seasons: {
      winter: 'The Iron Season|Winter Quarters', spring: 'Campaign Season|The Laurel', summer: 'High Triumph|The Golden Season',
      autumn: 'Tribute Season|The Census', wet: 'The Rains|The Flood Levy', dry: 'The Dry|The Dust Levy'
    },
    festivals: {
      winterSolstice: "Feast of the Sun's Return|Night of the Seven Braziers", summerSolstice: 'Triumph of the Sun|The Great Games',
      springEquinox: 'Feast of the Laurel|March of Banners', autumnEquinox: 'Tribute Day|The Census',
      harvest: 'Feast of the Granaries|Tribute Feast', newYear: 'The Accession|First Edict|The Swearing',
      remembrance: 'Day of the Honoured Dead|The Ashen Rite', rains: 'The Blessing of the Aqueducts', dry: 'The Water Levy',
      moon: "Night of {moon}|{moon}'s Games"
    },
    intercalary: 'The Accession@winter|Games Day@summer|The Census@autumn|The Swearing@spring|Edict Day',
    eraNouns: 'Legions Crowns Edicts Eagles Laurels Roads Standards', eraAdjs: 'Iron Long Sundered Second Silver Broken Gilt',
    eras: {
      founding: 'The Founding|Age of Roads|The First Standard', golden: 'The Iron Peace|The Long Accord|Age of Laurels',
      strife: 'The Sundered Crowns|The Crown Wars|The Year of Four Standards', dark: 'The Burning of the Archives|The Plague Levy|The Grey Legions',
      restoration: 'The Restoration|The Second Standard', faith: 'Age of Temples|The Edict of Lamps', discovery: 'Age of Roads|The Far Provinces',
      decline: 'The Long Decline|The Last Legions', present: 'The Iron Peace|Reckoning of the Crown|Years of the Accord'
    },
    moonWords: 'Eagle Coin Laurel Standard Crown Lamp',
    placeFirst: '', placeLast: '',
    given: '', family: '', titles: 'Legate|Prefect|Magistrate|Tribune|Consul|Censor|Governor'
  },

  fey: {
    label: 'Fey', blurb: 'Twilight courts, bargains, bright woods',
    tongue: {
      onsets: 'l:2 s:2 f m n th w p b r v sh gl fl tw', vowels: 'i:3 e:2 a:2 ee o ie y', codas: 'l:2 ll n m s sh th', finals: 'l n s th',
      ends: { people: 'i elle ie ling ay ow in ette', places: 'wick ling ow ay', moons: 'i ie ay o' }
    },
    monthFirst: {
      winter: 'Hush Frost Hollow Still Elder Snow', spring: 'Dew Clover Blossom Starling Bud Primrose',
      summer: 'Honey Moth Glimmer Rose Foxglove Lark', autumn: 'Bramble Hazel Rowan Fox Thistle Acorn Amber'
    },
    monthLast: { any: 'song tide', winter: 'hush veil fall dream', spring: 'dance wake bloom ring', summer: 'revel light glow dream', autumn: 'wane ring fall dance' },
    monthWhole: {
      winter: 'Hushfall Frostveil Hollowhush Stillsong Eldertide', spring: 'Dewdance Cloverwake Blossomring Starlingsong',
      summer: 'Honeyrevel Mothlight Glimmerfall Rosedream Larkglow', autumn: 'Bramblewane Hazelring Rowanfall Foxglow Thistlewane Acorndance'
    },
    weekdayWords: 'Dew Moss Moth Fern Thorn Wisp Hush Glim Burr Reed Wren Bee', weekdayRest: 'Revelday Hushday',
    seasons: {
      winter: 'The Hush|Frostveil|The Sleeping Court', spring: 'The Unfurling|Dewtime', summer: 'The Long Revel|Glimmerseason',
      autumn: 'The Bramble|The Waning Court', wet: 'The Rains|The Dripping', dry: 'The Dry|The Thirsting'
    },
    festivals: {
      winterSolstice: 'The Hush|Night of Stilled Revels|Frost-court', summerSolstice: 'The Long Revel|Night of Open Doors|The Glimmer Dance',
      springEquinox: 'Dewdance|The Unfurling|First Blossom Revel', autumnEquinox: 'Bramble Feast|The Turning Dance',
      harvest: 'Bramble Feast|Honey Gathering|The Last Fruit', newYear: 'The Ring Opens|First Dew',
      remembrance: 'Night of Borrowed Names|The Quiet Glade', rains: 'The Puddle Court', dry: 'The Thirsting Dance',
      moon: "{moon}'s Revel|Night of the {moon} Ring|Dance of {moon}"
    },
    intercalary: 'The Ring Opens@spring|Night of Open Doors@summer|The Hush@winter|Borrowed Day|The Quiet Glade@autumn',
    eraNouns: 'Rings Thorns Bargains Courts Glamours Masks', eraAdjs: 'Long Twilight Stolen Silver Laughing Broken Second',
    eras: {
      founding: 'The First Bargain|Age of Rings', golden: 'The Long Revel|The Twilight Courts|Age of Masks',
      strife: 'Age of Thorns|The Court Wars|The Broken Bargain', dark: 'The Unweaving|The Stolen Summer|The Iron Years',
      restoration: 'The Second Bargain|The Re-weaving', faith: 'The Oath-years|Age of Glamours', discovery: 'The Open Doors|Age of Paths',
      decline: 'The Fading Glamour|The Thinning', present: 'The Twilight Reckoning|Age of Masks|The Moth-count'
    },
    moonWords: 'Thimble Button Wisp Mirror Glimmer Pearl Dewdrop',
    placeFirst: 'Thistle Moth Dew Glimmer Briar Fox Hazel Rowan Bramble Honey Wisp Moon Fern Willow Nettle Clover Lark', placeLast: 'hollow dell ring mere down dingle glen bower hill knoll brook',
    given: 'Bramble Mirelle Nettle Pip Quill Rue Sorrel Tansy Thistle Wick Yew Linnet Moth Hazel Briony Wisp', family: '',
    titles: 'Lady~f|Lord~m|the Grey|the Laughing|Keeper|Knight of Thorns|the Bargainer'
  },

  grim: {
    label: 'Grim', blurb: 'Plague towns, gallows and long winters',
    tongue: {
      onsets: 'v:2 m d dr g gr k kr s sk th r z n b', vowels: 'o:3 a:2 u:2 e ae', codas: 'rn sk th g m k ll rr', finals: 'rn sk th g m k ll',
      ends: { people: 'a on ek us ra ane', places: 'ask oth umn orr ek', moons: 'a o us' }
    },
    monthFirst: {
      winter: 'Bone Pale Shroud Bleak Marrow Grave Frost Hollow', spring: 'Rot Rust Crow Mourn Gallow Thorn',
      summer: 'Ash Soot Blight Carrion Cinder Fly', autumn: 'Wither Knell Raven Dirge Rook Pyre'
    },
    monthLast: { any: 'month tide', winter: 'watch fast frost mere', spring: 'march wake moot', summer: 'wane moot', autumn: 'fall moot watch' },
    monthWhole: {
      winter: 'Bonewatch Palefast Shroudmoot Bleakmere Marrowfrost Gravewake', spring: 'Rustmarch Crowmonth Mourntide Rotwake Gallowmarch',
      summer: 'Ashwane Soottide Blightmonth Cindermoot Flymonth', autumn: 'Witherfall Knelltide Ravenwake Dirgemoot Rookfall Pyrewatch'
    },
    weekdayWords: 'Ash Bone Crow Rot Rust Soot Knell Pall Dirge Tallow Nail Wick', weekdayRest: 'Mournday Shutday',
    seasons: {
      winter: 'The Long Dark|Bonewinter|The Pall', spring: 'The Rot|The Thaw of Bones|Mourning-thaw', summer: 'The Blight|Ash Season|The Fly-months',
      autumn: 'The Withering|Knell Season|The Culling', wet: 'The Rains|The Rot', dry: 'The Dry|The Blight'
    },
    festivals: {
      winterSolstice: 'The Long Dark|Vigil of Ashes|Night of Shut Doors', summerSolstice: 'Day of Short Shadows|Pyre Night',
      springEquinox: 'The Unburying|Mourning-thaw|The Salt-circle', autumnEquinox: 'The Culling|Knell Day',
      harvest: 'The Culling|Tithe Day|The Last Cart', newYear: 'The Reckoning|Door-shutting',
      remembrance: 'Night of Names|The Pale Vigil|The Bell-count', rains: 'The Drowning Rite', dry: 'The Dust Vigil',
      moon: "Night of {moon}|{moon}'s Vigil|The {moon} Wake"
    },
    intercalary: 'The Pale Vigil@winter|The Bell-count|Door-shutting@winter|The Reckoning@autumn|Night of Names@autumn',
    eraNouns: 'Ash Gallows Crows Bells Pyres Shrouds Plagues', eraAdjs: 'Grey Hollow Pale Long Broken Black Second',
    eras: {
      founding: 'The Walling|Age of Gallows|The First Pyres', golden: 'The Brief Summer|The Bell Years',
      strife: 'The Hollow Crown|The Crow Wars|The Burning Years', dark: 'The Grey Years|The Plague-count|The Long Mourning',
      restoration: 'The Unburying|The Second Walling', faith: 'Age of Bells|The Penitent Years', discovery: 'The Barrow-breaking|Age of Salt',
      decline: 'The Withering|The Pale Reign', present: 'The Grey Reckoning|Age of Ash|The Pale Count'
    },
    moonWords: 'Skull Pale Widow Lantern Cinder Hollow Crow',
    placeFirst: 'Gallow Crow Bone Ash Soot Raven Rot Wither Mourn Grey Black Pale Rook Barrow Knell Hang', placeLast: 'moor fen wick mere hollow cross marsh tor end pit field',
    given: 'Agath~f Bram~m Cuthred~m Dorran~m Esk Galt~m Hesper~f Ida~f Jory~m Merrin Oda~f Silas~m Ulric~m Wendel~m Crispin~m Grette~f Hobb~m Ansel~m Mabyn~f Tobias~m Sabeth~f', family: 'Ashgrave Blackmoor Crowe Dunmourn Gallowglass Graves Hollin Kettleby Marrow Morrow Pall Sexton',
    titles: 'Gravedigger|Sexton|Hangman|Old|Mother~f|Brother~m|Warden|Plague-warden'
  }
};

/* Names that belong to published settings or to real calendars, faiths and famous places. A generated
   name that matches (whole name, or one word of it) is thrown away. Not exhaustive; it catches the
   names people would recognise at the table. */
var BLOCKED_NAMES = (
  // Forgotten Realms / D&D
  'hammer alturiak ches tarsakh mirtul kythorn flamerule eleasis eleint marpenoth uktar nightal greengrass highharvestide shieldmeet ' +
  'selune selûne sehanine mystra lathander tymora shar bane tempus chauntea silvanus oghma helm tyr torm kelemvor umberlee talos auril malar ' +
  'sune corellon moradin dumathoin gruumsh lolth vecna pelor heironeous hextor nerull boccob olidammara ehlonna kord bahamut tiamat asmodeus ' +
  'faerun toril waterdeep neverwinter candlekeep cormyr calimshan chult thay menzoberranzan greyhawk oerth eberron khorvaire sharn krynn ansalon ' +
  'athas mystara ravenloft barovia strahd sigil planescape spelljammer harptos celene solinari lunitari nuitari ' +
  'zarantyr olarune therendor eyre dravago nymm lharvion barrakas rhaan sypheros aryth vult ' +
  // Pathfinder
  'abadius calistril pharast gozran desnus sarenith erastus arodus rova lamashan neth kuthona moonday toilday wealday oathday fireday starday ' +
  'absalom varisia cheliax andoran golarion sandpoint magnimar korvosa somal ' +
  // Critical Role
  'horisal misuthar dualahei thunsheer unndilar brussendar sydenstar fessuran cuersaar duscar miresen grissen whelsen conthsen folsen yulisen ' +
  'catha ruidus exandria wildemount emon whitestone ' +
  // Elder Scrolls
  'sundas morndas tirdas middas turdas fredas loredas masser secunda tamriel skyrim morrowind cyrodiil nirn hearthfire frostfall ' +
  // Warhammer
  'morrslieb mannslieb sigmar altdorf hexenstag nachexen jahrdrung mitterfruhl pflugzeit sigmarzeit sommerzeit sonnstill vorgeheim geheimnistag ' +
  'nachgeheim erntezeit brauzeit kaldezeit ulriczeit mondstille primaris ' +
  // Tolkien
  'ithil anor isil arda eru iluvatar valar valinor gondor mordor rohan shire rivendell imladris lothlorien lorien moria khazad erebor mirkwood ' +
  'eriador numenor beleriand elbereth varda earendil galadriel elrond arwen legolas gandalf sauron morgoth balrog mithril afteryule solmath rethe ' +
  'astron thrimidge forelithe afterlithe wedmath halimath winterfilth blotmath foreyule lithe durin thorin balin dwalin gimli gloin bombur ' +
  'thrain thror fili kili celeborn thranduil luthien beren feanor fingolfin finrod turgon glorfindel cirdan haldir manwe ulmo yavanna ' +
  'nienna orome tulkas mandos irmo este aule elwing idril tuor turin maedhros celebrimbor ithilien osgiliath minas arien tilion eldamar ' +
  'tirion doriath gondolin nargothrond elendil isildur anarion aragorn arathorn silmaril lembas athelas mallorn telperion laurelin eldar ' +
  'quendi sindar noldor teleri vanyar avari morwen oakenshield ' +
  // other fiction
  'westeros winterfell braavos valyria belleteyn saovine novigrad temeria velen andor aiel perrin rand ' +
  // real months, weekdays and calendar words
  'january february march april may june july august september october november december monday tuesday wednesday thursday friday saturday sunday ' +
  'ianuarius januarius februarius martius aprilis maius iunius junius quintilis sextilis iulius julius augustus kalends nones ides ' +
  'muharram safar rabi jumada rajab shaban ramadan shawwal tishrei cheshvan kislev tevet shevat adar nisan iyar sivan tammuz elul ' +
  'thoth phaophi athyr choiak tybi mechir phamenoth pharmuthi pachon payni epiphi mesore vendemiaire brumaire frimaire nivose pluviose ' +
  'ventose germinal floreal prairial messidor thermidor fructidor yule samhain beltane imbolc lughnasadh lammas candlemas michaelmas ' +
  'martinmas easter christmas hanukkah diwali halloween walpurgis ostara litha mabon sabbath ' +
  // real gods, moons and famous places
  'odin thor freya loki zeus apollo athena amun amon aten anubis osiris isis horus ptah bastet sekhmet hathor sobek allah baal marduk ishtar ' +
  'inanna jupiter juno minerva mars venus mercury vesta janus saturn diana ceres bacchus pluto vulcan neptune selene luna artemis phoebe ' +
  'europa ganymede callisto phobos deimos titan ' +
  'sahara cairo medina mecca baghdad damascus petra marrakesh timbuktu tunis giza luxor thebes memphis kemet sheba babylon nineveh sumer ' +
  'akkad roma rome latium ostia pompeii capua verona caesar nero hadrian trajan valencia coruna aurelia khazar khazars israel canaan gaza'
).split(/\s+/).filter(Boolean);
var BLOCKED_SET = {};
BLOCKED_NAMES.forEach(function (n) { BLOCKED_SET[n] = 1; });
/* Fragments no invented word may contain: slurs, obscenities and words that read wrongly at a table. */
var BLOCKED_BITS = ['anus', 'rape', 'nazi', 'cum', 'fag', 'shit', 'tit', 'porn', 'dick', 'cock', 'cunt', 'fuck', 'piss', 'slut', 'whore',
  'nig', 'kkk', 'poo', 'butt', 'sex', 'fart', 'turd', 'semen', 'penis', 'vagin', 'boob', 'bitch', 'wank', 'twat', 'prick', 'crap', 'spic',
  'kike', 'chink', 'gook', 'jap', 'dyke', 'homo', 'jew', 'gay', 'isis', 'hitler', 'slave', 'pube', 'anal', 'orgy', 'nipple', 'puke', 'dung', 'pee'];

/* ── Naming ─────────────────────────────────────────────────────────────────────────────────────────
   One set of names shares a voice: its months all use the same kind of word (compounds or invented), its
   invented words share a small family of endings, and no first element or name repeats. Month names are
   chosen for the season each month falls in. */

/* Word lists: "Word", "Word:2" (a weight) or "Word~f" / "Word~m" (a given name or title that is a woman's or
   a man's, so "Brother" never goes with "Hester"). */
function toks(str) {
  return String(str || '').split(/\s+/).filter(Boolean).map(function (t) {
    var gm = /~([fm])$/.exec(t), g = gm ? gm[1] : null;
    if (gm) t = t.slice(0, -2);
    var i = t.lastIndexOf(':');
    return (i > 0 && /^\d+(\.\d+)?$/.test(t.slice(i + 1))) ? { v: t.slice(0, i), w: +t.slice(i + 1), g: g } : { v: t, w: 1, g: g };
  });
}
function phrases(str) { return String(str || '').split('|').map(function (s) { return s.trim().replace(/~[fm]$/, ''); }).filter(Boolean); }
function genderedPhrases(str) { return String(str || '').split('|').map(function (s) { s = s.trim(); var m = /~([fm])$/.exec(s); return { v: m ? s.slice(0, -2) : s, g: m ? m[1] : null }; }).filter(function (x) { return x.v; }); }
function pickTok(rng, list) { var t = rng.weighted(list, function (x) { return x.w; }); return t ? t.v : ''; }

var THEME_CACHE = {};
function themeData(id) {
  if (THEME_CACHE[id]) return THEME_CACHE[id];
  var th = THEMES[id];
  if (!th) throw GenError('There’s no naming theme called “' + id + '”. Try ' + joinList(Object.keys(THEMES)) + '.');
  var T = th.tongue, ends = {};
  ownKeys(T.ends).forEach(function (k) { ends[k] = toks(T.ends[k]); });
  var d = {
    id: id, raw: th,
    tongue: {
      onsets: toks(T.onsets), vowels: toks(T.vowels), codas: toks(T.codas), finals: toks(T.finals || T.codas),
      medial: toks(T.codas).filter(function (c) { return /^(l|r|n|m|s)$/.test(c.v); }),
      first: T.first ? toks(T.first) : null, syl: toks(T.syl || '1:1 2:4 3:2'), ends: ends
    },
    first: {}, last: {}, whole: {}
  };
  SEASON_TYPES.forEach(function (s) {
    d.first[s] = toks(th.monthFirst[s]).map(function (t) { return t.v; });
    d.last[s] = toks((th.monthLast[s] || '') + ' ' + th.monthLast.any).map(function (t) { return t.v; });
    d.whole[s] = toks(th.monthWhole[s]).map(function (t) { return t.v; });
  });
  THEME_CACHE[id] = d;
  return d;
}

var DIGRAPHS = { th: 1, sh: 1, kh: 1, ch: 1, st: 1, nd: 1 };
function tidyWord(w) {
  w = w.toLowerCase().replace(/(.)\1\1+/g, '$1$1').replace(/([aeiou])([aeiou])[aeiou]+/g, '$1$2').replace(/yy/g, 'y');
  // Break three-consonant pile-ups unless they are a known pair plus one.
  w = w.replace(/([^aeiouy])([^aeiouy])([^aeiouy])/g, function (m, a, b, c) { return DIGRAPHS[a + b] || DIGRAPHS[b + c] ? m : a + c; });
  return w;
}
var HARD_TONGUES = { dwarven: 1, grim: 1, desert: 1 };
/* Spellings one letter away from famous real names (Sahara, Khazar...), which the exact list can't catch. */
var NEAR_REAL = [/(^|[^a-z])[sz]a+h?h?ara/, /khaz[ae]r/, /^rom[ae]$/, /^atlant/, /^olymp/, /^egyp/, /^pers[ie]a/];
var KIND_LEN = { weekdays: [3, 5], moons: [4, 7], months: [5, 9], places: [4, 9], people: [4, 8], word: [3, 9] };
/* Invented words that turn out to be ordinary English read as mistakes ("Them", "Tom"), so they go. */
var COMMON_WORDS = {};
('the them then than they this that these those there their what when where which while with will would were was wasnt ' +
 'have has had having here hers him his her its our ours out own over under upon unto into onto once only other some such than ' +
 'too very can cant could did does done dont each few for from get got going gone good great had how just like made make many ' +
 'may more most much must near next nor not now off old one ones out same see seen shall she should since sit sat set tell ten ' +
 'tom tim sam ben dan ann anna bob jim jon kim ron ted tina lisa mark matt nick paul rob sara sean seth tony will ' +
 'go goes gone bam bum bus cab cat dog dot fan fat fig fin fit fun gag gap gas gel gem gin gum gun hat hen hit hog hot hug hut ' +
 'jam jar jet jog jug kid kin kit lab lad lag lap law lay led leg let lid lip lit log lot low mad man map mat men met mix mob ' +
 'mom mop mud mug nap net nod nut oak odd oil orb pad pal pan pat paw pay pen pet pie pig pin pit pod pop pot pub pun pup put ' +
 'rag ram ran rat raw red rib rid rim rip rod rot row rub rug rum run sad sag sap saw say sea sin sip sir sit six ski sky sob ' +
 'son sow spa spy sub sum sun tab tag tan tap tar tax tea tie tin tip toe ton top toy tub tug van vat vet vow wag war wax web ' +
 'wed wet who why wig win wit won yak yam yes yet zip zoo bake band bank barn base bath bean bear beat bell belt bend best ' +
 'bite blue boat body bone book boot born boss bowl burn busy cake call calm came camp card care case cash cave cell chip city ' +
 'clay club coal coat code cold come cook cool copy core corn cost crew crop cure dame damp dare dark data date dead deal dear ' +
 'debt deck deed deep deer desk dial diet dine dirt dish dive dock doll dome door dose dove down drag draw drew drop drum dual ' +
 'duck duke dull dump dune dust duty earn ease east easy edge else even ever evil exit face fact fade fail fair fake fall fame ' +
 'farm fast fate fear feed feel feet fell felt file fill film find fine fire firm fish fist flag flat fled flew flip flow foam ' +
 'fold folk fond food fool foot form fort foul four free frog fuel full fund fury fuse gain gala gale game gate gave gear gift ' +
 'girl give glad glow glue goal goat gold golf gone gown grab gray grew grey grid grim grin grip grow gulf guru hail hair half ' +
 'hall halt hand hang hard harm hate haul have head heal heap hear heat heel held hell helm help herb herd hero hide high hike ' +
 'hill hint hire hold hole holy home hood hook hope horn host hour huge hung hunt hurt idea iron item jail jazz joke jury keen ' +
 'keep kept kick kill kind king kiss knee knew knot know lace lack lady laid lake lamb lamp land lane last late lava lawn lazy ' +
 'lead leaf lean leap left lend lens less liar life lift lime line link lion list live load loan lock loft lone long look loop ' +
 'lord lose loss lost loud love luck lung lure lush made mail main male mall malt many mare mask mass mast mate maze meal mean ' +
 'meat meet melt memo menu mere mesh mess mild mile milk mill mind mine mint miss mist mode mole mood moon more moss most moth ' +
 'move much mule muse must myth nail name navy neat neck need nest news nice nine node none noon norm nose note noun oath obey ' +
 'omen once oval oven pace pack page paid pain pair pale palm park part pass past path peak pear peel peer pest pick pile pill ' +
 'pine pink pipe plan play plot plow plug plum plus poem poet pole poll pond pony pool poor pope pork port pose post pour pray ' +
 'prey pull pump pure push race rack rage raid rail rain rank rare rash rate read real rear rely rent rest rice rich ride ring ' +
 'rise risk road roam roar robe rock rode role roll roof room root rope rose ruby rude ruin rule rush rust safe saga sage said ' +
 'sail sake sale salt sand sang save seal seam seat seed seek seem seen self sell send sent shed ship shoe shop shot show shut ' +
 'sick side sigh sign silk sing sink site size skin slam slap slid slim slip slot slow snap snow soak soap soar sock soda sofa ' +
 'soft soil sold sole some song soon sore sort soul soup sour span spin spot star stay stem step stir stop such suit sung sunk ' +
 'sure swan swap sway swim tail take tale talk tall tame tank tape task team tear tell tend tent term test text than tide tidy ' +
 'tile till time tiny tire toll tomb tone took tool toss tour town trap tray tree trim trip true tube tuck tune turn twin type ' +
 'unit urge used user vain vale vary vase vast veil vein verb vest veto vice view vine visa void vote wade wage wait wake walk ' +
 'wall wand want ward warm warn wary wash wave weak wear weed week well went west what whip wide wife wild wind wine wing wire ' +
 'wise wish wolf wood wool word wore work worm wrap yard yarn year yell zero zone bore bard lore mora sora tora nora vera dora ' +
 'lena mona rosa nina gina lola lulu lala tara kara zara lara mara sara dana hana jana lana rana zelda thor loki mira vena ' +
 'sinus virus bonus focus genus status census cactus locus campus circus chorus fungus opus torus venus minus plus nexus lotus suzy susie').split(/\s+/).forEach(function (w) { COMMON_WORDS[w] = 1; });
function acceptableWord(w, kind) {
  var L = KIND_LEN[kind] || KIND_LEN.word;
  if (w.length < L[0] || w.length > L[1]) return false;
  if (BLOCKED_SET[w] || COMMON_WORDS[w]) return false;
  for (var i = 0; i < BLOCKED_BITS.length; i++) if (w.indexOf(BLOCKED_BITS[i]) >= 0) return false;
  if (/^[^aeiouy]{3}/.test(w) && !/^(thr|str|shr|spr|scr|skr)/.test(w)) return false;
  if (/[^aeiouy]{3}$/.test(w)) return false;
  if (/(q[^u]|q$|[jvwh]$|^x|[aeiou]{2}h$)/.test(w)) return false;
  if (/(.{2,3})\1/.test(w) && w.length < 7) return false; // "lala", "dordor"
  if (/([a-z]{2})[a-z]?\1/.test(w)) return false;           // "Fanfafen"
  if (w.length <= 8 && /([^aeiouy]).*\1.*\1/.test(w)) return false; // "Mumema"
  for (var r = 0; r < NEAR_REAL.length; r++) if (NEAR_REAL[r].test(w)) return false;
  var dip = w.match(/[aeiouy]{2}/g);
  if (dip && dip.length > 1) return false;                   // "Caecaen": one vowel pair per word is plenty
  if (!/[aeiouy]/.test(w)) return false;
  return true;
}
/* An invented word in the theme's tongue. endFamily (optional) keeps a set's words sounding related. */
function tongueWord(td, rng, kind, endFamily) {
  var T = td.tongue;
  for (var attempt = 0; attempt < 80; attempt++) {
    // Most names are two syllables; three reads as a mouthful at the table, so it stays rare.
    var n = kind === 'weekdays' ? 1 : kind === 'moons' ? 2 : kind === 'people' ? (rng.chance(0.85) ? 2 : 3)
      : (kind === 'months' || kind === 'places') ? (rng.chance(0.85) ? 2 : 3) : +pickTok(rng, T.syl);
    var w = '';
    for (var s = 0; s < n; s++) {
      var onset = (s === 0 && T.first && rng.chance(0.3)) ? '' : pickTok(rng, T.onsets);
      var vowel = (s === 0 && T.first && onset === '') ? pickTok(rng, T.first) : pickTok(rng, T.vowels);
      // Inside a word only soft consonants close a syllable; the harder finals wait for the end.
      var closeP = kind === 'weekdays' ? (HARD_TONGUES[td.id] ? 1 : 0.7) : 0.6;
      var coda = s === n - 1 ? (rng.chance(closeP) ? pickTok(rng, T.finals) : '') : (rng.chance(0.25) && T.medial.length ? pickTok(rng, T.medial) : '');
      w += onset + vowel + coda;
    }
    var fam = endFamily || T.ends[kind];
    if (fam && fam.length && kind !== 'weekdays' && rng.chance(endFamily ? 0.85 : (kind === 'moons' || kind === 'people' ? 0.8 : 0.6))) {
      var stem = w.replace(/[aeiouy]+[^aeiouy]*$/, '');
      if (stem.length >= 1) w = stem + (typeof fam[0] === 'string' ? rng.pick(fam) : pickTok(rng, fam));
    }
    w = tidyWord(w);
    if (acceptableWord(w, kind)) return cap(w);
  }
  return null;
}
function joinCompound(first, last) {
  var a = first, b = last.toLowerCase();
  if (a.slice(-1).toLowerCase() === b.charAt(0)) return null;          // "Frosttide"
  if (/[aeiouy]$/i.test(a) && /^[aeiouy]/.test(b)) return null;       // "Honeyebb"
  var w = a + b;
  return w.length <= 12 ? w : null;
}
function firstElement(td, name) {
  var lower = name.toLowerCase(), best = '';
  SEASON_TYPES.forEach(function (s) { td.first[s].forEach(function (f) { if (lower.indexOf(f.toLowerCase()) === 0 && f.length > best.length) best = f; }); });
  return (best || name.slice(0, 4)).toLowerCase();
}
function lastElement(td, name) {
  var lower = name.toLowerCase(), best = '';
  SEASON_TYPES.forEach(function (s) { td.last[s].forEach(function (l) { if (lower.slice(-l.length) === l && l.length > best.length) best = l; }); });
  return best;
}
function isFree(used, name) { return !!name && !used[name.toLowerCase()] && !BLOCKED_SET[name.toLowerCase()]; }
function take(used, name) { if (name) used[name.toLowerCase()] = 1; return name; }

/* Month names. types: one season type per month ('winter'...'autumn'; wet/dry allowed); intercalary: which
   months are festival days between months. style: 'words' | 'tongue' | 'mixed' (the theme's own habit). */
function nameMonths(td, rng, types, intercalary, style, used, locked) {
  used = used || {};
  var n = types.length, out = [], firsts = {}, lastUse = {}, maxLast = n <= 8 ? 1 : 2;
  var mode = style === 'mixed' || !style ? ({ elven: 'tongue', imperial: 'tongue', desert: rng.chance(0.5) ? 'tongue' : 'words' }[td.id] || 'words') : style;
  var endFamily = null;
  if (mode === 'tongue') {
    var ends = td.tongue.ends.months || td.tongue.ends.places || [];
    endFamily = rng.shuffle(ends.map(function (e) { return e.v; })).slice(0, Math.min(5, Math.max(2, Math.ceil(n / 3))));
  }
  (locked || []).forEach(function (nm) { if (nm) { take(used, nm); firsts[firstElement(td, nm)] = 1; var le = lastElement(td, nm); if (le) lastUse[le] = (lastUse[le] || 0) + 1; } });
  // "Midwinter@winter": a festival day's name may say which time of year it belongs to.
  var feastType = {};
  var feasts = rng.shuffle(phrases(td.raw.intercalary).map(function (f) { var q = f.split('@'); if (q[1]) feastType[q[0]] = q[1]; return q[0]; }));
  var typeOf = function (f) { return feastType[f] || inferSeasonType(f); };
  var ANCHOR_OF = { winter: 'winterSolstice', spring: 'springEquinox', summer: 'summerSolstice', autumn: 'harvest' };
  for (var i = 0; i < n; i++) {
    if (locked && locked[i]) { out.push(locked[i]); continue; }
    if (intercalary && intercalary[i]) {
      // A festival day is named for its place in the year: no "Midsummer" in the depths of winter.
      var st = types[i] === 'wet' ? 'summer' : types[i] === 'dry' ? 'winter' : types[i];
      var fits = feasts.filter(function (f) { var ft = typeOf(f); return isFree(used, f) && (!ft || ft === st); });
      var own = phrases((td.raw.festivals || {})[ANCHOR_OF[st]] || '').filter(function (f) { return isFree(used, f) && f.split(' ').length <= 3 && f.indexOf('{') < 0; });
      var fname = fits.filter(function (f) { return typeOf(f) === st; })[0] || (rng.chance(0.5) && own[0]) || fits[0] || own[0] || nameFestival(td, rng, 'newYear', {}, used);
      out.push(take(used, fname)); continue;
    }
    var t = types[i] === 'wet' ? 'summer' : types[i] === 'dry' ? 'winter' : (types[i] || SEASON_TYPES[i % 4]);
    var name = null;
    if (mode === 'tongue') {
      for (var a = 0; a < 40 && !name; a++) {
        var w = tongueWord(td, rng, 'months', endFamily);
        if (!w || !isFree(used, w)) continue;
        var o2 = w.slice(0, 2).toLowerCase(), end = endFamily.filter(function (e) { return w.toLowerCase().slice(-e.length) === e; })[0] || '';
        if ((firsts[o2] || 0) >= 2 && a < 35) continue;                         // spread the openings
        if (end && (lastUse[end] || 0) >= Math.ceil(n / 4) && a < 35) continue;  // and the endings
        name = w; firsts[o2] = (firsts[o2] || 0) + 1; if (end) lastUse[end] = (lastUse[end] || 0) + 1;
      }
    } else {
      for (var b = 0; b < 90 && !name; b++) {
        // Long years run out of one season's words: after a while borrow the neighbouring seasons' too.
        var ts = b < 40 ? t : SEASON_TYPES[(SEASON_TYPES.indexOf(t) + (b % 2 ? 1 : 3)) % 4];
        var cand = null;
        if (rng.chance(0.6)) cand = rng.pick(td.whole[ts]);
        else { var f = rng.pick(td.first[ts]), l = rng.pick(b < 60 ? td.last[ts] : td.last[t].concat(td.last[ts])); cand = f && l ? joinCompound(f, l) : null; }
        if (!cand || !isFree(used, cand)) continue;
        var fe = firstElement(td, cand), le2 = lastElement(td, cand);
        if (firsts[fe]) continue;
        if (le2 && (lastUse[le2] || 0) >= maxLast + (b >= 60 ? 1 : 0)) continue;
        name = cand; firsts[fe] = 1; if (le2) lastUse[le2] = (lastUse[le2] || 0) + 1;
      }
    }
    if (!name) name = tongueWord(td, rng, 'months') || ('Month ' + (i + 1));
    out.push(take(used, name));
  }
  return out;
}

/* Weekday names; rest: indexes of rest days. Short, distinct, and with distinct opening letters where
   possible, so their abbreviations still tell them apart. */
function nameWeekdays(td, rng, n, rest, style, used, locked) {
  used = used || {};
  rest = rest || [];
  var th = td.raw, out = [], opens = {};
  var mode = style === 'tongue' ? 'tongue' : style === 'words' ? 'words' : ({ elven: 'tongue', dwarven: rng.chance(0.6) ? 'tongue' : 'words', desert: rng.chance(0.5) ? 'tongue' : 'words' }[td.id] || 'words');
  var suffix = th.weekdaySuffix || (mode === 'words' && rng.chance(0.25) ? 'day' : '');
  var words = rng.shuffle(toks(th.weekdayWords).map(function (t) { return t.v; }));
  var restWords = rng.shuffle(toks(th.weekdayRest).map(function (t) { return t.v; }));
  (locked || []).forEach(function (nm) { if (nm) { take(used, nm); opens[nm.slice(0, 2).toLowerCase()] = 1; } });
  for (var i = 0; i < n; i++) {
    if (locked && locked[i]) { out.push(locked[i]); continue; }
    var name = null;
    if (rest.indexOf(i) >= 0 && mode === 'words') name = restWords.filter(function (r) { return isFree(used, r); })[0] || null;
    for (var a = 0; a < 40 && !name; a++) {
      var base = words[(i + a) % words.length];
      var w = mode === 'tongue' ? tongueWord(td, rng, 'weekdays') : (suffix ? (base.slice(-1).toLowerCase() === suffix.charAt(0) ? null : base + suffix) : base);
      if (!w || !isFree(used, w)) continue;
      if (opens[w.slice(0, 2).toLowerCase()] && a < 30) continue;
      name = w;
    }
    if (!name) name = 'Day ' + (i + 1);
    opens[name.slice(0, 2).toLowerCase()] = 1;
    out.push(take(used, name));
  }
  return out;
}

function nameFestival(td, rng, anchor, vars, used) {
  used = used || {};
  var list = phrases(td.raw.festivals[anchor] || '');
  var free = rng.shuffle(list).map(function (p) { return fill(p, vars || {}); }).filter(function (p) { return p.indexOf('{') < 0 && isFree(used, p); });
  if (free.length) return take(used, free[0]);
  var nouns = toks(td.raw.eraNouns).map(function (t) { return t.v; }), adjs = toks(td.raw.eraAdjs).map(function (t) { return t.v; });
  for (var a = 0; a < 30; a++) {
    var p = rng.pick(['Feast of ' + rng.pick(nouns), rng.pick(adjs) + ' Night', 'Day of ' + rng.pick(nouns), 'The ' + rng.pick(adjs) + ' Fair']);
    if (isFree(used, p)) return take(used, p);
  }
  return take(used, 'Festival ' + (Object.keys(used).length + 1));
}

var ERA_CHARACTERS = ['founding', 'golden', 'strife', 'dark', 'restoration', 'faith', 'discovery', 'decline', 'present'];
function nameEra(td, rng, character, used) {
  used = used || {};
  var list = phrases((td.raw.eras || {})[character] || '');
  var free = rng.shuffle(list).filter(function (p) { return isFree(used, p); });
  if (free.length) return take(used, free[0]);
  var nouns = toks(td.raw.eraNouns).map(function (t) { return t.v; }), adjs = toks(td.raw.eraAdjs).map(function (t) { return t.v; });
  for (var a = 0; a < 40; a++) {
    var p = rng.chance(0.5) ? 'Age of ' + rng.pick(nouns) : 'The ' + rng.pick(adjs) + ' ' + rng.pick(nouns);
    if (isFree(used, p)) return take(used, p);
  }
  return take(used, 'Age ' + roman(Object.keys(used).length + 1));
}
function nameSeason(td, rng, type, used) {
  used = used || {};
  var free = rng.shuffle(phrases(td.raw.seasons[type] || td.raw.seasons.winter)).filter(function (p) { return isFree(used, p); });
  if (free.length) return take(used, free[0]);
  var fallback = { winter: 'Winter', spring: 'Spring', summer: 'Summer', autumn: 'Autumn', wet: 'The Wet', dry: 'The Dry' }[type] || cap(type);
  return take(used, isFree(used, fallback) ? fallback : fallback + ' ' + roman(2));
}
/* How often a culture names a moon with a plain word (Cinder, Hollow) rather than an invented one. */
var MOON_WORDS = { fey: 0.8, dwarven: 0.7, pastoral: 0.7, grim: 0.65, nautical: 0.5, desert: 0.5, elven: 0.35, imperial: 0.3 };
function nameMoon(td, rng, used) {
  used = used || {};
  var words = toks(td.raw.moonWords).map(function (t) { return t.v; });
  for (var a = 0; a < 40; a++) {
    var w = rng.chance(MOON_WORDS[td.id] || 0.5) ? rng.pick(words) : tongueWord(td, rng, 'moons');
    if (w && isFree(used, w)) return take(used, w);
  }
  return take(used, 'Moon ' + roman(Object.keys(used).length + 1));
}
var PLACE_TONGUE = { elven: 0.6, desert: 0.5, imperial: 1, dwarven: 0.12, fey: 0.03, grim: 0.05, nautical: 0.08, pastoral: 0 };
/* Last parts that stand as words on their own, so "Crane Hollow" and "Oyster Quay" can be two words. */
var STANDALONE_LAST = { hollow: 1, cross: 1, green: 1, bridge: 1, well: 1, point: 1, reach: 1, sound: 1, cove: 1, sands: 1, quay: 1, hall: 1, gate: 1, crag: 1, marsh: 1, end: 1, moor: 1, fen: 1, dell: 1, ring: 1, glen: 1, knoll: 1, hill: 1, mere: 1, field: 1 };
function namePlace(td, rng, used) {
  used = used || {};
  var th = td.raw, firsts = toks(th.placeFirst).map(function (t) { return t.v; }), lasts = String(th.placeLast || '').split(th.placeLast && th.placeLast.indexOf('|') >= 0 ? '|' : /\s+/).filter(Boolean);
  for (var a = 0; a < 50; a++) {
    var w = null;
    if (!firsts.length || rng.chance(PLACE_TONGUE[td.id] || 0)) {
      w = tongueWord(td, rng, 'places');
      if (w && td.id === 'imperial' && rng.chance(0.2)) w = rng.pick(['Port ', 'Fort ', 'Castra ']) + w;
    } else {
      var f = rng.pick(firsts), l = rng.pick(lasts);
      if (l.charAt(0) === ' ') w = f + l;
      else if (STANDALONE_LAST[l] && rng.chance(0.2)) w = f + ' ' + cap(l);
      else w = joinCompound(f, l);
    }
    if (w && isFree(used, w)) return take(used, w);
  }
  return take(used, 'Place ' + (Object.keys(used).length + 1));
}
function namePerson(td, rng, used, opts) {
  used = used || {}; opts = opts || {};
  var th = td.raw, givenT = toks(th.given), family = toks(th.family).map(function (t) { return t.v; });
  var titles = genderedPhrases(th.titles);
  for (var a = 0; a < 50; a++) {
    var tt = opts.title === false ? null : (opts.title || rng.chance(0.45)) ? ((opts.title && typeof opts.title === 'string') ? { v: opts.title, g: null } : rng.pick(titles)) : null;
    // A gendered title takes a given name of the same kind, never an invented one whose sound might not fit.
    var pool = tt && tt.g ? givenT.filter(function (x) { return !x.g || x.g === tt.g; }) : givenT;
    var g = pool.length && (tt && tt.g && givenT.length ? true : rng.chance(0.85)) ? rng.pick(pool).v : tongueWord(td, rng, 'people');
    if (!g) continue;
    var fam = family.length && rng.chance(0.6) ? rng.pick(family) : null;
    var name = g + (fam ? ' ' + fam : '');
    var title = tt ? tt.v : null;
    // "the Grey" and "the Bargainer" are epithets: they follow the name ("Rue the Grey").
    var full = !title ? name : /^the /i.test(title) ? g + ' ' + title : / of /.test(title) ? name + ', ' + title : title + ' ' + name;
    if (!isFree(used, full) || used['given:' + g.toLowerCase()]) continue;
    used['given:' + g.toLowerCase()] = 1;
    return take(used, full);
  }
  return take(used, 'Someone ' + (Object.keys(used).length + 1));
}

/* Season types for n months: from the calendar when given, else spread evenly from midwinter. */
function monthSeasonTypes(cal, n) {
  var types = [];
  for (var i = 0; i < n; i++) {
    if (cal && cal.months.length === n) {
      var y = cal.currentYear, m = i + 1, d = Math.max(1, Math.ceil(cal.monthDays(m, y) / 2));
      types.push(cal.seasonTypeAt(y, m, d));
    } else {
      var th = mod((i + 0.5) / n - 0.04, 1);
      types.push(SEASON_TYPES[Math.round(th * 4) % 4]);
    }
  }
  return types;
}

var FESTIVAL_ANCHORS = ['winterSolstice', 'springEquinox', 'summerSolstice', 'autumnEquinox', 'harvest', 'newYear', 'remembrance', 'moon'];
var ANCHOR_WORDS = { winterSolstice: 'midwinter', springEquinox: 'the spring turn', summerSolstice: 'midsummer', autumnEquinox: 'the autumn turn', harvest: 'harvest', newYear: 'the new year', remembrance: 'remembrance', moon: 'a full moon', rains: 'the first rains', dry: 'the dry season' };

defineGenerator({
  id: 'names', label: 'Names', needsCalendar: false,
  blurb: 'Months, weekdays, festivals, eras, moons, seasons, places and people, in one culture’s voice.',
  makes: {
    months: { label: 'Months', type: 'count', default: 12, min: 0, max: 24 },
    weekdays: { label: 'Weekdays', type: 'count', default: 7, min: 0, max: 12 },
    festivals: { label: 'Festivals', type: 'count', default: 6, min: 0, max: 12 },
    eras: { label: 'Eras', type: 'count', default: 5, min: 0, max: 10 },
    moons: { label: 'Moons', type: 'count', default: 3, min: 0, max: 8 },
    seasons: { label: 'Seasons', type: 'count', default: 4, min: 0, max: 6 },
    places: { label: 'Places', type: 'count', default: 8, min: 0, max: 30 },
    people: { label: 'People', type: 'count', default: 6, min: 0, max: 30 }
  },
  scope: {},
  details: {
    theme: { label: 'Culture', type: 'select', default: 'pastoral', options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) },
    style: { label: 'Kind of words', type: 'select', default: 'mixed', options: [{ value: 'mixed', label: 'The culture’s own habit' }, { value: 'words', label: 'Plain-English compounds' }, { value: 'tongue', label: 'Invented words' }], more: true },
    restDays: { label: 'Rest days in the week', type: 'count', default: 1, min: 0, max: 3, more: true },
    avoid: { label: 'Never use', type: 'list', default: [], more: true, help: 'Names or fragments to keep out, one per line.' }
  },
  builtIns: Object.keys(THEMES).map(function (k) { return { id: 'names.' + k, name: THEMES[k].label + ' names', description: THEMES[k].blurb + '.', details: { theme: k } }; }),
  defaultRecipe: 'names.pastoral',
  run: runNames
});

function runNames(opts) {
  var recipe = resolveRecipe('names', opts.recipe);
  var seed = opts.seed == null ? 'chronicle' : String(opts.seed);
  var mk = recipe.makes, dt = recipe.details, td = themeData(dt.theme);
  var cal = opts.calendar ? makeCal(opts.calendar) : null;
  var locked = opts.locked || {};
  var used = {};
  (dt.avoid || []).forEach(function (a) { used[String(a).toLowerCase()] = 1; });
  var avoidBits = (dt.avoid || []).map(function (a) { return String(a).toLowerCase(); });
  var out = {}, R = function (k) { return makeRng(seed, 'names', dt.theme, k); };
  function lockedList(k) { var l = locked[k]; if (!l) return []; if (Array.isArray(l)) return l.slice(); var a = []; ownKeys(l).forEach(function (i) { a[+i] = l[i]; }); return a; }
  function clean(list) { return list.map(function (nm) { return nm; }); }
  if (mk.months) {
    var types = monthSeasonTypes(cal, mk.months);
    var inter = cal && cal.months.length === mk.months ? cal.months.map(function (m) { return m.intercalary; }) : null;
    out.months = clean(nameMonths(td, R('months'), types, inter, dt.style, used, lockedList('months'))).map(function (nm, i) { return { name: nm, season: types[i], intercalary: !!(inter && inter[i]) }; });
  }
  if (mk.weekdays) {
    var rest = [];
    for (var r = 0; r < dt.restDays && r < mk.weekdays; r++) rest.push(mk.weekdays - 1 - r * Math.max(1, Math.floor(mk.weekdays / Math.max(1, dt.restDays))));
    out.weekdays = nameWeekdays(td, R('weekdays'), mk.weekdays, rest, dt.style, used, lockedList('weekdays')).map(function (nm, i) { return { name: nm, rest: rest.indexOf(i) >= 0 }; });
  }
  if (mk.seasons) {
    var stypes = mk.seasons === 2 ? ['wet', 'dry'] : mk.seasons === 3 ? ['spring', 'summer', 'winter'] : SEASON_TYPES.concat(['wet', 'dry']).slice(0, mk.seasons);
    var sr = R('seasons'), ls = lockedList('seasons');
    out.seasons = stypes.map(function (t, i) { return { name: ls[i] ? take(used, ls[i]) : nameSeason(td, sr, t, used), type: t }; });
  }
  if (mk.moons) { var mr = R('moons'), lm = lockedList('moons'); out.moons = []; for (var m = 0; m < mk.moons; m++) out.moons.push(lm[m] ? take(used, lm[m]) : nameMoon(td, mr, used)); }
  if (mk.festivals) {
    var fr = R('festivals'), lf = lockedList('festivals'), moonName = out.moons && out.moons[0] ? out.moons[0] : null;
    out.festivals = [];
    for (var f = 0; f < mk.festivals; f++) {
      var anchor = FESTIVAL_ANCHORS[f % FESTIVAL_ANCHORS.length];
      out.festivals.push({ name: lf[f] ? take(used, lf[f]) : nameFestival(td, fr, anchor, { moon: moonName || nameMoon(td, fr, {}) }, used), anchor: anchor, when: ANCHOR_WORDS[anchor] });
    }
  }
  if (mk.eras) {
    var er = R('eras'), le = lockedList('eras'), chars = ['founding', 'golden', 'strife', 'dark', 'restoration', 'faith', 'discovery', 'decline'];
    out.eras = [];
    for (var e = 0; e < mk.eras; e++) {
      var ch = e === mk.eras - 1 ? 'present' : chars[e % chars.length];
      out.eras.push({ name: le[e] ? take(used, le[e]) : nameEra(td, er, ch, used), character: ch });
    }
  }
  if (mk.places) { var pr = R('places'), lp = lockedList('places'); out.places = []; for (var p = 0; p < mk.places; p++) out.places.push(lp[p] ? take(used, lp[p]) : namePlace(td, pr, used)); }
  if (mk.people) { var qr = R('people'), lq = lockedList('people'); out.people = []; for (var q = 0; q < mk.people; q++) out.people.push(lq[q] ? take(used, lq[q]) : namePerson(td, qr, used)); }
  // The "never use" list also rules out fragments inside names; anything caught is replaced.
  if (avoidBits.length) {
    ownKeys(out).forEach(function (k) {
      out[k] = out[k].map(function (item, i) {
        var nm = typeof item === 'string' ? item : item.name;
        if (!avoidBits.some(function (b) { return nm.toLowerCase().indexOf(b) >= 0; })) return item;
        var rr = makeRng(seed, 'names-avoid', k, i), repl = nm;
        for (var a = 0; a < 20; a++) {
          repl = k === 'places' ? namePlace(td, rr, used) : k === 'people' ? namePerson(td, rr, used) : k === 'moons' ? nameMoon(td, rr, used) : tongueWord(td, rr, k === 'weekdays' ? 'weekdays' : 'months') || repl;
          if (!avoidBits.some(function (b) { return repl.toLowerCase().indexOf(b) >= 0; })) break;
        }
        if (typeof item === 'string') return repl;
        var c2 = clone(item); c2.name = repl; return c2;
      });
    });
  }
  var parts = [];
  if (out.months) parts.push(count(out.months.length, 'month') + (cal ? ' named for the season each falls in' : ' that run from midwinter round the year'));
  if (out.weekdays) parts.push(count(out.weekdays.length, 'weekday') + (out.weekdays.some(function (w) { return w.rest; }) ? ' with ' + (out.weekdays.filter(function (w) { return w.rest; }).length === 1 ? 'a rest day' : 'rest days') : ''));
  if (out.festivals) parts.push(count(out.festivals.length, 'festival') + ' tied to the turns of the year');
  if (out.seasons) parts.push(count(out.seasons.length, 'season'));
  if (out.eras) parts.push(count(out.eras.length, 'era') + ' that read as a history, oldest first');
  if (out.moons) parts.push(count(out.moons.length, 'moon'));
  if (out.places) parts.push(count(out.places.length, 'place'));
  if (out.people) parts.push(count(out.people.length, 'person', 'people'));
  var lockedCount = sum(ownKeys(locked).map(function (k) { return lockedList(k).filter(Boolean).length; }));
  var summary = cap(THEMES[dt.theme].label) + ' names: ' + joinList(parts) + '. Every name in the set is different, the set keeps one voice, and none is taken from a published setting or a real calendar.' +
    (lockedCount ? ' The ' + count(lockedCount, 'name') + ' you locked stayed as they were.' : '') + (avoidBits.length ? ' Nothing uses ' + joinList(avoidBits.map(function (a) { return '“' + a + '”'; })) + '.' : '');
  return { generator: 'names', seed: seed, recipe: recipe, scope: { label: 'a set of names', days: 0 }, names: out, summary: summary, stats: { theme: dt.theme }, warnings: [] };
}

/* ── Moons ──────────────────────────────────────────────────────────────────────────────────────────
   Phase maths mirrors Moon.MoonPhase in model.go (0 new, 0.5 full, over AbsoluteDay). A full or new moon
   belongs to the one day nearest the instant the phase crosses it, as the moon-graph mockup draws it, so
   each cycle marks exactly one full moon. */

function moonPhaseAt(m, abs) {
  if (!(m.cycle > 0)) return 0;
  var raw = (abs + m.offset) / m.cycle, p = raw - Math.trunc(raw);
  return p < 0 ? p + 1 : p;
}
function litFraction(p) { return (1 - Math.cos(2 * Math.PI * p)) / 2; }
function daysToPhase(m, abs, target) { var p = moonPhaseAt(m, abs), d = mod(target - p + 0.5, 1) - 0.5; return Math.abs(d * m.cycle); }
/* Chronicle's MoonPhaseName buckets. */
function moonPhaseName(p) {
  return p < 0.125 ? 'New Moon' : p < 0.25 ? 'Waxing Crescent' : p < 0.375 ? 'First Quarter' : p < 0.5 ? 'Waxing Gibbous' :
    p < 0.625 ? 'Full Moon' : p < 0.75 ? 'Waning Gibbous' : p < 0.875 ? 'Last Quarter' : 'Waning Crescent';
}
/* The instants in [from, to] when a moon's phase equals target (0 new, 0.5 full), with the day they fall on
   and how far through that day (0..1, noon = 0.5). */
function phaseInstants(m, from, to, target) {
  var out = [];
  if (!(m.cycle > 0)) return out;
  var k = Math.floor((from + m.offset) / m.cycle - target) - 1;
  for (var guard = 0; guard < 100000; guard++, k++) {
    var t = (k + target) * m.cycle - m.offset;
    if (t > to + 0.5) break;
    var day = Math.round(t);
    if (day >= from && day <= to) out.push({ t: t, abs: day, frac: t - (day - 0.5) });
  }
  return out;
}
/* Moon events over [from, to] for the given moons: full, new, conjunctions (two or more full the same
   night), moonless nights (every moon new together), blue moons (a second full moon in one month) and the
   harvest and hunter's moons (the primary moon's full moons nearest the autumn turn). */
function findMoonEvents(cal, moons, from, to) {
  var ev = [], fullByDay = {}, newByDay = {};
  moons.forEach(function (m) {
    phaseInstants(m, from, to, 0.5).forEach(function (x) { ev.push({ type: 'full', moon: m, abs: x.abs, frac: x.frac }); (fullByDay[x.abs] = fullByDay[x.abs] || []).push(m); });
    phaseInstants(m, from, to, 0).forEach(function (x) { ev.push({ type: 'new', moon: m, abs: x.abs, frac: x.frac }); (newByDay[x.abs] = newByDay[x.abs] || []).push(m); });
  });
  if (moons.length >= 2) {
    ownKeys(fullByDay).forEach(function (k) { var ms = fullByDay[k]; if (ms.length >= 2) ev.push({ type: 'conjunction', moons: ms, abs: +k }); });
    // A moonless night: on the first moon's new-moon day, every other moon is within a day of new as well.
    phaseInstants(moons[0], from, to, 0).forEach(function (x) {
      if (moons.every(function (m) { return daysToPhase(m, x.abs, 0) <= 1.0; })) ev.push({ type: 'dark-night', moons: moons.slice(), abs: x.abs });
    });
  }
  // A blue moon only means something for a moon that is normally full once a month.
  var monthLen = cal.refYearLength / Math.max(1, cal.months.filter(function (M) { return !M.intercalary; }).length);
  moons.filter(function (m) { return m.cycle >= monthLen * 0.8 && m.cycle <= monthLen; }).forEach(function (m) {
    var byMonth = {};
    ev.filter(function (e) { return e.type === 'full' && e.moon === m; }).forEach(function (e) {
      var d = cal.fromAbs(e.abs);
      if (cal.isIntercalary(d.month)) return;
      var key = d.year + '-' + d.month;
      (byMonth[key] = byMonth[key] || []).push(e);
    });
    ownKeys(byMonth).forEach(function (k) { if (byMonth[k].length >= 2) ev.push({ type: 'blue-moon', moon: m, abs: byMonth[k][1].abs, frac: byMonth[k][1].frac }); });
  });
  var primary = moons.filter(function (m) { return !m.hidden && m.cycle > 0; })[0] || moons[0];
  if (primary) {
    var years = uniq([cal.fromAbs(from).year, cal.fromAbs(to).year]);
    for (var y = years[0]; y <= years[years.length - 1]; y++) {
      var eq = cal.absOf(cal.anchorDays(y).autumnEquinox);
      var fulls = phaseInstants(primary, eq - primary.cycle, eq + 2 * primary.cycle, 0.5);
      if (!fulls.length) continue;
      var best = fulls.slice().sort(function (a, b) { return Math.abs(a.abs - eq) - Math.abs(b.abs - eq); })[0];
      var hunter = fulls.filter(function (x) { return x.abs > best.abs; })[0];
      if (best.abs >= from && best.abs <= to) ev.push({ type: 'harvest-moon', moon: primary, abs: best.abs, frac: best.frac });
      if (hunter && hunter.abs >= from && hunter.abs <= to) ev.push({ type: 'hunters-moon', moon: primary, abs: hunter.abs, frac: hunter.frac });
    }
  }
  ev.sort(function (a, b) { return a.abs - b.abs || (a.type < b.type ? -1 : 1); });
  return ev;
}

/* ── Random moons ── */
var MOON_COLOURS = {
  pale: ['#e6e2d8', '#d9e1ec', '#efe8d8', '#cfd3d8', '#eadcb4', '#dfe8f3', '#e8e4ee'],
  coloured: ['#a98ee6', '#e3a24f', '#76b5a8', '#e6a0a8', '#c8744f', '#8fc9a0', '#8fb3e6', '#d7c26a'],
  strange: ['#b84a4a', '#54506e', '#6fae9a', '#7d5a8c', '#b8c46a', '#3f6f7a']
};
var SURFACE_PRESETS = {
  maria: 'dark seas on a bright face, like our own moon',
  highlands: 'heavily cratered, almost no seas',
  basin: 'one huge dark basin, like a bruise',
  rayed: 'bright young craters throwing long rays',
  ice: 'smooth and pale, few craters',
  ember: 'dark and pitted, scattered with bright spots'
};
function jitterHex(hex, rng, amount) {
  var c = hexToRgb(hex);
  return rgbToHex(c[0] + rng.range(-amount, amount), c[1] + rng.range(-amount, amount), c[2] + rng.range(-amount, amount));
}
/* A surface in the v5 mockup's SURF shape, so its moon renderer can draw it. */
function makeSurface(rng, preset, color, tint) {
  function sea(rMin, rMax, shade) {
    var a = rng.range(0, Math.PI * 2), d = Math.sqrt(rng.next()) * 0.7;
    var s = [round2(Math.cos(a) * d), round2(Math.sin(a) * d), round2(rng.range(rMin, rMax)), round2(rng.range(0.85, 1.1)), round2(shade || rng.range(0.78, 1.05))];
    if (rng.chance(0.4)) s.push(round2(rng.range(1, 1.7)), round2(rng.range(-1.5, 1.5)));
    return s;
  }
  var S = { preset: preset, seed: rng.int(1, 9999), craters: 3200, rMin: 0.0045, rMax: 0.15, mareKeep: 0.35, hi: 0.58, lo: 0.2, relief: 1, color: color, tint: round2(tint) };
  var i, n;
  if (preset === 'maria') { n = rng.int(4, 7); S.seas = []; for (i = 0; i < n; i++) S.seas.push(sea(0.12, 0.38)); S.rayAt = [[round2(rng.range(-0.6, 0.6)), round2(rng.range(-0.7, 0.7)), round2(rng.range(0.016, 0.03))], [round2(rng.range(-0.6, 0.6)), round2(rng.range(-0.7, 0.7)), round2(rng.range(0.016, 0.03))]]; }
  else if (preset === 'highlands') { n = rng.int(1, 2); S.seas = []; for (i = 0; i < n; i++) S.seas.push(sea(0.08, 0.2)); S.craters = rng.int(3500, 3800); S.rMax = 0.17; S.mareKeep = 0.6; S.rays = 1; S.hi = 0.5; S.lo = 0.27; S.relief = 1.35; }
  else if (preset === 'basin') { S.seas = [[round2(rng.range(-0.15, 0.15)), round2(rng.range(-0.15, 0.15)), round2(rng.range(0.4, 0.48)), 1.25, 0.78, 1, 0, 0.25], sea(0.08, 0.13), sea(0.08, 0.12)]; S.craters = 2600; S.rMax = 0.14; S.rays = 0; S.hi = 0.52; S.lo = 0.19; S.relief = 1.1; }
  else if (preset === 'rayed') { S.basins = rng.int(2, 3); S.basinR = [0.11, 0.22]; S.craters = rng.int(3000, 3200); S.rMax = 0.12; S.rays = rng.int(4, 6); S.rayLen = rng.int(11, 14); S.hi = 0.62; S.lo = 0.28; S.relief = 1.1; }
  else if (preset === 'ice') { S.basins = rng.int(1, 2); S.basinR = [0.1, 0.18]; S.craters = rng.int(1400, 1900); S.rMax = 0.1; S.mareKeep = 0.5; S.rays = 1; S.hi = 0.72; S.lo = 0.45; S.relief = 0.6; }
  else { n = rng.int(5, 8); S.seas = []; for (i = 0; i < n; i++) S.seas.push(sea(0.05, 0.14, rng.range(0.7, 0.85))); S.craters = 2400; S.rMax = 0.13; S.rays = 0; S.hi = 0.5; S.lo = 0.15; S.relief = 1.2; }
  return S;
}
function resonant(a, b) {
  var r = a > b ? a / b : b / a;
  for (var q = 1; q <= 4; q++) for (var p = q; p <= 4 * q; p++) if (Math.abs(r - p / q) < 0.04) return true;
  return false;
}

defineGenerator({
  id: 'moons', label: 'Moons', needsCalendar: false,
  blurb: 'Moons with a cycle, a colour, a surface and a name; with a calendar, it says when they are full together.',
  makes: {
    count: { label: 'How many moons', type: 'count', default: 2, min: 0, max: 6 },
    pale: { label: 'Pale, natural moons', type: 'freq', default: 'normal' },
    coloured: { label: 'Coloured moons', type: 'freq', default: 'normal' },
    strange: { label: 'Strange moons', type: 'freq', default: 'rare', help: 'Blood-red, bruise-violet, sickly green.' }
  },
  scope: {},
  details: {
    theme: { label: 'Culture for names', type: 'select', default: 'pastoral', options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) },
    cycles: { label: 'Cycle lengths', type: 'select', default: 'natural', options: [{ value: 'natural', label: 'One month-long moon, the rest faster or slower' }, { value: 'varied', label: 'Anything goes' }, { value: 'fast', label: 'Fast (under two weeks)' }, { value: 'slow', label: 'Slow (six weeks or more)' }] },
    surface: { label: 'Surfaces', type: 'select', default: 'mixed', more: true, options: [{ value: 'mixed', label: 'A mix' }].concat(ownKeys(SURFACE_PRESETS).map(function (k) { return { value: k, label: cap(k) + ': ' + SURFACE_PRESETS[k] }; })) },
    hidden: { label: 'Moons players can’t see', type: 'count', default: 0, min: 0, max: 6, more: true }
  },
  builtIns: [
    { id: 'moons.one', name: 'One moon', description: 'A single pale moon on a month-long cycle.', makes: { count: 1, coloured: 'off', strange: 'off' } },
    { id: 'moons.two', name: 'Two moons', description: 'A familiar pale moon and a faster or slower companion.', makes: { count: 2 } },
    { id: 'moons.many', name: 'A crowded sky', description: 'Four moons of different colours, one of them strange.', makes: { count: 4, coloured: 'often', strange: 'normal' }, details: { cycles: 'varied' } }
  ],
  defaultRecipe: 'moons.two',
  run: runMoons
});

function runMoons(opts) {
  var recipe = resolveRecipe('moons', opts.recipe), seed = seedOf(opts), mk = recipe.makes, dt = recipe.details, td = themeData(dt.theme);
  var cal = opts.calendar ? makeCal(opts.calendar) : null;
  var locked = (opts.locked || []).map(function (m) { return typeof m === 'string' ? (cal && cal.findMoon(m) ? cal.findMoon(m).raw : { name: m }) : m; });
  var used = {};
  locked.forEach(function (m) { used[String(m.name).toLowerCase()] = 1; });
  var cycles = locked.map(function (m) { return +m.cycle_days; }).filter(function (c) { return c > 0; });
  var out = locked.map(function (m) { var c = clone(m); c.gen = Object.assign({}, c.gen || {}, { key: makeKey('moon', c.name), locked: true }); return c; });
  var n = Math.max(0, mk.count - locked.length);
  for (var i = 0; i < n; i++) {
    var r = makeRng(seed, 'moon', dt.theme, i);
    var ch = r.weighted(['pale', 'coloured', 'strange'], function (k) { return freqMult(mk[k]) * (k === 'pale' && out.length === 0 ? 3 : 1); }) || 'pale';
    var cyc = 0;
    for (var a = 0; a < 40; a++) {
      var primary = out.length === 0 && dt.cycles === 'natural';
      cyc = dt.cycles === 'fast' ? r.range(5, 15) : dt.cycles === 'slow' ? r.range(40, 120) : dt.cycles === 'varied' ? r.range(6, 90) :
        primary ? r.range(26, 33) : r.pick([function () { return r.range(8, 15); }, function () { return r.range(45, 80); }, function () { return r.range(17, 24); }])();
      cyc = round1(cyc);
      if (cycles.every(function (c) { return Math.abs(c - cyc) >= 2.5 && !resonant(c, cyc); })) break;
    }
    cycles.push(cyc);
    var colour = jitterHex(r.pick(MOON_COLOURS[ch]), r, 10);
    var preset = dt.surface !== 'mixed' ? dt.surface : ch === 'pale' ? r.pick(['maria', 'maria', 'highlands', 'rayed']) : ch === 'coloured' ? r.pick(['rayed', 'ice', 'basin', 'maria']) : r.pick(['basin', 'ember', 'highlands']);
    var tint = ch === 'pale' ? r.range(0.2, 0.35) : r.range(0.45, 0.65);
    var name = nameMoon(td, r, used);
    var hidden = i >= n - dt.hidden;
    out.push({
      name: name, cycle_days: cyc, phase_offset: round2(r.range(0, cyc)), color: colour,
      base_design: 'moon-realistic-selene', tint: ch === 'pale' ? null : colour, phase_source: 'css-clip',
      size: round2(out.length === 0 ? r.range(0.95, 1.1) : r.range(0.45, 0.95)), orbit_speed: 1, visibility: hidden ? 'dm_only' : 'everyone',
      gen: { key: makeKey('moon', seed, i, name), generator: 'moons', character: ch, surface: makeSurface(r, preset, colour, tint), locked: false }
    });
  }
  var lines = out.map(function (m) {
    var sp = m.gen.surface ? SURFACE_PRESETS[m.gen.surface.preset] : null;
    return m.name + (m.gen.locked ? ' (kept)' : ', ' + withArticle(m.gen.character === 'pale' ? 'pale moon' : m.gen.character === 'coloured' ? 'coloured moon' : 'strange moon') + (sp ? ' (' + sp + ')' : '')) + ', full every ' + m.cycle_days + ' days' + (m.visibility === 'dm_only' ? ', hidden from players' : '');
  });
  var extra = '';
  if (cal && out.length >= 2) {
    var cm = out.map(function (m, i2) { return { name: m.name, cycle: m.cycle_days, offset: m.phase_offset, hidden: m.visibility === 'dm_only', index: i2 }; });
    var ys = cal.yearSpan(cal.currentYear), conj = findMoonEvents(cal, cm, ys[0], ys[1]).filter(function (e) { return e.type === 'conjunction'; });
    extra = ' In ' + cal.currentYear + ' two of them are full on the same night ' + (conj.length === 1 ? 'once' : conj.length === 2 ? 'twice' : numWord(conj.length) + ' times') + (conj.length ? ', first on ' + cal.fmtAbs(conj[0].abs) : '') + '.';
  }
  var summary = (out.length ? cap(numWord(out.length)) + (out.length === 1 ? ' moon: ' : ' moons: ') + lines.join('; ') + '.' : 'No moons.') +
    (out.length >= 2 ? ' Their cycles are kept apart and out of step, so they drift in and out of alignment instead of repeating every month.' : '') + extra;
  var preset = out.map(function (m) { return { name: m.name, cycle_days: m.cycle_days, phase_offset: m.phase_offset, color: m.color }; });
  return { generator: 'moons', seed: seed, recipe: recipe, scope: { label: 'a set of moons', days: 0 }, moons: out, presetMoons: preset, summary: summary, stats: { count: out.length }, warnings: [] };
}

/* ── Weather catalogue ──────────────────────────────────────────────────────────────────────────────
   Preset ids are Calendaria's 42, verbatim (the list the v4 weather-catalog contract test pinned against
   upstream), so what Chronicle generates is always something Foundry can show. Each preset also carries
   the features the engine reasons with: how cloudy, wet and energetic it is (neighbouring presets are
   likelier successors, so weather changes gradually), how long it tends to persist, its precipitation
   in Calendaria's type vocabulary, and how it shifts the day's temperature in winter and summer.
   glyph is the calendar mockup's six-icon vocabulary (clear, cloud, rain, snow, storm, fog).
   Fields: [label, category, glyph, colour, cloud, wet, energy, persistence, precipType, intensity,
            dT winter, dT summer, diurnal factor, wind add kph, forced kph, switch, effect]. effect is the
            preset's id in Calendaria's sky-effect vocabulary (the EFFECTS list, under "Your own weather"
            below), so every built-in can be drawn there with no separate lookup. */
var PRESET_ROWS = {
  'clear': ['Clear', 'Standard', 'clear', '#f2c14e', 0, 0, 0.1, 'stable', null, 0, -2, 1.5, 1.3, 0, 0, null, 'clear'],
  'partly-cloudy': ['Partly Cloudy', 'Standard', 'clear', '#d9c68a', 0.35, 0.05, 0.15, 'normal', null, 0, -0.5, 0.5, 1.1, 0, 0, null, 'clouds-light'],
  'cloudy': ['Cloudy', 'Standard', 'cloud', '#a3adb8', 0.7, 0.1, 0.2, 'normal', null, 0, 1, -1, 0.75, 0, 0, null, 'clouds-heavy'],
  'overcast': ['Overcast', 'Standard', 'cloud', '#838d98', 0.95, 0.2, 0.2, 'stable', null, 0, 1.5, -2, 0.55, 0, 0, null, 'clouds-overcast'],
  'drizzle': ['Drizzle', 'Standard', 'rain', '#90b4cf', 0.9, 0.4, 0.15, 'normal', 'drizzle', 0.2, 1, -2, 0.5, 0, 0, 'rain', 'rain'],
  'rain': ['Rain', 'Standard', 'rain', '#5b8fc7', 0.95, 0.75, 0.4, 'normal', 'rain', 0.55, 0.5, -3, 0.45, 6, 0, 'rain', 'rain'],
  'fog': ['Fog', 'Standard', 'fog', '#b9c0c8', 0.8, 0.3, 0, 'stable', 'drizzle', 0.05, -1, -2, 0.4, -10, 0, 'fog', 'fog'],
  'mist': ['Mist', 'Standard', 'fog', '#c9d1d9', 0.5, 0.25, 0.05, 'normal', null, 0, -0.5, -1, 0.6, -6, 0, 'fog', 'fog'],
  'windy': ['Windy', 'Standard', 'cloud', '#9fbfae', 0.4, 0.05, 0.6, 'normal', null, 0, -1, 0, 0.9, 18, 0, 'wind', 'gust'],
  'sunshower': ['Sunshower', 'Standard', 'rain', '#9fcbd8', 0.45, 0.35, 0.2, 'fleeting', 'rain', 0.25, 0, 0, 0.9, 0, 0, 'rain', 'rain'],
  'snow': ['Snow', 'Standard', 'snow', '#e6eef7', 0.95, 0.6, 0.3, 'normal', 'snow', 0.5, -1, -3, 0.5, 4, 0, 'snow', 'snow'],
  'sleet': ['Sleet', 'Standard', 'rain', '#a9bdcc', 0.95, 0.6, 0.4, 'normal', 'sleet', 0.5, 0, -2, 0.45, 8, 0, 'snow', 'sleet'],
  'heat-wave': ['Heat Wave', 'Standard', 'clear', '#ef8a3d', 0.05, 0, 0.1, 'stable', null, 0, 2, 3, 1.2, -3, 0, 'heat', 'haze'],
  'thunderstorm': ['Thunderstorm', 'Severe', 'storm', '#5f5c8f', 0.9, 0.8, 0.85, 'fleeting', 'rain', 0.8, 0, -3, 0.7, 0, 48, 'storms', 'lightning'],
  'blizzard': ['Blizzard', 'Severe', 'snow', '#d6e4f2', 1, 0.8, 0.95, 'fleeting', 'snow', 0.9, -3, -3, 0.4, 0, 70, 'blizzards', 'snow-heavy'],
  'hail': ['Hail', 'Severe', 'storm', '#b5c5d5', 0.9, 0.7, 0.8, 'fleeting', 'hail', 0.7, -1, -4, 0.7, 12, 0, 'storms', 'hail'],
  'tornado': ['Tornado', 'Severe', 'storm', '#6d6d72', 1, 0.7, 1, 'fleeting', 'rain', 0.7, 0, -2, 0.7, 0, 130, 'tornadoes', 'tornado'],
  'hurricane': ['Hurricane', 'Severe', 'storm', '#4b5b79', 1, 1, 1, 'fleeting', 'rain', 1, 0, -3, 0.3, 0, 140, 'tropical-storms', 'hurricane'],
  'ice-storm': ['Ice Storm', 'Severe', 'storm', '#a7d0e6', 1, 0.7, 0.7, 'fleeting', 'hail', 0.6, -1, -1, 0.3, 0, 42, 'blizzards', 'ice'],
  'monsoon': ['Monsoon', 'Severe', 'rain', '#3f6f9f', 1, 1, 0.6, 'stable', 'rain', 0.95, 0, -3, 0.3, 0, 45, 'tropical-storms', 'rain-heavy'],
  'ashfall': ['Ashfall', 'Environmental', 'fog', '#8b8681', 0.8, 0.1, 0.3, 'stable', null, 0, -1, -2, 0.6, 0, 0, 'smoke-ash', 'ashfall'],
  'sandstorm': ['Sandstorm', 'Environmental', 'fog', '#d4b07a', 0.6, 0, 0.9, 'fleeting', null, 0, 0, -3, 0.5, 45, 0, 'dust', 'sand'],
  'luminous-sky': ['Luminous Sky', 'Environmental', 'clear', '#78dfc2', 0.1, 0, 0.1, 'fleeting', null, 0, -1, 0, 1.2, 0, 0, 'colour', 'aurora'],
  'sakura-bloom': ['Sakura Bloom', 'Environmental', 'clear', '#f1b3c7', 0.2, 0, 0.2, 'normal', null, 0, 0, 0, 1.0, 4, 0, 'colour', 'petals'],
  'autumn-leaves': ['Autumn Leaves', 'Environmental', 'cloud', '#d68a3c', 0.4, 0.05, 0.5, 'normal', null, 0, -1, 0, 0.9, 12, 0, 'colour', 'leaves'],
  'rolling-fog': ['Rolling Fog', 'Environmental', 'fog', '#aab5be', 0.85, 0.35, 0.15, 'normal', null, 0, -1, -2, 0.4, -6, 0, 'fog', 'fog'],
  'wildfire-smoke': ['Wildfire Smoke', 'Environmental', 'fog', '#b08b6b', 0.6, 0, 0.2, 'stable', null, 0, 0, 1, 0.8, 0, 0, 'smoke-ash', 'smoke'],
  'dust-devil': ['Dust Devil', 'Environmental', 'clear', '#d8b98c', 0.1, 0, 0.5, 'fleeting', null, 0, 0, 1, 1.2, 10, 0, 'dust', 'sand'],
  'black-sun': ['Black Sun', 'Fantasy', 'cloud', '#3d3346', 0.5, 0, 0.5, 'fleeting', null, 0, -4, -6, 0.5, 0, 0, 'fantasy', 'void'],
  'ley-surge': ['Ley Surge', 'Fantasy', 'storm', '#9a6df0', 0.5, 0.1, 0.7, 'fleeting', null, 0, 0, 0, 0.8, 8, 0, 'fantasy', 'ley-surge'],
  'aether-haze': ['Aether Haze', 'Fantasy', 'fog', '#b8a7e8', 0.5, 0.1, 0.2, 'normal', null, 0, 0, 0, 0.7, -4, 0, 'fantasy', 'aether'],
  'nullfront': ['Nullfront', 'Fantasy', 'cloud', '#5e6571', 0.8, 0.1, 0.6, 'fleeting', null, 0, -2, -3, 0.5, 0, 0, 'fantasy', 'nullstatic'],
  'permafrost-surge': ['Permafrost Surge', 'Fantasy', 'snow', '#bfe5ff', 0.9, 0.6, 0.7, 'fleeting', 'snow', 0.7, -10, -12, 0.4, 10, 0, 'fantasy', 'ice'],
  'gravewind': ['Gravewind', 'Fantasy', 'cloud', '#7c867a', 0.6, 0.05, 0.7, 'normal', null, 0, -2, -3, 0.7, 20, 0, 'fantasy', 'spectral'],
  'veilfall': ['Veilfall', 'Fantasy', 'fog', '#8f97a9', 0.9, 0.3, 0.3, 'stable', 'rain', 0.3, -2, -3, 0.4, -4, 0, 'fantasy', 'veil'],
  'arcane-winds': ['Arcane Winds', 'Fantasy', 'cloud', '#8f7cf0', 0.4, 0, 0.8, 'fleeting', null, 0, 0, 0, 0.8, 26, 0, 'fantasy', 'arcane-wind'],
  'acid-rain': ['Acid Rain', 'Fantasy', 'rain', '#9ccf5b', 0.95, 0.7, 0.4, 'normal', 'rain', 0.6, 0, -2, 0.45, 4, 0, 'fantasy', 'rain-acid'],
  'blood-rain': ['Blood Rain', 'Fantasy', 'rain', '#b03b3b', 0.95, 0.6, 0.4, 'fleeting', 'rain', 0.5, 0, -2, 0.45, 4, 0, 'fantasy', 'rain-blood'],
  'meteor-shower': ['Meteor Shower', 'Fantasy', 'clear', '#7ea2ff', 0.05, 0, 0.2, 'fleeting', null, 0, -1, 0, 1.2, 0, 0, 'fantasy', 'meteors'],
  'spore-cloud': ['Spore Cloud', 'Fantasy', 'fog', '#b5c36f', 0.5, 0.2, 0.2, 'fleeting', null, 0, 0, 0, 0.7, -2, 0, 'fantasy', 'spores'],
  'divine-light': ['Divine Light', 'Fantasy', 'clear', '#fff0b0', 0.1, 0, 0.3, 'fleeting', null, 0, 1, 1, 1.1, 0, 0, 'fantasy', 'divine'],
  'plague-miasma': ['Plague Miasma', 'Fantasy', 'fog', '#909b5b', 0.7, 0.4, 0.1, 'stable', null, 0, 0, 1, 0.5, -6, 0, 'fantasy', 'miasma']
};
var PRESETS = {};
var PERSIST = { stable: 1, normal: 0.75, fleeting: 0.3 };
ownKeys(PRESET_ROWS).forEach(function (id) {
  var r = PRESET_ROWS[id];
  PRESETS[id] = {
    id: id, label: r[0], category: r[1], glyph: r[2], color: r[3], cloud: r[4], wet: r[5], energy: r[6], persist: PERSIST[r[7]], persistence: r[7],
    precipType: r[8], intensity: r[9], dTw: r[10], dTs: r[11], diurnal: r[12], windAdd: r[13], forced: r[14], group: r[15], effect: r[16]
  };
});
var WET_PRESETS = { drizzle: 1, rain: 1, sunshower: 1, snow: 1, sleet: 1, thunderstorm: 1, blizzard: 1, hail: 1, tornado: 1, hurricane: 1, 'ice-storm': 1, monsoon: 1, 'acid-rain': 1, 'blood-rain': 1, veilfall: 1, 'permafrost-surge': 1 };
var DRY_PRESETS = { clear: 1, 'partly-cloudy': 1, 'heat-wave': 1, 'dust-devil': 1, windy: 0.5 };

/* The recipe's switches: each groups the presets and multi-day systems it turns up or down. */
var WEATHER_SWITCHES = {
  rain: { label: 'Rain and drizzle', help: 'Drizzle, rain and sunshowers.' },
  snow: { label: 'Snow and sleet', help: 'Ordinary snow and sleet; blizzards have their own switch.' },
  fog: { label: 'Fog and mist', help: 'Fog, mist, rolling fog and fog that settles in for days.' },
  storms: { label: 'Thunderstorms', help: 'Thunder, hail, and the fronts that bring them.' },
  blizzards: { label: 'Blizzards and ice storms' },
  wind: { label: 'Wind and gales' },
  heat: { label: 'Heat waves' },
  'cold-snaps': { label: 'Cold snaps', help: 'Clear, still spells of hard cold.' },
  'tropical-storms': { label: 'Monsoons and hurricanes' },
  dust: { label: 'Dust and sandstorms' },
  tornadoes: { label: 'Tornadoes', default: 'rare' },
  colour: { label: 'Seasonal colour', help: 'Blossom, falling leaves and glowing skies, where the climate has them.' },
  'smoke-ash': { label: 'Smoke and ash' },
  fantasy: { label: 'Uncanny weather', help: 'The fantasy presets: veilfall, ley surges, blood rain and the rest.' },
  fronts: { label: 'Multi-day fronts and storms', help: 'How often weather arrives as a system lasting several days.' }
};

/* Multi-day systems. Each step is one day: 'a|b' picks one, '@cold' / '@warm' keep a choice to days that
   suit it, repeat marks the step that stretches to fill the system's length. dT and wind are added to the
   day; veer turns the wind (degrees clockwise). when: which days can start one. */
var SYSTEMS = {
  'rain-front': { label: 'A rain front', group: 'rain', len: [2, 4], steps: [
    { role: 'lead', p: 'cloudy|overcast', dT: 1, wind: 6 }, { role: 'peak', p: 'rain', wind: 10, repeat: true }, { role: 'tail', p: 'drizzle|partly-cloudy|windy', dT: -2, wind: 4, veer: 60 }] },
  'cold-front': { label: 'A cold front', group: 'storms', len: [2, 3], steps: [
    { role: 'lead', p: 'windy|cloudy', dT: 2, wind: 10 }, { role: 'peak', p: 'thunderstorm@warm|rain@mild|snow@cold', wind: 16 }, { role: 'tail', p: 'clear|partly-cloudy', dT: -5, wind: 8, veer: 90 }] },
  'winter-storm': { label: 'A winter storm', group: 'blizzards', len: [3, 5], when: 'cold', steps: [
    { role: 'lead', p: 'overcast', wind: 8 }, { role: 'peak', p: 'snow', dT: -1, wind: 14 }, { role: 'peak', p: 'blizzard', dT: -2, wind: 30, repeat: true }, { role: 'tail', p: 'clear', dT: -6, wind: 4, veer: 60 }] },
  'gale': { label: 'A gale', group: 'wind', len: [2, 3], steps: [
    { role: 'lead', p: 'windy', wind: 18 }, { role: 'peak', p: 'rain@mild|sleet@cold', wind: 40, repeat: true }, { role: 'tail', p: 'windy|partly-cloudy', wind: 14, veer: 45 }] },
  'heat-wave': { label: 'A heat wave', group: 'heat', len: [3, 7], when: 'hot', steps: [
    { role: 'lead', p: 'clear', dT: 2 }, { role: 'peak', p: 'heat-wave', dT: 4, wind: -4, repeat: true }, { role: 'tail', p: 'thunderstorm|partly-cloudy', dT: 1 }] },
  'thunder-break': { label: 'Heat, then thunder', group: 'storms', len: [2, 3], when: 'warm', steps: [
    { role: 'lead', p: 'clear|partly-cloudy', dT: 3, wind: -3, repeat: true }, { role: 'peak', p: 'thunderstorm|hail', wind: 12 }, { role: 'tail', p: 'partly-cloudy', dT: -3 }] },
  'cold-snap': { label: 'A cold snap', group: 'cold-snaps', len: [3, 6], when: 'cool', steps: [
    { role: 'lead', p: 'clear', dT: -5, wind: 6, veer: 45 }, { role: 'peak', p: 'clear|fog|mist', dT: -9, wind: -6, repeat: true }, { role: 'tail', p: 'cloudy|partly-cloudy', dT: -3 }] },
  'fog-spell': { label: 'A spell of fog', group: 'fog', len: [2, 4], steps: [
    { role: 'peak', p: 'fog', dT: -1, wind: -8, repeat: true }, { role: 'tail', p: 'mist|cloudy', wind: -4 }] },
  'sea-fog': { label: 'Sea fog', group: 'fog', len: [2, 4], steps: [
    { role: 'peak', p: 'fog|rolling-fog', dT: -2, wind: -4, repeat: true }, { role: 'tail', p: 'mist|partly-cloudy' }] },
  'thaw': { label: 'A thaw', group: 'rain', len: [2, 3], when: 'thaw', steps: [
    { role: 'lead', p: 'rain', dT: 4, wind: 6 }, { role: 'peak', p: 'fog|drizzle', dT: 3, repeat: true }, { role: 'tail', p: 'cloudy|partly-cloudy', dT: 2 }] },
  'monsoon-burst': { label: 'A monsoon burst', group: 'tropical-storms', len: [3, 6], when: 'warm', steps: [
    { role: 'lead', p: 'cloudy|overcast', wind: 6 }, { role: 'peak', p: 'monsoon|rain', wind: 10, repeat: true }, { role: 'tail', p: 'thunderstorm|cloudy', wind: 4 }] },
  'tropical-storm': { label: 'A tropical storm', group: 'tropical-storms', len: [3, 4], when: 'warm', steps: [
    { role: 'lead', p: 'windy|cloudy', wind: 20 }, { role: 'peak', p: 'hurricane', wind: 80 }, { role: 'tail', p: 'rain', wind: 30, repeat: true }, { role: 'tail', p: 'cloudy|windy', wind: 12 }] },
  'sandstorm': { label: 'A sandstorm', group: 'dust', len: [2, 3], steps: [
    { role: 'lead', p: 'windy', wind: 18 }, { role: 'peak', p: 'sandstorm', wind: 40, repeat: true }, { role: 'tail', p: 'dust-devil|windy|clear', wind: 10 }] },
  'ice-storm': { label: 'An ice storm', group: 'blizzards', len: [2, 3], when: 'freezing', steps: [
    { role: 'lead', p: 'overcast' }, { role: 'peak', p: 'ice-storm', wind: 14 }, { role: 'tail', p: 'cloudy|clear', dT: -3 }] },
  'ash-plume': { label: 'An ash plume', group: 'smoke-ash', len: [3, 6], steps: [
    { role: 'lead', p: 'wildfire-smoke', wind: 6 }, { role: 'peak', p: 'ashfall', dT: -2, repeat: true }, { role: 'tail', p: 'overcast|cloudy' }] },
  'acid-front': { label: 'A caustic front', group: 'fantasy', len: [2, 3], steps: [
    { role: 'lead', p: 'overcast' }, { role: 'peak', p: 'acid-rain', repeat: true }, { role: 'tail', p: 'cloudy' }] },
  'veil-thinning': { label: 'The veil thins', group: 'fantasy', len: [3, 5], steps: [
    { role: 'lead', p: 'rolling-fog', dT: -2, wind: -4 }, { role: 'peak', p: 'veilfall', dT: -3, repeat: true }, { role: 'tail', p: 'gravewind', wind: 14 }] },
  'miasma': { label: 'A miasma', group: 'fantasy', len: [2, 4], steps: [
    { role: 'lead', p: 'fog' }, { role: 'peak', p: 'plague-miasma', wind: -6, repeat: true }, { role: 'tail', p: 'mist' }] },
  'bloom': { label: 'The blossom', group: 'colour', len: [3, 6], steps: [
    { role: 'peak', p: 'sakura-bloom', repeat: true }, { role: 'tail', p: 'sunshower|luminous-sky' }] },
  'ley-storm': { label: 'A ley storm', group: 'fantasy', len: [2, 4], steps: [
    { role: 'lead', p: 'aether-haze' }, { role: 'peak', p: 'ley-surge', wind: 10, repeat: true }, { role: 'tail', p: 'arcane-winds', wind: 18 }] }
};

/* Climates. temp is the season's typical daytime range in C (its middle is the norm, its width sets the
   day-to-day swing), wind the typical speed in kph, diurnal how much colder nights run. Weights are
   relative shares of days, as in Calendaria; the engine compensates for persistence, so the long-run
   share of days comes out close to the weight's share. latitude drives auroras in the sky generator. */
function S(temp, wind, diurnal, weights, systems, dirs) { return { temp: temp, wind: wind, diurnal: diurnal, weights: weights, systems: systems || '', dirs: dirs || null }; }
var CLIMATES = {
  temperate: {
    name: 'Temperate', blurb: 'Four true seasons: grey wet winters with some snow, blustery springs, warm summers with thunder, long misty autumns.',
    latitude: 'mid', flavor: 'inland', dirs: 'W:3 SW:3 NW:2 S:1 N:1 E:0.5 NE:0.5 SE:0.5', inertia: 0.55,
    seasons: {
      winter: S([-3, 7], 18, 6, 'clear:2 partly-cloudy:2.5 cloudy:3 overcast:3.5 fog:1.5 mist:1 drizzle:1.5 rain:2.5 sleet:1 snow:2 windy:1', 'rain-front:0.22 winter-storm:0.08 cold-snap:0.07 fog-spell:0.06 gale:0.06'),
      spring: S([5, 16], 16, 9, 'clear:3 partly-cloudy:3.5 cloudy:2.5 overcast:1.5 drizzle:1.5 rain:2.5 sunshower:1 windy:1.5 fog:0.6 mist:0.8 thunderstorm:0.3 sleet:0.3 snow:0.2 hail:0.1', 'rain-front:0.22 cold-front:0.08 gale:0.04'),
      summer: S([16, 28], 11, 10, 'clear:4 partly-cloudy:3.5 cloudy:1.8 overcast:0.8 rain:1.5 drizzle:0.6 sunshower:0.7 thunderstorm:1 heat-wave:0.5 windy:0.4 mist:0.3 hail:0.15 tornado:0.02', 'rain-front:0.18 thunder-break:0.12 heat-wave:0.07 cold-front:0.1'),
      autumn: S([6, 17], 17, 8, 'clear:2 partly-cloudy:2.5 cloudy:3 overcast:2.5 fog:2 mist:1.5 drizzle:2 rain:3 windy:2 thunderstorm:0.2 sleet:0.2 autumn-leaves:0.6', 'rain-front:0.26 gale:0.1 fog-spell:0.08')
    }
  },
  'cold-coast': {
    name: 'Cold coast', blurb: 'Raw, windy and wet: gales off the sea, sleet more often than snow, and summer sea fogs.',
    latitude: 'high', flavor: 'coastal cold', dirs: 'W:3 SW:2 NW:2 N:1.5 S:1 E:0.5 NE:0.7 SE:0.5', inertia: 0.5,
    seasons: {
      winter: S([-5, 3], 28, 4, 'overcast:3 cloudy:3 snow:3 sleet:2 rain:1.5 windy:2.5 clear:1.2 partly-cloudy:1 fog:1 blizzard:0.4', 'gale:0.18 winter-storm:0.12 rain-front:0.12 cold-snap:0.05'),
      spring: S([0, 9], 22, 6, 'cloudy:3 overcast:2.5 partly-cloudy:2 clear:1.5 drizzle:2 rain:2 sleet:1 snow:1 windy:2 fog:1.5', 'gale:0.1 rain-front:0.15 sea-fog:0.08'),
      summer: S([9, 17], 16, 7, 'partly-cloudy:3 cloudy:3 clear:2 fog:2.5 mist:1.5 drizzle:2 rain:1.5 windy:1 sunshower:0.5', 'sea-fog:0.15 rain-front:0.1'),
      autumn: S([2, 10], 26, 5, 'overcast:3 cloudy:3 rain:3 drizzle:2 windy:2.5 partly-cloudy:1.2 clear:1 sleet:0.8 snow:0.5 fog:1', 'gale:0.22 rain-front:0.18')
    }
  },
  desert: {
    name: 'Desert', blurb: 'Hot, dry and bright; bitter nights in the cool season, heat that does not break in summer, winds that lift the sand.',
    latitude: 'low', flavor: 'desert', dirs: 'N:3 NE:3 E:1.5 NW:1 W:0.5 S:0.5', inertia: 0.65,
    seasons: {
      winter: S([12, 24], 14, 16, 'clear:6 partly-cloudy:2 windy:1.5 dust-devil:0.6 cloudy:0.6 rain:0.25 sandstorm:0.3', 'sandstorm:0.08'),
      spring: S([22, 34], 18, 16, 'clear:6 partly-cloudy:1.5 windy:2 dust-devil:1 sandstorm:0.6 cloudy:0.3', 'sandstorm:0.14'),
      summer: S([34, 45], 14, 15, 'clear:6 heat-wave:2 dust-devil:1 windy:1 partly-cloudy:0.6 sandstorm:0.4 thunderstorm:0.1', 'heat-wave:0.12 sandstorm:0.08'),
      autumn: S([24, 35], 13, 16, 'clear:6 partly-cloudy:1.5 windy:1.2 dust-devil:0.6 cloudy:0.4 rain:0.15 sandstorm:0.3', 'sandstorm:0.08')
    }
  },
  tropical: {
    name: 'Tropical', blurb: 'Warm all year: a bright dry season, a sticky build-up of thunder, then months of rain in bursts.',
    latitude: 'low', flavor: 'coastal tropical', dirs: 'E:3 NE:2 SE:2 S:1 SW:1', inertia: 0.55,
    seasons: {
      winter: S([24, 31], 14, 8, 'clear:4 partly-cloudy:4 cloudy:1.2 windy:0.8 sunshower:0.5 rain:0.4 mist:0.5', '', 'NE:3 E:2 N:1'),
      spring: S([27, 34], 12, 9, 'partly-cloudy:3 clear:2.5 cloudy:2 thunderstorm:1.2 heat-wave:0.6 sunshower:0.8 rain:0.8', 'thunder-break:0.12'),
      summer: S([25, 31], 18, 6, 'rain:3.5 cloudy:3 thunderstorm:2 monsoon:1 sunshower:1.2 drizzle:1 partly-cloudy:1.2 overcast:1.5', 'monsoon-burst:0.18 tropical-storm:0.03', 'SW:3 S:2 W:1'),
      autumn: S([25, 31], 16, 7, 'rain:2.5 cloudy:2.5 thunderstorm:1.5 partly-cloudy:2 sunshower:1 overcast:1 clear:1 windy:0.6', 'tropical-storm:0.07 monsoon-burst:0.06')
    }
  },
  highland: {
    name: 'Highland', blurb: 'Mountain weather: deep snow in winter, fog in the valleys, bright hard days and sudden summer storms.',
    latitude: 'mid', flavor: 'highland', dirs: 'W:2 NW:2 SW:1.5 N:1 E:1 S:1', inertia: 0.5,
    seasons: {
      winter: S([-10, 0], 22, 10, 'snow:3.5 clear:3 partly-cloudy:2 cloudy:2 overcast:2 blizzard:0.6 windy:1.5 fog:1', 'winter-storm:0.14 cold-snap:0.08'),
      spring: S([-1, 11], 20, 12, 'partly-cloudy:3 cloudy:2.5 clear:2.5 snow:1.2 sleet:1 rain:1.5 windy:2 fog:1', 'rain-front:0.12 thaw:0.08 gale:0.05'),
      summer: S([10, 22], 14, 13, 'clear:3.5 partly-cloudy:3 cloudy:1.5 thunderstorm:1.5 rain:1 hail:0.3 mist:1', 'thunder-break:0.1'),
      autumn: S([0, 11], 20, 11, 'clear:3 partly-cloudy:2.5 cloudy:2.5 overcast:1.5 fog:1.5 rain:1.5 snow:1 sleet:0.8 windy:2', 'rain-front:0.12 winter-storm:0.05 fog-spell:0.05')
    }
  },
  mediterranean: {
    name: 'Warm coast', blurb: 'Mild, rainy winters and long dry summers of blue sky; storms come back with the autumn.',
    latitude: 'low', flavor: 'coastal warm', dirs: 'NW:3 N:2 W:1.5 SE:1 S:1 E:0.5', inertia: 0.6,
    seasons: {
      winter: S([8, 15], 16, 8, 'cloudy:2.5 partly-cloudy:2.5 clear:2.5 rain:2.5 overcast:1.5 drizzle:1 windy:1.5 thunderstorm:0.3 fog:0.4', 'rain-front:0.2 gale:0.06'),
      spring: S([14, 22], 14, 10, 'clear:4 partly-cloudy:3 cloudy:1.5 rain:1.2 sunshower:0.6 windy:1 drizzle:0.5', 'rain-front:0.1'),
      summer: S([25, 33], 12, 11, 'clear:7 partly-cloudy:1.5 heat-wave:1.2 windy:0.8 wildfire-smoke:0.15 thunderstorm:0.15', 'heat-wave:0.1'),
      autumn: S([17, 26], 14, 9, 'clear:3.5 partly-cloudy:2.5 cloudy:1.5 rain:1.5 thunderstorm:0.8 windy:1 sunshower:0.4', 'rain-front:0.12 thunder-break:0.06')
    }
  },
  tundra: {
    name: 'Tundra', blurb: 'Polar cold: a long hard winter of snow and clear killing nights, a brief damp summer, fog off the ice.',
    latitude: 'polar', flavor: 'polar', dirs: 'N:3 NE:2 NW:2 E:1 W:1', inertia: 0.6,
    seasons: {
      winter: S([-30, -18], 20, 3, 'clear:3 partly-cloudy:2 overcast:2 snow:2 blizzard:0.8 windy:1.5 fog:0.8 cloudy:1.5', 'winter-storm:0.1 cold-snap:0.12'),
      spring: S([-18, -3], 18, 6, 'clear:3 partly-cloudy:2.5 cloudy:2 snow:2 blizzard:0.4 windy:1.5 overcast:1.5', 'winter-storm:0.06 cold-snap:0.06'),
      summer: S([2, 12], 16, 6, 'partly-cloudy:3 cloudy:3 overcast:2 drizzle:2 rain:1.5 fog:2 mist:1 clear:2 snow:0.2', 'fog-spell:0.08 rain-front:0.08'),
      autumn: S([-10, 1], 22, 4, 'overcast:3 cloudy:2.5 snow:3 sleet:1 windy:2 partly-cloudy:1.5 clear:1.5 blizzard:0.4 fog:0.8', 'winter-storm:0.1 gale:0.06')
    }
  },
  'fey-wilds': {
    name: 'Fey wilds', blurb: 'A season that never quite turns: blossom and sunshowers, glowing nights, mists that smell of honey.',
    latitude: 'mid', flavor: 'fey', dirs: 'W:2 SW:2 S:1 E:1 N:1 NE:0.5', inertia: 0.55, magic: true,
    seasons: {
      winter: S([3, 10], 10, 7, 'clear:2 partly-cloudy:2 mist:2 fog:1.5 drizzle:1 cloudy:1.5 snow:1 luminous-sky:0.6 aether-haze:0.4', 'ley-storm:0.03'),
      spring: S([10, 19], 10, 8, 'sakura-bloom:2.5 sunshower:2.5 partly-cloudy:3 clear:2.5 mist:1.2 drizzle:1 luminous-sky:0.6', 'bloom:0.15'),
      summer: S([17, 25], 8, 9, 'clear:3.5 partly-cloudy:2.5 sunshower:1.5 thunderstorm:0.6 luminous-sky:1 mist:0.6 aether-haze:0.6', 'ley-storm:0.04'),
      autumn: S([9, 18], 12, 7, 'autumn-leaves:3 mist:2 fog:1.5 partly-cloudy:2 clear:1.5 drizzle:1.5 rain:1 luminous-sky:0.6 aether-haze:0.5', 'ley-storm:0.04 fog-spell:0.05')
    }
  },
  ashlands: {
    name: 'Ashlands', blurb: 'Volcanic country: smoke on the wind, days of ashfall when the mountain speaks, bitter rain.',
    latitude: 'mid', flavor: 'ash', dirs: 'W:2 NW:2 SW:1 N:1 E:1', inertia: 0.55, magic: true,
    seasons: {
      winter: S([8, 18], 16, 8, 'overcast:2.5 ashfall:2 cloudy:2 partly-cloudy:1.5 clear:1 wildfire-smoke:1 acid-rain:0.6 windy:1 black-sun:0.15', 'ash-plume:0.12 acid-front:0.05'),
      spring: S([14, 25], 16, 10, 'ashfall:1.5 cloudy:2 partly-cloudy:2 clear:1.5 wildfire-smoke:1.2 thunderstorm:0.8 acid-rain:0.6 windy:1.2 dust-devil:0.5', 'ash-plume:0.14 acid-front:0.06'),
      summer: S([22, 34], 14, 11, 'clear:2 wildfire-smoke:2 heat-wave:1.2 ashfall:1.2 partly-cloudy:1.5 dust-devil:1 thunderstorm:0.8 black-sun:0.2', 'ash-plume:0.12 heat-wave:0.06'),
      autumn: S([14, 26], 16, 9, 'overcast:2 ashfall:1.8 cloudy:2 acid-rain:0.8 rain:0.6 wildfire-smoke:1 partly-cloudy:1.5 windy:1.2', 'ash-plume:0.12 acid-front:0.07')
    }
  },
  gloomfen: {
    name: 'Gloomfen', blurb: 'A haunted marsh: fog most mornings, drizzle most days, and now and then a mist the dead walk in.',
    latitude: 'mid', flavor: 'gloom', dirs: 'E:2 NE:2 N:1 SE:1 W:1', inertia: 0.6, magic: true,
    seasons: {
      winter: S([0, 7], 12, 5, 'fog:3 overcast:3 mist:2 drizzle:2 rolling-fog:1.5 cloudy:2 sleet:0.8 snow:0.6 gravewind:0.5 veilfall:0.4', 'veil-thinning:0.06 fog-spell:0.1'),
      spring: S([5, 14], 10, 6, 'mist:2.5 drizzle:2.5 rain:2 fog:2 overcast:2 cloudy:2 partly-cloudy:1 rolling-fog:1 spore-cloud:0.3', 'fog-spell:0.08 miasma:0.03'),
      summer: S([13, 22], 8, 7, 'mist:2 partly-cloudy:2 cloudy:2 drizzle:1.5 thunderstorm:0.8 fog:1 overcast:1.5 clear:1 spore-cloud:0.5 plague-miasma:0.2', 'miasma:0.05'),
      autumn: S([6, 14], 12, 5, 'fog:3 rolling-fog:2 mist:2 overcast:2.5 drizzle:2 rain:2 gravewind:0.8 veilfall:0.6 blood-rain:0.08', 'veil-thinning:0.08 fog-spell:0.1')
    }
  }
};

/* ── Weather engine ─────────────────────────────────────────────────────────────────────────────────
   Why it is built this way:
   - Every day's weather is a function of (seed, zone, date), not of which range was asked for: the chain
     of conditions is sampled between fixed anchor days every ANCHOR_EVERY days, and temperature and wind
     come from smooth noise keyed by the date. So a week asked for alone is that same week of the year.
   - Between two fixed days (anchors, multi-day systems, painted days, neighbouring days the caller already
     has) the chain is sampled as a bridge (forward-filtering, backward-sampling), so it bends gradually
     into whatever is fixed instead of snapping to it on the last day.
   - Transitions follow Calendaria's model (season weights, inertia, per-preset persistence) plus a
     similarity term, so clear skies cloud over before it rains rather than jumping straight to storm. */

var ANCHOR_EVERY = 32, SLOT_DAYS = 9, LOOKBACK = 40;
var COMPASS = ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW', 'NW', 'NNW'];
var COMPASS_WORDS = { N: 'north', NNE: 'north', NE: 'north-east', ENE: 'east', E: 'east', ESE: 'east', SE: 'south-east', SSE: 'south', S: 'south', SSW: 'south', SW: 'south-west', WSW: 'west', W: 'west', WNW: 'west', NW: 'north-west', NNW: 'north' };
var DIR_DEG = { N: 0, NE: 45, E: 90, SE: 135, S: 180, SW: 225, W: 270, NW: 315 };
function compassOf(deg) { return COMPASS[Math.round(mod(deg, 360) / 22.5) % 16]; }
function windTier(kph) { return kph <= 5 ? 'calm' : kph <= 20 ? 'light' : kph <= 40 ? 'moderate' : kph <= 60 ? 'strong' : kph <= 90 ? 'severe' : 'extreme'; }
var TIER_RANK = { calm: 0, light: 1, moderate: 2, strong: 3, severe: 4, extreme: 5 };

function parseWeights(w) {
  var out = {};
  if (typeof w === 'string') toks(w).forEach(function (t) { out[t.v] = t.w; });
  else if (isObj(w)) ownKeys(w).forEach(function (k) { out[k] = +w[k] || 0; });
  return out;
}

/* Checks a custom climate an owner wrote; returns plain-language problems. */
function checkClimate(c) {
  var errs = [];
  if (!isObj(c)) return ['A climate should be a set of settings with a name and seasons.'];
  if (!c.name) errs.push('Give the climate a name.');
  if (!isObj(c.seasons) || !SEASON_TYPES.some(function (s) { return c.seasons[s]; })) errs.push('A climate needs at least one season: winter, spring, summer or autumn.');
  SEASON_TYPES.forEach(function (s) {
    var se = c.seasons && c.seasons[s];
    if (!se) return;
    if (!Array.isArray(se.temp) || se.temp.length !== 2 || typeof se.temp[0] !== 'number' || typeof se.temp[1] !== 'number' || se.temp[0] > se.temp[1]) errs.push(cap(s) + '’s temperature should be a low and a high in °C, like [5, 16].');
    var w = parseWeights(se.weights), ids = ownKeys(w);
    if (!ids.length) errs.push(cap(s) + ' needs some weather: give at least one preset a weight, like "clear:3 rain:2".');
    var bad = ids.filter(function (id) { return !PRESETS[id]; });
    if (bad.length) errs.push(cap(s) + ' uses ' + joinList(bad.map(function (b) { return '“' + b + '”'; })) + ', which ' + (bad.length > 1 ? 'aren’t weather presets' : 'isn’t a weather preset') + '. Presets are Calendaria’s ids, like clear, rain, snow, fog, thunderstorm.');
    ownKeys(parseWeights(se.systems)).forEach(function (sid) { if (!SYSTEMS[sid]) errs.push(cap(s) + ' mentions a multi-day system “' + sid + '” that doesn’t exist. Try ' + joinList(ownKeys(SYSTEMS).slice(0, 6)) + '.'); });
  });
  return errs;
}

function climateSource(ref) {
  if (typeof ref === 'string') {
    if (!CLIMATES[ref]) throw GenError('There’s no climate called “' + ref + '”. The ready ones are ' + joinList(ownKeys(CLIMATES)) + '.');
    return { id: ref, def: CLIMATES[ref] };
  }
  var errs = checkClimate(ref);
  if (errs.length) throw GenError('This climate can’t be used yet: ' + errs.join(' '));
  return { id: ref.id || slugify(ref.name), def: ref };
}

/* Turns a climate plus the recipe's switches and details into what the engine samples from. kinds: the
   owner's own weather (checked); those that take part join the state space, the rest are left out entirely. */
function compileClimate(ref, recipe, kinds) {
  var src = climateSource(ref), c = src.def, mk = recipe.makes, dt = recipe.details;
  kinds = kinds || [];
  var REG = presetRegistry(kinds), active = kinds.filter(function (k) { return kindActive(k, src.id, mk); });
  var mult = function (group) { return group ? freqMult(mk[group]) : 1; };
  var seasons = {}, all = {};
  var defined = SEASON_TYPES.filter(function (s) { return c.seasons[s]; });
  SEASON_TYPES.forEach(function (st, i) {
    // A climate may skip a season; it borrows the nearest one it has.
    var se = c.seasons[st] || c.seasons[defined.slice().sort(function (a, b) { return Math.abs(SEASON_TYPES.indexOf(a) - i) - Math.abs(SEASON_TYPES.indexOf(b) - i); })[0]];
    var w = parseWeights(se.weights), out = {};
    ownKeys(w).forEach(function (id) {
      var P = PRESETS[id], v = w[id] * mult(P.group);
      if (WET_PRESETS[id]) v *= Math.exp(1.1 * dt.wetter); else if (DRY_PRESETS[id]) v *= Math.exp(-0.8 * dt.wetter * DRY_PRESETS[id]);
      if (P.group === 'wind') v *= Math.exp(0.9 * dt.windier);
      if (v > 0) { out[id] = v; all[id] = 1; }
    });
    // An owner's kind takes its season level times the average share of this season's weathers, with the
    // recipe's wetter and windier applied as they are to the preset it is like.
    if (active.length) {
      var have = ownKeys(out), avg = have.length ? sum(have.map(function (id) { return out[id]; })) / have.length : 0;
      active.forEach(function (k) {
        var v = freqMult(k.seasons[st]) * avg * freqMult(mk['kind:' + k.id] || 'normal');
        if (WET_PRESETS[k.like]) v *= Math.exp(1.1 * dt.wetter); else if (DRY_PRESETS[k.like]) v *= Math.exp(-0.8 * dt.wetter * DRY_PRESETS[k.like]);
        if (PRESETS[k.like].group === 'wind') v *= Math.exp(0.9 * dt.windier);
        if (v > 0) { out[k.id] = v; all[k.id] = 1; }
      });
    }
    var sys = {};
    ownKeys(parseWeights(se.systems)).forEach(function (sid) {
      var r = parseWeights(se.systems)[sid] * mult(SYSTEMS[sid].group) * freqMult(mk.fronts);
      if (r > 0) sys[sid] = r;
    });
    seasons[st] = {
      tmid: (se.temp[0] + se.temp[1]) / 2 + dt.warmer, tsd: Math.max(0.8, (se.temp[1] - se.temp[0]) / 4) * dt.variability,
      lo: se.temp[0] + dt.warmer, hi: se.temp[1] + dt.warmer,
      wind: (se.wind || 14) * Math.exp(0.5 * dt.windier), diurnal: se.diurnal || 8, weights: out, systems: sys,
      dirs: parseWeights(se.dirs || c.dirs || 'W:1 SW:1 NW:1 N:1 S:1 E:1')
    };
  });
  // Presets the systems can call on must be in the state space too.
  SEASON_TYPES.forEach(function (st) { ownKeys(seasons[st].systems).forEach(function (sid) { SYSTEMS[sid].steps.forEach(function (step) { step.p.split('|').forEach(function (p) { var id = p.split('@')[0]; if (freqMult(mk[PRESETS[id].group]) > 0 || !PRESETS[id].group) all[id] = 1; }); }); }); });
  var states = ownKeys(all).sort();
  if (!states.length) throw GenError('Every kind of weather this climate has is switched off, so there’s nothing left to generate. Switch at least one back on.');
  var idx = {};
  states.forEach(function (id, i) { idx[id] = i; });
  // Similar conditions follow each other more readily; wet-to-fair gets a bonus for clearing after rain.
  var aff = states.map(function (a) {
    var A = REG[a];
    return states.map(function (b) {
      var B = REG[b];
      var d = 1.4 * Math.abs(A.cloud - B.cloud) + 1.2 * Math.abs(A.wet - B.wet) + 1.6 * Math.abs(A.energy - B.energy);
      var v = 0.12 + Math.exp(-d);
      if (A.wet > 0.5 && B.cloud < 0.45 && B.wet < 0.1) v += 0.45;
      return v;
    });
  });
  var inertia = dt.continuity != null ? dt.continuity : (c.inertia || 0.55);
  var compiled = {
    id: src.id, name: c.name, def: c, latitude: c.latitude || 'mid', flavor: String(c.flavor || '').split(/\s+/).filter(Boolean), magic: !!c.magic,
    seasons: seasons, states: states, idx: idx, aff: aff, inertia: inertia,
    pers: states.map(function (id) { return clamp(inertia * REG[id].persist * 1.25, 0, 0.9); }),
    switches: mk, P: REG, kinds: active, allKinds: kinds
  };
  calibrate(compiled);
  return compiled;
}

/* The climate on a given day of the year: the two nearest canonical seasons, blended smoothly. */
function climateAt(C, theta) {
  var key = Math.round(theta * 720);
  C._cache = C._cache || {};
  if (C._cache[key]) return C._cache[key];
  var x = mod(theta, 1) * 4, i = Math.floor(x) % 4, t = smooth(x - Math.floor(x));
  var a = C.seasons[SEASON_TYPES[i]], b = C.seasons[SEASON_TYPES[(i + 1) % 4]];
  var w = C.states.map(function (id, j) { return lerp(a.cal[j], b.cal[j], t); });
  var sys = {};
  ownKeys(a.systems).concat(ownKeys(b.systems)).forEach(function (sid) { sys[sid] = lerp(a.systems[sid] || 0, b.systems[sid] || 0, t); });
  var dirs = {};
  ownKeys(a.dirs).concat(ownKeys(b.dirs)).forEach(function (k) { dirs[k] = lerp(a.dirs[k] || 0, b.dirs[k] || 0, t); });
  var p = {
    theta: theta, season: t < 0.5 ? SEASON_TYPES[i] : SEASON_TYPES[(i + 1) % 4],
    tmid: lerp(a.tmid, b.tmid, t), tsd: lerp(a.tsd, b.tsd, t), lo: lerp(a.lo, b.lo, t), hi: lerp(a.hi, b.hi, t),
    wind: lerp(a.wind, b.wind, t), diurnal: lerp(a.diurnal, b.diurnal, t), weights: w, systems: sys, dirs: dirs,
    summerness: (1 - Math.cos(2 * Math.PI * theta)) / 2
  };
  C._cache[key] = p;
  return p;
}
/* One day's transition out of state i: stay with the preset's persistence, otherwise move to another state,
   favouring similar weather. W: that day's calibrated weights. */
function transitionRow(C, from, W) {
  var S = C.states.length, s = C.pers[from], row = new Array(S), tot = 0;
  for (var j = 0; j < S; j++) { var v = j === from ? 0 : W[j] * (1 - C.pers[j]) * C.aff[from][j]; row[j] = v; tot += v; }
  if (tot <= 0) { for (j = 0; j < S; j++) row[j] = j === from ? 1 : 0; return row; }
  for (j = 0; j < S; j++) row[j] = (1 - s) * row[j] / tot;
  row[from] += s;
  return row;
}
function stationary(C, W) {
  var S = C.states.length, tot = sum(W) || 1, pi = W.map(function (w) { return w / tot; });
  var rows = C.states.map(function (x, i) { return transitionRow(C, i, W); });
  for (var it = 0; it < 150; it++) {
    var nx = new Array(S).fill(0);
    for (var i = 0; i < S; i++) if (pi[i]) for (var j = 0; j < S; j++) nx[j] += pi[i] * rows[i][j];
    pi = nx;
  }
  return pi;
}
/* Similarity makes "middling" weather (cloud, mist) easier to reach than its weight says. Scale each
   season's weights until the long-run share of days matches the owner's weights, so a weight means
   "this share of days", as in Calendaria's probability guide. */
function calibrate(C) {
  SEASON_TYPES.forEach(function (st) {
    var se = C.seasons[st], target = C.states.map(function (id) { return se.weights[id] || 0; });
    var tsum = sum(target);
    if (!tsum) { se.cal = target; return; }
    target = target.map(function (x) { return x / tsum; });
    var m = target.map(function (x) { return x > 0 ? 1 : 0; });
    for (var it = 0; it < 12; it++) {
      var W = target.map(function (x, i) { return x * m[i]; }), pi = stationary(C, W);
      for (var i = 0; i < m.length; i++) if (target[i] > 0 && pi[i] > 0) m[i] = clamp(m[i] * Math.pow(target[i] / pi[i], 0.85), 0.05, 20);
    }
    se.cal = target.map(function (x, i) { return x * m[i] * tsum; });
  });
}
function presetOffset(id, summerness, reg) { var P = (reg || PRESETS)[id]; return P ? lerp(P.dTw, P.dTs, summerness) : 0; }
/* Extremes stay believable: past a few degrees beyond the season's range, each extra degree counts less,
   so a heat wave on top of a hot spell reads as a record, not as 56 C. */
function softLimit(T, p) {
  var hi = p.hi + 5, lo = p.lo - 7;
  if (T > hi) return hi + (T - hi) * 0.35;
  if (T < lo) return lo + (T - lo) * 0.5;
  return T;
}

/* Smooth, date-keyed noise with unit variance: the day-to-day wander of temperature and wind. */
function noiseSource(seed) {
  var cache = {};
  function lattice(key, i) {
    var k = key + ':' + i;
    if (cache[k] === undefined) cache[k] = makeRng(seed, key, i).normal();
    return cache[k];
  }
  function vn(key, x) { var i = Math.floor(x), f = x - i; return lerp(lattice(key, i), lattice(key, i + 1), smooth(f)); }
  return {
    temp: function (zone, t) { return (0.85 * vn(zone + ':t1', t / 5) + 0.5 * vn(zone + ':t2', t / 2)) * 1.176; },
    wind: function (zone, t) { return vn(zone + ':w1', t / 3) * 1.16; },
    lattice: lattice
  };
}

/* Adds corrections so a smooth series passes exactly through its pinned values, fading with distance
   (the conditional mean of an AR(1) process between two known points). pins: sorted [{t, delta}]. */
function bridgeCorrection(t, pins, rho) {
  if (!pins.length) return 0;
  var i = 0;
  while (i < pins.length && pins[i].t < t) i++;
  if (i < pins.length && pins[i].t === t) return pins[i].delta;
  var L = pins[i - 1], R = pins[i];
  if (!L) return R.delta * Math.pow(rho, R.t - t);
  if (!R) return L.delta * Math.pow(rho, t - L.t);
  var n = R.t - L.t, a = t - L.t, b = R.t - t, den = 1 - Math.pow(rho, 2 * n);
  return L.delta * Math.pow(rho, a) * (1 - Math.pow(rho, 2 * b)) / den + R.delta * Math.pow(rho, b) * (1 - Math.pow(rho, 2 * a)) / den;
}

function stepChoice(step, T, rng, C) {
  var opts = step.p.split('|').map(function (o) { var q = o.split('@'); return { id: q[0], when: q[1] || null }; });
  var ok = opts.filter(function (o) {
    if (C.idx[o.id] == null) return false;
    if (o.when === 'warm') return T >= 14;
    if (o.when === 'mild') return T > 2;
    if (o.when === 'cold') return T <= 2;
    return true;
  });
  if (!ok.length) ok = opts.filter(function (o) { return C.idx[o.id] != null; });
  return ok.length ? rng.pick(ok).id : null;
}
function systemAllowed(sys, p) {
  if (!sys.when) return true;
  if (sys.when === 'cold') return p.tmid <= 4;
  if (sys.when === 'freezing') return p.tmid <= 1 && p.tmid >= -12;
  if (sys.when === 'cool') return p.tmid <= 12;
  if (sys.when === 'warm') return p.tmid >= 14;
  if (sys.when === 'hot') return p.tmid >= 20;
  if (sys.when === 'thaw') return p.tmid > -2 && p.tmid < 9 && p.season === 'spring';
  return true;
}
/* Multi-day systems: at most one per slot of SLOT_DAYS days, placed inside its slot, so each slot is
   decided on its own and the answer never depends on the range asked for. */
function systemsInWindow(C, cal, seed, zoneKey, from, to, nonceFor) {
  var out = [];
  for (var k = Math.floor(from / SLOT_DAYS); k <= Math.floor(to / SLOT_DAYS); k++) {
    var s0 = k * SLOT_DAYS, mid = s0 + Math.floor(SLOT_DAYS / 2);
    var p = climateAt(C, cal.thetaOfAbs(Math.max(1, mid)));
    var rng = makeRng(seed, 'wx-sys', zoneKey, k, nonceFor(mid));
    var cands = ownKeys(p.systems).filter(function (sid) { return p.systems[sid] > 0 && systemAllowed(SYSTEMS[sid], p); });
    var total = Math.min(0.85, sum(cands.map(function (sid) { return p.systems[sid]; })));
    var roll = rng.next();
    if (!cands.length || roll >= total) continue;
    var sid = rng.weighted(cands, function (x) { return p.systems[x]; }), sys = SYSTEMS[sid];
    var len = clamp(rng.int(sys.len[0], sys.len[1]), 1, SLOT_DAYS), start = s0 + rng.int(0, SLOT_DAYS - len);
    // Lay the steps out: lead first, tails last, the repeating step stretched to fill.
    var steps = sys.steps.slice(), plan = [];
    var fixed = steps.filter(function (s) { return !s.repeat; }).length;
    while (fixed + 1 > len && steps.length > 1) {
      var drop = steps.map(function (s, i) { return { s: s, i: i }; }).filter(function (x) { return !x.s.repeat && x.s.role === 'lead'; })[0] ||
        steps.map(function (s, i) { return { s: s, i: i }; }).filter(function (x) { return !x.s.repeat; }).slice(-1)[0];
      if (!drop) break;
      steps.splice(drop.i, 1);
      fixed = steps.filter(function (s) { return !s.repeat; }).length;
    }
    var reps = Math.max(1, len - fixed);
    steps.forEach(function (s) { if (s.repeat) for (var r = 0; r < reps; r++) plan.push(s); else plan.push(s); });
    plan = plan.slice(0, len);
    out.push({ id: sid, key: makeKey('sys', zoneKey, sid, start), label: sys.label, start: start, end: start + plan.length - 1, plan: plan, slot: k });
  }
  return out;
}

/* One zone over a scope. ctx: {cal, C, seed, zone, scope, locks: {abs: day}, context: {abs: day}, nonce}. */
function generateZone(ctx) {
  var cal = ctx.cal, C = ctx.C, seed = ctx.seed, zk = ctx.zone.id, N0 = noiseSource(seed);
  // A reroll draws fresh noise for its own days only; the neighbours it must meet are pinned below.
  var N1 = ctx.nonce ? noiseSource(seed + '|' + ctx.nonce) : N0;
  var N = { temp: function (z, t) { return (ctx.nonceSet[t] ? N1 : N0).temp(z, t); }, wind: function (z, t) { return (ctx.nonceSet[t] ? N1 : N0).wind(z, t); } };
  var scope = ctx.scope, locks = ctx.locks, context = ctx.context;
  var nonceFor = function (t) { return ctx.nonce && ctx.nonceSet[t] ? ctx.nonce : ''; };
  var first = scope.from, last = scope.to;
  var A = Math.floor((first - LOOKBACK) / ANCHOR_EVERY) * ANCHOR_EVERY, Z = Math.ceil((last + 1) / ANCHOR_EVERY) * ANCHOR_EVERY;
  A = Math.max(A, 1); if (Z <= last) Z = last + 1;
  var days = {}, S = C.states.length;
  for (var t = A; t <= Z; t++) {
    var th = cal.thetaOfAbs(t), p = climateAt(C, th);
    days[t] = { t: t, p: p, theta: th };
  }
  // Fixed days: painted (locked) days and the caller's neighbouring days outside the scope.
  var fixedDay = {};
  ownKeys(locks).forEach(function (k) { var n = +k; if (days[n]) fixedDay[n] = { src: 'lock', day: locks[k] }; });
  ownKeys(context).forEach(function (k) { var n = +k; if (days[n] && !fixedDay[n] && !scope.set[n]) fixedDay[n] = { src: 'context', day: context[k] }; });
  // Systems: dropped where they would run over a fixed day; the painted day wins.
  var systems = systemsInWindow(C, cal, seed, zk, A, Z, nonceFor).filter(function (s) {
    for (var d = s.start; d <= s.end; d++) if (fixedDay[d]) return false;
    return true;
  });
  var sysDay = {};
  systems.forEach(function (s) { s.plan.forEach(function (step, i) { sysDay[s.start + i] = { sys: s, step: step, i: i }; }); });

  // Temperature anomaly: smooth noise, then bridged onto fixed days. A painted day without a temperature
  // gets the nearest one its preset can happen at, so a summer blizzard comes with the cold that leads to it.
  var rhoT = 0.72, tPins = [], impliedT = {};
  ownKeys(fixedDay).map(Number).sort(function (a, b) { return a - b; }).forEach(function (n) {
    var fd = fixedDay[n].day, D = days[n];
    var base = D.p.tmid + N.temp(zk, n) * D.p.tsd;
    var off = fd.preset_id ? presetOffset(fd.preset_id, D.p.summerness, C.P) : 0, T0;
    if (typeof fd.temperature_celsius === 'number') T0 = fd.temperature_celsius;
    else if (fd.preset_id && C.P[fd.preset_id]) {
      var band = paintedBand(likeOf(C.P, fd.preset_id), D.p);
      T0 = band ? clamp(base + off, band[0], band[1]) : base + off;
      impliedT[n] = T0;
    } else return;
    tPins.push({ t: n, delta: T0 - off - base });
  });
  for (t = A; t <= Z; t++) {
    var Dd = days[t];
    Dd.anom = N.temp(zk, t) * Dd.p.tsd + bridgeCorrection(t, tPins, rhoT);
    Dd.sysDT = 0; Dd.sysWind = 0; Dd.veer = 0;
  }
  systems.forEach(function (s) {
    var turned = 0;
    s.plan.forEach(function (step, i) { var D = days[s.start + i]; if (!D) return; turned += step.veer || 0; D.sysDT = step.dT || 0; D.sysWind = step.wind || 0; D.veer = turned; });
  });

  // The chain of conditions. Pins: anchors, system days, fixed days with a preset.
  var pin = {};
  for (t = A; t <= Z; t++) if (t % ANCHOR_EVERY === 0 || t === A || t === Z) pin[t] = { kind: 'anchor' };
  ownKeys(sysDay).forEach(function (k) { var n = +k; if (days[n]) pin[n] = { kind: 'system' }; });
  ownKeys(fixedDay).forEach(function (k) { var n = +k; var fd = fixedDay[k].day; if (fd.preset_id) pin[n] = { kind: fixedDay[k].src, preset: fd.preset_id }; else if (pin[n] && pin[n].kind === 'anchor' && n !== A && n !== Z) delete pin[n]; });
  function weightsOn(D) {
    var z = D.anom / Math.max(0.5, D.p.tsd), T = D.p.tmid + D.anom + D.sysDT;
    return C.states.map(function (sid, i) {
      var w = D.p.weights[i];
      if (w <= 0) return 0;
      var P = C.P[sid], id = likeOf(C.P, sid); // an owner's kind answers to the thermometer as what it is like does
      if (P.precipType === 'snow' || id === 'blizzard' || id === 'permafrost-surge') w *= T <= 1 ? Math.exp(-0.35 * z) : T <= 3 ? 0.4 : 0.05;
      else if (P.precipType === 'rain' || P.precipType === 'drizzle') w *= T <= -2 ? 0.25 : 1;
      if (id === 'heat-wave') w *= Math.exp(1.1 * z) * (T >= 26 ? 1 : 0.1);
      if (id === 'thunderstorm' || id === 'hail') w *= T >= 12 ? Math.exp(0.4 * z) : 0.15;
      if (id === 'fog' || id === 'mist' || id === 'rolling-fog') w *= Math.exp(-0.25 * z);
      if (id === 'clear' && D.p.season === 'winter') w *= Math.exp(-0.2 * z); // cold snaps come with clear skies
      return w;
    });
  }
  function rowFor(D, from, W) { return transitionRow(C, from, W); }
  function stateOfPin(n) {
    var pn = pin[n], D = days[n];
    if (pn.kind === 'system') {
      var sd = sysDay[n], T = D.p.tmid + D.anom + (sd.step.dT || 0);
      return C.idx[stepChoice(sd.step, T, makeRng(seed, 'wx-step', zk, n, nonceFor(n)), C)];
    }
    if (pn.kind === 'anchor') {
      var W = weightsOn(D), r = makeRng(seed, 'wx-anchor', zk, n, nonceFor(n));
      var comp = W.map(function (w, i) { return w; });
      return C.states.indexOf(r.weighted(C.states, function (id, i) { return comp[i]; }));
    }
    return C.idx[pn.preset] != null ? C.idx[pn.preset] : null;   // a painted preset outside this climate
  }
  var softTarget = function (preset) { // likelihood over states for a painted preset the climate lacks
    var P = C.P[preset] || PRESETS.cloudy;
    return C.states.map(function (id) { var B = C.P[id]; return Math.exp(-(1.4 * Math.abs(P.cloud - B.cloud) + 1.2 * Math.abs(P.wet - B.wet) + 1.6 * Math.abs(P.energy - B.energy))); });
  };
  var pins = ownKeys(pin).map(Number).sort(function (a, b) { return a - b; });
  var stateAt = {};
  pins.forEach(function (n) { var s = stateOfPin(n); stateAt[n] = s; });
  for (var pi = 0; pi < pins.length - 1; pi++) {
    var a = pins[pi], b = pins[pi + 1];
    if (b - a < 2) continue;
    var left = stateAt[a], right = stateAt[b];
    if (left == null) { // a painted preset the climate lacks: start from its nearest likeness
      var st = softTarget(pin[a].preset), bi = 0;
      for (var q = 1; q < S; q++) if (st[q] > st[bi]) bi = q;
      left = bi;
    }
    var beta = {}, Ws = {};
    for (t = a + 1; t <= b; t++) Ws[t] = weightsOn(days[t]);
    beta[b] = right == null ? softTarget(pin[b].preset) : C.states.map(function (x, i) { return i === right ? 1 : 0; });
    for (t = b - 1; t > a; t--) {
      var nb = new Array(S), mx = 0;
      for (var i = 0; i < S; i++) {
        var row = rowFor(days[t + 1], i, Ws[t + 1]), v = 0;
        for (var j = 0; j < S; j++) v += row[j] * beta[t + 1][j];
        nb[i] = v; if (v > mx) mx = v;
      }
      for (i = 0; i < S; i++) nb[i] = mx > 0 ? nb[i] / mx : 1;
      beta[t] = nb;
    }
    var prev = left;
    for (t = a + 1; t < b; t++) {
      var r2 = rowFor(days[t], prev, Ws[t]), pr = r2.map(function (x, k) { return x * beta[t][k]; });
      var u = makeRng(seed, 'wx-chain', zk, t, nonceFor(t)), cur = u.weighted(C.states, function (id, k) { return pr[k]; });
      prev = cur == null ? prev : C.states.indexOf(cur);
      stateAt[t] = prev;
    }
  }
  // Wind: speed from smooth noise, direction from a drifting "weather direction" set every few days.
  function dirAt(n) {
    var i = Math.floor(n / 4), f = n / 4 - i;
    function latt(k) {
      var r = makeRng(seed, 'wx-dir', zk, k, nonceFor(k * 4)), p = climateAt(C, cal.thetaOfAbs(Math.max(1, k * 4)));
      var pts = ownKeys(p.dirs), pick = r.weighted(pts, function (x) { return p.dirs[x]; }) || 'W';
      return (DIR_DEG[pick] != null ? DIR_DEG[pick] : 270) + r.range(-20, 20);
    }
    return mod(angleLerp(latt(i), latt(i + 1), smooth(f)) + makeRng(seed, 'wx-dirj', zk, n).range(-12, 12), 360);
  }
  var dirPins = ownKeys(fixedDay).map(Number).filter(function (n) { var w = fixedDay[n].day.wind; return w && typeof w.direction_degrees === 'number'; });

  var out = {};
  for (t = A; t <= Z; t++) {
    var D = days[t], fd = fixedDay[t];
    var id = stateAt[t] != null ? C.states[stateAt[t]] : (fd && fd.day.preset_id) || 'cloudy';
    if (!C.P[id]) id = 'cloudy'; // a neighbouring day in weather this run wasn't given
    var T = D.p.tmid + D.anom + presetOffset(id, D.p.summerness, C.P) + D.sysDT;
    var sw = makeRng(seed, 'wx-day', zk, t, nonceFor(t));
    if (impliedT[t] != null) { id = fd.day.preset_id; T = impliedT[t]; }   // the painted day, as painted
    else {
      // Precipitation follows the thermometer: rain turns to sleet and snow in the cold, and back.
      id = phaseFor(id, T, C);
      if (id === 'heat-wave' && T < 26) id = 'clear';
      T = softLimit(D.p.tmid + D.anom + presetOffset(id, D.p.summerness, C.P) + D.sysDT, D.p);
    }
    var P = C.P[id], PH = P.custom ? PRESETS[P.like] : P; // an owner's kind wears its own icon but moves like its preset
    var low = T - D.p.diurnal * PH.diurnal * (0.85 + 0.3 * sw.next());
    var kph = D.p.wind * Math.exp(0.35 * N.wind(zk, t)) + PH.windAdd + D.sysWind;
    if (PH.forced) kph = PH.forced * (0.9 + 0.2 * sw.next());
    if (PH.glyph === 'fog') kph = Math.min(kph, 9 + 4 * sw.next());
    kph = Math.max(0, kph);
    var deg = mod(dirAt(t) + D.veer, 360);
    dirPins.forEach(function (n) { var w = Math.pow(0.6, Math.abs(n - t)); if (w > 0.05) deg = angleLerp(deg, fixedDay[n].day.wind.direction_degrees, w); });
    var sd = sysDay[t];
    var intensity = PH.precipType ? clamp(PH.intensity * (0.8 + 0.4 * sw.next()) * (sd && sd.step.role === 'peak' ? 1.15 : 1), 0.05, 1) : 0;
    out[t] = {
      abs: t, id: id, T: T, low: Math.min(low, T - 1), kph: kph, deg: deg, intensity: intensity, p: D.p, theta: D.theta,
      sys: sd ? { id: sd.sys.id, key: sd.sys.key, label: sd.sys.label, role: sd.step.role, day: sd.i + 1, of: sd.sys.plan.length, start: sd.sys.start, end: sd.sys.end } : null,
      fixed: fd ? fd.src : null
    };
  }
  return { byAbs: out, systems: systems, window: [A, Z], fixedDay: fixedDay };
}

/* The temperatures a painted preset can happen at, by the thresholds phaseFor uses: snow needs frost, rain a
   thaw, thunder some warmth, and a heat wave heat for its season. Presets with no such need return null. */
function paintedBand(id, p) {
  switch (id) {
    case 'snow': return [-40, 1];
    case 'blizzard': return [-40, -2];
    case 'permafrost-surge': return [-45, -6];
    case 'sleet': return [-2, 4];
    case 'ice-storm': return [-5, 1];
    case 'hail': return [5, 40];
    case 'thunderstorm': case 'tornado': return [9, 45];
    case 'hurricane': case 'monsoon': return [18, 45];
    case 'drizzle': case 'rain': case 'sunshower': return [2, 45];
    case 'heat-wave': return [Math.max(p.hi + 4, Math.min(26, p.hi + 10)), 55];
  }
  return null;
}
function phaseFor(id, T, C) {
  var P = PRESETS[id], allowed = function (x) { return C.switches[PRESETS[x].group] !== 'off' || !PRESETS[x].group; };
  var pick = function (want, fallback) { return allowed(want) ? want : fallback; };
  if (!P) return id;
  if (P.glyph === 'rain' && P.category === 'Standard' && id !== 'sleet' && T <= 0.5) {
    if (T <= -2) return pick(id === 'rain' || id === 'drizzle' ? 'snow' : 'snow', 'overcast');
    return pick('sleet', 'overcast');
  }
  if (id === 'thunderstorm' && T < 8) return T <= 0 ? pick('snow', 'overcast') : pick('rain', 'cloudy');
  if (id === 'hail' && T < 4) return pick('sleet', 'cloudy');
  if (id === 'snow' && T >= 3) return T < 5 ? pick('sleet', 'cloudy') : pick('rain', 'cloudy');
  if (id === 'sleet' && T >= 5) return pick('rain', 'cloudy');
  if (id === 'sleet' && T <= -3) return pick('snow', 'overcast');
  if (id === 'blizzard' && T >= 1) return T < 4 ? pick('sleet', 'windy') : pick('rain', 'windy');
  if (id === 'ice-storm' && T > 2) return pick('rain', 'cloudy');
  return id;
}

/* ── Weather in words ───────────────────────────────────────────────────────────────────────────────
   One short reading per day, the way the mockup writes it ("Sleet by evening", "Hard frost, still air").
   The words must agree with the numbers: nothing is "warm" at 4 C, and "by evening" is only said when the
   next day really is wet. Each candidate is a phrase plus a weight; the weather generator walks a spell
   through them so neighbouring days read differently, the same however long the range. */

function tempBand(T) {
  return T <= -15 ? 'bitter' : T <= -3 ? 'freezing' : T <= 3 ? 'cold' : T <= 9 ? 'chilly' : T <= 15 ? 'cool' : T <= 21 ? 'mild' : T <= 27 ? 'warm' : T <= 34 ? 'hot' : 'scorching';
}
var BAND_RANK = { bitter: 0, freezing: 1, cold: 2, chilly: 3, cool: 4, mild: 5, warm: 6, hot: 7, scorching: 8 };
/* reg: the run's presets, so an owner's kind is wet when what it is like is wet. */
function isWet(id, reg) { return !!WET_PRESETS[id] || !!(reg && reg[id] && reg[id].custom && WET_PRESETS[reg[id].like]); }
function dirWords(deg) { return 'the ' + COMPASS_WORDS[compassOf(deg)]; }

/* Candidate phrases for a day. d/prev/next are engine days; f is the climate's flavour set; reg the run's
   presets. An owner's kind speaks only in its own readings. */
function readingCandidates(d, prev, next, f, hist, reg) {
  if (reg && reg[d.id] && reg[d.id].custom) return reg[d.id].words.map(function (w) { return { text: w, w: 1 }; });
  var id = d.id, P = PRESETS[id], band = tempBand(d.T), rank = BAND_RANK[band], c = [];
  var tier = TIER_RANK[windTier(d.kph)], calm = tier <= 1, windy = tier >= 3, gale = tier >= 4;
  var from = dirWords(d.deg), season = d.p.season, z = (d.T - d.p.tmid) / Math.max(1, d.p.tsd);
  var wetPrev = prev && isWet(prev.id, reg), wetNext = next && isWet(next.id, reg), wet = isWet(id);
  var frost = d.low <= -0.5 && (id === 'clear' || id === 'partly-cloudy' || id === 'fog' || id === 'mist');
  var colderNext = next && next.T < d.T - 4, warmerNext = next && next.T > d.T + 4;
  var coast = f.coastal, desert = f.desert, high = f.highland, trop = f.tropical, gloom = f.gloom;
  var later = function (k) { return ['by evening', 'by afternoon', 'by nightfall', 'later'][k % 4]; };
  var clearing = function (k) { return ['by noon', 'by midday', 'by evening', 'later'][k % 4]; };
  var k = d.abs;
  function add(text, w) { if (text && text.length <= 34) c.push({ text: text, w: w == null ? 1 : w }); }
  var role = d.sys ? d.sys.role : null, sid = d.sys ? d.sys.id : null;

  // Systems first: they know what the day is part of, but only speak when the day agrees with them.
  var dry = !wet, g = P.glyph;
  if (sid === 'winter-storm' && role === 'lead' && dry) { add('Wind rising, snow coming', 2); if (wetNext) add('Snow by nightfall', 2); }
  if (sid === 'winter-storm' && role === 'tail' && g === 'clear') { add('Clear and bitter after the storm', 3); add('Still and white after the storm', 2); }
  if (sid === 'gale' && role === 'lead' && dry) add('Wind rising from ' + from, 3);
  if (sid === 'gale' && role === 'peak' && wet && windy) { add('Gale and driving rain', 3); add('A full gale from ' + from, 2); }
  if (sid === 'gale' && role === 'tail' && windy) add('The gale blowing itself out', 3);
  if (sid === 'cold-snap' && role === 'peak' && (g === 'clear' || g === 'fog') && rank <= 2) { add('Still, iron-hard cold', 2); if (calm && g === 'clear') add('Hard frost, still air', 2); }
  if (sid === 'thaw' && rank >= 3) { add('Mild, dripping eaves', 3); add('Snowmelt and mud', 2); }
  if (sid === 'rain-front' && role === 'lead' && dry) { add('Clouding over, rain later', 2); add('Cloud thickening from ' + from, 2); }
  if (sid === 'heat-wave' && role === 'tail' && id === 'thunderstorm') add('The heat breaks in thunder', 4);
  if (sid === 'veil-thinning' && role === 'lead' && g === 'fog') add('A grey mist that will not lift', 2);
  if (sid === 'ash-plume' && role === 'lead') add('Smoke on the wind from the mountain', 3);

  switch (id) {
    case 'clear':
      if (rank <= 1) { add(calm ? 'Hard frost, still air' : 'Bright sun, bitter wind', 2); add('Clear and bitter'); add(frost ? 'Rime on everything, blue sky' : 'Brilliant and freezing'); }
      else if (rank <= 3) { add('Clear and cold'); add('Crisp and clear'); add('Bright and cold'); add('Cold, clear, a hard blue sky'); add('Sun without warmth'); if (windy) add('Thin sun, a cold wind', 2); if (frost) { add('Frost, then thin sun', 2); add('White frost till mid-morning', 1.5); } }
      else if (rank === 4) { add('Clear and fresh'); add('Crisp and bright'); add('Blue sky, a cool breeze', windy ? 0.3 : 1); add('Cool, clean air'); add('Sunny, cool in the shade'); }
      else if (rank === 5) { add('Fine and mild'); add('Bright and mild'); add('Soft sunshine'); add('A clear, gentle day'); add('Mild sun, dry roads'); }
      else if (rank === 6) { add('Warm and bright'); add('Sunny and warm'); add(calm ? 'Blue sky, barely a breeze' : 'Warm sun, a light breeze'); add('A long, golden day'); add('Warm, dry and clear'); }
      else if (rank === 7) { add(calm ? 'Hot and still' : 'Hot and bright', 2); add('Hot sun, hard shadows'); }
      else { add(calm ? 'Scorching, not a breath of wind' : 'Blistering sun', 2); add('Furnace heat'); }
      if (desert) { if (rank >= 7) add(calm ? 'Hot, glassy calm' : 'Dry wind from the dunes', 3); if (d.low < d.T - 12) add('Cold at dawn, then glare', 2); }
      if (high && rank <= 4) add('Clear, with snow on the peaks', 2);
      if (coast && rank >= 4 && windy) add('Bright, with a stiff sea breeze', 2);
      if (wetPrev) { add(colderNext || (prev && d.T < prev.T - 3) ? 'Clearing and colder' : 'Bright after the rain', 3); if (season === 'spring' && rank >= 3) add('Mud and sunshine', 2); }
      if (hist.firstFrost) add('First frost', 5);
      if (z >= 1.4 && season === 'winter' && rank >= 4) add('A false spring', 3);
      if (z >= 1.4 && season === 'autumn') add('Summer come back for a day', 3);
      break;
    case 'partly-cloudy':
      if (rank <= 3) { add('Pale sun between clouds'); add(windy ? 'Thin sun, a cold wind' : 'Cold, with bright spells', windy ? 2 : 1); add('Cold sun, patchy cloud'); add('Brief sun, cold shadows'); add('Chilly, broken cloud'); }
      else if (rank <= 5) { add('Sun and cloud'); add('Bright spells'); add('Fair, with high cloud'); add('Cloud coming and going'); add('Fair and fresh'); }
      else { add('Warm, with drifting cloud'); add('Fair-weather cloud'); add('Sunny intervals'); add('Warm between the clouds'); add('Sun, and a few clouds'); }
      if (wetNext && !wet) add('Fair, clouding over later', 2);
      if (wetPrev) add('Brighter after the rain', 2);
      if (hist.firstFrost) add('First frost, then sun and cloud', 4);
      break;
    case 'cloudy': case 'overcast':
      if (rank <= 1) { add('Grey and bitter', 2); add('Iron-grey sky, hard cold'); add('A dead-white sky, bitter cold'); }
      else if (rank <= 3) { add('Grey and cold'); add(windy ? 'Raw and grey' : 'Low grey sky'); add('A cold, colourless day'); add(id === 'overcast' ? 'Leaden sky' : 'Dull and chilly'); }
      else if (rank <= 5) { add('Grey and mild'); add(calm ? 'Dull and still' : 'Grey, with a fresh breeze'); add('A soft grey day'); add(id === 'overcast' ? 'Overcast and quiet' : 'Cloudy but dry'); if (warmerNext) add('Overcast, softening', 2); }
      else { add(trop ? 'Low cloud, heavy air' : 'Close and grey', 2); add('Heavy, sullen sky'); add('Warm and overcast'); }
      if (wetNext && !wetPrev) { add('Clouding over from ' + from, 2); add('Thickening cloud', 1.5); }
      if (windy && season === 'autumn') add('Gusty, leaves falling', 2);
      if (windy && rank >= 2) add('Wind rattling the shutters', 1);
      if (colderNext && season !== 'summer') add('Grey, turning colder', 1.5);
      break;
    case 'drizzle':
      add(rank >= 6 ? 'Warm drizzle' : rank <= 3 ? 'Cold drizzle' : 'Fine drizzle', 2);
      add('Drizzle on and off'); add('Grey and drizzly');
      if (gloom) add('Drizzle over the reeds', 2);
      if (!wetPrev && wetNext) add('Drizzle, then rain later', 1.5);
      break;
    case 'rain': case 'sunshower': case 'monsoon':
      if (id === 'sunshower') { add('Sun through the rain', 2); add('Showers and sunshine'); add('Rain from a blue sky'); break; }
      if (id === 'monsoon') { add('Rain in torrents', 2); add('Monsoon downpours'); add('Rain without end'); break; }
      if (!wetPrev && wetNext) { add('Rain ' + later(k), 2); add('Rain moving in from ' + from, 1.5); }
      else if (!wetPrev && !wetNext) { add('A wet morning, brighter later', 1.5); add('Showers, then sun', 1.5); }
      else if (wetPrev && !wetNext) { add('Rain clearing ' + clearing(k), 2); add('Rain easing, brighter later', 1.5); }
      else { add('Steady rain'); add('Rain on and off'); add('Another wet, grey day'); add('Rain all day'); }
      if (d.intensity >= 0.7) { add(windy ? 'Driving rain' : 'Heavy rain', 2); if (trop) add('Downpour at noon', 2); }
      if (rank <= 3 && wetPrev) add('Cold, steady rain', 1.5);
      if (rank >= 6) add('Warm rain', 1.2);
      if (windy && season === 'spring') add('Blustery showers', 2);
      if (windy && coast && trop) add('Squalls off the reef', 3);
      if (trop && !wetNext) add('Brief sun between showers', 1.5);
      if (hist.recentSnow && rank >= 3) add('Rain on the snow; the thaw begins', 3);
      if (colderNext && next && next.T <= 1) add('Rain turning to sleet', 2);
      break;
    case 'snow':
      if (hist.firstSnow) add('First snow of the winter', 6);
      if (!wetPrev && wetNext) add('Snow ' + later(k), 2);
      if (d.intensity < 0.45) { add('Light snow from ' + from, 2); add('A few flakes, no more'); add('Snow flurries'); }
      else { add(windy ? 'Heavy snow, drifting' : 'Heavy snow', 2); add('Snow falling all day'); }
      if (d.T >= -0.5 && warmerNext) add('Wet snow turning to rain', 3);
      if (wetPrev && !wetNext) add('Snow easing ' + clearing(k), 1.5);
      if (high) add('Snow on the passes', 1.5);
      break;
    case 'sleet':
      add(!wetPrev ? 'Sleet by evening' : 'Sleet and slush', 2); add(windy ? 'Sleet and a raw wind' : 'Cold sleet on and off');
      break;
    case 'fog': case 'mist': case 'rolling-fog':
      if (id === 'rolling-fog') { add('Fog rolling in', 2); add('A bank of fog on the move'); }
      if (d.T <= 0 && id !== 'mist') add('Freezing fog until noon', 3);
      else if (d.T <= 0) add('Frozen mist, rime on the hedges', 2);
      else if (next && next.id === id) add(id === 'mist' ? 'Mist hanging all day' : 'Thick fog all day', 1.5);
      else add(id === 'mist' ? 'Morning mist, then sun' : 'Fog until noon', 2);
      if (coast) { add('Sea mist', 2); add('Fog off the sea'); }
      else if (high) add('Fog in the valleys', 2);
      else if (gloom) { add('Mist on the marsh', 2); add('Fog over the black water'); }
      else add('Mist over the river', 1.5);
      break;
    case 'windy': case 'autumn-leaves':
      if (id === 'autumn-leaves' || season === 'autumn') add('Gusty, leaves falling', 3);
      add(gale ? 'A gale from ' + from : 'Stiff breeze from ' + from, 2); add('Blustery and bright'); add('Wind rattling the shutters');
      if (desert) add('Dry wind from the dunes', 3);
      break;
    case 'thunderstorm':
      if (!wetPrev && prev && prev.T >= 18) add('Thunder by dusk', 3);
      add('Storms in the afternoon', 2); add('Thunder and heavy rain'); add(rank >= 6 ? 'Close and thundery' : rank <= 3 ? 'Cold rain and thunder' : 'Thundery showers');
      if (high) add('Lightning over the peaks', 2);
      break;
    case 'hail': add('Thunder and hail', 2); add('Hail showers'); add('Hailstorm at noon'); break;
    case 'blizzard': add('Blizzard: nobody travels', 2); add('White-out, drifting snow', 2); add('Blizzard from ' + from); break;
    case 'ice-storm': add('Freezing rain; everything glazed', 2); add('Ice on every branch'); break;
    case 'heat-wave': add('Heavy heat', 2); add(calm ? 'Heat haze and no wind' : 'Hot wind, no relief'); add('Stifling heat'); add('Heat that won’t break'); break;
    case 'hurricane': add('Hurricane: shutter everything', 2); add('Storm-force winds and rain'); break;
    case 'tornado': add('Funnel cloud to ' + (from === 'the south' ? 'the north' : 'the south'), 2); add('Green sky, then the twister'); break;
    case 'sandstorm': add('Sandstorm: stay under cover', 2); add('Dust storm from ' + from); add('Sand on the wind'); break;
    case 'dust-devil': add('Hot, with dust devils', 2); add('Whirlwinds of dust on the flats'); break;
    case 'ashfall': add('Ash falling like grey snow', 2); add('Ash on every sill'); add('Grey ash on the wind'); break;
    case 'wildfire-smoke': add('Smoke haze, a red sun', 2); add('Air thick with smoke'); break;
    case 'luminous-sky': add('Clear; the sky glows after dark', 2); add('Pale green light at night'); break;
    case 'sakura-bloom': add('Blossom on the wind', 2); add('Petals everywhere'); add('Mild, blossom falling'); break;
    case 'black-sun': add('The sun gone dim at noon', 2); add('A dark sun, a chill at midday'); break;
    case 'ley-surge': add('The air hums; lamps flicker', 2); add('A ley surge: magic runs wild'); break;
    case 'aether-haze': add('Shimmering haze, colours wrong', 2); add('A glittering haze'); break;
    case 'nullfront': add('Dead calm, colours dulled', 2); add('Grey and silent; nothing stirs'); break;
    case 'permafrost-surge': add('A killing cold out of nowhere', 2); add('Frost spreading at noon'); break;
    case 'gravewind': add('A wind that smells of graves', 2); add('Cold wind from the barrows'); break;
    case 'veilfall': add('A grey veil over everything', 2); add('Veilfall: voices in the mist'); break;
    case 'arcane-winds': add('Wind with sparks in it', 2); add('Arcane gusts; spells go astray'); break;
    case 'acid-rain': add('Stinging rain', 2); add('Rain that burns the leaves'); break;
    case 'blood-rain': add('Red rain', 2); add('Blood-red rain from a low sky'); break;
    case 'meteor-shower': add('Clear; falling stars tonight', 2); break;
    case 'spore-cloud': add('Spores on the air', 2); add('A yellow haze of spores'); break;
    case 'divine-light': add('Shafts of golden light', 2); add('A warm light from nowhere'); break;
    case 'plague-miasma': add('A sickly yellow mist', 2); add('Miasma over the low ground'); break;
  }
  if (!c.length) add(P.label, 1);
  if (z >= 1.6 && rank >= 3 && !wet) add('Unseasonably warm', 1.2);
  if (z <= -1.6 && !wet) add('Unseasonably cold', 1.2);
  return c;
}

/* ── The weather generator ── */

function seedOf(opts) { return opts.seed == null ? 'chronicle' : String(opts.seed); }
function byAbsMap(cal, list, zoneId, single) {
  var m = {};
  (list || []).forEach(function (d) {
    if (!d || !cal.valid(d.year, d.month, d.day)) return;
    if (d.zone_id && zoneId && d.zone_id !== zoneId) return;
    if (!d.zone_id && !single) return;
    m[cal.abs(d.year, d.month, d.day)] = d;
  });
  return m;
}

var WEATHER_MAKES = {};
ownKeys(WEATHER_SWITCHES).forEach(function (k) { var s = WEATHER_SWITCHES[k]; WEATHER_MAKES[k] = { label: s.label, help: s.help, type: 'freq', default: s.default || 'normal' }; });

defineGenerator({
  id: 'weather', label: 'Weather',
  blurb: 'Fills days with weather from a climate: seasons, spells that last, multi-day fronts and storms, and a reading in words.',
  makes: WEATHER_MAKES,
  scope: { range: true, zones: true },
  details: {
    climate: { label: 'Climate', type: 'select', default: 'temperate', options: ownKeys(CLIMATES).map(function (k) { return { value: k, label: CLIMATES[k].name }; }) },
    continuity: { label: 'How long weather lasts', type: 'range', default: 0.55, min: 0, max: 1, step: 0.05, help: '0 changes every day; 1 settles into long spells.' },
    warmer: { label: 'Warmer or colder', type: 'number', default: 0, min: -15, max: 15, step: 1, unit: '°C', more: true },
    wetter: { label: 'Wetter or drier', type: 'range', default: 0, min: -1, max: 1, step: 0.1, more: true },
    windier: { label: 'Windier or calmer', type: 'range', default: 0, min: -1, max: 1, step: 0.1, more: true },
    variability: { label: 'Day-to-day swing', type: 'range', default: 1, min: 0.3, max: 2, step: 0.1, more: true },
    forecastDays: { label: 'Forecast days', type: 'count', default: 7, min: 1, max: 30, more: true },
    forecastAccuracy: { label: 'Forecast accuracy', type: 'range', default: 0.7, min: 0, max: 1, step: 0.05, more: true, help: 'How far players can trust the forecast. The Director always sees the real weather.' },
    customClimate: { label: 'Your own climate', type: 'custom', default: null, more: true, check: function (v) { if (v == null) return null; var e = checkClimate(v); return e.length ? e.join(' ') : null; } }
  },
  builtIns: ownKeys(CLIMATES).map(function (k) { return { id: 'weather.' + k, name: CLIMATES[k].name + ' weather', description: CLIMATES[k].blurb, details: { climate: k } }; }),
  defaultRecipe: 'weather.temperate',
  run: runWeather
});

function zonesFor(opts, recipe, scope) {
  var climateRef = recipe.details.customClimate || recipe.details.climate;
  var zones = (opts.zones && opts.zones.length) ? opts.zones.map(function (z, i) {
    return { id: z.id || ('zone-' + (i + 1)), name: z.name || (z.climate && CLIMATES[z.climate] ? CLIMATES[z.climate].name : 'Zone ' + (i + 1)), climate: z.climate || climateRef };
  }) : [{ id: typeof climateRef === 'string' ? climateRef : slugify(climateRef.name), name: typeof climateRef === 'string' ? CLIMATES[climateRef].name : climateRef.name, climate: climateRef }];
  if (scope.zones) zones = zones.filter(function (z) { return scope.zones.indexOf(z.id) >= 0; });
  if (!zones.length) throw GenError('None of the zones asked for exist here' + (opts.zones ? '; this calendar’s zones are ' + joinList(opts.zones.map(function (z) { return z.id; })) : '') + '.');
  return zones;
}

function runWeather(opts) {
  var kinds = weatherKindsOf(opts);
  var recipe = resolveRecipe('weather', opts.recipe, opts);
  var cal = makeCal(opts.calendar), scope = resolveScope(cal, opts.scope), seed = seedOf(opts);
  var zones = zonesFor(opts, recipe, scope), single = zones.length === 1 && !(opts.zones && opts.zones.length > 1);
  var nonceSet = {};
  if (opts.nonce) scope.days.forEach(function (n) { nonceSet[n] = 1; });
  var allDays = [], zoneOut = [], warnings = [], statsAll = [], compiled = [];
  zones.forEach(function (zone) {
    var C = compileClimate(zone.climate, recipe, kinds);
    (opts.locked || []).forEach(function (L) {
      if (L && L.preset_id && !C.P[L.preset_id]) throw GenError('A painted day uses “' + L.preset_id + '”, which isn’t a weather preset' + (kinds.length ? ' or one of the kinds of your own weather given with this run.' : '. Pass your own weather in “kinds” to paint with it.'));
    });
    var locks = byAbsMap(cal, opts.locked, zone.id, single);
    var context = byAbsMap(cal, opts.context && opts.context.weather, zone.id, single);
    var g = generateZone({ cal: cal, C: C, seed: seed, zone: zone, scope: scope, locks: locks, context: context, nonce: opts.nonce || '', nonceSet: nonceSet });
    var f = {}; C.flavor.forEach(function (x) { f[x] = 1; });
    var memo = {};
    function hist(t) {
      var d = g.byAbs[t], h = {};
      var snowy = function (x) { return x && (x.id === 'snow' || x.id === 'blizzard' || x.id === 'sleet'); };
      var frosty = function (x) { return x && x.low <= -0.5 && (x.id === 'clear' || x.id === 'partly-cloudy' || x.id === 'fog' || x.id === 'mist'); };
      if (d.id === 'snow' && (d.p.season === 'autumn' || d.p.season === 'winter')) {
        h.firstSnow = true;
        for (var b = 1; b <= 30; b++) if (snowy(g.byAbs[t - b])) { h.firstSnow = false; break; }
      }
      if (frosty(d) && d.p.season === 'autumn') {
        h.firstFrost = true;
        for (var c2 = 1; c2 <= 25; c2++) if (frosty(g.byAbs[t - c2])) { h.firstFrost = false; break; }
      }
      for (var e = 1; e <= 5; e++) if (snowy(g.byAbs[t - e])) h.recentSnow = true;
      return h;
    }
    /* Words for a day. A spell of the same weather walks through its phrases in a fixed, weighted order
       (so three grey days read three ways), and no day repeats the two before it. To stay the same whatever
       range is asked for, that chain restarts every eighth day, where the day only steers clear of what
       the two days before would say on their own. */
    function runStart(t) { var s0 = t; while (s0 > t - 8 && g.byAbs[s0 - 1] && g.byAbs[s0 - 1].id === g.byAbs[t].id) s0--; return s0; }
    function orderFor(t) {
      if (memo['o' + t]) return memo['o' + t];
      var d = g.byAbs[t], s0 = runStart(t), tag = nonceSet[s0] ? opts.nonce : '', best = {};
      readingCandidates(d, g.byAbs[t - 1], g.byAbs[t + 1], f, hist(t), C.P).forEach(function (c) { if (!best[c.text] || best[c.text] < c.w) best[c.text] = c.w; });
      var list = ownKeys(best).filter(function (x) { return best[x] > 0; }).map(function (x) { return { text: x, k: -Math.log(Math.max(1e-9, makeRng(seed, 'wx-order', zone.id, s0, x, tag).next())) / best[x] }; });
      list.sort(function (a, b) { return a.k - b.k; });
      return (memo['o' + t] = list.map(function (x) { return x.text; }));
    }
    function alone(u) { var o = g.byAbs[u] ? orderFor(u) : []; return o.length ? o[(u - runStart(u)) % o.length] : null; }
    function wordsFor(t) {
      if (memo['w' + t] !== undefined) return memo['w' + t];
      var o = orderFor(t), reset = mod(t, 8) === 0;
      var y1 = reset ? alone(t - 1) : (g.byAbs[t - 1] ? wordsFor(t - 1) : null), y2 = reset ? alone(t - 2) : (g.byAbs[t - 2] ? wordsFor(t - 2) : null);
      var pool = o.filter(function (x) { return x !== y1 && x !== y2; });
      if (!pool.length) pool = o.filter(function (x) { return x !== y1; });
      if (!pool.length) pool = o;
      return (memo['w' + t] = pool.length ? pool[(t - runStart(t)) % pool.length] : C.P[g.byAbs[t].id].label);
    }
    var days = [];
    scope.days.forEach(function (t) {
      var date = cal.fromAbs(t);
      if (locks[t]) {
        var L = clone(locks[t]);
        var under = g.byAbs[t];
        L.gen = Object.assign({}, L.gen || {}, { key: makeKey('wx', zone.id, t), generator: 'weather', zone: zone.id, locked: true, season: under.p.season, normal_celsius: Math.round(under.p.tmid) });
        var LP = C.P[L.preset_id];
        if (LP && LP.custom) { L.gen.kind = LP.id; if (LP.look) L.gen.look = clone(LP.look); }
        if (opts.fillLocked) fillLockedDay(L, g.byAbs[t], zone, C.P);
        days.push(L);
        return;
      }
      var d = g.byAbs[t], P = C.P[d.id];
      var words = wordsFor(t);
      var kph = Math.round(d.kph);
      days.push({
        year: date.year, month: date.month, day: date.day,
        preset_id: d.id, preset_label: P.label, icon: P.glyph, color: P.color,
        temperature_celsius: Math.round(d.T),
        wind: { speed_kph: kph, speed_tier: windTier(kph), direction: compassOf(d.deg), direction_degrees: Math.round(d.deg) },
        precipitation: P.precipType ? { type: P.precipType, intensity: round2(d.intensity) } : null,
        zone_id: zone.id, zone_name: zone.name, description: words,
        gen: { key: makeKey('wx', zone.id, t), generator: 'weather', zone: zone.id, locked: false, low_celsius: Math.round(d.low), glyph: P.glyph, category: P.category, season: d.p.season, system: d.sys, normal_celsius: Math.round(d.p.tmid) }
      });
      if (P.custom) { var gk = days[days.length - 1].gen; gk.kind = P.id; if (P.look) gk.look = clone(P.look); }
    });
    var inScopeSystems = g.systems.filter(function (s) { return s.end >= scope.from && s.start <= scope.to; }).map(function (s) {
      return { key: s.key, id: s.id, label: s.label, start: cal.fromAbs(s.start), end: cal.fromAbs(s.end), days: s.end - s.start + 1, zone: zone.id };
    });
    zoneOut.push({ id: zone.id, name: zone.name, climate: C.id, climateName: C.name, days: days, systems: inScopeSystems });
    allDays = allDays.concat(days);
    statsAll.push(weatherStats(cal, C, days, inScopeSystems, locks, scope));
    compiled.push(C);
  });
  var summary = zoneOut.map(function (z, i) { return weatherSummary(cal, z, statsAll[i], recipe, scope, zoneOut.length > 1, compiled[i]); }).join(' ');
  return {
    generator: 'weather', seed: seed, recipe: recipe, scope: { label: scope.label, days: scope.days.length, kind: scope.kind },
    days: allDays, zones: zoneOut, summary: summary, stats: zoneOut.length === 1 ? statsAll[0] : statsAll, warnings: warnings.concat(scope.moonWarnings)
  };
}

/* When asked, a painted day gets the fields it lacks from the generated day under it; what was painted stays.
   reg: the run's presets, so a day painted with an owner's kind is filled from what that kind is like. */
function fillLockedDay(L, d, zone, reg) {
  if (!d) return;
  reg = reg || PRESETS;
  var filled = [], P = reg[L.preset_id] || reg[d.id];
  if (!L.preset_id) { L.preset_id = d.id; filled.push('preset_id'); }
  if (!L.preset_label) { L.preset_label = (reg[L.preset_id] || P).label; filled.push('preset_label'); }
  if (!L.icon) { L.icon = (reg[L.preset_id] || P).glyph; filled.push('icon'); }
  if (typeof L.temperature_celsius !== 'number') { L.temperature_celsius = Math.round(d.T - presetOffset(d.id, d.p.summerness, reg) + presetOffset(L.preset_id, d.p.summerness, reg)); filled.push('temperature_celsius'); }
  if (!L.wind) { var k = Math.round(reg[L.preset_id] && reg[L.preset_id].forced ? reg[L.preset_id].forced : d.kph); L.wind = { speed_kph: k, speed_tier: windTier(k), direction: compassOf(d.deg), direction_degrees: Math.round(d.deg) }; filled.push('wind'); }
  if (L.precipitation === undefined) { var Q = reg[L.preset_id]; L.precipitation = Q && Q.precipType ? { type: Q.precipType, intensity: Q.intensity } : null; filled.push('precipitation'); }
  if (!L.description) { var Q3 = reg[L.preset_id] || P; L.description = Q3.custom ? Q3.words[0] : Q3.label; filled.push('description'); }
  if (!L.zone_id) { L.zone_id = zone.id; L.zone_name = zone.name; }
  var Q2 = reg[L.preset_id] || P;
  if (L.gen.low_celsius == null) L.gen.low_celsius = Math.min(Math.round(d.low - d.T + L.temperature_celsius), L.temperature_celsius - 1);
  L.gen.glyph = Q2.glyph; L.gen.category = Q2.category;
  L.gen.filled = filled;
}

function weatherStats(cal, C, days, systems, locks, scope) {
  var st = { days: days.length, locked: 0, glyphs: {}, presets: {}, minT: null, maxT: null, bySeason: {}, byNamed: {}, systems: systems.length, longestDry: 0, longestWet: 0 };
  var run = 0, wrun = 0;
  days.forEach(function (d) {
    if (d.gen && d.gen.locked) st.locked++;
    var g = d.icon || (C.P[d.preset_id] && C.P[d.preset_id].glyph) || 'cloud';
    st.glyphs[g] = (st.glyphs[g] || 0) + 1;
    if (d.preset_id) st.presets[d.preset_id] = (st.presets[d.preset_id] || 0) + 1;
    if (typeof d.temperature_celsius === 'number') { st.minT = st.minT == null ? d.temperature_celsius : Math.min(st.minT, d.temperature_celsius); st.maxT = st.maxT == null ? d.temperature_celsius : Math.max(st.maxT, d.temperature_celsius); }
    var s = (d.gen && d.gen.season) || 'winter', b = st.bySeason[s] || (st.bySeason[s] = { n: 0, glyphs: {}, minT: 99, maxT: -99, first: d });
    // The calendar's own seasons ("The Rains", "Wintertide") when it has them, in the order the scope meets them.
    var cs = cal.seasons.length ? cal.seasonAt(d.month, d.day) : null, sn = cs ? cs.name : cap(s);
    var bn = st.byNamed[sn] || (st.byNamed[sn] = { n: 0, glyphs: {}, minT: 99, maxT: -99, order: ownKeys(st.byNamed).length });
    [b, bn].forEach(function (x) {
      x.n++; x.glyphs[g] = (x.glyphs[g] || 0) + 1;
      if (typeof d.temperature_celsius === 'number') { x.minT = Math.min(x.minT, d.temperature_celsius); x.maxT = Math.max(x.maxT, d.temperature_celsius); }
    });
    if (isWet(d.preset_id, C.P)) { wrun++; run = 0; } else { run++; wrun = 0; }
    st.longestDry = Math.max(st.longestDry, run); st.longestWet = Math.max(st.longestWet, wrun);
  });
  var changes = 0;
  for (var i = 1; i < days.length; i++) if (days[i].icon !== days[i - 1].icon) changes++;
  st.meanSpell = days.length ? round1(days.length / (changes + 1)) : 0;
  if (C.allKinds && C.allKinds.length) { // how many days each of the owner's kinds of weather got
    st.kinds = {};
    C.allKinds.forEach(function (k) { st.kinds[k.id] = 0; });
    days.forEach(function (d) { if (st.kinds[d.preset_id] !== undefined) st.kinds[d.preset_id]++; });
  }
  return st;
}

var GLYPH_WORDS = { clear: 'bright', cloud: 'grey', rain: 'wet', snow: 'snowy', storm: 'stormy', fog: 'foggy' };
var GLYPH_SPELLS = { clear: 'bright spells', cloud: 'grey days', rain: 'wet spells', snow: 'snow', storm: 'storms', fog: 'fog' };
function seasonLine(name, b) {
  var gs = ownKeys(b.glyphs).sort(function (x, y) { return b.glyphs[y] - b.glyphs[x]; });
  var top = gs[0], second = gs[1] && b.glyphs[gs[1]] >= b.n * 0.2 ? gs[1] : null;
  var feel = (b.glyphs[top] >= b.n * 0.55 ? 'mostly ' : '') + GLYPH_WORDS[top] + (second ? ', with ' + GLYPH_SPELLS[second] : '');
  var extras = [];
  if (b.glyphs.snow && top !== 'snow') extras.push(count(b.glyphs.snow, 'snowy day'));
  if (b.glyphs.storm) extras.push(count(b.glyphs.storm, 'stormy day'));
  if (b.glyphs.fog && top !== 'fog' && b.glyphs.fog >= 3) extras.push(count(b.glyphs.fog, 'day') + ' of fog');
  var plural = /^the\s.*[^s]s$/i.test(name); // "The Rains are", "The Thaw is"
  return name + (plural ? ' are ' : ' is ') + feel + ' (' + tempSpan(b.minT, b.maxT) + (extras.length ? ', ' + joinList(extras) : '') + ')';
}
function tempSpan(a, b) { return a === b ? a + '°C' : a + ' to ' + b + '°C'; }
function weatherSummary(cal, z, st, recipe, scope, multi, C) {
  var head = (multi ? z.name + ': ' : '') + (scope.kind === 'day' ? 'One day' : scope.kind === 'week' ? 'A week' : count(st.days, 'day')) +
    ' of ' + z.climateName + ' weather for ' + scope.label + '.';
  var parts = [head];
  if (st.days <= 10) {
    var bits = z.days.filter(function (d) { return !(d.gen && d.gen.locked); }).map(function (d) { return lowerFirst(d.description) + ' on the ' + d.day + ordSuffix(d.day); });
    if (bits.length) parts.push(cap(joinList(bits.slice(0, 5))) + (bits.length > 5 ? ', and so on' : '') + '; ' + tempSpan(st.minT, st.maxT) + '.');
  } else {
    var order = ownKeys(st.byNamed).filter(function (k) { return st.byNamed[k].n >= 7; }).sort(function (a, b) { return st.byNamed[a].order - st.byNamed[b].order; });
    var lines = order.map(function (k) { return seasonLine(k, st.byNamed[k]); });
    if (lines.length) parts.push(lines.join('; ') + '.');
    parts.push('Spells last about ' + st.meanSpell + ' days on average; the longest dry run is ' + count(st.longestDry, 'day') + (st.longestWet >= 3 ? ' and the longest wet one ' + count(st.longestWet, 'day') : '') + '.');
  }
  if (z.systems.length) {
    var longest = z.systems.slice().sort(function (a, b) { return b.days - a.days; })[0];
    parts.push((z.systems.length === 1 ? 'One multi-day system passes through: ' : cap(numWord(z.systems.length)) + ' multi-day systems pass through, the longest ') + lowerFirst(longest.label) + ' over ' + numWord(longest.days) + ' days from ' + cal.fmt(longest.start, false) + '.');
  }
  var mine = (C && C.allKinds) || [], nameOf = function (id) { var k = mine.filter(function (x) { return x.id === id; })[0]; return k ? k.name.toLowerCase() : id; };
  var off = ownKeys(recipe.makes).filter(function (k) { return recipe.makes[k] === 'off'; }).map(function (k) { return k.indexOf('kind:') === 0 ? nameOf(k.slice(5)) : WEATHER_SWITCHES[k].label.toLowerCase(); });
  if (off.length) parts.push('No ' + joinList(off) + ', as the recipe asks.');
  if (mine.length) parts.push(yourWeatherLine(mine, C, st, recipe));
  if (st.locked) parts.push('The ' + count(st.locked, 'day') + ' you painted ' + (st.locked === 1 ? 'was' : 'were') + ' kept exactly, and the days around ' + (st.locked === 1 ? 'it' : 'them') + ' ease into ' + (st.locked === 1 ? 'it.' : 'them.'));
  if (scope.kind === 'week' || scope.kind === 'day' || scope.kind === 'days') parts.push('These days match the same days of a full-year run with this seed, so they sit in the weather around them without a seam.');
  return parts.join(' ');
}
function ordSuffix(n) { var t = n % 100, u = n % 10; return (t > 10 && t < 14) ? 'th' : u === 1 ? 'st' : u === 2 ? 'nd' : u === 3 ? 'rd' : 'th'; }
/* What became of the owner's own weather in this run, including the kinds that could not come. */
function yourWeatherLine(mine, C, st, recipe) {
  var came = [], none = [], elsewhere = [], never = [];
  mine.forEach(function (k) {
    var nm = k.name.toLowerCase();
    if (recipe.makes['kind:' + k.id] === 'off') return; // already named as switched off
    if (k.climates && k.climates.indexOf(C.id) < 0) elsewhere.push(nm);
    else if (SEASON_TYPES.every(function (s) { return k.seasons[s] === 'off'; })) never.push(nm);
    else if (st.kinds && st.kinds[k.id]) came.push(nm + ' on ' + count(st.kinds[k.id], 'day'));
    else none.push(nm);
  });
  var out = [];
  if (came.length) out.push('Your own weather: ' + joinList(came) + '.');
  if (none.length) out.push(cap(joinList(none)) + ' didn’t come in this range.');
  if (elsewhere.length) out.push(cap(joinList(elsewhere)) + (elsewhere.length > 1 ? ' belong' : ' belongs') + ' to other climates, so ' + (elsewhere.length > 1 ? 'they never come' : 'it never comes') + ' in this one.');
  if (never.length) out.push(cap(joinList(never)) + (never.length > 1 ? ' are' : ' is') + ' off in every season.');
  return out.join(' ');
}

/* ── Forecast: the truth, blurred more the further ahead it looks. Players see this; the Director sees the truth. ── */
var FC_CATS = { clear: 'dry', cloud: 'cloud', rain: 'wet', snow: 'snow', storm: 'storm', fog: 'fog' };
function forecastWeather(opts) {
  var kinds = weatherKindsOf(opts);
  var recipe = resolveRecipe('weather', opts.recipe, opts), cal = makeCal(opts.calendar), seed = seedOf(opts);
  var today = dateOf(opts.today || { year: cal.currentYear, month: cal.currentMonth, day: cal.currentDay });
  if (!cal.validDate(today)) throw GenError('The forecast needs a real day to start from.');
  var horizon = opts.horizon || recipe.details.forecastDays, acc = opts.accuracy != null ? opts.accuracy : recipe.details.forecastAccuracy;
  var t0 = cal.absOf(today);
  var truth = opts.days;
  if (!truth) truth = runWeather({ calendar: cal, recipe: recipe, seed: seed, locked: opts.locked, zones: opts.zones, kinds: opts.kinds, scope: { range: { from: cal.fromAbs(t0 + 1), to: cal.fromAbs(t0 + horizon) } } }).days;
  var zone = opts.zone || (truth[0] && truth[0].zone_id);
  var tmap = {};
  truth.forEach(function (d) { if (!zone || d.zone_id === zone || !d.zone_id) tmap[cal.abs(d.year, d.month, d.day)] = d; });
  var C = compileClimate(recipe.details.customClimate || recipe.details.climate, recipe, kinds);
  var k = 0.34 - 0.24 * clamp(acc, 0, 1), entries = [];
  for (var lead = 1; lead <= horizon; lead++) {
    var t = t0 + lead, d = tmap[t];
    if (!d) continue;
    var conf = clamp(0.97 * Math.exp(-(lead - 1) * k), 0.18, 0.97);
    var r = makeRng(seed, 'wx-forecast', zone || '', t, lead);
    var p = climateAt(C, cal.thetaOfAbs(t)), clim = {};
    C.states.forEach(function (id, i) { var c2 = FC_CATS[C.P[id].glyph]; clim[c2] = (clim[c2] || 0) + p.weights[i]; });
    var tot = sum(ownKeys(clim).map(function (x) { return clim[x]; })) || 1;
    ownKeys(clim).forEach(function (x) { clim[x] /= tot; });
    var truthCat = FC_CATS[d.icon] || 'cloud', probs = {};
    ownKeys(clim).concat([truthCat]).forEach(function (x) { probs[x] = (1 - conf) * (clim[x] || 0) + (x === truthCat ? conf : 0); });
    var shown = r.chance(conf) ? truthCat : r.weighted(ownKeys(probs), function (x) { return probs[x]; });
    var varied = shown !== truthCat, presetId = d.preset_id;
    if (varied) { var best = null; C.states.forEach(function (id, i) { if (FC_CATS[C.P[id].glyph] === shown && (!best || p.weights[i] > p.weights[C.idx[best]])) best = id; }); presetId = best || d.preset_id; }
    var FP = C.P[presetId] || { label: d.preset_label || presetId, glyph: d.icon || 'cloud' }; // a day from weather this run wasn't given
    var sd = Math.max(1, p.tsd), center = d.temperature_celsius + r.normal() * (1 - conf) * sd * 0.8, spread = 1 + (1 - conf) * sd * 1.4;
    var wetChance = Math.round(((probs.wet || 0) + (probs.snow || 0) + (probs.storm || 0)) * 10) * 10;
    entries.push({
      year: cal.fromAbs(t).year, month: cal.fromAbs(t).month, day: cal.fromAbs(t).day, lead: lead, confidence: round2(conf),
      preset_id: presetId, preset_label: FP.label, icon: FP.glyph,
      temp_low: Math.round(center - spread), temp_high: Math.round(center + spread), precip_chance: wetChance,
      words: forecastWords(shown, conf, wetChance), varied: varied
    });
  }
  var first = entries[0], lastE = entries[entries.length - 1];
  var summary = entries.length ? ('A ' + numWord(entries.length) + '-day outlook from ' + cal.fmt(today) + ' for players. Tomorrow is ' + (first.confidence >= 0.85 ? 'close to certain' : 'fairly likely') + ' (' + Math.round(first.confidence * 100) + '%): ' + lowerFirst(first.words) + '. Confidence falls to about ' + Math.round(lastE.confidence * 100) + '% by ' + (entries.length === 1 ? 'then' : 'the ' + ordWord(entries.length) + ' day') + ', so the far end is a guess' + (entries.filter(function (e) { return e.varied; }).length ? ', and ' + count(entries.filter(function (e) { return e.varied; }).length, 'day') + ' will turn out differently from what it says' : '') + '. You always see the real weather.') : 'There is no weather yet for the days after ' + cal.fmt(today) + ' to forecast from.';
  return { generator: 'weather', kind: 'forecast', seed: seed, today: today, entries: entries, summary: summary };
}
function forecastWords(cat, conf, chance) {
  var names = { dry: ['Dry and bright', 'Probably dry', 'Mostly dry'], cloud: ['Grey and dry', 'Probably grey', 'Cloudy, maybe'], wet: ['Rain', 'Rain likely', 'Chance of rain'], snow: ['Snow', 'Snow likely', 'Chance of snow'], storm: ['Storms', 'Storms likely', 'Chance of storms'], fog: ['Fog', 'Fog likely', 'Chance of fog'] }[cat] || ['Unsettled', 'Unsettled', 'Unsettled'];
  if (conf < 0.35) return chance >= 50 ? 'Unsettled' : 'Hard to call';
  return conf >= 0.8 ? names[0] : conf >= 0.55 ? names[1] : names[2];
}

/* Calendaria-style probability guide: the share of days each preset gets in a season, as generated. */
function weatherGuide(opts) {
  opts = opts || {};
  var kinds = weatherKindsOf(opts), recipe = resolveRecipe('weather', opts.recipe, opts);
  var C = compileClimate(opts.climate || recipe.details.customClimate || recipe.details.climate, recipe, kinds);
  var season = opts.season || 'winter', theta = SEASON_THETA[season] != null ? SEASON_THETA[season] : 0;
  var p = climateAt(C, theta), S = C.states.length;
  var W = p.weights, pi = stationary(C, W), raw = C.seasons[season === 'wet' ? 'summer' : season === 'dry' ? 'winter' : season].weights;
  var rows = C.states.map(function (id, i) {
    var row = { id: id, label: C.P[id].label, glyph: C.P[id].glyph, effect: C.P[id].effect, weight: round2(raw[id] || 0), share: Math.round(pi[i] * 1000) / 10 };
    if (C.P[id].custom) row.custom = true;
    return row;
  }).filter(function (r) { return r.weight > 0; }).sort(function (a, b) { return b.share - a.share; });
  var sys = ownKeys(p.systems).map(function (sid) { return { id: sid, label: SYSTEMS[sid].label, perMonth: round1(p.systems[sid] * 30 / SLOT_DAYS) }; });
  return { climate: C.id, season: season, rows: rows, temperature: [Math.round(p.lo), Math.round(p.hi)], windKph: Math.round(p.wind), systems: sys };
}

/* A zone definition Chronicle can store (calendar_weather_zones.payload): presets with a label and a
   temperature (the minimum the old validator required), per-season overrides, and the climate itself. */
function zonePayload(climate, opts) {
  opts = opts || {};
  var kinds = weatherKindsOf(opts), recipe = resolveRecipe('weather', opts.recipe, opts);
  var C = compileClimate(climate || recipe.details.customClimate || recipe.details.climate, recipe, kinds);
  var presets = C.states.map(function (id) {
    var temps = [], wsum = 0;
    SEASON_TYPES.forEach(function (s) { var w = C.seasons[s].weights[id] || 0; if (w) { temps.push(C.seasons[s].tmid * w); wsum += w; } });
    return { id: id, label: C.P[id].label, temperature: wsum ? round1(sum(temps) / wsum) : round1(C.seasons.summer.tmid) };
  });
  var overrides = {};
  SEASON_TYPES.forEach(function (s) { var se = C.seasons[s]; overrides[s] = { temperature: [round1(se.lo), round1(se.hi)], weights: clone(se.weights), wind_kph: Math.round(se.wind) }; });
  var generator = { climate: clone(C.def), inertia: C.inertia };
  if (C.allKinds.length) generator.kinds = clone(C.kinds); // the owner's weather that can come in this zone
  return { zone_id: opts.zoneId || C.id, name: opts.name || C.name, payload: { presets: presets, season_overrides: overrides, generator: generator } };
}
/* Chronicle's WeatherInput (the flat shape PUT /calendar/weather takes). */
function toWeatherInput(d) {
  return {
    preset_id: d.preset_id || null, preset_label: d.preset_label || null, icon: d.icon || null, color: d.color || null,
    temperature_celsius: typeof d.temperature_celsius === 'number' ? d.temperature_celsius : null,
    wind_speed_kph: d.wind ? d.wind.speed_kph : null, wind_speed_tier: d.wind ? d.wind.speed_tier : null,
    wind_direction: d.wind ? d.wind.direction : null, wind_direction_degrees: d.wind ? d.wind.direction_degrees : null,
    precipitation_type: d.precipitation ? d.precipitation.type : null, precipitation_intensity: d.precipitation ? d.precipitation.intensity : null,
    zone_id: d.zone_id || null, zone_name: d.zone_name || null, description: d.description || null
  };
}
/* ── Your own weather ───────────────────────────────────────────────────────────────────────────────
   An owner's weather kind borrows a built-in preset's physics through `like` (temperature shift, wind,
   wetness, precipitation, persistence, painted temperature band) and brings its own name, icon, colour,
   readings, season levels and climates. A run that doesn't let a kind in (switched off in the recipe, off
   in every season, or another climate) never sees it, so that run is the same as one without it. */

var KIND_ICONS = ['clear', 'cloud', 'rain', 'snow', 'storm', 'fog'];
var KIND_LASTS = ['fleeting', 'normal', 'stable'];
/* The sky-effect vocabulary a look's `effect` and `accents` draw from: Calendaria's 38 HUD effects, in
   its own order (so a preset's `effect` above lines up with it), then 3 of our own. Each has a plain
   label and one of five groups, sensibly split by what the effect looks like rather than by preset
   category: Clouds and sky (cloud cover, haze, fog, light in the sky), Rain and snow (everything that
   falls, including the fantasy rains and lightning), Wind and dust (blown wind-borne stuff: sand, leaves,
   petals, spores, storms), Fire and smoke (ash, embers, smoke), Magic (the arcane and otherworldly, plus
   our own three extra accents). calendaria: false marks the 3 that aren't part of Calendaria's HUD. */
var EFFECT_ROWS = [
  ['clear', 'Clear skies', 'Clouds and sky', true], ['clouds-light', 'Light clouds', 'Clouds and sky', true],
  ['clouds-heavy', 'Heavy clouds', 'Clouds and sky', true], ['clouds-overcast', 'Overcast', 'Clouds and sky', true],
  ['rain', 'Rain', 'Rain and snow', true], ['rain-heavy', 'Heavy rain', 'Rain and snow', true],
  ['snow', 'Snow', 'Rain and snow', true], ['snow-heavy', 'Heavy snow', 'Rain and snow', true],
  ['fog', 'Fog', 'Clouds and sky', true], ['lightning', 'Lightning', 'Rain and snow', true],
  ['sand', 'Blowing sand', 'Wind and dust', true], ['ashfall', 'Ashfall', 'Fire and smoke', true],
  ['embers', 'Drifting embers', 'Fire and smoke', true], ['ice', 'Ice', 'Rain and snow', true],
  ['hail', 'Hail', 'Rain and snow', true], ['tornado', 'Tornado', 'Wind and dust', true],
  ['hurricane', 'Hurricane', 'Wind and dust', true], ['nullstatic', 'Null static', 'Magic', true],
  ['gust', 'Gusting wind', 'Wind and dust', true], ['aurora', 'Aurora', 'Clouds and sky', true],
  ['aether', 'Aether haze', 'Magic', true], ['void', 'Void', 'Magic', true],
  ['spectral', 'Spectral wind', 'Magic', true], ['arcane', 'Arcane energy', 'Magic', true],
  ['arcane-wind', 'Arcane wind', 'Magic', true], ['veil', 'The veil', 'Magic', true],
  ['petals', 'Falling petals', 'Wind and dust', true], ['sleet', 'Sleet', 'Rain and snow', true],
  ['haze', 'Heat haze', 'Clouds and sky', true], ['leaves', 'Falling leaves', 'Wind and dust', true],
  ['smoke', 'Smoke', 'Fire and smoke', true], ['rain-acid', 'Acid rain', 'Rain and snow', true],
  ['rain-blood', 'Blood rain', 'Rain and snow', true], ['meteors', 'Meteors', 'Clouds and sky', true],
  ['spores', 'Spores', 'Wind and dust', true], ['divine', 'Divine light', 'Magic', true],
  ['miasma', 'Miasma', 'Magic', true], ['ley-surge', 'Ley surge', 'Magic', true],
  ['motes', 'Drifting motes', 'Magic', false], ['sparks', 'Sparks', 'Magic', false], ['sigils', 'Floating sigils', 'Magic', false]
];
var EFFECTS = EFFECT_ROWS.map(function (r) { return r[0]; });
var EFFECT_INFO = {};
EFFECT_ROWS.forEach(function (r) { EFFECT_INFO[r[0]] = { id: r[0], label: r[1], group: r[2], calendaria: r[3] }; });
var KIND_ID_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
var HEX_RE = /^#[0-9a-fA-F]{6}$/;
var WEATHER_KIND_FIELDS = { id: 1, name: 1, icon: 1, color: 1, like: 1, seasons: 1, climates: 1, lasts: 1, words: 1, magic: 1, look: 1 };

/* Plain edit distance, for "did you mean" suggestions on a mistyped effect id. */
function editDistance(a, b) {
  a = String(a); b = String(b);
  var m = a.length, n = b.length, prev = [], cur = [];
  for (var j = 0; j <= n; j++) prev[j] = j;
  for (var i = 1; i <= m; i++) {
    cur[0] = i;
    for (var k = 1; k <= n; k++) cur[k] = a.charAt(i - 1) === b.charAt(k - 1) ? prev[k - 1] : 1 + Math.min(prev[k - 1], prev[k], cur[k - 1]);
    prev = cur.slice();
  }
  return prev[n];
}
/* The nearest id in list to a mistyped value, only when it's an obvious match: either is a prefix of the
   other (so "ash" finds "ashfall"), or the two are within a couple of letters of each other. Null when
   nothing is close enough to be worth guessing. */
function nearestOf(value, list) {
  var v = String(value == null ? '' : value).toLowerCase();
  if (!v) return null;
  var pre = list.filter(function (id) { return id !== v && (id.indexOf(v) === 0 || v.indexOf(id) === 0); });
  if (pre.length === 1) return pre[0];
  var best = null, bestD = Infinity;
  list.forEach(function (id) { var d = editDistance(v, id); if (d < bestD) { bestD = d; best = id; } });
  return best && bestD <= 2 && bestD < best.length ? best : null;
}

/* A look tells the sky renderer which of its hand-tuned effects to lay over an owner's weather: the main
   `effect` (falling back to the kind's `like` preset's effect when left out) plus up to two `accents`
   layered over it. */
function checkLook(look, label, path, errors, warnings) {
  if (look == null) return;
  if (!isObj(look)) { errors.push({ path: path, message: label + '’s look should be a set of settings, like {"effect": "arcane", "strength": 0.5}.' }); return; }
  if (look.effect != null && EFFECTS.indexOf(look.effect) < 0) {
    var se = nearestOf(look.effect, EFFECTS);
    errors.push({ path: path + '.effect', message: label + '’s effect should be one the sky can draw; ' + friendlyValue(look.effect) + ' isn’t one.' + (se ? ' Did you mean “' + se + '”?' : '') });
  }
  if (look.accents != null) {
    if (!Array.isArray(look.accents)) errors.push({ path: path + '.accents', message: label + '’s accents should be a list, like ["sparks", "veil"].' });
    else {
      if (look.accents.length > 2) errors.push({ path: path + '.accents', message: label + ' can carry at most two accents; this has ' + look.accents.length + '.' });
      var bad = look.accents.filter(function (a) { return EFFECTS.indexOf(a) < 0; });
      if (bad.length) {
        var sa = bad.length === 1 ? nearestOf(bad[0], EFFECTS) : null;
        errors.push({ path: path + '.accents', message: joinList(bad.map(friendlyValue)) + (bad.length > 1 ? ' aren’t accents' : ' isn’t an accent') + ' the sky can draw.' + (sa ? ' Did you mean “' + sa + '”?' : '') });
      }
    }
  }
  if (look.strength != null && !(typeof look.strength === 'number' && look.strength >= 0 && look.strength <= 1)) errors.push({ path: path + '.strength', message: label + '’s strength is a number from 0 (faint) to 1 (strong); ' + friendlyValue(look.strength) + ' isn’t.' });
  if (look.tint != null && !(typeof look.tint === 'string' && HEX_RE.test(look.tint))) errors.push({ path: path + '.tint', message: label + '’s tint should be a colour written like "#8a6df0".' });
  ownKeys(look).forEach(function (k) { if (k !== 'effect' && k !== 'accents' && k !== 'strength' && k !== 'tint') warnings.push({ path: path + '.' + k, message: '“' + k + '” isn’t part of a look, so it will be ignored.' }); });
}

/* Plain-language checks for one weather kind; others: the rest of the owner's kinds, for clashing ids. */
function validateWeatherKind(kind, others) {
  var errors = [], warnings = [];
  function err(path, msg) { errors.push({ path: path, message: msg }); }
  if (!isObj(kind)) return { ok: false, errors: [{ path: '', message: 'Your weather should be a set of settings: an id, a name, an icon, a colour and the built-in weather it is like.' }], warnings: [] };
  var named = typeof kind.name === 'string' && kind.name.trim(), label = named ? '“' + named + '”' : 'This weather', it = named ? label : 'this weather';
  if (typeof kind.id !== 'string' || !kind.id) err('id', 'Give ' + it + ' an id: a short name in lowercase letters, numbers and dashes, like "mana-storm".');
  else if (!KIND_ID_RE.test(kind.id)) err('id', '“' + kind.id + '” can’t be an id: use lowercase letters, numbers and single dashes, like "mana-storm".');
  else if (PRESETS[kind.id]) err('id', '“' + kind.id + '” is already a built-in weather (' + PRESETS[kind.id].label + '); give yours its own id.');
  else if ((others || []).some(function (o) { return o !== kind && isObj(o) && o.id === kind.id; })) err('id', 'Another of your weathers already uses the id “' + kind.id + '”.');
  if (typeof kind.name !== 'string' || !kind.name.trim()) err('name', 'Give your weather a name, like "Mana storm".');
  else if (kind.name.length > 40) err('name', label + ' is ' + kind.name.length + ' characters long; keep weather names under 40.');
  if (KIND_ICONS.indexOf(kind.icon) < 0) err('icon', label + ' needs an icon: clear, cloud, rain, snow, storm or fog' + (kind.icon != null ? '. ' + friendlyValue(kind.icon) + ' isn’t one of those.' : '.'));
  if (!(typeof kind.color === 'string' && HEX_RE.test(kind.color))) err('color', label + ' needs a colour written like "#7a5cff"' + (kind.color != null ? '; ' + friendlyValue(kind.color) + ' isn’t one.' : '.'));
  if (kind.like == null || kind.like === '') err('like', 'Say which built-in weather ' + it + ' is like; its temperature, wind and rain come from it. For example "rain", "fog" or "thunderstorm".');
  else if (!PRESETS[kind.like]) err('like', friendlyValue(kind.like) + ' isn’t a built-in weather ' + it + ' can borrow from. Try clear, cloudy, overcast, rain, snow, fog, windy, thunderstorm or blizzard.');
  var sea = kind.seasons, lv = {};
  if (sea != null && !isObj(sea)) err('seasons', label + '’s seasons should look like {"winter": "rare", "summer": "often"}; a season left out is normal.');
  else {
    SEASON_TYPES.forEach(function (s) {
      var v = sea && sea[s] !== undefined ? sea[s] : 'normal';
      if (FREQ_LEVELS.indexOf(v) < 0) err('seasons.' + s, label + ' in ' + s + ' can be off, rare, normal or often; ' + friendlyValue(v) + ' isn’t one of those.');
      lv[s] = v;
    });
    ownKeys(sea || {}).forEach(function (s) {
      if (SEASON_TYPES.indexOf(s) >= 0) return;
      warnings.push({ path: 'seasons.' + s, message: s === 'wet' || s === 'dry' ? 'A ' + s + ' season already uses ' + it + '’s ' + (s === 'wet' ? 'summer' : 'winter') + ' level, so “' + s + '” will be ignored.' : '“' + s + '” isn’t a season; use winter, spring, summer and autumn.' });
    });
    if (SEASON_TYPES.every(function (s) { return lv[s] === 'off'; })) warnings.push({ path: 'seasons', message: label + ' is off in every season, so it will never come. Switch a season on, or switch it off in the recipe instead.' });
  }
  if (kind.climates != null) {
    if (!Array.isArray(kind.climates) || kind.climates.some(function (c) { return typeof c !== 'string'; })) err('climates', label + '’s climates should be a list of climate ids, like ["tundra", "highland"]; leave it out for any climate.');
    else kind.climates.filter(function (c) { return !CLIMATES[c]; }).forEach(function (c) { warnings.push({ path: 'climates', message: '“' + c + '” isn’t one of the ready climates (' + joinList(ownKeys(CLIMATES)) + '); it only counts for a climate of your own with that id.' }); });
  }
  if (kind.lasts != null && KIND_LASTS.indexOf(kind.lasts) < 0) err('lasts', 'How long ' + it + ' lasts can be fleeting, normal or stable; ' + friendlyValue(kind.lasts) + ' isn’t one of those.');
  if (kind.words != null) {
    if (!Array.isArray(kind.words) || !kind.words.length) err('words', 'Give ' + it + ' one to eight readings, like ["Mana crackles in the air"].');
    else {
      if (kind.words.length > 8) err('words', label + ' has ' + kind.words.length + ' readings; keep it to eight.');
      var seenW = {};
      kind.words.forEach(function (w, i) {
        if (typeof w !== 'string' || !w.trim()) err('words[' + i + ']', 'Reading ' + (i + 1) + ' of ' + it + ' is empty.');
        else if (w.length > 34) err('words[' + i + ']', 'Reading ' + (i + 1) + ', “' + w + '”, is ' + w.length + ' characters; keep readings to 34 so they fit in a day.');
        else if (seenW[w]) warnings.push({ path: 'words[' + i + ']', message: '“' + w + '” is listed twice; once is enough.' });
        if (typeof w === 'string') seenW[w] = 1;
      });
    }
  }
  if (kind.magic != null && typeof kind.magic !== 'boolean') err('magic', 'magic is true or false.');
  checkLook(kind.look, label, 'look', errors, warnings);
  ownKeys(kind).forEach(function (k) { if (!WEATHER_KIND_FIELDS[k]) warnings.push({ path: k, message: '“' + k + '” isn’t a setting for your own weather, so it will be ignored.' }); });
  return { ok: !errors.length, errors: errors, warnings: warnings };
}

/* The kinds a run was given, checked together; any problem stops the run in the owner's own words. */
function weatherKindsOf(opts) {
  var list = opts && opts.kinds;
  if (list == null) return [];
  if (!Array.isArray(list)) throw GenError('Your own weather should be a list of kinds.');
  var errs = [];
  list.forEach(function (k) { validateWeatherKind(k, list).errors.forEach(function (e) { errs.push(e.message); }); });
  if (errs.length) throw GenError('Your own weather has ' + count(errs.length, 'problem') + ': ' + errs.join(' '));
  return list.map(function (k) {
    var seasons = {};
    SEASON_TYPES.forEach(function (s) { seasons[s] = k.seasons && k.seasons[s] !== undefined ? k.seasons[s] : 'normal'; });
    return { id: k.id, name: k.name.trim(), icon: k.icon, color: k.color, like: k.like, seasons: seasons, climates: k.climates && k.climates.length ? k.climates.slice() : null,
      lasts: k.lasts || null, words: (k.words && k.words.length ? k.words : [k.name.trim()]).map(String), magic: !!k.magic, look: k.look ? clone(k.look) : null };
  });
}
/* A custom kind's sky effect: its own look.effect when it set one, else whatever its like-preset uses. */
function kindEffect(k) { return (k.look && k.look.effect) || (PRESETS[k.like] && PRESETS[k.like].effect) || null; }
/* A kind as a preset: the physics of the preset it is like, with its own face. */
function kindPreset(k) {
  var P = Object.assign({}, PRESETS[k.like]);
  P.id = k.id; P.label = k.name; P.category = 'Yours'; P.glyph = k.icon; P.color = k.color; P.group = 'kind:' + k.id;
  if (k.lasts) { P.persistence = k.lasts; P.persist = PERSIST[k.lasts]; }
  P.custom = true; P.like = k.like; P.words = k.words; P.magic = k.magic; P.look = k.look; P.effect = kindEffect(k);
  return P;
}
/* The presets a run can use: the built-ins, plus its own kinds. With no kinds it is the built-in table itself. */
function presetRegistry(kinds) {
  if (!kinds || !kinds.length) return PRESETS;
  var reg = Object.assign({}, PRESETS);
  kinds.forEach(function (k) { reg[k.id] = kindPreset(k); });
  return reg;
}
/* Whether a kind takes part in this climate under this recipe at all. */
function kindActive(k, climateId, mk) {
  if (k.climates && k.climates.indexOf(climateId) < 0) return false;
  if (freqMult(mk['kind:' + k.id] || 'normal') <= 0) return false;
  return SEASON_TYPES.some(function (s) { return freqMult(k.seasons[s]) > 0; });
}
/* The built-in preset whose physics a preset follows: itself, or what an owner's kind is like. */
function likeOf(reg, id) { var P = reg && reg[id]; return P && P.custom ? P.like : id; }
/* The public preset list, with an owner's kinds after the built-ins. Each entry carries its Calendaria
   sky effect (a custom kind's own look.effect, else its like-preset's). */
function presetList(kinds) {
  var out = ownKeys(PRESETS).map(function (k) { var P = PRESETS[k]; return { id: k, label: P.label, category: P.category, glyph: P.glyph, color: P.color, switch: P.group, effect: P.effect }; });
  if (kinds == null) return out;
  weatherKindsOf({ kinds: kinds }).forEach(function (k) { out.push({ id: k.id, label: k.name, category: 'Yours', glyph: k.icon, color: k.color, switch: 'kind:' + k.id, custom: true, like: k.like, effect: kindEffect(k) }); });
  return out;
}
/* {list, byPreset}: the 41-effect vocabulary (id, label, group, whether Calendaria has it), and which
   effect each of the 42 built-in presets uses. */
function weatherEffects() {
  return {
    list: EFFECT_ROWS.map(function (r) { return { id: r[0], label: r[1], group: r[2], calendaria: r[3] }; }),
    byPreset: ownKeys(PRESETS).reduce(function (o, id) { o[id] = PRESETS[id].effect; return o; }, {})
  };
}

/* ── Rolling tables ──────────────────────────────────────────────────────────────────────────────────
   Format:
     { "format": "chronicle-roll-tables", "version": 1, "id": "...", "name": "...",
       "tables": [ { "id": "omens", "name": "Omens", "output": {kind, visibility, time},
                     "entries": [ "plain text", {weight, name, brief, text, roll, when, tag, tone, kind, visibility, time, days, once} ] } ] }
   An entry is either a fragment (plain string, or {text}) used inside other entries' templates, or an
   event ({name, brief}). {table} in a template rolls that table; {table|a} adds "a"/"an"; {Table}
   capitalises. {moon} {place} {person} {weekday} {month} {season} {year} {date} come from the day.
   roll: "table" makes the entry stand for a roll on another table. when: conditions on the day. */

var TABLE_FORMAT = 'chronicle-roll-tables';
var TEMPLATE_VARS = { moon: 1, place: 1, home: 1, person: 1, weekday: 1, month: 1, season: 1, year: 1, date: 1, other_moon: 1 };
var MOON_PHASES = ['full', 'new', 'waxing', 'waning', 'first-quarter', 'last-quarter', 'crescent', 'gibbous'];
var TIMES = { dawn: 6, morning: 9, noon: 12, afternoon: 15, dusk: 18, evening: 20, night: 22, midnight: 0 };

function entryObj(e) { return typeof e === 'string' ? { text: e } : e; }
function templateRefs(str) { var out = [], re = /\{([A-Za-z][\w-]*)(\|[a-z]+)?\}/g, m; while ((m = re.exec(String(str || '')))) out.push(m[1]); return out; }

/* Plain-language checks for a table set an owner wrote. opts.categories: the campaign's own categories,
   which an entry's kind may name alongside the calendar's. */
function validateTables(set, opts) {
  var errors = [], warnings = [], cats = categoryIds(opts && opts.categories);
  if (!isObj(set)) return { ok: false, errors: [{ path: '', message: 'A table set should be a JSON object with a list of tables.' }], warnings: [] };
  if (set.format && set.format !== TABLE_FORMAT) errors.push({ path: 'format', message: 'This file is a “' + set.format + '”, not a set of rolling tables.' });
  if (!Array.isArray(set.tables) || !set.tables.length) { errors.push({ path: 'tables', message: 'Add at least one table: {"id": "omens", "entries": ["..."]}.' }); return { ok: false, errors: errors, warnings: warnings }; }
  if (set.look != null) checkLook(set.look, 'This set', 'look', errors, warnings);
  var ids = {};
  set.tables.forEach(function (t, i) {
    var where = 'tables[' + i + ']';
    if (!isObj(t) || !t.id) { errors.push({ path: where, message: 'Table ' + (i + 1) + ' needs an id, a short name like “omens” that other tables can roll on.' }); return; }
    if (!/^[a-z][a-z0-9-]*$/.test(t.id)) errors.push({ path: where + '.id', message: '“' + t.id + '” can’t be a table id: use lowercase letters, numbers and dashes, starting with a letter.' });
    if (TEMPLATE_VARS[t.id]) errors.push({ path: where + '.id', message: '“' + t.id + '” is reserved for the day’s own ' + t.id + '; give the table another id.' });
    if (ids[t.id]) errors.push({ path: where + '.id', message: 'Two tables are called “' + t.id + '”; each needs its own id.' });
    ids[t.id] = t;
    if (!Array.isArray(t.entries) || !t.entries.length) errors.push({ path: where + '.entries', message: 'The table “' + t.id + '” has no entries yet.' });
    if (isObj(t.output) && t.output.announce != null) { var apT = announceProblem(t.output.announce); if (apT) errors.push({ path: where + '.output.announce', message: 'The “' + t.id + '” table’s events ' + apT }); }
  });
  set.tables.forEach(function (t, i) {
    if (!isObj(t) || !Array.isArray(t.entries)) return;
    t.entries.forEach(function (raw, j) {
      var e = entryObj(raw), where = 'tables[' + i + '].entries[' + j + ']', label = '“' + t.id + '” entry ' + (j + 1);
      if (!isObj(e)) { errors.push({ path: where, message: label + ' should be text or an object.' }); return; }
      if (e.weight != null && !(typeof e.weight === 'number' && e.weight >= 0)) errors.push({ path: where + '.weight', message: label + ' has weight ' + JSON.stringify(e.weight) + '; a weight is a number from 0 up (0 turns it off).' });
      if (!e.text && !e.name && !e.roll) errors.push({ path: where, message: label + ' is empty: give it text, a name and brief, or a table to roll on.' });
      if (e.roll && !ids[e.roll]) errors.push({ path: where + '.roll', message: label + ' rolls on “' + e.roll + '”, which isn’t a table in this set.' });
      [e.text, e.name, e.brief].forEach(function (s) {
        templateRefs(s).forEach(function (r) {
          var low = r.toLowerCase();
          if (!ids[low] && !ids[r] && !TEMPLATE_VARS[low]) errors.push({ path: where, message: label + ' uses “{' + r + '}”, which isn’t a table in this set or one of the day’s words ({moon}, {place}, {home}, {person}, {weekday}, {month}, {season}, {year}).' });
        });
      });
      if (e.when) checkWhen(e.when, label, where, errors);
      var k = e.kind || (t.output && t.output.kind);
      if (k && cats.indexOf(k) < 0) errors.push({ path: where + '.kind', message: label + ' is of kind “' + k + '”; the calendar’s kinds are ' + joinList(cats) + '.' });
      if (e.sky != null) checkSkySpec(e.sky, label, where + '.sky', errors, warnings);
      if (e.visibility && e.visibility !== 'everyone' && e.visibility !== 'dm_only') errors.push({ path: where + '.visibility', message: label + '’s visibility can be “everyone” or “dm_only”.' });
      if (e.time && TIMES[e.time] == null) errors.push({ path: where + '.time', message: label + '’s time can be ' + joinList(ownKeys(TIMES)) + '.' });
      if (e.tone && ['bright', 'grim', 'neutral'].indexOf(e.tone) < 0) errors.push({ path: where + '.tone', message: label + '’s tone can be bright, grim or neutral.' });
      if (e.announce != null) { var apE = announceProblem(e.announce); if (apE) errors.push({ path: where + '.announce', message: label + ' ' + apE }); }
    });
  });
  // A table that rolls back into itself, directly or not, would never finish.
  var visiting = {}, done = {};
  function walk(id, path) {
    if (done[id]) return;
    if (visiting[id]) { errors.push({ path: 'tables', message: 'The tables ' + path.concat([id]).map(function (x) { return '“' + x + '”'; }).join(' → ') + ' roll in a circle, so a roll would never finish.' }); return; }
    visiting[id] = 1;
    (ids[id].entries || []).forEach(function (raw) {
      var e = entryObj(raw);
      if (!isObj(e)) return;
      var refs = [].concat(e.roll ? [e.roll] : [], templateRefs(e.text), templateRefs(e.name), templateRefs(e.brief)).map(function (r) { return ids[r] ? r : r.toLowerCase(); }).filter(function (r) { return ids[r]; });
      refs.forEach(function (r) { walk(r, path.concat([id])); });
    });
    visiting[id] = 0; done[id] = 1;
  }
  ownKeys(ids).forEach(function (id) { walk(id, []); });
  return { ok: errors.length === 0, errors: errors, warnings: warnings };
}
function checkWhen(w, label, where, errors) {
  if (!isObj(w)) { errors.push({ path: where + '.when', message: label + '’s conditions should look like {"season": ["winter"]}.' }); return; }
  ownKeys(w).forEach(function (k) {
    var v = w[k];
    if (k === 'season' || k === 'month' || k === 'weekday' || k === 'weather' || k === 'climate') { if (!Array.isArray(v)) errors.push({ path: where + '.when.' + k, message: label + ': “' + k + '” should be a list, like ["winter", "autumn"].' }); }
    else if (k === 'moon') {
      if (!isObj(v)) errors.push({ path: where + '.when.moon', message: label + ': the moon condition looks like {"name": "any", "phase": "full", "within": 1}.' });
      else if (v.phase && MOON_PHASES.indexOf(v.phase) < 0) errors.push({ path: where + '.when.moon.phase', message: label + ': “' + v.phase + '” isn’t a phase; use ' + joinList(MOON_PHASES) + '.' });
    } else if (k === 'not') checkWhen(v, label, where + '.not', errors);
    else if (k === 'rest' || k === 'festival') { if (typeof v !== 'boolean') errors.push({ path: where + '.when.' + k, message: label + ': “' + k + '” is true or false.' }); }
    else if (k === 'chance') { if (!(typeof v === 'number' && v >= 0 && v <= 1)) errors.push({ path: where + '.when.chance', message: label + ': chance is a number from 0 to 1.' }); }
    else errors.push({ path: where + '.when.' + k, message: label + ' has a condition “' + k + '” this engine doesn’t know. It knows season, month, weekday, moon, weather, climate, rest, festival, chance and not.' });
  });
}

/* ── Rolling ── */
function phaseMatches(m, abs, phase, within) {
  var p = moonPhaseAt(m, abs);
  within = within == null ? 1 : within;
  switch (phase) {
    case 'full': return daysToPhase(m, abs, 0.5) <= within + 0.5;
    case 'new': return daysToPhase(m, abs, 0) <= within + 0.5;
    case 'first-quarter': return daysToPhase(m, abs, 0.25) <= within + 0.5;
    case 'last-quarter': return daysToPhase(m, abs, 0.75) <= within + 0.5;
    case 'waxing': return p > 0.03 && p < 0.47;
    case 'waning': return p > 0.53 && p < 0.97;
    case 'crescent': return (p > 0.03 && p < 0.22) || (p > 0.78 && p < 0.97);
    case 'gibbous': return (p > 0.28 && p < 0.47) || (p > 0.53 && p < 0.72);
  }
  return true;
}
function dayContext(cal, abs, extra) {
  var d = cal.fromAbs(abs), s = cal.seasonAt(d.month, d.day), wd = cal.weekday(d.year, d.month, d.day);
  return Object.assign({
    cal: cal, abs: abs, date: d, seasonType: cal.seasonTypeAt(d.year, d.month, d.day), seasonName: s ? s.name : '',
    weekday: wd >= 0 && cal.weekdays[wd] ? cal.weekdays[wd].name : '', weekdayIndex: wd, monthName: cal.months[d.month - 1].name,
    rest: cal.isRestDay(d.year, d.month, d.day), festival: cal.isIntercalary(d.month), moons: cal.moons
  }, extra || {});
}
function whenOk(w, ctx, bind) {
  if (!w) return true;
  var lc = function (x) { return String(x).toLowerCase(); };
  if (w.season && !w.season.some(function (x) { var v = lc(x); return v === ctx.seasonType || v === lc(ctx.seasonName) || (v === 'wet' && ctx.seasonName && inferSeasonType(ctx.seasonName) === 'wet') || (v === 'dry' && ctx.seasonName && inferSeasonType(ctx.seasonName) === 'dry'); })) return false;
  if (w.month && !w.month.some(function (x) { return typeof x === 'number' ? x === ctx.date.month : lc(x) === lc(ctx.monthName); })) return false;
  if (w.weekday && !w.weekday.some(function (x) { return typeof x === 'number' ? x - 1 === ctx.weekdayIndex : lc(x) === lc(ctx.weekday); })) return false;
  if (w.rest != null && !!w.rest !== !!ctx.rest) return false;
  if (w.festival != null && !!w.festival !== !!ctx.festival) return false;
  if (w.climate && (!ctx.climate || w.climate.indexOf(ctx.climate) < 0)) return false;
  if (w.weather) {
    if (!ctx.weather) return false;
    var wx = ctx.weather;
    if (!w.weather.some(function (x) { return x === wx.preset_id || x === wx.icon; })) return false;
  }
  if (w.moon) {
    var ms = (ctx.scopeMoons || ctx.moons || []).filter(function (m) { return !w.moon.name || w.moon.name === 'any' || lc(m.name) === lc(w.moon.name); });
    var ok = ms.filter(function (m) { return !w.moon.phase || phaseMatches(m, ctx.abs, w.moon.phase, w.moon.within); });
    if (!ok.length) return false;
    if (bind) bind.moon = ok[0];
  }
  if (w.not && whenOk(w.not, ctx, null)) return false;
  return true;
}
/* Rolls one table. state: {used: {entryKey: count}, depth}. Returns {name, brief, text, entry, tableId} or null. */
function rollOn(set, tableId, ctx, rng, state, mults) {
  state = state || { used: {}, depth: 0 };
  var tables = {};
  set.tables.forEach(function (t) { tables[t.id] = t; });
  var t = tables[tableId];
  if (!t || state.depth > 6) return null;
  var entries = t.entries.map(entryObj);
  var binds = [];
  var pick = rng.weighted(entries, function (e, i) {
    var w = e.weight == null ? 1 : e.weight;
    if (w <= 0) return 0;
    var b = {};
    if (!whenOk(e.when, ctx, b)) return 0;
    binds[i] = b;
    if (e.when && e.when.chance != null) w *= e.when.chance;
    if (mults) {
      if (e.tag && mults.tags && mults.tags[e.tag] != null) w *= mults.tags[e.tag];
      if (mults.tone) w *= e.tone === 'bright' ? mults.tone.bright : e.tone === 'grim' ? mults.tone.grim : 1;
    }
    var used = state.used[tableId + '#' + i] || 0;
    if (used) w *= e.once ? 0 : Math.pow(0.08, used);
    return w;
  });
  if (!pick) return null;
  var idx = entries.indexOf(pick);
  state.used[tableId + '#' + idx] = (state.used[tableId + '#' + idx] || 0) + 1;
  var local = Object.assign({}, ctx);
  if (binds[idx] && binds[idx].moon) local.boundMoon = binds[idx].moon;
  if (pick.roll) {
    state.depth++;
    var sub = rollOn(set, pick.roll, local, rng, state, mults);
    state.depth--;
    if (!sub) return null;
    return mergeOutput(sub, pick, t);
  }
  // One entry is one story: the same {word} says the same thing in its name and in its brief.
  var seen = {};
  function expand(s) {
    if (!s) return s;
    return String(s).replace(/\{([A-Za-z][\w-]*)(\|([a-z]+))?\}/g, function (m, ref, _, mod2) {
      var low = ref.toLowerCase(), capital = ref.charAt(0) !== low.charAt(0), val;
      if (seen[low] != null) val = seen[low];
      else if (tables[low] || tables[ref]) {
        state.depth++;
        var r = rollOn(set, tables[ref] ? ref : low, local, rng, state, mults);
        state.depth--;
        val = r ? (r.text || r.name || '') : '';
      } else val = contextWord(low, local, rng);
      seen[low] = val;
      if (mod2 === 'a') val = withArticle(val);
      if (mod2 === 'lower') val = lowerFirst(val);
      return capital ? cap(val) : val;
    });
  }
  var out = { text: expand(pick.text), name: expand(pick.name), brief: expand(pick.brief), entry: pick, tableId: tableId, moon: local.boundMoon ? local.boundMoon.name : (local.usedMoon || null) };
  return mergeOutput(out, pick, t);
}
function mergeOutput(out, entry, table) {
  var o = table.output || {};
  out.kind = out.kind || entry.kind || o.kind || null;
  out.visibility = out.visibility || entry.visibility || o.visibility || null;
  out.time = out.time || entry.time || o.time || null;
  out.days = out.days || entry.days || o.days || null;
  out.tag = out.tag || entry.tag || null;
  out.tone = out.tone || entry.tone || null;
  out.announce = out.announce || entry.announce || o.announce || null;
  if (entry.sky) out.sky = entry.sky;
  return out;
}
function contextWord(k, ctx, rng) {
  if (k === 'moon') { var m = ctx.boundMoon || (ctx.scopeMoons && ctx.scopeMoons[0]) || (ctx.moons && ctx.moons[0]); ctx.usedMoon = m ? m.name : null; return m ? m.name : 'the moon'; }
  if (k === 'other_moon') { var ms = (ctx.scopeMoons || ctx.moons || []).filter(function (x) { return x !== ctx.boundMoon; }); return ms.length ? ms[0].name : 'the other moon'; }
  if (k === 'place') return ctx.place ? ctx.place(rng) : 'the next town';
  if (k === 'home') return ctx.home || 'the town';
  if (k === 'person') return ctx.person ? ctx.person(rng) : 'someone';
  if (k === 'weekday') return ctx.weekday || 'market day';
  if (k === 'month') return ctx.monthName;
  if (k === 'season') return ctx.seasonName ? lowerFirst(ctx.seasonName) : ctx.seasonType;
  if (k === 'year') return String(ctx.date.year);
  if (k === 'date') return ctx.cal.fmt(ctx.date);
  return '';
}

/* ── The starter tables ─────────────────────────────────────────────────────────────────────────────
   Read-only built-ins; owners copy them to edit. Each event entry is a short name and a one-line brief a
   Director can use as it stands: a concrete detail and a reason to care, not a mood. */
var STARTER_TABLES = {
  format: TABLE_FORMAT, version: 1, id: 'starter', name: 'Starter tables', builtIn: true,
  tables: [
    { id: 'building', name: 'Buildings', entries: ['mill roof', 'temple steps', 'old gallows', 'well-head', 'bell tower', 'granary', 'toll bridge', 'churchyard wall', 'market cross', 'reeve’s house', 'tithe barn', 'ferry landing'] },
    { id: 'trade', name: 'Trades', entries: ['chandler', 'cooper', 'tanner', 'apothecary', 'farrier', 'glassblower', 'bookbinder', 'rope-maker', 'dyer', 'moneylender'] },
    { id: 'faction', name: 'Secret societies', entries: ['Grey Lantern', 'Salt Court', 'Brotherhood of the Hollow Coin', 'Quiet Company', 'Ninth Bell', 'Widow’s Hand'] },
    { id: 'goods', name: 'Goods', entries: ['pepper', 'blue dye', 'lamp oil', 'good steel', 'salt fish', 'wax', 'paper', 'saffron', 'rope', 'glass beads'] },
    { id: 'beast', name: 'Beasts', entries: ['hare', 'fox', 'badger', 'white stag', 'otter', 'heron', 'adder', 'boar'] },

    { id: 'omens', name: 'Omens', output: { kind: 'sky', visibility: 'everyone' }, entries: [
      { weight: 3, tag: 'moon', tone: 'grim', name: '{Moon} rises red', brief: '{Moon} comes up the colour of rust. The old women bar their doors; the young ones go out to look.', when: { moon: { name: 'any', phase: 'full', within: 1 } }, time: 'dusk' },
      { weight: 3, tag: 'moon', name: 'A ring around {moon}', brief: 'A wide pale halo around {moon}. Sailors say rain within three days; the sexton says a death.', when: { moon: { name: 'any', phase: 'full', within: 2 } }, time: 'night' },
      { weight: 2, tag: 'moon', name: '{Moon} seen at noon', brief: '{Moon} hangs white in the daytime sky, and nobody in {home} can agree whether that is lucky.', when: { moon: { name: 'any', phase: 'gibbous' } }, time: 'noon' },
      { weight: 2, tag: 'moon', tone: 'grim', name: 'Owls silent under the full moon', brief: 'Not one owl calls all night under the full {moon}. The gamekeeper sleeps with his lamp lit.', when: { moon: { name: 'any', phase: 'full', within: 1 } } },
      { weight: 2, tag: 'moon', tone: 'grim', name: 'Dogs howl in the dark of the moon', brief: 'Every dog in {home} howls at once, a little after midnight, and then stops.', when: { moon: { name: 'any', phase: 'new', within: 1 } }, time: 'midnight' },
      { weight: 2, tag: 'moon', tone: 'bright', name: 'The shrine well runs clear', brief: 'Under the new {moon} the old shrine well runs clear for the first time in years. Pilgrims start to arrive.', when: { moon: { name: 'any', phase: 'new', within: 1 } } },
      { weight: 2, tag: 'birds', tone: 'grim', name: 'Crows on the {building}', brief: 'Forty crows sit on the {building} from dawn to dusk and do not make a sound.' },
      { weight: 2, tag: 'birds', tone: 'bright', name: 'Swallows back early', brief: 'The swallows are back three weeks early. Farmers are sowing on the strength of it.', when: { season: ['spring'] } },
      { weight: 1, tag: 'birds', name: 'A white raven over {home}', brief: 'A white raven is seen over {home}. Half the town wants it caught; the other half wants it left alone.' },
      { weight: 1, tag: 'birds', name: 'Geese flying north in autumn', brief: 'Skeins of geese fly north, calling, when every other year they have gone south.', when: { season: ['autumn'] } },
      { weight: 1, tag: 'birds', tone: 'grim', name: 'An owl at noon', brief: 'An owl sits on the well-head at midday and watches everyone who comes to draw water.' },
      { weight: 2, tag: 'hearth', tone: 'grim', name: 'Milk sours overnight', brief: 'Every pail in the lower farms has turned by morning. Someone mutters about the new family at the mill.' },
      { weight: 2, tag: 'hearth', tone: 'grim', name: 'Candles burn blue', brief: 'Every candle in the temple burns blue for the length of one prayer, then steadies.' },
      { weight: 1, tag: 'hearth', name: 'No bread will rise', brief: 'Not one loaf in {home} rises today. The bakers meet behind closed doors.' },
      { weight: 1, tag: 'hearth', tone: 'bright', name: 'A cricket in the chimney', brief: 'A cricket sings in the inn’s chimney corner all night: a good year coming, everyone agrees.', when: { season: ['winter'] } },
      { weight: 1, tag: 'water', tone: 'grim', name: 'The mill-race stands still', brief: 'For an hour at dusk the mill-race stops dead, though the wheel keeps turning.' },
      { weight: 1, tag: 'water', name: 'A fish in the well bucket', brief: 'A silver fish comes up in the bucket at the crossroads well, where no fish could be.' },
      { weight: 1, tag: 'water', name: 'Bells under the water', brief: 'Fishermen swear they hear bells ringing under the water at slack tide.', when: { climate: ['cold-coast', 'mediterranean', 'tropical'] } },
      { weight: 1, tag: 'beasts', tone: 'bright', name: 'A calf with a star', brief: 'A calf is born at a farm near {home} with a white star on its brow. People queue to touch it for luck.', when: { season: ['spring'] } },
      { weight: 1, tag: 'beasts', tone: 'grim', name: 'Bees swarm to the gallows', brief: 'A swarm settles on the old gallows and will not be moved, smoked or sung away.' },
      { weight: 1, tag: 'beasts', name: 'A {beast} that will not run', brief: '{Beast|a} stands in the lane outside {home} and watches travellers pass, unafraid, for three days.' },
      { weight: 1, tag: 'weather', name: 'Thunder from a clear sky', brief: 'Three claps of thunder out of a clear blue sky, then nothing at all.' },
      { weight: 1, tag: 'weather', tone: 'grim', name: 'Snow in high summer', brief: 'A few flakes of snow fall at noon in the height of summer and melt before they touch the ground.', when: { season: ['summer'] } },
      { weight: 1, tag: 'dreams', tone: 'grim', name: 'The same dream', brief: 'Half of {home} wakes having dreamed of the same door. Nobody will say what was behind it.' },
      { weight: 1, tag: 'dreams', name: 'Names spoken in sleep', brief: 'A child recites a list of names in her sleep, none of them anyone in {home} knows.' }
    ] },

    { id: 'rumours', name: 'Rumours', output: { kind: 'quest', visibility: 'everyone' }, entries: [
      { weight: 2, tag: 'land', name: 'Talk that the common is sold', brief: 'Nobody has seen a deed, but three strangers were measuring the common grazing at dawn.' },
      { weight: 2, tag: 'treasure', name: 'Talk of silver near {place}', brief: 'A tinker paid for his ale in raw silver and said he found it near {place}. He was gone before sunrise.' },
      { weight: 2, tag: 'crime', name: 'Whispers about the new {trade}', brief: 'The new {trade} pays in old coin, minted before the king was crowned. Where is it coming from?' },
      { weight: 2, tag: 'strangers', tone: 'grim', name: 'Word that the ferryman is dead', brief: 'The ferry still crosses, but nobody has seen the ferryman’s face since the thaw.' },
      { weight: 2, tag: 'people', name: 'A reward for a missing son', brief: '{Person} offers a silver mark for news of a son who went to the fair and never came home.' },
      { weight: 1, tag: 'monsters', tone: 'grim', name: 'Talk of a wolf that stands up', brief: 'Shepherds on the high pasture say it rises on its hind legs to look at them.', when: { season: ['winter', 'autumn'] } },
      { weight: 2, tag: 'trade', name: 'Word of a lost caravan', brief: 'Twelve wagons of salt never reached {home}. The guild says bandits; the drivers’ wives say otherwise.' },
      { weight: 1, tag: 'people', tone: 'bright', name: 'Talk of a sudden wedding', brief: 'The reeve’s daughter is to marry a stranger from {place}. Nobody knows how it happened; everybody has a theory.' },
      { weight: 2, tag: 'monsters', tone: 'grim', name: 'Lights in the ruined keep', brief: 'Lights in the old keep two nights running. The watch refuses to go up after dark.' },
      { weight: 2, tag: 'trade', name: 'Word the bridge toll will double', brief: 'The steward was overheard saying so in the tavern. The carters are already grumbling.' },
      { weight: 1, tag: 'strangers', name: 'A physician who takes no fee', brief: 'A physician has taken rooms over the {trade}’s shop and treats anyone for free. Nobody asks why yet.' },
      { weight: 1, tag: 'nobility', name: 'Talk of a true heir', brief: 'An old woman in the almshouse says she nursed the true heir, and she still has the ring.' },
      { weight: 1, tag: 'crime', tone: 'grim', name: 'Word from the deep gallery', brief: 'The deep gallery was sealed last week. The miners were paid for the whole month and told to say nothing.' },
      { weight: 1, tag: 'faith', tone: 'grim', name: 'Whispers about the new priest', brief: 'The new priest was seen burying something under the yew tree at night.' },
      { weight: 1, tag: 'crime', name: 'The {faction} is recruiting', brief: 'People in {home} say the {faction} is looking for new hands, and paying in advance.' }
    ] },

    { id: 'market', name: 'Market happenings', output: { kind: 'downtime', visibility: 'everyone' }, entries: [
      { weight: 2, tag: 'trade', name: 'A {goods} seller from {place}', brief: 'A stranger from {place} is selling {goods} at half the usual price. The guild is not pleased.' },
      { weight: 2, tag: 'crime', name: 'Short weights at the mill stall', brief: 'The miller is caught with a hollow weight. He spends the afternoon in the stocks and his flour goes for nothing.' },
      { weight: 2, tag: 'trade', tone: 'grim', name: 'No salt at any stall', brief: 'The salt carts from the coast haven’t come. Salt prices double by noon.' },
      { weight: 1, tag: 'trade', tone: 'bright', name: 'Horse fair on the green', brief: 'Dealers bring forty horses. A grey mare goes for twice what anyone expected.', kind: 'social' },
      { weight: 2, tag: 'crime', name: 'A pickpocket caught', brief: 'The watch catches a pickpocket with six purses. One of them belongs to the reeve.' },
      { weight: 1, tag: 'news', name: 'A pedlar selling news', brief: 'A ribbon-seller is also selling news from the capital, a farthing a story, and some of it is true.' },
      { weight: 1, tag: 'fun', tone: 'bright', name: 'Cheese-rolling on the hill', brief: 'The dairymen roll a whole cheese down the hill and the whole market chases it.', kind: 'social' },
      { weight: 1, tag: 'crime', tone: 'grim', name: 'False silver at three stalls', brief: 'Clipped and false coins turn up all morning. Everyone bites their change.' },
      { weight: 1, tag: 'strangers', tone: 'grim', name: 'A stranger buys every candle', brief: 'A woman in a grey hood buys every candle in the market and pays in gold.' },
      { weight: 1, tag: 'fun', name: 'Players on a wagon-stage', brief: 'Travelling players put on a comedy about a greedy lord who looks a great deal like ours.', kind: 'social' },
      { weight: 1, tag: 'trade', tone: 'grim', name: 'Bread goes up a farthing', brief: 'The bakers put a farthing on every loaf: the harvest was thin.', when: { season: ['autumn', 'winter'] } },
      { weight: 1, tag: 'trade', tone: 'bright', name: 'Wool buyers in early', brief: 'Buyers from the south arrive early and pay well. Every shepherd in the market is suddenly cheerful.', when: { season: ['spring', 'summer'] } },
      { weight: 1, tag: 'strangers', name: 'An unopened crate at auction', brief: 'A crate sealed with a dead lord’s crest is auctioned unopened. The bidding turns ugly.' }
    ] },

    { id: 'sky-sights', name: 'Sky sights', output: { kind: 'sky', visibility: 'everyone' }, entries: [
      { weight: 2, tag: 'natural', name: 'Sun dogs at dawn', brief: 'Two false suns flank the real one for an hour after sunrise.', when: { season: ['winter', 'autumn'] }, time: 'dawn' },
      { weight: 2, tag: 'natural', name: 'A halo round the sun', brief: 'A great ring around the sun at noon. Rain before long, say the old hands.', time: 'noon' },
      { weight: 1, tag: 'natural', name: 'Night-shining cloud', brief: 'Thin silver-blue clouds glow in the north long after sunset.', when: { season: ['summer'] }, time: 'night' },
      { weight: 1, tag: 'natural', name: 'The false dawn', brief: 'A faint cone of light rises in the west after dusk, pale as milk.', when: { season: ['spring', 'autumn'] }, time: 'evening' },
      { weight: 1, tag: 'natural', name: 'A green flash at sunset', brief: 'The last sliver of the sun flashes green over the water.', when: { climate: ['cold-coast', 'mediterranean', 'tropical', 'desert'] }, time: 'dusk' },
      { weight: 2, tag: 'natural', name: 'Silent lightning', brief: 'Sheet lightning flickers along the horizon all evening without a sound.', when: { season: ['summer'] }, time: 'evening' },
      { weight: 2, tag: 'natural', name: 'A ring around {moon}', brief: 'A wide ring of pale light around {moon}, with a faint rainbow at its rim.', when: { moon: { name: 'any', phase: 'full', within: 2 } }, time: 'night' },
      { weight: 1, tag: 'natural', name: 'Sunbeams like spokes', brief: 'Shafts of light fan out from behind the hills at sunset, like the spokes of a wheel.', time: 'dusk' },
      { weight: 1, tag: 'natural', name: 'Fire on the weathervanes', brief: 'Blue flame plays on the weathervanes during the storm and burns nothing.', when: { weather: ['thunderstorm', 'storm'] }, time: 'night' },
      { weight: 2, tag: 'strange', tone: 'grim', name: 'Lights over the marsh', brief: 'Green lights drift over the marsh after dark and keep pace with anyone walking the dyke.', time: 'night' },
      { weight: 1, tag: 'strange', name: 'A star that wanders', brief: 'One star crawls across the sky all night, against the drift of all the others.', time: 'night' },
      { weight: 1, tag: 'strange', name: 'The sky ripples', brief: 'For a moment at dusk the whole sky ripples, like a sheet being shaken out.', time: 'dusk' },
      { weight: 1, tag: 'strange', tone: 'grim', name: 'A second sunset', brief: 'The sun sets, and an hour later sets again, in the north.', time: 'evening' },
      { weight: 1, tag: 'strange', tone: 'grim', name: 'Stars go out over the hills', brief: 'A patch of stars winks out over the hills and does not come back until dawn.', time: 'night' },
      { weight: 1, tag: 'strange', tone: 'grim', name: 'A doorway of light', brief: 'A tall rectangle of light stands on the ridge for a minute at midnight, then closes.', time: 'midnight' },
      { weight: 2, tag: 'strange', name: 'Colours in a moonless sky', brief: 'With {moon} dark, slow bands of violet and green move across the sky without a sound.', when: { moon: { name: 'any', phase: 'new', within: 1 } }, time: 'night' }
    ] },

    { id: 'road', name: 'Strange encounters on the road', output: { kind: 'quest', visibility: 'everyone' }, entries: [
      { weight: 2, tag: 'uncanny', name: 'An empty cart in the road', brief: 'A cart stands in the road with its oxen still yoked and calm, and nobody anywhere. The load is under a tarp.' },
      { weight: 2, tag: 'uncanny', name: 'A milestone that lies', brief: 'The milestone says {place} is four miles on. It has always said fourteen, and the paint is fresh.' },
      { weight: 1, tag: 'uncanny', tone: 'grim', name: 'Pilgrims walking away', brief: 'A line of pilgrims walks away from the shrine, not towards it, singing a funeral hymn.' },
      { weight: 2, tag: 'danger', name: 'A tollgate nobody owns', brief: 'A new tollgate across the old road, kept by two polite men with good swords and no lord’s badge.' },
      { weight: 1, tag: 'uncanny', tone: 'grim', name: 'A girl selling maps', brief: 'A girl at the crossroads sells hand-drawn maps. Some of the villages on them don’t exist. Yet.' },
      { weight: 1, tag: 'people', tone: 'bright', name: 'A wedding on the bridge', brief: 'A wedding party holds the bridge and won’t let anyone cross without a dance and a toast.' },
      { weight: 1, tag: 'uncanny', tone: 'grim', name: 'Two crows keep pace', brief: 'A pair of crows follows the party all day, never more than a field away.' },
      { weight: 1, tag: 'people', name: 'A pedlar who knows your name', brief: 'A pedlar greets each traveller by name and sells them exactly what they will need tomorrow.' },
      { weight: 2, tag: 'danger', name: 'The bridge is out', brief: 'Floodwater took the bridge. The ford is a mile upstream, and someone has already put a price on it.', when: { season: ['spring', 'autumn'] } },
      { weight: 1, tag: 'uncanny', name: 'A shrine freshly dressed', brief: 'A wayside shrine hung with fresh flowers and new paint, in a valley everyone says is empty.' },
      { weight: 1, tag: 'people', name: 'A soldier walking home', brief: 'A one-armed soldier walking home from a war nobody here has heard of.' },
      { weight: 1, tag: 'people', name: 'A hound with a silver collar', brief: 'A fine hound with a silver collar attaches itself to the party. The collar bears a crest.' },
      { weight: 1, tag: 'uncanny', name: 'Wheel ruts that stop', brief: 'Deep wheel ruts leave the road, cross a meadow and stop dead in the middle of it.' },
      { weight: 1, tag: 'uncanny', tone: 'grim', name: 'An inn that wasn’t there', brief: 'The lights of an inn where the map shows only forest: hot food, a fire and a landlord who asks nothing.', time: 'dusk' }
    ] },

    { id: 'civic', name: 'Civic happenings', output: { kind: 'social', visibility: 'everyone' }, entries: [
      { weight: 2, name: 'Council meets on the {issue}', brief: 'The council meets about the {issue}. It will vote to do nothing, at length.', time: 'morning' },
      { weight: 2, name: 'Envoy from {place}', brief: 'A trade envoy arrives from {place} with sealed letters and a long memory for insults.', time: 'noon' },
      { weight: 2, tone: 'bright', name: 'Name-day feast', brief: '{Person} turns sixty and the whole street is invited. Someone will make a speech.', time: 'evening' },
      { weight: 1, tone: 'bright', name: 'A wedding at the temple', brief: '{Person} marries a widow from {place}; her brothers watch the groom all through the vows.' },
      { weight: 1, tone: 'grim', name: 'A funeral procession', brief: '{Person} is buried. Half the town comes to pay respects and the other half to make sure.' },
      { weight: 1, name: 'The travelling court sits', brief: 'A judge hears the year’s backlog: three thefts, a boundary quarrel and a charge of witchcraft.', time: 'morning' },
      { weight: 1, name: 'A guild elects its master', brief: 'The weavers elect a new master. The vote is closer than anyone will admit.' },
      { weight: 1, tone: 'grim', name: 'A new edict read out', brief: 'A herald reads the new edict at the market cross: a tax on salt, candles and dogs.', time: 'noon' },
      { weight: 1, name: 'A duel on the common', brief: 'Two young gentlemen settle a matter of honour at dawn. The watch arrives late, on purpose.', time: 'dawn' },
      { weight: 1, name: 'Hiring fair in the square', brief: 'Farmhands and maids stand in the square with the tools of their trade, waiting to be hired for the year.', when: { season: ['autumn', 'spring'] } },
      { weight: 1, tone: 'bright', name: 'Beating the bounds', brief: 'The parish walks its boundary, and small boys are bumped on each marker stone so they’ll remember it.', when: { season: ['spring'] } }
    ] },
    { id: 'issue', name: 'Council business', entries: ['harbour chain', 'mill rights', 'new bridge', 'pig question', 'night watch', 'river dredging', 'bell tower repairs', 'common grazing'] },

    { id: 'trade-news', name: 'Trade and work', output: { kind: 'downtime', visibility: 'everyone' }, entries: [
      { weight: 2, name: 'Guild dues collected', brief: 'Every guild hall pays its quarter. Prices creep up for a week.' },
      { weight: 2, name: 'A caravan from {place}', brief: 'Twenty wagons in from {place} with cloth, dye and news. The inns fill up by dusk.' },
      { weight: 1, tone: 'grim', name: 'The mill stops for repairs', brief: 'The mill-wheel is out for a week. Flour gets dear and tempers short.' },
      { weight: 2, name: 'Shearing begins', brief: 'The shearing gangs arrive; for a week every barn is full of wool and swearing.', when: { season: ['spring'] } },
      { weight: 2, tone: 'bright', name: 'Hay in on the water-meadows', brief: 'The hay is cut and in before the rain, just. Cider for everyone who helped.', when: { season: ['summer'] } },
      { weight: 2, name: 'Harvest hands wanted', brief: 'Every farm is short of hands. Wages double and the taverns empty.', when: { season: ['autumn'] } },
      { weight: 2, tone: 'grim', name: 'The pig-killing', brief: 'The pigs are killed and salted. Every yard smells of smoke and brine for days.', when: { season: ['autumn', 'winter'] } },
      { weight: 1, tone: 'bright', name: 'Cider pressing', brief: 'The presses run all day at the orchard farms; the first cider is terrible and everyone drinks it.', when: { season: ['autumn'] } },
      { weight: 1, name: 'Ice-cutting on the mill pond', brief: 'Blocks of ice are sawn from the pond and packed in straw for the summer.', when: { season: ['winter'] } },
      { weight: 1, name: 'The ferry laid up', brief: 'The ferry stays on the bank until the ice is surveyed. The long road round adds a day.', when: { season: ['winter'] } },
      { weight: 1, name: 'Nets mended on the quay', brief: 'The whole quay turns out to mend nets before the next storm.', when: { climate: ['cold-coast', 'mediterranean', 'tropical'] } },
      { weight: 1, tone: 'grim', name: 'Coin runs short', brief: 'There is not enough small silver in town; tradesmen start taking tallies and promises.' }
    ] },

    { id: 'troubles', name: 'Troubles', output: { kind: 'quest', visibility: 'everyone' }, entries: [
      { weight: 2, name: 'Sheep taken from the high fold', brief: 'Three ewes gone overnight: no blood, no tracks. The shepherd wants someone to sit up with him.' },
      { weight: 1, tone: 'grim', name: 'A child missing in the woods', brief: 'A girl didn’t come home from the woods. Her basket was found full, set neatly on a stump.' },
      { weight: 2, name: 'Barn fire outside {home}', brief: 'A barn burns just outside {home}. The owner says it was set; his neighbour has fitted a new lock.' },
      { weight: 2, name: 'Robbery on the south road', brief: 'Two carters robbed in a week, both by a woman on a grey horse who apologised.' },
      { weight: 1, tone: 'grim', name: 'The well is fouled', brief: 'The main well stinks of something dead. Water must be carried up from the river until it’s found.' },
      { weight: 1, tone: 'grim', name: 'A body in the reeds', brief: 'A drowned man lies in the reeds below the mill, well dressed, with no purse and no name.' },
      { weight: 2, name: 'Wolves at the folds', brief: 'Wolves come down to the folds two nights running. The reeve offers a bounty on every pelt.', when: { season: ['winter'] } },
      { weight: 1, name: 'A knife-fight at the inn', brief: 'Drovers and bargemen fight at the inn and a man is stabbed. Both sides want justice, meaning revenge.' },
      { weight: 1, tone: 'grim', name: 'Graves opened and closed', brief: 'Three graves in the churchyard have been opened and carefully filled in again.' },
      { weight: 1, name: 'The tax collector is late', brief: 'The collector hasn’t arrived and nobody knows whether to be glad or afraid.' },
      { weight: 1, visibility: 'dm_only', tone: 'grim', name: 'The {faction} meets', brief: 'While the town sleeps, the {faction} meets in the old tannery. The party could overhear.', time: 'midnight' },
      { weight: 1, visibility: 'dm_only', name: 'A spy in the reeve’s house', brief: 'The reeve’s new clerk copies every letter that crosses his desk and sends the copies to {place}.' }
    ] },

    { id: 'nature', name: 'Seasons and nature', output: { kind: 'sky', visibility: 'everyone' }, entries: [
      { weight: 2, name: 'The mill pond freezes hard', brief: 'The pond freezes thick enough to walk on: first children, then skaters, then fools with carts.', when: { season: ['winter'] } },
      { weight: 1, name: 'Snow on the far hills', brief: 'The far hills turn white overnight. The drovers start bringing the flocks down.', when: { season: ['autumn', 'winter'] } },
      { weight: 2, name: 'The river ice breaks', brief: 'The river ice breaks with a noise like a battle. The ferry will run again in a week.', when: { season: ['spring'] } },
      { weight: 2, tone: 'bright', name: 'The swallows return', brief: 'The swallows are back under every eave. Spring is official.', when: { season: ['spring'] } },
      { weight: 1, tone: 'bright', name: 'Blossom in the orchards', brief: 'The orchards are white with blossom for three days. Everyone prays for no frost.', when: { season: ['spring'] } },
      { weight: 1, name: 'The river runs low', brief: 'The river is lower than anyone remembers; old things are showing in the mud.', when: { season: ['summer'] } },
      { weight: 1, tone: 'bright', name: 'Glow-worms on the lanes', brief: 'The hedges along the lanes are full of glow-worms. Courting couples take the long way home.', when: { season: ['summer'] } },
      { weight: 2, name: 'Leaves turning on the ridge', brief: 'The beeches on the ridge have turned copper overnight.', when: { season: ['autumn'] } },
      { weight: 1, name: 'Geese going over', brief: 'Great skeins of geese go over all day, calling. The winter is coming early.', when: { season: ['autumn'] } },
      { weight: 1, name: 'Deer rutting in the forest', brief: 'The stags are roaring in the forest at night. Nobody goes in after dusk.', when: { season: ['autumn'] } },
      { weight: 2, tone: 'bright', name: 'The first rains', brief: 'The first real rain in months. Everyone stands out in it, whatever they are wearing.', when: { season: ['wet'] } },
      { weight: 1, name: 'The wells run low', brief: 'The shallow wells are dry; the long queue at the deep well starts before dawn.', when: { season: ['dry'] } }
    ] },

    { id: 'customs-midwinter', name: 'Customs: midwinter', entries: [
      'Fires burn on every hill until dawn, and nobody sleeps before the bells.',
      'Every household sets a candle in the window for the year\u2019s dead.',
      'The longest night is kept awake: stories by the fire until the sky greys, and a prize for the last one to nod off.',
      'Evergreen boughs over every door, and a log that must burn from dusk to dawn without going out.'] },
    { id: 'customs-midsummer', name: 'Customs: midsummer', entries: [
      'Oaths are renewed at noon and small debts forgiven at dusk.',
      'Bonfires on the hills; the young leap them for luck and the old count the scorch marks.',
      'Nobody works after noon, and the lord pays for the ale.',
      'Wreaths of flowers go into the river at sunset; whoever\u2019s floats farthest has the luck of the year.'] },
    { id: 'customs-spring', name: 'Customs: the spring turn', entries: [
      'The oldest farmer cuts the first furrow while the youngest scatters salt behind him.',
      'Houses are swept out and the dust thrown to the wind, and nobody lends anything all day.',
      'The flocks are driven between two fires to keep them from harm for the year.',
      'Children go door to door with green branches and are paid in eggs.'] },
    { id: 'customs-autumn', name: 'Customs: the autumn turn', entries: [
      'Scales hang at the market cross: everything is weighed and nothing is sold.',
      'Accounts are settled before sunset; a debt carried past the turn is bad luck for both.',
      'The last apples are left on the trees for whoever walks the orchards at night.'] },
    { id: 'customs-harvest', name: 'Customs: harvest', entries: [
      'The last sheaf is dressed in ribbons and carried to the feast on a cart.',
      'Tithes are counted in the great barn, and the clerk counts them twice.',
      'Everyone eats at long tables in the barn, and the landowner waits on the reapers.',
      'A loaf is baked from the first grain and broken over the threshold of every house.'] },
    { id: 'customs-newyear', name: 'Customs: the new year', entries: [
      'Doors are opened at dawn to let the old year out and the new one in.',
      'The first person over the threshold after midnight decides the year\u2019s luck; dark-haired strangers are paid to call.',
      'Every quarrel still open is supposed to end today, and a surprising number do.'] },
    { id: 'customs-remembrance', name: 'Customs: remembrance', entries: [
      'The names of the year\u2019s dead are read aloud, and a cup is poured on the ground for each.',
      'Lanterns are set on the graves at dusk, and nobody walks the lanes after dark.',
      'A place is laid at every table for someone who isn\u2019t coming.'] },
    { id: 'customs-moon', name: 'Customs: moon festivals', entries: [
      'Lanterns are floated down the river under {moon}, one for every child born this year.',
      'Masks are worn from moonrise to midnight, and nothing said behind one is held against anyone.',
      'Couples who walk the moon-road to the hill shrine and back are as good as betrothed.'] },
    { id: 'customs-rains', name: 'Customs: the rains', entries: [
      'Everyone stands out in the first rain, whatever they are wearing.',
      'The cisterns are opened and blessed, and the first bucket is poured back on the ground.'] },
    { id: 'customs-dry', name: 'Customs: the dry season', entries: [
      'The cisterns are sealed and blessed, and water is shared out by the well-judge.',
      'Songs are sung for rain that everyone knows won\u2019t come for months.'] },
    { id: 'customs-day', name: 'Customs: days between months', entries: [
      'Nobody works; the day belongs to no week and no debt falls due on it.',
      'Masks are worn from noon to midnight, and nothing said behind one is held against anyone.',
      'Everyone eats at long tables in the street, and the lord waits on them.',
      'The old tales are told in the square, each by a different family, in the same order every year.'] }
  ]
};
deepFreeze(STARTER_TABLES);

/* ── Generators made from tables ────────────────────────────────────────────────────────────────────
   An owner's generator is a recipe that points at a table set and the table to roll, plus how often.
   Its switches are the tags found in that table, so "no omens about birds" is a switch like any other. */

var TONE_MULTS = { bright: { bright: 2, grim: 0.35 }, balanced: { bright: 1, grim: 1 }, grim: { bright: 0.4, grim: 2.2 } };
/* "Per week" and "per month" mean this calendar's own week and an average ordinary month of it. */
function periodDays(cal, per) {
  if (per === 'day') return 1;
  if (per === 'week') return cal.weekLength || 7;
  var regular = cal.months.filter(function (M) { return !M.intercalary; }).length || cal.months.length || 1;
  return per === 'month' ? cal.refYearLength / regular : cal.refYearLength;
}

function tableSetOf(ref) {
  if (ref == null || ref === 'starter') return STARTER_TABLES;
  if (typeof ref === 'string') throw GenError('There’s no built-in table set called “' + ref + '”; the built-in one is “starter”.');
  return ref;
}
function tagsOf(set, tableId, seen) {
  seen = seen || {};
  if (seen[tableId]) return [];
  seen[tableId] = 1;
  var t = set.tables.filter(function (x) { return x.id === tableId; })[0], tags = [];
  if (!t) return tags;
  t.entries.map(entryObj).forEach(function (e) { if (!isObj(e)) return; if (e.tag) tags.push(e.tag); if (e.roll) tags = tags.concat(tagsOf(set, e.roll, seen)); });
  return uniq(tags);
}
/* Places a table's rolls over the scope: each day has its own keyed chance, so the days chosen for a week
   match the same week of a year. Returns events in Chronicle's shape. */
function placeRolls(o) {
  var cal = o.cal, out = [], state = { used: {}, depth: 0 };
  o.scope.days.forEach(function (t) {
    if (o.onlyDay && !o.onlyDay(t)) return;
    var r = makeRng(o.seed, o.stream, t);
    var boost = o.boost ? o.boost(t) : 1;
    if (!r.chance(Math.min(0.95, o.perDay * boost))) return;
    var ctx = dayContext(cal, t, o.ctx(t));
    var tables = o.pickTables ? o.pickTables(t, r) : [o.start];
    for (var i = 0; i < tables.length; i++) {
      var roll = rollOn(o.set, tables[i], ctx, r, state, o.mults(tables[i]));
      if (!roll || !(roll.name || roll.text)) continue;
      if (o.secrets === false && roll.visibility === 'dm_only') continue;
      out.push(rollToEvent(cal, t, roll, o, tables[i]));
      break;
    }
  });
  return out;
}
function rollToEvent(cal, t, roll, o, tableId) {
  var d = cal.fromAbs(t);
  var ev = {
    name: cap(roll.name || roll.text), description: roll.brief || '', year: d.year, month: d.month, day: d.day,
    is_recurring: false, visibility: o.visibility === 'dm_only' ? 'dm_only' : (roll.visibility || 'everyone'),
    category: roll.kind && (o.catIds || EVENT_KINDS).indexOf(roll.kind) >= 0 ? roll.kind : (o.defaultKind || 'quest'), all_day: !roll.time
  };
  if (roll.time) { ev.start_hour = Math.round(TIMES[roll.time] * cal.hoursPerDay / 24) % cal.hoursPerDay; ev.start_minute = 0; }
  if (roll.days > 1) { var e = cal.fromAbs(t + roll.days - 1); ev.end_year = e.year; ev.end_month = e.month; ev.end_day = e.day; }
  ev.announce = announceOf(ev, roll.announce);
  ev.gen = { key: makeKey('ev', o.seed, o.stream, t), generator: o.generator, kind: o.kindOf ? o.kindOf(tableId) : tableId, table: tableId, tag: roll.tag || null, tone: roll.tone || null, recipe: o.recipeId, moon: roll.moon || null, locked: false };
  // An entry drawn in the sky carries what the sky renderer needs, as the built-in sky events do.
  if (roll.sky) attachCustomSky(cal, t, ev, roll.sky, makeRng(o.seed, o.stream, 'sky', t), roll.time ? ev.start_hour : null);
  return ev;
}

function tableRecipeExtra(r, input, errors, warnings, ctx) {
  var set;
  r.tables = input.tables == null ? 'starter' : input.tables;
  try { set = tableSetOf(r.tables); } catch (e) { errors.push({ path: 'tables', message: e.message }); return; }
  if (set !== STARTER_TABLES) {
    var v = validateTables(set, ctx);
    v.errors.forEach(function (e) { errors.push({ path: 'tables.' + e.path, message: e.message }); });
  }
  var withOutput = (set.tables || []).filter(function (t) { return (t.entries || []).some(function (e) { return isObj(e) && (e.name || e.roll); }); });
  r.start = input.start || (withOutput[0] && withOutput[0].id) || null;
  if (!r.start || !(set.tables || []).some(function (t) { return t.id === r.start; })) { errors.push({ path: 'start', message: 'Say which table to roll: “' + input.start + '” isn’t in this set.' }); return; }
  var tags = tagsOf(set, r.start), mk = input.makes || {};
  r.makes = {};
  tags.forEach(function (tag) {
    var v2 = mk[tag] === undefined ? 'normal' : mk[tag];
    if (FREQ_LEVELS.indexOf(v2) < 0) { errors.push({ path: 'makes.' + tag, message: cap(tag) + ' can be off, rare, normal or often — ' + friendlyValue(v2) + ' isn’t one of those.' }); v2 = 'normal'; }
    r.makes[tag] = v2;
  });
  ownKeys(mk).forEach(function (k) { if (tags.indexOf(k) < 0) warnings.push({ path: 'makes.' + k, message: '“' + k + '” isn’t a tag in the “' + r.start + '” table, so it will be ignored.' + (tags.length ? ' Its tags are ' + joinList(tags) + '.' : '') }); });
}

defineGenerator({
  id: 'table', label: 'From your tables', noun: 'table',
  blurb: 'Rolls on a table you choose (the starter omens, rumours, market happenings, sky sights and road encounters, or your own) and puts the results on the calendar.',
  makes: {}, extraRecipe: tableRecipeExtra,
  scope: { range: true, moons: true, categories: true },
  details: {
    rate: { label: 'How many', type: 'number', default: 2, min: 0, max: 60, step: 0.5 },
    per: { label: 'Per', type: 'select', default: 'month', options: [{ value: 'day', label: 'day' }, { value: 'week', label: 'week' }, { value: 'month', label: 'month' }, { value: 'year', label: 'year' }] },
    tone: { label: 'Tone', type: 'select', default: 'balanced', options: [{ value: 'bright', label: 'Bright' }, { value: 'balanced', label: 'Balanced' }, { value: 'grim', label: 'Grim' }] },
    weekday: { label: 'Only on', type: 'weekday', default: 'auto', more: true, help: '“auto” for any day, or a weekday’s name.' },
    moonPull: { label: 'Pull towards full and new moons', type: 'range', default: 0.5, min: 0, max: 1, step: 0.1, more: true },
    secrets: { label: 'Include secrets only you see', type: 'bool', default: true, more: true },
    visibility: { label: 'Who sees them', type: 'select', default: 'table', more: true, options: [{ value: 'table', label: 'As each entry says' }, { value: 'dm_only', label: 'Only you' }] },
    theme: { label: 'Culture for names', type: 'select', default: 'pastoral', more: true, options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) },
    place: { label: 'Home town', type: 'text', default: '', max: 60, more: true, help: 'Used for {place}; left empty, the generator names one.' }
  },
  builtIns: [
    { id: 'table.omens', name: 'Omens', description: 'Signs and portents, most of them near a full or new moon.', start: 'omens', details: { rate: 2, per: 'month', moonPull: 0.8 } },
    { id: 'table.rumours', name: 'Rumours', description: 'Talk in the taverns: hooks, most of them half true.', start: 'rumours', details: { rate: 2, per: 'month', moonPull: 0 } },
    { id: 'table.market', name: 'Market happenings', description: 'What happens on market day. Set it to your market’s weekday.', start: 'market', details: { rate: 0.6, per: 'week', moonPull: 0 } },
    { id: 'table.sky-sights', name: 'Sky sights', description: 'Halos, sun dogs, silent lightning, and stranger things.', start: 'sky-sights', details: { rate: 2, per: 'month', moonPull: 0.3 } },
    { id: 'table.road', name: 'On the road', description: 'Strange encounters for a journey: scope it to the days of travel.', start: 'road', details: { rate: 3, per: 'week', moonPull: 0 } }
  ].map(function (b) { var o = clone(b); o.tables = 'starter'; return o; }),
  defaultRecipe: 'table.omens',
  run: runTableGen
});

/* Builds what a roll can say about its day: moons in scope, places and people in the theme, weather. */
function rollContextFactory(cal, seed, td, placeName, scopeMoons, weatherMap, climate) {
  var used = {}, neighbours = [];
  var pr = makeRng(seed, 'places');
  var home = placeName || namePlace(td, pr, used);
  for (var i = 0; i < 6; i++) neighbours.push(namePlace(td, pr, used));
  var people = [];
  var qr = makeRng(seed, 'people');
  for (var j = 0; j < 12; j++) people.push(namePerson(td, qr, used, { title: j % 3 === 0 ? undefined : false }));
  return {
    home: home, neighbours: neighbours,
    ctx: function (t) {
      return { scopeMoons: scopeMoons, climate: climate || null, weather: weatherMap ? weatherMap[t] : null, home: home,
        place: function (rng) { return rng.pick(neighbours); },
        person: function (rng) { return rng.pick(people); } };
    }
  };
}
function weekdayFilter(cal, spec) {
  if (spec == null || spec === 'auto' || spec === '') return null;
  var idx = typeof spec === 'number' ? spec - 1 : cal.weekdays.map(function (w) { return w.name.toLowerCase(); }).indexOf(String(spec).toLowerCase());
  if (idx < 0 || idx >= cal.weekLength) throw GenError('This calendar has no weekday called “' + spec + '”.' + (cal.weekLength ? ' Its weekdays are ' + joinList(cal.weekdays.map(function (w) { return w.name; })) + '.' : ''));
  return function (t) { return cal.weekdayOfAbs(t) === idx; };
}
/* Moves rolls towards full and new moons without adding any: the pull is divided by its average over the
   day's year, so "two a month" stays two a month however many moons the sky has. */
function moonPullFor(cal, moons, strength) {
  if (!strength || !moons.length) return null;
  var hi = 1 + 3 * strength, lo = 1 - 0.5 * strength, means = {};
  function near(t) { return moons.some(function (m) { return daysToPhase(m, t, 0.5) <= 1.5 || daysToPhase(m, t, 0) <= 1.5; }); }
  function mean(y) {
    if (means[y] == null) { var sp = cal.yearSpan(y), s = 0; for (var a = sp[0]; a <= sp[1]; a++) s += near(a) ? hi : lo; means[y] = s / (sp[1] - sp[0] + 1); }
    return means[y];
  }
  return function (t) { return (near(t) ? hi : lo) / mean(cal.fromAbs(t).year); };
}

function runTableGen(opts) {
  var cats = categoriesOf(opts);
  var recipe = resolveRecipe('table', opts.recipe, opts), cal = makeCal(opts.calendar), seed = seedOf(opts), dt = recipe.details;
  var scopeIn = Object.assign({}, opts.scope || {});
  if (!scopeIn.moons && recipe.scope.moons) scopeIn.moons = recipe.scope.moons;
  if (!scopeIn.categories && recipe.scope.categories) scopeIn.categories = recipe.scope.categories;
  var scope = resolveScope(cal, scopeIn), set = tableSetOf(recipe.tables), td = themeData(dt.theme);
  var weatherMap = null;
  if (opts.context && opts.context.weather) { weatherMap = {}; opts.context.weather.forEach(function (d) { if (cal.valid(d.year, d.month, d.day)) weatherMap[cal.abs(d.year, d.month, d.day)] = d; }); }
  var rc = rollContextFactory(cal, seed, td, dt.place, scope.moons, weatherMap, opts.context && opts.context.climate);
  var onlyDay = weekdayFilter(cal, dt.weekday);
  var daysPer = periodDays(cal, dt.per);
  var share = onlyDay ? (cal.weekLength || 7) : 1;
  var tags = {}; ownKeys(recipe.makes).forEach(function (k) { tags[k] = freqMult(recipe.makes[k]); });
  var pull = moonPullFor(cal, scope.moons, dt.moonPull);
  var events = placeRolls({
    cal: cal, scope: scope, seed: seed, stream: 'table|' + (recipe.id || recipe.name) + '|' + recipe.start, set: set, start: recipe.start,
    perDay: dt.rate / daysPer * share, onlyDay: onlyDay, boost: pull, ctx: rc.ctx, secrets: dt.secrets, visibility: dt.visibility,
    mults: function () { return { tags: tags, tone: TONE_MULTS[dt.tone] }; }, generator: 'table', recipeId: recipe.id || recipe.name, defaultKind: 'quest', catIds: categoryIds(cats)
  });
  if (scope.categories) events = events.filter(function (e) { return scope.categories.indexOf(e.category) >= 0; });
  var byTag = countBy(events, function (e) { return e.gen.tag || 'other'; });
  var tname = (set.tables.filter(function (t) { return t.id === recipe.start; })[0] || {}).name || recipe.start;
  var summary = cap(count(events.length, 'roll')) + ' on “' + tname + '” for ' + scope.label + ', about ' + dt.rate + ' a ' + dt.per + (onlyDay ? ' and only on ' + dt.weekday + 's' : '') + '.' +
    (events.length ? ' ' + cap(joinList(ownKeys(byTag).map(function (k) { return count(byTag[k], k === 'other' ? 'other' : k + ' entry', k === 'other' ? 'others' : k + ' entries'); }))) + '.' : '') +
    (pull && events.some(function (e) { return e.gen.moon; }) ? ' ' + cap(count(events.filter(function (e) { return e.gen.moon; }).length, 'of them is', 'of them are')) + ' tied to a moon.' : '') +
    (ownKeys(recipe.makes).some(function (k) { return recipe.makes[k] === 'off'; }) ? ' Switched off: ' + joinList(ownKeys(recipe.makes).filter(function (k) { return recipe.makes[k] === 'off'; })) + '.' : '') +
    (events.some(function (e) { return e.visibility === 'dm_only'; }) ? ' ' + cap(count(events.filter(function (e) { return e.visibility === 'dm_only'; }).length, 'is a secret', 'are secrets')) + ' only you can see.' : '');
  return { generator: 'table', seed: seed, recipe: recipe, scope: { label: scope.label, days: scope.days.length, kind: scope.kind }, events: events, summary: summary.replace(/ ,/g, ','), stats: byTag, warnings: scope.moonWarnings };
}

/* One roll, for a "roll it" button or a preview. */
function rollTableOnce(setRef, tableId, opts) {
  opts = opts || {};
  var set = tableSetOf(setRef), v = set === STARTER_TABLES ? { ok: true } : validateTables(set, opts);
  if (!v.ok) throw GenError('These tables have problems: ' + v.errors.map(function (e) { return e.message; }).join(' '));
  var cal = makeCal(opts.calendar), date = dateOf(opts.date || { year: cal.currentYear, month: cal.currentMonth, day: cal.currentDay });
  var t = cal.absOf(date), td = themeData(opts.theme || 'pastoral');
  var rc = rollContextFactory(cal, seedOf(opts), td, opts.place, cal.moons.filter(function (m) { return m.cycle > 0; }), null, opts.climate);
  var roll = rollOn(set, tableId, dayContext(cal, t, rc.ctx(t)), makeRng(seedOf(opts), 'roll-once', tableId, t, opts.n || 0), { used: {}, depth: 0 }, null);
  return roll ? { name: roll.name ? cap(roll.name) : null, brief: roll.brief || null, text: roll.text || null, kind: roll.kind, visibility: roll.visibility, tag: roll.tag, moon: roll.moon } : null;
}

/* ── Sky events ─────────────────────────────────────────────────────────────────────────────────────
   Everything here is a calendar event of kind "sky" plus a `sky` object a renderer can draw from: type,
   peak (date and hour), intensity 0..1, where in the sky, how long. Kinds draw from their own random
   streams, so switching one off never moves another. Eclipses are not random at all: they follow each
   moon's own cycle through a simple nodal model (eclipse seasons twice a "node year"), so the same moons
   always eclipse on the same days, and the recipe only widens or narrows the eclipse windows. */

var SKY_KINDS = {
  'meteor-shower': { label: 'Meteor showers', help: 'Yearly showers that return on the same dates, and now and then an unexpected one.' },
  comet: { label: 'Comets', default: 'rare', help: 'Visitors that brighten and fade over weeks; one great comet returns every few generations.' },
  aurora: { label: 'Auroras', help: 'Common in cold climates, rare in temperate ones, never in the tropics unless the sky is magic.' },
  'eclipse-solar': { label: 'Eclipses of the sun' },
  'eclipse-lunar': { label: 'Partial eclipses of a moon' },
  'blood-moon': { label: 'Blood moons', help: 'Total eclipses of a moon, when it turns red.' },
  conjunction: { label: 'Moons full together' },
  'harvest-moon': { label: 'Harvest and hunter’s moons' },
  'blue-moon': { label: 'Blue moons', help: 'A second full moon in one month.' },
  'dark-night': { label: 'Moonless nights', default: 'rare', help: 'Nights when every moon is new at once.' },
  'falling-star': { label: 'Falling stars', help: 'A single bright fireball; now and then one comes down.' }
};
var SKY_MAKES = {};
ownKeys(SKY_KINDS).forEach(function (k) { SKY_MAKES[k] = { label: SKY_KINDS[k].label, help: SKY_KINDS[k].help, type: 'freq', default: SKY_KINDS[k].default || 'normal' }; });
var SHOWER_COUNT = { off: 0, rare: 1, normal: 2, often: 4 };
var SHOWER_WORDS = {
  pastoral: 'Lantern Harvest Hearth Sheaf Chaff Ember Candle', nautical: 'Salt Pearl Spray Beacon Lantern Brine', dwarven: 'Spark Anvil Ember Cinder Forge Ingot',
  elven: 'Silver Star Dew Song Mirror Willow', desert: 'Lamp Brass Glass Ember Scorpion Date', imperial: 'Legion Laurel Eagle Crown Torch',
  fey: 'Wish Glimmer Moth Dew Thistle Firefly', grim: 'Ash Cinder Knell Crow Pyre Tallow'
};
var SKY_COLOURS = { warm: '#fff1c9', blue: '#d6e8ff', gold: '#ffd58a', green: '#bff2cf', red: '#ff9d8a', violet: '#d8b4ff' };

defineGenerator({
  id: 'sky', label: 'Sky events',
  blurb: 'Meteor showers, comets, auroras, eclipses, blood moons, moons full together and falling stars, as calendar events a sky renderer can draw.',
  makes: SKY_MAKES,
  scope: { range: true, moons: true },
  details: {
    latitude: { label: 'How far north', type: 'select', default: 'auto', options: [{ value: 'auto', label: 'From the climate' }, { value: 'polar', label: 'Polar' }, { value: 'high', label: 'Cold north' }, { value: 'mid', label: 'Temperate' }, { value: 'low', label: 'Warm south' }], help: 'Auroras depend on it.' },
    magic: { label: 'How magic the sky is', type: 'select', default: 'none', options: [{ value: 'none', label: 'Natural' }, { value: 'low', label: 'A little uncanny' }, { value: 'high', label: 'Openly magic' }] },
    visibility: { label: 'Who sees new sky events', type: 'select', default: 'everyone', options: [{ value: 'everyone', label: 'Everyone' }, { value: 'dm_only', label: 'Only you, until you reveal them' }], more: true },
    auroraMoon: { label: 'Moon that stirs the auroras', type: 'text', default: '', max: 60, more: true, help: 'A moon’s name: auroras come more often when it is full.' },
    theme: { label: 'Culture for names', type: 'select', default: 'pastoral', more: true, options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) }
  },
  builtIns: [
    { id: 'sky.classic', name: 'A natural sky', description: 'Showers, the odd comet, auroras where it is cold enough, eclipses from the moons’ own cycles.' },
    { id: 'sky.high-magic', name: 'A magic sky', description: 'Blood moons and auroras stirred by a moon.', makes: { 'blood-moon': 'often', aurora: 'often', 'dark-night': 'normal', comet: 'normal' }, details: { magic: 'high' } },
    { id: 'sky.omens', name: 'A sky full of omens', description: 'Eclipses, blood moons and moonless nights come often, and falling stars are common.', makes: { 'eclipse-solar': 'often', 'eclipse-lunar': 'often', 'blood-moon': 'often', 'dark-night': 'often', 'falling-star': 'often' }, details: { magic: 'low' } },
    { id: 'sky.quiet', name: 'A quiet sky', description: 'Only the big moments: one shower a year, rare eclipses.', makes: { 'meteor-shower': 'rare', aurora: 'rare', 'eclipse-solar': 'rare', 'eclipse-lunar': 'rare', 'blood-moon': 'rare', conjunction: 'rare', 'harvest-moon': 'off', 'blue-moon': 'off', 'dark-night': 'off', 'falling-star': 'rare' } }
  ],
  defaultRecipe: 'sky.classic',
  run: runSky
});

function hourOf(cal, frac) { return clamp(Math.floor(frac * cal.hoursPerDay), 0, cal.hoursPerDay - 1); }
function skyEvent(cal, abs, f) {
  var d = cal.fromAbs(abs);
  var ev = {
    name: f.name, description: f.brief, year: d.year, month: d.month, day: d.day,
    is_recurring: !!f.yearly, visibility: f.visibility || 'everyone', category: 'sky', all_day: f.hour == null
  };
  if (f.yearly) ev.recurrence_type = 'yearly';
  if (f.hour != null) { ev.start_hour = f.hour; ev.start_minute = 0; }
  if (f.endAbs && f.endAbs > abs) { var e = cal.fromAbs(f.endAbs); ev.end_year = e.year; ev.end_month = e.month; ev.end_day = e.day; }
  ev.announce = announceOf(ev, null); // built-in sky kinds never set this themselves
  ev.gen = { key: f.key, generator: 'sky', kind: f.kind, recipe: f.recipeId, moon: f.moon || null, locked: false };
  ev.sky = f.sky;
  ev.sky.peak = { year: d.year, month: d.month, day: d.day, hour: f.hour != null ? f.hour : null };
  if (f.peakAbs != null && f.peakAbs !== abs) { var pk = cal.fromAbs(f.peakAbs); ev.sky.peak = { year: pk.year, month: pk.month, day: pk.day, hour: f.hour != null ? f.hour : null }; }
  return ev;
}
function regularDay(cal, abs) { // the next day that isn't a between-months festival day
  for (var i = 0; i < 20; i++) { var d = cal.fromAbs(abs + i); if (!cal.isIntercalary(d.month)) return abs + i; }
  return abs;
}
function skyDir(r, altitudes) { var c = r.pick(['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW']); return { compass: c, degrees: DIR_DEG[c], altitude: r.pick(altitudes || ['low', 'mid', 'high']) }; }
function dirPhrase(dir) { return (dir.altitude === 'high' ? 'high in ' : dir.altitude === 'low' ? 'low in ' : '') + 'the ' + COMPASS_WORDS[dir.compass]; }

/* A fireball in one of several voices; the place and direction keep each one its own. */
var SKY_ADJ = { N: 'northern', NE: 'north-eastern', E: 'eastern', SE: 'south-eastern', S: 'southern', SW: 'south-western', W: 'western', NW: 'north-western' };
function fireballWords(r, place, dir, hue, landed) {
  var colour = { green: 'green', warm: 'white-gold', blue: 'blue-white' }[hue] || 'white', sky = 'the ' + (SKY_ADJ[dir.compass] || 'western') + ' sky', low = dir.altitude === 'low' ? 'low ' : '';
  if (landed) {
    return {
      name: r.pick(['A star comes down near ' + place, 'Something falls near ' + place, 'The ' + place + ' stone', 'A fallen star at ' + place]),
      brief: r.pick([
        'A fireball splits the sky over ' + place + ' and comes down beyond the ridge. Shepherds say the ground was warm at dawn.',
        'A ' + colour + ' fireball comes down in a barley field near ' + place + ', leaving a scorched hollow and a black stone too hot to touch.',
        'Something falls into the mere by ' + place + ' with a hiss the whole village hears; the water steams until noon.',
        'A falling star smashes through a barn roof at ' + place + '. The stone is heavy as iron, and the priest wants it.',
        'Something comes down in the woods near ' + place + ' with a sound like a door slamming. Nobody has gone to look yet.'])
    };
  }
  var open = r.pick([
    'A ' + colour + ' fireball streaks ' + low + 'across ' + sky + ' over ' + place,
    'A meteor bright as the moon slides ' + (low ? 'low ' : '') + 'down ' + sky + ' over ' + place,
    'A ball of ' + colour + ' fire tears ' + low + 'across ' + sky + ', seen from ' + place,
    'For a heartbeat ' + place + ' is lit like day as a fireball passes ' + (low ? 'low ' : '') + 'in ' + sky]);
  var close = r.pick([
    ', bright enough to throw shadows, and breaks apart in silence.',
    ' and leaves a smoky trail that hangs for a quarter of an hour.',
    '; a long moment later comes a boom like distant thunder.',
    ' and bursts into three pieces before it fades.',
    '; every dog for miles starts barking.',
    ' and is gone before anyone can point.']);
  return { name: r.pick(['A fireball over ' + place, 'A ' + colour + ' fireball', 'The ' + place + ' fireball', 'A bright meteor over ' + place]), brief: open + close };
}
/* Auroras by how far north and how strong: a red glow on the horizon in the south, curtains overhead up
   north. Frequent up north, so each is built from two parts to keep a year of them from repeating. */
function auroraWords(r, kind, nights, moon) {
  var run = nights > 1 ? (kind === 'faint' ? ' for ' + numWord(nights) + ' nights' : ', on ' + numWord(nights) + ' nights running') : '';
  if (kind === 'arcane') return {
    name: r.pick(['The violet veil', 'Spell-light in the sky', 'Violet curtains']),
    brief: r.pick(['Violet and green light ripples overhead', 'Sheets of violet light hang from the zenith', 'The sky shimmers violet from rim to rim']) + (moon ? ' while ' + moon.name + ' is full' : '') + run + '; ' + r.pick(['spells feel easier to cast under it.', 'hedge-witches sit up all night to work.', 'iron tools hum faintly until dawn.'])
  };
  if (kind === 'faint') return {
    name: r.pick(['Red lights low in the north', 'A red glow in the north', 'The northern glow', 'A blush on the northern sky']),
    brief: r.pick(['A red glow low on the northern horizon', 'A dull red light along the north', 'A faint crimson arch over the northern hills']) + run + '; ' + r.pick(['half the town thinks a barn is burning.', 'the old people say it means a hard winter.', 'the watch rings the fire bell before anyone understands.', 'nobody under sixty has seen it before.'])
  };
  if (kind === 'strong') return {
    name: r.pick(['The sky on fire in the north', 'The great lights', 'Curtains of fire overhead', 'The crimson lights']),
    brief: r.pick(['Curtains of green and red fill half the sky, bright enough to read by', 'Red and green light pours down from overhead, rippling like cloth', 'The whole northern sky burns green and crimson', 'Light crowns the sky overhead and spills down on every side', 'Sheets of crimson and green race from one horizon to the other']) + run + '; ' +
      r.pick(['nobody sleeps.', 'the temple bell is rung, just in case.', 'it will be talked about for years.', 'the snow on the hills glows red under it.'])
  };
  return {
    name: r.pick(['Green curtains in the north', 'Northern lights', 'Green fire in the north', 'The lights in the north', 'The dancers', 'A green arch in the north']),
    brief: r.pick(['Green light moves along the northern sky from dusk to midnight', 'Pale green curtains fold and unfold over the northern hills', 'A green arch stands over the north and slowly breaks into rays', 'Faint green bands drift across the north all night', 'A green glow rises behind the northern ridges after dark', 'Rays of green light flicker up from the northern horizon']) + run +
      r.pick(['.', '; the children are let up late to watch.', '; the dogs will not settle.', '; it is gone by the small hours.', '; fishermen say the herring will come early.', '; the old folk call it the dancers.'])
  };
}

function runSky(opts) {
  var recipe = resolveRecipe('sky', opts.recipe), cal = makeCal(opts.calendar), seed = seedOf(opts), mk = recipe.makes, dt = recipe.details;
  var scopeIn = Object.assign({}, opts.scope || {});
  if (!scopeIn.moons && recipe.scope.moons) scopeIn.moons = recipe.scope.moons;
  var scope = resolveScope(cal, scopeIn), moons = scope.moons.filter(function (m) { return m.cycle > 0; });
  var td = themeData(dt.theme), used = {}, events = [], notes = [], rid = recipe.id || 'sky';
  var from = scope.from, to = scope.to;
  var inScope = function (a, b) { for (var n = a; n <= (b || a); n++) if (scope.set[n]) return true; return false; };
  var vis = function (ms) { return (dt.visibility === 'dm_only' || (ms || []).some(function (m) { return m && m.hidden; })) ? 'dm_only' : 'everyone'; };
  var brightest = cal.moons.filter(function (m) { return !m.hidden && m.cycle > 0; }).sort(function (a, b) { return b.size - a.size; })[0];
  var latitude = dt.latitude !== 'auto' ? dt.latitude : (opts.context && opts.context.climate && CLIMATES[opts.context.climate] ? CLIMATES[opts.context.climate].latitude : (opts.climate && CLIMATES[opts.climate] ? CLIMATES[opts.climate].latitude : 'mid'));

  // Meteor showers: fixed (month, day) per world, so they return on the same date every year.
  var nShowers = SHOWER_COUNT[mk['meteor-shower']] || 0, words = toks(SHOWER_WORDS[dt.theme] || SHOWER_WORDS.pastoral).map(function (t) { return t.v; });
  var refYear = cal.currentYear, refLen = cal.yearLength(refYear);
  for (var i = 0; i < nShowers; i++) {
    var r = makeRng(seed, 'sky-shower', i);
    var doy = Math.floor(((i + r.range(0.15, 0.85)) / nShowers) * refLen);
    var base = cal.fromAbs(regularDay(cal, cal.abs(refYear, 1, 1) + doy));
    var major = i === 0, zhr = major ? r.int(60, 120) : r.int(10, 40), dir = skyDir(r, ['low', 'mid', 'high']);
    var name = null, monthName = cal.months[base.month - 1].name;
    for (var a2 = 0; a2 < 20 && !name; a2++) {
      var w = r.pick(words), fall = joinCompound(w, 'fall'), cand = r.pick([fall ? 'The ' + fall : 'The ' + w + ' Rain', 'The ' + w + ' Sparks', 'The ' + monthName + ' Sparks', 'The ' + w + ' Rain']);
      if (isFree(used, cand)) name = take(used, cand);
    }
    var colour = r.pick(['warm', 'blue', 'gold', 'warm']);
    for (var y = scope.years[0]; y <= scope.years[scope.years.length - 1]; y++) {
      if (!cal.valid(y, base.month, base.day)) continue;
      var pa = cal.abs(y, base.month, base.day);
      if (!inScope(pa)) continue;
      var lit = brightest ? litFraction(moonPhaseAt(brightest, pa)) : 0;
      events.push(skyEvent(cal, pa, {
        key: makeKey('sky', seed, 'shower', i), kind: 'meteor-shower', recipeId: rid, yearly: true, hour: hourOf(cal, 0.1), visibility: vis(),
        name: name, brief: (major ? 'The year’s best meteor shower: up to ' + zhr + ' an hour after midnight' : 'A thin meteor shower, ' + zhr + ' or so an hour at best') + ', streaming from ' + dirPhrase(dir) + '.',
        sky: { type: 'meteor-shower', intensity: round2(zhr / 120), rate_per_hour: zhr, direction: dir, duration: { days: 5, before: 2, after: 2 }, color: SKY_COLOURS[colour], moonlit: lit > 0.6, moonlight: round2(lit) }
      }));
      break; // one yearly event covers every later year
    }
  }
  scope.years.forEach(function (y) {
    var r2 = makeRng(seed, 'sky-outburst', y);
    if (!r2.chance(0.12 * freqMult(mk['meteor-shower']))) return;
    var ys = cal.yearSpan(y), pa2 = regularDay(cal, r2.int(ys[0], ys[1] - 20)), storm = r2.chance(0.25);
    if (!inScope(pa2)) return;
    var dir2 = skyDir(r2);
    events.push(skyEvent(cal, pa2, {
      key: makeKey('sky', seed, 'outburst', y), kind: 'meteor-shower', recipeId: rid, hour: hourOf(cal, 0.05), visibility: vis(),
      name: storm ? 'The night the stars fell' : 'An unlooked-for meteor shower', brief: storm ? 'Hundreds of shooting stars an hour from ' + dirPhrase(dir2) + ', so many the sky seems to be falling. By morning it is over.' : 'Shooting stars out of ' + dirPhrase(dir2) + ' for one night only; no almanac predicted it.',
      sky: { type: storm ? 'meteor-storm' : 'meteor-shower', intensity: storm ? 1 : 0.4, rate_per_hour: storm ? r2.int(400, 1000) : r2.int(20, 50), direction: dir2, duration: { days: 1 }, color: SKY_COLOURS[storm ? 'gold' : 'warm'] }
    }));
  });

  // Comets: rare visitors over weeks, plus one great comet that returns every few generations.
  if (mk.comet !== 'off') {
    var wr = makeRng(seed, 'sky-great-comet'), P = wr.int(60, 130), phase = wr.int(0, P - 1), greatName = wr.pick(['{P}’s Star', 'The Pilgrim Star', 'The Long-Tailed Star', 'The Widow’s Lamp', 'The Red Wanderer']);
    greatName = greatName.replace('{P}', namePerson(td, wr, {}, { title: false }).split(' ')[0]);
    scope.years.forEach(function (y) {
      var r3 = makeRng(seed, 'sky-comet', y), great = mod(y - phase, P) === 0;
      if (!great && !r3.chance(0.09 * freqMult(mk.comet))) return;
      var ys = cal.yearSpan(y), L = great ? r3.int(50, 80) : r3.int(20, 60), peak = r3.int(ys[0] + 10, ys[1] - 10), start = peak - Math.round(L * 0.55), end = start + L - 1;
      if (!inScope(start, end)) return;
      var inten = great ? r3.range(0.85, 1) : r3.range(0.3, 0.8), dir3 = { compass: r3.pick(['W', 'E', 'NW', 'NE']), degrees: 0, altitude: 'low' };
      dir3.degrees = DIR_DEG[dir3.compass];
      var cname = great ? greatName : null;
      for (var a3 = 0; a3 < 20 && !cname; a3++) { var c3 = r3.pick(['The ' + r3.pick(['Pale', 'Long', 'Silver', 'Red', 'Crooked', 'Weeping']) + ' Star', namePerson(td, r3, {}, { title: false }).split(' ')[0] + '’s Comet', 'The Broom Star', 'The ' + r3.pick(['Harbinger', 'Stranger', 'Wanderer', 'Visitor'])]); if (isFree(used, c3)) cname = take(used, c3); }
      var evening = dir3.compass.indexOf('W') >= 0;
      events.push(skyEvent(cal, start, {
        key: makeKey('sky', seed, 'comet', y), kind: 'comet', recipeId: rid, endAbs: end, peakAbs: peak, hour: hourOf(cal, evening ? 0.83 : 0.2), visibility: vis(),
        name: great ? cname + ' returns' : cname,
        brief: (great ? 'The great comet is back, last seen in ' + (y - P) + '. ' : '') + 'A comet ' + (evening ? 'low in the west after sunset' : 'low in the east before dawn') + ', brightest around ' + cal.fmtAbs(peak, false) + (inten > 0.85 ? ', when its tail stretches halfway up the sky and it can be seen by day' : '') + '. It fades over ' + numWord(Math.round((end - peak) / 7)) + ' weeks.',
        sky: { type: 'comet', intensity: round2(inten), tail_degrees: Math.round(inten * 55), direction: dir3, duration: { days: L }, curve: { shape: 'rise-and-fade', peak_offset_days: peak - start, width_days: round1(L / 3.5) }, color: r3.pick([SKY_COLOURS.blue, SKY_COLOURS.warm, SKY_COLOURS.green]),
          phases: [{ label: 'first seen', date: cal.fromAbs(start) }, { label: 'brightest', date: cal.fromAbs(peak) }, { label: 'last seen', date: cal.fromAbs(end) }], periodic_years: great ? P : null }
      }));
    });
  }

  // Auroras: likelier the further north, in darker seasons, in the busy years of an 11-year cycle, and
  // on a 27-day rhythm (the sun's turning), so displays come back about a month apart.
  var base = { polar: 0.16, high: 0.06, mid: 0.008, low: 0 }[latitude] || 0;
  var arcane = dt.magic === 'high' ? 0.006 : 0;
  var am = dt.auroraMoon ? cal.findMoon(dt.auroraMoon) : null;
  if (dt.auroraMoon && !am) notes.push('There is no moon called “' + dt.auroraMoon + '”, so no moon stirs the auroras.');
  if (mk.aurora !== 'off' && (base > 0 || arcane > 0 || am)) {
    var wr2 = makeRng(seed, 'sky-aurora-world'), phi27 = wr2.range(0, 27), phi11 = wr2.range(0, 11);
    var run = null;
    var flush = function () {
      if (!run) return;
      var len = run.end - run.start + 1, strong = run.max > 0.75, arcaneRun = run.arcane;
      var aw = auroraWords(makeRng(seed, 'sky-aurora-words', run.start), arcaneRun ? 'arcane' : latitude === 'mid' || latitude === 'low' ? 'faint' : strong ? 'strong' : 'plain', len, am);
      var nm = aw.name, br = aw.brief;
      events.push(skyEvent(cal, run.start, {
        key: makeKey('sky', seed, 'aurora', run.start), kind: 'aurora', recipeId: rid, endAbs: run.end, peakAbs: run.peak, hour: hourOf(cal, 0.9), visibility: vis(am ? [am] : []), moon: am ? am.name : null,
        name: nm, brief: br,
        sky: { type: arcaneRun ? 'arcane-aurora' : 'aurora', intensity: round2(run.max), direction: { compass: 'N', degrees: 0, altitude: strong && latitude !== 'mid' ? 'high' : 'low' }, duration: { days: len }, colors: arcaneRun ? [SKY_COLOURS.violet, SKY_COLOURS.green] : strong || latitude === 'mid' ? [SKY_COLOURS.green, SKY_COLOURS.red] : [SKY_COLOURS.green], color: arcaneRun ? SKY_COLOURS.violet : SKY_COLOURS.green }
      }));
      run = null;
    };
    for (var t = from - 3; t <= to; t++) {
      var th = cal.thetaOfAbs(Math.max(1, t)), summerness = (1 - Math.cos(2 * Math.PI * th)) / 2, equinox = Math.pow(Math.sin(2 * Math.PI * th), 2);
      var dark = (latitude === 'polar' || latitude === 'high') ? 0.25 + 0.75 * (1 - summerness) + 0.3 * equinox : 0.6 + 0.4 * (1 - summerness) + 0.3 * equinox;
      var yr = cal.fromAbs(Math.max(1, t)).year, cyc = 0.55 + 0.45 * Math.sin(2 * Math.PI * (yr + phi11) / 11), rot = 1 + 0.9 * Math.cos(2 * Math.PI * (t - phi27) / 27);
      var moonF = am ? 1 + 2.5 * litFraction(moonPhaseAt(am, t)) : 1;
      var p = (base + arcane + (am && base === 0 ? 0.004 : 0)) * freqMult(mk.aurora) * dark * cyc * rot * moonF;
      var ra = makeRng(seed, 'sky-aurora', t);
      if (ra.chance(p)) {
        var inten2 = clamp(ra.range(0.25, 1) * (0.6 + 0.4 * rot / 1.9), 0.1, 1);
        if (run && run.end === t - 1) { run.end = t; if (inten2 > run.max) { run.max = inten2; run.peak = t; } }
        else { flush(); run = { start: t, end: t, peak: t, max: inten2, arcane: base === 0 || (arcane > 0 && ra.chance(arcane / (base + arcane))) }; }
      } else if (run && run.end < t - 1) flush();
    }
    flush();
    events = events.filter(function (e) { return e.gen.kind !== 'aurora' || inScope(cal.absOf(e), e.end_year ? cal.abs(e.end_year, e.end_month, e.end_day) : null); });
  }

  // Eclipses, from each moon's cycle.
  var yl = cal.refYearLength, skipped = { total: 0 };
  moons.forEach(function (m) {
    var nr = makeRng('eclipse-node', m.name, m.cycle, m.offset), E = yl * nr.range(0.92, 0.97), phi = nr.range(0, E / 2);
    var sizeF = Math.sqrt(clamp(m.size || 1, 0.5, 1.3)), cap2 = E / 4;
    var Ws = Math.min(cap2, 0.55 * m.cycle * sizeF * freqMult(mk['eclipse-solar']));
    var Wl = Math.min(cap2, 0.42 * m.cycle * sizeF * Math.max(freqMult(mk['eclipse-lunar']), freqMult(mk['blood-moon'])));
    function nearestSeason(t) { var k = Math.round((t - phi) / (E / 2)); return Math.abs(t - (phi + k * E / 2)); }
    // Only some eclipses are seen from here: a solar one's shadow is a narrow path, a lunar one is seen
    // from the night side. The odds are fixed per moon and day, not per seed, like the eclipses themselves.
    var seenHere = function (kind, abs, p) { return makeRng('eclipse-seen', kind, m.name, abs).chance(Math.min(1, p)); };
    // The sun has to be up for a solar eclipse and the full moon (so night) for a lunar one.
    var inDaylight = function (fr) { return fr >= 0.29 && fr <= 0.71; };
    var atNight = function (fr, widen) { return fr >= 0.75 - widen || fr < 0.25 + widen; };
    if (Ws > 0) phaseInstants(m, from, to, 0).forEach(function (x) {
      var dd = nearestSeason(x.t);
      if (dd >= Ws || !scope.set[x.abs]) return;
      var cent = dd / Ws, type = cent < 0.3 ? (m.size >= 0.95 ? 'total' : 'annular') : 'partial', hr = hourOf(cal, x.frac);
      if (!inDaylight(x.frac) || !seenHere('solar', x.abs, (type === 'partial' ? 0.8 : 0.45) * Math.sqrt(freqMult(mk['eclipse-solar'])))) return;
      var nm = type === 'total' ? 'The sun goes dark' : type === 'annular' ? 'A ring of fire' : m.name + ' bites the sun';
      var br = type === 'total' ? m.name + ' covers the sun for ' + numWord(clamp(Math.round(7 * (1 - cent / 0.3)), 2, 7)) + ' minutes; birds fall quiet and stars come out ' + (x.frac < 0.46 ? 'in the middle of the morning' : x.frac <= 0.54 ? 'at midday' : 'in the middle of the afternoon') + '.' :
        type === 'annular' ? m.name + ' slides across the sun but is too small to cover it: a ring of fire hangs in the sky for a few minutes.' :
        m.name + ' covers ' + (cent < 0.6 ? 'most' : 'a bite') + ' of the sun; the light turns thin and strange for an hour.';
      events.push(skyEvent(cal, x.abs, { key: makeKey('sky', 'solar', m.name, x.abs), kind: 'eclipse-solar', recipeId: rid, hour: hr, visibility: vis([m]), moon: m.name, name: nm, brief: br,
        sky: { type: 'eclipse-solar', subtype: type, moon: m.name, intensity: round2(1 - cent), magnitude: round2(1 - cent * 0.7), direction: { compass: 'S', degrees: 180, altitude: 'high' }, duration: { minutes: type === 'partial' ? 120 : clamp(Math.round(7 * (1 - cent / 0.3)), 2, 7) } } }));
    });
    if (Wl > 0) phaseInstants(m, from, to, 0.5).forEach(function (x) {
      var dd = nearestSeason(x.t);
      if (dd >= Wl || !scope.set[x.abs]) return;
      var cent = dd / Wl, type = cent < 0.35 ? 'total' : cent < 0.75 ? 'partial' : 'penumbral', hr = hourOf(cal, x.frac);
      if (!atNight(x.frac, Math.max(freqMult(mk['eclipse-lunar']), freqMult(mk['blood-moon'])) > 1 ? 0.04 : 0)) return;
      if (type === 'total' && mk['blood-moon'] === 'off') { skipped.total++; return; }
      if (type !== 'total' && mk['eclipse-lunar'] === 'off') return;
      if (type === 'penumbral' && mk['eclipse-lunar'] !== 'often') return;
      var bloodName = { grim: 'The Bleeding Moon', pastoral: m.name + ' turns to blood', fey: 'The red revel' }[dt.theme] || 'Blood moon: ' + m.name + ' in eclipse';
      var nm2 = type === 'total' ? bloodName : type === 'partial' ? 'A shadow across ' + m.name : m.name + ' dims';
      var br2 = type === 'total' ? m.name + ' turns copper-red for an hour ' + (x.frac >= 0.9 || x.frac < 0.1 ? 'around midnight' : x.frac >= 0.5 ? 'in the evening' : 'before dawn') + '; anyone awake can see it.' :
        type === 'partial' ? 'A dark bite creeps across ' + m.name + ' and withdraws over three hours.' : m.name + ' dulls, as if seen through smoke; few notice.';
      events.push(skyEvent(cal, x.abs, { key: makeKey('sky', 'lunar', m.name, x.abs), kind: type === 'total' ? 'blood-moon' : 'eclipse-lunar', recipeId: rid, hour: hr, visibility: vis([m]), moon: m.name, name: nm2, brief: br2,
        sky: { type: type === 'total' ? 'blood-moon' : 'eclipse-lunar', subtype: type, moon: m.name, intensity: round2(1 - cent), color: type === 'total' ? '#b8472f' : null, duration: { minutes: type === 'total' ? 60 : 180 }, direction: { compass: 'S', degrees: 180, altitude: 'mid' } } }));
    });
  });
  if (skipped.total) notes.push(cap(count(skipped.total, 'total eclipse')) + ' of a moon fell in this range; blood moons are off, so ' + (skipped.total === 1 ? 'it was' : 'they were') + ' left out.');

  // Moons full together, harvest and blue moons, moonless nights.
  var mev = moons.length ? findMoonEvents(cal, moons, from, to) : [];
  mev.forEach(function (e) {
    if (!scope.set[e.abs]) return;
    var r4 = makeRng(seed, 'sky-moon', e.type, e.abs);
    if (e.type === 'conjunction' && mk.conjunction !== 'off' && r4.chance(Math.min(1, freqMult(mk.conjunction)))) {
      var names = e.moons.map(function (m) { return m.name; }), all = e.moons.length === moons.length && moons.length >= 3;
      events.push(skyEvent(cal, e.abs, { key: makeKey('sky', 'conj', names.join('+'), e.abs), kind: 'conjunction', recipeId: rid, hour: hourOf(cal, 0.85), visibility: vis(e.moons), moon: names.join(', '),
        name: all ? 'Every moon full at once' : names.length === 2 ? r4.pick([names[0] + ' and ' + names[1] + ' full together', 'Twin moons: ' + names[0] + ' and ' + names[1]]) : joinList(names) + ' full together',
        brief: all ? 'Every moon is full tonight. There is no real darkness at all, and nobody sleeps well.' : 'Both moons are full tonight; the night is nearly as bright as dusk and shadows fall two ways.',
        sky: { type: 'conjunction', moons: names, intensity: round2(Math.min(1, 0.5 + 0.2 * names.length)), direction: { compass: 'E', degrees: 90, altitude: 'low' }, duration: { days: 1 } } }));
    }
    if ((e.type === 'harvest-moon' || e.type === 'hunters-moon') && mk['harvest-moon'] !== 'off') {
      var harvest = e.type === 'harvest-moon';
      events.push(skyEvent(cal, e.abs, { key: makeKey('sky', e.type, e.moon.name, e.abs), kind: 'harvest-moon', recipeId: rid, hour: hourOf(cal, 0.78), visibility: vis([e.moon]), moon: e.moon.name,
        name: harvest ? 'Harvest moon' : 'Hunter’s moon',
        brief: harvest ? e.moon.name + ' rises full, huge and orange at dusk, and the harvest goes on by its light.' : 'The full ' + e.moon.name + ' after the harvest moon: light enough to track by, and the deer are fat.',
        sky: { type: harvest ? 'harvest-moon' : 'hunters-moon', moon: e.moon.name, intensity: 0.7, color: SKY_COLOURS.gold, direction: { compass: 'E', degrees: 90, altitude: 'low' }, duration: { days: 1 } } }));
    }
    if (e.type === 'blue-moon' && mk['blue-moon'] !== 'off') {
      var dm = cal.fromAbs(e.abs);
      events.push(skyEvent(cal, e.abs, { key: makeKey('sky', 'blue', e.moon.name, e.abs), kind: 'blue-moon', recipeId: rid, hour: hourOf(cal, 0.85), visibility: vis([e.moon]), moon: e.moon.name,
        name: 'A second full ' + e.moon.name, brief: e.moon.name + ' is full for the second time in ' + cal.months[dm.month - 1].name + '. The almanac-sellers are delighted.',
        sky: { type: 'blue-moon', moon: e.moon.name, intensity: 0.5, duration: { days: 1 } } }));
    }
    if (e.type === 'dark-night' && mk['dark-night'] !== 'off' && r4.chance(Math.min(1, freqMult(mk['dark-night'])))) {
      events.push(skyEvent(cal, e.abs, { key: makeKey('sky', 'dark', e.abs), kind: 'dark-night', recipeId: rid, hour: hourOf(cal, 0.95), visibility: vis(e.moons),
        name: 'Moonless night', brief: 'Every moon is new at once: the darkest night of the season. Smugglers and thieves are busy.',
        sky: { type: 'dark-night', moons: e.moons.map(function (m) { return m.name; }), intensity: 1, duration: { days: 1 } } }));
    }
  });

  // Falling stars: a nightly chance.
  scope.days.forEach(function (t) {
    var r5 = makeRng(seed, 'sky-bolide', t);
    if (mk['falling-star'] !== 'off' && r5.chance(0.012 * freqMult(mk['falling-star']))) {
      var landed = r5.chance(dt.magic === 'none' ? 0.08 : 0.2), place = namePlace(td, makeRng(seed, 'sky-bolide-place', t), used), dir5 = skyDir(r5, ['low', 'mid']);
      var hue = r5.pick(['green', 'warm', 'blue']), fw = fireballWords(makeRng(seed, 'sky-bolide-words', t), place, dir5, hue, landed);
      events.push(skyEvent(cal, t, { key: makeKey('sky', 'bolide', t), kind: 'falling-star', recipeId: rid, hour: hourOf(cal, r5.range(0.8, 0.98)), visibility: vis(),
        name: fw.name, brief: fw.brief,
        sky: { type: landed ? 'star-fall' : 'shooting-star', intensity: round2(r5.range(0.5, 1)), direction: dir5, duration: { minutes: 1 }, color: SKY_COLOURS[hue], landed: landed } }));
    }
  });

  events.sort(function (a, b) { return cal.absOf(a) - cal.absOf(b) || (a.start_hour || 0) - (b.start_hour || 0); });
  return { generator: 'sky', seed: seed, recipe: recipe, scope: { label: scope.label, days: scope.days.length, kind: scope.kind, moons: moons.map(function (m) { return m.name; }) }, events: events, summary: skySummary(cal, events, recipe, scope, moons, notes, latitude), stats: countBy(events, function (e) { return e.gen.kind; }), warnings: scope.moonWarnings.concat(notes) };
}
function countBy(list, f) { var o = {}; list.forEach(function (x) { var k = f(x); o[k] = (o[k] || 0) + 1; }); return o; }
function skySummary(cal, events, recipe, scope, moons, notes, latitude) {
  var by = countBy(events, function (e) { return e.gen.kind; }), parts = [];
  var first = function (k) { return events.filter(function (e) { return e.gen.kind === k; })[0]; };
  var on = function (e) { return cal.fmt({ year: e.year, month: e.month, day: e.day }, scope.years.length > 1); };
  if (by['meteor-shower']) { var yearly = events.filter(function (e) { return e.gen.kind === 'meteor-shower' && e.is_recurring; }); parts.push(yearly.length ? count(yearly.length, 'meteor shower') + ' that come back every year (' + joinList(yearly.map(function (e) { return e.name + ' on ' + on(e); })) + ')' : 'an unexpected meteor shower'); }
  if (by.comet) { var c = first('comet'); parts.push(c.name + ', a comet in the sky from ' + on(c) + ' for ' + numWord(c.sky.duration.days) + ' days'); }
  if (by['eclipse-solar']) parts.push(count(by['eclipse-solar'], 'eclipse') + ' of the sun (first on ' + on(first('eclipse-solar')) + ')');
  if (by['blood-moon']) parts.push(count(by['blood-moon'], 'blood moon') + ' (' + joinList(events.filter(function (e) { return e.gen.kind === 'blood-moon'; }).slice(0, 3).map(function (e) { return e.gen.moon + ' on ' + on(e); })) + ')');
  if (by['eclipse-lunar']) parts.push(count(by['eclipse-lunar'], 'partial eclipse') + ' of a moon');
  if (by.conjunction) parts.push(count(by.conjunction, 'night') + ' when two moons are full together');
  if (by.aurora) parts.push('auroras on ' + count(sum(events.filter(function (e) { return e.gen.kind === 'aurora'; }).map(function (e) { return e.sky.duration.days; })), 'night'));
  if (by['harvest-moon']) parts.push('the harvest and hunter’s moons');
  if (by['blue-moon']) parts.push(count(by['blue-moon'], 'blue moon'));
  if (by['dark-night']) parts.push(count(by['dark-night'], 'moonless night'));
  if (by['falling-star']) parts.push(count(by['falling-star'], 'falling star'));
  var head = 'The sky over ' + scope.label + (moons.length && moons.length < scope.moons.length ? '' : '') + (moons.length === 1 && cal.moons.length > 1 ? ', for ' + moons[0].name + ' only' : '') + ': ';
  var s = parts.length ? head + joinList(parts) + '.' : head + 'nothing out of the ordinary.';
  if (!by.aurora && recipe.makes.aurora !== 'off' && latitude === 'low') s += ' No auroras: this far south they never reach the sky.';
  if (by.comet === undefined && recipe.makes.comet !== 'off' && scope.kind === 'year') s += ' No comet this year.';
  var off = ownKeys(recipe.makes).filter(function (k) { return recipe.makes[k] === 'off'; }).map(function (k) { return SKY_KINDS[k].label.toLowerCase(); });
  if (off.length) s += ' Switched off: ' + joinList(off) + '.';
  if (notes.length) s += ' ' + notes.join(' ');
  if (moons.length) s += ' Eclipses follow each moon’s own cycle, so they fall on the same days whatever the seed.';
  else s += cal.moons.length ? ' No moon is in scope, so there are no eclipses or moon nights.' : ' This calendar has no moons, so there are no eclipses, blood moons or moon nights.';
  return s;
}

/* ── Events: festivals, markets, fairs, and the small happenings of a year ──────────────────────────
   Festivals sit where the calendar's own seasons put the turns of the year (Calendaria's rule: an
   equinox is the first day of its season, a solstice the middle of it) and repeat yearly on the same
   month and day. Markets are one weekly event on a real weekday. Everything smaller is rolled from the
   starter tables, one keyed chance per day, so the owner can copy and edit the very tables used here. */

var EVENT_SWITCHES = {
  'season-festivals': { label: 'Festivals at the turns of the year', help: 'Midwinter, midsummer, the spring and autumn turns, the new year.' },
  harvest: { label: 'Harvest festival' },
  'festival-days': { label: 'Between-month festival days', help: 'An observance for each of the calendar’s own festival days.' },
  remembrance: { label: 'A day of remembrance', default: 'rare' },
  'moon-festivals': { label: 'Moon festivals', help: 'A feast on a particular full moon each year.' },
  markets: { label: 'Market days', help: 'A weekly market on one weekday.' },
  fairs: { label: 'Fairs', help: 'Two- or three-day fairs in spring and autumn.' },
  omens: { label: 'Omens', help: 'Signs and portents, often near a full or new moon.' },
  rumours: { label: 'Rumours', help: 'Talk that could lead somewhere.' },
  civic: { label: 'Town life', help: 'Councils, envoys, weddings, feasts, courts.' },
  trade: { label: 'Work and trade', help: 'Shearing, the pig-killing, caravans, guild dues.' },
  troubles: { label: 'Troubles', help: 'Thefts, fires, missing people: ready-made hooks.' },
  nature: { label: 'Seasons and nature', help: 'The ice breaking, the swallows coming back.' }
};
var EVENT_MAKES = {};
ownKeys(EVENT_SWITCHES).forEach(function (k) { EVENT_MAKES[k] = { label: EVENT_SWITCHES[k].label, help: EVENT_SWITCHES[k].help, type: 'freq', default: EVENT_SWITCHES[k].default || 'normal' }; });
var HAPPENING_TABLES = { omens: 'omens', rumours: 'rumours', civic: 'civic', trade: 'trade-news', troubles: 'troubles', nature: 'nature' };
var HAPPENING_SHARE = { omens: 1, rumours: 1.2, civic: 1.4, trade: 1.2, troubles: 1, nature: 0.8 };
var KIND_CATEGORY = { festival: 'festival', market: 'downtime', fair: 'festival', omens: 'sky', rumours: 'quest', civic: 'social', trade: 'downtime', troubles: 'quest', nature: 'sky' };

defineGenerator({
  id: 'events', label: 'Events',
  blurb: 'Festivals and holidays, market days and fairs, omens and rumours, and the small happenings that make a year feel lived in.',
  makes: EVENT_MAKES,
  scope: { range: true, moons: true, categories: true },
  details: {
    density: { label: 'How busy', type: 'range', default: 0.5, min: 0, max: 1, step: 0.05, help: 'From a quiet year (about one happening a month) to a busy one (about nine).' },
    tone: { label: 'Tone', type: 'select', default: 'balanced', options: [{ value: 'bright', label: 'Bright' }, { value: 'balanced', label: 'Balanced' }, { value: 'grim', label: 'Grim' }] },
    theme: { label: 'Culture for names', type: 'select', default: 'pastoral', options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) },
    marketDay: { label: 'Market day', type: 'weekday', default: 'auto', more: true },
    marketEvery: { label: 'Market repeats', type: 'select', default: 'weekly', more: true, options: [{ value: 'weekly', label: 'Every week' }, { value: 'biweekly', label: 'Every other week' }, { value: 'monthly', label: 'Once a month' }] },
    place: { label: 'Home town', type: 'text', default: '', max: 60, more: true },
    secrets: { label: 'Include secrets only you see', type: 'bool', default: true, more: true },
    moonFestivalMoon: { label: 'Moon for moon festivals', type: 'text', default: '', max: 60, more: true, help: 'Empty: the brightest moon in scope.' }
  },
  builtIns: [
    { id: 'events.market-town', name: 'A market town', description: 'Festivals through the year, a weekly market, fairs, and a steady stream of town life.' },
    { id: 'events.frontier', name: 'A frontier village', description: 'Fewer feasts, more trouble: rumours, raids and things in the woods.', makes: { fairs: 'rare', civic: 'rare', troubles: 'often', rumours: 'often', omens: 'normal' }, details: { tone: 'grim', density: 0.55 } },
    { id: 'events.festive', name: 'A festive year', description: 'Every excuse for a feast; the tone stays bright.', makes: { 'season-festivals': 'often', 'moon-festivals': 'often', fairs: 'often', remembrance: 'normal', troubles: 'rare' }, details: { tone: 'bright', density: 0.6 } },
    { id: 'events.dark-omens', name: 'A year of omens', description: 'Portents gather around the moons; the town grows uneasy.', makes: { omens: 'often', rumours: 'often', troubles: 'normal', fairs: 'rare' }, details: { tone: 'grim', density: 0.6, theme: 'grim' } }
  ],
  defaultRecipe: 'events.market-town',
  run: runEvents
});

function makeEvent(cal, abs, f) {
  var d = cal.fromAbs(abs);
  var ev = { name: f.name, description: f.brief || '', year: d.year, month: d.month, day: d.day, is_recurring: !!f.repeat, visibility: f.visibility || 'everyone', category: f.category, all_day: f.hour == null };
  if (f.repeat) { ev.recurrence_type = f.repeat; if (f.interval) ev.recurrence_interval = f.interval; }
  if (f.hour != null) { ev.start_hour = f.hour; ev.start_minute = 0; }
  if (f.days > 1) { var e = cal.fromAbs(abs + f.days - 1); ev.end_year = e.year; ev.end_month = e.month; ev.end_day = e.day; }
  ev.announce = announceOf(ev, f.announce); // f.announce: an owner's kind of event only, via placeEventKind
  ev.gen = { key: f.key, generator: 'events', kind: f.kind, anchor: f.anchor || null, recipe: f.recipeId, moon: f.moon || null, locked: false };
  return ev;
}

function runEvents(opts) {
  var cats = categoriesOf(opts), mine = eventKindsOf(opts, cats);
  var recipe = resolveRecipe('events', opts.recipe, opts), cal = makeCal(opts.calendar), seed = seedOf(opts), mk = recipe.makes, dt = recipe.details;
  var scopeIn = Object.assign({}, opts.scope || {});
  if (!scopeIn.moons && recipe.scope.moons) scopeIn.moons = recipe.scope.moons;
  if (!scopeIn.categories && recipe.scope.categories) scopeIn.categories = recipe.scope.categories;
  var scope = resolveScope(cal, scopeIn), td = themeData(dt.theme), used = {}, rid = recipe.id || recipe.name;
  var weatherMap = null;
  if (opts.context && opts.context.weather) { weatherMap = {}; opts.context.weather.forEach(function (d) { if (cal.valid(d.year, d.month, d.day)) weatherMap[cal.abs(d.year, d.month, d.day)] = d; }); }
  var rc = rollContextFactory(cal, seed, td, dt.place, scope.moons.filter(function (m) { return m.cycle > 0; }), weatherMap, opts.context && opts.context.climate);
  var locked = (opts.locked || []).filter(function (e) { return e && cal.valid(e.year, e.month, e.day); });
  var lockedKinds = {}, lockedDays = {};
  locked.forEach(function (e) { var k = e.gen && e.gen.kind; if (k) lockedKinds[k] = (lockedKinds[k] || []).concat([cal.absOf(e)]); lockedDays[cal.absOf(e)] = (lockedDays[cal.absOf(e)] || 0) + 1; });
  var nearLocked = function (kind, t, win) { return (lockedKinds[kind] || []).some(function (n) { return Math.abs(n - t) <= win; }); };
  var events = [], festivalDays = {};
  var inScope = function (t) { return !!scope.set[t]; };
  var customsState = { used: {}, depth: 0 };
  var CUSTOMS = { winterSolstice: 'midwinter', summerSolstice: 'midsummer', springEquinox: 'spring', autumnEquinox: 'autumn', harvest: 'harvest', newYear: 'newyear', remembrance: 'remembrance', moon: 'moon', rains: 'rains', dry: 'dry' };
  function custom(t, rr, anchor) { var c = rollOn(STARTER_TABLES, 'customs-' + (CUSTOMS[anchor] || 'day'), dayContext(cal, t, rc.ctx(t)), rr, customsState, null); return c ? c.text : ''; }
  // The first occurrence of a yearly (month, day) inside the scope.
  function firstIn(m, d) {
    for (var yi = 0; yi < scope.years.length; yi++) { var y = scope.years[yi]; if (cal.valid(y, m, d) && !cal.isLeapOnly(y, m, d) && inScope(cal.abs(y, m, d))) return cal.abs(y, m, d); }
    return null;
  }

  // Yearly festivals, placed on a common year so they keep one month and day.
  var refYear = cal.currentYear; while (cal.isLeap(refYear)) refYear++;
  var an = cal.anchorDays(refYear);
  var fests = [];
  if (mk['season-festivals'] !== 'off') {
    fests.push({ anchor: 'winterSolstice', date: an.winterSolstice, hour: null, lead: 'the longest night' });
    fests.push({ anchor: 'summerSolstice', date: an.summerSolstice, lead: 'the longest day' });
    fests.push({ anchor: 'springEquinox', date: an.springEquinox, lead: 'the turn into spring' });
    fests.push({ anchor: 'autumnEquinox', date: an.autumnEquinox, lead: 'the turn into autumn' });
    fests.push({ anchor: 'newYear', date: { year: refYear, month: 1, day: 1 }, lead: 'the first day of the year' });
    if (an.rainsBegin) fests.push({ anchor: 'rains', date: an.rainsBegin, lead: 'the first day of the rains' });
    if (an.dryBegins) fests.push({ anchor: 'dry', date: an.dryBegins, lead: 'the start of the dry' });
  }
  var autumn = cal.seasons.filter(function (s) { return s.type === 'autumn'; })[0];
  if (mk.harvest !== 'off') {
    var hAbs = autumn ? cal.abs(refYear, 1, 1) + Math.round(autumn.startDoy + autumn.length * 0.2) % cal.yearLength(refYear) : cal.absOf(an.autumnEquinox) + 12;
    fests.push({ anchor: 'harvest', date: cal.fromAbs(hAbs), lead: 'the end of the harvest' });
  }
  if (mk.remembrance !== 'off') fests.push({ anchor: 'remembrance', date: cal.fromAbs(cal.absOf(an.autumnEquinox) + Math.round(cal.refYearLength * 0.14)), lead: 'the dark half of the year' });
  var freqGate = function (key, kindMk) { return makeRng(seed, 'ev-fest-gate', key).chance(Math.min(1, freqMult(kindMk) + 0.35)); };
  // Where the calendar already has a festival day between months close to a turn of the year, that day
  // is the festival: it takes the occasion's customs and no second feast is invented beside it.
  var feastDays = cal.months.filter(function (M) { return M.intercalary && M.days >= 1 && M.days <= 5; });
  feastDays.forEach(function (M) { M.anchor = null; });
  if (mk['festival-days'] !== 'off') fests = fests.filter(function (f) {
    var fa = cal.absOf(f.date), best = null;
    feastDays.forEach(function (M) { var ma = cal.abs(refYear, M.index, 1), dd = Math.min(Math.abs(ma - fa), cal.refYearLength - Math.abs(ma - fa)); if (dd <= 20 && !M.anchor && (!best || dd < best.dd)) best = { M: M, dd: dd }; });
    if (best) { best.M.anchor = f.anchor; return false; }
    return true;
  });
  fests.forEach(function (f, i) {
    var d = f.date;
    if (cal.isIntercalary(d.month)) { var nxt = cal.fromAbs(regularDay(cal, cal.absOf(d))); d = nxt; }
    var kindMk = f.anchor === 'harvest' ? mk.harvest : f.anchor === 'remembrance' ? mk.remembrance : mk['season-festivals'];
    if (!freqGate(f.anchor, kindMk)) return;
    while (festivalDays[d.month * 100 + d.day]) d = cal.fromAbs(cal.absOf(d) + 2);
    festivalDays[d.month * 100 + d.day] = 1;
    var t = firstIn(d.month, d.day);
    if (t == null || nearLocked('festival', t, 1)) return;
    var r = makeRng(seed, 'ev-fest', f.anchor);
    var name = nameFestival(td, r, f.anchor, { moon: scope.moons[0] ? scope.moons[0].name : 'the moon' }, used);
    events.push(makeEvent(cal, t, { key: makeKey('ev', seed, 'fest', f.anchor), kind: 'festival', anchor: f.anchor, category: 'festival', repeat: 'yearly', recipeId: rid,
      name: name, brief: cap(f.lead) + '. ' + custom(t, r, f.anchor) }));
  });
  // Between-month festival days: the calendar already names them; the event says how they're kept.
  if (mk['festival-days'] !== 'off') {
    cal.months.forEach(function (M) {
      if (!M.intercalary || M.days < 1 || M.days > 5) return;
      var t = firstIn(M.index, 1);
      if (t == null || nearLocked('festival', t, 0)) return;
      var r = makeRng(seed, 'ev-feastday', M.name);
      if (!r.chance(Math.min(1, freqMult(mk['festival-days']) + 0.35))) return;
      events.push(makeEvent(cal, t, { key: makeKey('ev', seed, 'feastday', M.name), kind: 'festival', anchor: 'festival-day', category: 'festival', repeat: 'yearly', recipeId: rid,
        name: M.name, brief: 'A day outside the weeks' + (M.anchor ? ', kept for ' + ANCHOR_WORDS[M.anchor] : '') + '. ' + custom(t, r, M.anchor), days: M.days }));
    });
  }
  // Moon festivals: the first full moon of spring (moons drift, so each year gets its own date).
  var fm = dt.moonFestivalMoon ? cal.findMoon(dt.moonFestivalMoon) : scope.moons.filter(function (m) { return !m.hidden && m.cycle > 0; }).sort(function (a, b) { return b.size - a.size; })[0];
  if (mk['moon-festivals'] !== 'off' && fm && fm.cycle > 0) {
    scope.years.forEach(function (y) {
      var sp = cal.absOf(cal.anchorDays(y).springEquinox), full = phaseInstants(fm, sp, sp + Math.ceil(fm.cycle) + 1, 0.5)[0];
      var r = makeRng(seed, 'ev-moonfest', fm.name);
      var once = makeRng(seed, 'ev-moonfest-gate', y);
      if (!full || !inScope(full.abs) || !once.chance(Math.min(1, freqMult(mk['moon-festivals']) + 0.35))) return;
      var nm = nameFestival(td, r, 'moon', { moon: fm.name }, used);
      used[nm.toLowerCase()] = 0; // the same festival every year
      events.push(makeEvent(cal, full.abs, { key: makeKey('ev', seed, 'moonfest', fm.name, y), kind: 'festival', anchor: 'moon', category: 'festival', recipeId: rid, moon: fm.name, hour: Math.round(cal.hoursPerDay * 0.8),
        name: nm, brief: 'The first full ' + fm.name + ' after the spring turn. ' + custom(full.abs, r, 'moon'), visibility: fm.hidden ? 'dm_only' : 'everyone' }));
    });
  }
  // The weekly market, one recurring event.
  var marketNote = '';
  if (mk.markets !== 'off' && !lockedKinds.market) {
    var mdIdx = null;
    if (cal.weekLength) {
      if (dt.marketDay !== 'auto') { var f0 = weekdayFilter(cal, dt.marketDay); for (var w = 0; w < cal.weekLength; w++) if (f0 && f0(regularDay(cal, scope.from) + w) ) { mdIdx = cal.weekdayOfAbs(regularDay(cal, scope.from) + w); break; } }
      else {
        var cand = cal.weekdays.filter(function (x) { return !x.rest; }).map(function (x) { return x.index; });
        var mid = Math.floor(cal.weekLength / 2);
        mdIdx = cand.sort(function (a, b) { return Math.abs(a - mid) - Math.abs(b - mid); })[makeRng(seed, 'ev-marketday').int(0, Math.min(1, cand.length - 1))];
      }
    }
    var every = dt.marketEvery === 'monthly' || mdIdx == null ? 'monthly' : dt.marketEvery;
    var startT = null;
    for (var s2 = 0; s2 < scope.days.length; s2++) {
      var tt = scope.days[s2], dd = cal.fromAbs(tt);
      if (cal.isIntercalary(dd.month)) continue;
      if (every === 'monthly' ? true : cal.weekdayOfAbs(tt) === mdIdx) { startT = tt; break; }
    }
    if (startT != null) {
      var r = makeRng(seed, 'ev-market'), dayName = mdIdx != null ? cal.weekdays[mdIdx].name : null, place = rc.home;
      var numbered = dayName && /^\d/.test(dayName), dayWords = numbered ? 'the ' + ordWord(mdIdx + 1) + ' day of each week' : dayName;
      var mname = r.pick(dayName && !numbered ? [place + ' market', dayName + ' market', 'Market day'] : [place + ' market', 'Market day']);
      if (dt.theme === 'nautical') mname = r.pick(['Fish market on the quay', place + ' market']);
      events.push(makeEvent(cal, startT, { key: makeKey('ev', seed, 'market'), kind: 'market', category: 'downtime', repeat: every === 'monthly' ? 'monthly' : every, recipeId: rid, hour: Math.round(cal.hoursPerDay * 0.3),
        name: mname, brief: 'Market day' + (dayName ? (numbered ? ' on ' + dayWords : ' every ' + (every === 'biweekly' ? 'other ' : '') + dayName) : ' once a month') + '. Stalls from the villages around ' + place + ', fresh prices and fresher gossip.' }));
      marketNote = dayName ? (numbered ? 'a market on ' + dayWords : 'a market every ' + (every === 'biweekly' ? 'other ' : '') + dayName) : 'a monthly market';
    }
  }
  // Fairs: spring and autumn, two or three days, their own dates each year.
  if (mk.fairs !== 'off') {
    scope.years.forEach(function (y) {
      var ad = cal.anchorDays(y);
      [['spring', ad.springEquinox, 14], ['autumn', ad.autumnEquinox, 20]].forEach(function (fa) {
        var r = makeRng(seed, 'ev-fair', fa[0], y);
        if (!r.chance(Math.min(1, 0.55 * freqMult(mk.fairs) + 0.2))) return;
        var t = regularDay(cal, cal.absOf(fa[1]) + fa[2] + r.int(-4, 6));
        if (!inScope(t) || nearLocked('fair', t, 3)) return;
        var len = r.int(2, 3);
        var nm = r.pick(fa[0] === 'spring' ? ['Wool Fair', 'Thaw Fair', 'Lambing Fair', 'Spring Fair'] : ['Goose Fair', 'Hiring Fair', 'Apple Fair', 'Horse Fair', 'Autumn Fair']);
        events.push(makeEvent(cal, t, { key: makeKey('ev', seed, 'fair', fa[0], y), kind: 'fair', category: 'festival', recipeId: rid, days: len,
          name: rc.home + ' ' + nm, brief: numWord(len) === 'two' ? 'Two days of stalls, wrestling and bad cider on the green.' : 'Three days of stalls, a horse sale and a travelling show; the inns are full and the watch is tired.' }));
      });
    });
  }
  // The small happenings.
  var perMonth = 0.8 + 8 * dt.density * dt.density, perDay = perMonth / (cal.refYearLength / 12);
  var kinds = ownKeys(HAPPENING_TABLES).filter(function (k) { return mk[k] !== 'off' && (!scope.categories || scope.categories.indexOf(KIND_CATEGORY[k]) >= 0); });
  var totalShare = sum(kinds.map(function (k) { return HAPPENING_SHARE[k] * freqMult(mk[k]); })), baseShare = sum(ownKeys(HAPPENING_SHARE).map(function (k) { return HAPPENING_SHARE[k]; }));
  var pull = moonPullFor(cal, scope.moons.filter(function (m) { return m.cycle > 0; }), 0.7);
  var tone = TONE_MULTS[dt.tone];
  var happenings = kinds.length ? placeRolls({
    cal: cal, scope: scope, seed: seed, stream: 'ev-happening', set: STARTER_TABLES,
    perDay: perDay * totalShare / baseShare, ctx: rc.ctx, secrets: dt.secrets, generator: 'events', recipeId: rid,
    onlyDay: function (t) { return !lockedDays[t] || lockedDays[t] < 2; },
    pickTables: function (t, r) {
      var near = pull ? pull(t) > 1 : false;
      var order = [], pool = kinds.slice();
      while (pool.length) {
        var k = r.weighted(pool, function (x) { return HAPPENING_SHARE[x] * freqMult(mk[x]) * (x === 'omens' && near ? 3 : 1); });
        order.push(HAPPENING_TABLES[k]); pool.splice(pool.indexOf(k), 1);
      }
      return order;
    },
    kindOf: function (tableId) { return ownKeys(HAPPENING_TABLES).filter(function (k) { return HAPPENING_TABLES[k] === tableId; })[0] || tableId; },
    mults: function () { return { tone: tone }; }
  }) : [];
  happenings.forEach(function (e) { if (!e.category) e.category = KIND_CATEGORY[e.gen.kind] || 'quest'; });
  events = events.concat(happenings);
  // The owner's own kinds come last, each on its own stream, so none of them moves anything above.
  mine.forEach(function (k) {
    var mult = freqMult(mk['kind:' + k.id] || 'normal');
    if (mult <= 0 || (k.visibility === 'dm_only' && dt.secrets === false)) return;
    events = events.concat(placeEventKind(k, mult, { cal: cal, scope: scope, seed: seed, ctx: rc.ctx, recipeId: rid,
      dayFree: function (t, kind) { return !(lockedDays[t] >= 2) && !nearLocked(kind, t, 0); } }));
  });
  if (scope.categories) events = events.filter(function (e) { return scope.categories.indexOf(e.category) >= 0; });
  events.sort(function (a, b) { return cal.absOf(a) - cal.absOf(b); });
  return { generator: 'events', seed: seed, recipe: recipe, scope: { label: scope.label, days: scope.days.length, kind: scope.kind }, events: events, locked: locked,
    summary: eventsSummary(cal, events, recipe, scope, marketNote, rc.home, mine), stats: countBy(events, function (e) { return e.gen.kind; }), warnings: scope.moonWarnings };
}

function eventsSummary(cal, events, recipe, scope, marketNote, home, mine) {
  var by = countBy(events, function (e) { return e.gen.kind; }), parts = [];
  var fests = events.filter(function (e) { return e.gen.kind === 'festival' && e.is_recurring; });
  if (fests.length) parts.push(count(fests.length, 'festival') + ' that come back every year (' + (fests.length > 4 ? fests.slice(0, 4).map(function (e) { return e.name; }).join(', ') + ' and more' : joinList(fests.map(function (e) { return e.name; }))) + ')');
  var mf = events.filter(function (e) { return e.gen.anchor === 'moon'; });
  if (mf.length) parts.push(mf.length === 1 ? 'a moon festival on ' + cal.fmt(mf[0]) : count(mf.length, 'moon festival'));
  if (marketNote) parts.push(marketNote + ' in ' + home);
  if (by.fair) parts.push(count(by.fair, 'fair'));
  var small = ['omens', 'rumours', 'civic', 'trade', 'troubles', 'nature'].filter(function (k) { return by[k]; });
  var nSmall = sum(small.map(function (k) { return by[k]; }));
  var words = { omens: 'omen', rumours: 'rumour', civic: 'piece of town life', trade: 'turn of work and trade', troubles: 'trouble', nature: 'seasonal sign' };
  var plurals = { civic: 'pieces of town life', trade: 'turns of work and trade', nature: 'seasonal signs' };
  if (nSmall) parts.push(count(nSmall, 'smaller happening') + ' (' + joinList(small.map(function (k) { return count(by[k], words[k], plurals[k]); })) + ')');
  var s = 'For ' + scope.label + ': ' + (parts.length ? joinList(parts) : 'nothing this time; festivals and markets fall on other days, and happenings are spread thinly at this density') + '.';
  var moonOmens = events.filter(function (e) { return e.gen.kind === 'omens' && e.gen.moon; }).length;
  if (moonOmens) s += ' ' + cap(count(moonOmens, 'omen')) + ' fall' + (moonOmens === 1 ? 's' : '') + ' near a full or new moon.';
  var secrets = events.filter(function (e) { return e.visibility === 'dm_only'; }).length;
  if (secrets) s += ' ' + cap(count(secrets, 'is a secret', 'are secrets')) + ' only you can see.';
  s += ' The tone is ' + recipe.details.tone + '.';
  mine = mine || [];
  var nameOf = function (id) { var k = mine.filter(function (x) { return x.id === id; })[0]; return k ? k.name.toLowerCase() : id; };
  var off = ownKeys(recipe.makes).filter(function (k) { return recipe.makes[k] === 'off'; }).map(function (k) { return k.indexOf('kind:') === 0 ? nameOf(k.slice(5)) : EVENT_SWITCHES[k].label.toLowerCase(); });
  if (mine.length) {
    var times = function (n) { return n === 1 ? 'once' : n === 2 ? 'twice' : numWord(n) + ' times'; }, came = [], none = [];
    mine.forEach(function (k) {
      if (recipe.makes['kind:' + k.id] === 'off') return;
      var n = events.filter(function (e) { return e.gen.kind === 'kind:' + k.id; }).length;
      if (n) came.push(k.name.toLowerCase() + ' ' + times(n)); else none.push(k.name.toLowerCase());
    });
    if (came.length) s += ' Your own kinds of event: ' + joinList(came) + '.';
    if (none.length) s += ' ' + cap(joinList(none)) + ' didn’t come up in this range.';
  }
  if (off.length) s += ' Switched off: ' + joinList(off) + '.';
  return s.replace(/ \(\)/g, '');
}
/* ── Your own kinds of event ──────────────────────────────────────────────────────────────────────────
   An owner describes a kind of happening once (a name, a category, a few examples, when it can happen and
   how often) and the events generator places it as it places its own happenings: a keyed chance per day,
   on the kind's own random stream, with the day's words filled into the examples. It is placed after
   everything else, so switching a kind off leaves every other event exactly as it was. tables.quick turns
   the same kind into a one-table set, so opening it in the full table builder loses nothing. */

var EVENT_KIND_FIELDS = { id: 1, name: 1, category: 1, icon: 1, examples: 1, when: 1, often: 1, per: 1, visibility: 1, sky: 1, look: 1, announce: 1 };
var OFTEN_LEVELS = ['rare', 'normal', 'often'];
var KIND_PERIODS = ['week', 'month', 'year'];
var SKY_SHAPES = ['streak', 'comet', 'curtain', 'glow', 'ring', 'star', 'doorway', 'veil'];
var SKY_SIZES = ['small', 'medium', 'large'];
var SKY_COMPASS = ['N', 'NE', 'E', 'SE', 'S', 'SW', 'W', 'NW'];
var SKY_ALTITUDES = ['low', 'mid', 'high'];
var SHAPE_HOURS = { streak: 1, comet: 6, curtain: 4, glow: 3, ring: 2, star: 2, doorway: 1, veil: 5 };
var DAY_WORDS_SHOWN = '{moon}, {home}, {place}, {person}, {weekday}, {month} and {season}';

/* A sky spec says which of the renderer's hand-tuned shapes draws an owner's event, and where. */
function checkSkySpec(sky, label, path, errors, warnings) {
  if (!isObj(sky)) { errors.push({ path: path, message: label + '’s sky should look like {"shape": "glow", "color": "#9fe0ff"}.' }); return; }
  if (SKY_SHAPES.indexOf(sky.shape) < 0) errors.push({ path: path + '.shape', message: label + '’s sky needs a shape the renderer can draw: ' + joinList(SKY_SHAPES).replace(/ and ([^ ]+)$/, ' or $1') + (sky.shape != null ? '. ' + friendlyValue(sky.shape) + ' isn’t one of those.' : '.') });
  if (sky.color != null && !(typeof sky.color === 'string' && HEX_RE.test(sky.color))) errors.push({ path: path + '.color', message: label + '’s sky colour should be written like "#9fe0ff".' });
  if (sky.size != null && SKY_SIZES.indexOf(sky.size) < 0) errors.push({ path: path + '.size', message: label + '’s sky size can be small, medium or large; ' + friendlyValue(sky.size) + ' isn’t one of those.' });
  if (sky.where != null) {
    if (!isObj(sky.where)) errors.push({ path: path + '.where', message: label + '’s sky position should look like {"compass": "N", "altitude": "low"}.' });
    else {
      if (sky.where.compass != null && SKY_COMPASS.indexOf(sky.where.compass) < 0) errors.push({ path: path + '.where.compass', message: label + '’s compass point can be N, NE, E, SE, S, SW, W or NW; ' + friendlyValue(sky.where.compass) + ' isn’t one of those.' });
      if (sky.where.altitude != null && SKY_ALTITUDES.indexOf(sky.where.altitude) < 0) errors.push({ path: path + '.where.altitude', message: label + '’s height in the sky can be low, mid or high; ' + friendlyValue(sky.where.altitude) + ' isn’t one of those.' });
    }
  }
  if (sky.hours != null && !(typeof sky.hours === 'number' && sky.hours === Math.floor(sky.hours) && sky.hours >= 1 && sky.hours <= 12)) errors.push({ path: path + '.hours', message: label + ' can stay in the sky for 1 to 12 hours, as a whole number; ' + friendlyValue(sky.hours) + ' isn’t.' });
  ownKeys(sky).forEach(function (k) { if (['shape', 'color', 'size', 'where', 'hours'].indexOf(k) < 0) warnings.push({ path: path + '.' + k, message: '“' + k + '” isn’t part of a sky, so it will be ignored.' }); });
}

/* What an owner-made event looks like in the sky, shaped as the built-in sky events are. */
function attachCustomSky(cal, t, ev, spec, r, hour) {
  var where = spec.where || {}, compass = where.compass || r.pick(SKY_COMPASS), altitude = where.altitude || r.pick(SKY_ALTITUDES);
  var h = hour != null ? hour : hourOf(cal, r.range(0.8, 0.96)), d = cal.fromAbs(t);
  var colour = spec.color || SKY_COLOURS[r.pick(['warm', 'blue', 'green', 'violet'])];
  ev.all_day = false; ev.start_hour = h; ev.start_minute = 0;
  ev.sky = { type: 'custom', shape: spec.shape, color: colour, size: spec.size || 'medium', intensity: round2(r.range(0.4, 0.9)),
    direction: { compass: compass, degrees: DIR_DEG[compass], altitude: altitude }, duration: { hours: spec.hours || SHAPE_HOURS[spec.shape] || 2 },
    peak: { year: d.year, month: d.month, day: d.day, hour: h } };
  return ev;
}

function exampleOf(x) { return typeof x === 'string' ? { name: x } : x; }
/* Plain-language checks for one kind of event. opts.categories: the campaign's own; opts.others: the
   owner's other kinds, for clashing ids. */
function validateEventKind(kind, opts) {
  opts = opts || {};
  var errors = [], warnings = [], cats = categoryIds(opts.categories);
  function err(path, msg) { errors.push({ path: path, message: msg }); }
  if (!isObj(kind)) return { ok: false, errors: [{ path: '', message: 'A kind of event should be a set of settings: an id, a name, a category and some examples.' }], warnings: [] };
  var named = typeof kind.name === 'string' && kind.name.trim(), label = named ? '“' + named + '”' : 'This kind of event', it = named ? label : 'this kind of event';
  if (typeof kind.id !== 'string' || !kind.id) err('id', 'Give ' + it + ' an id: lowercase letters, numbers and dashes, starting with a letter, like "bandit-raid".');
  else if (!/^[a-z][a-z0-9-]*$/.test(kind.id)) err('id', '“' + kind.id + '” can’t be an id: use lowercase letters, numbers and dashes, starting with a letter, like "bandit-raid".');
  else if (TEMPLATE_VARS[kind.id]) err('id', '“' + kind.id + '” is reserved for the day’s own ' + kind.id + '; give the kind another id.');
  else if ((opts.others || []).some(function (o) { return o !== kind && isObj(o) && o.id === kind.id; })) err('id', 'Another of your kinds of event already uses the id “' + kind.id + '”.');
  if (typeof kind.name !== 'string' || !kind.name.trim()) err('name', 'Give your kind of event a name, like "Bandit raid".');
  else if (kind.name.length > 60) err('name', label + ' is ' + kind.name.length + ' characters long; keep names under 60.');
  if (cats.indexOf(kind.category) < 0) err('category', label + ' needs a category: ' + joinList(cats).replace(/ and ([^ ]+)$/, ' or $1') + (kind.category != null ? '. ' + friendlyValue(kind.category) + ' isn’t one of those.' : '.'));
  if (kind.icon != null && (typeof kind.icon !== 'string' || kind.icon.length > 40)) err('icon', label + '’s icon should be a short name.');
  if (!Array.isArray(kind.examples) || !kind.examples.length) err('examples', 'Give ' + it + ' one to twelve examples, like [{"name": "Raiders at {place}", "brief": "Smoke on the hills by morning."}].');
  else {
    if (kind.examples.length > 12) err('examples', label + ' has ' + kind.examples.length + ' examples; keep it to twelve.');
    kind.examples.forEach(function (raw, i) {
      var ex = exampleOf(raw), where = 'examples[' + i + ']', lab = 'Example ' + (i + 1) + ' of ' + it;
      if (!isObj(ex) || typeof ex.name !== 'string' || !ex.name.trim()) { err(where, lab + ' needs a name.'); return; }
      if (ex.name.length > 80) err(where + '.name', lab + ' has a name of ' + ex.name.length + ' characters; keep it under 80.');
      if (ex.brief != null && typeof ex.brief !== 'string') err(where + '.brief', lab + '’s brief should be text.');
      else if (ex.brief && ex.brief.length > 300) err(where + '.brief', lab + '’s brief is ' + ex.brief.length + ' characters; keep it under 300.');
      [ex.name, ex.brief].forEach(function (s) {
        templateRefs(s).forEach(function (w) { if (!TEMPLATE_VARS[w.toLowerCase()]) err(where, lab + ' uses “{' + w + '}”, which isn’t one of the day’s words: ' + DAY_WORDS_SHOWN + '.'); });
      });
    });
  }
  if (kind.when != null) {
    var we = [];
    checkWhen(kind.when, label, '', we);
    we.forEach(function (e) { errors.push({ path: e.path.replace(/^\./, ''), message: e.message }); });
  }
  if (kind.often != null && OFTEN_LEVELS.indexOf(kind.often) < 0) err('often', 'How often ' + it + ' happens can be rare, normal or often; ' + friendlyValue(kind.often) + ' isn’t one of those.');
  if (kind.per != null && KIND_PERIODS.indexOf(kind.per) < 0) err('per', label + ' can be counted per week, month or year; ' + friendlyValue(kind.per) + ' isn’t one of those.');
  if (kind.visibility != null && kind.visibility !== 'everyone' && kind.visibility !== 'dm_only') err('visibility', 'Who sees ' + it + ' can be “everyone” or “dm_only”.');
  if (kind.announce != null) { var ap = announceProblem(kind.announce); if (ap) err('announce', label + ' ' + ap); }
  if (kind.sky != null) checkSkySpec(kind.sky, label, 'sky', errors, warnings);
  checkLook(kind.look, label, 'look', errors, warnings);
  ownKeys(kind).forEach(function (k) { if (!EVENT_KIND_FIELDS[k]) warnings.push({ path: k, message: '“' + k + '” isn’t a setting for a kind of event, so it will be ignored.' }); });
  return { ok: !errors.length, errors: errors, warnings: warnings };
}
function normaliseEventKind(k) {
  return { id: k.id, name: k.name.trim(), category: k.category, icon: k.icon || null,
    examples: k.examples.map(exampleOf).map(function (e) { return e.brief ? { name: e.name, brief: e.brief } : { name: e.name }; }),
    when: k.when ? clone(k.when) : null, often: k.often || 'normal', per: k.per || 'month', visibility: k.visibility || 'everyone',
    sky: k.sky ? clone(k.sky) : null, look: k.look ? clone(k.look) : null,
    announce: k.announce === 'ahead' || k.announce === 'on-the-day' ? k.announce : null };
}
/* The kinds a run was given, checked together; any problem stops the run in the owner's own words. */
function eventKindsOf(opts, cats) {
  var list = opts && opts.eventKinds;
  if (list == null) return [];
  if (!Array.isArray(list)) throw GenError('Your kinds of event should be a list.');
  var errs = [];
  list.forEach(function (k) { validateEventKind(k, { categories: cats, others: list }).errors.forEach(function (e) { errs.push(e.message); }); });
  if (errs.length) throw GenError('Your kinds of event have ' + count(errs.length, 'problem') + ': ' + errs.join(' '));
  return list.map(normaliseEventKind);
}
/* A one-table set rolling the kind's examples under its conditions, and a table recipe at its rate. */
function quickTables(kind, opts) {
  var cats = categoriesOf(opts), v = validateEventKind(kind, { categories: cats });
  if (!v.ok) throw GenError('This kind of event has ' + count(v.errors.length, 'problem') + ': ' + v.errors.map(function (e) { return e.message; }).join(' '), v.errors);
  var k = normaliseEventKind(kind), table = { id: k.id, name: k.name };
  if (k.icon) table.icon = k.icon;
  table.output = { kind: k.category };
  if (k.visibility !== 'everyone') table.output.visibility = k.visibility;
  if (k.announce) table.output.announce = k.announce;
  table.entries = k.examples.map(function (ex) {
    var e = { name: ex.name };
    if (ex.brief) e.brief = ex.brief;
    if (k.when) e.when = clone(k.when);
    if (k.sky) e.sky = clone(k.sky);
    return e;
  });
  var set = { format: TABLE_FORMAT, version: 1, id: k.id, name: k.name, tables: [table] };
  if (k.look) set.look = clone(k.look);
  var rv = validateRecipe({ format: RECIPE_FORMAT, version: 1, generator: 'table', name: k.name, description: 'Rolls “' + k.name + '” ' + (k.often === 'normal' ? 'about once' : k.often === 'often' ? 'about twice' : 'now and then') + ' a ' + k.per + '.',
    tables: set, start: k.id, details: { rate: FREQ[k.often], per: k.per, moonPull: 0, visibility: k.visibility === 'dm_only' ? 'dm_only' : 'table' } }, { categories: cats });
  if (!rv.ok) throw GenError('The table recipe for this kind can’t be made: ' + rv.errors.map(function (e) { return e.message; }).join(' '));
  return { set: set, recipe: rv.recipe };
}
/* Places one kind over the run's scope. Examples cycle through a keyed order each year, counting the
   kind's earlier days that year, so a week asked for alone says what that week of the year says. */
function placeEventKind(k, mult, o) {
  var cal = o.cal, perDay = FREQ[k.often] * mult / periodDays(cal, k.per), n = k.examples.length, byYear = {}, out = [];
  function happens(t) {
    if (!makeRng(o.seed, 'ev-kind', k.id, t).chance(Math.min(0.95, perDay))) return false;
    return !k.when || whenOk(k.when, dayContext(cal, t, o.ctx(t)), null);
  }
  function example(t) {
    var y = cal.fromAbs(t).year;
    if (!byYear[y]) {
      var sp = cal.yearSpan(y), at = {}, c = 0;
      for (var a = sp[0]; a <= sp[1]; a++) if (happens(a)) at[a] = c++;
      byYear[y] = { at: at, rounds: [] };
    }
    var Y = byYear[y], i = Y.at[t], round = Math.floor(i / n);
    // Each round through the examples gets its own keyed order, and never opens with the one that closed the last.
    for (var r = Y.rounds.length; r <= round; r++) {
      var ord = makeRng(o.seed, 'ev-kind-order', k.id, y, r).shuffle(k.examples.map(function (x, j) { return j; }));
      if (r && n > 1 && ord[0] === Y.rounds[r - 1][n - 1]) { var sw = ord[0]; ord[0] = ord[1]; ord[1] = sw; }
      Y.rounds.push(ord);
    }
    return k.examples[Y.rounds[round][i % n]];
  }
  o.scope.days.forEach(function (t) {
    if (!o.dayFree(t, 'kind:' + k.id) || !happens(t)) return;
    var ex = example(t), entry = { name: ex.name };
    if (ex.brief) entry.brief = ex.brief;
    if (k.when) entry.when = k.when;
    var set = { tables: [{ id: k.id, entries: [entry] }] };
    var roll = rollOn(set, k.id, dayContext(cal, t, o.ctx(t)), makeRng(o.seed, 'ev-kind-words', k.id, t), { used: {}, depth: 0 }, null);
    if (!roll) return;
    var ev = makeEvent(cal, t, { key: makeKey('ev', o.seed, 'kind', k.id, t), kind: 'kind:' + k.id, category: k.category, recipeId: o.recipeId, moon: roll.moon, visibility: k.visibility, name: cap(roll.name), brief: roll.brief || '', announce: k.announce });
    if (k.sky) attachCustomSky(cal, t, ev, k.sky, makeRng(o.seed, 'ev-kind-sky', k.id, t), null);
    out.push(ev);
  });
  return out;
}

/* ── History ────────────────────────────────────────────────────────────────────────────────────────
   Eras follow a story arc (a founding, then plenty or strife, dark years, a restoration...), and each new
   era begins with the event that caused it. Inside an era, events come in threads that point at each
   other: a war has its cause, its battles and its treaty; a plague its aftermath; a crowning its heir.
   Every event lands on a real day of the calendar, in a season that suits it (wars start in spring,
   plagues come in the heat, coronations fall on feast days). */

var HISTORY_KINDS = {
  foundings: { label: 'Foundings', help: 'Towns, orders and guilds founded.' },
  wars: { label: 'Wars', help: 'Each with its cause, battles and treaty.' },
  plagues: { label: 'Plagues' },
  famines: { label: 'Famines' },
  discoveries: { label: 'Discoveries', help: 'Seams of silver, new roads, charted coasts.' },
  coronations: { label: 'Crownings and successions' },
  disasters: { label: 'Disasters', help: 'Floods, fires, earthquakes.' },
  portents: { label: 'Portents', help: 'Comets and eclipses remembered before great events.' },
  migrations: { label: 'Migrations' },
  schisms: { label: 'Schisms and faiths' }
};
var HISTORY_MAKES = {};
ownKeys(HISTORY_KINDS).forEach(function (k) { HISTORY_MAKES[k] = { label: HISTORY_KINDS[k].label, help: HISTORY_KINDS[k].help, type: 'freq', default: 'normal' }; });
var ERA_NEXT = {
  founding: { golden: 3, strife: 2, discovery: 2, faith: 1 }, golden: { strife: 3, decline: 2, faith: 1, discovery: 1 },
  strife: { dark: 2, restoration: 3, golden: 1 }, dark: { restoration: 3, faith: 2, strife: 1 }, restoration: { golden: 3, discovery: 2, strife: 1 },
  faith: { strife: 2, golden: 2, dark: 1 }, discovery: { golden: 2, strife: 2, faith: 1 }, decline: { dark: 2, strife: 3, restoration: 1 }
};
var ERA_HUE = { founding: 50, golden: 78, strife: 25, dark: 290, restoration: 145, faith: 330, discovery: 195, decline: 235, present: 220 };
var HIST_THETA = { war: [0.22, 0.45], battle: [0.3, 0.7], siege: [0.35, 0.8], treaty: [0.85, 1.1], plague: [0.45, 0.72], famine: [0.05, 0.3], coronation: [0.2, 0.4], founding: [0.2, 0.45], flood: [0.18, 0.3], fire: [0.55, 0.75], quake: [0, 1], discovery: [0.3, 0.7], migration: [0.25, 0.5], schism: [0, 1], death: [0, 1], portent: [0, 1], charter: [0.2, 0.6] };

defineGenerator({
  id: 'history', label: 'History',
  blurb: 'Eras and the events of past centuries, placed on real dates, with wars that end in treaties and plagues that leave their mark.',
  makes: HISTORY_MAKES,
  scope: {},
  details: {
    years: { label: 'How far back', type: 'count', default: 0, min: 0, max: 5000, help: '0 lets the generator choose from the calendar’s current year.' },
    eras: { label: 'How many eras', type: 'count', default: 0, min: 0, max: 8, help: '0 chooses from the length of the history.' },
    density: { label: 'Events per century', type: 'number', default: 4, min: 0.5, max: 20, step: 0.5 },
    tone: { label: 'Tone', type: 'select', default: 'balanced', options: [{ value: 'bright', label: 'Hopeful' }, { value: 'balanced', label: 'Balanced' }, { value: 'grim', label: 'Grim' }] },
    theme: { label: 'Culture for names', type: 'select', default: 'pastoral', options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) },
    realm: { label: 'Name of the realm', type: 'text', default: '', max: 60, more: true },
    secrets: { label: 'Include hidden history only you see', type: 'bool', default: true, more: true }
  },
  builtIns: [
    { id: 'history.kingdom', name: 'A kingdom’s history', description: 'Dynasties, wars of succession, plagues and restorations.' },
    { id: 'history.frontier', name: 'A frontier’s history', description: 'Settlers, discoveries and migrations; wars are smaller and nearer.', makes: { discoveries: 'often', foundings: 'often', migrations: 'often', coronations: 'rare', schisms: 'rare' }, details: { tone: 'bright' } },
    { id: 'history.dark', name: 'A dark history', description: 'Plagues, famines, broken crowns and omens that came true.', makes: { plagues: 'often', famines: 'often', portents: 'often', disasters: 'often', discoveries: 'rare' }, details: { tone: 'grim', theme: 'grim' } }
  ],
  defaultRecipe: 'history.kingdom',
  run: runHistory
});

function runHistory(opts) {
  var recipe = resolveRecipe('history', opts.recipe), cal = makeCal(opts.calendar), seed = seedOf(opts), mk = recipe.makes, dt = recipe.details, td = themeData(dt.theme);
  var used = {}, R = function () { var a = [seed, 'history']; for (var i = 0; i < arguments.length; i++) a.push(arguments[i]); return makeRng.apply(null, a); };
  var now = cal.currentYear;
  var span = dt.years > 0 ? dt.years : (now >= 1000 ? R('span').int(700, 1100) : now >= 200 ? Math.round(now * 0.8) : Math.max(0, now - 1));
  var startYear = Math.max(1, now - span);
  span = now - startYear;
  var warnings = [];
  if (span < 40) warnings.push('This calendar starts at year ' + cal.currentYear + ', so there are only ' + span + ' years of history to fill; raise the current year or set a longer history.');
  var wantEras = dt.eras > 0 ? dt.eras : clamp(Math.round(span / 180), span < 80 ? 1 : 3, 6);
  wantEras = Math.max(1, Math.min(wantEras, Math.floor(span / 15) || 1));
  var toneBias = { bright: { golden: 1.6, restoration: 1.5, discovery: 1.4, dark: 0.5, strife: 0.7, decline: 0.6 }, grim: { dark: 1.8, strife: 1.5, decline: 1.5, golden: 0.6, restoration: 0.8 } }[dt.tone] || {};
  var kindOn = function (k) { return mk[k] !== 'off'; };
  // Characters: an arc, not a coin toss per era.
  var er = R('eras'), chars = ['founding'];
  for (var i = 1; i < wantEras - 1; i++) {
    var prev = chars[i - 1], opts2 = ERA_NEXT[prev] || ERA_NEXT.golden, keys = ownKeys(opts2).filter(function (k) { return !(k === 'dark' && !kindOn('plagues') && !kindOn('famines') && !kindOn('disasters')) && !(k === 'strife' && !kindOn('wars')); });
    chars.push(er.weighted(keys.length ? keys : ['golden'], function (k) { return (opts2[k] || 1) * (toneBias[k] || 1) * (chars.indexOf(k) >= 0 ? 0.5 : 1); }));
  }
  if (wantEras > 1) chars.push('present');
  var cuts = [startYear], minLen = Math.max(8, Math.floor(span / (wantEras * 2.5)));
  var weights = chars.map(function () { return er.range(0.7, 1.4); }), wsum = sum(weights), acc = startYear;
  for (i = 0; i < chars.length - 1; i++) { acc += Math.max(minLen, Math.round(span * weights[i] / wsum)); cuts.push(Math.min(acc, now - Math.max(1, Math.floor(minLen / 2)))); }
  var locked = opts.locked || {}, lockedEras = (locked.eras || []).slice(), lockedEvents = (locked.events || []).slice();
  var realm = dt.realm || (R('realm').chance(0.5) ? 'the ' + namePlace(td, R('realm-name'), used) + ' lands' : 'the realm of ' + namePlace(td, R('realm-name2'), used));
  var places = [], pr = R('places');
  for (i = 0; i < 16; i++) places.push(namePlace(td, pr, used));
  var home = places[0];
  // One place per kind of story: the same bridge is not built twice.
  var placeUse = {};
  function placeFor(kind, r, avoid, fits) {
    var free = places.slice(1).filter(function (p) { return !(placeUse[kind] || {})[p] && p !== avoid; });
    var good = fits ? free.filter(fits) : free;   // no "bridge at Harrow Bridge"
    var p = good.length ? r.pick(good) : free.length ? r.pick(free) : r.pick(places.slice(1));
    (placeUse[kind] = placeUse[kind] || {})[p] = 1;
    return p;
  }
  // Rulers: a house reuses a handful of names, so regnal numbers come naturally.
  var house = null;
  function newHouse(r) {
    var givens = [];
    for (var g = 0; g < 4; g++) givens.push(namePerson(td, r, {}, { title: false }).split(' ')[0]);
    house = { name: namePlace(td, r, used), givens: uniq(givens), count: {} };
  }
  function ruler(r) {
    if (!house || r.chance(0.2)) newHouse(r);
    var g = r.pick(house.givens);
    house.count[g] = (house.count[g] || 0) + 1;
    return g + (house.count[g] > 1 ? ' ' + roman(house.count[g]) : '');
  }
  // Templates are used once each per history where possible; a repeat is a last resort.
  var tUse = {};
  function variantIdx(key, r, n) {
    var fresh = [];
    for (var j = 0; j < n; j++) if (!tUse[key + '#' + j]) fresh.push(j);
    var i = fresh.length ? r.pick(fresh) : r.int(0, n - 1);
    tUse[key + '#' + i] = 1;
    return i;
  }
  function variant(key, r, list) { return list[variantIdx(key, r, list.length)]; }
  // A story can be told several ways; each telling is used once before any repeats.
  function tell(key, r, d) { return Array.isArray(d) ? d[variantIdx(key, r, d.length)] : d; }
  var names = {};
  var events = [], eras = [];
  var dayIn = function (year, kind, r) {
    var w = HIST_THETA[kind] || [0, 1], target = mod(r.range(w[0], w[1]), 1), len = cal.yearLength(year), bestD = 0, bd = 9, s0 = cal.abs(year, 1, 1);
    for (var d = 0; d < len; d += 3) { var dd = angleDiff(cal.thetaOfAbs(s0 + d) * 360, target * 360); if (dd < bd) { bd = dd; bestD = d; } }
    var t = s0 + clamp(bestD + r.int(-2, 2), 0, len - 1), dte = cal.fromAbs(t);
    if (cal.isIntercalary(dte.month) && kind !== 'coronation') dte = cal.fromAbs(regularDay(cal, t));
    return dte;
  };
  function ev(kind, year, name, brief, r, extra) {
    extra = extra || {};
    if (year > now || year < startYear || names[name]) return null;
    if (extra.secret && !dt.secrets) return null;
    var d = dayIn(year, extra.theta || kind, r);
    var cat = { war: 'quest', battle: 'quest', siege: 'quest', rebellion: 'quest', raid: 'quest', plague: 'quest', famine: 'quest', flood: 'quest', fire: 'quest', quake: 'quest', portent: 'sky', discovery: 'quest', migration: 'social' }[kind] || 'social';
    // Past events sit in their own category, so history never mixes into the views of play; gen.was keeps
    // the category the event would have had in its own day.
    var e = { name: name, description: brief, year: d.year, month: d.month, day: d.day, is_recurring: false, visibility: extra.secret ? 'dm_only' : 'everyone', category: 'history', all_day: true,
      announce: 'on-the-day', // settled history, never something players learn of ahead of the day it's told
      gen: { key: makeKey('hist', seed, kind, year, name), generator: 'history', kind: kind, thread: extra.thread || null, causedBy: extra.cause ? extra.cause.gen.key : null, era: extra.era != null ? extra.era : null, told: extra.told || null, locked: false, was: cat } };
    names[name] = 1;
    events.push(e);
    return e;
  }
  var causes = ['Salt Roads', 'Two Crowns', 'River Tolls', 'Broken Oath', 'Three Winters', 'Iron Pass', 'Stolen Bride', 'Bridges', 'Wool Tax'];
  var plagues = ['the Grey Cough', 'the Weeping Sickness', 'the Red Fever', 'the Sweating Death', 'the Pale Rot', 'the Shaking Ague', 'the Blue Lung'];

  chars.forEach(function (ch, ei) {
    var y0 = cuts[ei], y1 = ei + 1 < cuts.length ? cuts[ei + 1] - 1 : null, len = (y1 == null ? now : y1) - y0 + 1;
    var r = R('era', ei), eraName = nameEra(td, r, ch, used), pivot = null, pr2 = R('pivot', ei);
    // The event that opens this era.
    if (ei === 0) {
      pivot = ev('founding', y0, home + ' founded', variant('found', r, ['Settlers clear the ' + r.pick(['thornwood', 'river meadows', 'high valley', 'marsh edge']) + ' and raise the first walls of ' + home + '.', 'The first charter of ' + home + ' is signed under an oak that still stands.', 'A ford, a mill and a shrine: ' + home + ' begins as all three.']), pr2, { era: ei, thread: 'origin', told: home + ' was founded' });
    } else if (ch === 'strife' && kindOn('wars')) {
      var a = placeFor('war', r), b = placeFor('treaty', r, a), heir = house ? house.name : a;
      var wn = variant('war', r, ['The War of the ' + r.pick(causes), 'The ' + a + ' War', 'The ' + heir + ' War of Succession', 'The War of the ' + r.pick(causes)]);
      if (names[wn + ' begins']) wn = 'The Second ' + wn.replace(/^The /, '');
      pivot = ev('war', y0, wn + ' begins', variant('warcause', r, ['The ' + a + ' lords refuse the crown’s tithe, and the crown sends soldiers.', 'A disputed crown: two heirs, two armies, and ' + realm + ' split down the middle.', 'Raiders from beyond ' + b + ' burn the border towns, and the realm answers.', 'A murdered envoy, a burned bridge and a king who would not apologise.']), pr2, { era: ei, thread: 'war-' + ei, told: lowerFirst(wn.replace(/^The /, 'the ')) + ' began' });
      if (pivot) {
        var bY = y0 + r.int(1, 3), tY = y0 + r.int(4, Math.max(5, Math.min(30, len - 1))), field = placeFor('battle', r, a);
        var bat = ev('battle', bY, 'Battle of ' + field, variant('battle', r, ['The decisive field of ' + wn.replace(/^The /, 'the ') + '; the dead are still turned up by ploughs.', 'Fought in fog at ' + field + '; both sides claimed the victory until the counting was done.', 'A charge across the river at ' + field + ' ends the war in all but name.']), pr2, { era: ei, cause: pivot, thread: 'war-' + ei });
        if (r.chance(0.6)) ev('siege', bY + r.int(0, 2), 'Siege of ' + a, a + ' holds out for ' + numWord(r.int(3, 11)) + ' months, eating its horses by the end.', pr2, { era: ei, cause: pivot, thread: 'war-' + ei });
        ev('treaty', tY, 'Treaty of ' + b, variant('treaty', r, ['Ends ' + wn.replace(/^The /, 'the ') + '. Nobody is satisfied, which is how everyone knows it is fair.', 'Signed at ' + b + ' in a tent between the armies; the border is drawn along a river that has since moved.', 'Peace, a marriage and a ransom nobody finished paying.']), pr2, { era: ei, cause: bat || pivot, thread: 'war-' + ei });
      }
    } else if (ch === 'dark') {
      if (kindOn('plagues') && (r.chance(0.6) || (!kindOn('famines') && !kindOn('disasters')))) {
        var pn = variant('plague', r, plagues), where = placeFor('plague', r), empty = placeFor('abandoned', r, where);
        pivot = ev('plague', y0, cap(pn) + ' comes to ' + where, 'It arrives with a ' + r.pick(['wool ship', 'caravan', 'band of pilgrims', 'returning army']) + '; within a year one in ' + r.pick(['four', 'five', 'three']) + ' is dead.', pr2, { era: ei, thread: 'plague-' + ei, told: pn + ' came to ' + where });
        if (pivot) {
          ev('plague', y0 + r.int(1, 3), empty + ' abandoned', 'The survivors of ' + pn + ' leave ' + empty + ' to the brambles. Its bell still hangs in the empty tower.', pr2, { era: ei, cause: pivot, thread: 'plague-' + ei });
          if (kindOn('schisms')) ev('founding', y0 + r.int(2, 8), 'The Order of the ' + variant('order', r, ['Lantern', 'Open Hand', 'White Linen', 'Quiet Bell']) + ' founded', 'Founded to nurse the sick of ' + pn + '; it outlives the plague by centuries.', pr2, { era: ei, cause: pivot, thread: 'plague-' + ei });
        }
      } else if (kindOn('famines') && (r.chance(0.6) || !kindOn('disasters'))) {
        var fn = 'The ' + variant('famine', r, ['Hungry', 'Three Lean', 'Black', 'Long']) + ' Winters', riot = placeFor('riot', r);
        pivot = ev('famine', y0, fn + ' begin', 'The harvest fails ' + r.pick(['two years', 'three years', 'four years']) + ' running; the granaries are empty by midwinter.', pr2, { era: ei, thread: 'famine-' + ei, told: lowerFirst(fn) + ' began' });
        if (pivot) ev('rebellion', y0 + r.int(1, 2), 'Grain riots in ' + riot, 'The granary of ' + riot + ' is stormed; the reeve hangs three men and gives away the rest of the grain.', pr2, { era: ei, cause: pivot, thread: 'famine-' + ei });
      } else if (kindOn('disasters')) {
        var dk = r.pick(['flood', 'fire', 'quake']), dp = placeFor(dk, r);
        pivot = ev(dk, y0, dk === 'flood' ? 'The Great Flood at ' + dp : dk === 'fire' ? 'The Burning of ' + dp : 'The earth shakes at ' + dp, dk === 'flood' ? 'The river takes the lower town of ' + dp + ' in one night; the flood mark is cut into the church wall.' : dk === 'fire' ? 'A baker’s oven, a dry summer and a wind from the south: ' + dp + ' burns for two days.' : 'The ground opens at ' + dp + '; the old keep falls and the springs run hot for a month.', pr2, { era: ei, thread: 'disaster-' + ei, told: dk === 'flood' ? 'the river drowned ' + dp : dk === 'fire' ? dp + ' burned' : 'the earth shook at ' + dp });
        if (pivot) ev('founding', y0 + r.int(2, 10), dp + ' rebuilt in stone', 'The new ' + dp + ' is built of stone and wider streets, paid for with a tax nobody has forgotten.', pr2, { era: ei, cause: pivot, thread: 'disaster-' + ei });
      }
    } else if ((ch === 'restoration' || ch === 'golden' || ch === 'present') && kindOn('coronations')) {
      if (ch === 'restoration') newHouse(r);
      var rn = ruler(r);
      pivot = ev('coronation', y0, rn + ' crowned', variant('crown', r, ['Crowned at ' + home + ' on a feast day, with the old crown found in a monastery chest.', 'The lords agree on ' + rn + ' at last, mostly because nobody else will take it.', 'A crowning in the rain; the omens are argued about for a generation.', 'Crowned before the whole army, which had just put ' + rn + ' there.']), pr2, { era: ei, thread: 'dynasty-' + ei, theta: 'coronation', told: rn + ' was crowned' });
    } else if (ch === 'discovery' && kindOn('discoveries')) {
      var dp2 = placeFor('discovery', r), what = variant('found-what', r, ['silver', 'good iron', 'salt', 'a pass through the mountains', 'a sea road to the south']);
      var road = what.indexOf('pass') >= 0 || what.indexOf('road') >= 0;
      pivot = ev('discovery', y0, cap(what) + (road ? ' found' : ' found near ' + dp2), 'Found by ' + namePerson(td, r, used, { title: false }) + ', who ' + r.pick(['died poor', 'died very rich', 'was never seen again', 'sold the claim for a horse']) + '.', pr2, { era: ei, thread: 'discovery-' + ei, told: what + ' was found' + (road ? '' : ' near ' + dp2) });
      if (pivot && kindOn('foundings')) { var town = placeFor('founding', r); ev('founding', y0 + r.int(2, 12), town + ' founded', 'A camp that became a town, raised by the people who came for the ' + (road ? 'road' : what) + '.', pr2, { era: ei, cause: pivot, thread: 'discovery-' + ei }); }
    } else if (ch === 'faith' && kindOn('schisms')) {
      pivot = ev('schism', y0, 'The temple splits', 'The priests quarrel over ' + r.pick(['the calendar itself', 'who may read the old books', 'the price of pardons', 'a prophet from ' + placeFor('prophet', r)]) + ', and each side burns the other’s books.', pr2, { era: ei, thread: 'faith-' + ei, told: 'the temple split' });
    } else if (ch === 'decline' && kindOn('coronations')) {
      var old = ruler(r);
      pivot = ev('death', y0, old + ' dies without an heir', 'Three cousins, four claims and no will. The lords start counting their spears.', pr2, { era: ei, thread: 'dynasty-' + ei, told: old + ' died without an heir' });
    }
    // A portent remembered before the turning point: a comet or a dark sun a year or two earlier.
    if (pivot && ei > 0 && kindOn('portents') && R('portent', ei).chance(0.45 * freqMult(mk.portents))) {
      var py = pivot.year - R('portent-y', ei).int(1, 2), pk = R('portent-k', ei).chance(0.5), gap = pivot.year - py;
      ev('portent', py, pk ? 'A comet over ' + home + ' for forty nights' + (names['A comet over ' + home + ' for forty nights'] ? ' again' : '') : 'The sun goes dark at noon' + (names['The sun goes dark at noon'] ? ' again' : ''),
        (pk ? 'Afterwards everyone said the comet had foretold it: ' : 'Remembered afterwards as the first sign: ') + (gap === 1 ? 'the next year ' : numWord(gap) + ' years later ') + (pivot.gen.told || lowerFirst(pivot.name)) + '.', R('portent-d', ei), { era: ei, thread: pivot.gen.thread, theta: 'portent', cause: null });
    }
    // Threads inside the era, never the same story twice.
    var nInner = Math.max(0, Math.round(dt.density * len / 100 * (0.7 + 0.6 * r.next())) - (pivot ? 1 : 0));
    for (var k = 0; k < nInner; k++) {
      var tr = R('thread', ei, k), yy = y0 + tr.int(Math.min(3, len - 1), Math.max(3, len - 1));
      var pool = { foundings: 2, wars: ch === 'strife' ? 4 : 1, plagues: ch === 'dark' ? 2 : 0.5, famines: ch === 'dark' ? 1.5 : 0.4, discoveries: ch === 'discovery' ? 3 : 1, coronations: 1.5, disasters: 0.7, migrations: 0.8, schisms: ch === 'faith' ? 3 : 0.6 };
      var kind = tr.weighted(ownKeys(pool).filter(kindOn), function (x) { return pool[x] * freqMult(mk[x]) * (dt.tone === 'grim' && (x === 'wars' || x === 'plagues' || x === 'famines' || x === 'disasters') ? 1.4 : dt.tone === 'bright' && (x === 'foundings' || x === 'discoveries') ? 1.4 : 1); });
      if (!kind) break;
      var th = 'inner-' + ei + '-' + k;
      innerThread(kind, yy, tr, th, ei);
    }
    var hue = ERA_HUE[ch] + r.int(-12, 12);
    eras.push({ name: eraName, start_year: y0, end_year: y1, description: eraDescription(ch, pivot, r, realm, places), color: oklch(r.range(0.55, 0.62), r.range(0.08, 0.13), hue), sort_order: ei, gen: { key: makeKey('era', seed, ei), character: ch, locked: false, cause: pivot ? pivot.gen.key : null } });
  });

  function innerThread(kind, yy, tr, th, ei) {
    var o = { era: ei, thread: th };
    if (kind === 'foundings') {
      var p = placeFor('charter', tr), fl = [
        [p + ' granted its charter', ['After forty years of asking and one good harvest.', 'Bought, not granted: the price was a new wing on the old king’s hunting lodge.', 'The charter is still read aloud every spring, including the parts nobody understands.']],
        ['The ' + tr.pick(['weavers’', 'masons’', 'salt-carters’', 'lamp-makers’', 'brewers’', 'tanners’', 'coopers’', 'rope-makers’', 'bell-founders’']) + ' guild founded', ['Seven masters, one seal and a feast that ran for three days.', 'Its first act is to fine a man for working by candlelight.', 'Its hall is the first building in the town with glass in every window.']],
        ['The bridge at ' + placeFor('bridge', tr, null, function (x) { return !/bridge|ford/i.test(x); }) + ' built', ['Paid for by a merchant who wanted his name on it; the name wore off.', 'Nine arches of grey stone; the toll pays for it twice over within the century.', 'The ferryman’s family never forgave it, and still haven’t.']],
        ['The great market hall at ' + p + ' opened', ['The first stone is laid on a feast day; the last, eleven years later.', 'It is built over the old bear-pit, and the cellars are said to be unquiet.', 'Its bell sets the price of grain for three valleys.']],
        ['A school of letters opened at ' + p, ['Twelve pupils, one master and a library of forty books, half of them borrowed.', 'Founded by a widow who could not read and meant her daughters to.', 'The master is a runaway monk; the town decides not to ask.']],
        ['The walls of ' + p + ' raised', ['A ditch, then a bank, then a palisade, and at last stone; the town stops growing past them for a century.', 'Every household owes a cartload of stone a year until they are done; it takes a generation.', 'Paid for by a toll on every cart through the gate, which is still collected.']],
        ['A mill raised on the ' + p + ' brook', ['The miller takes one sack in ten, and the lord takes the miller’s.', 'The first mill in the valley; the old hand-querns are broken up to pave its yard.', 'Built by the abbey; the villagers now pay to grind what they used to grind for nothing.']]];
      var fi = variantIdx('inner-found', tr, fl.length);
      ev('founding', yy, fl[fi][0], tell('inner-found-told-' + fi, tr, fl[fi][1]), tr, o);
    } else if (kind === 'wars') {
      var q = placeFor('rising', tr), w = variant('inner-war', tr, [
        ['rebellion', 'The ' + q + ' rising', 'The lords of ' + q + ' refuse the levy and bar their gates.', [['Pardons for the ' + q + ' lords', 'The ringleaders are pardoned in public and poisoned in private.'], ['The gates of ' + q + ' opened', 'By a bribe, not a battering ram; the lords keep their lands and lose their sons as hostages.']]],
        ['raid', 'Raiders burn ' + q, 'They come by night, take the cattle and the bell, and are gone by dawn.', [['The ' + q + ' bell found', 'It turns up in a foreign church; nobody asks for it back.'], ['The raiders\u2019 captain taken', 'Caught at a fair two years later, selling the calves of the stolen cattle.']]],
        ['rebellion', 'The peasants of ' + q + ' rise', 'Over a new tax on salt; they march on the castle with scythes and a list of grievances.', [['The salt tax dropped', 'The list is read, the tax is dropped, and the leaders are hanged anyway.'], ['The lord of ' + q + ' flees', 'His cousin keeps the castle, and the tax.']]],
        ['battle', 'The fight at ' + q + ' ford', 'Two lords\u2019 retinues meet at a ford and neither will give way.', [['The ' + q + ' marriage', 'The feud is settled by a wedding between the two houses the next spring.'], ['The ' + q + ' feud', 'Still going three generations later, over a ford that has since silted up.']]],
        ['siege', 'The outlaw keep at ' + q, 'A disinherited knight holds the old keep and taxes the road.', [['The keep at ' + q + ' pulled down', 'Stone by stone, by the villagers he had robbed, who were paid in its stone.']]],
        ['raid', 'Pirates in the ' + q + ' estuary', 'Three ships, black sails, and a summer of burned landings.', [['The pirates hanged at ' + q, 'Eleven gibbets on the headland, left up for a year as a warning.']]],
        ['rebellion', 'The ' + q + ' mutiny', 'Unpaid soldiers seize the ' + q + ' garrison and elect their own captain.', [['The mutineers paid off', 'Every man gets his back pay and a pardon; the paymaster is quietly hanged.']]]]);
      var st = ev(w[0], yy, w[1], w[2], tr, o);
      if (st) { var af = tr.pick(w[3]); ev('treaty', yy + tr.int(1, 3), af[0], af[1], tr, { era: ei, thread: th, cause: st, secret: tr.chance(0.25) }); }
    } else if (kind === 'plagues') {
      var pl = placeFor('fever', tr), vp = variant('inner-plague', tr, [
        ['Fever at ' + pl, 'A fever in the lower town of ' + pl + '; the gates are shut for a summer.'],
        ['The sweating summer at ' + pl, 'The sick are carried to the island in the river and fed by boat.'],
        ['Murrain in the ' + pl + ' herds', 'Half the herds die; the price of wool and meat doubles for years.'],
        ['The spotted sickness at ' + pl, 'Children mostly; the town builds a new chapel with the money it saved on schooling.'],
        ['The coughing winter at ' + pl, 'It takes the old and the very young; the churchyard is extended twice before spring.'],
        ['A pox brought to ' + pl + ' by traders', 'The market is shut for a season and the traders’ lodgings are burned to the ground.']]);
      ev('plague', yy, vp[0], vp[1], tr, o);
    } else if (kind === 'famines') {
      var vf = variant('inner-famine', tr, [
        ['The year of the late frost', 'Late frosts kill the blossom and the grain; the price of bread trebles.'],
        ['The blight in the rye', 'The rye turns black in the ear; those who eat it dance until they die.'],
        ['The turnip winter', 'Nothing grows but turnips, and the songs about turnips date from then.'],
        ['The locust summer', 'They come from the south in a cloud that darkens the noon, and leave nothing green.'],
        ['The wet harvest', 'It rains from midsummer to the first frost, and the grain sprouts in the stook.'],
        ['The year of the dead bees', 'No honey, no wax and no fruit; the orchards stand bare all autumn.']]);
      ev('famine', yy, vf[0], vf[1], tr, o);
    } else if (kind === 'discoveries') {
      var dq = placeFor('find', tr), dl = [
        ['A barrow opened near ' + dq, ['What was found inside is in the temple treasury; what was taken out at night is not.', 'Inside: a warrior, a horse, and a bronze mirror nobody will look into.', 'A queen of the old people, a gold torc and a dog at her feet; the torc is gone by the end of the week.']],
        ['The ' + dq + ' road cut through', ['Trade doubles in a decade; so do the robberies.', 'Forty miles of stone road; the old drove-way over the hills is left to the sheep.', 'It follows an older road nobody remembered, straight as a spear through the forest.']],
        ['A hoard ploughed up at ' + dq, ['Old coins with a king nobody can name.', 'Silver arm-rings and a gold cup; the ploughman’s grandchildren are still spending it.']],
        ['A spring found at ' + dq, ['Hot, salty and said to cure the gout; an inn is built before the priests can bless it.', 'Sweet water in a dry country; the village moves half a mile to be near it.']],
        ['Salt found at ' + dq, ['A brine spring, boiled down in lead pans; the town grows rich and everyone’s food is too salty.', 'The salt road brings merchants from four kingdoms, and a garrison to watch them.']],
        ['The ' + dq + ' caves charted', ['Three days underground; they come back with maps, crystals and one man fewer.', 'Painted halls deep in the hill, older than anyone’s gods; the priests have the entrance walled up.']]];
      var di = variantIdx('inner-disc', tr, dl.length);
      ev('discovery', yy, dl[di][0], tell('inner-disc-told-' + di, tr, dl[di][1]), tr, o);
    } else if (kind === 'coronations') {
      var rn2 = ruler(tr);
      var cr = ev('coronation', yy, rn2 + ' crowned', variant('inner-crown', tr, ['A short reign is expected; it lasts ' + numWord(tr.int(20, 40)) + ' years.', 'Crowned young, with an uncle holding the reins for a decade.', 'The crowning feast lasts nine days and bankrupts two lords.', 'Crowned in a borrowed cathedral, with a borrowed crown.', 'Crowned at dawn, on horseback, because the cathedral roof had fallen in.', 'Crowned by acclaim of the people, or so the chronicle says; the chronicler was paid by the crown.', 'Crowned in the rain, in a hurry, before the other claimant could arrive.', 'The regalia turn out to be paste; the real stones paid for the last war.']), tr, { era: ei, thread: th, theta: 'coronation' });
      if (cr && tr.chance(0.35)) ev('death', yy + tr.int(5, 30), rn2 + ' dies', variant('inner-death', tr, ['A fall from a horse; nobody saw the horse.', 'Of a fever, in bed, with every claimant in the next room.', 'Poison, say the songs. Old age, says the court.', 'Drowned crossing the river in spate, against advice.', 'In battle, at the head of the levy, which then went home.', 'Choked on a fishbone at the midwinter feast.']), tr, { era: ei, thread: th, cause: cr, secret: tr.chance(0.2) });
    } else if (kind === 'disasters') {
      var dd = placeFor('disaster', tr), v3 = variant('inner-dis', tr, [['flood', 'The river takes the bridge at ' + dd, 'Rebuilt in a year, badly.'], ['fire', 'Fire at ' + dd, 'Half the market goes up; the guildhall is saved by bucket-chain and luck.'], ['quake', 'The hill slips at ' + dd, 'A whole street slides into the valley one wet night; the church is left standing on its own.'], ['flood', 'The sea breaks the dyke at ' + dd, 'The salt spoils the fields for seven years.'], ['fire', 'The great fire of ' + dd, 'Three days burning; the town is rebuilt in stone, and a new law forbids thatch.'], ['flood', 'The ' + dd + ' flood', 'The river rises in a single night and takes the lower town; marks on the church wall show how high.']]);
      ev(v3[0], yy, v3[1], v3[2], tr, o);
    } else if (kind === 'migrations') {
      var mg = placeFor('settle', tr), vm = variant('inner-mig', tr, [
        ['Settlers come to ' + mg, 'Families from over the ' + tr.pick(['mountains', 'sea', 'border', 'marshes']) + ' take up the empty land around ' + mg + '; their names are still different.'],
        ['Refugees at ' + mg, 'They came with nothing but their bells and their saints, and stayed.'],
        ['The ' + mg + ' clearances', 'The old tenants are turned off for sheep; they go to the towns, and the towns grow.'],
        ['The fen-folk move to ' + mg, 'Driven out by the draining of the marshes, they bring eels, reed-craft and a grudge.'],
        ['Miners come to ' + mg, 'They speak their own tongue underground and ours above it, and marry into every family within ten years.']]);
      ev('migration', yy, vm[0], vm[1], tr, o);
    } else if (kind === 'schisms') {
      var sc = placeFor('faith', tr), v4 = variant('inner-faith', tr, [['A saint’s bones found at ' + sc, 'Pilgrims come for a century, and the town grows rich on them.'], ['The temple at ' + sc + ' consecrated', 'The stone came from a ruin nobody else would touch.'], ['A heretic burned at ' + sc, 'His followers keep his name alive in a song that is still banned.'], ['The ' + sc + ' miracle', 'A statue weeps for three days; a canon is sent to investigate and stays for good.'], ['The ' + sc + ' schism', 'Two priests, one altar and a quarrel over the feast days that splits the town for sixty years.'], ['A hermit on the pillar at ' + sc, 'He lives on top of it for twenty years; pilgrims leave bread and questions at its foot.']]);
      ev('schism', yy, v4[0], v4[1], tr, o);
    }
  }
  // Locked eras stay; generated eras that overlap them give way.
  if (lockedEras.length) {
    var keptEras = lockedEras.map(function (e) { var c = clone(e); c.gen = Object.assign({}, c.gen || {}, { locked: true }); return c; });
    eras = eras.filter(function (e) { return !keptEras.some(function (k) { var ke = k.end_year == null ? now : k.end_year, ee = e.end_year == null ? now : e.end_year; return e.start_year <= ke && ee >= k.start_year; }); }).concat(keptEras);
    eras.sort(function (a, b) { return a.start_year - b.start_year; });
    eras.forEach(function (e, i) { e.sort_order = i; });
  }
  events.sort(function (a, b) { return cal.absOf(a) - cal.absOf(b); });
  var summary = historySummary(cal, eras, events, realm, startYear, now, recipe);
  return { generator: 'history', seed: seed, recipe: recipe, scope: { label: 'the years ' + startYear + ' to ' + now, days: 0 }, eras: eras, events: events, locked: lockedEvents, realm: realm, summary: summary, stats: countBy(events, function (e) { return e.gen.kind; }), warnings: warnings };
}

function eraDescription(ch, pivot, r, realm, places) {
  var p = places[0], opener = pivot ? cap(pivot.gen.told || lowerFirst(pivot.name)) + ' in ' + pivot.year + '. ' : '';
  var body = {
    founding: r.pick(['Settlers clear the land around ' + p + '; the first roads are cut and the first quarrels begin.', 'Hedge-lords and river towns, each with its own toll and its own saint.']),
    golden: r.pick(['Markets, bridges and full barns. The songs people still sing were written now.', 'A long peace under one crown; the roads are safe enough for pedlars and gossip.']),
    strife: r.pick(['Decades of war, fought for crowns and tolls, and paid for by the villages.', 'Every lord a king in his own valley, and every valley at war with the next.']),
    dark: r.pick(['Sickness and hunger; half the villages empty and the woods creep back.', 'Hard years: the bells ring more for funerals than for feasts.']),
    restoration: r.pick(['The realm is put back together, road by road and marriage by marriage.', 'The old roads reopen and the abandoned fields go under the plough again.']),
    faith: r.pick(['The temples grow rich and quarrelsome, and the pilgrims’ roads grow busy.', 'An age of saints, relics and heresies, and of the lawyers who argued about them.']),
    discovery: r.pick(['Roads, charts and seams of ore: the hills are opened up and the towns grow.', 'Fortunes made overnight and lost as fast; new towns where there were none.']),
    decline: r.pick(['Slow years: the crown weakens and the lords stop listening.', 'The treasury empties, the roads go unmended, and nobody is quite in charge.']),
    present: r.pick(['The realm is quiet enough for trade and gossip, which is to say not very quiet at all.', 'Years are counted from then. It has held, mostly.'])
  }[ch] || '';
  return (opener + body).trim();
}
function historySummary(cal, eras, events, realm, from, now, recipe) {
  var span = now - from, list = eras.map(function (e) { return e.name + ' (' + e.start_year + (e.end_year == null ? ' on' : '–' + e.end_year) + ')'; });
  var turning = eras.filter(function (e) { return e.gen && e.gen.cause; }).slice(1, 4).map(function (e) {
    var c = events.filter(function (x) { return x.gen.key === e.gen.cause; })[0];
    return c ? (c.gen.told || lowerFirst(c.name)) + ' in ' + c.year + ', which began ' + e.name : null;
  }).filter(Boolean);
  var linked = events.filter(function (e) { return e.gen.causedBy; }).length, secrets = events.filter(function (e) { return e.visibility === 'dm_only'; }).length;
  var s = cap(numWord(span) === String(span) ? span + ' years' : numWord(span) + ' years') + ' of history for ' + realm + ', from ' + from + ' to ' + now + ', in ' + count(eras.length, 'era') + ': ' + list.join(', ') + '.';
  if (turning.length) s += ' Turning points: ' + turning.join('; ') + '.';
  s += ' ' + cap(count(events.length, 'event')) + ' in all, ' + linked + ' of them following from an earlier one (each war has its battles and its treaty, each plague its aftermath), all on real dates of this calendar.';
  if (secrets) s += ' ' + cap(count(secrets, 'is hidden history', 'are hidden history')) + ' only you can see.';
  var off = ownKeys(recipe.makes).filter(function (k) { return recipe.makes[k] === 'off'; }).map(function (k) { return HISTORY_KINDS[k].label.toLowerCase(); });
  if (off.length) s += ' No ' + joinList(off) + ', as asked.';
  return s;
}

/* ── Whole calendars ────────────────────────────────────────────────────────────────────────────────
   Output is Chronicle's own preset/export shape ({format: 'chronicle-calendar-v1', version: 2, calendar})
   with exactly the export's fields, so it goes through the same importer a preset does (presets.go: "a
   preset IS an export"). What the preset shape cannot hold (moon surfaces, season types) comes back in
   `extras`. Leap rules are what Chronicle can express: every N years from an offset, adding days to
   chosen months. */

var CAL_TEMPLATES = {
  'gregorian-like': { label: 'Like our own', phrase: 'a calendar much like our own', blurb: 'Twelve months of 28 to 31 days, seven-day weeks, a leap day every four years.' },
  'harptos-like': { label: 'Festival days', phrase: 'a calendar of thirty-day months and festival days', blurb: 'Twelve thirty-day months, ten-day weeks, festival days between the months.' },
  even: { label: 'Even months', phrase: 'an even-month calendar', blurb: 'Months all the same length, with a day or two left over for the year’s turn.' },
  lunar: { label: 'By the moon', phrase: 'a moon-reckoned calendar', blurb: 'Months of 29 and 30 days that follow the moon, and a leap month every few years.' },
  wild: { label: 'Anything goes', phrase: 'an unusual calendar', blurb: 'Uneven months, an odd week, festival days wherever they fall.' }
};
var CAL_YEAR = { pastoral: [1100, 1600], nautical: [300, 700], dwarven: [3000, 5000], elven: [200, 900], desert: [800, 1500], imperial: [600, 1300], fey: [60, 400], grim: [900, 1400] };
var SEASON_COLOURS = { winter: 'oklch(0.65 0.05 240)', spring: 'oklch(0.65 0.10 140)', summer: 'oklch(0.72 0.12 85)', autumn: 'oklch(0.58 0.12 40)', wet: 'oklch(0.60 0.10 205)', dry: 'oklch(0.70 0.10 80)' };
var EXPORT_KEYS = {
  calendar: 'name description mode epoch_name current_year current_month current_day current_hour current_minute hours_per_day minutes_per_hour seconds_per_minute leap_year_every leap_year_offset tracks_real_time real_time_zone months weekdays moons seasons eras cycles festivals weather',
  month: 'name days sort_order is_intercalary leap_year_days', weekday: 'name sort_order is_rest_day', moon: 'name cycle_days phase_offset color',
  season: 'name start_month start_day end_month end_day description color weather_effect', era: 'name start_year end_year description color sort_order',
  festival: 'name month day after_month description color icon sort_order'
};

defineGenerator({
  id: 'calendar', label: 'Calendar', needsCalendar: false,
  blurb: 'A whole calendar from a template, a culture and a dash of chance: months, weeks, festival days, leap years, seasons, moons and an era.',
  makes: {
    'festival-days': { label: 'Festival days between months', type: 'freq', default: 'normal' },
    leap: { label: 'Leap years', type: 'bool', default: true },
    moons: { label: 'Moons', type: 'count', default: 1, min: 0, max: 5 },
    seasons: { label: 'Seasons', type: 'count', default: 4, min: 0, max: 6 },
    festivals: { label: 'Named festivals on dates', type: 'bool', default: true, help: 'The solstices and equinoxes, as the export’s festival list.' }
  },
  scope: {},
  details: {
    template: { label: 'Template', type: 'select', default: 'gregorian-like', options: ownKeys(CAL_TEMPLATES).map(function (k) { return { value: k, label: CAL_TEMPLATES[k].label }; }) },
    theme: { label: 'Culture', type: 'select', default: 'pastoral', options: Object.keys(THEMES).map(function (k) { return { value: k, label: THEMES[k].label }; }) },
    randomness: { label: 'How far from the template', type: 'range', default: 0.3, min: 0, max: 1, step: 0.05 },
    style: { label: 'Kind of words', type: 'select', default: 'mixed', more: true, options: [{ value: 'mixed', label: 'The culture’s own habit' }, { value: 'words', label: 'Plain-English compounds' }, { value: 'tongue', label: 'Invented words' }] },
    months: { label: 'Months (0 = template)', type: 'count', default: 0, min: 0, max: 24, more: true },
    weekLength: { label: 'Days in a week (0 = template)', type: 'count', default: 0, min: 0, max: 14, more: true },
    currentYear: { label: 'Current year (0 = choose)', type: 'count', default: 0, min: 0, max: 100000, more: true },
    name: { label: 'Calendar name (empty = choose)', type: 'text', default: '', max: 80, more: true }
  },
  builtIns: [
    { id: 'calendar.gregorian-like', name: 'Familiar', description: 'Close to our own calendar, renamed for your world.', details: { template: 'gregorian-like', randomness: 0.15 } },
    { id: 'calendar.harptos-like', name: 'Tendays and feast days', description: 'Thirty-day months, ten-day weeks and festival days between the months.', details: { template: 'harptos-like', randomness: 0.2 } },
    { id: 'calendar.dwarven', name: 'Deep-count', description: 'Even months for people who live underground; a long vigil at the year’s end.', details: { template: 'even', theme: 'dwarven', randomness: 0.3 }, makes: { moons: 0, seasons: 2 } },
    { id: 'calendar.lunar', name: 'Moon-reckoned', description: 'Months that follow the moon, with a leap month every third year.', details: { template: 'lunar', randomness: 0.2 } },
    { id: 'calendar.wild', name: 'Strange', description: 'Uneven months and an unusual week, for a world that is not ours.', details: { template: 'wild', randomness: 0.7, theme: 'fey' }, makes: { moons: 2, seasons: 3 } }
  ],
  defaultRecipe: 'calendar.gregorian-like',
  run: runCalendarGen
});

/* Where a season starts, as a share of the year, for n seasons of the given types. */
function seasonPlan(n, types) {
  var starts = { winter: 0.88, spring: 0.21, summer: 0.46, autumn: 0.71, wet: 0.38, dry: 0.88 };
  if (n === 2) return [{ type: types[0] || 'wet', at: 0.38 }, { type: types[1] || 'dry', at: 0.88 }];
  if (n === 3) return [{ type: 'winter', at: 0.85 }, { type: 'spring', at: 0.2 }, { type: 'summer', at: 0.5 }];
  var list = SEASON_TYPES.map(function (t) { return { type: t, at: starts[t] }; });
  if (n === 5) list.push({ type: null, at: 0.08, name: 'Lengthening' });
  if (n === 6) list.push({ type: null, at: 0.08 }, { type: null, at: 0.58 });
  return list.slice(0, Math.max(n, 4)).sort(function (a, b) { return a.at - b.at; });
}

function runCalendarGen(opts) {
  var recipe = resolveRecipe('calendar', opts.recipe), mk = recipe.makes, dt = recipe.details, seed = seedOf(opts), td = themeData(dt.theme);
  var tpl = dt.template, rnd = dt.randomness, R = function (k) { return makeRng(seed, 'calendar', tpl, dt.theme, k); };
  var r = R('shape'), months = [], week = 7, rest = [], leapEvery = 0, leapOffset = 0, leapMonth = -1;
  var fdMult = freqMult(mk['festival-days']);
  function feastCount(base) { return fdMult === 0 ? 0 : Math.max(0, Math.round(base * (fdMult >= 2 ? 1.6 : fdMult < 1 ? 0.4 : 1) + (r.chance(rnd) ? r.int(-1, 1) : 0))); }
  if (tpl === 'gregorian-like') {
    var gl = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
    if (rnd > 0.2) for (var i = 0; i < 12; i++) if (r.chance(rnd * 0.5)) { var j = r.int(0, 11); var dd = r.int(1, 2); if (gl[i] - dd >= 27 && gl[j] + dd <= 32) { gl[i] -= dd; gl[j] += dd; } }
    months = gl.map(function (d) { return { days: d }; });
    week = rnd > 0.6 && r.chance(0.5) ? r.pick([6, 8]) : 7; rest = [week - 1];
    leapEvery = 4; leapMonth = 1;
    var nf = feastCount(0.5);
    for (var f = 0; f < nf; f++) { var at = r.int(1, months.length - 1); months.splice(at, 0, { days: 1, intercalary: true }); }
  } else if (tpl === 'harptos-like') {
    months = [];
    for (i = 0; i < 12; i++) months.push({ days: 30 });
    var slots = [1, 4, 7, 9, 11], nFeast = Math.min(5, feastCount(5));
    slots = r.shuffle(slots).slice(0, nFeast).sort(function (a, b) { return b - a; });
    slots.forEach(function (s) { months.splice(s, 0, { days: 1, intercalary: true, midsummer: s === 7 }); });
    week = rnd > 0.5 && r.chance(0.5) ? r.pick([8, 9]) : 10; rest = rnd > 0.3 && r.chance(0.5) ? [week - 1] : [];
    leapEvery = 4;
    leapMonth = months.map(function (m) { return !!m.midsummer; }).indexOf(true);
    if (leapMonth < 0) leapMonth = months.map(function (m) { return !!m.intercalary; }).lastIndexOf(true);
  } else if (tpl === 'even') {
    var shapes = [[10, 36, 5], [13, 28, 1], [9, 40, 5], [8, 45, 5], [12, 30, 5], [10, 36, 1]], sh = r.pick(shapes);
    for (i = 0; i < sh[0]; i++) months.push({ days: sh[1] });
    var extra = sh[2], nfe = Math.min(extra, Math.max(1, feastCount(1)));
    if (nfe) { var per = Math.floor(extra / nfe); for (f = 0; f < nfe; f++) months.splice(Math.round((f + 1) * months.length / nfe), 0, { days: per + (f === nfe - 1 ? extra - per * nfe : 0), intercalary: true }); }
    week = r.pick([5, 6, 6, 7, 8, 9]); rest = week >= 7 ? [week - 1] : [];
    leapEvery = r.chance(0.5) ? r.int(4, 8) : 0; leapMonth = months.length - 1;
  } else if (tpl === 'lunar') {
    for (i = 0; i < 12; i++) months.push({ days: i % 2 === 0 ? 30 : 29 });
    months.push({ days: 0, leapOnly: 30 });
    week = r.pick([7, 7, 8]); rest = [week - 1];
    leapEvery = 3; leapMonth = months.length - 1;
  } else {
    var nm = dt.months || r.int(6, 15), target = r.int(300, 420);
    for (i = 0; i < nm; i++) months.push({ days: 0 });
    var left = target;
    months.forEach(function (m, k) { m.days = k === nm - 1 ? clamp(left, 18, 48) : clamp(Math.round(target / nm * r.range(0.6, 1.4)), 18, 48); left -= m.days; });
    var nw = feastCount(2);
    for (f = 0; f < nw; f++) months.splice(r.int(1, months.length - 1), 0, { days: r.int(1, 5), intercalary: true });
    week = r.int(4, 12); rest = week >= 6 ? [week - 1] : [];
    if (week >= 9 && r.chance(0.5)) rest.push(Math.floor(week / 2) - 1);
    leapEvery = r.chance(0.6) ? r.int(2, 8) : 0; leapMonth = r.int(0, months.length - 1);
  }
  // Owner overrides: a fixed number of regular months or days in the week.
  if (dt.months && tpl !== 'wild') {
    var regular = months.filter(function (m) { return !m.intercalary && !m.leapOnly; });
    var avg = Math.round(sum(regular.map(function (m) { return m.days; })) / regular.length);
    while (regular.length > dt.months) { var drop = regular.pop(); months.splice(months.indexOf(drop), 1); }
    while (regular.length < dt.months) { var add = { days: avg }; months.push(add); regular.push(add); }
  }
  if (dt.weekLength) { week = dt.weekLength; rest = week >= 6 ? [week - 1] : []; }
  if (!mk.leap) leapEvery = 0;
  if (leapEvery && r.chance(rnd)) leapOffset = r.int(0, leapEvery - 1);
  leapMonth = clamp(leapMonth, 0, months.length - 1);

  // A draft calendar, to find each month's season before naming it.
  var draft = { name: 'draft', months: months.map(function (m, k) { return { name: 'M' + k, days: m.leapOnly ? 0 : m.days, sort_order: k, is_intercalary: !!m.intercalary, leap_year_days: (m.leapOnly || 0) + (leapEvery && k === leapMonth && !m.leapOnly ? 1 : 0) }; }), weekdays: [] };
  var nSeasons = mk.seasons;
  var seasonTypes = nSeasons === 2 ? (dt.theme === 'desert' || tpl === 'wild' && r.chance(0.3) ? ['wet', 'dry'] : ['summer', 'winter']) : null;
  var plan = nSeasons ? seasonPlan(nSeasons, seasonTypes || []) : [];
  var dc = makeCal(draft), L = dc.refYearLength, ry = 1;
  while (dc.isLeap(ry)) ry++;
  function dateAtShare(share) {
    var d = dc.fromAbs(dc.abs(ry, 1, 1) + Math.floor(mod(share, 1) * L));
    var n = 0;
    while (dc.isIntercalary(d.month) && n++ < 10) d = dc.fromAbs(dc.absOf(d) + 1);
    return d;
  }
  var seasonsOut = [];
  var sr = R('seasons'), used = {};
  plan.forEach(function (p, k) {
    var s0 = dateAtShare(p.at), nxt = plan[(k + 1) % plan.length], e0 = dc.fromAbs(dc.absOf(dateAtShare(nxt.at)) - 1);
    while (dc.isLeapOnly(ry, e0.month, e0.day)) e0 = dc.fromAbs(dc.absOf(e0) - 1);
    var type = p.type, nmS = type ? nameSeason(td, sr, type, used) : take(used, p.name && isFree(used, p.name) ? p.name : nameSeason(td, sr, k < plan.length / 2 ? 'spring' : 'autumn', used));
    seasonsOut.push({ name: nmS, start_month: s0.month, start_day: s0.day, end_month: e0.month, end_day: e0.day, color: type ? SEASON_COLOURS[type] : 'oklch(0.62 0.08 ' + (k * 60 % 360) + ')', _type: type });
  });
  var seasonTypeMap = {};
  seasonsOut.forEach(function (s) { seasonTypeMap[s.name] = s._type; });
  var named = makeCal({ name: 'draft', months: draft.months, weekdays: [], seasons: seasonsOut }, { seasonTypes: seasonTypeMap });
  var types = monthSeasonTypes(named, months.length);
  var monthNames = nameMonths(td, R('months'), types, months.map(function (m) { return !!m.intercalary; }), dt.style, used, null);
  months.forEach(function (m, k) { if (m.leapOnly) monthNames[k] = isFree(used, 'The Leap Month') ? take(used, 'The Leap Month') : monthNames[k]; });
  var wdNames = nameWeekdays(td, R('weekdays'), week, rest, dt.style, used, null);
  // Moons: the lunar template's moon keeps time with its months.
  var nMoons = mk.moons;
  var moonRes = nMoons ? runMoons({ recipe: { generator: 'moons', makes: { count: nMoons, strange: tpl === 'wild' ? 'normal' : 'rare' }, details: { theme: dt.theme, cycles: tpl === 'wild' ? 'varied' : 'natural' } }, seed: seed + '|calendar-moons' }) : { moons: [] };
  var year = dt.currentYear || (function () { var yr = CAL_YEAR[dt.theme] || [1000, 1500]; return R('year').int(yr[0], yr[1]); })();
  if (tpl === 'lunar' && moonRes.moons.length) {
    var mm = moonRes.moons[0];
    mm.cycle_days = 29.5;
    var a0 = named.abs(year, 1, 1);
    mm.phase_offset = round2(mod(-a0, 29.5)); // the first of the year is a new moon in the current year
  }
  var er = R('era'), eraName = nameEra(td, er, 'present', used);
  // The epoch is the era's initials ("Reckoning of Wards" is RW), as the shipped presets write it.
  var epoch = eraName.split(/\s+/).filter(function (w) { return /^[A-Z]/.test(w) && w !== 'The'; }).map(function (w) { return w.charAt(0); }).join('');
  if (epoch.length < 2) epoch = eraName.replace(/^The\s+/, '').split(/[\s-]+/)[0].slice(0, 4);
  var place = namePlace(td, R('name'), used);
  var calName = dt.name || R('calname').pick([place + ' Reckoning', 'The ' + place + ' Count', 'Calendar of ' + place, place + ' Almanac']);
  var cal = {
    name: calName, mode: 'fantasy', epoch_name: epoch,
    current_year: year, current_month: 1, current_day: 1, current_hour: 0, current_minute: 0,
    hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60,
    leap_year_every: leapEvery, leap_year_offset: leapOffset, tracks_real_time: false,
    months: months.map(function (m, k) { return { name: monthNames[k], days: m.leapOnly ? 0 : m.days, sort_order: k, is_intercalary: !!m.intercalary, leap_year_days: (m.leapOnly || 0) + (leapEvery && k === leapMonth && !m.leapOnly ? 1 : 0) }; }),
    weekdays: wdNames.map(function (n, k) { return { name: n, sort_order: k, is_rest_day: rest.indexOf(k) >= 0 }; }),
    moons: moonRes.moons.map(function (m) { return { name: m.name, cycle_days: m.cycle_days, phase_offset: m.phase_offset, color: m.color }; }),
    seasons: seasonsOut.map(function (s) { return { name: s.name, start_month: s.start_month, start_day: s.start_day, end_month: s.end_month, end_day: s.end_day, color: s.color }; }),
    eras: [{ name: eraName, start_year: 1, color: oklch(0.56, 0.08, er.int(0, 359)), sort_order: 0, description: epoch }]
  };
  if (mk.festivals && cal.seasons.length) {
    var fcal = makeCal(cal, { seasonTypes: seasonTypeMap }), an = fcal.anchorDays(ry), fr = R('festivals'), fests = [];
    [['winterSolstice', an.winterSolstice], ['springEquinox', an.springEquinox], ['summerSolstice', an.summerSolstice], ['autumnEquinox', an.autumnEquinox]].forEach(function (x) {
      var d = x[1];
      if (!d || fcal.isIntercalary(d.month) || fcal.isLeapOnly(ry, d.month, d.day)) return;
      if (fests.some(function (f) { return f.month === d.month && f.day === d.day; })) return;
      fests.push({ name: nameFestival(td, fr, x[0], { moon: cal.moons[0] ? cal.moons[0].name : 'the moon' }, used), month: d.month, day: d.day, description: cap(ANCHOR_WORDS[x[0]]) + '.', sort_order: fests.length });
    });
    if (fests.length) cal.festivals = fests;
  }
  var preset = { format: 'chronicle-calendar-v1', version: 2, calendar: cal };
  var check = validatePreset(preset);
  var reg = cal.months.filter(function (m) { return !m.is_intercalary && m.days > 0; }), feasts = cal.months.filter(function (m) { return m.is_intercalary; });
  var yl = sum(cal.months.map(function (m) { return m.days; }));
  var summary = calName + ', ' + CAL_TEMPLATES[tpl].phrase + ' in ' + withArticle(THEMES[dt.theme].label.toLowerCase()) + ' voice: ' + count(reg.length, 'month') + ' (' + monthLengths(reg) + ')' +
    (feasts.length ? ' and ' + count(feasts.length, 'festival day') + ' between them (' + joinList(feasts.map(function (m) { return m.name; })) + ')' : '') + ', ' + yl + ' days in a common year. Weeks of ' + numWord(week) + ' days' +
    (rest.length ? ' with ' + joinList(rest.map(function (k) { return cal.weekdays[k].name; })) + ' to rest' : '') + '. ' +
    (leapEvery ? 'Every ' + ordWord(leapEvery) + ' year is a leap year, adding ' + (months[leapMonth].leapOnly ? 'the whole of ' + cal.months[leapMonth].name : 'a day to ' + cal.months[leapMonth].name) + '. ' : 'No leap years. ') +
    (cal.seasons.length ? cap(count(cal.seasons.length, 'season')) + ': ' + joinList(cal.seasons.map(function (s) { return s.name; })) + '. ' : '') +
    (cal.moons.length ? (cal.moons.length === 1 ? 'One moon, ' + cal.moons[0].name + ', full every ' + cal.moons[0].cycle_days + ' days' : cap(numWord(cal.moons.length)) + ' moons: ' + joinList(cal.moons.map(function (m) { return m.name + ' (' + m.cycle_days + ' days)'; }))) + '. ' : '') +
    'The year is ' + year + ' ' + epoch + ', counted from ' + eraName + '. ' + (check.ok ? 'It matches the shape of Chronicle’s presets.' : 'It does not yet match the preset shape: ' + check.errors[0].message);
  return {
    generator: 'calendar', seed: seed, recipe: recipe, scope: { label: 'a calendar', days: 0 }, preset: preset,
    extras: { template: tpl, seasonTypes: seasonTypeMap, moons: moonRes.moons, validation: check }, summary: summary, stats: { months: reg.length, festivalDays: feasts.length, yearLength: yl, weekLength: week }, warnings: check.warnings
  };
}
function monthLengths(ms) {
  var ls = uniq(ms.map(function (m) { return m.days; })).sort(function (a, b) { return a - b; });
  return ls.length === 1 ? 'all of ' + ls[0] + ' days' : ls.length === 2 ? ls[0] + ' or ' + ls[1] + ' days' : ls[0] + ' to ' + ls[ls.length - 1] + ' days';
}

/* ── Calendar from the sky ──────────────────────────────────────────────────────────────────────────
   Derives a calendar's shape from how many suns, moons and wandering planets the world has, instead of
   from a template+culture recipe: the year's length comes from the sun(s); months come from the main
   moon's cycle, with whatever the cycle didn't spend becoming festival days (a small remainder) or
   folded into the last month (too large to be a "festival"); the week is one day per visible wanderer --
   the way our own 7-day week came from 7 wanderers -- unless that would be too short to feel like a
   week, in which case a quarter of the main moon's cycle (the moon's own quarters) takes over; and the
   leap rule falls out of whatever a whole-day month count dropped from the true, fractional year length.
   Seasons/names/era/festivals-on-the-solstices are the same tail calendar.generate() itself ends with,
   so the two are interchangeable from a caller's point of view (same result shape, same summary/
   preset/extras/warnings contract) -- this is a second way to REACH a calendar, not a second shape. */
function runCalendarFromSky(opts) {
  opts = opts || {};
  var seed = seedOf(opts);
  var suns = clamp(Math.round(opts.suns == null ? 1 : +opts.suns), 1, 3);
  var moonCount = clamp(Math.round(opts.moons == null ? 1 : +opts.moons), 0, 4);
  var planets = clamp(Math.round(opts.planets == null ? 0 : +opts.planets), 0, 7);
  var seasonsWanted = clamp(Math.round(opts.seasons == null ? 4 : +opts.seasons), 0, 6);
  var theme = opts.theme || 'pastoral';
  var style = opts.style || 'mixed';
  var td = themeData(theme);
  var R = function (k) { return makeRng(seed, 'sky', suns, moonCount, planets, k); };
  var used = {};

  // The year's length comes from the sun(s): one sun gives an Earth-like year; extra suns lengthen and
  // vary it (a binary/trinary system's combined seasonal cycle) -- a seed-stable flavour knob, same
  // honesty every other template already has (CAL_TEMPLATES never claims real astronomy either).
  var trueYearLength = R('year').range(300, 420) * (1 + 0.15 * (suns - 1));

  // Moons, before months: the main (first) moon sizes the months. No moon at all falls back to a
  // familiar ~30-day month so there is still something to count.
  var moonRes = moonCount ? runMoons({ recipe: { generator: 'moons', makes: { count: moonCount, strange: 'rare' }, details: { theme: theme, cycles: 'natural' } }, seed: seed + '|sky-moons' }) : { moons: [] };
  var mainCycle = moonRes.moons.length ? moonRes.moons[0].cycle_days : 0;
  var monthLen = mainCycle > 0 ? mainCycle : 30;
  var monthCount = Math.max(1, Math.round(trueYearLength / monthLen));
  var months = [], placed = 0, i;
  for (i = 0; i < monthCount; i++) { var len = Math.max(1, Math.round(monthLen)); months.push({ days: len }); placed += len; }
  var remainder = Math.round(trueYearLength) - placed;
  if (remainder !== 0 && Math.abs(remainder) <= 10 && months.length > 1) {
    months.splice(R('festival').int(1, months.length - 1), 0, { days: Math.max(1, Math.abs(remainder)), intercalary: true });
  } else if (remainder !== 0 && months.length) {
    months[months.length - 1].days = Math.max(1, months[months.length - 1].days + remainder);
  }
  var yearLength = sum(months.map(function (m) { return m.days; }));

  // The week is one day per visible wanderer, unless that is too few to feel like a week.
  var wanderers = suns + moonCount + planets;
  var quarterWeek = wanderers < 3 && mainCycle > 0;
  var week = wanderers >= 3 ? wanderers : (mainCycle > 0 ? Math.max(3, Math.round(mainCycle / 4)) : Math.max(3, wanderers));
  week = clamp(week, 2, 14);
  var rest = week >= 6 ? [week - 1] : [];

  // The leap rule comes from the fractional year: however many years it takes the fraction a whole-day
  // month count dropped to add back up to one whole day.
  var fracRemainder = Math.abs(trueYearLength - Math.round(trueYearLength));
  var leapEvery = 0, leapOffset = 0;
  if (fracRemainder > 0.02) { leapEvery = clamp(Math.round(1 / fracRemainder), 2, 40); leapOffset = R('leapoffset').int(0, leapEvery - 1); }
  var leapMonth = months.length - 1;

  // From here on this mirrors calendar.generate()'s own tail: place seasons, name everything, pick an
  // era and a calendar name, then check the result against Chronicle's own preset shape.
  var draftMonths = months.map(function (m, k) { return { name: 'M' + k, days: m.days, sort_order: k, is_intercalary: !!m.intercalary, leap_year_days: (leapEvery && k === leapMonth ? 1 : 0) }; });
  var plan = seasonsWanted ? seasonPlan(seasonsWanted, []) : [];
  var dc = makeCal({ name: 'draft', months: draftMonths, weekdays: [] }), L = dc.refYearLength, ry = 1;
  while (dc.isLeap(ry)) ry++;
  function dateAtShare(share) {
    var d = dc.fromAbs(dc.abs(ry, 1, 1) + Math.floor(mod(share, 1) * L)), n = 0;
    while (dc.isIntercalary(d.month) && n++ < 10) d = dc.fromAbs(dc.absOf(d) + 1);
    return d;
  }
  var seasonsOut = [], sr = R('seasons');
  plan.forEach(function (p, k) {
    var s0 = dateAtShare(p.at), nxt = plan[(k + 1) % plan.length], e0 = dc.fromAbs(dc.absOf(dateAtShare(nxt.at)) - 1);
    while (dc.isLeapOnly(ry, e0.month, e0.day)) e0 = dc.fromAbs(dc.absOf(e0) - 1);
    var nmS = nameSeason(td, sr, p.type, used);
    seasonsOut.push({ name: nmS, start_month: s0.month, start_day: s0.day, end_month: e0.month, end_day: e0.day, color: p.type ? SEASON_COLOURS[p.type] : 'oklch(0.62 0.08 ' + (k * 60 % 360) + ')', _type: p.type });
  });
  var seasonTypeMap = {};
  seasonsOut.forEach(function (s) { seasonTypeMap[s.name] = s._type; });
  var named = makeCal({ name: 'draft', months: draftMonths, weekdays: [], seasons: seasonsOut }, { seasonTypes: seasonTypeMap });
  var types = monthSeasonTypes(named, months.length);
  var monthNames = nameMonths(td, R('months'), types, months.map(function (m) { return !!m.intercalary; }), style, used, null);
  var wdNames = nameWeekdays(td, R('weekdays'), week, rest, style, used, null);
  var year = (function () { var yb = CAL_YEAR[theme] || [1000, 1500]; return R('yearnum').int(yb[0], yb[1]); })();
  var er = R('era'), eraName = nameEra(td, er, 'present', used);
  var epoch = eraName.split(/\s+/).filter(function (w) { return /^[A-Z]/.test(w) && w !== 'The'; }).map(function (w) { return w.charAt(0); }).join('');
  if (epoch.length < 2) epoch = eraName.replace(/^The\s+/, '').split(/[\s-]+/)[0].slice(0, 4);
  var place = namePlace(td, R('name'), used);
  var calName = R('calname').pick([place + ' Reckoning', 'The ' + place + ' Count', 'Calendar of ' + place, place + ' Almanac']);
  var cal = {
    name: calName, mode: 'fantasy', epoch_name: epoch,
    current_year: year, current_month: 1, current_day: 1, current_hour: 0, current_minute: 0,
    hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60,
    leap_year_every: leapEvery, leap_year_offset: leapOffset, tracks_real_time: false,
    months: months.map(function (m, k) { return { name: monthNames[k], days: m.days, sort_order: k, is_intercalary: !!m.intercalary, leap_year_days: (leapEvery && k === leapMonth ? 1 : 0) }; }),
    weekdays: wdNames.map(function (n, k) { return { name: n, sort_order: k, is_rest_day: rest.indexOf(k) >= 0 }; }),
    moons: moonRes.moons.map(function (m) { return { name: m.name, cycle_days: m.cycle_days, phase_offset: m.phase_offset, color: m.color }; }),
    seasons: seasonsOut.map(function (s) { return { name: s.name, start_month: s.start_month, start_day: s.start_day, end_month: s.end_month, end_day: s.end_day, color: s.color }; }),
    eras: [{ name: eraName, start_year: 1, color: oklch(0.56, 0.08, er.int(0, 359)), sort_order: 0, description: epoch }]
  };
  if (cal.seasons.length) {
    var fcal = makeCal(cal, { seasonTypes: seasonTypeMap }), an = fcal.anchorDays(ry), fr2 = R('festivals'), fests = [];
    [['winterSolstice', an.winterSolstice], ['springEquinox', an.springEquinox], ['summerSolstice', an.summerSolstice], ['autumnEquinox', an.autumnEquinox]].forEach(function (x) {
      var d = x[1];
      if (!d || fcal.isIntercalary(d.month) || fcal.isLeapOnly(ry, d.month, d.day)) return;
      if (fests.some(function (f) { return f.month === d.month && f.day === d.day; })) return;
      fests.push({ name: nameFestival(td, fr2, x[0], { moon: cal.moons[0] ? cal.moons[0].name : 'the moon' }, used), month: d.month, day: d.day, description: cap(ANCHOR_WORDS[x[0]]) + '.', sort_order: fests.length });
    });
    if (fests.length) cal.festivals = fests;
  }
  var preset = { format: 'chronicle-calendar-v1', version: 2, calendar: cal };
  var check = validatePreset(preset);
  var reg = cal.months.filter(function (m) { return !m.is_intercalary; }), feasts = cal.months.filter(function (m) { return m.is_intercalary; });

  var skyPhrase = numWord(suns) + (suns === 1 ? ' sun' : ' suns') + (moonCount ? ' and ' + count(moonCount, 'moon') : ', no moons') + (planets ? ' and ' + count(planets, 'wandering planet') : '');
  var weekSentence = quarterWeek
    ? 'The week is ' + numWord(week) + ' days, a quarter of the main moon’s cycle -- ' + count(wanderers, 'visible wanderer', 'visible wanderers') + ' would have made too short a week on ' + (wanderers === 1 ? 'its' : 'their') + ' own. '
    : 'The week is ' + numWord(week) + ' days, one for each of the ' + count(wanderers, 'visible wanderer', 'visible wanderers') + ' (suns, moons and wandering planets together). ';
  var summary = cap(skyPhrase) + ': ' + calName + ' has ' + count(reg.length, 'month') + ' of ' + (mainCycle > 0 ? 'about ' + Math.round(mainCycle) + ' days each, the main moon’s own cycle' : monthLen + ' days each') +
    (feasts.length ? ', plus ' + count(feasts.length, 'festival day') + ' the cycle left over' : '') + ', ' + yearLength + ' days in a common year. ' + weekSentence +
    (leapEvery ? 'The year runs ' + round2(fracRemainder) + ' days short of a whole day, so every ' + ordWord(leapEvery) + ' year is a leap year. ' : 'The year divides evenly, so there are no leap years. ') +
    (cal.seasons.length ? cap(count(cal.seasons.length, 'season')) + ': ' + joinList(cal.seasons.map(function (s) { return s.name; })) + '. ' : '') +
    (cal.moons.length ? (cal.moons.length === 1 ? 'One moon, ' + cal.moons[0].name + ', full every ' + cal.moons[0].cycle_days + ' days. ' : cap(numWord(cal.moons.length)) + ' moons: ' + joinList(cal.moons.map(function (m) { return m.name + ' (' + m.cycle_days + ' days)'; })) + '. ') : '') +
    'The year is ' + year + ' ' + epoch + ', counted from ' + eraName + '. ' + (check.ok ? 'It matches the shape of Chronicle’s presets.' : 'It does not yet match the preset shape: ' + check.errors[0].message);

  return {
    generator: 'calendar', seed: seed, recipe: { generator: 'calendar', name: 'From the sky', builtIn: false, details: { suns: suns, moons: moonCount, planets: planets, theme: theme } },
    scope: { label: 'a calendar', days: 0 }, preset: preset,
    extras: { template: 'sky', seasonTypes: seasonTypeMap, moons: moonRes.moons, validation: check, sky: { suns: suns, moons: moonCount, planets: planets, wanderers: wanderers, mainMoonCycle: mainCycle, quarterWeek: quarterWeek } },
    summary: summary, stats: { months: reg.length, festivalDays: feasts.length, yearLength: yearLength, weekLength: week }, warnings: check.warnings
  };
}

/* Any calendar the engine accepts, as Chronicle's preset/export shape: only the export's fields survive. */
function toPreset(input) {
  var c = input && input.__cal ? input.raw : (input && input.format && input.calendar) ? input.calendar : input;
  if (!isObj(c)) throw GenError('No calendar was given. Pass a Chronicle calendar (the preset or export shape).');
  function pick(o, kind) { var out = {}; EXPORT_KEYS[kind].split(' ').forEach(function (k) { if (o[k] !== undefined) out[k] = clone(o[k]); }); return out; }
  var cal = pick(c, 'calendar');
  [['months', 'month'], ['weekdays', 'weekday'], ['moons', 'moon'], ['seasons', 'season'], ['eras', 'era'], ['festivals', 'festival']].forEach(function (x) { if (Array.isArray(c[x[0]])) cal[x[0]] = c[x[0]].map(function (o) { return isObj(o) ? pick(o, x[1]) : o; }); });
  return { format: 'chronicle-calendar-v1', version: 2, calendar: cal };
}

/* Checks a calendar against the preset/export shape (export.go) and the model's own rules. */
function validatePreset(p) {
  var errors = [], warnings = [];
  function err(path, msg) { errors.push({ path: path, message: msg }); }
  if (!isObj(p)) return { ok: false, errors: [{ path: '', message: 'A preset is a JSON object.' }], warnings: [] };
  if (p.format !== 'chronicle-calendar-v1') err('format', 'The format must be "chronicle-calendar-v1".');
  if (p.version !== 2) err('version', 'The version must be 2.');
  var c = p.calendar;
  if (!isObj(c)) { err('calendar', 'There is no calendar object.'); return { ok: false, errors: errors, warnings: warnings }; }
  var allowed = {};
  ownKeys(EXPORT_KEYS).forEach(function (k) { allowed[k] = {}; EXPORT_KEYS[k].split(' ').forEach(function (f) { allowed[k][f] = 1; }); });
  ownKeys(c).forEach(function (k) { if (!allowed.calendar[k]) err('calendar.' + k, '“' + k + '” is not a field of a Chronicle calendar export.'); });
  var isInt = function (v) { return typeof v === 'number' && v === Math.floor(v); };
  if (typeof c.name !== 'string' || !c.name.trim()) err('calendar.name', 'The calendar needs a name.');
  if (c.mode !== 'fantasy' && c.mode !== 'reallife') err('calendar.mode', 'mode is "fantasy" or "reallife".');
  ['current_year', 'current_month', 'current_day', 'current_hour', 'current_minute', 'hours_per_day', 'minutes_per_hour', 'seconds_per_minute', 'leap_year_every', 'leap_year_offset'].forEach(function (k) { if (!isInt(c[k])) err('calendar.' + k, k + ' must be a whole number.'); });
  if (!(c.hours_per_day > 0 && c.minutes_per_hour > 0 && c.seconds_per_minute > 0)) err('calendar', 'Hours, minutes and seconds per unit must be above zero.');
  if (c.leap_year_every < 0) err('calendar.leap_year_every', 'leap_year_every cannot be negative.');
  if (typeof c.tracks_real_time !== 'boolean') err('calendar.tracks_real_time', 'tracks_real_time must be true or false.');
  function checkList(name, list, kind, fn) {
    if (list === undefined) return;
    if (!Array.isArray(list)) { err('calendar.' + name, name + ' must be a list.'); return; }
    list.forEach(function (x, i) {
      if (!isObj(x)) { err('calendar.' + name + '[' + i + ']', 'Each of ' + name + ' is an object.'); return; }
      ownKeys(x).forEach(function (k) { if (!allowed[kind][k]) err('calendar.' + name + '[' + i + '].' + k, '“' + k + '” is not a field of an exported ' + kind + '.'); });
      if (typeof x.name !== 'string' || !x.name.trim()) err('calendar.' + name + '[' + i + '].name', 'Every ' + kind + ' needs a name.');
      if (typeof x.color === 'string' && x.color.length > 20) warnings.push({ path: 'calendar.' + name + '[' + i + '].color', message: 'The colour “' + x.color + '” is over 20 characters, the size of Chronicle’s colour columns.' });
      fn(x, 'calendar.' + name + '[' + i + ']', i);
    });
  }
  if (!Array.isArray(c.months) || !c.months.length) err('calendar.months', 'A calendar needs at least one month.');
  checkList('months', c.months, 'month', function (m, path, i) {
    if (!isInt(m.days) || m.days < 0) err(path + '.days', 'days must be a whole number, 0 or more.');
    if (m.sort_order !== i) err(path + '.sort_order', 'sort_order should run 0, 1, 2... in order; this one is ' + m.sort_order + '.');
    if (typeof m.is_intercalary !== 'boolean') err(path + '.is_intercalary', 'is_intercalary must be true or false.');
    if (!isInt(m.leap_year_days) || m.leap_year_days < 0) err(path + '.leap_year_days', 'leap_year_days must be a whole number, 0 or more.');
  });
  if (Array.isArray(c.months) && c.months.every(function (m) { return !(m.days > 0) && !(m.leap_year_days > 0); })) err('calendar.months', 'Every month has zero days.');
  if (!Array.isArray(c.weekdays)) err('calendar.weekdays', 'weekdays must be a list (it may be empty).');
  checkList('weekdays', c.weekdays, 'weekday', function (w, path, i) {
    if (w.sort_order !== i) err(path + '.sort_order', 'sort_order should run 0, 1, 2... in order.');
    if (typeof w.is_rest_day !== 'boolean') err(path + '.is_rest_day', 'is_rest_day must be true or false.');
  });
  checkList('moons', c.moons, 'moon', function (m, path) {
    if (!(typeof m.cycle_days === 'number' && m.cycle_days > 0)) err(path + '.cycle_days', 'cycle_days must be a number above zero.');
    if (typeof m.phase_offset !== 'number') err(path + '.phase_offset', 'phase_offset must be a number.');
    if (typeof m.color !== 'string') err(path + '.color', 'color must be a string.');
  });
  var n = Array.isArray(c.months) ? c.months.length : 0;
  var daysOf = function (mi) { var M = c.months[mi - 1]; return M ? M.days : 0; };
  checkList('seasons', c.seasons, 'season', function (s, path) {
    ['start_month', 'end_month'].forEach(function (k) { if (!isInt(s[k]) || s[k] < 1 || s[k] > n) err(path + '.' + k, k + ' must name one of the ' + n + ' months (1 to ' + n + ').'); });
    if (!isInt(s.start_day) || s.start_day < 1 || s.start_day > daysOf(s.start_month)) err(path + '.start_day', 'start_day ' + s.start_day + ' is not a day of month ' + s.start_month + '.');
    if (!isInt(s.end_day) || s.end_day < 1 || s.end_day > daysOf(s.end_month)) err(path + '.end_day', 'end_day ' + s.end_day + ' is not a day of month ' + s.end_month + '.');
    if (typeof s.color !== 'string') err(path + '.color', 'color must be a string.');
  });
  checkList('eras', c.eras, 'era', function (e, path) {
    if (!isInt(e.start_year)) err(path + '.start_year', 'start_year must be a whole number.');
    if (e.end_year != null && (!isInt(e.end_year) || e.end_year < e.start_year)) err(path + '.end_year', 'end_year must be a whole number after start_year, or left out while the era is ongoing.');
    if (typeof e.color !== 'string') err(path + '.color', 'color must be a string.');
  });
  checkList('festivals', c.festivals, 'festival', function (f, path) {
    if (f.month != null && (!isInt(f.month) || f.month < 1 || f.month > n)) err(path + '.month', 'month must be 1 to ' + n + '.');
    if (f.month != null && f.day != null && (!isInt(f.day) || f.day < 1 || f.day > daysOf(f.month))) err(path + '.day', 'day ' + f.day + ' is not in month ' + f.month + '.');
  });
  if (isInt(c.current_month) && (c.current_month < 1 || c.current_month > n)) err('calendar.current_month', 'current_month must be 1 to ' + n + '.');
  else if (isInt(c.current_day) && n && (c.current_day < 1 || c.current_day > Math.max(1, daysOf(c.current_month)))) err('calendar.current_day', 'current_day is not a day of the current month.');
  return { ok: errors.length === 0, errors: errors, warnings: warnings };
}

/* ── Mass helpers ───────────────────────────────────────────────────────────────────────────────────
   Pure functions over lists the UI already holds: each returns a new list, what changed, and a sentence
   saying so. Locked items (gen.locked, or keys listed in `locked`) never move and are never replaced. */

function isLockedItem(item, lockedKeys) { return !!(item && ((item.gen && item.gen.locked) || (lockedKeys && item.gen && lockedKeys.indexOf(item.gen.key) >= 0))); }
function itemAbs(cal, it) { return cal.abs(it.year, it.month, it.day); }

/* Rerolls a scope: weather is regenerated there with a fresh draw and bridged into the days on either
   side; for event generators, the unlocked generated events inside the scope are replaced. */
function massReroll(o) {
  var gen = o.generator || 'weather', cal = makeCal(o.calendar), scope = resolveScope(cal, o.scope), existing = o.existing || [];
  var nonce = o.nonce || ('r' + (hash32(JSON.stringify(o.scope) + '|' + (o.seed || '')) >>> 0).toString(36));
  var lockedKeys = o.locked || [];
  if (gen === 'weather') {
    var keep = existing.filter(function (d) { return !scope.set[itemAbs(cal, d)] || isLockedItem(d, lockedKeys); });
    var locks = existing.filter(function (d) { return scope.set[itemAbs(cal, d)] && isLockedItem(d, lockedKeys); });
    var res = runWeather({ calendar: cal, recipe: o.recipe, seed: o.seed, scope: o.scope, zones: o.zones, kinds: o.kinds, locked: locks, context: { weather: keep }, nonce: nonce });
    var byKey = {};
    existing.forEach(function (d) { byKey[(d.zone_id || '') + '|' + itemAbs(cal, d)] = d; });
    res.days.forEach(function (d) { byKey[(d.zone_id || '') + '|' + itemAbs(cal, d)] = d; });
    var merged = ownKeys(byKey).map(function (k) { return byKey[k]; }).sort(function (a, b) { return itemAbs(cal, a) - itemAbs(cal, b); });
    var changed = res.days.filter(function (d) { return !(d.gen && d.gen.locked); }).length;
    return { items: merged, changed: changed, summary: 'Rerolled ' + count(changed, 'day') + ' of weather for ' + scope.label + (locks.length ? ', keeping the ' + count(locks.length, 'day') + ' you painted' : '') + '. The new days meet the weather on either side without a jump. ' + res.summary, result: res };
  }
  var dropped = existing.filter(function (e) { return e.gen && e.gen.generator === gen && !isLockedItem(e, lockedKeys) && scope.set[itemAbs(cal, e)]; });
  var kept = existing.filter(function (e) { return dropped.indexOf(e) < 0; });
  var runOpts = { calendar: cal, recipe: o.recipe, seed: String(o.seed == null ? 'chronicle' : o.seed) + '#' + nonce, scope: o.scope, locked: kept, context: o.context, eventKinds: o.eventKinds, categories: o.categories };
  var fresh = run(gen, runOpts);
  var items = (fresh.events || []).filter(function (e) { return !kept.some(function (k) { return k.gen && k.gen.key === e.gen.key; }); });
  var merged2 = kept.concat(items).sort(function (a, b) { return itemAbs(cal, a) - itemAbs(cal, b); });
  return { items: merged2, changed: items.length, removed: dropped.length, summary: 'Rerolled ' + scope.label + ': ' + count(dropped.length, 'generated event') + ' replaced by ' + numWord(items.length) + ' new ' + (items.length === 1 ? 'one' : 'ones') + '; locked and hand-made events stayed.', result: fresh };
}
/* From a date to the end of its year, leaving everything before it as it is (and blending into it). */
function massFillRestOfYear(o) {
  var cal = makeCal(o.calendar), from = dateOf(o.from || { year: cal.currentYear, month: cal.currentMonth, day: cal.currentDay });
  var last = cal.fromAbs(cal.yearSpan(from.year)[1]);
  var gen = o.generator || 'weather', existing = o.existing || [], t0 = cal.absOf(from);
  if (gen === 'weather') {
    var have = {};
    existing.forEach(function (d) { have[(d.zone_id || '') + '|' + itemAbs(cal, d)] = 1; });
    var res = runWeather({ calendar: cal, recipe: o.recipe, seed: o.seed, zones: o.zones, kinds: o.kinds, scope: { range: { from: from, to: last } }, locked: existing.filter(function (d) { return itemAbs(cal, d) >= t0 && isLockedItem(d, o.locked); }), context: { weather: existing } });
    var added = res.days.filter(function (d) { return !have[(d.zone_id || '') + '|' + itemAbs(cal, d)] || !(d.gen && d.gen.locked); });
    var byKey = {};
    existing.forEach(function (d) { byKey[(d.zone_id || '') + '|' + itemAbs(cal, d)] = d; });
    res.days.forEach(function (d) { var k = (d.zone_id || '') + '|' + itemAbs(cal, d); if (!byKey[k] || !isLockedItem(byKey[k], o.locked)) byKey[k] = d; });
    return { items: ownKeys(byKey).map(function (k) { return byKey[k]; }).sort(function (a, b) { return itemAbs(cal, a) - itemAbs(cal, b); }), changed: added.length, summary: 'Filled ' + count(res.days.length, 'day') + ' from ' + cal.fmt(from) + ' to the end of ' + from.year + ', continuing from the weather before it. ' + res.summary, result: res };
  }
  return massReroll({ generator: gen, calendar: cal, recipe: o.recipe, seed: o.seed, scope: { range: { from: from, to: last } }, existing: existing.filter(function (e) { return itemAbs(cal, e) < t0 || isLockedItem(e, o.locked); }), locked: o.locked, context: o.context, nonce: o.nonce || 'fill', kinds: o.kinds, eventKinds: o.eventKinds, categories: o.categories });
}
/* Moves events by `days` (negative is earlier). Spans, repeats and their end dates move with them. */
function massShift(events, o) {
  var cal = makeCal(o.calendar), n = o.days | 0, keys = o.keys, lockedKeys = o.locked || [];
  var pick = typeof o.filter === 'function' ? o.filter : function (e) { return !keys || (e.gen && keys.indexOf(e.gen.key) >= 0); };
  var moved = 0, stayed = 0, notes = [];
  var out = events.map(function (e) {
    if (!pick(e)) return e;
    if (isLockedItem(e, lockedKeys)) { stayed++; return e; }
    var c = clone(e), a = itemAbs(cal, e) + n, d = cal.fromAbs(a);
    c.year = d.year; c.month = d.month; c.day = d.day;
    if (e.end_year != null) { var ed = cal.fromAbs(cal.abs(e.end_year, e.end_month, e.end_day) + n); c.end_year = ed.year; c.end_month = ed.month; c.end_day = ed.day; }
    if (e.recurrence_end_year != null) { var rd = cal.fromAbs(cal.abs(e.recurrence_end_year, e.recurrence_end_month, e.recurrence_end_day) + n); c.recurrence_end_year = rd.year; c.recurrence_end_month = rd.month; c.recurrence_end_day = rd.day; }
    if (c.is_recurring && c.recurrence_type === 'yearly' && cal.isLeapOnly(d.year, d.month, d.day)) notes.push(e.name + ' now falls on a leap day, so it will only come round in leap years.');
    if (c.is_recurring && (c.recurrence_type === 'weekly' || c.recurrence_type === 'biweekly') && cal.weekLength && cal.weekdayOfAbs(a) !== cal.weekdayOfAbs(itemAbs(cal, e))) notes.push(e.name + ' now repeats on ' + cal.weekdays[cal.weekdayOfAbs(a)].name + 's.');
    if (c.gen) c.gen = Object.assign({}, c.gen, { shiftedBy: (e.gen.shiftedBy || 0) + n });
    moved++;
    return c;
  });
  var dir = n >= 0 ? 'later' : 'earlier';
  return { items: out, changed: moved, summary: 'Moved ' + count(moved, 'event') + ' ' + numWord(Math.abs(n)) + (Math.abs(n) === 1 ? ' day ' : ' days ') + dir + (stayed ? '; ' + count(stayed, 'locked event') + ' stayed put' : '') + '.' + (notes.length ? ' ' + notes.join(' ') : '') };
}
/* Copies one month's events into another, day for day. Days the target month lacks are skipped (or
   pinned to its last day with overflow: 'clamp'); repeating events stay behind unless asked for. */
function massCopyMonth(events, o) {
  var cal = makeCal(o.calendar), from = o.from, to = o.to, clampOver = o.overflow === 'clamp';
  var toLen = cal.monthDays(to.month, to.year), skipped = [], made = [];
  events.forEach(function (e) {
    if (e.year !== from.year || e.month !== from.month) return;
    if (e.is_recurring && !o.includeRecurring) return;
    var day = e.day;
    if (day > toLen) { if (!clampOver) { skipped.push(e.name); return; } day = toLen; }
    var c = clone(e), span = e.end_year != null ? cal.abs(e.end_year, e.end_month, e.end_day) - itemAbs(cal, e) : 0;
    c.year = to.year; c.month = to.month; c.day = day;
    if (span) { var ed = cal.fromAbs(cal.abs(to.year, to.month, day) + span); c.end_year = ed.year; c.end_month = ed.month; c.end_day = ed.day; }
    c.gen = Object.assign({}, c.gen || {}, { key: makeKey('copy', (e.gen && e.gen.key) || e.name, to.year, to.month), copiedFrom: e.gen ? e.gen.key : null, locked: false });
    made.push(c);
  });
  var fromName = cal.months[from.month - 1].name, toName = cal.months[to.month - 1].name;
  return { items: events.concat(made), added: made, changed: made.length, summary: 'Copied ' + count(made.length, 'event') + ' from ' + fromName + ' ' + from.year + ' to ' + toName + ' ' + to.year + '.' + (skipped.length ? ' ' + joinList(skipped) + (skipped.length === 1 ? ' falls' : ' fall') + ' on a day ' + toName + ' doesn’t have, so ' + (skipped.length === 1 ? 'it was' : 'they were') + ' left out.' : '') };
}
/* One event per month from a pattern: rule 'day' (a day of the month), 'last' (the last day), 'weekday'
   (the nth weekday, nth may be -1 for the last). With asRecurring and a day every month has, it is a
   single monthly repeating event instead of a list. */
function massEveryMonth(pattern, o) {
  var cal = makeCal(o.calendar), year = o.year != null ? o.year : cal.currentYear, rule = o.rule || 'day', made = [], missing = [];
  var regular = cal.months.filter(function (M) { return o.includeFestivalDays || !M.intercalary; });
  if (rule === 'day' && o.asRecurring && regular.every(function (M) { return cal.monthDays(M.index, year) >= (o.day || 1); })) {
    var e0 = Object.assign({}, clone(pattern), { year: year, month: regular[0].index, day: o.day || 1, is_recurring: true, recurrence_type: 'monthly' });
    e0.gen = Object.assign({}, pattern.gen || {}, { key: makeKey('monthly', pattern.name, year), locked: false });
    return { items: [e0], changed: 1, summary: pattern.name + ' repeats on day ' + (o.day || 1) + ' of every month from ' + cal.fmt(e0) + '.' };
  }
  regular.forEach(function (M) {
    var len = cal.monthDays(M.index, year), day = null;
    if (!len) return;
    if (rule === 'day') day = (o.day || 1) <= len ? (o.day || 1) : null;
    else if (rule === 'last') day = len;
    else if (rule === 'weekday') {
      var want = typeof o.weekday === 'number' ? o.weekday - 1 : cal.weekdays.map(function (w) { return w.name.toLowerCase(); }).indexOf(String(o.weekday).toLowerCase());
      var hits = [];
      for (var d = 1; d <= len; d++) if (cal.weekday(year, M.index, d) === want) hits.push(d);
      var nth = o.nth || 1;
      day = nth > 0 ? hits[nth - 1] : hits[hits.length + nth];
    }
    if (!day) { missing.push(M.name); return; }
    var e = Object.assign({}, clone(pattern), { year: year, month: M.index, day: day, is_recurring: false });
    e.gen = Object.assign({}, pattern.gen || {}, { key: makeKey('everymonth', pattern.name, year, M.index), locked: false });
    made.push(e);
  });
  return { items: made, changed: made.length, summary: 'Placed ' + pattern.name + ' in ' + count(made.length, 'month') + ' of ' + year + '.' + (missing.length ? ' ' + joinList(missing) + (missing.length === 1 ? ' has' : ' have') + ' no such day, so ' + (missing.length === 1 ? 'it was' : 'they were') + ' skipped.' : '') };
}
/* "Select a bunch": keys of the items matching every filter given. */
function massSelect(items, f) {
  f = f || {};
  var cal = f.calendar ? makeCal(f.calendar) : null, from = f.from && cal ? cal.absOf(dateOf(f.from)) : null, to = f.to && cal ? cal.absOf(dateOf(f.to)) : null;
  var text = f.text ? String(f.text).toLowerCase() : null;
  return items.filter(function (e) {
    var g = e.gen || {};
    if (f.generator && g.generator !== f.generator) return false;
    if (f.kind && [].concat(f.kind).indexOf(g.kind) < 0) return false;
    if (f.category && [].concat(f.category).indexOf(e.category) < 0) return false;
    if (f.moon && !(g.moon && String(g.moon).toLowerCase().indexOf(String(f.moon).toLowerCase()) >= 0)) return false;
    if (f.visibility && e.visibility !== f.visibility) return false;
    if (f.locked != null && !!g.locked !== !!f.locked) return false;
    if (from != null && cal && itemAbs(cal, e) < from) return false;
    if (to != null && cal && itemAbs(cal, e) > to) return false;
    if (text && (String(e.name || '') + ' ' + String(e.description || '')).toLowerCase().indexOf(text) < 0) return false;
    return true;
  }).map(function (e) { return g0(e); });
  function g0(e) { return e.gen && e.gen.key ? e.gen.key : null; }
}

/* ── Public surface ── */
function run(generatorId, opts) {
  var g = GENERATORS[generatorId];
  if (!g) throw GenError('There’s no generator called “' + generatorId + '”. Try one of ' + joinList(GEN_ORDER) + '.');
  return g.run(opts || {});
}
var API = {
  version: VERSION,
  run: run,
  randomSeed: randomSeed,
  generators: generatorList(),
  cal: { from: makeCal, scope: function (calendar, scope) { var c = makeCal(calendar); var s = resolveScope(c, scope); return { days: s.days.map(c.fromAbs), label: s.label, kind: s.kind }; }, validatePreset: validatePreset, toPreset: toPreset },
  recipes: {
    builtIn: function (generatorId) { return ownKeys(BUILTIN_RECIPES).filter(function (k) { return !generatorId || BUILTIN_RECIPES[k].generator === generatorId; }).map(function (k) { return clone(BUILTIN_RECIPES[k]); }); },
    get: function (id) { return BUILTIN_RECIPES[id] ? clone(BUILTIN_RECIPES[id]) : null; },
    defaults: recipeDefaults, copy: copyRecipe, validate: validateRecipe, toJSON: recipeToJSON, fromJSON: recipeFromJSON, describe: describeRecipe,
    levels: FREQ_LEVELS.slice()
  },
  weather: {
    generate: function (o) { return run('weather', o); }, forecast: forecastWeather, guide: weatherGuide, zonePayload: zonePayload, toWeatherInput: toWeatherInput,
    climates: function () { return ownKeys(CLIMATES).map(function (k) { return { id: k, name: CLIMATES[k].name, blurb: CLIMATES[k].blurb, latitude: CLIMATES[k].latitude, magic: !!CLIMATES[k].magic }; }); },
    climate: function (id) { return CLIMATES[id] ? clone(CLIMATES[id]) : null; }, checkClimate: checkClimate,
    presets: presetList, validateKind: validateWeatherKind, effects: weatherEffects,
    systems: function () { return ownKeys(SYSTEMS).map(function (k) { return { id: k, label: SYSTEMS[k].label, switch: SYSTEMS[k].group, days: SYSTEMS[k].len.slice() }; }); }
  },
  sky: { generate: function (o) { return run('sky', o); }, kinds: function () { return clone(SKY_KINDS); } },
  events: { generate: function (o) { return run('events', o); }, kinds: function () { return clone(EVENT_SWITCHES); }, categories: EVENT_KINDS.slice(), validateKind: validateEventKind, validateCategories: checkCategories },
  tables: {
    builtIn: function () { return clone(STARTER_TABLES); }, validate: validateTables, roll: rollTableOnce, generate: function (o) { return run('table', o); },
    toJSON: function (set) { return JSON.stringify(set, null, 2); },
    fromJSON: function (text, opts) { var o; try { o = JSON.parse(text); } catch (e) { return { ok: false, errors: [{ path: '', message: jsonErrorMessage(String(text), e, 'table') }], warnings: [], tables: null }; } var v = validateTables(o, opts); v.tables = v.ok ? o : null; return v; },
    quick: quickTables, format: TABLE_FORMAT
  },
  moons: {
    generate: function (o) { return run('moons', o); },
    phase: function (moon, calendar, date) { var c = makeCal(calendar), m = { cycle: +moon.cycle_days || +moon.cycle, offset: +moon.phase_offset || +moon.offset || 0 }; var p = moonPhaseAt(m, c.absOf(dateOf(date))); return { phase: round2(p), name: moonPhaseName(p), lit: round2(litFraction(p)) }; },
    moonEvents: function (o) { var c = makeCal(o.calendar), s = resolveScope(c, o.scope); return findMoonEvents(c, s.moons.filter(function (m) { return m.cycle > 0; }), s.from, s.to).map(function (e) { return { type: e.type, date: c.fromAbs(e.abs), moon: e.moon ? e.moon.name : null, moons: e.moons ? e.moons.map(function (m) { return m.name; }) : null }; }); }
  },
  history: { generate: function (o) { return run('history', o); }, kinds: function () { return clone(HISTORY_KINDS); } },
  calendar: { generate: function (o) { return run('calendar', o); }, templates: function () { return clone(CAL_TEMPLATES); }, fromSky: function (o) { return runCalendarFromSky(o || {}); } },
  mass: { reroll: massReroll, fillRestOfYear: massFillRestOfYear, shiftEvents: massShift, copyMonth: massCopyMonth, everyMonth: massEveryMonth, select: massSelect },
  names: { generate: function (o) { return run('names', o); }, themes: function () { return Object.keys(THEMES).map(function (k) { return { id: k, label: THEMES[k].label, blurb: THEMES[k].blurb }; }); } },
  _internal: { PRESETS: PRESETS, SYSTEMS: SYSTEMS, CLIMATES: CLIMATES, STARTER_TABLES: STARTER_TABLES, findMoonEvents: findMoonEvents, moonPhaseAt: moonPhaseAt, compileClimate: compileClimate, makeCal: makeCal, resolveScope: resolveScope, makeRng: makeRng, gregorianJDN: gregorianJDN, themeData: themeData, tongueWord: tongueWord, BLOCKED_SET: BLOCKED_SET }
};
return API;
});
