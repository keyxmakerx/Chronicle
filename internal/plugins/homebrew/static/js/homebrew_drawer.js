/**
 * homebrew_drawer.js: the Homebrew drawer on the DM Screen. It runs the
 * campaign's homebrew at the table: the events a Director sets off by hand,
 * the rules waiting on them, what the rules just changed, and every page's
 * trackers.
 *
 * The rule sections come from the homebrew plugin's /drawer and the pages
 * from the trackers plugin's /trackers/pages; each page's trackers are the
 * trackers widget itself, so stepping one here is the same change as on the
 * page. A waiting rule's browser steps run through Chronicle.homebrewActions,
 * which the plugin owning that action fills; one nobody filled is named for
 * the Director to do. A rule waiting at an "Ask the Director" step is drawn
 * as an answer card: Yes, No or Other, and after Yes the steps that follow
 * with their own fields and parts to change ("Yes, and…"); the server checks
 * each change with the step's owner. User text goes in with textContent,
 * never as markup. Styles are in ../css/drawer.css.
 */
(function () {
  'use strict';

  // Pages shown before "Show more", so a big campaign opens quickly.
  var PAGES_SHOWN = 8;

  // ---------- Pure helpers (tested in test/js/homebrew_drawer.test.mjs) ----------

  var STATUS = { done: 'Done', failed: 'Stopped', stopped: 'Stopped', dismissed: 'Set aside', running: 'Running', held: 'Waiting',
    declined: 'Said no', replied: 'Replied', withdrawn: 'Taken back', missed: 'Roll fell short' };

  function statusText(s) { return STATUS[s] || s; }

  // ago says when a run happened, in the words the drawer uses.
  function ago(at, now) {
    var t = Date.parse(at);
    if (isNaN(t)) return '';
    var min = Math.floor((now - t) / 60000);
    if (min < 1) return 'just now';
    if (min < 60) return min + ' min ago';
    var hr = Math.floor(min / 60);
    if (hr < 24) return hr + (hr === 1 ? ' hour ago' : ' hours ago');
    return new Date(t).toLocaleDateString();
  }

  // stepNote says what Run does with one step of a waiting rule. have says
  // whether this browser has a handler for a browser step.
  function stepNote(step, have) {
    if (step.rolls) return 'The player rolls this';
    if (step.byHand) return 'For you to do';
    if (step.browser) return have ? 'Runs here' : 'For you to do: nothing on this page can run it';
    return 'Runs on the server';
  }

  function pressLabel(p) {
    return p.label + (p.rules === 1 ? ' (1 rule)' : ' (' + p.rules + ' rules)');
  }

  // pressURL is where a press goes. A button the Director named carries its
  // name, since one event stands behind every named button.
  function pressURL(endpoint, p) {
    var url = endpoint + '/press/' + encodeURIComponent(p.event);
    return p.button ? url + '?button=' + encodeURIComponent(p.button) : url;
  }

  // pressRunning is the status while a press runs; a named button keeps the
  // Director's own spelling.
  function pressRunning(p) {
    return 'Running ' + (p.button ? p.label : p.label.toLowerCase()) + '…';
  }

  // askTitle is an ask's heading: "Mira wants to draw a card".
  function askTitle(ask) {
    var who = (ask && ask.asker) || 'A player';
    return ask && ask.ask ? who + ' wants to ' + ask.ask : who + ' is asking you';
  }

  // argValue reads one field of a step's args as the form shows it.
  function argValue(args, f) {
    var v = args ? args[f.name] : undefined;
    return v == null ? '' : String(v);
  }

  // editsFor turns the answer card's values into the steps a Yes changes:
  // only the steps whose part or fields differ from the rule, each with its
  // whole args, so the owner's Check sees what will run. values is keyed
  // by step, {part, args: {field: text}}; a whole-number field that is empty
  // is left out, and one that is not a whole number comes back as an error.
  function editsFor(later, values) {
    var out = [], bad = '';
    (later || []).forEach(function (st) {
      var v = values[st.step];
      if (!v) return;
      var args = {}, changed = false, orig = st.args || {};
      Object.keys(orig).forEach(function (k) { args[k] = orig[k]; });
      (st.fields || []).forEach(function (f) {
        if (!(f.name in v.args)) return;
        var txt = String(v.args[f.name]).trim();
        if (txt === argValue(orig, f)) return;
        changed = true;
        if (f.type === 'whole') {
          if (txt === '') { delete args[f.name]; return; }
          if (!/^-?\d+$/.test(txt)) { bad = bad || (f.label + ' needs a whole number.'); return; }
          args[f.name] = parseInt(txt, 10);
        } else if (txt === '') {
          delete args[f.name];
        } else {
          args[f.name] = txt;
        }
      });
      var edit = { step: st.step };
      if (v.part != null && v.part !== (st.part || '')) { edit.part = v.part; changed = true; }
      if (!changed) return;
      edit.args = args;
      out.push(edit);
    });
    return { steps: out, error: bad };
  }

  // sendLabel says what Send does for the answer picked.
  function sendLabel(choice, later, changes) {
    if (choice === 'no') return 'Send: no';
    if (choice === 'other') return 'Send reply';
    var names = (later || []).filter(function (st) { return !st.ask; }).map(function (st) { return st.label; });
    if (!names.length) return 'Send: yes';
    return 'Send: yes' + (changes ? ', with changes' : '') + ', then ' + names.join(', then ').toLowerCase();
  }

  var H = { statusText: statusText, ago: ago, stepNote: stepNote, pressLabel: pressLabel, pressURL: pressURL, pressRunning: pressRunning,
    askTitle: askTitle, editsFor: editsFor, sendLabel: sendLabel };
  if (typeof window !== 'undefined') window.HomebrewDrawer = H;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  // ---------- DOM ----------

  function h(tag, cls, text, attrs, on) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null && text !== '') e.textContent = text;
    if (attrs) Object.keys(attrs).forEach(function (k) {
      if (attrs[k] === false || attrs[k] == null) return;
      e.setAttribute(k, attrs[k] === true ? '' : attrs[k]);
    });
    if (on) Object.keys(on).forEach(function (k) { e.addEventListener(k, on[k]); });
    return e;
  }
  function append(parent, kids) {
    kids.forEach(function (k) { if (k) parent.appendChild(k); });
    return parent;
  }
  function fa(name) { return h('i', 'fa-solid ' + name, '', { 'aria-hidden': 'true' }); }
  function btn(label, cls, onclick, icon, disabled) {
    var b = h('button', 'hbd-btn' + (cls ? ' ' + cls : ''), '', { type: 'button', disabled: !!disabled }, { click: onclick });
    if (icon) b.appendChild(fa(icon));
    b.appendChild(document.createTextNode(label));
    return b;
  }

  function api(url, method, body, csrf) {
    return Chronicle.apiFetch(url, { method: method || 'GET', body: body, csrfToken: csrf }).then(function (res) {
      return res.text().then(function (txt) {
        var data = null;
        try { data = txt ? JSON.parse(txt) : null; } catch (e) { data = null; }
        if (!res.ok) {
          var err = new Error((data && (data.message || data.error)) || 'Something went wrong. Try again.');
          err.status = res.status;
          throw err;
        }
        return data;
      });
    });
  }

  function csrf(S) { return S.cfg.csrfToken || (Chronicle.getCsrf ? Chronicle.getCsrf() : ''); }

  Chronicle.register('homebrew-drawer', {
    init: function (el, cfg) {
      var S = { el: el, cfg: cfg, drawer: null, pages: null, allPages: false, busy: false, status: '', statusErr: false, answers: {} };
      el._hbd = S;
      // The Table's socket says when an ask is made, answered or taken back,
      // here or in another screen; the list is read again rather than trusted
      // from the message, which carries only an id.
      S.onAsk = function () { if (!S.busy && !S.editing) reload(S); };
      window.addEventListener('chronicle:homebrew-asks', S.onAsk);
      load(S);
    },
    destroy: function (el) {
      if (el._hbd) window.removeEventListener('chronicle:homebrew-asks', el._hbd.onAsk);
      el._hbd = null;
    }
  });

  function load(S) {
    var rules = api(S.cfg.endpoint + '/drawer').then(function (d) { S.drawer = d; }, function () { S.drawer = false; });
    // The trackers plugin may be off; the drawer then has no pages section.
    var pages = S.cfg.trackers
      ? api(S.cfg.trackers + '/pages').then(function (p) { S.pages = p || []; }, function () { S.pages = false; })
      : Promise.resolve();
    return Promise.all([rules, pages]).then(function () { draw(S); });
  }

  function setStatus(S, msg, isErr) {
    S.status = msg || ''; S.statusErr = !!isErr;
    var line = S.el.querySelector('.hbd-status');
    if (line) { line.textContent = S.status; line.classList.toggle('err', S.statusErr); }
  }

  function draw(S) {
    var el = S.el;
    el.textContent = '';
    if (S.drawer === false && S.pages === false) {
      el.appendChild(h('p', 'hbd-empty', 'The drawer could not load. Close it and open it again.'));
      return;
    }
    el.appendChild(h('p', 'hbd-status' + (S.statusErr ? ' err' : ''), S.status, { role: 'status', 'aria-live': 'polite' }));
    if (S.drawer) {
      el.appendChild(pressSection(S));
      el.appendChild(waitingSection(S));
      el.appendChild(recentSection(S));
    }
    if (S.pages) el.appendChild(pagesSection(S));
    // The trackers widgets drawn just now mount here; boot.js only scans
    // what htmx swaps.
    if (Chronicle.mountWidgets) Chronicle.mountWidgets(el);
  }

  function section(title, icon, cls) {
    var s = h('section', 'hbd-sec' + (cls ? ' ' + cls : ''), '', { 'aria-label': title });
    s.appendChild(append(h('h4', 'hbd-h'), [fa(icon), document.createTextNode(title)]));
    return s;
  }

  // ---------- Presses ----------

  function pressSection(S) {
    var s = section('Set off', 'fa-hand-pointer', 'hbd-dm');
    var presses = S.drawer.presses || [];
    if (!presses.length) {
      s.appendChild(h('p', 'hbd-muted', 'No saved rule starts from a button here. A rule whose When is "A respite is logged" adds one, and "A drawer button is pressed" adds a button with the name you give it.'));
      return s;
    }
    var row = h('div', 'hbd-row');
    presses.forEach(function (p) {
      row.appendChild(btn(pressLabel(p), 'primary', function () { press(S, p); }, 'fa-play', S.busy));
    });
    s.appendChild(row);
    return s;
  }

  function press(S, p) {
    S.busy = true;
    setStatus(S, pressRunning(p));
    api(pressURL(S.cfg.endpoint, p), 'POST', null, csrf(S)).then(function (d) {
      S.busy = false; S.drawer = d;
      S.status = p.label + ': done. What changed is below.'; S.statusErr = false;
      draw(S);
    }).catch(function (e) {
      S.busy = false; setStatus(S, e.message, true); draw(S);
    });
  }

  // ---------- Waiting on you ----------

  function waitingSection(S) {
    var s = section('Waiting on you', 'fa-hourglass-half', 'hbd-dm');
    var runs = S.drawer.waiting || [];
    if (!runs.length) {
      s.appendChild(h('p', 'hbd-muted', 'Nothing is waiting. A rule that is held, or runs in your browser, waits here.'));
      return s;
    }
    // A roll waits on a player, not on you, so it is not counted.
    var needs = runs.filter(function (r) { return r.kind !== 'roll'; }).length;
    if (needs) s.querySelector('.hbd-h').appendChild(h('span', 'hbd-count', needs === 1 ? '1 needs you' : needs + ' need you'));
    // Asks first, the first asked at the top: a player is waiting on them.
    var asks = runs.filter(function (r) { return r.ask; }).reverse();
    asks.forEach(function (r) { s.appendChild(askCard(S, r)); });
    runs.forEach(function (r) { if (!r.ask) s.appendChild(waitingCard(S, r)); });
    return s;
  }

  // ---------- An ask: the answer card ----------

  // answerState is what the Director has picked and typed on one ask, kept
  // across redraws so a live reload does not wipe it.
  function answerState(S, r) {
    var a = S.answers[r.id];
    if (!a) a = S.answers[r.id] = { choice: '', reply: '', values: {}, more: false, error: '' };
    return a;
  }

  function askCard(S, r) {
    var ask = r.ask, A = answerState(S, r);
    var later = (ask.later || []).filter(function (st) { return st.action !== 'ask-director'; });
    var c = h('div', 'hbd-card hbd-ask', '', { role: 'group', 'aria-label': askTitle(ask), 'data-hbd-ask': r.id });
    c.appendChild(h('h5', 'hbd-ask-title', askTitle(ask)));
    c.appendChild(h('p', 'hbd-muted', 'From the ' + r.recipe + ' recipe · ' + r.what));
    var pick = h('div', 'hbd-row hbd-pick', '', { role: 'group', 'aria-label': 'Your answer' });
    [['yes', 'Yes'], ['no', 'No']].forEach(function (o) {
      pick.appendChild(h('button', 'hbd-btn hbd-choice', o[1], { type: 'button', 'aria-pressed': String(A.choice === o[0]), disabled: S.busy },
        { click: function () { A.choice = o[0]; A.error = ''; draw(S); focusPick(S, r, o[0]); } }));
    });
    pick.appendChild(h('button', 'hbd-btn quiet hbd-other', 'Other…', { type: 'button', 'aria-pressed': String(A.choice === 'other'), disabled: S.busy },
      { click: function () { A.choice = 'other'; A.error = ''; draw(S); var t = S.el.querySelector('[data-hbd-reply="' + r.id + '"]'); if (t) t.focus(); } }));
    c.appendChild(pick);
    if (A.choice === 'yes' && later.length) c.appendChild(conditions(S, r, A, later));
    if (A.choice === 'other') {
      var ta = h('textarea', 'hbd-reply', '', { rows: 2, maxlength: 200, 'aria-label': 'Your reply to ' + ((ask.asker) || 'the player'),
        placeholder: 'Not yet: finish the fight first.', 'data-hbd-reply': r.id }, {
        input: function () { A.reply = ta.value; }, focus: function () { S.editing = true; }, blur: function () { S.editing = false; }
      });
      ta.value = A.reply;
      c.appendChild(ta);
    }
    if (A.error) c.appendChild(h('p', 'hbd-err', A.error, { role: 'alert' }));
    if (A.choice) {
      var ed = A.choice === 'yes' ? editsFor(ask.later, A.values) : { steps: [] };
      var send = btn(sendLabel(A.choice, later, ed.steps.length > 0), 'primary hbd-send', function () { answer(S, r, send); }, null, S.busy);
      send.setAttribute('data-working-label', 'Sending…');
      send.setAttribute('data-hbd-send', r.id);
      c.appendChild(send);
      c.appendChild(h('p', 'hbd-muted', (ask.asker || 'The player') + ' sees your answer' + (A.choice === 'yes' ? ' and what you changed.' : '.')));
    }
    return c;
  }

  function focusPick(S, r, choice) {
    var b = S.el.querySelector('[data-hbd-ask="' + r.id + '"] .hbd-choice[aria-pressed="true"]');
    if (b) b.focus();
  }

  // conditions draws "Yes, and…": for each step after the ask, its part and
  // its fields, as the rule has them, to change before sending. Whole-number
  // fields wait behind "Change how many" so the common answer stays short.
  function conditions(S, r, A, later) {
    var box = h('div', 'hbd-and', '', { role: 'group', 'aria-label': 'Yes, and…' });
    box.appendChild(h('p', 'hbd-and-h', 'Yes, and…'));
    var wholes = 0;
    later.forEach(function (st) {
      var v = A.values[st.step] || (A.values[st.step] = { part: null, args: {} });
      box.appendChild(h('p', 'hbd-and-step', st.label));
      if (st.parts && st.parts.length > 1) {
        var sel = h('select', 'hbd-input', '', { 'aria-label': st.label + ': on' }, { change: function () { v.part = sel.value; refreshSend(S, r, A, later); } });
        st.parts.forEach(function (p) {
          var o = h('option', '', p.label, { value: p.key });
          if (p.key === (v.part != null ? v.part : st.part)) o.selected = true;
          sel.appendChild(o);
        });
        box.appendChild(condRow('On', sel));
      }
      (st.fields || []).forEach(function (f) {
        if (f.type === 'whole') { wholes++; if (!A.more) return; }
        var cur = f.name in v.args ? v.args[f.name] : argValue(st.args, f);
        var inp = h('input', 'hbd-input', '', { type: f.type === 'whole' ? 'number' : 'text', inputmode: f.type === 'whole' ? 'numeric' : null,
          step: f.type === 'whole' ? '1' : null, 'aria-label': st.label + ': ' + f.label, placeholder: f.type === 'whole' ? '' : 'Any' }, {
          input: function () { v.args[f.name] = inp.value; refreshSend(S, r, A, later); },
          focus: function () { S.editing = true; }, blur: function () { S.editing = false; }
        });
        inp.value = cur;
        box.appendChild(condRow(f.label, inp));
      });
    });
    if (wholes && !A.more) {
      box.appendChild(h('button', 'hbd-chip', '+ Change how many', { type: 'button' }, { click: function () { A.more = true; draw(S); } }));
    }
    return box;
  }

  function condRow(label, control) {
    return append(h('label', 'hbd-cond'), [h('span', 'hbd-cond-l', label), control]);
  }

  // refreshSend keeps the Send label true to the changes while typing,
  // without redrawing the field being typed in.
  function refreshSend(S, r, A, later) {
    var b = S.el.querySelector('[data-hbd-send="' + r.id + '"]');
    if (b) b.lastChild.textContent = sendLabel('yes', later, editsFor(r.ask.later, A.values).steps.length > 0);
  }

  // laterRun is the part of a waiting ask that a Yes runs in this browser:
  // the steps after the ask, with the Director's changes, for browserSteps.
  function laterRun(r, steps) {
    var edits = {};
    (steps || []).forEach(function (e) { edits[e.step] = e; });
    var out = { id: r.id, recipeId: r.recipeId, scope: r.scope, steps: [] };
    (r.steps || []).forEach(function (st, i) {
      if (i <= r.ask.step) return;
      var e = edits[i], copy = {};
      Object.keys(st).forEach(function (k) { copy[k] = st[k]; });
      if (e) { if (e.args) copy.args = e.args; if (e.part != null) copy.part = e.part; }
      out.steps.push(copy);
    });
    return out;
  }

  function answer(S, r, button) {
    var A = answerState(S, r);
    var body = { answer: A.choice };
    if (A.choice === 'other') {
      if (!A.reply.trim()) { A.error = 'Write the reply ' + (r.ask.asker || 'the player') + ' gets.'; draw(S); return; }
      body.reply = A.reply.trim();
    }
    if (A.choice === 'yes') {
      var ed = editsFor(r.ask.later, A.values);
      if (ed.error) { A.error = ed.error; draw(S); return; }
      if (ed.steps.length) body.steps = ed.steps;
    }
    S.busy = true;
    if (Chronicle.working) Chronicle.working(button, true);
    api(S.cfg.endpoint + '/runs/' + encodeURIComponent(r.id) + '/answer', 'POST', JSON.stringify(body), csrf(S)).then(function (done) {
      delete S.answers[r.id];
      var failed = done && done.status === 'failed';
      var after = A.choice === 'yes' && !failed && done.status !== 'held' ? browserSteps(S, laterRun(r, body.steps)) : Promise.resolve([]);
      return after.then(function (extra) {
        var said = A.choice === 'yes' ? 'Sent: yes.' : A.choice === 'no' ? 'Sent: no.' : 'Reply sent.';
        S.status = said + (failed ? ' ' + (done.lines || []).slice(-1)[0] : '') + (extra.length ? ' ' + extra.join(' ') : '');
        S.statusErr = failed;
      });
    }).catch(function (e) {
      // A refused change keeps the card open with the owner's own words.
      A.error = e.message;
    }).then(function () { S.busy = false; return reload(S); });
  }

  function waitingCard(S, r) {
    var c = h('div', 'hbd-card', '', { 'aria-label': r.recipe + ': ' + r.what });
    c.appendChild(append(h('div', 'hbd-ch'), [h('strong', '', r.recipe), h('span', 'hbd-when', ago(r.at, Date.now()))]));
    c.appendChild(h('p', '', r.what));
    if (r.waits) c.appendChild(h('p', 'hbd-muted', r.waits));
    var acts = Chronicle.homebrewActions || {};
    if (r.steps && r.steps.length) {
      var ul = h('ul', 'hbd-steps');
      r.steps.forEach(function (st) {
        ul.appendChild(append(h('li'), [h('span', '', st.label), h('span', 'hbd-muted', ' · ' + stepNote(st, typeof acts[st.action] === 'function'))]));
      });
      c.appendChild(ul);
    }
    // Only the player can make a roll the rule waits on; you can set it aside.
    c.appendChild(append(h('div', 'hbd-row'), r.kind === 'roll' ? [
      btn('Set aside', '', function () { dismiss(S, r); }, null, S.busy)
    ] : [
      btn('Run', 'primary', function () { runHeld(S, r); }, 'fa-play', S.busy),
      btn('Set aside', '', function () { dismiss(S, r); }, null, S.busy)
    ]));
    return c;
  }

  function runHeld(S, r) {
    S.busy = true; setStatus(S, 'Running…');
    api(S.cfg.endpoint + '/runs/' + encodeURIComponent(r.id) + '/run', 'POST', null, csrf(S)).then(function (done) {
      // A rule stops at its first failing step, so a failed server step
      // leaves the browser steps after it unrun too.
      var failed = done && done.status === 'failed';
      return (failed ? Promise.resolve([]) : browserSteps(S, r)).then(function (extra) {
        var lines = (done && done.lines || []).concat(extra);
        S.status = lines.join(' ') || 'Done.'; S.statusErr = failed;
      });
    }).catch(function (e) {
      S.status = e.message; S.statusErr = true;
    }).then(function () { S.busy = false; return reload(S); });
  }

  // browserSteps runs a held rule's browser steps after the server ran the
  // rest. One that fails is reported and the others still run.
  function browserSteps(S, r) {
    var acts = Chronicle.homebrewActions || {};
    var out = [];
    var chain = Promise.resolve();
    (r.steps || []).forEach(function (st) {
      if (!st.browser || typeof acts[st.action] !== 'function') return;
      chain = chain.then(function () {
        return Promise.resolve(acts[st.action]({ campaignId: S.cfg.campaignId, run: r, step: st })).then(function (line) {
          if (line) out.push(String(line));
        }, function (e) {
          out.push(st.label + ' did not run: ' + ((e && e.message) || 'something went wrong.'));
        });
      });
    });
    return chain.then(function () { return out; });
  }

  function dismiss(S, r) {
    S.busy = true; setStatus(S, 'Setting it aside…');
    api(S.cfg.endpoint + '/runs/' + encodeURIComponent(r.id) + '/dismiss', 'POST', null, csrf(S)).then(function () {
      S.status = 'Set aside.'; S.statusErr = false;
    }).catch(function (e) {
      S.status = e.message; S.statusErr = true;
    }).then(function () { S.busy = false; return reload(S); });
  }

  function reload(S) {
    return api(S.cfg.endpoint + '/drawer').then(function (d) { S.drawer = d; draw(S); }, function () { draw(S); });
  }

  // ---------- What just changed ----------

  function recentSection(S) {
    var s = section('What just changed', 'fa-clock-rotate-left', 'hbd-dm');
    var runs = S.drawer.recent || [];
    if (!runs.length) {
      s.appendChild(h('p', 'hbd-muted', 'No rule has run yet.'));
      return s;
    }
    var ul = h('ul', 'hbd-log');
    var now = Date.now();
    runs.forEach(function (r) {
      var li = h('li', 'hbd-run hbd-' + r.status);
      li.appendChild(append(h('div', 'hbd-ch'), [
        h('strong', '', r.recipe), h('span', 'hbd-pill', statusText(r.status)), h('span', 'hbd-when', ago(r.at, now))
      ]));
      li.appendChild(h('p', 'hbd-muted', r.what));
      (r.lines || []).forEach(function (l) { li.appendChild(h('p', '', l)); });
      ul.appendChild(li);
    });
    s.appendChild(ul);
    return s;
  }

  // ---------- Trackers by page ----------

  function pagesSection(S) {
    var s = section('Trackers', 'fa-list-check');
    if (!S.pages.length) {
      // The Trackers block is placed by the shared chooser (put_on_page.js),
      // which loads on sight from this wrapper. Placing it changes a page
      // layout, which only the owner may do, so a co-DM reads the line plain.
      if (S.cfg.canPlace !== true) {
        s.appendChild(h('p', 'hbd-muted', 'No page has a tracker yet. Put a Trackers block on a page and it shows here.'));
        return s;
      }
      var line = h('p', 'hbd-muted', 'No page has a tracker yet. ', { 'data-widget': 'put-on-page', 'data-campaign-id': S.cfg.campaignId });
      line.appendChild(h('button', 'hbd-btn quiet', 'Put a Trackers block on a page', { type: 'button', 'data-put-block': 'trackers', 'aria-haspopup': 'dialog', 'aria-expanded': 'false' }));
      line.appendChild(document.createTextNode(' and it shows here.'));
      s.appendChild(line);
      return s;
    }
    var shown = S.allPages ? S.pages : S.pages.slice(0, PAGES_SHOWN);
    shown.forEach(function (p) {
      var box = h('div', 'hbd-page');
      box.appendChild(h('a', 'hbd-page-name', p.name, { href: '/campaigns/' + S.cfg.campaignId + '/entities/' + p.id }));
      box.appendChild(h('div', 'tk', '', {
        'data-widget': 'trackers', 'data-campaign-id': S.cfg.campaignId,
        'data-endpoint': S.cfg.trackers + '/page/' + p.id, 'data-csrf-token': csrf(S)
      }));
      s.appendChild(box);
    });
    if (!S.allPages && S.pages.length > PAGES_SHOWN) {
      s.appendChild(btn('Show ' + (S.pages.length - PAGES_SHOWN) + ' more pages', 'quiet', function () { S.allPages = true; draw(S); }));
    }
    return s;
  }
})();
