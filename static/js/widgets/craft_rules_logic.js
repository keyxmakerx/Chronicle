/**
 * craft_rules_logic.js -- what a recipe's "When… then…" rules mean
 *
 * A rule is data: when every condition is true, then one effect, with an
 * optional chance and who sees it. Conditions name things from the
 * campaign's own calendar (its moons, events, rolled events, weather and
 * seasons) by the names the calendar gives them, so nothing here is built
 * for one world. This file is pure: it cleans rules written by a Director or
 * their AI, reads a calendar into a run of days, and answers whether a rule
 * is on, when it is next on, whether the recipe can be made on a day, and
 * how a rule reads as a sentence. craft_rules.ai.md is the guide;
 * test/js/craft_rules_logic.test.mjs pins it.
 *
 * The editor only previews. The crafting app evaluates the same data when
 * the project roll is made, against the calendar's date at that moment.
 */
(function () {
  'use strict';

  /* ---------- The fixed lists a rule may name ---------- */
  var KINDS = [['moon', 'Moon'], ['event', 'Calendar event'], ['rolled', 'Rolled event'], ['weather', 'Weather'], ['season', 'Season'],
    ['time', 'Time of day'], ['bench', 'Bench'], ['ingredient', 'Ingredient'], ['maker', 'Maker'], ['always', 'Every time']];
  var EFFECTS = [['only', 'Only then'], ['cannot', 'Can’t be made'], ['edge', 'Edge'], ['bane', 'Bane'], ['points', 'Project points'],
    ['extra', 'An extra one'], ['fine', 'Comes out fine'], ['becomes', 'Becomes…'], ['mark', 'Marked…'], ['quirk', 'A quirk'],
    ['mishap', 'A mishap'], ['discover', 'Discovers…'], ['say', 'The bench says…']];
  var CHANCES = ['always', '1 in 2', '1 in 3', '1 in 4', '1 in 6', '1 in 10', '1 in 20'];
  /* Who sees a rule: written in the recipe's book, shown on the bench when
     it fires, or the Director's alone. The AI's word for "shown" is
     "whenItHappens", which says what it means. */
  var PLAYERS = {book: 'book', shown: 'whenItHappens', secret: 'secret'};
  var PHASES = ['new', 'waxing crescent', 'first quarter', 'waxing gibbous', 'full', 'waning gibbous', 'last quarter', 'waning crescent'];
  var STATES = ['full', 'half', 'crescent', 'dark', 'vanished'];
  var STATE_WORDS = {full: 'Full', half: 'Half', crescent: 'Crescent', dark: 'Dark', vanished: 'Vanished'};
  var STATE_PHASE = {full: 0.5, half: 0.25, crescent: 0.125, dark: 0, vanished: 0};
  var TIMES = ['dawn', 'day', 'dusk', 'night'];
  /* Moves a look can add to a spot, beside the shared engine's effects. */
  var MOVES = {shake: {name: 'Shake', note: 'rattles what’s there'}, dim: {name: 'Dim', note: 'the light drops'}};
  /* The built-in weather kinds in the calendar's order, each with the
     calendar's picture for it (clear, cloud, rain, snow, storm or fog). A
     test pins them to the generator's presets in chronicle_gen.js, which
     another pins to weatherPresets in weather_kinds.go. */
  var WEATHER_TABLE = [['Clear', 'clear'], ['Partly Cloudy', 'clear'], ['Cloudy', 'cloud'], ['Overcast', 'cloud'], ['Drizzle', 'rain'],
    ['Rain', 'rain'], ['Fog', 'fog'], ['Mist', 'fog'], ['Windy', 'cloud'], ['Sunshower', 'rain'], ['Snow', 'snow'], ['Sleet', 'rain'],
    ['Heat Wave', 'clear'], ['Thunderstorm', 'storm'], ['Blizzard', 'snow'], ['Hail', 'storm'], ['Tornado', 'storm'], ['Hurricane', 'storm'],
    ['Ice Storm', 'storm'], ['Monsoon', 'rain'], ['Ashfall', 'fog'], ['Sandstorm', 'fog'], ['Luminous Sky', 'clear'], ['Sakura Bloom', 'clear'],
    ['Autumn Leaves', 'cloud'], ['Rolling Fog', 'fog'], ['Wildfire Smoke', 'fog'], ['Dust Devil', 'clear'], ['Black Sun', 'cloud'],
    ['Ley Surge', 'storm'], ['Aether Haze', 'fog'], ['Nullfront', 'cloud'], ['Permafrost Surge', 'snow'], ['Gravewind', 'cloud'],
    ['Veilfall', 'fog'], ['Arcane Winds', 'cloud'], ['Acid Rain', 'rain'], ['Blood Rain', 'rain'], ['Meteor Shower', 'clear'],
    ['Spore Cloud', 'fog'], ['Divine Light', 'clear'], ['Plague Miasma', 'fog']];
  var WEATHER = WEATHER_TABLE.map(function (w) { return w[0]; });
  var GLYPHS = ['clear', 'cloud', 'rain', 'snow', 'storm', 'fog'];
  /* Conditions that are not about the day: the preview can't know them, so
     a rule with one is "on, if…" rather than on or off. */
  var DAYLESS = {time: 1, bench: 1, ingredient: 1, maker: 1};
  var LIMITS = {rules: 40, when: 3, text: 120, line: 240, words: 48};

  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function str(v, max) { return typeof v === 'string' ? v.trim().slice(0, max || LIMITS.text) : ''; }
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return {'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]; }); }
  function clone(o) { return o == null ? o : JSON.parse(JSON.stringify(o)); }
  function lower(s) { return String(s || '').toLowerCase(); }
  function oneOf(list, v, dflt) { return list.indexOf(v) >= 0 ? v : dflt; }
  function kindOK(k) { return KINDS.some(function (x) { return x[0] === k; }); }
  function effectOK(k) { return EFFECTS.some(function (x) { return x[0] === k; }); }

  /* The phase name centred on its turning point, as the calendar names it
     (Moon.MoonPhaseName): new runs from 15/16 of the cycle to 1/16. */
  function phaseName(p) {
    if (p < 0.0625 || p >= 0.9375) return 'new';
    return PHASES[Math.floor((p - 0.0625) / 0.125) + 1];
  }
  function cyclePhase(m, abs) {
    if (!(m.cycle > 0)) return 0;
    var raw = (abs + m.offset) / m.cycle;
    return raw - Math.floor(raw);
  }

  /* ---------- Reading a calendar into days ---------- */

  /* world(o) reads one calendar, as the viewer may see it, into what the
     rules need. o.calendar is the calendar API's JSON; o.events, o.weather
     and o.rolls are what was read for the days ahead ({name, year, month,
     day}; {"y-m-d": label}; {event, result, year, month, day}); o.eventNames
     and o.rolled are the calendar's whole lists for the pickers; o.sky is
     SkyWorld (dayIndex, monthDays), so a day's moons are read on the same
     day count the calendar page uses. o.lists holds the campaign's own
     benches, skills, items, recipes, marks and tables. */
  function world(o) {
    var cal = o.calendar || {}, sky = o.sky, months = (cal.months || []).slice();
    var W = {name: cal.name || 'Calendar', span: o.span || 30, horizon: o.horizon || 400, lists: o.lists || {}, hand: {}};
    W.months = months.map(function (m) { return m.name; });
    W.moons = (cal.moons || []).filter(function (m) { return m && m.name; }).map(function (m) {
      var manual = m.phase_mode === 'manual', words = {};
      STATES.forEach(function (s) { words[s] = str((m.state_names || {})[s], 40) || STATE_WORDS[s]; });
      return {name: m.name, color: /^#[0-9a-f]{6}$/i.test(m.color || '') ? m.color : '#d8d4c8', manual: manual, words: words,
        cycle: +m.cycle_days || 0, offset: +m.phase_offset || 0, raw: m};
    }).filter(function (m) { return m.manual || m.cycle > 0; });
    W.seasons = (cal.seasons || []).filter(function (s) { return s && s.name; }).map(function (s) {
      return {name: s.name, from: [s.start_month, s.start_day], to: [s.end_month, s.end_day]};
    });
    W.eventNames = uniq((o.eventNames || []).concat((o.events || []).map(function (e) { return e.name; })));
    W.rolled = (o.rolled || []).filter(function (r) { return r && r.name; }).map(function (r) { return {name: r.name, results: uniq(r.results || [])}; });
    /* The owner's own kinds of weather come as {name, icon}; a day's
       reading as a label or {label, icon}. Each label keeps its picture. */
    W.glyphs = {};
    WEATHER_TABLE.forEach(function (w) { W.glyphs[w[0]] = w[1]; });
    var own = (o.weatherKinds || []).map(function (k) {
      var n = typeof k === 'string' ? k : k && str(k.name, 60);
      if (n && k && GLYPHS.indexOf(k.icon) >= 0) W.glyphs[n] = k.icon;
      return n;
    });
    W.weatherKinds = uniq(WEATHER.concat(own));
    W.forecastKinds = [];

    var evByDay = {}, wxByDay = {}, rollByDay = {};
    (o.events || []).forEach(function (e) { var k = e.year + '-' + e.month + '-' + e.day; (evByDay[k] = evByDay[k] || []).push(e.name); });
    (o.rolls || []).forEach(function (r) { var k = r.year + '-' + r.month + '-' + r.day; (rollByDay[k] = rollByDay[k] || {})[r.event] = r.result; });
    Object.keys(o.weather || {}).forEach(function (k) {
      var w = o.weather[k], n = typeof w === 'string' ? w : w && str(w.label, 60);
      if (!n) return;
      wxByDay[k] = n;
      if (w.icon && GLYPHS.indexOf(w.icon) >= 0 && !W.glyphs[n]) W.glyphs[n] = w.icon;
      if (W.weatherKinds.indexOf(n) < 0) W.weatherKinds.push(n);
      if (W.forecastKinds.indexOf(n) < 0) W.forecastKinds.push(n);
    });

    function mdays(i, y) { return sky && sky.monthDays ? sky.monthDays(cal, i, y) : (months[i] || {}).days || 30; }
    function idx(y, m, d) { return sky && sky.dayIndex ? sky.dayIndex(cal, y, m, d) : 0; }
    function seasonOf(m, d) {
      var v = m * 100 + d, hit = '';
      W.seasons.forEach(function (s) {
        var a = s.from[0] * 100 + s.from[1], b = s.to[0] * 100 + s.to[1];
        if (a <= b ? (v >= a && v <= b) : (v >= a || v <= b)) hit = hit || s.name;
      });
      return hit;
    }
    /* Days are made on demand from today forward and kept. Events, weather
       and rolls are known only for the days read; past those, a rule that
       needs them says so rather than guessing. */
    var days = [], y = cal.current_year, m = cal.current_month, d = cal.current_day;
    if (!months.length || !(m >= 1) || !(d >= 1)) { y = 1; m = 1; d = 1; months = months.length ? months : [{name: 'Month', days: 30}]; W.months = months.map(function (x) { return x.name; }); }
    W.knownTo = (o.knownDays == null ? W.span : o.knownDays) - 1;
    W.day = function (i) {
      while (days.length <= i) {
        var k = y + '-' + m + '-' + d, n = days.length, abs = n ? days[n - 1].abs + 1 : idx(y, m, d);
        days.push({i: n, year: y, month: m, day: d, abs: abs, monthName: months[m - 1] ? months[m - 1].name : '',
          label: (months[m - 1] ? months[m - 1].name + ' ' : '') + d, season: seasonOf(m, d), known: n <= W.knownTo,
          events: evByDay[k] || [], weather: wxByDay[k] || '', rolled: rollByDay[k] || {}});
        d++;
        if (d > mdays(m - 1, y)) { d = 1; m++; if (m > months.length) { m = 1; y++; } }
      }
      return days[i];
    };
    W.today = W.day(0);
    /* A hand-set moon's state today, from the marks the viewer was shown;
       the preview can be set to any state, since its future is unknown. */
    W.moons.forEach(function (mo) { if (mo.manual) W.hand[mo.name] = handToday(cal, mo.raw, W.today, sky) || 'full'; });
    return W;
  }
  function handToday(cal, mo, today, sky) {
    var st = '';
    (mo.marks || []).forEach(function (k) {
      var a = sky && sky.dayIndex ? sky.dayIndex(cal, k.year, k.month, k.day) : 0;
      if (a <= today.abs && STATES.indexOf(k.state) >= 0) st = k.state;
    });
    return st;
  }
  function uniq(a) { var out = []; a.forEach(function (x) { if (typeof x === 'string' && x && out.indexOf(x) < 0) out.push(x); }); return out; }
  function moonBy(W, n) { for (var i = 0; i < W.moons.length; i++) if (W.moons[i].name === n) return W.moons[i]; return null; }
  function rolledBy(W, n) { for (var i = 0; i < W.rolled.length; i++) if (W.rolled[i].name === n) return W.rolled[i]; return null; }
  function seasonBy(W, n) { for (var i = 0; i < W.seasons.length; i++) if (W.seasons[i].name === n) return W.seasons[i]; return null; }
  /* A moon on a day: a cycling moon's phase and its name, or a hand-set
     moon's state as the preview has it. */
  function moonOn(W, mo, day) {
    if (mo.manual) { var s = W.hand[mo.name] || ''; return {state: s, p: STATE_PHASE[s] || 0, gone: s === 'vanished'}; }
    var p = cyclePhase(mo, day.abs);
    return {p: p, name: phaseName(p)};
  }
  function phaseWord(W, c) {
    var mo = moonBy(W, c.moon);
    if (mo && mo.manual) return mo.words[c.phase] || c.phase;
    return c.phase;
  }

  /* ---------- Cleaning what a Director or their AI wrote ---------- */

  /* cleanCond keeps a condition's own fields and nothing else. Names stay
     as written, even when the calendar has no such name: the rule is then
     flagged and stays off until it is fixed, never pointed at a guess. */
  function cleanCond(c) {
    if (!c || typeof c !== 'object' || !kindOK(c.kind)) return null;
    switch (c.kind) {
      case 'moon':
        var ph = lower(c.phase);
        return {kind: 'moon', moon: str(c.moon), phase: PHASES.indexOf(ph) >= 0 || STATES.indexOf(ph) >= 0 || ph === 'waxing' || ph === 'waning' ? ph : 'full'};
      case 'event': return {kind: 'event', event: str(c.event)};
      case 'rolled': return {kind: 'rolled', event: str(c.event), result: str(c.result)};
      case 'weather': return {kind: 'weather', weather: str(c.weather)};
      case 'season': return {kind: 'season', season: str(c.season)};
      case 'time': return {kind: 'time', time: oneOf(TIMES, lower(c.time), 'night')};
      case 'bench': return {kind: 'bench', bench: str(c.bench)};
      case 'ingredient': return {kind: 'ingredient', mark: str(c.mark)};
      case 'maker': return {kind: 'maker', skill: str(c.skill)};
    }
    return {kind: 'always'};
  }
  function cleanThen(t) {
    if (!t || typeof t !== 'object' || !effectOK(t.kind)) return null;
    var o = {kind: t.kind}, n = Math.round(+t.amount);
    switch (t.kind) {
      case 'points': o.amount = isFinite(n) && n ? Math.max(-10, Math.min(10, n)) : 2; break;
      case 'extra': o.amount = isFinite(n) ? Math.max(1, Math.min(3, n)) : 1; break;
      case 'becomes': o.item = str(t.item); break;
      case 'mark': o.mark = str(t.mark); break;
      case 'quirk': case 'mishap': o.table = str(t.table); break;
      case 'discover': o.recipe = str(t.recipe); break;
      case 'say': o.text = str(t.text, LIMITS.line); break;
    }
    return o;
  }
  /* cleanLook keeps the spots and looks the shared effects engine knows
     (spots and looks are passed in, from ItemLookFX), the two moves, and
     short words. Anything else is dropped. */
  function cleanLook(l, spots, looks) {
    if (!l || typeof l !== 'object') return null;
    var out = {spots: {}, words: str(l.words, LIMITS.words)}, sp = l.spots && typeof l.spots === 'object' ? l.spots : {};
    (spots || []).forEach(function (k) {
      if (!has(sp, k) || !sp[k] || typeof sp[k] !== 'object') return;
      var v = {};
      if (typeof sp[k].look === 'string' && looks && has(looks, sp[k].look)) v.look = sp[k].look;
      if (typeof sp[k].move === 'string' && has(MOVES, sp[k].move)) v.move = sp[k].move;
      if (v.look || v.move) out.spots[k] = v;
    });
    return out;
  }
  /* cleanRule reads one rule as the AI writes it (players "whenItHappens",
     a "bookLine") or as the editor holds it, and returns the editor's form,
     or null when it has no condition or effect the list knows. */
  function cleanRule(r, fx) {
    if (!r || typeof r !== 'object') return null;
    var when = (Array.isArray(r.when) ? r.when : [r.when]).map(cleanCond).filter(Boolean).slice(0, LIMITS.when);
    var then = cleanThen(r.then);
    if (!when.length || !then) return null;
    if (when.some(function (c) { return c.kind === 'always'; })) when = [{kind: 'always'}];
    var players = r.players === 'whenItHappens' ? 'shown' : oneOf(['book', 'shown', 'secret'], r.players, 'book');
    var out = {when: when, then: then, chance: then.kind === 'only' ? 'always' : oneOf(CHANCES, r.chance, 'always'), players: players,
      line: str(r.bookLine != null ? r.bookLine : r.line, LIMITS.line), look: null, draft: r.draft === true};
    if (players !== 'secret' && r.look) out.look = cleanLook(r.look, fx && fx.spots, fx && fx.looks);
    return out;
  }
  function cleanRules(list, fx) {
    return (Array.isArray(list) ? list : []).slice(0, LIMITS.rules).map(function (r) { return cleanRule(r, fx); }).filter(Boolean);
  }
  /* What the AI reads and writes: the same rule, keys and all. */
  function lookData(l) {
    if (!l) return null;
    var o = {spots: {}};
    Object.keys(l.spots || {}).forEach(function (k) { var s = l.spots[k], v = {}; if (s.look) v.look = s.look; if (s.move) v.move = s.move; if (v.look || v.move) o.spots[k] = v; });
    if (l.words) o.words = l.words;
    return o;
  }
  function ruleData(r) {
    var o = {when: clone(r.when), then: clone(r.then)};
    if (r.chance && r.chance !== 'always') o.chance = r.chance;
    o.players = PLAYERS[r.players] || 'book';
    if (r.players === 'book' && r.line) o.bookLine = r.line;
    if (r.players !== 'secret' && r.look) o.look = lookData(r.look);
    if (r.draft) o.draft = true;
    return o;
  }

  /* ---------- Is a rule on? ---------- */

  /* A name the calendar doesn't have: the rule stays off and says so. */
  function unknownNames(W, r) {
    var out = [];
    r.when.forEach(function (c) {
      if (c.kind === 'moon' && !moonBy(W, c.moon)) out.push(c.moon || 'a moon');
      else if (c.kind === 'moon') {
        var mo = moonBy(W, c.moon), ok = mo.manual ? STATES.indexOf(c.phase) >= 0 : (PHASES.indexOf(c.phase) >= 0 || c.phase === 'waxing' || c.phase === 'waning');
        if (!ok) out.push(c.phase);
      }
      if (c.kind === 'event' && W.eventNames.indexOf(c.event) < 0) out.push(c.event || 'an event');
      if (c.kind === 'rolled') {
        var ro = rolledBy(W, c.event);
        if (!ro) out.push(c.event || 'a rolled event'); else if (ro.results.length && ro.results.indexOf(c.result) < 0) out.push(c.result || 'a result');
      }
      if (c.kind === 'weather' && W.weatherKinds.indexOf(c.weather) < 0) out.push(c.weather || 'a weather');
      if (c.kind === 'season' && !seasonBy(W, c.season)) out.push(c.season || 'a season');
    });
    return out;
  }
  /* condOn: true or false on the day, or null when it isn't about the day. */
  function condOn(W, c, day) {
    switch (c.kind) {
      case 'moon':
        var mo = moonBy(W, c.moon); if (!mo) return false;
        var at = moonOn(W, mo, day);
        if (mo.manual) return at.state === c.phase;
        if (c.phase === 'waxing') return at.p > 0.0625 && at.p < 0.4375;
        if (c.phase === 'waning') return at.p >= 0.5625 && at.p < 0.9375;
        return at.name === c.phase;
      case 'event': return day.events.indexOf(c.event) >= 0;
      case 'rolled': return day.rolled[c.event] === c.result;
      case 'weather': return day.weather === c.weather;
      case 'season': return day.season === c.season;
      case 'always': return true;
    }
    return DAYLESS[c.kind] ? null : false;
  }
  function ruleOn(W, r, day) {
    if (unknownNames(W, r).length) return false;
    var any = false;
    for (var i = 0; i < r.when.length; i++) { var v = condOn(W, r.when[i], day); if (v === false) return false; if (v === null) any = true; }
    return any ? null : true;
  }
  function handMoon(W, r) {
    for (var i = 0; i < r.when.length; i++) { var c = r.when[i], mo = c.kind === 'moon' && moonBy(W, c.moon); if (mo && mo.manual) return {m: mo, c: c}; }
    return null;
  }
  /* Rules on events, rolls and weather can only be read as far as the
     calendar has them. */
  function needsKnown(r) { return r.when.some(function (c) { return c.kind === 'weather' || c.kind === 'event' || c.kind === 'rolled'; }); }
  function withHand(W, h, fn) {
    if (!h) return fn();
    var save = W.hand; W.hand = {}; Object.keys(save).forEach(function (k) { W.hand[k] = save[k]; });
    W.hand[h.m.name] = h.c.phase;
    try { return fn(); } finally { W.hand = save; }
  }
  /* onDays lists the day numbers in [from, to] a rule is on (or may be on). */
  function onDays(W, r, from, to, assumeHand) {
    var out = [];
    withHand(W, assumeHand ? handMoon(W, r) : null, function () {
      for (var i = from; i <= to; i++) if (ruleOn(W, r, W.day(i)) !== false) out.push(i);
    });
    return out;
  }
  function runs(list) {
    var out = [], s = null, p = null;
    list.forEach(function (d) { if (s !== null && d === p + 1) { p = d; return; } if (s !== null) out.push([s, p]); s = p = d; });
    if (s !== null) out.push([s, p]);
    return out;
  }
  function span(W, a, b) {
    var A = W.day(a), B = W.day(b);
    if (a === b) return A.label;
    if (A.monthName === B.monthName && A.year === B.year) return A.monthName + ' ' + A.day + '–' + B.day;
    return A.label + ' – ' + B.label;
  }
  function dayless(c) {
    switch (c.kind) {
      case 'time': return 'if made at ' + c.time;
      case 'bench': return 'if made at the ' + c.bench;
      case 'ingredient': return 'if an ingredient is ' + c.mark;
      case 'maker': return 'if the maker knows ' + c.skill;
    }
    return '';
  }
  /* Two or more cycling moons in fixed phases may never line up: scanned
     over the horizon, whatever else the rule asks. */
  function neverTogether(W, r) {
    var cm = r.when.filter(function (c) { var mo = c.kind === 'moon' && moonBy(W, c.moon); return mo && !mo.manual; });
    if (cm.length < 2) return false;
    for (var i = 0; i <= W.horizon; i++) {
      var day = W.day(i);
      if (cm.every(function (c) { return condOn(W, c, day); })) return false;
    }
    return true;
  }

  /* status: the chip a rule shows, measured from preview day `at`. */
  function status(W, r, at) {
    at = at || 0;
    var bad = unknownNames(W, r), day = W.day(at), label = at === 0 ? 'On today' : 'On this day';
    if (bad.length) return {cls: 'warn', key: 'unknown', text: '“' + bad[0] + '” isn’t on this calendar'};
    if (neverTogether(W, r)) return {cls: 'warn', key: 'never', text: 'Never happens'};
    var on = ruleOn(W, r, day), h = handMoon(W, r), dl = r.when.filter(function (c) { return DAYLESS[c.kind]; });
    if (on === true) return {cls: 'on', key: 'on', text: label};
    if (on === null) return {cls: 'on', key: 'on', text: label + ', ' + dayless(dl[0])};
    if (h && r.when.every(function (c) { return c === h.c || condOn(W, c, day) !== false; }))
      return {cls: 'hand', key: 'hand', text: 'Waits for you to set ' + h.m.name + ' to ' + h.m.words[h.c.phase]};
    var end = needsKnown(r) ? W.knownTo : Math.min(W.horizon, at + W.horizon), next = onDays(W, r, at + 1, end, !!h);
    if (next.length) {
      var run = runs(next)[0], n = run[0] - at;
      return {cls: h ? 'hand' : '', key: 'next', at: run[0], text: 'Next: ' + span(W, run[0], run[1]) + ' · in ' + n + (n === 1 ? ' day' : ' days') +
        (h ? ', if ' + h.m.name + ' is ' + h.m.words[h.c.phase] : '')};
    }
    if (needsKnown(r)) {
      /* The weather may well come, just never on a day the rest holds. */
      var w = r.when.filter(function (c) { return c.kind === 'weather' && !onDays(W, {when: [c]}, at + 1, W.knownTo, false).length; })[0];
      return {cls: '', key: 'unknown-ahead', text: (w ? 'No ' + w.weather + ' in the weather' : r.when.length > 1 ? 'Doesn’t line up' : 'Not on the calendar') + ' before ' + W.day(W.knownTo).label};
    }
    return {cls: 'warn', key: 'never', text: 'Never happens'};
  }

  /* verdict: can the recipe be made on a day, and what applies. Drafts wait
     for approval and count for nothing. "Can't be made" wins outright when
     it is certain; with any "only then" rule, at least one must be on; an
     edge and a bane cancel, as in any Draw Steel roll. */
  function verdict(W, rules, at) {
    var day = W.day(at || 0), act = [], blockers = [], only = [], onlyOn = [];
    rules.forEach(function (r, i) {
      if (r.draft) return;
      var v = ruleOn(W, r, day);
      if (r.then.kind === 'only') { only.push(i); if (v !== false) onlyOn.push(i); return; }
      if (v === false) return;
      if (r.then.kind === 'cannot') { blockers.push({i: i, r: r, maybe: v === null || r.chance !== 'always'}); return; }
      act.push({i: i, r: r, maybe: v === null});
    });
    var hard = blockers.filter(function (b) { return !b.maybe; });
    var edges = act.filter(function (a) { return a.r.then.kind === 'edge' && !a.maybe && a.r.chance === 'always'; }).length;
    var banes = act.filter(function (a) { return a.r.then.kind === 'bane' && !a.maybe && a.r.chance === 'always'; }).length;
    return {can: !hard.length && (!only.length || onlyOn.length > 0), blockers: blockers, only: only, onlyOn: onlyOn, act: act,
      roll: edges > banes ? 'edge' : banes > edges ? 'bane' : 'plain'};
  }
  /* A rule whose every day is also a certain "can't be made" day never
     takes effect. Checked over the days the calendar can tell. */
  function cancelledBy(W, rules, i) {
    var r = rules[i];
    if (r.draft || r.then.kind === 'cannot' || r.then.kind === 'only') return -1;
    var end = needsKnown(r) ? W.knownTo : Math.min(W.horizon, 120), mine = onDays(W, r, 0, end, true);
    if (!mine.length) return -1;
    for (var j = 0; j < rules.length; j++) {
      var b = rules[j];
      if (j === i || b.draft || b.then.kind !== 'cannot' || b.chance !== 'always' || b.when.some(function (c) { return DAYLESS[c.kind]; })) continue;
      var theirs = onDays(W, b, 0, end, false);
      if (mine.every(function (d) { return theirs.indexOf(d) >= 0; })) return j;
    }
    return -1;
  }

  /* ---------- Words: every rule reads as one sentence ---------- */
  function condText(W, c) {
    switch (c.kind) {
      case 'moon': return '<b>' + esc(c.moon) + '</b> is <b>' + esc(phaseWord(W, c)) + '</b>';
      case 'event': return 'the calendar shows <b>' + esc(c.event) + '</b>';
      case 'rolled': return 'the <b>' + esc(c.event) + '</b> comes up <b>' + esc(c.result) + '</b>';
      case 'weather': return 'the weather is <b>' + esc(c.weather) + '</b>';
      case 'season': return 'it is <b>' + esc(c.season) + '</b>';
      case 'time': return 'it is made at <b>' + esc(c.time) + '</b>';
      case 'bench': return 'it is made at the <b>' + esc(c.bench) + '</b>';
      case 'ingredient': return 'an ingredient is <b>' + esc(c.mark) + '</b>';
      case 'maker': return 'the maker knows <b>' + esc(c.skill) + '</b>';
    }
    return '';
  }
  function thenText(t) {
    switch (t.kind) {
      case 'only': return 'it can <b>only be made then</b>';
      case 'cannot': return 'it <b>can’t be made</b>';
      case 'edge': return 'the project roll has an <b>edge</b>';
      case 'bane': return 'the project roll has a <b>bane</b>';
      case 'points': var n = +t.amount || 0; return n < 0 ? 'it loses <b>' + (-n) + ' project points</b>' : 'it gains <b>' + n + ' project points</b>';
      case 'extra': return 'it makes <b>' + (t.amount > 1 ? t.amount + ' extra' : 'one extra') + '</b>';
      case 'fine': return 'it comes out <b>fine</b>';
      case 'becomes': return 'it becomes <b>' + esc(t.item) + '</b>';
      case 'mark': return 'it is marked <b>' + esc(t.mark) + '</b>';
      case 'quirk': return 'it gets a <b>quirk</b> from <b>' + esc(t.table) + '</b>';
      case 'mishap': return 'a <b>mishap</b> from <b>' + esc(t.table) + '</b> strikes mid-brew';
      case 'discover': return 'the maker <b>discovers ' + esc(t.recipe) + '</b>';
      case 'say': return 'the bench says <b>“' + esc(t.text) + '”</b>';
    }
    return '';
  }
  function sentence(W, r) {
    var t = thenText(r.then);
    if (r.chance && r.chance !== 'always') t = 'there’s a <b>' + esc(r.chance) + '</b> chance ' + t;
    if (r.when.length === 1 && r.when[0].kind === 'always') return 'Every time, ' + t + '.';
    return ('When ' + r.when.map(function (c) { return condText(W, c); }).join(' and ') + ', ' + t + '.').replace(/([.!?])”<\/b>\.$/, '$1”</b>');
  }
  function whenTitle(W, c) {
    switch (c.kind) {
      case 'moon': return c.moon + ' is ' + phaseWord(W, c);
      case 'event': return c.event;
      case 'rolled': return c.result + ' comes up';
      case 'weather': return c.weather;
      case 'season': return c.season;
      case 'time': return 'At ' + c.time;
      case 'bench': return 'At the ' + c.bench;
      case 'ingredient': return 'An ingredient is ' + c.mark;
      case 'maker': return 'Maker knows ' + c.skill;
    }
    return 'Every brew';
  }
  function thenTitle(t) {
    switch (t.kind) {
      case 'only': return 'Only then';
      case 'cannot': return 'Can’t be made';
      case 'edge': return 'Edge on the roll';
      case 'bane': return 'Bane on the roll';
      case 'points': return ((+t.amount || 0) < 0 ? '' : '+') + (+t.amount || 0) + ' project points';
      case 'extra': return t.amount > 1 ? t.amount + ' extra' : 'One extra';
      case 'fine': return 'Comes out fine';
      case 'becomes': return 'Becomes ' + t.item;
      case 'mark': return 'Marked ' + t.mark;
      case 'quirk': return 'A quirk';
      case 'mishap': return 'A mishap';
      case 'discover': return 'Discovers ' + t.recipe;
      case 'say': return 'The bench speaks';
    }
    return '';
  }
  /* One tone per kind of outcome, so a glance says what a rule does. */
  var TONE = {only: 'only', cannot: 'block', edge: 'help', bane: 'risk', extra: 'help', fine: 'help', becomes: 'change', mark: 'change',
    quirk: 'risk', mishap: 'risk', discover: 'change', say: 'change'};
  function tone(t) { return t.kind === 'points' ? ((+t.amount || 0) < 0 ? 'risk' : 'help') : (TONE[t.kind] || 'change'); }
  /* Effects that put something on the cloth when they fire. */
  function makes(t) { return t.kind === 'becomes' || t.kind === 'extra' || t.kind === 'fine' || t.kind === 'mark'; }

  /* ---------- Reading what the calendar API sends ---------- */

  /* flattenEvents turns one month's event read into one entry per day an
     event falls on: a repeating event's own dates (the server has already
     applied its skips and moves), or a one-off event's start date. */
  function flattenEvents(list, year, month) {
    var out = [];
    (Array.isArray(list) ? list : (list && list.data) || []).forEach(function (e) {
      if (!e || !e.name) return;
      if (Array.isArray(e.occurrences)) {
        e.occurrences.forEach(function (o) { if (!o.skipped && o.year === year && o.month === month) out.push({name: e.name, year: o.year, month: o.month, day: o.day}); });
      } else if (e.year === year && e.month === month) out.push({name: e.name, year: e.year, month: e.month, day: e.day});
    });
    return out;
  }
  /* flattenRolls turns one month's rolls ({eventID: [{year, month, day,
     result}]}) into entries named by the event, using names {id: name}. */
  function flattenRolls(map, names) {
    var out = [];
    Object.keys(map || {}).forEach(function (id) {
      var n = names[id]; if (!n || !Array.isArray(map[id])) return;
      map[id].forEach(function (r) { if (r && r.result) out.push({event: n, result: r.result, year: r.year, month: r.month, day: r.day}); });
    });
    return out;
  }

  /* ---------- Starting points, drawn from the calendar ---------- */

  function cycling(W) { return W.moons.filter(function (m) { return !m.manual; }); }
  function first(a) { return a && a.length ? a[0] : ''; }
  /* defCond and defThen fill a new condition or effect with the first name
     the calendar or campaign has, so a fresh rule is never pointed at
     something that isn't there. */
  function defCond(W, k) {
    var L = W.lists;
    switch (k) {
      case 'moon': var m = cycling(W)[0] || W.moons[0]; return {kind: 'moon', moon: m ? m.name : '', phase: 'full'};
      case 'event': return {kind: 'event', event: first(W.eventNames)};
      case 'rolled': var r = W.rolled[0]; return {kind: 'rolled', event: r ? r.name : '', result: r ? first(r.results) : ''};
      case 'weather': return {kind: 'weather', weather: W.forecastKinds.filter(function (w) { return w !== 'Clear'; })[0] || 'Rain'};
      case 'season': return {kind: 'season', season: W.seasons[0] ? W.seasons[0].name : ''};
      case 'time': return {kind: 'time', time: 'night'};
      case 'bench': return {kind: 'bench', bench: first(L.benches)};
      case 'ingredient': return {kind: 'ingredient', mark: first(L.marks)};
      case 'maker': return {kind: 'maker', skill: first(L.skills)};
    }
    return {kind: 'always'};
  }
  function defThen(W, k) {
    var L = W.lists;
    switch (k) {
      case 'points': return {kind: k, amount: 2};
      case 'extra': return {kind: k, amount: 1};
      case 'becomes': return {kind: k, item: first(L.items)};
      case 'mark': return {kind: k, mark: first(L.marks)};
      case 'quirk': case 'mishap': return {kind: k, table: first(L.tables)};
      case 'discover': return {kind: k, recipe: first(L.recipes)};
      case 'say': return {kind: k, text: 'The candle flame leans toward the window.'};
    }
    return {kind: k};
  }
  /* A new rule's look matches what it does; the Director can change it. */
  function defLook(W, r) {
    var t = tone(r.then);
    var spots = t === 'help' ? {window: {look: 'dawn'}, work: {look: 'bloom'}} : t === 'risk' ? {work: {look: 'rot'}} :
      t === 'block' ? {window: {look: 'frost'}, heat: {move: 'dim'}} : {result: {look: 'glass'}};
    return {spots: spots, words: r.then.kind === 'cannot' ? 'Not tonight' : whenTitle(W, r.when[0]).slice(0, LIMITS.words)};
  }
  function rule(when, then, chance, players) { return {when: when, then: then, chance: chance, players: players, line: '', look: null, draft: false}; }
  /* starters are the common rules, written in this calendar's own names;
     one the calendar or campaign can't fill is left out. */
  function starters(W) {
    var L = W.lists, cm = cycling(W), hand = W.moons.filter(function (m) { return m.manual; })[0], out = [];
    if (cm[0]) out.push({key: 'better', title: 'Better under a full moon', note: 'The roll gets an edge while ' + cm[0].name + ' is full.',
      rule: rule([{kind: 'moon', moon: cm[0].name, phase: 'full'}], {kind: 'edge'}, 'always', 'book')});
    if (cm.length) { var m2 = cm[1] || cm[0]; out.push({key: 'not', title: 'Not under a full moon', note: 'It can’t be made while ' + m2.name + ' is full.',
      rule: rule([{kind: 'moon', moon: m2.name, phase: cm[1] ? 'full' : 'new'}], {kind: 'cannot'}, 'always', 'book')}); }
    out.push({key: 'night', title: 'Only at night', note: 'It can be made only after dark.', rule: rule([{kind: 'time', time: 'night'}], {kind: 'only'}, 'always', 'book')});
    out.push({key: 'weather', title: 'Weather gives extra', note: 'Strange weather sometimes makes one more.', rule: rule([defCond(W, 'weather')], {kind: 'extra', amount: 1}, '1 in 6', 'shown')});
    if (W.eventNames.length && L.recipes && L.recipes.length) out.push({key: 'event', title: 'An event teaches a recipe', note: 'A rare day lets the maker discover something.',
      rule: rule([defCond(W, 'event')], defThen(W, 'discover'), '1 in 2', 'secret')});
    if (W.rolled.length && L.items && L.items.length) out.push({key: 'rolled', title: 'A rolled event changes it', note: 'What the calendar rolls changes what comes out.',
      rule: rule([defCond(W, 'rolled')], defThen(W, 'becomes'), 'always', 'shown')});
    if ((hand || cm[0]) && L.tables && L.tables.length) out.push({key: 'quirk', title: 'A hidden quirk', note: 'Under a moon, it sometimes comes out strange.',
      rule: rule([{kind: 'moon', moon: (hand || cm[0]).name, phase: hand ? 'full' : 'new'}], defThen(W, 'quirk'), '1 in 4', 'secret')});
    return out;
  }

  /* ---------- Talking to the Director's AI ---------- */

  /* aiBrief is what "Copy for your AI" hands over: every name this calendar
     and campaign has, the shapes a rule may take, and the recipe's rules as
     they stand, so the AI writes rules that can only name real things. */
  function aiBrief(W, recipe, rules, fx) {
    var looks = fx && fx.looks ? Object.keys(fx.looks) : [];
    return {
      recipe: recipe,
      howRulesWork: 'Each rule has "when" (1 to 3 conditions, all must be true), "then" (one effect), an optional "chance" and "players". ' +
        'Use only the names listed under "calendar" and "campaign". Rules you send arrive as drafts the Director approves one by one.',
      calendar: {
        moons: W.moons.map(function (m) { return m.manual ? {name: m.name, setByHand: true, states: STATES.slice(), stateWords: m.words} : {name: m.name}; }),
        moonPhases: PHASES.concat(['waxing', 'waning']),
        events: W.eventNames.slice(0, 80), rolledEvents: W.rolled.map(function (r) { return {name: r.name, results: r.results}; }),
        weather: W.weatherKinds.slice(), seasons: W.seasons.map(function (s) { return s.name; }), timesOfDay: TIMES.slice()
      },
      campaign: {benches: W.lists.benches || [], ingredientMarks: W.lists.marks || [], skills: W.lists.skills || [], items: W.lists.items || [],
        recipes: W.lists.recipes || [], tables: W.lists.tables || []},
      conditions: [{kind: 'moon', moon: 'a moon', phase: 'a phase, or a state for a moon set by hand'}, {kind: 'event', event: 'an event'},
        {kind: 'rolled', event: 'a rolled event', result: 'one of its results'}, {kind: 'weather', weather: 'a weather'}, {kind: 'season', season: 'a season'},
        {kind: 'time', time: 'dawn, day, dusk or night'}, {kind: 'bench', bench: 'a bench'}, {kind: 'ingredient', mark: 'an ingredient mark'},
        {kind: 'maker', skill: 'a skill'}, {kind: 'always'}],
      effects: [{kind: 'only'}, {kind: 'cannot'}, {kind: 'edge'}, {kind: 'bane'}, {kind: 'points', amount: '-10 to 10'}, {kind: 'extra', amount: '1 to 3'},
        {kind: 'fine'}, {kind: 'becomes', item: 'an item'}, {kind: 'mark', mark: 'a mark'}, {kind: 'quirk', table: 'a table'}, {kind: 'mishap', table: 'a table'},
        {kind: 'discover', recipe: 'a recipe'}, {kind: 'say', text: 'up to ' + LIMITS.line + ' characters'}],
      chances: CHANCES.slice(), players: ['book', 'whenItHappens', 'secret'], bookLine: 'optional, for players "book": the line the recipe book prints',
      look: {about: 'What players see when the rule fires, for players "book" or "whenItHappens".', spots: (fx && fx.spots) || [], looks: looks,
        moves: Object.keys(MOVES), words: 'up to ' + LIMITS.words + ' characters', example: {spots: {window: {look: looks[0] || 'dawn'}, heat: {move: 'dim'}}, words: 'Not tonight'}},
      rules: rules.map(ruleData)
    };
  }
  /* ---------- A recipe's "If it goes wrong" and "Extras" settings ----------
     Forgiving like the rules: an unknown value falls back to the setting's
     default, and a setting that is off is null. The server cleans the same
     way (internal/plugins/crafting/extras.go);
     test/js/fixtures/craft_extras_clean.json pins both to one answer. */
  var EXTRAS = {
    botch: ['bane', 'baneAndQuirk', 'baneAndIngredients'],
    fine: ['edge', 'points', 'extra', 'none'], poor: ['bane', 'points', 'none'], subCost: ['bane', 'points', 'none'],
    after: [2, 3, 5, 10], gives: ['skip', 'edge', 'points'], learnedBy: ['maker', 'party'], failedTry: ['used', 'half', 'kept'],
    tags: ['metal', 'remnant_part', 'body_part', 'bone', 'hide', 'liquid', 'herb', 'magic_item', 'magic_essence'],
    limits: {subs: 8, salvage: 4, days: 360, hint: 240, mishapMoments: 20}
  };
  function obj(v) { return v && typeof v === 'object' && !Array.isArray(v) ? v : null; }
  function pick(v, opts) { return opts.indexOf(v) >= 0 ? v : opts[0]; }
  // num is a JSON number or numeric string, rounded; NaN for anything else.
  function num(v) {
    if (typeof v === 'boolean') return v ? 1 : 0;
    if (typeof v === 'string') { if (!/^\s*[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?\s*$/.test(v)) return NaN; v = +v; }
    if (typeof v !== 'number' || !isFinite(v)) return NaN;
    return Math.floor(v + 0.5);
  }
  function clampN(n, lo, hi) { return n < lo ? lo : n > hi ? hi : n; }
  function defaultExtras() {
    return {botch: {costs: 'bane'}, mishaps: null, quality: null, substitutes: [], breakthrough: false, mastery: null, discover: null,
      keeps: null, makersMark: false, salvage: [], othersCanHelp: false};
  }
  /* cleanExtras reads a recipe's extras as the AI or the editor wrote them;
     materials is how many materials the recipe has. */
  function cleanExtras(raw, materials) {
    var out = defaultExtras(), m = obj(raw), x, i;
    if (!m) return out;
    if ((x = obj(m.botch))) {
      if (x.costs === 'baneAndQuirk') { var qt = str(x.quirkTable); out.botch = qt ? {costs: x.costs, quirkTable: qt} : {costs: 'bane'}; }
      else if (x.costs === 'baneAndIngredients') out.botch = {costs: x.costs};
    }
    if ((x = obj(m.mishaps)) && str(x.table)) {
      var c = x.chance;
      if (c === 'always' || CHANCES.indexOf(c) < 0) c = '1 in 6';
      out.mishaps = {chance: c, table: str(x.table)};
    }
    if ((x = obj(m.quality))) out.quality = {fine: pick(x.fine, EXTRAS.fine), poor: pick(x.poor, EXTRAS.poor)};
    if (Array.isArray(m.substitutes)) {
      var seen = {};
      m.substitutes.forEach(function (v) {
        var s = obj(v);
        if (!s || out.substitutes.length >= EXTRAS.limits.subs) return;
        var f = num(s['for']);
        if (!isFinite(f) || f < 0 || f >= materials || seen[f] || EXTRAS.tags.indexOf(s.use) < 0) return;
        seen[f] = true;
        out.substitutes.push({'for': f, use: s.use, costs: pick(s.costs, EXTRAS.subCost)});
      });
    }
    out.breakthrough = m.breakthrough === true;
    if ((x = obj(m.mastery))) {
      var n = num(x.afterCleanBrews);
      out.mastery = {afterCleanBrews: EXTRAS.after.indexOf(n) >= 0 ? n : 3, gives: pick(x.gives, EXTRAS.gives)};
    }
    if ((x = obj(m.discover))) {
      var close = typeof x.closeWhen === 'string' ? x.closeWhen : '', ok = false;
      for (i = 1; i < materials; i++) if (close === i + ' of ' + materials) ok = true;
      if (!ok) close = materials > 1 ? '1 of ' + materials : '';
      out.discover = {closeWhen: close, learnedBy: pick(x.learnedBy, EXTRAS.learnedBy), failedTry: pick(x.failedTry, EXTRAS.failedTry)};
      var hint = str(x.hint, EXTRAS.limits.hint);
      if (hint) out.discover.hint = hint;
    }
    if ((x = obj(m.keeps))) {
      var d = num(x.days);
      out.keeps = {days: clampN(isFinite(d) ? d : 30, 1, EXTRAS.limits.days), thenBecomes: str(x.thenBecomes)};
    }
    out.makersMark = m.makersMark === true;
    if (Array.isArray(m.salvage)) {
      m.salvage.forEach(function (v) {
        var s = obj(v), item = s && str(s.item);
        if (!item || out.salvage.length >= EXTRAS.limits.salvage) return;
        var q = num(s.qty);
        out.salvage.push({item: item, qty: clampN(isFinite(q) ? q : 1, 1, 2)});
      });
    }
    out.othersCanHelp = m.othersCanHelp === true;
    return out;
  }

  /* parseAI reads what the Director pasted from their AI: one rule, a list
     of rules, or {"rules": [...]}, with or without a code fence. Every rule
     is cleaned and comes back as a draft; the count of ones that could not
     be read is kept so the page can say so. */
  function parseAI(text, fx) {
    var src = String(text || '').trim().replace(/^```[a-z]*\s*/i, '').replace(/\s*```$/, ''), data;
    if (!src) return {rules: [], dropped: 0, error: 'empty'};
    try { data = JSON.parse(src); } catch (e) { return {rules: [], dropped: 0, error: 'json'}; }
    var list = Array.isArray(data) ? data : data && Array.isArray(data.rules) ? data.rules : data && typeof data === 'object' ? [data] : [];
    var rules = [], dropped = 0;
    list.slice(0, LIMITS.rules).forEach(function (r) { var c = cleanRule(r, fx); if (c) { c.draft = true; rules.push(c); } else dropped++; });
    dropped += Math.max(0, list.length - LIMITS.rules);
    return {rules: rules, dropped: dropped, error: list.length ? '' : 'none'};
  }

  window.CraftRules = {
    KINDS: KINDS, EFFECTS: EFFECTS, CHANCES: CHANCES, PLAYERS: PLAYERS, PHASES: PHASES, STATES: STATES, TIMES: TIMES, MOVES: MOVES,
    WEATHER: WEATHER, WEATHER_TABLE: WEATHER_TABLE, GLYPHS: GLYPHS, DAYLESS: DAYLESS, LIMITS: LIMITS,
    esc: esc, clone: clone, phaseName: phaseName, world: world, moonBy: moonBy, rolledBy: rolledBy, moonOn: moonOn, phaseWord: phaseWord,
    cleanCond: cleanCond, cleanThen: cleanThen, cleanLook: cleanLook, cleanRule: cleanRule, cleanRules: cleanRules, ruleData: ruleData, lookData: lookData,
    unknownNames: unknownNames, condOn: condOn, ruleOn: ruleOn, handMoon: handMoon, onDays: onDays, runs: runs, span: span, dayless: dayless,
    neverTogether: neverTogether, status: status, verdict: verdict, cancelledBy: cancelledBy,
    sentence: sentence, whenTitle: whenTitle, thenTitle: thenTitle, tone: tone, makes: makes,
    flattenEvents: flattenEvents, flattenRolls: flattenRolls, defCond: defCond, defThen: defThen, defLook: defLook, starters: starters,
    aiBrief: aiBrief, parseAI: parseAI, EXTRAS: EXTRAS, defaultExtras: defaultExtras, cleanExtras: cleanExtras
  };
})();
