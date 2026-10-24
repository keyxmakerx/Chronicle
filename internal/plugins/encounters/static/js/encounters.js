/**
 * encounters.js: the Encounters block and its slide-out builder.
 *
 * The block lists the fights planned on this page; pressing one slides the
 * builder out over the page. The builder saves as you go, so there is no
 * Save button to forget before game night. Only the DM ever gets this block
 * (the server refuses everyone else too). Every piece of user text goes in
 * with textContent, never as markup.
 *
 * A game system can add its own panel (Draw Steel's encounter budget): the
 * builder fires `chronicle:encounter-draft` on document after every change,
 * with the draft and an empty element the system may fill.
 *
 * Stages say when something changes mid-fight. The form choices for a
 * creature belong to the game system, so the builder asks for them with
 * `chronicle:encounter-forms` ({campaignId, entityIds, reply(map)}) whenever
 * the creature pages change; without an answer only a creature's first form
 * is offered.
 *
 * The pure helpers at the top are exposed as window.EncountersHelpers for the
 * node tests. Styles live in static/css/encounters.css, scoped under .enc.
 */
(function () {
  'use strict';

  // The server's limits, mirrored so the builder can stop short of a refusal.
  var LIM = { name: 120, heroes: 12, rows: 30, count: 40, perPage: 50, stages: 8, forms: 10, arrive: 30, note: 200, formName: 80, versions: 6, versionAdds: 10, versionName: 80 };
  var FIRST_FORM = 'Its first form';
  var GROUPS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'];
  var STARTS = [
    { v: 'seen', label: 'Starts seen' },
    { v: 'hidden', label: 'Starts hidden' },
    { v: 'later', label: 'Arrives later' }
  ];
  var SAVE_DELAY = 600;

  // ---------- Pure helpers ----------
  function uid() { return 'c_' + Math.random().toString(36).slice(2, 10); }
  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  function intIn(v, lo, hi, dflt) {
    v = parseInt(v, 10);
    return isNaN(v) ? dflt : Math.max(lo, Math.min(hi, v));
  }

  function newStage() {
    return {
      id: 's_' + Math.random().toString(36).slice(2, 10),
      when: { kind: 'manual', row: '', percent: 50, round: 2 },
      forms: [], arrive: [], how: 'reveal', note: ''
    };
  }

  function normaliseStage(s) {
    s = s && typeof s === 'object' ? s : {};
    var w = s.when || {};
    return {
      id: s.id || newStage().id,
      when: {
        kind: w.kind === 'stamina' || w.kind === 'round' ? w.kind : 'manual',
        row: w.row || '',
        percent: intIn(w.percent, 1, 99, 50),
        round: intIn(w.round, 2, 99, 2)
      },
      forms: (s.forms || []).map(function (f) {
        return { row: f.row || '', form: f.form || '', formName: f.formName || '' };
      }),
      arrive: (s.arrive || []).filter(function (id) { return typeof id === 'string'; }),
      how: s.how === 'quiet' ? 'quiet' : 'reveal',
      note: typeof s.note === 'string' ? s.note : ''
    };
  }

  // normalise fills what a short or older answer lacks so drawing never
  // meets an undefined list.
  function normalise(e) {
    e = e && typeof e === 'object' ? e : {};
    return {
      id: e.id || '',
      name: typeof e.name === 'string' ? e.name : '',
      ready: !!e.ready,
      version: e.version || 0,
      page: e.page || { id: '', name: '' },
      heroes: (e.heroes || []).map(function (h) { return { id: h.id, name: h.name || '' }; }),
      creatures: (e.creatures || []).map(normaliseRow),
      stages: (e.stages || []).map(normaliseStage),
      versions: (e.versions || []).map(normaliseVersion),
      picked: e.picked || null
    };
  }

  function normaliseRow(c) {
    return {
      id: c.id || uid(), entityId: c.entityId || '', name: c.name || '',
      count: Math.max(1, Math.min(LIM.count, c.count || 1)),
      group: GROUPS.indexOf(c.group) >= 0 ? c.group : 'A',
      start: c.start === 'hidden' || c.start === 'later' ? c.start : 'seen'
    };
  }

  function normaliseVersion(v) {
    v = v && typeof v === 'object' ? v : {};
    var p = v.pick && v.pick.test && v.pick.result ? v.pick : null;
    return {
      id: v.id || uid(),
      name: typeof v.name === 'string' ? v.name : '',
      pick: p ? { test: p.test, result: p.result, testName: p.testName || '', resultLabel: p.resultLabel || '' } : null,
      add: (v.add || []).map(normaliseRow),
      remove: (v.remove || []).filter(function (id) { return typeof id === 'string'; }),
      counts: (v.counts || []).filter(function (x) { return x && x.row; }).map(function (x) {
        return { row: x.row, count: Math.max(1, Math.min(LIM.count, x.count || 1)) };
      })
    };
  }

  // addCreature adds one of a creature. A creature already on the list gets
  // one more instead of a second row, so the list stays one row per kind.
  function addCreature(e, pick) {
    var n = clone(e);
    var same = n.creatures.filter(function (c) {
      return pick.entityId ? c.entityId === pick.entityId : !c.entityId && c.name.toLowerCase() === pick.name.toLowerCase();
    })[0];
    if (same) {
      same.count = Math.min(LIM.count, same.count + 1);
      return n;
    }
    if (n.creatures.length >= LIM.rows) return n;
    var last = n.creatures[n.creatures.length - 1];
    n.creatures.push({
      id: uid(), entityId: pick.entityId || '', name: String(pick.name).slice(0, LIM.name), count: 1,
      group: last ? last.group : 'A', start: 'seen'
    });
    return n;
  }

  function setCount(e, rowId, count) {
    var n = clone(e);
    n.creatures.forEach(function (c) {
      if (c.id === rowId) c.count = Math.max(1, Math.min(LIM.count, count));
    });
    return n;
  }

  function setRow(e, rowId, key, value) {
    var n = clone(e);
    n.creatures.forEach(function (c) { if (c.id === rowId) c[key] = value; });
    return n;
  }

  // removeRow also takes the row out of every stage, so a stage never keeps
  // pointing at a row the server would no longer find.
  function removeRow(e, rowId) {
    var n = clone(e);
    n.creatures = n.creatures.filter(function (c) { return c.id !== rowId; });
    n.stages.forEach(function (s) {
      s.forms = s.forms.filter(function (f) { return f.row !== rowId; });
      s.arrive = s.arrive.filter(function (id) { return id !== rowId; });
      if (s.when.kind === 'stamina' && s.when.row === rowId) s.when.row = n.creatures.length ? n.creatures[0].id : '';
    });
    n.versions.forEach(function (v) {
      v.remove = v.remove.filter(function (id) { return id !== rowId; });
      v.counts = v.counts.filter(function (x) { return x.row !== rowId; });
    });
    return n;
  }

  function stageById(n, id) {
    return n.stages.filter(function (s) { return s.id === id; })[0];
  }

  function addStage(e) {
    var n = clone(e);
    if (n.stages.length < LIM.stages) n.stages.push(newStage());
    return n;
  }

  function removeStage(e, id) {
    var n = clone(e);
    n.stages = n.stages.filter(function (s) { return s.id !== id; });
    return n;
  }

  // setWhen merges a change into a stage's trigger. Moving to Stamina picks
  // the first creature row so the choice is never blank when rows exist.
  function setWhen(e, id, patch) {
    var n = clone(e), s = stageById(n, id);
    if (!s) return n;
    Object.keys(patch).forEach(function (k) { s.when[k] = patch[k]; });
    if (s.when.kind === 'stamina' && !n.creatures.some(function (c) { return c.id === s.when.row; })) {
      s.when.row = n.creatures.length ? n.creatures[0].id : '';
    }
    return n;
  }

  function setStageField(e, id, key, value) {
    var n = clone(e), s = stageById(n, id);
    if (s) s[key] = value;
    return n;
  }

  // pagedRows are the creature rows that have a page, the only ones that can
  // change form.
  function pagedRows(e) {
    return e.creatures.filter(function (c) { return c.entityId; });
  }

  function addForm(e, id) {
    var n = clone(e), s = stageById(n, id), rows = pagedRows(n);
    if (s && rows.length && s.forms.length < LIM.forms) s.forms.push({ row: rows[0].id, form: '', formName: FIRST_FORM });
    return n;
  }

  // setForm changes a form change's creature or form. formName follows the
  // chosen option so Foundry can show it without asking the system.
  function setForm(e, id, i, patch) {
    var n = clone(e), s = stageById(n, id);
    if (!s || !s.forms[i]) return n;
    var f = s.forms[i];
    if (patch.row !== undefined && patch.row !== f.row) { f.row = patch.row; f.form = ''; f.formName = FIRST_FORM; }
    if (patch.form !== undefined) { f.form = patch.form; f.formName = patch.form ? String(patch.formName || '').slice(0, LIM.formName) : FIRST_FORM; }
    return n;
  }

  function removeForm(e, id, i) {
    var n = clone(e), s = stageById(n, id);
    if (s) s.forms.splice(i, 1);
    return n;
  }

  function toggleArrive(e, id, rowId, on) {
    var n = clone(e), s = stageById(n, id);
    if (!s) return n;
    s.arrive = s.arrive.filter(function (r) { return r !== rowId; });
    if (on && s.arrive.length < LIM.arrive) s.arrive.push(rowId);
    return n;
  }

  // arriveOrder lists the rows a stage can bring in, those that start "later"
  // first since they are what a stage usually brings.
  function arriveOrder(e) {
    var later = e.creatures.filter(function (c) { return c.start === 'later'; });
    var rest = e.creatures.filter(function (c) { return c.start !== 'later'; });
    return later.concat(rest);
  }

  // stageReady says whether a stage is complete enough to save: it does
  // something and its trigger names what it needs. An unfinished stage stays
  // in the builder but is left out of the save so it cannot block the rest.
  function stageReady(e, s) {
    if (!s.forms.length && !s.arrive.length) return false;
    var w = s.when;
    if (w.kind === 'stamina') {
      return e.creatures.some(function (c) { return c.id === w.row; }) && w.percent >= 1 && w.percent <= 99;
    }
    if (w.kind === 'round') return w.round >= 2 && w.round <= 99;
    return true;
  }

  function toggleHero(e, hero, on) {
    var n = clone(e);
    n.heroes = n.heroes.filter(function (h) { return h.id !== hero.id; });
    if (on && n.heroes.length < LIM.heroes) n.heroes.push({ id: hero.id, name: hero.name });
    return n;
  }

  // ---------- Versions ----------
  // A version stores only its changes to the base rows, so editing a shared
  // row once changes every version.

  function versionById(n, id) {
    return n.versions.filter(function (v) { return v.id === id; })[0];
  }

  function addVersion(e) {
    var n = clone(e);
    if (n.versions.length < LIM.versions) {
      n.versions.push({ id: uid(), name: 'Version ' + (n.versions.length + 1), pick: null, add: [], remove: [], counts: [] });
    }
    return n;
  }

  // removeVersion also takes its added rows out of every stage.
  function removeVersion(e, id) {
    var n = clone(e), v = versionById(n, id);
    if (!v) return n;
    var gone = v.add.map(function (c) { return c.id; });
    n.versions = n.versions.filter(function (x) { return x.id !== id; });
    n.stages.forEach(function (s) {
      s.forms = s.forms.filter(function (f) { return gone.indexOf(f.row) < 0; });
      s.arrive = s.arrive.filter(function (r) { return gone.indexOf(r) < 0; });
    });
    return n;
  }

  function setVersionName(e, id, name) {
    var n = clone(e), v = versionById(n, id);
    if (v) v.name = String(name).slice(0, LIM.versionName);
    return n;
  }

  // setPick sets the result that chooses a version. A result another version
  // already uses moves here, since one result can choose only one version.
  function setPick(e, id, test, result) {
    var n = clone(e), v = versionById(n, id);
    if (!v) return n;
    if (!test || !result) { v.pick = null; return n; }
    n.versions.forEach(function (x) {
      if (x.id !== id && x.pick && x.pick.test === test && x.pick.result === result) x.pick = null;
    });
    v.pick = { test: test, result: result, testName: '', resultLabel: '' };
    return n;
  }

  // keepRow puts a base row in or out of a version. A row left out loses its
  // changed count, since the server refuses both on one row.
  function keepRow(e, id, rowId, on) {
    var n = clone(e), v = versionById(n, id);
    if (!v) return n;
    v.remove = v.remove.filter(function (r) { return r !== rowId; });
    if (!on) {
      v.remove.push(rowId);
      v.counts = v.counts.filter(function (x) { return x.row !== rowId; });
    }
    return n;
  }

  // setVersionCount plays a base row with another count; the base count
  // itself is stored as no change.
  function setVersionCount(e, id, rowId, count) {
    var n = clone(e), v = versionById(n, id);
    var row = n.creatures.filter(function (c) { return c.id === rowId; })[0];
    if (!v || !row) return n;
    count = Math.max(1, Math.min(LIM.count, count));
    v.counts = v.counts.filter(function (x) { return x.row !== rowId; });
    if (count !== row.count) v.counts.push({ row: rowId, count: count });
    return n;
  }

  // versionCount is how many of a base row a version plays; 0 when left out.
  function versionCount(e, v, rowId) {
    if (v.remove.indexOf(rowId) >= 0) return 0;
    var ch = v.counts.filter(function (x) { return x.row === rowId; })[0];
    if (ch) return ch.count;
    var row = e.creatures.filter(function (c) { return c.id === rowId; })[0];
    return row ? row.count : 0;
  }

  // addVersionRow adds a creature only this version has; one already added
  // gets one more.
  function addVersionRow(e, id, pick) {
    var n = clone(e), v = versionById(n, id);
    if (!v) return n;
    var same = v.add.filter(function (c) {
      return pick.entityId ? c.entityId === pick.entityId : !c.entityId && c.name.toLowerCase() === pick.name.toLowerCase();
    })[0];
    if (same) { same.count = Math.min(LIM.count, same.count + 1); return n; }
    if (v.add.length >= LIM.versionAdds) return n;
    var last = n.creatures[n.creatures.length - 1];
    v.add.push({
      id: uid(), entityId: pick.entityId || '', name: String(pick.name).slice(0, LIM.name), count: 1,
      group: last ? last.group : 'A', start: 'seen'
    });
    return n;
  }

  function setVersionRow(e, id, rowId, key, value) {
    var n = clone(e), v = versionById(n, id);
    if (!v) return n;
    v.add.forEach(function (c) {
      if (c.id !== rowId) return;
      c[key] = key === 'count' ? Math.max(1, Math.min(LIM.count, value)) : value;
    });
    return n;
  }

  function removeVersionRow(e, id, rowId) {
    var n = clone(e), v = versionById(n, id);
    if (!v) return n;
    v.add = v.add.filter(function (c) { return c.id !== rowId; });
    n.stages.forEach(function (s) {
      s.forms = s.forms.filter(function (f) { return f.row !== rowId; });
      s.arrive = s.arrive.filter(function (r) { return r !== rowId; });
    });
    return n;
  }

  // versionTotal counts the creatures a version plays.
  function versionTotal(e, v) {
    var n = 0;
    e.creatures.forEach(function (c) { n += versionCount(e, v, c.id); });
    v.add.forEach(function (c) { n += c.count; });
    return n;
  }

  // pickedLine says, in words, which version the recorded results choose.
  function pickedLine(e) {
    var p = e.picked;
    if (!e.versions.length || !p) return '';
    var v = p.version ? versionById(e, p.version) : null;
    if (v) return 'The recorded result chooses ' + (v.name || 'a version') + '.';
    if (p.open && p.open.length) return 'No result recorded yet, so the base list plays until one is.';
    return 'No recorded result chooses a version, so the base list plays.';
  }

  function totalCreatures(e) {
    return e.creatures.reduce(function (s, c) { return s + c.count; }, 0);
  }

  // countByStart says how the fight opens, for the card under the block.
  function countByStart(e) {
    var out = { seen: 0, hidden: 0, later: 0 };
    e.creatures.forEach(function (c) { out[c.start] += c.count; });
    return out;
  }

  // summary is the one line under an encounter's name in the block.
  function summary(e) {
    var n = totalCreatures(e);
    if (!n) return 'No creatures yet';
    var by = countByStart(e), parts = [n + (n === 1 ? ' creature' : ' creatures')];
    if (by.hidden) parts.push(by.hidden + ' hidden');
    if (by.later) parts.push(by.later + ' arriving later');
    return parts.join(', ');
  }

  // toSave is the PUT body: the whole encounter, as the server stores it.
  function toSave(e) {
    return {
      name: e.name.trim() || 'Untitled encounter',
      ready: e.ready,
      heroes: e.heroes.map(function (h) { return h.id; }),
      creatures: e.creatures.map(function (c) {
        return { id: c.id, entityId: c.entityId, name: c.name, count: c.count, group: c.group, start: c.start };
      }),
      stages: e.stages.filter(function (s) { return stageReady(e, s); }).map(function (s) {
        return {
          id: s.id,
          when: { kind: s.when.kind, row: s.when.kind === 'stamina' ? s.when.row : '',
            percent: s.when.kind === 'stamina' ? s.when.percent : 0, round: s.when.kind === 'round' ? s.when.round : 0 },
          forms: s.forms.map(function (f) { return { row: f.row, form: f.form, formName: f.formName }; }),
          arrive: s.arrive.slice(), how: s.how, note: s.note.trim()
        };
      }),
      versions: e.versions.map(function (v) {
        return {
          id: v.id, name: v.name.trim() || 'Untitled version',
          pick: v.pick ? { test: v.pick.test, result: v.pick.result } : null,
          add: v.add.map(function (c) {
            return { id: c.id, entityId: c.entityId, name: c.name, count: c.count, group: c.group, start: c.start };
          }),
          remove: v.remove.slice(),
          counts: v.counts.map(function (x) { return { row: x.row, count: x.count }; })
        };
      }),
      version: e.version
    };
  }

  // pickTypes reads the page categories the server says a picker lists (the
  // campaign's character list for heroes; creature categories and NPCs for
  // creatures), keeping only whole positive ids.
  function pickTypes(ids) {
    return (Array.isArray(ids) ? ids : []).filter(function (id) {
      return typeof id === 'number' && id > 0 && Math.floor(id) === id;
    });
  }

  // MIN_SEARCH is the shortest search the server answers.
  var MIN_SEARCH = 2;

  window.EncountersHelpers = {
    LIM: LIM, normalise: normalise, addCreature: addCreature, setCount: setCount, setRow: setRow,
    removeRow: removeRow, toggleHero: toggleHero, totalCreatures: totalCreatures,
    countByStart: countByStart, summary: summary, toSave: toSave, pickTypes: pickTypes, MIN_SEARCH: MIN_SEARCH,
    FIRST_FORM: FIRST_FORM, addStage: addStage, removeStage: removeStage, setWhen: setWhen,
    setStageField: setStageField, addForm: addForm, setForm: setForm, removeForm: removeForm,
    toggleArrive: toggleArrive, arriveOrder: arriveOrder, stageReady: stageReady, pagedRows: pagedRows,
    addVersion: addVersion, removeVersion: removeVersion, setVersionName: setVersionName, setPick: setPick,
    keepRow: keepRow, setVersionCount: setVersionCount, versionCount: versionCount, addVersionRow: addVersionRow,
    setVersionRow: setVersionRow, removeVersionRow: removeVersionRow, versionTotal: versionTotal, pickedLine: pickedLine
  };

  if (typeof Chronicle === 'undefined' || typeof document === 'undefined') return;

  // ---------- DOM ----------
  // h builds an element. text goes in as textContent; on is a map of event
  // handlers attached to this element directly.
  function h(tag, cls, text, attrs, on) {
    var el = document.createElement(tag);
    if (cls) el.className = cls;
    if (text != null && text !== '') el.textContent = text;
    if (attrs) Object.keys(attrs).forEach(function (k) {
      if (attrs[k] === false || attrs[k] == null) return;
      el.setAttribute(k, attrs[k] === true ? '' : attrs[k]);
    });
    if (on) Object.keys(on).forEach(function (k) { el.addEventListener(k, on[k]); });
    return el;
  }
  function append(parent, kids) {
    kids.forEach(function (k) { if (k) parent.appendChild(k); });
    return parent;
  }
  function btn(label, cls, onclick, attrs) {
    return h('button', 'enc-btn ' + (cls || ''), label, Object.assign({ type: 'button' }, attrs || {}), { click: onclick });
  }

  function api(S, method, url, body) {
    return Chronicle.apiFetch(url, { method: method, body: body, csrfToken: S.cfg.csrfToken }).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (err) { data = null; }
        if (!res.ok) {
          var e = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
          e.status = res.status;
          throw e;
        }
        return data;
      });
    });
  }
  function base(S) { return '/campaigns/' + encodeURIComponent(S.cfg.campaignId); }
  function apiBase(S) { return '/api/v1/campaigns/' + encodeURIComponent(S.cfg.campaignId); }

  // ---------- The block ----------
  var mounted = 0;

  Chronicle.register('encounters', {
    init: function (el, cfg) {
      var S = {
        el: el, cfg: cfg, id: 'enc' + (++mounted), list: [], canEdit: false,
        open: null, draft: null, saving: Promise.resolve(), timer: null, status: '', statusErr: false,
        heroes: null, heroTypes: [], creatureTypes: null, drawer: null, opener: null, formsMap: null, formsKey: ''
      };
      el.classList.add('enc');
      el._enc = S;
      load(S);
    },
    destroy: function (el) {
      var S = el._enc; if (!S) return;
      flush(S);
      closeDrawer(S, true);
      el._enc = null;
    }
  });

  // load fetches the page's encounters. A first load that fails leaves the
  // block out; a reload that fails keeps what is on screen.
  function load(S) {
    return api(S, 'GET', base(S) + '/pages/' + encodeURIComponent(S.cfg.entityId) + '/encounters').then(function (v) {
      S.list = ((v && v.encounters) || []).map(normalise);
      S.canEdit = !!(v && v.canEdit);
      S.heroTypes = pickTypes(v && v.heroTypeIds);
      S.creatureTypes = pickTypes(v && v.creatureTypeIds);
      S.loaded = true;
      drawBlock(S);
    }).catch(function () {
      if (!S.loaded) S.el.textContent = '';
    });
  }

  function drawBlock(S) {
    var el = S.el;
    el.textContent = '';
    var head = h('div', 'enc-head');
    append(head, [
      h('h3', 'enc-title cb-head', 'Encounters'),
      S.canEdit ? btn('New', 'enc-new', function () { create(S); }, {
        disabled: S.list.length >= LIM.perPage, 'aria-label': 'New encounter'
      }) : null
    ]);
    el.appendChild(head);
    if (!S.list.length) {
      el.appendChild(h('p', 'enc-empty', S.canEdit
        ? 'Plan the fights on this page. Foundry can load them on game night.'
        : 'No encounters planned here.'));
      return;
    }
    var ul = h('ul', 'enc-cards');
    S.list.forEach(function (e) {
      var li = h('li');
      var card = h('button', 'enc-card', '', { type: 'button', 'aria-haspopup': 'dialog', 'data-enc': e.id }, {
        click: function () { openDrawer(S, e.id, card); }
      });
      fillCard(card, e);
      li.appendChild(card);
      ul.appendChild(li);
    });
    el.appendChild(ul);
  }

  function fillCard(card, e) {
    card.textContent = '';
    var top = h('span', 'enc-card-top');
    append(top, [h('span', 'enc-card-name', e.name || 'Untitled encounter'), e.ready ? h('span', 'enc-ready', 'Ready') : null]);
    append(card, [top, h('span', 'enc-card-sub', summary(e))]);
  }

  // refreshCard updates one card in place, so it matches what was saved
  // while the builder is still open without replacing the button focus
  // returns to.
  function refreshCard(S, e) {
    var card = S.el.querySelector('[data-enc="' + e.id + '"]');
    if (card) fillCard(card, e);
  }

  function create(S) {
    if (S.creating) return;
    S.creating = true;
    var newBtn = S.el.querySelector('.enc-new');
    if (newBtn) newBtn.disabled = true;
    api(S, 'POST', base(S) + '/pages/' + encodeURIComponent(S.cfg.entityId) + '/encounters', {
      name: 'New encounter', ready: false, heroes: [], creatures: [], stages: []
    }).then(function (e) {
      S.creating = false;
      e = normalise(e);
      S.list.push(e);
      drawBlock(S);
      openDrawer(S, e.id, S.el.querySelector('.enc-new'), true);
    }).catch(function (err) {
      S.creating = false;
      drawBlock(S);
      var p = S.el.querySelector('.enc-empty') || S.el.appendChild(h('p', 'enc-empty'));
      p.textContent = err.message;
    });
  }

  // ---------- The builder ----------
  function openDrawer(S, id, opener, fresh) {
    var e = S.list.filter(function (x) { return x.id === id; })[0];
    if (!e) return;
    closeDrawer(S, true);
    S.open = id;
    S.draft = clone(e);
    S.opener = opener || null;
    S.status = ''; S.statusErr = false;
    var scrim = h('div', 'enc-scrim', '', null, { click: function () { closeDrawer(S); } });
    var aside = h('aside', 'enc-drawer', '', { role: 'dialog', 'aria-modal': 'true', 'aria-label': 'Encounter builder' }, {
      keydown: function (ev) {
        if (ev.key === 'Escape') { ev.stopPropagation(); closeDrawer(S); }
        else if (ev.key === 'Tab') { ev.stopPropagation(); trapTab(aside, ev); }
      }
    });
    var root = h('div', 'enc enc-layer');
    append(root, [scrim, aside]);
    // Inside a dialog already open (a page opened over the DM Screen) the
    // builder joins that dialog, so it draws above it and the dialog's focus
    // rules count it as inside.
    var host = (S.el.closest && S.el.closest('[aria-modal="true"]')) || document.body;
    host.appendChild(root);
    S.drawer = { root: root, aside: aside };
    drawDrawer(S);
    var name = aside.querySelector('.enc-name');
    if (name) { name.focus(); if (fresh) name.select(); }
    // The panel slides in on the next frame so the browser sees it move.
    requestAnimationFrame(function () { root.classList.add('open'); });
    loadPickers(S);
    loadTests(S);
    S.formsMap = null; S.formsKey = '';
    askForms(S);
  }

  // askForms asks the game system which forms each creature page has. Only
  // asks again when the set of pages changes; a reply for an older set is
  // ignored.
  function askForms(S) {
    var ids = [];
    pagedRows(S.draft).forEach(function (c) { if (ids.indexOf(c.entityId) < 0) ids.push(c.entityId); });
    var key = ids.join(',');
    if (key === S.formsKey) return;
    S.formsKey = key;
    if (!ids.length) { S.formsMap = {}; return; }
    try {
      document.dispatchEvent(new CustomEvent('chronicle:encounter-forms', {
        detail: {
          campaignId: S.cfg.campaignId, entityIds: ids,
          reply: function (map) {
            if (S.formsKey !== key || !S.drawer || !map || typeof map !== 'object') return;
            S.formsMap = Object.assign({}, S.formsMap || {}, map);
            drawDrawer(S);
          }
        }
      }));
    } catch (err) { /* no system, or it failed: first forms only */ }
  }

  // trapTab keeps Tab inside the builder, which is modal.
  function trapTab(aside, ev) {
    var f = Array.prototype.filter.call(aside.querySelectorAll(
      'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
    ), function (n) { return n.offsetWidth || n.offsetHeight || n.getClientRects().length; });
    if (!f.length) { ev.preventDefault(); return; }
    var first = f[0], last = f[f.length - 1], a = document.activeElement;
    if (ev.shiftKey && (a === first || !aside.contains(a))) { ev.preventDefault(); last.focus(); }
    else if (!ev.shiftKey && (a === last || !aside.contains(a))) { ev.preventDefault(); first.focus(); }
  }

  function closeDrawer(S, quiet) {
    if (!S.drawer) return;
    flush(S);
    var root = S.drawer.root, id = S.open;
    S.drawer = null; S.open = null; S.draft = null;
    if (quiet) { root.remove(); return; }
    root.classList.remove('open');
    setTimeout(function () { root.remove(); }, 300);
    var wasNew = S.opener && S.opener.classList.contains('enc-new');
    drawBlock(S);
    // Focus goes back where the DM was: the encounter's card, redrawn above.
    var back = S.el.querySelector(wasNew ? '.enc-new' : '[data-enc="' + id + '"]') || S.el.querySelector('.enc-new');
    if (back) back.focus();
  }

  function setStatus(S, msg, isErr) {
    S.status = msg; S.statusErr = !!isErr;
    var line = S.drawer && S.drawer.aside.querySelector('.enc-status');
    if (line) { line.textContent = msg; line.classList.toggle('err', !!isErr); }
  }

  // change applies an edit to the draft, redraws, tells a game system, and
  // schedules a save.
  function change(S, next, focusSel) {
    S.draft = next;
    drawDrawer(S, focusSel);
    schedule(S);
    askForms(S);
  }

  function schedule(S) {
    clearTimeout(S.timer);
    setStatus(S, 'Saving…');
    if (Chronicle.markDirty) Chronicle.markDirty(S.id);
    S.timer = setTimeout(function () { save(S); }, SAVE_DELAY);
  }

  function flush(S) {
    if (S.timer) { clearTimeout(S.timer); S.timer = null; save(S); }
  }

  // Saves go one at a time so each carries the version the last one
  // returned; quick edits cannot trip the server's conflict check.
  function save(S) {
    S.timer = null;
    var draft = S.draft, id = S.open;
    if (!draft || !id) return S.saving;
    var run = function () {
      var cur = S.list.filter(function (x) { return x.id === id; })[0];
      if (!cur) return null;
      var body = toSave(draft);
      body.version = cur.version;
      return api(S, 'PUT', base(S) + '/encounters/' + encodeURIComponent(id), body).then(function (v) {
        v = normalise(v);
        S.list = S.list.map(function (x) { return x.id === id ? v : x; });
        if (S.open === id && S.draft) S.draft.version = v.version;
        if (Chronicle.markClean) Chronicle.markClean(S.id);
        setStatus(S, 'Saved');
        if (S.drawer) refreshCard(S, v); else drawBlock(S);
      }).catch(function (err) {
        if (err.status === 409) {
          setStatus(S, 'Someone else changed this. Showing their version.', true);
          return load(S).then(function () {
            var fresh = S.list.filter(function (x) { return x.id === id; })[0];
            if (fresh && S.open === id) { S.draft = clone(fresh); drawDrawer(S); }
            setStatus(S, 'Someone else changed this. Showing their version.', true);
          });
        }
        setStatus(S, err.message, true);
      });
    };
    S.saving = S.saving.then(run, run);
    return S.saving;
  }

  function remove(S, id) {
    clearTimeout(S.timer); S.timer = null;
    api(S, 'DELETE', base(S) + '/encounters/' + encodeURIComponent(id)).then(function () {
      S.list = S.list.filter(function (x) { return x.id !== id; });
      if (Chronicle.markClean) Chronicle.markClean(S.id);
      closeDrawer(S);
    }).catch(function (err) { setStatus(S, err.message, true); });
  }

  // tellSystem lets a game system add its panel (an encounter budget) to the
  // slot. A system that listens late still gets the next change.
  function tellSystem(S, slot) {
    var d = S.draft;
    try {
      document.dispatchEvent(new CustomEvent('chronicle:encounter-draft', {
        detail: {
          campaignId: S.cfg.campaignId,
          encounter: { id: d.id, name: d.name, heroes: clone(d.heroes), creatures: clone(d.creatures), stages: clone(d.stages) },
          slot: slot
        }
      }));
    } catch (err) { /* a system panel failing never breaks the builder */ }
  }

  // loadPickers fetches the hero pages once per block, from the categories
  // the page list named. A failure leaves the hero picker empty; typing a
  // creature's name still works.
  function loadPickers(S) {
    if (S.heroes) return;
    Promise.all((S.heroTypes || []).map(function (tid) {
      return api(S, 'GET', apiBase(S) + '/entities?type_id=' + tid + '&per_page=100');
    })).then(function (pages) {
      // A category's list can include its sub-categories' pages, which the
      // server also names, so a page may arrive twice.
      var all = [], seen = {};
      (pages || []).forEach(function (env) {
        ((env && env.data) || []).forEach(function (p) {
          if (!seen[p.id]) { seen[p.id] = true; all.push({ id: p.id, name: p.name }); }
        });
      });
      S.heroes = all.sort(function (a, b) { return a.name.localeCompare(b.name); });
      if (S.drawer) drawDrawer(S);
    }).catch(function () {
      S.heroes = S.heroes || []; S.creatureTypes = S.creatureTypes || [];
      if (S.drawer) drawDrawer(S);
    });
  }

  function searchCreatures(S, q) {
    var types = S.creatureTypes || [];
    if (!q || q.length < MIN_SEARCH || !types.length) return Promise.resolve([]);
    return Promise.all(types.map(function (tid) {
      return api(S, 'GET', apiBase(S) + '/entities?type_id=' + tid + '&per_page=8&q=' + encodeURIComponent(q))
        .catch(function () { return null; });
    })).then(function (pages) {
      var out = [], seen = {};
      pages.forEach(function (env) {
        ((env && env.data) || []).forEach(function (p) {
          if (!seen[p.id]) { seen[p.id] = true; out.push({ entityId: p.id, name: p.name }); }
        });
      });
      return out.slice(0, 8);
    });
  }

  function drawDrawer(S, focusSel) {
    if (!S.drawer) return;
    var aside = S.drawer.aside, d = S.draft, edit = S.canEdit;
    var keep = focusSel || (document.activeElement && aside.contains(document.activeElement) && document.activeElement.getAttribute('data-k'));
    aside.textContent = '';

    var top = h('div', 'enc-d-top');
    var name = h('input', 'enc-input enc-name', '', {
      type: 'text', maxlength: String(LIM.name), 'aria-label': 'Encounter name', 'data-k': 'name', readonly: !edit
    }, {
      input: function () { S.draft.name = name.value; schedule(S); tellSystem(S, slot); }
    });
    name.value = d.name;
    append(top, [name, h('button', 'enc-x', '×', { type: 'button', 'aria-label': 'Close the builder' }, { click: function () { closeDrawer(S); } })]);

    var body = h('div', 'enc-d-body');
    var slot = h('div', 'enc-system', '', { 'data-encounter-budget': true });
    append(body, [heroSection(S), slot, creatureSection(S), versionSection(S), stageSection(S), readySection(S)]);

    var foot = h('div', 'enc-d-foot');
    append(foot, [
      h('span', 'enc-status' + (S.statusErr ? ' err' : ''), S.status, { role: 'status', 'aria-live': 'polite' }),
      h('span', 'enc-hint', 'Foundry finds it in its Encounters tab.'),
      edit ? deleteButton(S) : null
    ]);
    append(aside, [top, body, foot]);
    tellSystem(S, slot);
    if (keep) {
      var t = aside.querySelector('[data-k="' + keep + '"]');
      if (t) t.focus();
    }
  }

  function heroSection(S) {
    var d = S.draft, sec = h('section', 'enc-sec');
    var on = {};
    d.heroes.forEach(function (x) { on[x.id] = true; });
    var label = h('p', 'enc-lab', "Who's fighting");
    label.appendChild(h('b', '', d.heroes.length + (d.heroes.length === 1 ? ' hero' : ' heroes')));
    sec.appendChild(label);
    if (S.heroes === null) { sec.appendChild(h('p', 'enc-muted', 'Loading heroes…')); return sec; }
    // A hero ticked earlier whose page this list no longer has stays shown.
    var list = S.heroes.slice();
    d.heroes.forEach(function (x) {
      if (!list.some(function (p) { return p.id === x.id; })) list.push(x);
    });
    if (!list.length) { sec.appendChild(h('p', 'enc-muted', 'No hero pages yet. Make a character page for each hero.')); return sec; }
    var wrap = h('div', 'enc-heroes');
    list.forEach(function (p) {
      var lab = h('label', 'enc-hero' + (on[p.id] ? ' on' : ''));
      var cb = h('input', '', '', { type: 'checkbox', 'data-k': 'hero-' + p.id, disabled: !S.canEdit || (!on[p.id] && d.heroes.length >= LIM.heroes) }, {
        change: function () { change(S, toggleHero(S.draft, p, cb.checked), 'hero-' + p.id); }
      });
      cb.checked = !!on[p.id];
      append(lab, [cb, h('span', '', p.name)]);
      wrap.appendChild(lab);
    });
    sec.appendChild(wrap);
    return sec;
  }

  function creatureSection(S) {
    var d = S.draft, sec = h('section', 'enc-sec'), edit = S.canEdit;
    var label = h('p', 'enc-lab', 'Creatures');
    label.appendChild(h('b', '', String(totalCreatures(d))));
    sec.appendChild(label);
    if (!d.creatures.length) sec.appendChild(h('p', 'enc-muted', 'Add creature pages below. Each row says how the creature starts the fight.'));
    var rows = h('div', 'enc-rows');
    d.creatures.forEach(function (c) {
      var row = h('div', 'enc-row');
      var who = h('div', 'enc-who');
      append(who, [h('b', '', c.name), c.entityId ? null : h('span', 'enc-muted', 'No page yet')]);
      var grp = h('select', 'enc-sel', '', { 'aria-label': 'Initiative group for ' + c.name, 'data-k': 'g-' + c.id, disabled: !edit }, {
        change: function () { change(S, setRow(S.draft, c.id, 'group', grp.value), 'g-' + c.id); }
      });
      GROUPS.forEach(function (g) { grp.appendChild(h('option', '', 'Group ' + g, { value: g, selected: g === c.group })); });
      var st = h('select', 'enc-sel enc-start-' + c.start, '', { 'aria-label': 'How ' + c.name + ' starts', 'data-k': 's-' + c.id, disabled: !edit }, {
        change: function () { change(S, setRow(S.draft, c.id, 'start', st.value), 's-' + c.id); }
      });
      STARTS.forEach(function (o) { st.appendChild(h('option', '', o.label, { value: o.v, selected: o.v === c.start })); });
      var step = h('span', 'enc-step');
      append(step, [
        h('button', '', '−', { type: 'button', 'aria-label': 'One fewer ' + c.name, 'data-k': 'm-' + c.id, disabled: !edit || c.count <= 1 }, {
          click: function () { change(S, setCount(S.draft, c.id, c.count - 1), 'm-' + c.id); }
        }),
        h('output', '', String(c.count), { 'aria-label': c.count + ' ' + c.name }),
        h('button', '', '+', { type: 'button', 'aria-label': 'One more ' + c.name, 'data-k': 'p-' + c.id, disabled: !edit || c.count >= LIM.count }, {
          click: function () { change(S, setCount(S.draft, c.id, c.count + 1), 'p-' + c.id); }
        })
      ]);
      append(row, [who, grp, st, step, edit ? h('button', 'enc-x enc-rm', '×', { type: 'button', 'aria-label': 'Remove ' + c.name }, {
        click: function () { change(S, removeRow(S.draft, c.id), 'add'); }
      }) : null]);
      rows.appendChild(row);
    });
    sec.appendChild(rows);
    if (edit) {
      sec.appendChild(addBox(S, 'add', d.creatures.length >= LIM.rows, function (p) {
        change(S, addCreature(S.draft, p), 'add');
      }));
    }
    return sec;
  }

  // addBox searches the creature pages as the DM types. Enter, or the last
  // choice, adds a creature by name when no page fits.
  function addBox(S, key, full, onPick, placeholder) {
    var box = h('div', 'enc-add');
    var results = h('ul', 'enc-results', '', { role: 'listbox', 'aria-label': 'Creature pages' });
    var seq = 0;
    var input = h('input', 'enc-input', '', {
      type: 'search', placeholder: full ? 'This list has the most rows it can hold' : (placeholder || 'Add a creature page'),
      'aria-label': placeholder || 'Add a creature', 'data-k': key, maxlength: String(LIM.name), disabled: full, autocomplete: 'off'
    }, {
      input: function () {
        var q = input.value.trim(), mine = ++seq;
        searchCreatures(S, q).then(function (found) {
          if (mine !== seq) return;
          showResults(found, q);
        });
      },
      keydown: function (ev) {
        if (ev.key !== 'Enter') return;
        ev.preventDefault();
        var first = results.querySelector('button');
        if (first) first.click();
      }
    });
    function pick(p) { onPick(p); }
    function showResults(found, q) {
      results.textContent = '';
      found.forEach(function (p) {
        var li = h('li');
        li.appendChild(h('button', '', p.name, { type: 'button', role: 'option' }, { click: function () { pick(p); } }));
        results.appendChild(li);
      });
      if (q) {
        var li = h('li');
        li.appendChild(h('button', 'enc-byname', 'Add "' + q + '" without a page', { type: 'button', role: 'option' }, {
          click: function () { pick({ entityId: '', name: q }); }
        }));
        results.appendChild(li);
      }
    }
    append(box, [input, results]);
    return box;
  }

  // ---------- Versions ----------

  // loadTests fetches the tests a version can be picked by. A failure leaves
  // the picks already saved readable and offers no new ones.
  function loadTests(S) {
    S.tests = S.canEdit ? null : [];
    if (!S.canEdit) return;
    api(S, 'GET', base(S) + '/tests').then(function (list) {
      S.tests = Array.isArray(list) ? list : [];
      if (S.drawer) drawDrawer(S);
    }).catch(function () {
      S.tests = [];
      if (S.drawer) drawDrawer(S);
    });
  }

  function versionSection(S) {
    var d = S.draft, sec = h('section', 'enc-sec'), edit = S.canEdit;
    var label = h('p', 'enc-lab', 'Versions');
    if (d.versions.length) label.appendChild(h('b', '', String(d.versions.length)));
    sec.appendChild(label);
    if (!d.versions.length) {
      sec.appendChild(h('p', 'enc-muted', 'A version is this fight played another way: some creatures added, left out or in other numbers. A test result can choose which one plays.'));
      if (edit) sec.appendChild(btn('Add a version', 'enc-quiet', function () { openVersion(S, addVersion(S.draft)); }, { 'data-k': 'add-version' }));
      return sec;
    }
    if (!versionById(d, S.vtab)) S.vtab = d.versions[0].id;
    var saved = S.list.filter(function (x) { return x.id === S.open; })[0];
    var line = saved ? pickedLine(saved) : '';
    if (line) sec.appendChild(h('p', 'enc-muted enc-picked', line));

    var tabs = h('div', 'enc-tabs', '', { role: 'tablist', 'aria-label': 'Versions' });
    d.versions.forEach(function (v) {
      var on = v.id === S.vtab;
      tabs.appendChild(h('button', 'enc-tab' + (on ? ' on' : ''), v.name || 'Untitled version', {
        type: 'button', role: 'tab', 'aria-selected': on ? 'true' : 'false', 'data-k': 'vt-' + v.id
      }, { click: function () { S.vtab = v.id; drawDrawer(S, 'vt-' + v.id); } }));
    });
    if (edit) {
      tabs.appendChild(btn('Add a version', 'enc-quiet', function () { openVersion(S, addVersion(S.draft)); }, {
        'data-k': 'add-version', disabled: d.versions.length >= LIM.versions
      }));
    }
    sec.appendChild(tabs);
    sec.appendChild(versionPanel(S, versionById(d, S.vtab)));
    return sec;
  }

  // openVersion saves a new version and shows its tab.
  function openVersion(S, next) {
    var v = next.versions[next.versions.length - 1];
    if (v) S.vtab = v.id;
    change(S, next, v ? 'vn-' + v.id : 'add-version');
  }

  function versionPanel(S, v) {
    var d = S.draft, edit = S.canEdit, k = v.id;
    var card = h('div', 'enc-stage enc-version', '', { role: 'tabpanel', 'data-version': v.id });

    var name = h('input', 'enc-input', '', {
      type: 'text', maxlength: String(LIM.versionName), 'aria-label': 'Version name', 'data-k': 'vn-' + k, readonly: !edit
    }, {
      input: function () { S.draft = setVersionName(S.draft, v.id, name.value); schedule(S); }
    });
    name.value = v.name;
    card.appendChild(field('Name', name));
    card.appendChild(field('Chosen by', pickSelect(S, v)));

    // The base rows, each kept, left out or played in other numbers.
    var rows = h('div', 'enc-rows');
    if (!d.creatures.length) rows.appendChild(h('span', 'enc-muted', 'No creature rows yet.'));
    d.creatures.forEach(function (c) {
      var n = versionCount(d, v, c.id), kept = n > 0;
      var row = h('div', 'enc-vrow' + (kept ? '' : ' out'));
      var lab = h('label', 'enc-vkeep');
      var cb = h('input', '', '', { type: 'checkbox', 'data-k': 'vk-' + k + '-' + c.id, disabled: !edit }, {
        change: function () { change(S, keepRow(S.draft, v.id, c.id, cb.checked), 'vk-' + k + '-' + c.id); }
      });
      cb.checked = kept;
      append(lab, [cb, h('span', '', c.name)]);
      row.appendChild(lab);
      if (kept) {
        row.appendChild(stepper(S, c.name, n, 'vc-' + k + '-' + c.id, function (to) {
          return setVersionCount(S.draft, v.id, c.id, to);
        }));
        if (n !== c.count) row.appendChild(h('span', 'enc-muted enc-vwas', 'base ' + c.count));
      } else {
        row.appendChild(h('span', 'enc-muted', 'Left out'));
      }
      rows.appendChild(row);
    });
    card.appendChild(field('From the base list', rows));

    // Rows only this version has.
    var adds = h('div', 'enc-rows');
    v.add.forEach(function (c) {
      var row = h('div', 'enc-row');
      var st = h('select', 'enc-sel enc-start-' + c.start, '', { 'aria-label': 'How ' + c.name + ' starts', 'data-k': 'vs-' + c.id, disabled: !edit }, {
        change: function () { change(S, setVersionRow(S.draft, v.id, c.id, 'start', st.value), 'vs-' + c.id); }
      });
      STARTS.forEach(function (o) { st.appendChild(h('option', '', o.label, { value: o.v, selected: o.v === c.start })); });
      var grp = h('select', 'enc-sel', '', { 'aria-label': 'Initiative group for ' + c.name, 'data-k': 'vg-' + c.id, disabled: !edit }, {
        change: function () { change(S, setVersionRow(S.draft, v.id, c.id, 'group', grp.value), 'vg-' + c.id); }
      });
      GROUPS.forEach(function (g) { grp.appendChild(h('option', '', 'Group ' + g, { value: g, selected: g === c.group })); });
      var who = h('div', 'enc-who');
      append(who, [h('b', '', c.name), c.entityId ? null : h('span', 'enc-muted', 'No page yet')]);
      append(row, [who, grp, st, stepper(S, c.name, c.count, 'va-' + c.id, function (to) {
        return setVersionRow(S.draft, v.id, c.id, 'count', to);
      }), edit ? h('button', 'enc-x enc-rm', '×', { type: 'button', 'aria-label': 'Remove ' + c.name + ' from this version' }, {
        click: function () { change(S, removeVersionRow(S.draft, v.id, c.id), 'vadd-' + k); }
      }) : null]);
      adds.appendChild(row);
    });
    if (edit) {
      adds.appendChild(addBox(S, 'vadd-' + k, v.add.length >= LIM.versionAdds, function (p) {
        change(S, addVersionRow(S.draft, v.id, p), 'vadd-' + k);
      }, 'Add a creature only this version has'));
    } else if (!v.add.length) {
      adds.appendChild(h('span', 'enc-muted', 'None.'));
    }
    card.appendChild(field('Only in this version', adds));

    var foot = h('div', 'enc-stage-head');
    var total = versionTotal(d, v);
    append(foot, [
      h('span', 'enc-muted', total + (total === 1 ? ' creature' : ' creatures') + ' in this version'),
      edit ? btn('Remove version', 'enc-ghost', function () { change(S, removeVersion(S.draft, v.id), 'add-version'); }, { 'data-k': 'rv-' + k }) : null
    ]);
    card.appendChild(foot);
    return card;
  }

  // stepper is the − count + control; next returns the draft after a step.
  function stepper(S, name, count, key, next) {
    var edit = S.canEdit, step = h('span', 'enc-step');
    append(step, [
      h('button', '', '−', { type: 'button', 'aria-label': 'One fewer ' + name, 'data-k': key + '-m', disabled: !edit || count <= 1 }, {
        click: function () { change(S, next(count - 1), key + '-m'); }
      }),
      h('output', '', String(count), { 'aria-label': count + ' ' + name }),
      h('button', '', '+', { type: 'button', 'aria-label': 'One more ' + name, 'data-k': key + '-p', disabled: !edit || count >= LIM.count }, {
        click: function () { change(S, next(count + 1), key + '-p'); }
      })
    ]);
    return step;
  }

  // pickSelect lists every test result that can choose this version. A saved
  // pick the list does not have (not loaded, or the list failed) stays shown
  // so opening the builder never silently changes it.
  function pickSelect(S, v) {
    var cur = v.pick ? v.pick.test + '\u0000' + v.pick.result : '';
    var sel = h('select', 'enc-sel', '', { 'aria-label': 'Test result that chooses this version', 'data-k': 'vp-' + v.id, disabled: !S.canEdit }, {
      change: function () {
        var parts = sel.value ? sel.value.split('\u0000') : ['', ''];
        change(S, setPick(S.draft, v.id, parts[0], parts[1]), 'vp-' + v.id);
      }
    });
    sel.appendChild(h('option', '', 'Only I choose it', { value: '', selected: !cur }));
    var found = !cur;
    (S.tests || []).forEach(function (t) {
      var g = h('optgroup', '', '', { label: t.name + (t.page && t.page.name ? ' (' + t.page.name + ')' : '') });
      (t.results || []).forEach(function (r) {
        var val = t.id + '\u0000' + r.id;
        if (val === cur) found = true;
        g.appendChild(h('option', '', t.name + ': ' + r.label, { value: val, selected: val === cur }));
      });
      sel.appendChild(g);
    });
    if (!found) {
      sel.appendChild(h('option', '', (v.pick.testName || 'A test') + ': ' + (v.pick.resultLabel || 'a result'), { value: cur, selected: true }));
    }
    if (S.tests == null) sel.appendChild(h('option', '', 'Loading tests…', { value: '', disabled: true }));
    else if (!S.tests.length && S.canEdit) sel.appendChild(h('option', '', 'No tests yet. Add one to a quest or adventure page.', { value: '', disabled: true }));
    return sel;
  }

  // ---------- Stages ----------
  var WHEN_KINDS = [
    { v: 'manual', label: 'When I start it' },
    { v: 'stamina', label: 'When a creature is low on Stamina' },
    { v: 'round', label: 'At the start of a round' }
  ];

  function stageSection(S) {
    var d = S.draft, sec = h('section', 'enc-sec'), edit = S.canEdit;
    var label = h('p', 'enc-lab', 'Stages');
    if (d.stages.length) label.appendChild(h('b', '', String(d.stages.length)));
    sec.appendChild(label);
    if (!d.stages.length) {
      sec.appendChild(h('p', 'enc-muted', 'Stages say when things change mid-fight: a creature taking another form, or the later rows arriving. You start each one from the DM Screen in Foundry.'));
    }
    d.stages.forEach(function (st, i) { sec.appendChild(stageCard(S, st, i)); });
    if (edit) {
      sec.appendChild(btn('Add a stage', 'enc-add-stage', function () {
        change(S, addStage(S.draft), 'add-stage');
      }, { 'data-k': 'add-stage', disabled: d.stages.length >= LIM.stages }));
    }
    return sec;
  }

  // field wraps a control in a labelled row.
  function field(label, control) {
    var f = h('div', 'enc-field');
    append(f, [h('span', 'enc-flab', label), control]);
    return f;
  }

  function rowSelect(S, rows, value, key, aria, onPick) {
    var sel = h('select', 'enc-sel', '', { 'aria-label': aria, 'data-k': key, disabled: !S.canEdit }, {
      change: function () { onPick(sel.value); }
    });
    rows.forEach(function (c) { sel.appendChild(h('option', '', c.name, { value: c.id, selected: c.id === value })); });
    return sel;
  }

  function stageCard(S, st, i) {
    var d = S.draft, edit = S.canEdit, k = st.id, card = h('div', 'enc-stage', '', { 'data-stage': st.id });
    var head = h('div', 'enc-stage-head');
    append(head, [
      h('b', 'enc-stage-title', 'Stage ' + (i + 1)),
      edit ? btn('Remove stage', 'enc-ghost', function () { change(S, removeStage(S.draft, st.id), 'add-stage'); }, { 'data-k': 'rs-' + k }) : null
    ]);
    card.appendChild(head);

    // When
    var kind = h('select', 'enc-sel', '', { 'aria-label': 'When stage ' + (i + 1) + ' is due', 'data-k': 'w-' + k, disabled: !edit }, {
      change: function () { change(S, setWhen(S.draft, st.id, { kind: kind.value }), 'w-' + k); }
    });
    WHEN_KINDS.forEach(function (o) { kind.appendChild(h('option', '', o.label, { value: o.v, selected: o.v === st.when.kind })); });
    var when = h('div', 'enc-inline');
    when.appendChild(kind);
    if (st.when.kind === 'stamina') {
      if (!d.creatures.length) {
        when.appendChild(h('span', 'enc-muted', 'Add a creature row first.'));
      } else {
        when.appendChild(rowSelect(S, d.creatures, st.when.row, 'wr-' + k, 'Creature watched for stage ' + (i + 1), function (v) {
          change(S, setWhen(S.draft, st.id, { row: v }), 'wr-' + k);
        }));
        var pct = h('input', 'enc-input enc-num', '', {
          type: 'number', min: '1', max: '99', step: '1', inputmode: 'numeric', 'data-k': 'wp-' + k, disabled: !edit,
          'aria-label': 'At or below this percent of Stamina, stage ' + (i + 1)
        }, {
          input: function () {
            var v = parseInt(pct.value, 10);
            S.draft = setWhen(S.draft, st.id, { percent: isNaN(v) ? 0 : v });
            schedule(S); tellSystemLater(S);
          },
          change: function () { change(S, setWhen(S.draft, st.id, { percent: intIn(pct.value, 1, 99, 50) }), 'wp-' + k); }
        });
        pct.value = String(st.when.percent);
        append(when, [h('span', 'enc-muted', 'at or below'), pct, h('span', 'enc-muted', '% Stamina')]);
      }
    } else if (st.when.kind === 'round') {
      var rnd = h('input', 'enc-input enc-num', '', {
        type: 'number', min: '2', max: '99', step: '1', inputmode: 'numeric', 'data-k': 'wn-' + k, disabled: !edit,
        'aria-label': 'Round number, stage ' + (i + 1)
      }, {
        input: function () {
          var v = parseInt(rnd.value, 10);
          S.draft = setWhen(S.draft, st.id, { round: isNaN(v) ? 0 : v });
          schedule(S); tellSystemLater(S);
        },
        change: function () { change(S, setWhen(S.draft, st.id, { round: intIn(rnd.value, 2, 99, 2) }), 'wn-' + k); }
      });
      rnd.value = String(st.when.round);
      append(when, [h('span', 'enc-muted', 'Round'), rnd]);
    }
    card.appendChild(field('When', when));

    // Forms
    var forms = h('div', 'enc-forms');
    var paged = pagedRows(d);
    st.forms.forEach(function (f, fi) { forms.appendChild(formRow(S, st, f, fi, paged)); });
    if (edit) {
      forms.appendChild(btn('Add a form change', 'enc-quiet', function () {
        change(S, addForm(S.draft, st.id), 'af-' + k);
      }, { 'data-k': 'af-' + k, disabled: !paged.length || st.forms.length >= LIM.forms }));
    }
    if (!paged.length) forms.appendChild(h('span', 'enc-muted', 'Form changes need a creature that has a page.'));
    card.appendChild(field('Forms', forms));

    // Arrive
    var arrive = h('div', 'enc-arrive');
    var order = arriveOrder(d);
    if (!order.length) arrive.appendChild(h('span', 'enc-muted', 'No creature rows yet.'));
    order.forEach(function (c) {
      var on = st.arrive.indexOf(c.id) >= 0;
      var lab = h('label', 'enc-hero' + (on ? ' on' : ''));
      var cb = h('input', '', '', { type: 'checkbox', 'data-k': 'ar-' + k + '-' + c.id, disabled: !edit || (!on && st.arrive.length >= LIM.arrive) }, {
        change: function () { change(S, toggleArrive(S.draft, st.id, c.id, cb.checked), 'ar-' + k + '-' + c.id); }
      });
      cb.checked = on;
      append(lab, [cb, h('span', '', c.name + (c.count > 1 ? ' ×' + c.count : '') + (c.start === 'later' ? ' (later)' : ''))]);
      arrive.appendChild(lab);
    });
    card.appendChild(field('Arrive', arrive));

    // How players see a form change
    var seg = h('div', 'enc-seg', '', { role: 'radiogroup', 'aria-label': 'How players see the form change, stage ' + (i + 1) });
    [{ v: 'quiet', label: 'Quiet' }, { v: 'reveal', label: 'Reveal' }].forEach(function (o) {
      var lab = h('label', 'enc-seg-opt' + (st.how === o.v ? ' on' : ''));
      var rb = h('input', '', '', { type: 'radio', name: S.id + '-how-' + k, value: o.v, 'data-k': 'h-' + k + '-' + o.v, disabled: !edit }, {
        change: function () { if (rb.checked) change(S, setStageField(S.draft, st.id, 'how', o.v), 'h-' + k + '-' + o.v); }
      });
      rb.checked = st.how === o.v;
      append(lab, [rb, h('span', '', o.label)]);
      seg.appendChild(lab);
    });
    card.appendChild(field('How players see the form change', seg));

    // Note
    var note = h('input', 'enc-input', '', {
      type: 'text', maxlength: String(LIM.note), placeholder: 'Note for yourself (optional)',
      'aria-label': 'Note for stage ' + (i + 1), 'data-k': 'n-' + k, readonly: !edit
    }, {
      input: function () { S.draft = setStageField(S.draft, st.id, 'note', note.value); schedule(S); tellSystemLater(S); }
    });
    note.value = st.note;
    card.appendChild(note);

    if (!stageReady(d, st)) {
      card.appendChild(h('p', 'enc-muted enc-stage-warn', 'Not saved yet. Add a form change or an arrival' +
        (st.when.kind === 'stamina' ? ', and pick the creature to watch' : '') + ' to keep this stage.'));
    }
    return card;
  }

  // formRow is one creature changing form: who, and into what.
  function formRow(S, st, f, fi, paged) {
    var k = st.id + '-' + fi, edit = S.canEdit;
    var row = h('div', 'enc-formrow');
    var who = rowSelect(S, paged.some(function (c) { return c.id === f.row; }) ? paged : paged.concat(
      S.draft.creatures.filter(function (c) { return c.id === f.row; })), f.row, 'fc-' + k, 'Creature changing form', function (v) {
      change(S, setForm(S.draft, st.id, fi, { row: v }), 'fc-' + k);
    });
    var entity = (S.draft.creatures.filter(function (c) { return c.id === f.row; })[0] || {}).entityId;
    var known = S.formsMap && entity ? S.formsMap[entity] : null;
    var opts = [{ id: '', name: FIRST_FORM }].concat((known || []).filter(function (o) { return o && o.id; }));
    // A saved form the system did not list (or has not answered yet) stays
    // selectable so opening the builder never silently changes it.
    if (f.form && !opts.some(function (o) { return o.id === f.form; })) opts.push({ id: f.form, name: f.formName || f.form });
    var form = h('select', 'enc-sel', '', { 'aria-label': 'Form to take', 'data-k': 'ff-' + k, disabled: !edit }, {
      change: function () {
        var o = opts.filter(function (x) { return x.id === form.value; })[0] || { id: '', name: FIRST_FORM };
        change(S, setForm(S.draft, st.id, fi, { form: o.id, formName: o.name }), 'ff-' + k);
      }
    });
    opts.forEach(function (o) { form.appendChild(h('option', '', o.name, { value: o.id, selected: o.id === f.form })); });
    append(row, [who, form, edit ? h('button', 'enc-x enc-rm', '×', { type: 'button', 'aria-label': 'Remove this form change', 'data-k': 'rf-' + k }, {
      click: function () { change(S, removeForm(S.draft, st.id, fi), 'af-' + st.id); }
    }) : null]);
    if (S.formsMap && entity && (!known || !known.length)) {
      row.appendChild(h('span', 'enc-muted enc-formrow-hint', 'This creature has no other forms'));
    }
    return row;
  }

  // tellSystemLater refreshes a game system's panel after a typed edit, which
  // does not redraw the builder.
  function tellSystemLater(S) {
    var slot = S.drawer && S.drawer.aside.querySelector('.enc-system');
    if (slot) tellSystem(S, slot);
  }

  function readySection(S) {
    var lab = h('label', 'enc-readybox');
    var cb = h('input', '', '', { type: 'checkbox', 'data-k': 'ready', disabled: !S.canEdit }, {
      change: function () { var n = clone(S.draft); n.ready = cb.checked; change(S, n, 'ready'); }
    });
    cb.checked = S.draft.ready;
    var text = h('span');
    append(text, [h('b', '', 'Ready for game night'),
      h('span', 'enc-muted', 'Ready fights are the ones Foundry lists first. Untick it after the fight.')]);
    append(lab, [cb, text]);
    return lab;
  }

  // deleteButton asks once, in place, before deleting.
  function deleteButton(S) {
    var b = btn('Delete', 'enc-danger', function () {
      if (b.getAttribute('data-armed')) { remove(S, S.open); return; }
      b.setAttribute('data-armed', '1');
      b.textContent = 'Delete for good?';
      setTimeout(function () { if (b.isConnected) { b.removeAttribute('data-armed'); b.textContent = 'Delete'; } }, 4000);
    });
    return b;
  }
})();
