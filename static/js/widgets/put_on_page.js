/**
 * put_on_page.js: the "Put on a page" chooser. A button carrying
 * data-put-block="<block type>" inside a data-widget="put-on-page" element
 * (it also carries data-campaign-id) opens a small list of the campaign's
 * categories. Choosing one adds the block, in a section of its own, to the end
 * of that category's page layouts: the same layout the Customizer's Add
 * widgets edits, saved through POST .../entity-types/:id/layout/blocks.
 *
 * The category list and the suggested one come from the server
 * (GET .../entity-types/placement), so this file holds no category rules.
 * The Game & features page and the DM Screen's Trackers section both use it.
 * The list grows out of its button with the menus' .mn-* motion, which Calm
 * and Off already retime. Owner and co-DM only: the routes check the role.
 */
(function () {
  'use strict';

  // How the block reads in the heading and the toasts.
  var NOUN = {
    quest_board: 'the quest board',
    notice_boards: 'the notice boards',
    trackers: 'the Trackers',
    page_tests: 'the Tests'
  };

  // Categories shown before "Another category…", the suggested one included.
  var SHORT = 3;

  // ---------- Pure helpers (tested in test/js/put_on_page.test.mjs) ----------

  function noun(block) { return NOUN[block] || 'this'; }

  function heading(block) { return 'Put ' + noun(block) + ' on'; }

  function rowLabel(cat) { return cat.namePlural + ' pages'; }

  // view splits the server's list into what shows at first and what waits
  // behind "Another category…". The server already puts the suggested one first.
  function view(cats) {
    cats = cats || [];
    var shown = cats.slice(0, SHORT);
    return { shown: shown, rest: cats.slice(SHORT) };
  }

  function doneMessage(block, cat, status) {
    if (status === 'added') return 'Put ' + noun(block) + ' at the end of ' + rowLabel(cat) + '. Move it later in the Customizer.';
    return noun(block).charAt(0).toUpperCase() + noun(block).slice(1) + ' is already on ' + rowLabel(cat) + '.';
  }

  var P = { noun: noun, heading: heading, rowLabel: rowLabel, view: view, doneMessage: doneMessage, SHORT: SHORT };
  if (typeof window !== 'undefined') window.PutOnPage = P;
  if (typeof Chronicle === 'undefined' || !Chronicle.register) return;

  // ---------- Chooser ----------

  var open = null;

  function el(tag, cls, text, attrs) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text) n.textContent = text;
    if (attrs) Object.keys(attrs).forEach(function (k) { n.setAttribute(k, attrs[k]); });
    return n;
  }

  function plain() { return document.documentElement.getAttribute('data-motion') === 'off'; }

  function closeChooser(returnFocus) {
    if (!open) return;
    var o = open;
    open = null;
    document.removeEventListener('pointerdown', o.onOutside, true);
    document.removeEventListener('keydown', o.onKey, true);
    window.removeEventListener('resize', o.onResize);
    o.opener.setAttribute('aria-expanded', 'false');
    var pop = o.pop;
    function gone() { if (pop.parentNode) pop.parentNode.removeChild(pop); }
    if (plain()) gone();
    else {
      pop.classList.add('mn-leave', 'mn-to');
      void pop.offsetWidth;
      pop.classList.remove('mn-to');
      pop.classList.add('mn-from');
      setTimeout(gone, (Chronicle.motionMs ? Chronicle.motionMs('--dur-everyday', 120) : 120));
    }
    if (returnFocus && o.opener.isConnected) o.opener.focus({ preventScroll: true });
  }

  // place puts the list under its button, kept inside the window.
  function place(pop, opener) {
    var b = opener.getBoundingClientRect();
    var r = pop.getBoundingClientRect();
    var left = Math.max(16, Math.min(b.left, window.innerWidth - r.width - 16));
    var top = b.bottom + 6;
    if (top + r.height > window.innerHeight - 8 && b.top - r.height - 6 > 8) top = b.top - r.height - 6;
    pop.style.left = left + 'px';
    pop.style.top = top + 'px';
    pop.style.transformOrigin = (b.left - left + b.width / 2) + 'px ' + (top > b.top ? 'top' : 'bottom');
  }

  function choose(S, cat, row) {
    if (S.busy) return;
    S.busy = true;
    row.setAttribute('aria-busy', 'true');
    Chronicle.apiFetch(S.base + '/entity-types/' + cat.id + '/layout/blocks', { method: 'POST', body: { type: S.block } })
      .then(function (r) { return r.json().then(function (d) { return { ok: r.ok, d: d || {} }; }, function () { return { ok: r.ok, d: {} }; }); })
      .then(function (x) {
        S.busy = false;
        if (!x.ok) {
          row.removeAttribute('aria-busy');
          Chronicle.notify(x.d.message || 'That could not be put on the page. Try again.', 'error');
          return;
        }
        closeChooser(true);
        Chronicle.notify(doneMessage(S.block, cat, x.d.status), 'success');
      }, function () {
        S.busy = false;
        row.removeAttribute('aria-busy');
        Chronicle.notify('That could not be put on the page. Check your connection and try again.', 'error');
      });
  }

  function draw(S, cats) {
    var pop = S.pop;
    pop.textContent = '';
    pop.appendChild(el('h3', 'pop-title', heading(S.block), { id: 'pop-title' }));
    pop.setAttribute('aria-labelledby', 'pop-title');
    var v = view(cats);
    var list = el('ul', 'pop-list');
    function add(cat, hidden) {
      var li = el('li');
      if (hidden) li.hidden = true;
      var b = el('button', 'pop-row', '', { type: 'button' });
      b.appendChild(el('span', 'pop-name', rowLabel(cat)));
      if (cat.suggested) b.appendChild(el('span', 'pop-tag', 'Suggested'));
      else if (cat.placed) b.appendChild(el('span', 'pop-tag is-quiet', 'Already there'));
      b.addEventListener('click', function () { choose(S, cat, b); });
      li.appendChild(b);
      list.appendChild(li);
      return li;
    }
    v.shown.forEach(function (c) { add(c, false); });
    var waiting = v.rest.map(function (c) { return add(c, true); });
    pop.appendChild(list);
    if (waiting.length) {
      var more = el('button', 'pop-more quiet-word', 'Another category…', { type: 'button' });
      more.addEventListener('click', function () {
        waiting.forEach(function (li) { li.hidden = false; });
        more.remove();
        place(pop, S.opener);
        var first = waiting[0].querySelector('button');
        if (first) first.focus();
      });
      pop.appendChild(more);
    }
    if (!cats.length) pop.appendChild(el('p', 'pop-hint', 'This campaign has no categories to put it on yet.'));
    pop.appendChild(el('p', 'pop-hint', 'It goes at the end of each page. Move it later in the Customizer.'));
  }

  function openChooser(opener, base, block) {
    if (open && open.opener === opener) { closeChooser(true); return; }
    closeChooser(false);
    var pop = el('div', 'put-pop', '', { role: 'dialog', 'aria-label': heading(block) });
    var S = { base: base, block: block, opener: opener, pop: pop, busy: false };
    pop.appendChild(el('p', 'pop-hint', 'Loading…', { role: 'status' }));
    document.body.appendChild(pop);
    opener.setAttribute('aria-expanded', 'true');
    place(pop, opener);
    if (!plain()) {
      pop.classList.add('mn-enter', 'mn-from');
      void pop.offsetWidth;
      pop.classList.remove('mn-from');
      pop.classList.add('mn-to');
      setTimeout(function () { pop.classList.remove('mn-enter', 'mn-to'); }, (Chronicle.motionMs ? Chronicle.motionMs('--dur-everyday', 120) : 120) + 40);
    }
    function onOutside(e) { if (!pop.contains(e.target) && !opener.contains(e.target)) closeChooser(false); }
    function onKey(e) { if (e.key === 'Escape') { e.stopPropagation(); closeChooser(true); } }
    function onResize() { place(pop, opener); }
    open = { opener: opener, pop: pop, onOutside: onOutside, onKey: onKey, onResize: onResize };
    document.addEventListener('pointerdown', onOutside, true);
    document.addEventListener('keydown', onKey, true);
    window.addEventListener('resize', onResize);

    Chronicle.apiFetch(base + '/entity-types/placement?block=' + encodeURIComponent(block))
      .then(function (r) { if (!r.ok) throw new Error('status ' + r.status); return r.json(); })
      .then(function (d) {
        if (open && open.pop === pop) {
          draw(S, (d && d.categories) || []);
          place(pop, opener);
          var f = pop.querySelector('.pop-row');
          if (f) f.focus({ preventScroll: true });
        }
      })
      .catch(function () {
        pop.textContent = '';
        pop.appendChild(el('p', 'pop-hint', 'The categories could not load. Close this and try again.', { role: 'alert' }));
      });
  }

  Chronicle.register('put-on-page', {
    init: function (host, config) {
      var base = '/campaigns/' + config.campaignId;
      function onClick(e) {
        var b = e.target.closest && e.target.closest('[data-put-block]');
        if (!b || !host.contains(b)) return;
        e.preventDefault();
        openChooser(b, base, b.getAttribute('data-put-block'));
      }
      host.addEventListener('click', onClick);
      host._putOnPage = onClick;
    },
    destroy: function (host) {
      if (host._putOnPage) host.removeEventListener('click', host._putOnPage);
      closeChooser(false);
    }
  });
})();
