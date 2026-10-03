/**
 * site_look.js -- the live preview on Admin > Site look (data-widget="site-look").
 *
 * The page is a plain form that works without this script, with a preview
 * window server-rendered from the saved look. This widget layers on top of it:
 * it shows the Sign-in page / Discover campaigns / Browser tab switcher, redraws
 * the preview as the admin edits the name, logo, look, background, "Move slowly"
 * and welcome line (before anything is saved), and, when a part of the preview
 * is clicked, highlights and focuses the panel that controls it.
 *
 * Look colours are never held here: each look radio carries its accent, its two
 * top bar colours and its heading font stack as data attributes, rendered from
 * the Go table (internal/sitelook), so there is no second table to drift.
 *
 * "Move slowly" reuses the campaign header's moving background
 * (Chronicle.headerMotion.drive), which runs on the shared MotionRest clock, so
 * it eases to still when the person steps away and restarts on return. Nothing
 * moves under prefers-reduced-motion, html[data-cz-reduce] or a person's own
 * Calmer setting; headerMotion.reduced() is the one place that decides that.
 *
 * All names and lines reach the preview as text nodes; none is parsed as HTML.
 */
(function () {
  'use strict';

  var DEFAULT_NAME = 'Chronicle';

  // initial is the first letter of the name, upper-cased, as the letter mark
  // shows it; a blank name is the default one.
  function initial(name) {
    var n = String(name || '').replace(/^\s+|\s+$/g, '') || DEFAULT_NAME;
    return n.charAt(0).toUpperCase();
  }

  function displayName(name) {
    return String(name || '').replace(/^\s+|\s+$/g, '') || DEFAULT_NAME;
  }

  // view works out what the preview shows from the form's state, with the same
  // fallbacks the real sign-in page has: "the look's colours" needs a look and
  // "a picture" needs a picture, otherwise the background is plain; and
  // nothing can move over a plain background.
  //   s: { look: id or '', background, hasPicture, move }
  function view(s) {
    var bg = 'plain';
    if (s.background === 'look' && s.look) bg = 'look';
    else if (s.background === 'picture' && s.hasPicture) bg = 'picture';
    return { bg: bg, look: !!s.look, moving: !!s.move && bg !== 'plain' };
  }

  // motionPlan lists what drifts: the sign-in gradient or picture, and the top
  // bar when a look is chosen. Nothing at all when reduced.
  function motionPlan(v, reduced) {
    var plan = [];
    if (!v.moving || reduced) return plan;
    if (v.bg === 'look') plan.push('flow');
    if (v.bg === 'picture') plan.push('pan');
    if (v.look) plan.push('top');
    return plan;
  }

  Chronicle.siteLook = { initial: initial, displayName: displayName, view: view, motionPlan: motionPlan };

  function start(root) {
    var win = root.querySelector('[data-sl-win]');
    if (!win) return null;
    function all(sel, scope) { return Array.prototype.slice.call((scope || root).querySelectorAll(sel)); }
    function one(sel) { return root.querySelector(sel); }
    function field(name) { return root.querySelector('[name="' + name + '"]'); }

    var nameEl = field('name'), logoFile = field('logo_file'), removeLogo = field('remove_logo');
    var favEl = field('favicon'), welcomeEl = field('welcome'), moveEl = field('move');
    var picFile = field('picture_file'), removePic = field('remove_picture');
    var savedLogo = win.getAttribute('data-sl-saved-logo') || '';
    var savedPic = win.getAttribute('data-sl-saved-picture') || '';
    var objectURLs = { logo: '', pic: '' };
    var handles = [], planKey = '';
    var gone = false;

    function checked(name) {
      var el = root.querySelector('[name="' + name + '"]:checked');
      return el || null;
    }

    // fileURL turns a freshly chosen file into a preview-only URL, releasing
    // the one it replaces.
    function fileURL(input, slot) {
      var f = input && input.files && input.files[0];
      if (objectURLs[slot]) { URL.revokeObjectURL(objectURLs[slot]); objectURLs[slot] = ''; }
      if (f && window.URL && URL.createObjectURL) objectURLs[slot] = URL.createObjectURL(f);
      return objectURLs[slot];
    }
    var logoURL = '', picURL = '';

    function currentLogo() {
      if (logoURL) return logoURL;
      if (removeLogo && removeLogo.checked) return '';
      return savedLogo;
    }
    function currentPic() {
      if (picURL) return picURL;
      if (removePic && removePic.checked) return '';
      return savedPic;
    }

    function setMark(mark, url, letter) {
      while (mark.firstChild) mark.removeChild(mark.firstChild);
      if (url) {
        var img = document.createElement('img');
        img.alt = '';
        img.src = url;
        mark.appendChild(img);
      } else {
        mark.appendChild(document.createTextNode(letter));
      }
    }

    function stopMotion() {
      for (var i = 0; i < handles.length; i++) handles[i].destroy();
      handles = [];
      planKey = '';
    }

    function syncMotion(v) {
      var HM = window.Chronicle && Chronicle.headerMotion;
      var reduced = HM && HM.reduced ? HM.reduced() : true;
      var plan = HM ? motionPlan(v, reduced) : [];
      var key = plan.join(',');
      if (key === planKey) return;
      stopMotion();
      planKey = key;
      var targets = { flow: one('[data-sl-flow]'), pan: one('[data-sl-pic]'), top: one('[data-sl-flow-top]') };
      for (var i = 0; i < plan.length; i++) {
        var h = targets[plan[i]] && HM.drive(targets[plan[i]], false, plan[i] === 'pan');
        if (h) handles.push(h);
      }
    }

    function render() {
      if (gone) return;
      var look = checked('look'), bgRadio = checked('background');
      var name = displayName(nameEl ? nameEl.value : '');
      var letter = initial(nameEl ? nameEl.value : '');
      var v = view({
        look: look ? look.value : '',
        background: bgRadio ? bgRadio.value : 'plain',
        hasPicture: !!currentPic(),
        move: !!(moveEl && moveEl.checked)
      });

      // Colours and heading font, straight from the chosen look's data attributes.
      if (look && look.value) {
        win.style.setProperty('--sa', look.getAttribute('data-accent'));
        win.style.setProperty('--hd1', look.getAttribute('data-from'));
        win.style.setProperty('--hd2', look.getAttribute('data-to'));
        var font = look.getAttribute('data-font');
        win.style.setProperty('--hf', font || 'inherit');
      } else {
        win.style.setProperty('--sa', '#6366f1');
        win.style.removeProperty('--hd1');
        win.style.removeProperty('--hd2');
        win.style.setProperty('--hf', 'inherit');
      }
      win.setAttribute('data-sl-look', v.look ? 'on' : 'off');
      win.setAttribute('data-sl-moving', v.moving ? 'on' : 'off');

      all('[data-sl-name]').forEach(function (e) { e.textContent = name; });
      var logo = currentLogo();
      all('[data-sl-mark]').forEach(function (m) {
        if (m.parentNode && m.parentNode.hasAttribute('data-sl-fav')) return;
        setMark(m, logo, letter);
      });
      // The tab shows the logo only when the box is ticked and a logo exists;
      // otherwise Chronicle's own icon stays.
      all('[data-sl-fav]').forEach(function (f) {
        var useLogo = !!(favEl && favEl.checked && logo);
        while (f.firstChild) f.removeChild(f.firstChild);
        f.classList.toggle('sl-fav-ship', !useLogo);
        if (useLogo) {
          var img = document.createElement('img');
          img.alt = '';
          img.src = logo;
          img.style.width = '100%';
          img.style.height = '100%';
          img.style.objectFit = 'cover';
          f.appendChild(img);
        } else {
          f.appendChild(document.createTextNode('C'));
        }
      });
      all('[data-sl-tabtitle]').forEach(function (e) { e.textContent = 'Discover campaigns | ' + name; });
      var w = welcomeEl ? welcomeEl.value.replace(/^\s+|\s+$/g, '') : '';
      all('[data-sl-welcome]').forEach(function (e) { e.textContent = w; e.hidden = !w; });

      var signin = one('[data-sl-view="signin"]');
      if (signin) signin.className = 'sl-signin sl-bg-' + v.bg;
      var pic = one('[data-sl-pic]');
      if (pic) {
        var url = v.bg === 'picture' ? currentPic() : '';
        if (url) pic.src = url; else pic.removeAttribute('src');
        pic.hidden = !url;
      }

      // "Move slowly" has nothing to move over a plain background.
      if (moveEl) {
        moveEl.disabled = v.bg === 'plain';
        if (moveEl.parentNode) moveEl.parentNode.style.opacity = moveEl.disabled ? '.55' : '';
      }
      syncMotion(v);
    }

    function onChange(e) {
      var t = e.target;
      if (t === logoFile) logoURL = fileURL(logoFile, 'logo');
      if (t === picFile) picURL = fileURL(picFile, 'pic');
      render();
    }

    // Views.
    var switcher = one('[data-sl-switcher]');
    if (switcher) switcher.hidden = false;
    function show(name) {
      all('[data-sl-switch]').forEach(function (b) { b.setAttribute('aria-pressed', b.getAttribute('data-sl-switch') === name ? 'true' : 'false'); });
      all('[data-sl-view]').forEach(function (s) { s.hidden = s.getAttribute('data-sl-view') !== name; });
    }

    // Focus the panel behind a clicked part of the preview.
    function focusPanel(id) {
      var panel = document.getElementById(id);
      if (!panel) return;
      all('.sl-panel').forEach(function (p) { p.classList.toggle('sl-focus', p === panel); });
      var calm = window.Chronicle && Chronicle.headerMotion ? Chronicle.headerMotion.reduced() :
        !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
      if (panel.scrollIntoView) panel.scrollIntoView({ behavior: calm ? 'auto' : 'smooth', block: 'nearest' });
      var f = panel.querySelector('input:not([disabled]),button');
      if (f && f.focus) f.focus({ preventScroll: true });
    }
    function focusTarget(node) {
      while (node && node !== root) {
        if (node.getAttribute && node.getAttribute('data-sl-focus')) return node;
        node = node.parentNode;
      }
      return null;
    }
    all('[data-sl-focus]').forEach(function (e) {
      e.setAttribute('tabindex', '0');
      e.setAttribute('role', 'button');
    });

    function onClick(e) {
      var sw = e.target.getAttribute && e.target.getAttribute('data-sl-switch');
      if (sw) { show(sw); return; }
      var t = focusTarget(e.target);
      if (t) focusPanel(t.getAttribute('data-sl-focus'));
    }
    function onKey(e) {
      if (e.key !== 'Enter' && e.key !== ' ') return;
      var t = e.target.getAttribute && e.target.getAttribute('data-sl-focus') ? e.target : null;
      if (!t) return;
      e.preventDefault();
      focusPanel(t.getAttribute('data-sl-focus'));
    }

    root.addEventListener('input', onChange);
    root.addEventListener('change', onChange);
    root.addEventListener('click', onClick);
    root.addEventListener('keydown', onKey);
    render();

    return {
      destroy: function () {
        gone = true;
        stopMotion();
        root.removeEventListener('input', onChange);
        root.removeEventListener('change', onChange);
        root.removeEventListener('click', onClick);
        root.removeEventListener('keydown', onKey);
        if (objectURLs.logo) URL.revokeObjectURL(objectURLs.logo);
        if (objectURLs.pic) URL.revokeObjectURL(objectURLs.pic);
      }
    };
  }

  Chronicle.register('site-look', {
    init: function (el) { el._siteLook = start(el); },
    destroy: function (el) {
      if (el._siteLook) el._siteLook.destroy();
      el._siteLook = null;
    }
  });
})();
