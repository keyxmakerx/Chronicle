/**
 * editor_outline.js -- "On this page" outline for long page entries.
 *
 * Exposes Chronicle.EditorOutline.attach(rootEl, contentEl, editorDom), which
 * editor.js calls when its mount has data-outline="true". The outline is
 * built from the headings in the rendered editor DOM, never from the stored
 * document: the server strips GM-only text before a player's editor loads,
 * so a heading that was entirely secret arrives empty and is skipped here.
 *
 * It shows only when the page has MIN_HEADINGS or more non-empty headings;
 * shorter pages look exactly as before. Wide editors get a sticky column on
 * the right, narrow ones a folded "On this page" button above the text
 * (CSS container query on .chronicle-editor--outlined).
 */
(function () {
  'use strict';

  var MIN_HEADINGS = 3;
  // How far below the scroll area's top a heading counts as "being read".
  var READ_LINE_PX = 96;

  // collect turns heading-like items ({level, text}) into outline entries,
  // dropping empty ones. Pure so the rule is testable without a DOM.
  function collect(items) {
    var out = [];
    for (var i = 0; i < items.length; i++) {
      var text = (items[i].text || '').replace(/\s+/g, ' ').trim();
      if (!text) continue;
      out.push({ index: i, level: items[i].level, text: text });
    }
    return out;
  }

  function shouldShow(entries) {
    return entries.length >= MIN_HEADINGS;
  }

  function prefersReducedMotion() {
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  // The page scrolls inside #main-content in the app shell; fall back to the
  // window elsewhere (e.g. a public page).
  function scrollRoot(el) {
    var main = document.getElementById('main-content');
    return main && main.contains(el) ? main : window;
  }

  function attach(rootEl, contentEl, editorDom) {
    if (!rootEl || !contentEl || !editorDom) return null;

    var nav = document.createElement('nav');
    nav.className = 'chronicle-outline';
    nav.setAttribute('aria-label', 'On this page');
    nav.hidden = true;

    var inner = document.createElement('div');
    inner.className = 'chronicle-outline__inner';
    nav.appendChild(inner);

    var toggle = document.createElement('button');
    toggle.type = 'button';
    toggle.className = 'chronicle-outline__toggle';
    toggle.setAttribute('aria-expanded', 'false');
    toggle.innerHTML = '<i class="fa-solid fa-list-ul" aria-hidden="true"></i> On this page';
    inner.appendChild(toggle);

    var label = document.createElement('p');
    label.className = 'chronicle-outline__label';
    label.textContent = 'On this page';
    inner.appendChild(label);

    var list = document.createElement('ol');
    list.className = 'chronicle-outline__list';
    inner.appendChild(list);

    contentEl.appendChild(nav);

    var headings = [];
    var links = [];
    var current = -1;
    // A clicked heading stays highlighted while the jump scrolls, even when
    // the page ends before that heading can reach the top.
    var lockTimer = null;
    var root = scrollRoot(rootEl);

    toggle.addEventListener('click', function () {
      var open = nav.classList.toggle('is-open');
      toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    });

    list.addEventListener('click', function (e) {
      var btn = e.target.closest('button[data-outline-index]');
      if (!btn) return;
      var h = headings[Number(btn.getAttribute('data-outline-index'))];
      if (!h) return;
      setCurrent(Number(btn.getAttribute('data-outline-index')));
      if (lockTimer) clearTimeout(lockTimer);
      lockTimer = setTimeout(function () { lockTimer = null; }, 1000);
      h.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' });
      nav.classList.remove('is-open');
      toggle.setAttribute('aria-expanded', 'false');
    });

    function highlight() {
      if (nav.hidden || !headings.length || lockTimer) return;
      var top = root === window ? 0 : root.getBoundingClientRect().top;
      var idx = 0;
      for (var i = 0; i < headings.length; i++) {
        if (headings[i].getBoundingClientRect().top - top <= READ_LINE_PX) idx = i;
      }
      setCurrent(idx);
    }

    function setCurrent(idx) {
      if (idx === current) return;
      current = idx;
      for (var j = 0; j < links.length; j++) {
        var on = j === idx;
        links[j].classList.toggle('is-current', on);
        if (on) links[j].setAttribute('aria-current', 'location');
        else links[j].removeAttribute('aria-current');
      }
    }

    function rebuild() {
      var nodes = editorDom.querySelectorAll('h1, h2, h3');
      var items = [];
      for (var i = 0; i < nodes.length; i++) {
        items.push({ level: Number(nodes[i].tagName.charAt(1)), text: nodes[i].textContent });
      }
      var entries = collect(items);
      var show = shouldShow(entries);
      nav.hidden = !show;
      rootEl.classList.toggle('chronicle-editor--outlined', show);
      headings = [];
      links = [];
      current = -1;
      list.innerHTML = '';
      if (!show) return;

      // Indent relative to the page's top heading level, so a page that
      // starts at h2 isn't shown indented.
      var minLevel = 3;
      entries.forEach(function (en) { if (en.level < minLevel) minLevel = en.level; });

      entries.forEach(function (en, k) {
        headings.push(nodes[en.index]);
        var li = document.createElement('li');
        li.className = 'chronicle-outline__item chronicle-outline__item--d' + Math.min(en.level - minLevel, 2);
        var btn = document.createElement('button');
        btn.type = 'button';
        btn.setAttribute('data-outline-index', String(k));
        btn.textContent = en.text;
        li.appendChild(btn);
        list.appendChild(li);
        links.push(btn);
      });
      highlight();
    }

    var pending = null;
    function schedule() {
      if (pending) clearTimeout(pending);
      pending = setTimeout(function () { pending = null; rebuild(); }, 250);
    }

    var observer = new MutationObserver(schedule);
    observer.observe(editorDom, { childList: true, subtree: true, characterData: true });

    var ticking = false;
    function onScroll() {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(function () { ticking = false; highlight(); });
    }
    root.addEventListener('scroll', onScroll, { passive: true });

    rebuild();

    return {
      refresh: rebuild,
      destroy: function () {
        observer.disconnect();
        root.removeEventListener('scroll', onScroll);
        if (pending) clearTimeout(pending);
        if (lockTimer) clearTimeout(lockTimer);
        rootEl.classList.remove('chronicle-editor--outlined');
        if (nav.parentNode) nav.parentNode.removeChild(nav);
      },
    };
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.EditorOutline = {
    attach: attach,
    collect: collect,
    shouldShow: shouldShow,
    MIN_HEADINGS: MIN_HEADINGS,
  };
})();
