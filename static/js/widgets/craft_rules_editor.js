/**
 * craft_rules_editor.js -- the "When it can be made" tab of the crafting rules editor
 *
 * A strip of the calendar's next days shows when each rule is on and
 * whether the recipe can be made; below it every rule reads When → Then,
 * and opens in place into pickers made of pictures. New rules start from
 * common ones written in the calendar's own names, and what the Director's
 * AI writes arrives as drafts to approve. All meaning comes from CraftRules
 * (craft_rules_logic.js); this file only draws and listens. The widget
 * (craft_rules.js) hands it the shared state. Exposes window.CraftRulesEditor.
 */
(function () {
  'use strict';
  var R = window.CraftRules;
  if (!R) return;
  var esc = R.esc;

  /* ---------- Pictures ---------- */
  var FA = {moon: 'moon', event: 'calendar-day', rolled: 'dice', weather: 'cloud', season: 'leaf', time: 'clock', bench: 'flask', ingredient: 'gem',
    maker: 'user', always: 'infinity', only: 'calendar-check', cannot: 'ban', edge: 'angles-up', bane: 'angles-down', points: 'circle-plus',
    extra: 'clone', fine: 'star', becomes: 'right-left', mark: 'tag', quirk: 'circle-question', mishap: 'burst', discover: 'lightbulb', say: 'comment',
    book: 'book-open', shown: 'eye', secret: 'lock'};
  var GLYPH = {clear: 'sun', cloud: 'cloud', rain: 'cloud-rain', snow: 'snowflake', storm: 'cloud-bolt', fog: 'smog'};
  var TIME = {dawn: 'cloud-sun', day: 'sun', dusk: 'cloud-moon', night: 'moon'};
  function fa(name, cls) { return '<i class="fa-solid fa-' + name + (cls ? ' ' + cls : '') + '" aria-hidden="true"></i>'; }
  function ico(k, cls) { return fa(FA[k] || 'circle', cls); }
  function wxIcon(W, label, cls) { return fa(GLYPH[W.glyphs[label]] || 'cloud', cls); }
  /* A moon drawn at its phase (0 new, .5 full) in its own colour: a picture
     of the calendar's moon, as the calendar page draws it, not an icon. */
  var PHASE_P = {'new': 0, 'waxing crescent': 0.125, 'first quarter': 0.25, 'waxing gibbous': 0.375, full: 0.5, 'waning gibbous': 0.625,
    'last quarter': 0.75, 'waning crescent': 0.875, waxing: 0.3, waning: 0.7};
  var HAND_P = {full: 0.5, half: 0.25, crescent: 0.125, dark: 0, vanished: 0};
  function moonAt(color, p, size, gone) {
    var r = 8.5, c = 12, k = Math.cos(2 * Math.PI * p), rx = (Math.abs(k) * r).toFixed(2), wax = p < 0.5, lit = '';
    if (!gone && p > 0.001 && p < 0.999) {
      lit = Math.abs(p - 0.5) < 0.001 ? '<circle cx="12" cy="12" r="' + r + '" fill="' + color + '"/>' :
        '<path d="M12 ' + (c - r) + 'A' + r + ' ' + r + ' 0 0 ' + (wax ? 1 : 0) + ' 12 ' + (c + r) + 'A' + rx + ' ' + r + ' 0 0 ' + (k > 0 ? (wax ? 0 : 1) : (wax ? 1 : 0)) + ' 12 ' + (c - r) + 'Z" fill="' + color + '"/>';
    }
    /* The faint outer ring keeps a pale moon visible on a light page. */
    return '<svg class="crr-moon" width="' + size + '" height="' + size + '" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9.9" fill="none" stroke="rgb(0 0 0 / .2)" stroke-width=".8"/><circle cx="12" cy="12" r="' + r +
      '" fill="rgb(0 0 0 / .22)" stroke="' + color + '" stroke-width="1.4"' + (gone ? ' stroke-dasharray="2.5 2.5" fill-opacity="0"' : '') + '/>' + lit + '</svg>';
  }
  function moonPic(W, name, phase, size) {
    var m = R.moonBy(W, name);
    if (!m) return fa('moon');
    return m.manual ? moonAt(m.color, HAND_P[phase] || 0, size, phase === 'vanished') : moonAt(m.color, PHASE_P[phase] || 0, size);
  }
  function moonDay(W, m, day, size) {
    var at = R.moonOn(W, m, day);
    return moonAt(m.color, at.p, size, at.gone);
  }
  function condPic(W, c, size) {
    switch (c.kind) {
      case 'moon': return moonPic(W, c.moon, c.phase, size);
      case 'weather': return wxIcon(W, c.weather);
      case 'time': return fa(TIME[c.time] || 'clock');
    }
    return ico(c.kind);
  }

  /* ---------- Words ---------- */
  var KNAME = {}; R.KINDS.forEach(function (k) { KNAME[k[0]] = k[1]; });
  var VIS = {book: ['Written in the book', 'Always on the recipe page'], shown: ['Shown when it happens', 'A moment on the bench when it fires'],
    secret: ['Only you', 'Players never see it']};
  function cap(s) { return s.charAt(0).toUpperCase() + s.slice(1); }
  function days(n) { return n + (n === 1 ? ' day' : ' days'); }
  function whenSub(c) { return c.kind === 'rolled' ? c.event : KNAME[c.kind]; }
  function thenSub(r) {
    var c = r.chance && r.chance !== 'always' ? r.chance + ' chance' : 'Every time';
    if (r.then.kind === 'quirk' || r.then.kind === 'mishap') return c + ' · ' + r.then.table;
    if (r.then.kind === 'only') return 'On no other day';
    return c;
  }
  function tile(pic, small, big, sub, tone) {
    return '<span class="crr-tile crr-' + tone + '"><span class="crr-ti">' + pic + '</span><span class="crr-tt"><small>' + esc(small) + '</small><b>' + esc(big) + '</b>' +
      (sub ? '<span>' + esc(sub) + '</span>' : '') + '</span></span>';
  }
  function whenTile(W, c, firstOne) { return tile(condPic(W, c, 24), firstOne ? 'When' : 'And', R.whenTitle(W, c), whenSub(c), 'when'); }
  function thenTile(r) { return tile(ico(r.then.kind), 'Then', R.thenTitle(r.then), thenSub(r), R.tone(r.then)); }

  /* What the AI writes: the same data, keys and all. Small objects stay on
     one line so a rule reads at a glance. */
  function inline(v) {
    if (Array.isArray(v)) return '[' + v.map(inline).join(', ') + ']';
    if (v && typeof v === 'object') return '{ ' + Object.keys(v).map(function (k) { return JSON.stringify(k) + ': ' + inline(v[k]); }).join(', ') + ' }';
    return JSON.stringify(v);
  }
  function fmt(v, ind) {
    var s = inline(v);
    if (s.length + ind * 2 <= 66 || v === null || typeof v !== 'object') return s;
    var pad = new Array(ind + 2).join('  '), end = new Array(ind + 1).join('  ');
    if (Array.isArray(v)) return '[\n' + v.map(function (x) { return pad + fmt(x, ind + 1); }).join(',\n') + '\n' + end + ']';
    return '{\n' + Object.keys(v).map(function (k) { return pad + JSON.stringify(k) + ': ' + fmt(v[k], ind + 1); }).join(',\n') + '\n' + end + '}';
  }
  function hl(o) {
    return fmt(o, 0).replace(/("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?)/g, function (m, s, colon, lit, num) {
      if (s) return colon ? '<span class="k">' + esc(s) + '</span>' + colon : '<span class="s">' + esc(s) + '</span>';
      return '<span class="n">' + (lit || num) + '</span>';
    });
  }

  /* ---------- The calendar strip ---------- */
  function cell(S, i, inner, cls) {
    return '<div class="crr-tl-cell' + (cls ? ' ' + cls : '') + (i === 0 ? ' is-today' : '') + (i === S.at ? ' is-sel' : '') + '" data-d="' + i + '">' + (inner || '') + '</div>';
  }
  function lab(pic, name, cls) { return '<div class="crr-tl-lab' + (cls ? ' ' + cls : '') + '">' + (pic || '') + '<b>' + esc(name) + '</b></div>'; }
  function note(S, text, cls) { return '<div class="crr-tl-note' + (cls ? ' ' + cls : '') + '" style="grid-column:span ' + S.W.span + '">' + text + '</div>'; }
  function bars(S, r, tone) {
    var W = S.W, N = W.span, on = R.onDays(W, r, 0, N - 1, false), set = {}, h = '', maybe = r.chance && r.chance !== 'always';
    on.forEach(function (d) { set[d] = R.ruleOn(W, r, W.day(d)); });
    for (var i = 0; i < N; i++) {
      if (!(i in set)) { h += cell(S, i); continue; }
      h += cell(S, i, '<i class="crr-bar crr-' + tone + ((i - 1) in set ? '' : ' start') + ((i + 1) in set ? '' : ' end') + (maybe || set[i] === null ? ' maybe' : '') +
        '" style="animation-delay:' + (i * 12) + 'ms"></i>');
    }
    return h;
  }
  function timeline(S) {
    var W = S.W, N = W.span, rec = S.rec(), h = '<div class="crr-tl-scroll"><div class="crr-tl-grid" style="--n:' + N + '">', i, m = null, run = 0;
    h += '<div class="crr-tl-lab crr-tl-corner"></div>';
    for (i = 0; i <= N; i++) {
      var mn = i < N ? W.day(i).monthName : null;
      if (mn !== m || i === N) { if (run) h += '<div class="crr-tl-mon" style="grid-column:span ' + run + '">' + esc(m) + '</div>'; m = mn; run = 0; }
      run++;
    }
    h += lab('', 'Day');
    for (i = 0; i < N; i++) {
      var d = W.day(i);
      h += '<button type="button" class="crr-tl-day' + (i === 0 ? ' is-today' : '') + (i === S.at ? ' is-sel' : '') + '" data-d="' + i + '" tabindex="' + (i === S.at ? 0 : -1) +
        '" aria-pressed="' + (i === S.at) + '" aria-label="' + esc(d.label + (i === 0 ? ', today' : ', in ' + days(i))) + '">' + d.day + '</button>';
    }
    W.moons.forEach(function (mo) {
      if (mo.manual) {
        var st = W.hand[mo.name];
        h += lab(moonAt(mo.color, HAND_P[st] || 0, 16, st === 'vanished'), mo.name) + note(S, 'You set ' + esc(mo.name) + ' by hand. The preview has it as ' +
          '<select class="crr-in crr-sm" data-act="hand" data-moon="' + esc(mo.name) + '" aria-label="Preview ' + esc(mo.name) + ' as">' +
          R.STATES.map(function (s) { return '<option value="' + s + '"' + (s === st ? ' selected' : '') + '>' + esc(mo.words[s]) + '</option>'; }).join('') + '</select>', 'crr-hand');
        return;
      }
      h += lab(moonAt(mo.color, 0.5, 16), mo.name);
      for (var j = 0; j < N; j++) h += cell(S, j, '<span title="' + esc(mo.name + ': ' + R.moonOn(W, mo, W.day(j)).name) + '">' + moonDay(W, mo, W.day(j), 16) + '</span>', 'crr-ico');
    });
    h += lab(fa('cloud'), 'Weather');
    for (i = 0; i < N; i++) { var wx = W.day(i).weather; h += cell(S, i, wx ? '<span title="' + esc(wx) + '">' + wxIcon(W, wx) + '</span>' : '', 'crr-ico'); }
    h += lab(ico('event'), 'Events');
    for (i = 0; i < N; i++) {
      var dy = W.day(i), ev = dy.events, ro = Object.keys(dy.rolled);
      h += cell(S, i, ev.length ? '<span title="' + esc(ev.join(', ')) + '">' + fa('star') + '</span>' : ro.length ? '<span title="' + esc(ro.map(function (k) { return k + ': ' + dy.rolled[k]; }).join(', ')) + '">' + ico('rolled') + '</span>' : '', 'crr-ico');
    }
    h += '<div class="crr-tl-sep"></div>';
    rec.rules.forEach(function (r, k) {
      var tone = R.tone(r.then), st = R.status(W, r, 0), hm = R.handMoon(W, r);
      h += '<button type="button" class="crr-tl-lab crr-tl-rule' + (r.draft ? ' is-draft' : '') + '" data-goto="' + k + '"><span class="crr-rn crr-' + tone + '">' + (k + 1) + '</span><b>' + esc(R.thenTitle(r.then)) + '</b></button>';
      if (st.key === 'unknown' || st.key === 'never') { h += note(S, esc(st.key === 'never' ? 'Never happens on this calendar' : st.text), 'crr-warnrow'); return; }
      if (hm && W.hand[hm.m.name] !== hm.c.phase) { h += note(S, 'Waits for you to set ' + esc(hm.m.name) + ' to ' + esc(hm.m.words[hm.c.phase]), 'crr-hand'); return; }
      if (!R.onDays(W, r, 0, N - 1, false).length) { h += note(S, esc(st.text), 'crr-quietrow'); return; }
      h += bars(S, r, tone);
    });
    if (!rec.rules.length) h += lab('', 'No rules yet') + note(S, 'Add one below. Until then it brews as written, any day.', 'crr-quietrow');
    h += '<div class="crr-tl-sep"></div>' + lab(fa('check'), 'Can it be made?', 'crr-tl-can');
    for (i = 0; i < N; i++) {
      var v = R.verdict(W, rec.rules, i);
      h += cell(S, i, v.can ? '<span class="crr-yes" title="Can be made">' + fa('check') + '</span>' : '<span class="crr-no" title="Can’t be made">' + fa('xmark') + '</span>', 'crr-ico');
    }
    h += '</div></div><div class="crr-tl-foot"><span class="crr-key crr-help"><i></i>Helps</span><span class="crr-key crr-risk"><i></i>Risky</span><span class="crr-key crr-block"><i></i>Stops it</span>' +
      '<span class="crr-key crr-change"><i></i>Changes what comes out</span><span class="crr-key crr-only"><i></i>Only then</span><span class="crr-key crr-help"><i class="m"></i>A chance, not certain</span>' +
      '<span class="crr-grow"></span><span>Today is ' + esc(W.today.label) + '. Events and weather are known to ' + esc(W.day(W.knownTo).label) + '.</span></div>';
    return h + dayPanel(S);
  }
  function li(S, r, i, maybe) {
    var dl = r.when.filter(function (c) { return R.DAYLESS[c.kind]; })[0];
    return '<li class="crr-' + R.tone(r.then) + '">' + ico(r.then.kind) + '<span>' + (i + 1) + ' · ' + esc(R.thenTitle(r.then)) + '</span>' +
      (r.chance && r.chance !== 'always' ? '<em>' + esc(r.chance) + '</em>' : '') + (maybe && dl ? '<em>' + esc(R.dayless(dl)) + '</em>' : '') + '</li>';
  }
  function dayPanel(S) {
    var W = S.W, d = W.day(S.at), rec = S.rec(), v = R.verdict(W, rec.rules, S.at);
    var meta = [d.season, S.at === 0 ? 'today' : 'in ' + days(S.at), d.weather].filter(Boolean).join(' · ');
    var h = '<div class="crr-dayp" aria-live="polite"><div class="crr-dt"><b>' + esc(d.label + (d.year ? ', year ' + d.year : '')) + '</b><small>' + esc(meta) + '</small>' +
      '<span class="crr-verd' + (v.can ? '' : ' no') + '">' + (v.can ? fa('check') + 'Can be made' : fa('xmark') + 'Can’t be made') + '</span>' +
      (v.can && v.roll !== 'plain' ? '<span class="crr-verd crr-' + (v.roll === 'edge' ? 'help' : 'risk') + '">' + ico(v.roll) + (v.roll === 'edge' ? 'Edge on the roll' : 'Bane on the roll') + '</span>' : '') + '</div><ul>';
    if (!v.can && v.blockers.length) v.blockers.forEach(function (b) { h += li(S, b.r, b.i, b.maybe); });
    if (!v.can && !v.blockers.filter(function (b) { return !b.maybe; }).length && v.only.length) h += '<li class="crr-only">' + ico('only') + '<span>Only on the days rule ' + (v.only[0] + 1) + ' allows</span></li>';
    v.act.forEach(function (a) { h += li(S, a.r, a.i, a.maybe); });
    if (v.can && !v.act.length) h += '<li class="crr-when crr-quiet"><span>No rules change anything this day. It brews as written.</span></li>';
    return h + '</ul></div>';
  }

  /* ---------- Rule cards ---------- */
  function cx(act, v, label, on, pic, extra) {
    return '<button type="button" class="crr-cx" data-act="' + act + '" data-v="' + esc(v) + '"' + (extra || '') + ' aria-pressed="' + !!on + '">' + (pic || '') + esc(label) + '</button>';
  }
  function tx(act, v, label, on, pic, sub, tone, extra) {
    return '<button type="button" class="crr-tx' + (tone ? ' crr-' + tone : '') + '" data-act="' + act + '" data-v="' + esc(v) + '"' + (extra || '') + ' aria-pressed="' + !!on + '">' +
      (pic || '') + '<span>' + esc(label) + '</span>' + (sub ? '<span class="crr-seen">' + esc(sub) + '</span>' : '') + '</button>';
  }
  function hint(t) { return '<p class="crr-hint">' + t + '</p>'; }
  function nextOn(S, test) {
    for (var d = S.at; d <= S.at + S.W.horizon; d++) if (test(S.W.day(d))) { var n = d - S.at; return n ? 'in ' + days(n) : 'today'; }
    return '';
  }
  /* What can be picked for one condition, as pictures. */
  function picker(S, c, j) {
    var W = S.W, L = W.lists, a = ' data-j="' + j + '"', h = '';
    switch (c.kind) {
      case 'moon':
        if (!W.moons.length) return hint('This calendar has no moons.');
        var m = R.moonBy(W, c.moon);
        h += '<div class="crr-chips">' + W.moons.map(function (mo) { return cx('moon', mo.name, mo.name, mo.name === c.moon, moonAt(mo.color, 0.5, 16), a); }).join('') + '</div>';
        if (m && m.manual) {
          h += '<div class="crr-tiles">' + R.STATES.map(function (s) { return tx('phase', s, m.words[s], c.phase === s, moonPic(W, m.name, s, 24), s === W.hand[m.name] ? 'set now' : '', 'moon', a); }).join('') + '</div>' +
            hint(esc(m.name) + ' is moved by hand, so this rule waits until you set it. The calendar uses your own words for it.');
        } else if (m) {
          h += '<div class="crr-ring">' + R.PHASES.map(function (p) {
            return tx('phase', p, cap(p.replace('waxing ', 'wax. ').replace('waning ', 'wan. ')), c.phase === p, moonPic(W, m.name, p, 24),
              nextOn(S, function (d) { return R.moonOn(W, m, d).name === p; }), 'moon', a + ' title="' + esc(cap(p)) + '"');
          }).join('') + '</div><div class="crr-chips">' + cx('phase', 'waxing', 'Any waxing', c.phase === 'waxing', '', a) + cx('phase', 'waning', 'Any waning', c.phase === 'waning', '', a) + '</div>';
        }
        return h;
      case 'event':
        if (!W.eventNames.length) return hint('This calendar has no events yet.');
        return '<div class="crr-chips">' + W.eventNames.slice(0, 60).map(function (e) {
          return cx('event', e, e, c.event === e, ico('event'), a);
        }).join('') + '</div>' + (c.event ? hint('Next: ' + esc(nextOn(S, function (d) { return d.events.indexOf(c.event) >= 0; }) || 'not in the days the calendar has read') + '.') : '');
      case 'rolled':
        if (!W.rolled.length) return hint('No event on this calendar is rolled yet. Give one a roll on the calendar, and it shows here.');
        var ro = R.rolledBy(W, c.event);
        return '<div class="crr-chips">' + W.rolled.map(function (e) { return cx('event', e.name, e.name, c.event === e.name, ico('rolled'), a); }).join('') + '</div>' +
          '<div class="crr-grp">Comes up</div><div class="crr-chips">' + (ro ? ro.results : []).map(function (x) { return cx('result', x, x, c.result === x, '', a); }).join('') + '</div>' +
          hint('You roll it on the calendar; the rule follows whatever comes up.');
      case 'weather':
        var fc = W.forecastKinds, rest = W.weatherKinds.filter(function (w) { return fc.indexOf(w) < 0; });
        if (fc.length) h += '<div class="crr-grp">In the forecast</div><div class="crr-chips">' + fc.map(function (w) { return cx('weather', w, w, c.weather === w, wxIcon(W, w), a); }).join('') + '</div>';
        return h + '<div class="crr-grp">' + (fc.length ? 'Other weather' : 'Weather') + '</div><div class="crr-chips">' + rest.map(function (w) { return cx('weather', w, w, c.weather === w, wxIcon(W, w), a); }).join('') + '</div>';
      case 'season':
        if (!W.seasons.length) return hint('This calendar has no seasons.');
        return '<div class="crr-tiles">' + W.seasons.map(function (s) {
          return tx('season', s.name, s.name, c.season === s.name, ico('season'), (W.months[s.from[0] - 1] || '') + ' ' + s.from[1] + ' – ' + (W.months[s.to[0] - 1] || '') + ' ' + s.to[1], '', a);
        }).join('') + '</div>';
      case 'time': return '<div class="crr-tiles">' + R.TIMES.map(function (t) { return tx('time', t, cap(t), c.time === t, fa(TIME[t]), '', '', a); }).join('') + '</div>';
      case 'bench': return '<div class="crr-chips">' + (L.benches || []).map(function (b) { return cx('bench', b, b, c.bench === b, '', a); }).join('') + '</div>';
      case 'ingredient': return '<div class="crr-chips">' + (L.marks || []).map(function (b) { return cx('mark', b, b, c.mark === b, '', a); }).join('') + '</div>' + hint('Marks come from harvesting and from other recipes.');
      case 'maker': return '<div class="crr-chips">' + (L.skills || []).map(function (b) { return cx('skill', b, b, c.skill === b, '', a); }).join('') + '</div>';
    }
    return hint('No condition: it applies to every brew.');
  }
  function condBox(S, r, c, j) {
    var h = '<div class="crr-cond">' + (r.when.length > 1 ? '<button type="button" class="crr-ib crr-rm" data-act="rm-cond" data-j="' + j + '" aria-label="Remove this part">' + fa('xmark') + '</button>' : '');
    h += '<div class="crr-chips" role="group" aria-label="What kind of thing">' + R.KINDS.map(function (k) { return cx('kind', k[0], k[1], c.kind === k[0], ico(k[0]), ' data-j="' + j + '"'); }).join('') + '</div>';
    return h + picker(S, c, j) + '</div>';
  }
  function stepper(label, value) {
    return '<div class="crr-dice"><span class="crr-hint">' + label + '</span><span class="crr-stepper"><button type="button" data-act="amt" data-v="-1" aria-label="One fewer">' + fa('minus') +
      '</button><output>' + value + '</output><button type="button" data-act="amt" data-v="1" aria-label="One more">' + fa('plus') + '</button></span></div>';
  }
  function detail(S, r) {
    var t = r.then, L = S.W.lists;
    function list(act, items, cur) { return '<div class="crr-chips">' + (items || []).map(function (x) { return cx(act, x, x, cur === x); }).join('') + '</div>'; }
    switch (t.kind) {
      case 'points': return stepper('How many', (+t.amount > 0 ? '+' : '') + (+t.amount || 0));
      case 'extra': return stepper('How many extra', +t.amount || 1);
      case 'becomes': return list('item', L.items, t.item);
      case 'mark': return list('tmark', L.marks, t.mark);
      case 'quirk': case 'mishap': return list('table', L.tables, t.table);
      case 'discover': return list('recipe', L.recipes, t.recipe);
      case 'say': return '<input class="crr-in" data-act="say" maxlength="' + R.LIMITS.line + '" value="' + esc(t.text) + '" aria-label="What the bench says">';
    }
    return '';
  }
  function spotName(k) {
    var S2 = window.CraftRulesStage && window.CraftRulesStage.SPOTS, s = S2 && S2.filter(function (x) { return x.k === k; })[0];
    return s ? s.name : k;
  }
  function lookStrip(S, r) {
    var LOOKS = (window.ItemLookFX || {}).LOOKS || {}, l = r.look || {spots: {}};
    var dots = Object.keys(l.spots).map(function (k) {
      var s = l.spots[k], L = s.look && LOOKS[s.look];
      return '<span class="crr-lk"><i style="background:' + (L ? L.color : 'transparent') + '"></i>' + esc(spotName(k)) + ': ' +
        esc([L ? L.name : '', s.move ? R.MOVES[s.move].name : ''].filter(Boolean).join(' + ')) + '</span>';
    }).join('');
    return '<div class="crr-lookstrip"><span class="crr-hint">When it fires, players see</span>' + (dots || '<span class="crr-hint"><b>nothing yet</b></span>') +
      '<span class="crr-grow"></span><button type="button" class="btn btn-secondary btn-sm" data-act="look">' + fa('play') + '<span>Watch it and change it</span></button></div>';
  }
  function editor(S, r) {
    var tone = R.tone(r.then), h = '<div class="crr-rin">';
    h += '<div class="crr-step crr-when"><span class="crr-lbl">When</span><div class="crr-body">' + r.when.map(function (q, j) { return condBox(S, r, q, j); }).join('') +
      (r.when[0].kind !== 'always' && r.when.length < R.LIMITS.when ? '<button type="button" class="quiet-word crr-addand" data-act="add-cond">' + fa('plus') + '&nbsp;And something else must be true</button>' : '') + '</div></div>';
    h += '<div class="crr-step crr-' + tone + '"><span class="crr-lbl">Then</span><div class="crr-body"><div class="crr-tiles crr-effs">' +
      R.EFFECTS.map(function (e) { return tx('eff', e[0], e[1], r.then.kind === e[0], ico(e[0]), '', R.tone(R.defThen(S.W, e[0]))); }).join('') + '</div>' + detail(S, r);
    if (r.then.kind !== 'only') {
      h += '<div class="crr-dice"><span class="crr-hint">How often</span><div class="crr-seg" role="group" aria-label="How often">' + R.CHANCES.map(function (x) {
        return '<button type="button" data-act="chance" data-v="' + x + '" aria-pressed="' + ((r.chance || 'always') === x) + '">' + (x === 'always' ? 'Every time' : x) + '</button>';
      }).join('') + '</div></div>';
    }
    h += '</div></div><div class="crr-step crr-when"><span class="crr-lbl">Players</span><div class="crr-body"><div class="crr-vcards">' + ['book', 'shown', 'secret'].map(function (p) {
      return '<button type="button" class="crr-vc" data-act="vis" data-v="' + p + '" aria-pressed="' + (r.players === p) + '"><b>' + ico(p) + esc(VIS[p][0]) + '</b><small>' + esc(VIS[p][1]) + '</small></button>';
    }).join('') + '</div>';
    if (r.players === 'book') h += '<label class="crr-ctl"><span>Written in the book as</span><input class="crr-in" data-act="line" maxlength="' + R.LIMITS.line + '" value="' + esc(r.line || '') + '" placeholder="Leave empty to use the rule’s own words"></label>';
    if (r.players !== 'secret') h += lookStrip(S, r);
    h += '</div></div><div class="crr-racts"><button type="button" class="btn btn-ghost btn-sm crr-danger" data-act="del">' + fa('trash') + '<span>Delete</span></button><span class="crr-grow"></span>' +
      '<button type="button" class="btn btn-primary btn-sm" data-act="done">Done</button></div></div>';
    return h;
  }
  function warn(S, r, i) {
    var W = S.W, st = R.status(W, r, 0);
    if (st.key === 'never') {
      var cm = r.when.filter(function (c) { var m = c.kind === 'moon' && R.moonBy(W, c.moon); return m && !m.manual; }), hand = W.moons.filter(function (m) { return m.manual; })[0];
      return '<div class="crr-rwarn">' + fa('triangle-exclamation') + '<span>' + (cm.length > 1 ? esc(cm.map(function (c) { return c.moon; }).join(' and ')) + ' never line up like that, so this never happens.' :
        'Nothing on this calendar matches it, so it never happens.') + '</span>' +
        (cm.length > 1 && hand ? '<button type="button" class="btn btn-secondary btn-sm" data-act="fix">Use “' + esc(hand.name) + ' is ' + esc(hand.words.full) + '” instead</button>' : '') + '</div>';
    }
    if (st.key === 'unknown') return '<div class="crr-rwarn">' + fa('triangle-exclamation') + '<span>' + esc(st.text) + '. Pick one from your calendar.</span></div>';
    var by = R.cancelledBy(W, S.rec().rules, i);
    if (by >= 0) return '<div class="crr-rwarn">' + fa('triangle-exclamation') + '<span>Rule ' + (by + 1) + ' stops it being made on every day this one is on, so this never takes effect.</span></div>';
    return '';
  }
  function card(S, r, i) {
    var W = S.W, st = R.status(W, r, 0), open = !!r._open, h = '<li class="crr-rc' + (open ? ' is-open' : '') + (r.draft ? ' is-draft' : '') + (st.cls === 'warn' ? ' is-warned' : '') + (r._new ? ' is-new' : '') + '" data-i="' + i + '">';
    r._new = false;
    h += '<div class="crr-rhd" data-act="toggle" role="button" tabindex="0" aria-expanded="' + open + '"><span class="crr-rn crr-' + R.tone(r.then) + '">' + (i + 1) + '</span><div class="crr-flow">' +
      r.when.map(function (c, j) { return (j ? '<span class="crr-plus">+</span>' : '') + whenTile(W, c, !j); }).join('') + '<span class="crr-arrow">' + fa('arrow-right') + '</span>' + thenTile(r) + '</div>' +
      '<div class="crr-rside"><span class="crr-stc' + (st.cls ? ' is-' + st.cls : '') + '"><i></i>' + esc(st.text) + '</span><span class="crr-visi" title="' + esc(VIS[r.players][0]) + '">' + ico(r.players) +
      '<span class="crr-sr">' + esc(VIS[r.players][0]) + '</span></span><span class="crr-chev">' + fa('chevron-down') + '</span></div></div>';
    h += '<p class="crr-rsent">' + R.sentence(W, r) + '</p>' + warn(S, r, i) + '<div class="crr-aicap">What the AI writes</div><pre class="crr-aij">' + hl(R.ruleData(r)) + '</pre>';
    h += '<div class="crr-rexp"><div>' + (open ? editor(S, r) : '') + '</div></div>';
    if (r.draft) h += '<div class="crr-rdraft">' + fa('wand-magic-sparkles') + '<span>Drafted by your AI. It changes nothing, and players can’t see it, until you approve it.</span>' +
      '<button type="button" class="btn btn-primary btn-sm" data-act="approve">Approve</button><button type="button" class="btn btn-secondary btn-sm" data-act="discard">Discard</button></div>';
    return h + '</li>';
  }

  /* ---------- The tab ---------- */
  function startsFold(S) {
    return '<div class="crr-fold crr-starts" hidden><div class="crr-fold-hd"><h4>Start a rule</h4><button type="button" class="crr-ib" data-act="close-starts" aria-label="Close">' + fa('xmark') + '</button></div>' +
      '<div class="crr-startgrid">' + R.starters(S.W).map(function (s, i) {
        return '<button type="button" class="crr-st" data-act="start" data-v="' + i + '"><span class="crr-mini">' + whenTile(S.W, s.rule.when[0], true) + '<span class="crr-arrow">' + fa('arrow-right') + '</span>' + thenTile(s.rule) +
          '</span><b>' + esc(s.title) + '</b><small>' + esc(s.note) + '</small></button>';
      }).join('') + '<button type="button" class="crr-st crr-blank" data-act="start" data-v="-1">' + fa('plus') + '<b>Start from scratch</b><small>Pick the When and the Then yourself.</small></button></div>' +
      hint('Every name comes from your calendar, so a rule shows its moons, events and weather in your calendar’s words.') + '</div>';
  }
  function aiFold() {
    return '<div class="crr-fold crr-paste" hidden><div class="crr-fold-hd"><h4>Paste what your AI wrote</h4><button type="button" class="crr-ib" data-act="close-paste" aria-label="Close">' + fa('xmark') + '</button></div>' +
      '<textarea class="crr-in crr-area" rows="6" spellcheck="false" aria-label="What your AI wrote" placeholder="One rule, a list of rules, or { &quot;rules&quot;: [ … ] }"></textarea>' +
      '<div class="crr-racts"><span class="crr-hint crr-paste-msg" role="status"></span><span class="crr-grow"></span><button type="button" class="btn btn-primary btn-sm" data-act="read-paste">Add them as drafts</button></div>' +
      '<pre class="crr-aij crr-block crr-brief" hidden></pre></div>';
  }
  function render(S, root) {
    root.innerHTML = '<div class="crr-pane">' +
      '<div class="crr-aibar">' + fa('wand-magic-sparkles') + '<span>Your AI can write these rules. Give it the calendar’s names, then paste back what it writes; each rule arrives as a draft for you to approve.</span>' +
      '<span class="crr-grow"></span><button type="button" class="btn btn-secondary btn-sm" data-act="copy-ai">' + fa('copy') + '<span>Copy what it needs</span></button>' +
      '<button type="button" class="btn btn-secondary btn-sm" data-act="open-paste">' + fa('paste') + '<span>Paste what it wrote</span></button></div>' + aiFold() +
      // Without a calendar there are no days to show, only rules
      '<div class="crr-sech"' + (S.noCal ? ' hidden' : '') + '><h4>When it’s on</h4><small>The next ' + S.W.span + ' days of the calendar. Tap a day to see what applies.</small></div><div class="crr-tl"' + (S.noCal ? ' hidden' : '') + '>' + timeline(S) + '</div>' +
      '<div class="crr-sech"><h4>Rules</h4><small>Each one reads When → Then. Tap one to change it.</small><button type="button" class="btn btn-primary btn-sm crr-r" data-act="new">' + fa('plus') + '<span>New rule</span></button></div>' +
      '<ol class="crr-rlist">' + S.rec().rules.map(function (r, i) { return card(S, r, i); }).join('') + '</ol>' + startsFold(S) +
      '<button type="button" class="crr-addcard" data-act="new">' + fa('plus') + '<span><b>New rule</b><small>Start from a common one, or from scratch</small></span></button></div>';
  }

  function mount(root, S) {
    var dead = false;
    render(S, root);
    function $(sel) { return root.querySelector(sel); }
    function refreshTL() {
      var t = $('.crr-tl'), sc = t.querySelector('.crr-tl-scroll'), sx = sc ? sc.scrollLeft : 0;
      t.innerHTML = timeline(S);
      sc = t.querySelector('.crr-tl-scroll'); if (sc) sc.scrollLeft = sx;
    }
    function all() {
      $('.crr-rlist').innerHTML = S.rec().rules.map(function (r, i) { return card(S, r, i); }).join('');
      var f = $('.crr-starts'), was = f.hidden, tmp = document.createElement('div'); tmp.innerHTML = startsFold(S); tmp.firstChild.hidden = was; f.replaceWith(tmp.firstChild);
      refreshTL();
    }
    function refreshCard(i, focus) {
      var li = $('.crr-rc[data-i="' + i + '"]'); if (!li) return all();
      var t = document.createElement('div'); t.innerHTML = card(S, S.rec().rules[i], i); li.replaceWith(t.firstChild);
      refreshTL();
      if (focus) { var f = root.querySelector('.crr-rc[data-i="' + i + '"] ' + focus); if (f) f.focus(); }
    }
    function open(i, force) {
      var rules = S.rec().rules;
      rules.forEach(function (r, k) {
        if (k === i || !r._open) return;
        r._open = false; var o = $('.crr-rc[data-i="' + k + '"]');
        if (o) { o.classList.remove('is-open'); o.querySelector('.crr-rhd').setAttribute('aria-expanded', 'false'); }
      });
      var r = rules[i], li = $('.crr-rc[data-i="' + i + '"]'); if (!r || !li) return;
      r._open = force == null ? !r._open : force;
      if (r._open) { li.querySelector('.crr-rexp>div').innerHTML = editor(S, r); void li.offsetHeight; li.classList.add('is-open'); }
      else li.classList.remove('is-open');
      li.querySelector('.crr-rhd').setAttribute('aria-expanded', String(r._open));
    }
    function add(r, opened) {
      var rules = S.rec().rules;
      if (rules.length >= R.LIMITS.rules) { S.say('A recipe holds at most ' + R.LIMITS.rules + ' rules.'); return false; }
      rules.forEach(function (q) { q._open = false; });
      r._new = true; r._open = !!opened; r.line = r.line || '';
      if (!r.look && r.players !== 'secret') r.look = R.defLook(S.W, r);
      r._def = r.look ? R.clone(r.look) : null;
      rules.push(r); all(); S.changed('rules');
      var li = $('.crr-rc[data-i="' + (rules.length - 1) + '"]');
      if (li && li.scrollIntoView) li.scrollIntoView({block: 'nearest', behavior: S.calm() ? 'auto' : 'smooth'});
      return true;
    }
    function fold(sel, show, focusSel) {
      var f = $(sel); f.hidden = !show;
      if (show && focusSel) { var x = f.querySelector(focusSel); if (x) x.focus(); }
    }
    function setDay(d) {
      S.setDay(Math.max(0, Math.min(S.W.span - 1, d))); refreshTL();
      var b = $('.crr-tl-day[data-d="' + S.at + '"]'); if (b) b.focus({preventScroll: false});
    }
    function copyBrief() {
      var text = JSON.stringify(R.aiBrief(S.W, S.rec().name, S.rec().rules, S.fx), null, 2), msg = $('.crr-paste-msg');
      function shown() { fold('.crr-paste', true); var pre = $('.crr-brief'); pre.hidden = false; pre.textContent = text; msg.textContent = 'Copying isn’t allowed here. Select the text below and copy it.'; }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(function () { if (!dead) S.say('Copied. Paste it to your AI with what you want this recipe to do.'); }, shown);
      } else shown();
    }
    function readPaste() {
      var got = R.parseAI($('.crr-area').value, S.fx), msg = $('.crr-paste-msg');
      if (got.error === 'empty') { msg.textContent = 'Paste what your AI wrote first.'; return; }
      if (got.error === 'json') { msg.textContent = 'That isn’t rule data. Ask your AI to answer with the JSON only.'; return; }
      var room = R.LIMITS.rules - S.rec().rules.length, kept = got.rules.slice(0, Math.max(0, room));
      kept.forEach(function (r) {
        r.line = r.line || '';
        if (!r.look && r.players !== 'secret') r.look = R.defLook(S.W, r);
        r._def = r.look ? R.clone(r.look) : null; S.rec().rules.push(r);
      });
      all(); S.changed('rules');
      var skipped = got.dropped + got.rules.length - kept.length;
      msg.textContent = kept.length ? 'Added ' + kept.length + (kept.length === 1 ? ' draft' : ' drafts') + ' below.' + (skipped ? ' ' + skipped + ' couldn’t be read or didn’t fit.' : '') :
        'Nothing could be read as a rule' + (skipped ? ' (' + skipped + ' skipped)' : '') + '.';
      if (kept.length) $('.crr-area').value = '';
    }
    function act(t) {
      var a = t.getAttribute('data-act'), v = t.getAttribute('data-v'), li = t.closest('.crr-rc'), rules = S.rec().rules, i = li ? +li.getAttribute('data-i') : -1, r = i >= 0 ? rules[i] : null;
      var j = +(t.getAttribute('data-j') || 0), c = r && r.when[j], W = S.W;
      var focus = '[data-act="' + a + '"]' + (v != null ? '[data-v="' + String(v).replace(/["\\]/g, '\\$&') + '"]' : '') + (t.hasAttribute('data-j') ? '[data-j="' + j + '"]' : '');
      switch (a) {
        case 'toggle': open(i); return;
        case 'done': open(i, false); li.querySelector('.crr-rhd').focus(); return;
        case 'kind': r.when[j] = R.defCond(W, v); if (v === 'always') r.when = [r.when[j]]; break;
        case 'moon': var m = R.moonBy(W, v); c.moon = v; if (m.manual ? R.STATES.indexOf(c.phase) < 0 : R.PHASES.indexOf(c.phase) < 0 && c.phase !== 'waxing' && c.phase !== 'waning') c.phase = 'full'; break;
        case 'phase': c.phase = v; break;
        case 'event': c.event = v; if (c.kind === 'rolled') { var ro = R.rolledBy(W, v); c.result = ro && ro.results[0] || ''; } break;
        case 'result': c.result = v; break;
        case 'weather': c.weather = v; break;
        case 'season': c.season = v; break;
        case 'time': c.time = v; break;
        case 'bench': c.bench = v; break;
        case 'mark': c.mark = v; break;
        case 'skill': c.skill = v; break;
        case 'add-cond': r.when.push(R.defCond(W, 'weather')); focus = '.crr-addand'; break;
        case 'rm-cond': r.when.splice(j, 1); focus = '.crr-addand'; break;
        case 'eff': r.then = R.defThen(W, v); if (v === 'only') r.chance = 'always'; break;
        case 'amt':
          var n = (+r.then.amount || 0) + (+v);
          if (r.then.kind === 'extra') n = Math.max(1, Math.min(3, n)); else { n = Math.max(-10, Math.min(10, n)); if (!n) n = +v; }
          r.then.amount = n; break;
        case 'item': r.then.item = v; break;
        case 'tmark': r.then.mark = v; break;
        case 'table': r.then.table = v; break;
        case 'recipe': r.then.recipe = v; break;
        case 'chance': r.chance = v; break;
        case 'vis': r.players = v; if (v !== 'secret' && !r.look) { r.look = R.defLook(W, r); r._def = R.clone(r.look); } break;
        case 'del': case 'discard': rules.splice(i, 1); all(); S.changed('rules'); var nx = $('.crr-rc[data-i="' + Math.min(i, rules.length - 1) + '"] .crr-rhd') || $('[data-act="new"]'); if (nx) nx.focus(); return;
        case 'approve': r.draft = false; focus = '.crr-rhd'; break;
        case 'fix':
          var hand = W.moons.filter(function (x) { return x.manual; })[0];
          for (var k = r.when.length - 1; k >= 0; k--) { var mm = r.when[k].kind === 'moon' && R.moonBy(W, r.when[k].moon); if (mm && !mm.manual) { r.when[k] = {kind: 'moon', moon: hand.name, phase: 'full'}; break; } }
          focus = '.crr-rhd'; break;
        case 'look': S.goLook('rule:' + i); return;
        case 'new': fold('.crr-starts', true, '.crr-st'); return;
        case 'close-starts': fold('.crr-starts', false); var nb = $('.crr-sech [data-act="new"]'); if (nb) nb.focus(); return;
        case 'start':
          var st = R.starters(W)[+v];
          fold('.crr-starts', false);
          add(st ? R.clone(st.rule) : {when: [R.defCond(W, 'moon')], then: R.defThen(W, 'edge'), chance: 'always', players: 'shown', line: '', look: null, draft: false}, !st);
          return;
        case 'copy-ai': copyBrief(); return;
        case 'open-paste': fold('.crr-paste', true, '.crr-area'); return;
        case 'close-paste': fold('.crr-paste', false); $('[data-act="open-paste"]').focus(); return;
        case 'read-paste': readPaste(); return;
        default: return;
      }
      refreshCard(i, focus); S.changed('rules');
    }
    function onClick(e) {
      var d = e.target.closest('[data-d]');
      if (d && root.contains(d) && !e.target.closest('select')) { setDay(+d.getAttribute('data-d')); return; }
      var g = e.target.closest('[data-goto]');
      if (g) {
        var i = +g.getAttribute('data-goto'), li = $('.crr-rc[data-i="' + i + '"]');
        if (!S.rec().rules[i]._open) open(i, true);
        if (li && li.scrollIntoView) li.scrollIntoView({block: 'start', behavior: S.calm() ? 'auto' : 'smooth'});
        return;
      }
      var t = e.target.closest('[data-act]');
      if (!t || !root.contains(t) || t.tagName === 'INPUT' || t.tagName === 'SELECT' || t.tagName === 'TEXTAREA') return;
      act(t);
    }
    function onKey(e) {
      var t = e.target;
      if ((e.key === 'Enter' || e.key === ' ') && t.classList.contains('crr-rhd')) { e.preventDefault(); open(+t.closest('.crr-rc').getAttribute('data-i')); return; }
      if (t.classList.contains('crr-tl-day')) {
        var n = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : e.key === 'Home' ? -S.at : e.key === 'End' ? S.W.span : 0;
        if (n) { e.preventDefault(); setDay(S.at + n); }
        return;
      }
      if (e.key !== 'Escape') return;
      var f = t.closest && t.closest('.crr-fold');
      if (f) { e.stopPropagation(); fold(f.classList.contains('crr-paste') ? '.crr-paste' : '.crr-starts', false); var b = $(f.classList.contains('crr-paste') ? '[data-act="open-paste"]' : '[data-act="new"]'); if (b) b.focus(); return; }
      var li = t.closest && t.closest('.crr-rc.is-open');
      if (li) { e.stopPropagation(); open(+li.getAttribute('data-i'), false); li.querySelector('.crr-rhd').focus(); }
    }
    function onChange(e) {
      var t = e.target;
      if (t.getAttribute('data-act') === 'hand') { S.W.hand[t.getAttribute('data-moon')] = t.value; all(); S.changed('day'); }
    }
    function onInput(e) {
      var t = e.target, li = t.closest('.crr-rc'); if (!li) return;
      var r = S.rec().rules[+li.getAttribute('data-i')], a = t.getAttribute('data-act');
      if (a === 'line') r.line = t.value.slice(0, R.LIMITS.line);
      if (a === 'say') { r.then.text = t.value.slice(0, R.LIMITS.line); li.querySelector('.crr-rsent').innerHTML = R.sentence(S.W, r); }
      li.querySelector('.crr-aij').innerHTML = hl(R.ruleData(r));
    }
    root.addEventListener('click', onClick);
    root.addEventListener('keydown', onKey);
    root.addEventListener('change', onChange);
    root.addEventListener('input', onInput);
    return {
      refresh: function () { render(S, root); },
      rules: all,
      destroy: function () {
        dead = true;
        root.removeEventListener('click', onClick); root.removeEventListener('keydown', onKey);
        root.removeEventListener('change', onChange); root.removeEventListener('input', onInput);
      }
    };
  }

  window.CraftRulesEditor = {mount: mount, hl: hl, moonAt: moonAt, fa: fa, ico: ico, condPic: condPic};
})();
