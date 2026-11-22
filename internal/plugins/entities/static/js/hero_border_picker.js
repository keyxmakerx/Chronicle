/**
 * hero_border_picker.js — the samples and the colours an owner picks a card's
 * border and colour with. Two screens draw it: the Customizer's Hero creator
 * page (in its panel) and the "Card border" row of the entry form.
 *
 * window.ChronicleBorderPicker(lib, borders) returns the picker's parts.
 *   lib      the parsed cardborders.PickerJSON:
 *            { groups: [{ name, keys }], borders: { key: { name, colour, tints } } }
 *   borders  a ChronicleHeroBorders instance. The caller shares one between its
 *            cards and the picker, so a border in a colour is made once; it must
 *            have been built on a root that holds the picker, which is how a
 *            finished coloured picture reaches the samples.
 *
 * A look is { border, colour } or null (no border). Keys and colours reach the
 * page only through borders.attrs, which validates them, or through a hex that
 * has passed HEX below; names go through Chronicle.escapeHtml.
 *
 * Needs hero_creator.css, hero_borders.css and hero_border_picker.css.
 */
(function () {
  'use strict';

  var HEX = /^#[0-9a-f]{6}$/i;
  var COLOURS = [['#b3243a', 'Crimson'], ['#d9622b', 'Ember'], ['#e0a030', 'Gold'], ['#6fae4f', 'Moss'], ['#2fb5a4', 'Teal'], ['#4aa3e0', 'Sky'],
    ['#5b5fd6', 'Indigo'], ['#9a5bd6', 'Violet'], ['#d65b9a', 'Rose'], ['#d9d2c0', 'Bone'], ['#8a8f98', 'Iron']];
  var CLEARED = ['--k', '--trim-img', '--trim-corner', '--trim-ground'];

  function esc(v) { return window.Chronicle.escapeHtml(v); }
  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function low(v) { return String(v).toLowerCase(); }
  function all(sel, root) { return Array.prototype.slice.call(root.querySelectorAll(sel)); }

  function colourName(hex) {
    var h = low(hex), hit = COLOURS.filter(function (c) { return c[0] === h; })[0];
    return hit ? hit[1] : h;
  }

  // A swatch's colour as the one custom property it is written in; anything
  // that is not six hex digits is left out.
  function cvar(hex) { return HEX.test(hex) ? ' style="--c:' + low(hex) + '"' : ''; }

  function create(lib, borders) {
    lib = lib || { groups: [], borders: {} };
    // The page's own data, made safe once: a name is text, a colour is six hex
    // digits, so nothing below has to doubt them.
    var info = {};
    Object.keys(lib.borders || {}).forEach(function (k) {
      var b = lib.borders[k] || {};
      info[k] = { name: String(b.name == null ? k : b.name), colour: HEX.test(b.colour) ? low(b.colour) : '#888888', tints: !!b.tints };
    });

    function known(key) { return typeof key === 'string' && has(info, key); }
    // The look as it may be drawn: a library border, in a colour that is six
    // hex digits (else the border's own). Null when there is no such border.
    function norm(look) {
      if (!look || !known(look.border)) return null;
      var col = low(look.colour);
      return { border: look.border, colour: HEX.test(col) ? col : low(info[look.border].colour) };
    }
    function isOwn(look) {
      var l = norm(look);
      return !!l && l.colour === low(info[l.border].colour);
    }
    function words(look) {
      var l = norm(look);
      if (!l) return 'No border';
      return info[l.border].name + (isOwn(l) ? '' : ', ' + colourName(l.colour).toLowerCase());
    }
    // Where the game system names no border (sys null) and the card has none,
    // the system's sample stands for the card and None is not also pressed.
    function noneOn(raw, sys) { return !raw && sys !== null; }
    function sysOn(look, sys) { return same(look, norm(sys)); }
    function same(a, b) {
      return (!a && !b) || (!!a && !!b && a.border === b.border && low(a.colour) === low(b.colour));
    }

    // Writes a look onto an element the way the creator writes it into a card's
    // opening tag, through the same attributes the colouring script makes.
    function paint(el, look) {
      var l = norm(look), s = l ? borders.attrs(l) : '', m = s && / data-trim="([^"]+)" style="([^"]*)"/.exec(s);
      CLEARED.forEach(function (p) { el.style.removeProperty(p); });
      if (!m) { el.removeAttribute('data-trim'); return; }
      el.setAttribute('data-trim', m[1]);
      // The style comes out of an attribute, where a quote is written as an entity.
      m[2].split('&quot;').join('"').split(';').forEach(function (d) {
        var k = d.indexOf(':');
        if (k > 0) el.style.setProperty(d.slice(0, k), d.slice(k + 1));
      });
    }
    function sample(look) {
      var l = norm(look);
      return '<span class="hcw-sm"><span class="hc-gcard"' + (l ? borders.attrs(l) : '') + '></span></span>';
    }

    function tile(key, label, look, on) {
      return '<button type="button" class="hcw-t" data-b="' + esc(key) + '" aria-pressed="' + on + '" title="' + esc(label) + '">' +
        sample(look) + '<span class="hcw-tn">' + esc(label) + '</span></button>';
    }

    function coloursHTML(look) {
      var lab = '<span class="hcw-lab">Colour</span>';
      if (!look) return '<div class="hcw-cgh">' + lab + '</div><p class="hcw-note">Pick a border first; then it can take a colour.</p>';
      var b = info[look.border], own = isOwn(look), cur = look.colour, ownHex = low(b.colour);
      var listed = own || COLOURS.some(function (c) { return c[0] === cur; });
      var h = '<div class="hcw-cgh" data-cb="' + esc(look.border) + '">' + lab + '<small data-cname>' + (own ? 'Its own' : esc(colourName(cur))) + '</small></div><div class="hcw-cs">' +
        '<button type="button" class="hcw-c hcw-c--own" data-c="" aria-pressed="' + own + '"' + cvar(ownHex) + ' title="' + esc(b.name) + '’s own colour" aria-label="' + esc(b.name) + '’s own colour"></button>';
      COLOURS.forEach(function (c) {
        h += '<button type="button" class="hcw-c" data-c="' + c[0] + '" aria-pressed="' + (!own && c[0] === cur) + '"' + cvar(c[0]) + ' title="' + c[1] + '" aria-label="' + c[1] + '"></button>';
      });
      h += '<label class="hcw-c hcw-c--any"' + (listed ? '' : ' data-set') + cvar(cur) + ' title="Any colour"><input type="color" data-any value="' + cur + '" aria-label="Any colour"></label></div>';
      if (!info[look.border].tints) h += '<p class="hcw-note">' + esc(b.name) + ' keeps its own tint. Your colour shows in the card’s small label and its glow.</p>';
      return h;
    }

    // state: { look, sys }; sys undefined offers no "game system's" sample.
    // opts: { none, sysLabel, lay, split, top }.
    function html(state, opts) {
      state = state || {};
      opts = opts || {};
      var raw = state.look, look = norm(raw), sys = state.sys, hasSys = sys !== undefined;
      var colour = look && !isOwn(look) ? look.colour : null;
      var split = !!opts.split, top = !!opts.top, lay = opts.lay === 'field' ? 'field' : 'wall';
      var cg = '<div class="hcw-cg' + (split ? (top ? ' hcw-top' : ' hcw-foot') : '') + '" data-cg>' + coloursHTML(look) + '</div>';
      var wall = '<div class="hcw-wall" data-wall><div class="hcw-g"><div class="hcw-ts">' +
        tile('', opts.none || 'None', null, noneOn(raw, sys)) +
        (hasSys ? tile('@sys', opts.sysLabel || 'System’s', sys, sysOn(look, sys)) : '') + '</div></div>';
      (lib.groups || []).forEach(function (g) {
        var keys = (g.keys || []).filter(known);
        if (!keys.length) return;
        wall += '<div class="hcw-g"><h4>' + esc(g.name) + '</h4><div class="hcw-ts">' + keys.map(function (key) {
          return tile(key, info[key].name, { border: key, colour: colour || info[key].colour }, !!look && look.border === key);
        }).join('') + '</div></div>';
      });
      wall += '</div>';
      return '<div class="hcw-pk hc' + (split ? ' hcw-pk--split' : '') + '" data-pk data-lay="' + lay + '">' + (top ? cg + wall : wall + cg) + '</div>';
    }

    // Brings a drawn picker in line with state without redrawing it, so a
    // colour being dragged keeps its native picker open and nothing scrolls away.
    function sync(pk, state) {
      state = state || {};
      var raw = state.look, look = norm(raw), sys = state.sys;
      var colour = look && !isOwn(look) ? look.colour : null;
      all('[data-b]', pk).forEach(function (btn) {
        var key = btn.getAttribute('data-b'), on, lk;
        if (key === '') { on = noneOn(raw, sys); lk = null; }
        else if (key === '@sys') { on = sys !== undefined && sysOn(look, sys); lk = sys; }
        else { on = !!look && look.border === key; lk = known(key) ? { border: key, colour: colour || info[key].colour } : null; }
        btn.setAttribute('aria-pressed', String(on));
        var card = btn.querySelector('.hc-gcard');
        if (card) paint(card, lk);
      });
      var cg = pk.querySelector('[data-cg]');
      if (!cg) return;
      var head = cg.querySelector('[data-cb]'), any = cg.querySelector('[data-any]');
      // The same border's colours are updated where they stand. Only a change of
      // border (or of having one) draws them again, and the keyboard keeps its place.
      if (look && head && any && head.getAttribute('data-cb') === look.border) {
        var own = isOwn(look), editing = document.activeElement === any, lab = any.parentNode;
        var listed = own || COLOURS.some(function (c) { return c[0] === look.colour; });
        all('button[data-c]', cg).forEach(function (x) {
          var c = x.getAttribute('data-c');
          x.setAttribute('aria-pressed', String(c === '' ? own : !editing && !own && c === look.colour));
        });
        if (editing || !listed) lab.setAttribute('data-set', ''); else lab.removeAttribute('data-set');
        lab.style.setProperty('--c', look.colour);
        if (!editing) any.value = look.colour;
        var nm = cg.querySelector('[data-cname]');
        if (nm) nm.textContent = editing ? look.colour : own ? 'Its own' : colourName(look.colour);
        return;
      }
      var held = cg.contains(document.activeElement) ? document.activeElement.getAttribute('data-c') : null;
      cg.innerHTML = coloursHTML(look);
      if (held !== null) {
        var back = all('button[data-c]', cg).filter(function (x) { return x.getAttribute('data-c') === held; })[0];
        if (back) back.focus({ preventScroll: true });
      }
    }

    function ask(pk, handlers, look, merge) { handlers.change(pk, look, !!merge); }

    function onBorder(pk, handlers, key) {
      var st = handlers.state(pk);
      if (!st) return;
      var cur = norm(st.look);
      if (key === '') ask(pk, handlers, null);
      else if (key === '@sys') { if (st.sys !== undefined) ask(pk, handlers, st.sys); }
      else if (known(key)) ask(pk, handlers, { border: key, colour: cur && !isOwn(cur) ? cur.colour : low(info[key].colour) });
    }
    function onColour(pk, handlers, hex, merge) {
      var st = handlers.state(pk), cur = st && norm(st.look);
      if (!cur) return;
      if (hex === '') hex = info[cur.border].colour;
      if (!HEX.test(hex)) return;
      ask(pk, handlers, { border: cur.border, colour: low(hex) }, merge);
    }

    // One set of listeners on a container that holds any number of pickers.
    function bind(container, handlers) {
      function pkOf(el) {
        var pk = el.closest && el.closest('[data-pk]');
        return pk && container.contains(pk) ? pk : null;
      }
      function onClick(ev) {
        var pk = pkOf(ev.target), x;
        if (!pk) return;
        if ((x = ev.target.closest('[data-b]')) && pk.contains(x)) onBorder(pk, handlers, x.getAttribute('data-b'));
        else if ((x = ev.target.closest('button[data-c]')) && pk.contains(x)) onColour(pk, handlers, x.getAttribute('data-c'), false);
      }
      function onInput(ev) {
        var t = ev.target, pk = pkOf(t);
        if (pk && t.matches && t.matches('[data-any]')) onColour(pk, handlers, t.value, true);
      }
      container.addEventListener('click', onClick);
      container.addEventListener('input', onInput);
      return function () {
        container.removeEventListener('click', onClick);
        container.removeEventListener('input', onInput);
      };
    }

    return { COLOURS: COLOURS, colourName: colourName, isOwn: isOwn, words: words, same: same, paint: paint, sample: sample, html: html, sync: sync, bind: bind };
  }

  window.ChronicleBorderPicker = create;
})();
