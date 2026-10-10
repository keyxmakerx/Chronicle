/**
 * own_entries.js — the Directors' "Your own entries" page.
 *
 * A page in the site's plain look with a tab per pick list the game system
 * has (Ancestry, Kit…, named by the system). Opening an entry, or "Add",
 * grows an editor over the page from the row clicked: the form on the left,
 * and on the right what a player sees in the hero creator, where the points
 * can be test-spent. The form has the parts the system's own entries share
 * (facts, lists of traits with costs, the points budget), so a homebrew
 * ancestry is bought from in the creator exactly like a packaged one.
 * "Paste from an AI" reads the AI Import format into the form for checking.
 *
 * The pick lists, their labels and the system's own entries come from the
 * creator's plan; the campaign's entries are read and written through the
 * systems plugin's entry API, which also enforces the Director rule and
 * validates every list. Descriptions are HTML the server sanitizes; a
 * passage wrapped in <span data-secret> is kept from players, as on pages.
 *
 * Served from the plugin body-script registry so it survives boosted
 * navigation. Re-wires after HTMX swaps.
 */
(function () {
  'use strict';

  var DIRTY = 'own-entry';

  function esc(v) {
    var s = (v == null) ? '' : String(v);
    return s.replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }
  function words(key) {
    var s = String(key).replace(/_display$/, '').replace(/[_-]+/g, ' ').trim();
    return s.charAt(0).toUpperCase() + s.slice(1);
  }
  function lower(s) { return String(s || '').toLowerCase(); }
  function an(w) { return /^[aeiou]/i.test(w) ? 'an' : 'a'; }
  // A fact's label becomes the property key the creator reads ("Size" → size).
  function keyOf(label) {
    var k = lower(label).replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    if (!/^[a-z]/.test(k)) k = 'f_' + k;
    return k.slice(0, 40);
  }
  function isFact(v) { return typeof v === 'number' || (typeof v === 'string' && v.length > 0 && v.length <= 40); }
  function dirty(on) {
    if (!window.Chronicle) return;
    if (on && Chronicle.markDirty) Chronicle.markDirty(DIRTY);
    if (!on && Chronicle.markClean) Chronicle.markClean(DIRTY);
  }
  function notify(msg, kind) { if (window.Chronicle && Chronicle.notify) Chronicle.notify(msg, kind || 'success'); }
  function api(url, opts) {
    if (window.Chronicle && Chronicle.apiFetch) return Chronicle.apiFetch(url, opts);
    var o = { method: opts && opts.method || 'GET', credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (opts && opts.body) { o.headers['Content-Type'] = 'application/json'; o.body = JSON.stringify(opts.body); }
    if (opts && opts.csrfToken) o.headers['X-CSRF-Token'] = opts.csrfToken;
    return fetch(url, o);
  }
  function json(r) {
    return r.json().catch(function () { return {}; }).then(function (b) { return { ok: r.ok, status: r.status, body: b }; });
  }

  // The facts a system's own entries share: scalar properties present on at
  // least half of them, budget keys and display twins aside, most common
  // first, at most four. A new entry starts with these lines.
  function suggestedFacts(pkg) {
    var count = {};
    pkg.forEach(function (c) {
      Object.keys(c.properties || {}).forEach(function (k) {
        if (/(_display|_points)$/.test(k) || !isFact(c.properties[k])) return;
        count[k] = (count[k] || 0) + 1;
      });
    });
    return Object.keys(count).filter(function (k) { return count[k] * 2 >= pkg.length; })
      .sort(function (a, b) { return count[b] - count[a] || (a < b ? -1 : 1); }).slice(0, 4);
  }

  // A list of named items, as the creator reads one (creator's namedList).
  function namedList(v) {
    return Array.isArray(v) && v.length > 0 && v.every(function (it) { return it && typeof it === 'object' && typeof it.name === 'string'; });
  }
  // The shape a system's own entries share, so a Director's entry can carry
  // the same parts: scalar facts, lists of named items that at least half of
  // them have (a list whose items cost points is one a hero buys from), and
  // the points budget, read as the creator does from a "*_points" number.
  function shapeOf(pkg) {
    var lists = {}, budgets = {};
    pkg.forEach(function (c) {
      var p = c.properties || {};
      Object.keys(p).forEach(function (k) {
        if (namedList(p[k])) {
          var l = lists[k] || (lists[k] = { n: 0, buy: 0 });
          l.n++;
          if (p[k].every(function (it) { return typeof it.cost === 'number'; })) l.buy++;
        } else if (/_points$/.test(k) && typeof p[k] === 'number' && p[k] > 0) {
          var b = budgets[k] || (budgets[k] = {});
          b[p[k]] = (b[p[k]] || 0) + 1;
        }
      });
    });
    var keys = Object.keys(lists).filter(function (k) { return lists[k].n * 2 >= pkg.length; }).sort();
    // What every hero gets comes before what they buy, as in the creator.
    var shape = { facts: suggestedFacts(pkg), lists: keys.map(function (k) {
      return { key: k, label: words(k), buy: lists[k].buy * 2 >= lists[k].n };
    }).sort(function (a, b) { return a.buy - b.buy; }), budgetKey: '', budget: 0 };
    var bk = Object.keys(budgets).sort()[0];
    if (bk && shape.lists.some(function (l) { return l.buy; })) {
      shape.budgetKey = bk;
      shape.budget = +Object.keys(budgets[bk]).sort(function (a, b) { return budgets[bk][b] - budgets[bk][a]; })[0];
    }
    return shape;
  }

  function Entries(root) {
    this.root = root;
    this.planUrl = root.getAttribute('data-plan-url');
    this.entriesUrl = root.getAttribute('data-entries-url');
    this.csrf = root.getAttribute('data-csrf') || '';
    this.system = '';
    this.fields = [];   // {key, label, pkg, facts, lists, budgetKey, budget, sample}
    this.entries = [];  // the campaign's own, every field
    this.field = 0;
    this.open = null;   // entry id, or 'new'
    this.draft = null;
    this.confirm = false;
    this.paste = false;
    this.tryBuy = [];   // traits ticked in the player's view, never saved
    this.busy = false;
  }

  Entries.prototype.load = function () {
    var self = this;
    Promise.all([api(this.planUrl).then(json), api(this.entriesUrl).then(json)]).then(function (res) {
      if (!res[0].ok || !res[1].ok) throw new Error('load');
      var plan = res[0].body || {};
      self.system = plan.systemName || 'The game system';
      self.fields = (plan.steps || []).map(function (st) {
        var pkg = (st.choices || []).filter(function (c) { return c.source !== 'campaign'; });
        var sh = shapeOf(pkg);
        return { key: st.fieldKey, label: st.label, pkg: pkg.length, facts: sh.facts, lists: sh.lists,
          budgetKey: sh.budgetKey, budget: sh.budget, sample: pkg[0] || null };
      });
      self.entries = (res[1].body && res[1].body.entries) || [];
      self.render();
    }).catch(function () {
      var t = self.root.querySelector('.hc-loading__text');
      if (t) t.textContent = 'Your entries could not be opened. Reload the page to try again.';
    });
  };

  Entries.prototype.cur = function () { return this.fields[this.field]; };
  Entries.prototype.mine = function (key) {
    return this.entries.filter(function (e) { return e.fieldKey === key; })
      .sort(function (a, b) { return lower(a.name) < lower(b.name) ? -1 : 1; });
  };

  function plural(w) {
    w = lower(w);
    if (/[^aeiou]y$/.test(w)) return w.slice(0, -1) + 'ies';
    if (/(s|x|z|ch|sh)$/.test(w)) return w + 'es';
    return w + 's';
  }
  // Numbers stay numbers so the creator can do sums with them ("3" points).
  function factValue(v) { v = v.trim(); return /^-?\d+(\.\d+)?$/.test(v) ? Number(v) : v; }
  function itemOf(it) {
    return { name: String(it.name || ''), cost: typeof it.cost === 'number' ? it.cost : 1, description: String(it.description || '') };
  }
  // A draft holds facts as label/value lines, each shaped list by its key,
  // and the points budget; lists the system doesn't share are kept as they are.
  function draftOf(e, f) {
    var lists = {};
    f.lists.forEach(function (l) { lists[l.key] = []; });
    if (!e) {
      return { id: '', name: '', summary: '', text: '', who: 'everyone', budget: f.budget, lists: lists,
        facts: f.facts.map(function (k) { return { label: words(k), value: '', key: k }; }) };
    }
    var props = e.properties || {}, facts = [], budget = f.budget;
    Object.keys(props).forEach(function (k) {
      var v = props[k];
      if (namedList(v)) lists[k] = v.map(itemOf);
      else if (k === f.budgetKey && typeof v === 'number') budget = v;
      else if (!Array.isArray(v) && v !== null && typeof v !== 'object') facts.push({ label: words(k), value: String(v), key: k });
    });
    return { id: e.id, name: e.name || '', summary: e.summary || '', text: e.description || '', who: e.visibility || 'everyone',
      budget: budget, lists: lists, facts: facts };
  }
  // The writing box's HTML reduced to the few tags it can make, parsed in an
  // inert document and stripped of every attribute but data-secret, so the
  // preview never runs anything dropped into the box. The server sanitizes
  // what is saved on its own.
  var KEEP = { P: 1, BR: 1, B: 1, STRONG: 1, I: 1, EM: 1, U: 1, SPAN: 1, DIV: 1, UL: 1, OL: 1, LI: 1 };
  function tidy(html) {
    var doc = new DOMParser().parseFromString('<div>' + (html || '') + '</div>', 'text/html');
    var root = doc.body.firstChild;
    (function walk(el) {
      Array.prototype.slice.call(el.childNodes).forEach(function (n) {
        if (n.nodeType === 3) return;
        if (n.nodeType !== 1) { n.remove(); return; }
        walk(n);
        if (!KEEP[n.tagName]) {
          if (/^(SCRIPT|STYLE|IFRAME|OBJECT|EMBED|TEMPLATE|SVG|MATH)$/i.test(n.tagName)) n.remove();
          else n.replaceWith.apply(n, Array.prototype.slice.call(n.childNodes));
          return;
        }
        var secret = n.hasAttribute('data-secret');
        Array.prototype.slice.call(n.attributes).forEach(function (a) { n.removeAttribute(a.name); });
        if (secret) n.setAttribute('data-secret', 'true');
      });
    })(root);
    return root;
  }
  function sameDraft(a, b) { return JSON.stringify(a) === JSON.stringify(b); }

  // The page, and the editor that grows over it from the row clicked.
  Entries.prototype.render = function () {
    var open = this.open !== null;
    this.root.innerHTML = '<div class="oe-desk' + (open ? ' is-open' : '') + '"><div class="hc-wrap paper-stack">' + this.pageHTML() + '</div>' +
      '<div class="oe-scrim" data-act="close" aria-hidden="true"></div>' +
      '<section class="oe-pop" role="dialog" aria-modal="true" aria-label="Entry"' + (open ? '' : ' hidden') + '></section></div>';
    if (open) this.fillPop();
  };

  Entries.prototype.pageHTML = function () {
    var self = this, f = this.cur();
    var head = '<div class="hc-head"><p class="paper-kind">' + esc(this.system) + ' · Directors only</p><h1 tabindex="-1">Your own entries</h1>' +
      '<p>Entries you add here sit beside ' + esc(this.system) + '’s own in the hero creator and the sheet’s pick lists. A system update never changes them.</p></div>';
    if (!f) {
      return '<section class="hc-page paper" aria-label="Your own entries">' + head +
        '<p class="hc-empty">' + esc(this.system) + ' doesn’t list any choices for making a hero, so there is nothing to add to here.</p></section>';
    }
    var tabs = this.fields.map(function (x, i) {
      return '<button type="button" role="tab" data-field="' + i + '" aria-selected="' + (i === self.field) + '">' + esc(x.label) +
        '<small>' + self.mine(x.key).length + '</small></button>';
    }).join('');
    var mine = this.mine(f.key), pl = plural(f.label);
    var rows = mine.length ? '<ul class="oe-list">' + mine.map(function (e) {
      return '<li><button type="button" class="hc-row" data-open="' + esc(e.id) + '" aria-expanded="' + (self.open === String(e.id)) + '"><b>' + esc(e.name) +
        (e.visibility === 'directors' ? ' <span class="hc-seal oe-seal--dm">Directors only</span>' : '') +
        '</b><span class="hc-row__go" aria-hidden="true">Edit ›</span><small>' + esc(e.summary) + '</small></button></li>';
    }).join('') + '</ul>'
      : '<p class="hc-empty">No ' + esc(pl) + ' of your own yet. Add one and players see it next to ' + esc(this.system) + '’s ' + esc(pl) + ' when they make a hero.</p>';
    return '<section class="hc-page paper" aria-label="Your own entries">' + head +
      '<div class="oe-fields" role="tablist" aria-label="Pick lists">' + tabs + '</div>' + rows +
      '<p class="oe-pkg">' + esc(this.system) + ' brings ' + f.pkg + ' ' + esc(f.pkg === 1 ? lower(f.label) : pl) + ' of its own; they are not changed here.</p>' +
      '<div class="hc-foot"><span>' + mine.length + ' of your own</span><span class="hc-foot__sp"></span>' +
      '<button type="button" class="hc-btn" data-act="add">+ Add ' + an(f.label) + ' ' + esc(lower(f.label)) + '</button></div></section>';
  };

  function sing(label) { return lower(label).replace(/ies$/, 'y').replace(/s$/, ''); }
  function costSeg(li, ii, c) {
    return '<span class="oe-cost" role="group" aria-label="Cost">' + [1, 2, 3].map(function (n) {
      return '<button type="button" data-cost="' + n + '" data-li="' + esc(li) + '" data-ii="' + ii + '" aria-pressed="' + (c === n) + '">' + n + ' pt' + (n > 1 ? 's' : '') + '</button>';
    }).join('') + '</span>';
  }

  Entries.prototype.writeHTML = function (d) {
    var f = this.cur();
    var facts = d.facts.map(function (p, i) {
      return '<input aria-label="Fact name" data-fact="' + i + '" data-part="label" maxlength="40" value="' + esc(p.label) + '" placeholder="Name, e.g. Size">' +
        '<input aria-label="Fact value" data-fact="' + i + '" data-part="value" maxlength="200" value="' + esc(p.value) + '" placeholder="Value">' +
        '<button type="button" class="oe-rm" data-act="rmfact" data-i="' + i + '" aria-label="Remove ' + esc(p.label || 'this fact') + '">×</button>';
    }).join('');
    var hidden = /data-secret/.test(d.text);
    var lists = f.lists.map(function (l) {
      var items = d.lists[l.key] || [];
      return '<div class="oe-f"><span class="oe-k">' + esc(l.label) + '</span>' + items.map(function (it, i) {
        return '<div class="oe-item"><input class="oe-item__name" aria-label="' + esc(sing(l.label)) + ' name" data-li="' + esc(l.key) + '" data-ii="' + i + '" data-part="name" maxlength="100" value="' + esc(it.name) + '" placeholder="Name">' +
          (l.buy ? costSeg(l.key, i, it.cost) : '') +
          '<button type="button" class="oe-rm" data-act="rmitem" data-li="' + esc(l.key) + '" data-ii="' + i + '" aria-label="Remove ' + esc(it.name || sing(l.label)) + '">×</button>' +
          '<textarea rows="2" aria-label="What it does" data-li="' + esc(l.key) + '" data-ii="' + i + '" data-part="description" maxlength="2000" placeholder="What it does">' + esc(it.description) + '</textarea></div>';
      }).join('') + '<div><button type="button" class="oe-link" data-act="additem" data-li="' + esc(l.key) + '">+ Add ' + an(sing(l.label)) + ' ' + esc(sing(l.label)) + '</button></div></div>';
    }).join('');
    var budget = f.budgetKey ? '<div class="oe-bud"><span class="oe-k" id="oe-bud-k">' + esc(words(f.budgetKey)) + '</span>' +
      '<span class="oe-step" role="group" aria-labelledby="oe-bud-k"><button type="button" data-act="minus" aria-label="One point fewer">−</button>' +
      '<output>' + d.budget + '</output><button type="button" data-act="plus" aria-label="One point more">+</button></span>' +
      '<span class="oe-hint">Points a hero spends on ' + esc(lower(f.lists.filter(function (l) { return l.buy; })[0].label)) + '.</span></div>' : '';
    return '<div class="oe-ed">' +
      '<label class="oe-f"><span class="oe-k">Name</span><input class="oe-big" data-k="name" maxlength="100" autocomplete="off" value="' + esc(d.name) + '" placeholder="' + esc(f.label) + ' name"></label>' +
      '<label class="oe-f"><span class="oe-k">One line for the list</span><input data-k="summary" maxlength="300" autocomplete="off" value="' + esc(d.summary) + '" placeholder="What a player reads before opening it"></label>' +
      '<div class="oe-f"><span class="oe-k" id="oe-text-k">The whole entry</span><div class="oe-tool">' +
        '<button type="button" data-act="bold" title="Bold"><b>B</b></button><button type="button" data-act="italic" title="Italic"><i>I</i></button>' +
        '<button type="button" data-act="secret">Hide from players</button></div>' +
        '<div class="oe-text" contenteditable="true" role="textbox" aria-multiline="true" aria-labelledby="oe-text-k" data-k="text" data-placeholder="Write it as you would a page.">' + d.text + '</div>' +
        '<span class="oe-hint" data-hint>' + (hidden ? '<b>Part of this entry is hidden from players.</b> Directors still read it.'
          : 'Select a passage and press “Hide from players” to keep it for Directors.') + '</span></div>' +
      '<div class="oe-f"><span class="oe-k">Facts</span>' + (d.facts.length ? '<div class="oe-facts">' + facts + '</div>' : '') +
        '<div><button type="button" class="oe-link" data-act="addfact">+ Add a fact</button></div></div>' +
      budget + lists +
      '<fieldset class="oe-f oe-who"><legend class="oe-k">Who sees it</legend>' +
        '<label><input type="radio" name="oe-who" value="everyone"' + (d.who !== 'directors' ? ' checked' : '') + '><span>Everyone</span>' +
          '<small>Players can pick it in the hero creator and on their sheet.</small></label>' +
        '<label><input type="radio" name="oe-who" value="directors"' + (d.who === 'directors' ? ' checked' : '') + '><span>Directors only</span>' +
          '<small>For ' + an(f.label) + ' ' + esc(lower(f.label)) + ' the party hasn’t discovered yet. Switch it to Everyone when they do.</small></label></fieldset>' +
      '</div>';
  };

  // The system's rules for an entry, read back. Each is [message, blocks]:
  // what the server would refuse blocks Save; a budget a hero can't spend
  // is only a warning, since a Director may mean it.
  Entries.prototype.problems = function (d) {
    var f = this.cur(), out = [];
    if (!d.name.trim()) out.push(['Give it a name.', true]);
    f.lists.forEach(function (l) {
      var items = d.lists[l.key] || [], seen = {}, total = 0;
      items.forEach(function (it) {
        var k = lower(it.name.trim());
        if (!k) out.push(['One of the ' + lower(l.label) + ' has no name yet.', true]);
        else if (seen[k]) out.push(['“' + it.name.trim() + '” is in ' + lower(l.label) + ' twice.', true]);
        seen[k] = true;
        total += it.cost;
      });
      if (l.buy && f.budgetKey) {
        if (!items.length) out.push(['Add ' + lower(l.label) + ' for heroes to buy.', false]);
        else if (total < d.budget) out.push(['The ' + lower(l.label) + ' add up to ' + total + ' point' + (total === 1 ? '' : 's') + ', less than the ' + d.budget + ' a hero spends. Add more, or spend fewer.', false]);
      }
    });
    return out;
  };
  Entries.prototype.checksHTML = function (d) {
    var f = this.cur(), p = this.problems(d);
    if (!p.length) {
      return '<p class="oe-ready">✓ Ready' + (f.budgetKey ? '. A hero can spend all ' + d.budget + ' points.' : '.') + '</p>';
    }
    return '<ul class="oe-problems" aria-label="Before you save">' + p.map(function (m) { return '<li class="' + (m[1] ? 'is-block' : '') + '">' + esc(m[0]) + '</li>'; }).join('') + '</ul>';
  };

  // What a player sees in the hero creator, drawn from the draft as typed.
  Entries.prototype.viewHTML = function (d) {
    var self = this, f = this.cur();
    var facts = d.facts.filter(function (p) { return p.label.trim() && p.value.trim(); });
    var box = tidy(d.text);
    box.querySelectorAll('[data-secret]').forEach(function (s) { s.remove(); });
    var text = box.textContent.trim() ? box.innerHTML : '';
    var spent = this.tryBuy.reduce(function (n, t) {
      var it = (d.lists[t.split(':')[0]] || [])[+t.split(':')[1]];
      return n + (it ? it.cost : 0);
    }, 0);
    var lists = f.lists.map(function (l) {
      var items = (d.lists[l.key] || []).filter(function (it) { return it.name.trim(); });
      if (!items.length) return '';
      if (!l.buy || !f.budgetKey) {
        return '<div class="hc-sec"><span class="hc-k">' + esc(l.label) + '</span>' + items.map(function (it) {
          return '<div class="hc-item"><b>' + esc(it.name) + '</b>' + (it.description.trim() ? '<p>' + esc(it.description) + '</p>' : '') + '</div>';
        }).join('') + '</div>';
      }
      var left = d.budget - spent, dots = '';
      for (var i = 0; i < d.budget; i++) dots += '<i class="' + (i < spent ? 'is-used' : '') + '"></i>';
      return '<div class="hc-sec"><span class="hc-k">' + esc(l.label) + ' <span class="hc-pts" aria-live="polite">' + dots + ' ' + Math.max(0, left) + ' of ' + d.budget + ' left</span></span>' +
        (d.lists[l.key] || []).map(function (it, i) {
          if (!it.name.trim()) return '';
          var tk = l.key + ':' + i, on = self.tryBuy.indexOf(tk) >= 0;
          return '<label class="hc-opt"><input type="checkbox" data-try="' + esc(tk) + '"' + (on ? ' checked' : '') + (!on && it.cost > left ? ' disabled' : '') + '>' +
            '<b>' + esc(it.name) + '</b><span class="hc-opt__c">' + it.cost + ' pt' + (it.cost === 1 ? '' : 's') + '</span>' +
            (it.description.trim() ? '<p>' + esc(it.description) + '</p>' : '') + '</label>';
        }).join('') + '</div>';
    }).join('');
    return '<p class="oe-note">What a player sees in the hero creator' + (d.who === 'directors' ? ', once you switch it to Everyone' : '') +
      (f.budgetKey ? '. Tick to try spending the points' : '') + '.</p>' +
      '<div class="hc-kind"><span class="paper-kind">' + esc(f.label) + ' · this campaign</span><span class="hc-seal">Campaign</span></div>' +
      '<h3 class="hc-name">' + esc(d.name.trim() || 'Unnamed') + '</h3>' + (d.summary.trim() ? '<p class="hc-lede">' + esc(d.summary) + '</p>' : '') +
      (facts.length ? '<div class="hc-facts">' + facts.map(function (p) {
        return '<div><b>' + esc(p.value) + '</b><span class="hc-k">' + esc(p.label) + '</span></div>';
      }).join('') + '</div>' : '') +
      (text ? '<div class="hc-text">' + text + '</div>' : '') + lists +
      '<div class="oe-checks" data-checks>' + this.checksHTML(d) + '</div>';
  };

  Entries.prototype.pasteHTML = function () {
    var f = this.cur();
    return '<div class="oe-paste" role="group" aria-label="Paste from an AI">' +
      '<p>Paste ' + an(lower(f.label)) + ' ' + esc(lower(f.label)) + ' an AI wrote in Chronicle’s format. It fills in the form so you can check it; nothing is saved until you press Save. “Copy the guide” copies the format with ' + esc(this.system) + '’s shape and an example, for any AI chat.</p>' +
      '<label class="sr-only" for="oe-paste-box">Text to read in</label><textarea id="oe-paste-box" spellcheck="false" placeholder="---&#10;name: …&#10;---"></textarea>' +
      '<div class="oe-paste__acts"><button type="button" class="oe-btn" data-act="readin">Read it in</button>' +
      '<button type="button" class="oe-btn oe-btn--ghost" data-act="guide">Copy the guide</button>' +
      '<button type="button" class="oe-link" data-act="pasteclose">Close</button><span class="oe-paste__msg" role="status" data-paste-msg></span></div></div>';
  };

  Entries.prototype.fillPop = function () {
    var pop = this.root.querySelector('.oe-pop'), d = this.draft, isNew = !d.id, f = this.cur(), body;
    if (this.confirm === 'discard' || this.confirm === 'delete') {
      var del = this.confirm === 'delete';
      body = '<div class="oe-confirm" role="alertdialog" aria-label="' + (del ? 'Delete ' + esc(d.name) : 'Unsaved changes') + '">' +
        (del ? 'Delete <b>' + esc(d.name) + '</b>? It leaves the pick lists for everyone. Heroes who picked it keep the name on their sheet.'
          : 'Close ' + (d.name.trim() ? '<b>' + esc(d.name) + '</b>' : 'this entry') + ' without saving? What you changed is lost.') +
        '<div class="oe-confirm__row"><button type="button" class="oe-btn oe-btn--danger" data-act="' + (del ? 'dodel' : 'discard') + '">' + (del ? 'Delete' : 'Don’t save') + '</button>' +
        '<button type="button" class="oe-btn oe-btn--ghost" data-act="nodel">' + (del ? 'Keep it' : 'Keep writing') + '</button></div></div>';
    } else {
      body = (this.paste ? this.pasteHTML() : '') + '<div class="oe-cols">' + this.writeHTML(d) + '<aside class="oe-view" aria-label="What a player sees">' + this.viewHTML(d) + '</aside></div>';
    }
    pop.innerHTML = '<header class="oe-pop__head"><div><p class="oe-kind">' + esc(f.label) + ' · this campaign · Directors only</p>' +
      '<h2 data-pop-title tabindex="-1">' + esc(d.name.trim() || (isNew ? 'New ' + lower(f.label) : 'Unnamed')) + '</h2></div>' +
      '<div class="oe-pop__acts">' + (this.confirm ? '' : '<button type="button" class="oe-btn oe-btn--ghost" data-act="paste" aria-expanded="' + this.paste + '">Paste from an AI</button>') +
      '<button type="button" class="oe-x" data-act="close" aria-label="Close">×</button></div></header>' +
      '<div class="oe-pop__body">' + body + '</div>' +
      '<footer class="oe-pop__foot">' + (isNew ? '' : '<button type="button" class="oe-link oe-del" data-act="del">Delete</button>') +
      '<p class="oe-error" role="alert" hidden data-error></p><span class="hc-foot__sp"></span>' +
      '<button type="button" class="oe-btn oe-btn--ghost" data-act="close">Cancel</button>' +
      '<button type="button" class="oe-btn" data-act="save"' + (this.canSave() ? '' : ' disabled') + '>' + (isNew ? 'Add ' + esc(lower(f.label)) : 'Save') + '</button></footer>';
  };

  Entries.prototype.canSave = function () {
    return !!this.draft && !this.busy && !this.problems(this.draft).some(function (p) { return p[1]; });
  };

  Entries.prototype.openPop = function (e, from) {
    var f = this.cur();
    this.draft = draftOf(e, f);
    this.saved = JSON.parse(JSON.stringify(this.draft));
    this.open = e ? String(e.id) : 'new';
    this.confirm = false;
    this.paste = false;
    this.tryBuy = [];
    var desk = this.root.querySelector('.oe-desk'), pop = this.root.querySelector('.oe-pop');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', String(r === from)); });
    this.fillPop();
    pop.hidden = false;
    // It grows out of the row (or button) that opened it.
    if (from) {
      var r = from.getBoundingClientRect(), d = desk.getBoundingClientRect();
      pop.style.transformOrigin = Math.round(r.left - d.left + r.width / 2) + 'px ' + Math.round(r.top - d.top + r.height / 2) + 'px';
    }
    this.fit();
    requestAnimationFrame(function () { desk.classList.add('is-open'); });
    if (pop.getBoundingClientRect().top < 0) pop.scrollIntoView({ block: 'start' });
    var n = pop.querySelector('[data-k="name"]');
    if (n) n.focus({ preventScroll: true });
    this.from = from;
  };

  // The page behind grows with the editor, so the editor never clips.
  Entries.prototype.fit = function () {
    var desk = this.root.querySelector('.oe-desk'), pop = this.root.querySelector('.oe-pop');
    if (desk && pop) desk.style.minHeight = this.open === null ? '' : (pop.offsetHeight + 8) + 'px';
  };

  Entries.prototype.closePop = function (force) {
    if (this.open === null) return;
    // Unsaved writing is never thrown away on a stray click: it asks first.
    if (!force && this.isDirty()) {
      this.syncText();
      this.confirm = 'discard';
      this.paste = false;
      this.fillPop();
      var keep = this.root.querySelector('[data-act="nodel"]');
      if (keep) keep.focus();
      return;
    }
    var desk = this.root.querySelector('.oe-desk'), pop = this.root.querySelector('.oe-pop'), self = this;
    this.open = null;
    this.draft = null;
    this.confirm = false;
    dirty(false);
    desk.classList.remove('is-open');
    this.root.querySelectorAll('.hc-row').forEach(function (r) { r.setAttribute('aria-expanded', 'false'); });
    var done = function () { if (self.open === null) { pop.hidden = true; self.fit(); } };
    var ms = parseFloat(getComputedStyle(pop).transitionDuration) * 1000 || 0;
    setTimeout(done, ms);
    var back = this.from && document.body.contains(this.from) ? this.from : this.root.querySelector('[data-act="add"]');
    if (back) back.focus({ preventScroll: true });
  };

  Entries.prototype.isDirty = function () { return !!this.draft && !sameDraft(this.draft, this.saved); };

  // After a keystroke: the player's view, the title and Save follow the
  // draft; the form itself is left alone so focus and typing are undisturbed.
  Entries.prototype.touched = function () {
    if (!this.draft) return;
    dirty(this.isDirty());
    var d = this.draft, view = this.root.querySelector('.oe-view');
    if (view) view.innerHTML = this.viewHTML(d);
    var t = this.root.querySelector('[data-pop-title]');
    if (t) t.textContent = d.name.trim() || (d.id ? 'Unnamed' : 'New ' + lower(this.cur().label));
    var sv = this.root.querySelector('[data-act="save"]');
    if (sv) sv.disabled = !this.canSave();
  };

  // Redraws the form after a change that adds or removes fields, keeping the
  // scroll place.
  Entries.prototype.redraw = function () {
    var body = this.root.querySelector('.oe-pop__body'), top = body ? body.scrollTop : 0;
    this.fillPop();
    body = this.root.querySelector('.oe-pop__body');
    if (body) body.scrollTop = top;
    this.fit();
  };

  Entries.prototype.syncText = function () {
    var box = this.root.querySelector('.oe-text');
    if (!box || !this.draft) return;
    this.draft.text = (box.textContent.trim() || box.querySelector('li')) ? box.innerHTML : '';
    var hint = this.root.querySelector('[data-hint]');
    if (hint) {
      hint.innerHTML = box.querySelector('[data-secret]') ? '<b>Part of this entry is hidden from players.</b> Directors still read it.'
        : 'Select a passage and press “Hide from players” to keep it for Directors.';
    }
  };

  // Wraps the selection in a secret span, or unwraps it when the selection
  // already sits inside one, so the button works both ways.
  Entries.prototype.secret = function () {
    var box = this.root.querySelector('.oe-text'), sel = window.getSelection();
    if (!box || !sel.rangeCount) return;
    var range = sel.getRangeAt(0);
    if (!box.contains(range.commonAncestorContainer)) { notify('Select a passage in the entry first.', 'info'); return; }
    var n = range.commonAncestorContainer, inside = null;
    for (; n && n !== box; n = n.parentNode) if (n.nodeType === 1 && n.hasAttribute('data-secret')) { inside = n; break; }
    if (inside) {
      inside.replaceWith.apply(inside, Array.prototype.slice.call(inside.childNodes));
      this.syncText(); this.touched(); this.announce('Passage shown to players again');
      return;
    }
    if (range.collapsed) { notify('Select a passage in the entry first.', 'info'); return; }
    var span = document.createElement('span');
    span.setAttribute('data-secret', 'true');
    span.appendChild(range.extractContents());
    range.insertNode(span);
    sel.removeAllRanges();
    this.syncText(); this.touched(); this.announce('Passage hidden from players');
  };

  Entries.prototype.announce = function (msg) {
    var live = this.root.querySelector('.hc-live');
    if (!live) { live = document.createElement('div'); live.className = 'hc-live sr-only'; live.setAttribute('aria-live', 'polite'); this.root.appendChild(live); }
    live.textContent = msg;
  };

  Entries.prototype.fail = function (msg) {
    this.busy = false;
    var err = this.root.querySelector('.oe-pop [data-error]');
    if (err) { err.textContent = msg; err.hidden = false; }
    this.touched();
  };

  // The properties sent to Chronicle: facts under snake_case keys (numbers
  // as numbers), the budget, and each list with only its named items.
  Entries.prototype.properties = function () {
    var d = this.draft, f = this.cur(), props = {}, seen = {};
    for (var i = 0; i < d.facts.length; i++) {
      var p = d.facts[i];
      if (!p.label.trim() || !p.value.trim()) continue;
      // An untouched label keeps the key it came with, so a fact the system
      // spells differently is not renamed by a save.
      var k = (p.key && words(p.key) === p.label) ? p.key : keyOf(p.label);
      if (seen[k]) return { error: 'Two facts are both called “' + p.label.trim() + '”. Rename or remove one.' };
      seen[k] = true;
      props[k] = factValue(p.value);
    }
    if (f.budgetKey) props[f.budgetKey] = d.budget;
    Object.keys(d.lists).forEach(function (key) {
      var buy = f.lists.some(function (l) { return l.key === key && l.buy; });
      var items = d.lists[key].filter(function (it) { return it.name.trim(); }).map(function (it) {
        var o = { name: it.name.trim() };
        if (buy) o.cost = it.cost;
        if (it.description.trim()) o.description = it.description.trim();
        return o;
      });
      if (items.length) props[key] = items;
    });
    return { props: props };
  };

  Entries.prototype.save = function () {
    var self = this, d = this.draft, f = this.cur();
    this.syncText();
    if (!this.canSave()) return;
    var built = this.properties();
    if (built.error) { this.fail(built.error); return; }
    var text = tidy(d.text);
    var body = { name: d.name.trim(), summary: d.summary.trim(), description: text.textContent.trim() ? text.innerHTML : '',
      properties: built.props, visibility: d.who === 'directors' ? 'directors' : 'everyone' };
    var isNew = !d.id;
    if (isNew) body.fieldKey = f.key;
    this.busy = true;
    this.touched();
    var err = this.root.querySelector('.oe-pop [data-error]');
    if (err) err.hidden = true;
    api(isNew ? this.entriesUrl : this.entriesUrl + '/' + encodeURIComponent(d.id), { method: isNew ? 'POST' : 'PUT', body: body, csrfToken: this.csrf })
      .then(json).then(function (res) {
        if (!res.ok || !res.body.entry) { self.fail(res.body.message || res.body.error || 'The entry could not be saved. Try again.'); return; }
        var e = res.body.entry;
        self.entries = self.entries.filter(function (x) { return x.id !== e.id; }).concat([e]);
        self.busy = false;
        self.closePop(true);
        self.render();
        var row = self.root.querySelector('.hc-row[data-open="' + e.id + '"]');
        if (row) row.focus({ preventScroll: true });
        notify(e.name + (isNew ? ' added' : ' saved'));
      }).catch(function () { self.fail('The entry could not be saved. Check your connection and try again.'); });
  };

  Entries.prototype.remove = function () {
    var self = this, d = this.draft;
    if (this.busy || !d.id) return;
    this.busy = true;
    api(this.entriesUrl + '/' + encodeURIComponent(d.id), { method: 'DELETE', csrfToken: this.csrf }).then(function (r) {
      if (!r.ok) return json(r).then(function (res) { self.fail(res.body.message || res.body.error || 'The entry could not be deleted. Try again.'); });
      self.entries = self.entries.filter(function (x) { return x.id !== d.id; });
      self.busy = false;
      self.closePop(true);
      self.render();
      var add = self.root.querySelector('[data-act="add"]');
      if (add) add.focus({ preventScroll: true });
      notify(d.name + ' deleted');
    }).catch(function () { self.fail('The entry could not be deleted. Check your connection and try again.'); });
  };

  // Reads the homebrew format AI Import uses: YAML front matter between
  // "---" lines, then the entry's text. Only the small part of YAML an entry
  // needs is understood (key: value, lists of items, "|" and ">" blocks);
  // anything else is reported rather than guessed at.
  function unquote(v) {
    v = v.trim();
    if (/^".*"$/.test(v)) { try { return JSON.parse(v); } catch (e) { return v.slice(1, -1); } }
    if (/^'.*'$/.test(v)) return v.slice(1, -1).replace(/''/g, "'");
    return v;
  }
  function scalar(v) {
    var s = unquote(v);
    if (s !== v.trim()) return s;
    if (/^-?\d+(\.\d+)?$/.test(s)) return Number(s);
    if (/^(true|yes)$/i.test(s)) return true;
    if (/^(false|no)$/i.test(s)) return false;
    return s;
  }
  function readYAML(src) {
    var lines = src.split('\n'), out = {}, errs = [], i = 0;
    function block(style, ind) {
      var buf = [];
      while (i < lines.length && (!lines[i].trim() || lines[i].match(/^\s*/)[0].length > ind)) { buf.push(lines[i].trim()); i++; }
      while (buf.length && !buf[buf.length - 1]) buf.pop();
      return style === '|' ? buf.join('\n') : buf.join(' ').replace(/\s+/g, ' ').trim();
    }
    function value(raw, ind) { return /^[|>][+-]?$/.test(raw.trim()) ? block(raw.trim().charAt(0), ind) : scalar(raw); }
    while (i < lines.length) {
      var line = lines[i];
      if (!line.trim() || /^\s*#/.test(line)) { i++; continue; }
      var m = line.match(/^([A-Za-z_][\w-]*):\s*(.*)$/);
      if (!m) { errs.push('Line ' + (i + 1) + ' isn’t “name: value”.'); i++; continue; }
      i++;
      if (m[2].trim()) { out[m[1]] = value(m[2], 0); continue; }
      var list = [], cur = null;
      while (i < lines.length && (!lines[i].trim() || /^\s/.test(lines[i]))) {
        var l = lines[i], ind = l.match(/^\s*/)[0].length;
        if (!l.trim()) { i++; continue; }
        var item = l.trim().match(/^-\s+([A-Za-z_][\w-]*):\s*(.*)$/), kv = l.trim().match(/^([A-Za-z_][\w-]*):\s*(.*)$/);
        i++;
        if (item) { cur = {}; list.push(cur); cur[item[1]] = value(item[2], ind + 1); }
        else if (kv && cur) cur[kv[1]] = value(kv[2], ind);
        else errs.push('Line ' + i + ' under “' + m[1] + '” isn’t an item.');
      }
      out[m[1]] = list;
    }
    return { data: out, errs: errs };
  }

  Entries.prototype.readIn = function (text) {
    var f = this.cur(), src = text.replace(/\r/g, '').replace(/^\s*```[a-z]*\n?|\n?```\s*$/g, '').trim(), body = '';
    var fm = src.match(/^---\s*\n([\s\S]*?)\n---\s*(?:\n([\s\S]*))?$/);
    if (fm) { body = (fm[2] || '').trim(); src = fm[1]; }
    var y = readYAML(src), data = y.data;
    if (y.errs.length) return y.errs[0];
    if (data.kind && data.kind !== 'system-entry') return 'This is “' + data.kind + '”, not an entry.';
    if (data.entry && data.entry !== f.key) {
      var other = this.fields.filter(function (x) { return x.key === data.entry; })[0];
      return other ? 'This is ' + an(lower(other.label)) + ' ' + lower(other.label) + '. Open the ' + other.label + ' tab to read it in.' : 'This is for “' + data.entry + '”, which isn’t a pick list here.';
    }
    if (!data.name) return 'No name found. Check the text has a “name:” line.';
    var d = this.draft, lists = {};
    f.lists.forEach(function (l) { lists[l.key] = []; });
    d.name = String(data.name);
    d.summary = data.summary ? String(data.summary) : '';
    d.who = (data.director === true || data.visibility === 'directors') ? 'directors' : 'everyone';
    d.facts = [];
    var txt = body || (typeof data.description === 'string' ? data.description : '');
    d.text = txt ? txt.split(/\n\s*\n/).map(function (para) { return '<p>' + esc(para.trim()).replace(/\n/g, '<br>') + '</p>'; }).join('') : '';
    Object.keys(data).forEach(function (k) {
      var v = data[k];
      if (/^(kind|entry|name|summary|director|visibility|description|action|tags)$/.test(k)) return;
      if (Array.isArray(v)) {
        lists[k] = v.filter(function (it) { return it && it.name; }).map(function (it) {
          return { name: String(it.name), cost: typeof it.cost === 'number' ? it.cost : 1, description: String(it.description || it.text || '') };
        });
      } else if (k === f.budgetKey && +v > 0) d.budget = Math.min(20, Math.round(+v));
      else d.facts.push({ label: words(k), value: String(v), key: k });
    });
    d.lists = lists;
    this.tryBuy = [];
    return '';
  };

  // A guide any AI chat can follow: the format, filled with this system's
  // shape for the pick list, and one of the system's own entries as a model.
  function yamlStr(v) {
    if (typeof v === 'number' || typeof v === 'boolean') return String(v);
    var s = String(v);
    return /^[\w][\w .,'()/+-]*$/.test(s) && !/^(true|false|yes|no|null)$/i.test(s) && !/^\d/.test(s) ? s : JSON.stringify(s);
  }
  function plainRules(s) { return String(s || '').replace(/\{@\w+ ([^}|]*)(?:\|([^}]*))?\}/g, function (_, a, b) { return b || a; }); }
  Entries.prototype.guide = function () {
    var f = this.cur(), out = [], label = lower(f.label);
    out.push('Write ' + an(label) + ' ' + label + ' for ' + this.system + ' in Chronicle’s homebrew format. Reply with only one block like the template below, filled in.');
    if (f.budgetKey) {
      var buy = f.lists.filter(function (l) { return l.buy; }).map(function (l) { return l.key; });
      out.push('A hero spends ' + f.budgetKey + ' (usually ' + f.budget + ') on items from ' + buy.join(', ') + '; each item there has a whole-number cost, usually 1 or 2.');
    }
    out.push('', 'Template:', '---', 'kind: system-entry', 'entry: ' + f.key, 'name: …', 'summary: one line a player reads in the list');
    f.facts.forEach(function (k) { out.push(k + ': …'); });
    if (f.budgetKey) out.push(f.budgetKey + ': ' + f.budget);
    f.lists.forEach(function (l) { out.push(l.key + ':', '  - name: …'); if (l.buy) out.push('    cost: 1'); out.push('    description: what it does'); });
    out.push('---', 'The whole entry, as plain paragraphs.');
    var c = f.sample;
    if (c) {
      out.push('', 'One of ' + this.system + '’s own ' + plural(f.label) + ', in the same format, as a model:', '---', 'kind: system-entry', 'entry: ' + f.key, 'name: ' + yamlStr(c.name));
      if (c.summary) out.push('summary: ' + yamlStr(plainRules(c.summary)));
      var p = c.properties || {};
      f.facts.concat(f.budgetKey ? [f.budgetKey] : []).forEach(function (k) { if (p[k] != null && !Array.isArray(p[k])) out.push(k + ': ' + yamlStr(p[k])); });
      f.lists.forEach(function (l) {
        if (!namedList(p[l.key])) return;
        out.push(l.key + ':');
        p[l.key].slice(0, 2).forEach(function (it) {
          out.push('  - name: ' + yamlStr(it.name));
          if (typeof it.cost === 'number') out.push('    cost: ' + it.cost);
          if (it.description) out.push('    description: ' + yamlStr(plainRules(it.description)));
        });
      });
      out.push('---');
    }
    return out.join('\n');
  };

  Entries.prototype.wire = function () {
    var self = this, root = this.root;
    // The tools act on the writing box's selection, so pressing them must not
    // take focus away from it.
    root.addEventListener('mousedown', function (e) { if (e.target.closest('.oe-tool button')) e.preventDefault(); });
    root.addEventListener('click', function (e) {
      var t = e.target.closest('button, .oe-scrim');
      if (!t || !root.contains(t)) return;
      if (t.hasAttribute('data-field')) {
        if (self.open !== null) return;
        self.field = +t.getAttribute('data-field');
        self.render();
        var tab = root.querySelector('.oe-fields [data-field="' + self.field + '"]');
        if (tab) tab.focus();
        return;
      }
      if (t.hasAttribute('data-open')) {
        var id = t.getAttribute('data-open');
        var hit = self.entries.filter(function (x) { return String(x.id) === id; })[0];
        if (hit && self.open === null) self.openPop(hit, t);
        return;
      }
      var d = self.draft, li = t.getAttribute('data-li'), ii = +t.getAttribute('data-ii');
      if (t.hasAttribute('data-cost')) {
        d.lists[li][ii].cost = +t.getAttribute('data-cost');
        t.parentNode.querySelectorAll('button').forEach(function (b) { b.setAttribute('aria-pressed', String(b === t)); });
        self.tryBuy = [];
        self.touched();
        return;
      }
      switch (t.getAttribute('data-act')) {
        case 'add': if (self.open === null) self.openPop(null, t); break;
        case 'close': self.closePop(); break;
        case 'save': self.save(); break;
        case 'bold': case 'italic':
          document.execCommand(t.getAttribute('data-act'));
          self.syncText(); self.touched();
          break;
        case 'secret': self.secret(); break;
        case 'addfact':
          self.syncText();
          d.facts.push({ label: '', value: '', key: '' });
          self.redraw(); self.touched();
          var fs = root.querySelectorAll('.oe-facts input[data-part="label"]');
          if (fs.length) fs[fs.length - 1].focus();
          break;
        case 'rmfact':
          self.syncText();
          d.facts.splice(+t.getAttribute('data-i'), 1);
          self.redraw(); self.touched();
          break;
        case 'additem':
          self.syncText();
          (d.lists[li] = d.lists[li] || []).push({ name: '', cost: 1, description: '' });
          self.redraw(); self.touched();
          var ns = root.querySelectorAll('.oe-item__name[data-li="' + li + '"]');
          if (ns.length) ns[ns.length - 1].focus();
          break;
        case 'rmitem':
          self.syncText();
          d.lists[li].splice(ii, 1);
          self.tryBuy = [];
          self.redraw(); self.touched();
          break;
        case 'plus': case 'minus':
          d.budget = Math.max(1, Math.min(20, d.budget + (t.getAttribute('data-act') === 'plus' ? 1 : -1)));
          t.parentNode.querySelector('output').textContent = d.budget;
          self.tryBuy = [];
          self.touched();
          break;
        case 'paste':
          self.syncText();
          self.paste = !self.paste;
          self.redraw();
          if (self.paste) root.querySelector('#oe-paste-box').focus();
          break;
        case 'pasteclose': self.paste = false; self.redraw(); break;
        case 'readin':
          var msg = self.readIn(root.querySelector('#oe-paste-box').value);
          if (msg) { var m = root.querySelector('[data-paste-msg]'); m.textContent = msg; m.classList.add('is-bad'); break; }
          self.paste = false;
          self.redraw(); self.touched();
          self.announce(self.draft.name + ' read in. Check it, then save.');
          notify(self.draft.name + ' read in. Check it, then save.', 'info');
          break;
        case 'guide':
          var text = self.guide(), box = root.querySelector('#oe-paste-box'), out = root.querySelector('[data-paste-msg]');
          var ok = function () { out.textContent = 'Guide copied. Paste it into any AI chat with your idea.'; out.classList.remove('is-bad'); };
          var fallback = function () { box.value = text; box.select(); out.textContent = 'The guide is in the box; copy it from there.'; };
          if (navigator.clipboard && navigator.clipboard.writeText) navigator.clipboard.writeText(text).then(ok, fallback); else fallback();
          break;
        case 'del':
          self.syncText();
          self.confirm = 'delete'; self.paste = false; self.redraw();
          var keep = root.querySelector('[data-act="nodel"]');
          if (keep) keep.focus();
          break;
        case 'nodel': self.confirm = false; self.redraw(); break;
        case 'dodel': self.remove(); break;
        case 'discard': self.closePop(true); break;
      }
    });
    root.addEventListener('input', function (e) {
      var el = e.target, d = self.draft;
      if (!d || !el.getAttribute) return;
      var k = el.getAttribute('data-k');
      if (k === 'text') self.syncText();
      else if (k) d[k] = el.value;
      if (el.hasAttribute('data-fact')) d.facts[+el.getAttribute('data-fact')][el.getAttribute('data-part')] = el.value;
      if (el.hasAttribute('data-li') && el.hasAttribute('data-part')) d.lists[el.getAttribute('data-li')][+el.getAttribute('data-ii')][el.getAttribute('data-part')] = el.value;
      if (k || el.hasAttribute('data-fact') || el.hasAttribute('data-li')) self.touched();
    });
    root.addEventListener('change', function (e) {
      var el = e.target;
      if (el.name === 'oe-who' && self.draft) { self.draft.who = el.value; self.touched(); return; }
      if (el.hasAttribute && el.hasAttribute('data-try')) {
        var tk = el.getAttribute('data-try');
        if (el.checked) self.tryBuy.push(tk); else self.tryBuy = self.tryBuy.filter(function (x) { return x !== tk; });
        self.touched();
        var again = root.querySelector('[data-try="' + tk + '"]');
        if (again) again.focus();
      }
    });
    // Pasted or dropped text goes in plain, so formatting from elsewhere (and
    // anything hidden in it) never reaches the entry.
    root.addEventListener('paste', function (e) {
      if (!e.target.closest || !e.target.closest('.oe-text')) return;
      e.preventDefault();
      document.execCommand('insertText', false, (e.clipboardData || window.clipboardData).getData('text/plain'));
    });
    root.addEventListener('drop', function (e) {
      if (e.target.closest && e.target.closest('.oe-text')) e.preventDefault();
    });
    root.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && self.open !== null) {
        e.preventDefault();
        if (self.paste) { self.paste = false; self.redraw(); }
        else if (self.confirm) { self.confirm = false; self.redraw(); }
        else self.closePop();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && self.open !== null) { e.preventDefault(); self.save(); return; }
      if (e.key === 'Enter' && e.target.tagName === 'INPUT' && e.target.closest('.oe-pop') && e.target.type !== 'radio' && e.target.type !== 'checkbox') { e.preventDefault(); self.save(); }
    });
    if (window.ResizeObserver) {
      new ResizeObserver(function () { if (self.open !== null) self.fit(); }).observe(root);
    }
  };

  function boot(scope) {
    var nodes = (scope || document).querySelectorAll('[data-own-entries]:not([data-oe-ready])');
    for (var i = 0; i < nodes.length; i++) {
      nodes[i].setAttribute('data-oe-ready', 'true');
      var p = new Entries(nodes[i]);
      p.wire();
      p.load();
    }
  }

  function start() {
    boot(document);
    document.addEventListener('htmx:afterSettle', function (e) { boot(e.target); });
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
