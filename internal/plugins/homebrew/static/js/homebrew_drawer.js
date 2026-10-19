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
 * the Director to do. User text goes in with textContent, never as markup.
 * Styles are in ../css/drawer.css.
 */
(function () {
  'use strict';

  // Pages shown before "Show more", so a big campaign opens quickly.
  var PAGES_SHOWN = 8;

  // ---------- Pure helpers (tested in test/js/homebrew_drawer.test.mjs) ----------

  var STATUS = { done: 'Done', failed: 'Stopped', stopped: 'Stopped', dismissed: 'Set aside', running: 'Running', held: 'Waiting' };

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
    if (step.byHand) return 'For you to do';
    if (step.browser) return have ? 'Runs here' : 'For you to do: nothing on this page can run it';
    return 'Runs on the server';
  }

  function pressLabel(p) {
    return p.label + (p.rules === 1 ? ' (1 rule)' : ' (' + p.rules + ' rules)');
  }

  var H = { statusText: statusText, ago: ago, stepNote: stepNote, pressLabel: pressLabel };
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
      var S = { el: el, cfg: cfg, drawer: null, pages: null, allPages: false, busy: false, status: '', statusErr: false };
      el._hbd = S;
      load(S);
    },
    destroy: function (el) { el._hbd = null; }
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
      s.appendChild(h('p', 'hbd-muted', 'No saved rule starts from a button here. A rule whose When is "Log a respite" adds one.'));
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
    setStatus(S, 'Running ' + p.label.toLowerCase() + '…');
    api(S.cfg.endpoint + '/press/' + encodeURIComponent(p.event), 'POST', null, csrf(S)).then(function (d) {
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
    runs.forEach(function (r) { s.appendChild(waitingCard(S, r)); });
    return s;
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
    c.appendChild(append(h('div', 'hbd-row'), [
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
      s.appendChild(h('p', 'hbd-muted', 'No page has a tracker yet. Place a recipe or a Trackers block on a page and it shows here.'));
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
