/**
 * Deck cards
 *
 * Owns every move a card makes (internal/plugins/decks/card*.templ draws the
 * card; static/css/decks_card.css holds its look). Two kinds of move:
 *
 *   turnOver(cardEl, loadFace)  the deck's own turn-over, one of ten
 *   move(cardEl, name, opts)    the fixed moves, the same for every deck:
 *                               draw, deal, shuffle, play, used, gone, ready
 *
 * A face is never requested or preloaded before a turn is asked for. The turn
 * starts loadFace, plays its first half, waits at the halfway point for the
 * face for at most 400ms, and if the face is still not there shows plain
 * material and lets the card's words and picture ease in when it arrives.
 *
 * Motion follows the page: html[data-motion] is "calm" (nothing travels, a
 * change fades in 200ms) or "off" (the end state at once); a device asking for
 * reduced motion is Off. Full-motion durations scale with the owner's Motion
 * speed through --dur-standard. At most one card turns at a time; later turns
 * queue. A weak device, or more than twelve cards on screen, plays burn as
 * sweep and ink as a fade; the small shape always does.
 *
 * Every move ends on the plain resting card: effect layers are removed, no
 * animation is left filling, and a new move on the same card settles the
 * old one first. Only transform, opacity, filter, clip-path and mask move.
 * Text reaches the DOM only as the server's markup or through textContent.
 */
(function () {
  'use strict';

  var C = (window.Chronicle = window.Chronicle || {});

  var TURNS = ['flip', 'slow', 'toss', 'slam', 'sweep', 'burn', 'ink', 'seal', 'bolt', 'forge'];
  var MOVES = ['draw', 'deal', 'shuffle', 'play', 'used', 'gone', 'ready'];
  var SHAPES = ['tall', 'standard', 'small'];
  var HALFWAY_WAIT = 400; // ms the turn waits at the halfway point for a face
  var CROWD = 12; // more cards than this in view and the dear moves play as their twins
  var DEAL_MAX = 7; // cards a deal moves; the rest are simply there
  var DEAL_GAP = 60; // ms between dealt cards
  var CALM_MS = 200;
  var FAIL_WORDS = 'That card could not be turned over. Try again.';

  var EASE_OUT = 'cubic-bezier(.16,1,.3,1)';
  var EASE_IN = 'cubic-bezier(.4,0,1,1)';
  var EASE_INOUT = 'cubic-bezier(.4,0,.2,1)';
  var EASE_SLIDE = 'cubic-bezier(.22,.8,.24,1)';
  var EASE_FLIP_IN = 'cubic-bezier(.45,0,.25,1)';

  // ---- Pure helpers (unit-tested in test/js/deck_cards.test.mjs) ----

  /** levelOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function levelOf(attr, deviceReduced) {
    if (deviceReduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /** deviceIsLimited: the repo's weak-device test (calendar_era_blend.js). */
  function deviceIsLimited(nav) {
    nav = nav || (typeof navigator !== 'undefined' ? navigator : {});
    return !!((nav.hardwareConcurrency && nav.hardwareConcurrency <= 4) || (nav.deviceMemory && nav.deviceMemory <= 4));
  }

  /**
   * planTurn: what a deck's turn-over plays right now. Off shows the end at
   * once, Calm fades. Full plays the deck's own, except that burn becomes
   * sweep and ink becomes a fade on a weak device, on a crowded screen and on
   * the small shape, which are cheaper twins that read the same.
   */
  function planTurn(turn, ctx) {
    var name = TURNS.indexOf(turn) >= 0 ? turn : 'flip';
    if (ctx.level === 'off') return { kind: 'off', name: name };
    if (ctx.level === 'calm') return { kind: 'calm', name: name };
    var crowd = !!ctx.crowded && !ctx.staged; // a preview stage always shows the real turn-over
    var cheap = !!(ctx.weak || crowd || ctx.shape === 'small');
    if (cheap && name === 'burn') name = 'sweep';
    if (cheap && name === 'ink') name = 'fade';
    return { kind: 'full', name: name };
  }

  /** shapeOf and turnOf: a stored value outside the lists reads as the default, as on the server. */
  function shapeOf(v) { return SHAPES.indexOf(v) >= 0 ? v : 'tall'; }
  function turnOf(v) { return TURNS.indexOf(v) >= 0 ? v : 'flip'; }

  /**
   * crowdedFor: whether the screen holds more than CROWD cards in view. Only
   * cards that intersect the viewport count, and a card on a preview stage
   * (data-dk-stage) ignores the rule so a Director sees the real turn-over.
   */
  function crowdedFor(card, cards, vw, vh) {
    if (card.closest && card.closest('[data-dk-stage]')) return false;
    var n = 0;
    for (var i = 0; i < cards.length; i++) {
      var r = cards[i].getBoundingClientRect();
      if (r.width > 0 && r.height > 0 && r.right > 0 && r.bottom > 0 && r.left < vw && r.top < vh) n++;
    }
    return n > CROWD;
  }

  /** finishFor: foil shows as sheen on the small shape, where it would be noise. */
  function finishFor(finish, shape) {
    return finish === 'foil' && shape === 'small' ? 'sheen' : finish;
  }

  /** dealPlan: which cards a deal moves, and when each starts. */
  function dealPlan(n) {
    var out = [];
    for (var i = 0; i < n; i++) out.push({ index: i, moves: i < DEAL_MAX, delay: i < DEAL_MAX ? i * DEAL_GAP : 0 });
    return out;
  }

  /** createQueue: runs jobs one at a time, in the order they were asked. */
  function createQueue() {
    var tail = Promise.resolve();
    var q = {
      pending: 0,
      push: function (job) {
        q.pending++;
        var run = tail.then(function () { return job(); });
        tail = run.then(function () { q.pending--; }, function () { q.pending--; });
        return run;
      }
    };
    return q;
  }

  /**
   * sequence: one turn-over, with its hooks supplied by the caller so it can
   * be driven without a browser. load() starts the face fetch the moment the
   * turn begins; firstHalf() plays the first half; at the halfway point the
   * turn waits for the face for at most `wait` ms. With the face it plays the
   * second half on the face; without it, on plain material, and fill() puts
   * the face in whenever it arrives. A failed load ends on the back, with
   * nothing half done.
   */
  function sequence(h) {
    var state = { face: null, failed: false, done: false };
    var asked;
    try { asked = Promise.resolve(h.load()); } catch (e) { asked = Promise.reject(e); }
    var loading = asked.then(
      function (f) { state.face = f; state.done = true; },
      function () { state.failed = true; state.done = true; }
    );
    var wait = h.wait == null ? HALFWAY_WAIT : h.wait;
    return Promise.resolve(h.firstHalf()).then(function () {
      if (state.done) return;
      return Promise.race([loading, h.sleep(wait)]);
    }).then(function () {
      if (state.failed) return h.fail();
      if (state.done) return h.secondHalf(state.face, false);
      var second = h.secondHalf(null, true);
      return Promise.all([second, loading.then(function () {
        return state.failed ? h.fail() : (h.fill && h.fill(state.face));
      })]);
    });
  }

  // ---- The browser side ----

  function sleep(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }

  function pageLevel() {
    var reduced = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    return levelOf(document.documentElement.getAttribute('data-motion'), reduced);
  }

  // Motion speed retimes --dur-standard; every card duration scales with it.
  function speed() {
    var ms = C.motionMs ? C.motionMs('--dur-standard', 200) : 200;
    return ms > 0 ? ms / 200 : 1;
  }

  function crowded(card) {
    return crowdedFor(card, document.querySelectorAll('[data-dk-card]'), window.innerWidth, window.innerHeight);
  }

  var turnQueue = createQueue();

  function state(el) { return el.__dk || (el.__dk = { anims: [], fx: [] }); }

  /** settle: finish whatever the card was doing and clear its effect layers. */
  function settle(el) {
    var st = state(el);
    st.anims.splice(0).forEach(function (a) { try { a.finish(); } catch (e) { /* already ended */ } try { a.cancel(); } catch (e2) { /* gone */ } });
    st.fx.splice(0).forEach(function (n) { if (n.parentNode) n.parentNode.removeChild(n); });
    el.classList.remove('dk-turning');
    var t = el.querySelector('.dk-turner');
    if (t) t.classList.remove('dk-3d', 'dk-flat', 'dk-burning');
  }

  /** run: one animation on one element, resolved when it ends. */
  function run(el, target, frames, opts) {
    var st = state(el);
    if (st.dead || !target.animate) return Promise.resolve();
    var a = target.animate(frames, opts);
    st.anims.push(a);
    if (st.skip) { try { a.finish(); } catch (e) { /* already ended */ } }
    return a.finished.then(function () {}, function () {});
  }

  function layer(el, parent, cls, html) {
    var n = document.createElement('span');
    if (state(el).dead) return n; // a turn that failed draws nothing more
    n.className = 'dk-fx ' + cls;
    n.setAttribute('aria-hidden', 'true');
    if (html) n.innerHTML = html; // fixed markup written in this file
    parent.appendChild(n);
    state(el).fx.push(n);
    return n;
  }

  function faceNodes(markup) {
    var t = document.createElement('template');
    t.innerHTML = markup;
    return t.content;
  }

  /** cardName: what the card is called, for the one announcement when it turns. */
  function cardName(root) {
    var c = root.querySelector('.dk-card');
    var label = c && c.getAttribute('aria-label');
    if (label) return label;
    var n = root.querySelector('.dk-name');
    return n ? n.textContent.trim() : '';
  }

  /**
   * pictureSrcOk: a deferred picture is only ever a same-origin media path,
   * /media/<uuid>, optionally /thumb/<digits>, optionally a query (signed
   * links carry expires and sig). Anything else is never requested.
   */
  var PICTURE_RE = /^\/media\/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(\/thumb\/[0-9]+)?(\?[A-Za-z0-9=&%_.~+-]*)?$/;
  function pictureSrcOk(src) { return typeof src === 'string' && PICTURE_RE.test(src); }

  /**
   * pictureSrcOf: the address to request, rebuilt from the validated parts
   * (the id, the thumb size and a re-encoded query), never the raw attribute.
   * '' when the attribute is not an allowed media path.
   */
  function pictureSrcOf(src) {
    if (!pictureSrcOk(src)) return '';
    var m = /^\/media\/([0-9a-fA-F-]{36})(?:\/thumb\/([0-9]+))?(?:\?(.*))?$/.exec(src);
    var out = '/media/' + encodeURIComponent(m[1]);
    if (m[2]) out += '/thumb/' + encodeURIComponent(m[2]);
    if (m[3]) {
      var q = new URLSearchParams(m[3]).toString();
      if (q) out += '?' + q;
    }
    return out;
  }

  /** faceSrcOk: a face is fetched only from a path on this site: one leading slash, not //host or /\host. */
  function faceSrcOk(src) {
    return typeof src === 'string' && src.charAt(0) === '/' && src.charAt(1) !== '/' && src.charAt(1) !== '\\' && !/[\s\u0000-\u001f]/.test(src);
  }

  function pictureReady(root) {
    var imgs = Array.prototype.slice.call(root.querySelectorAll('img[data-src]'));
    if (!imgs.length) return Promise.resolve();
    return Promise.all(imgs.map(function (img) {
      var src = pictureSrcOf(img.getAttribute('data-src'));
      if (!src) { img.removeAttribute('data-src'); return Promise.resolve(); } // plain field, no request
      return new Promise(function (res) {
        img.onload = img.onerror = function () { img.removeAttribute('data-src'); res(); };
        img.src = src;
      });
    }));
  }

  /** loaderFor: where a card's face comes from, when the caller gave none. */
  function loaderFor(card) {
    var btn = card.querySelector('.dk-turn');
    var src = btn && btn.getAttribute('data-face-src');
    if (src && faceSrcOk(src)) {
      return function () {
        return fetch(src, { credentials: 'same-origin' }).then(function (r) {
          if (!r.ok) throw new Error('face ' + r.status);
          return r.text();
        });
      };
    }
    var host = card.closest('[data-dk-one]');
    var tpl = host && host.querySelector('template.dk-face-tpl');
    if (tpl) {
      return function () {
        var ms = parseInt(tpl.getAttribute('data-delay'), 10) || 0;
        return sleep(ms).then(function () { return tpl.innerHTML; });
      };
    }
    return function () { return Promise.reject(new Error('no face')); };
  }

  /** land: the face becomes the card; the back, the slot and the button go. */
  function land(card, faceRoot, focusKept) {
    var turner = card.querySelector('.dk-turner');
    var btn = card.querySelector('.dk-turn');
    var hadFocus = btn && document.activeElement === btn;
    var frag = document.createDocumentFragment();
    while (faceRoot.firstChild) frag.appendChild(faceRoot.firstChild);
    var name = cardName(frag);
    settle(card);
    if (turner) turner.parentNode.replaceChild(frag, turner);
    if (btn && btn.parentNode) btn.parentNode.removeChild(btn);
    var sr = card.querySelector('.dk-sr:not(.dk-live)');
    if (sr && sr.parentNode) sr.parentNode.removeChild(sr);
    card.setAttribute('data-state', 'up');
    card.setAttribute('tabindex', '-1');
    var live = card.querySelector('.dk-live');
    if (live) live.textContent = name; // announced once
    if (hadFocus || focusKept) card.focus({ preventScroll: true });
  }

  // ---- The ten turn-overs: a first half and a second half each ----

  // Each turn-over's phases in milliseconds at normal Motion speed: a is the
  // first half, b the second, each a list of phases that run one after
  // another (effects that run beside a phase are never longer than it). The
  // total is capped at 900, and test/js/deck_cards.test.mjs sums this table.
  var PH = {
    flip: { a: [170], b: [170] }, slow: { a: [370], b: [410] }, toss: { a: [320], b: [320] },
    slam: { a: [250], b: [210, 330] }, sweep: { a: [90], b: [470] }, burn: { a: [120], b: [580] },
    ink: { a: [120], b: [600] }, fade: { a: [], b: [260] }, seal: { a: [120, 220], b: [480] },
    bolt: { a: [300], b: [90, 460] }, forge: { a: [250], b: [560] }
  };
  // The fixed moves: draw, deal, shuffle, play and used only say where a card
  // went and stay under 500; gone may take up to 1000; ready breathes twice.
  var MOVE_PHASES = {
    draw: [460], deal: [460], shuffle: [60, 220, 180], play: [380], used: [360], gone: [780], ready: [1000, 1000]
  };

  function dur(ms) { return Math.round(ms * speed()); }
  function opt(ms, easing, delay) { return { duration: dur(ms), easing: easing, fill: 'forwards', delay: delay ? dur(delay) : 0 }; }

  var TURN_HALVES = {
    flip: {
      three: true,
      a: function (c, t) { return run(c, t, [{ transform: 'rotateY(0)' }, { transform: 'rotateY(90deg)' }], opt(PH.flip.a[0], EASE_FLIP_IN)); },
      b: function (c, t) { return run(c, t, [{ transform: 'rotateY(90deg)' }, { transform: 'rotateY(180deg)' }], opt(PH.flip.b[0], EASE_OUT)); }
    },
    slow: {
      three: true,
      a: function (c, t) {
        layer(c, c, 'dk-fx-shadow');
        return run(c, t, [
          { transform: 'translateY(0) scale(1) rotateY(0)', easing: EASE_OUT },
          { transform: 'translateY(-10px) scale(1.06) rotateY(0)', offset: .45, easing: EASE_INOUT },
          { transform: 'translateY(-10px) scale(1.06) rotateY(90deg)' }
        ], opt(PH.slow.a[0], 'linear'));
      },
      b: function (c, t) {
        return run(c, t, [
          { transform: 'translateY(-10px) scale(1.06) rotateY(90deg)', easing: EASE_INOUT },
          { transform: 'translateY(-10px) scale(1.06) rotateY(180deg)', offset: .5, easing: EASE_INOUT },
          { transform: 'translateY(0) scale(1) rotateY(180deg)' }
        ], opt(PH.slow.b[0], 'linear'));
      }
    },
    toss: {
      three: true,
      a: function (c, t) {
        layer(c, c, 'dk-fx-shadow');
        return run(c, t, [{ transform: 'translateY(0) scale(1) rotateZ(0) rotateY(0)' },
          { transform: 'translateY(-26px) scale(1.09) rotateZ(-5deg) rotateY(270deg)' }], opt(PH.toss.a[0], 'cubic-bezier(.2,.7,.4,1)'));
      },
      b: function (c, t) {
        return run(c, t, [{ transform: 'translateY(-26px) scale(1.09) rotateZ(-5deg) rotateY(270deg)' },
          { transform: 'translateY(0) scale(1) rotateZ(0) rotateY(540deg)' }], opt(PH.toss.b[0], 'cubic-bezier(.5,0,.8,.5)'));
      }
    },
    slam: {
      three: true,
      a: function (c, t) {
        return run(c, t, [{ transform: 'translateY(0) scale(1) rotateY(0)' },
          { transform: 'translateY(-12px) scale(1.16) rotateY(90deg)' }], opt(PH.slam.a[0], 'cubic-bezier(.2,.7,.3,1)'));
      },
      b: function (c, t) {
        var ring = layer(c, c, 'dk-fx-ring');
        var p = run(c, t, [{ transform: 'translateY(-12px) scale(1.16) rotateY(90deg)' },
          { transform: 'translateY(0) scale(1) rotateY(180deg)' }], opt(PH.slam.b[0], 'cubic-bezier(.7,0,1,.6)'));
        return p.then(function () {
          return run(c, ring, [{ opacity: .55, transform: 'scale(1)' }, { opacity: 0, transform: 'scale(1.22)' }], opt(PH.slam.b[1], EASE_OUT));
        });
      }
    },
    sweep: {
      a: function (c, t) {
        var bar = layer(c, t, 'dk-fx-bar');
        bar.__bar = true;
        return run(c, bar, [{ opacity: 0 }, { opacity: 1 }], opt(PH.sweep.a[0], 'linear'));
      },
      b: function (c, t, h) {
        var bar = t.querySelector('.dk-fx-bar');
        var back = t.querySelector('.dk-back');
        var px = t.offsetHeight;
        run(c, bar, [{ transform: 'translateY(0)', opacity: 1 }, { transform: 'translateY(' + px + 'px)', opacity: 1, offset: .9 }, { transform: 'translateY(' + px + 'px)', opacity: 0 }], opt(PH.sweep.b[0], EASE_INOUT));
        return run(c, back, [{ clipPath: 'inset(0 0 0 0 round 12px)' }, { clipPath: 'inset(100% 0 0 0 round 12px)' }], opt(PH.sweep.b[0], EASE_INOUT));
      }
    },
    burn: {
      a: function (c, t) {
        var ember = layer(c, t, 'dk-fx-ember');
        t.classList.add('dk-burning');
        t.style.setProperty('--dk-r', '-4%');
        return Promise.all([
          run(c, t, [{ '--dk-r': '-4%' }, { '--dk-r': '8%' }], opt(PH.burn.a[0], EASE_IN)),
          run(c, ember, [{ opacity: 0 }, { opacity: 1 }], opt(PH.burn.a[0], 'linear'))
        ]);
      },
      b: function (c, t) {
        var ember = t.querySelector('.dk-fx-ember');
        run(c, ember, [{ opacity: 1 }, { opacity: 1, offset: .8 }, { opacity: 0 }], opt(PH.burn.b[0], 'linear'));
        return run(c, t, [{ '--dk-r': '8%' }, { '--dk-r': '125%' }], opt(PH.burn.b[0], 'cubic-bezier(.35,0,.6,1)'));
      }
    },
    ink: {
      a: function (c, t) {
        var back = t.querySelector('.dk-back');
        return run(c, back, [{ filter: 'blur(0) grayscale(0)' }, { filter: 'blur(3px) grayscale(.7)' }], opt(PH.ink.a[0], 'linear'));
      },
      b: function (c, t) {
        var back = t.querySelector('.dk-back');
        var slot = t.querySelector('.dk-faceslot');
        run(c, slot, [{ filter: 'blur(9px) grayscale(1) contrast(2.2) brightness(1.5)', opacity: .2 }, { opacity: 1, offset: .35 }, { filter: 'none', opacity: 1 }], opt(PH.ink.b[0], EASE_OUT));
        return run(c, back, [{ opacity: 1 }, { opacity: 0 }], opt(260, 'ease-out'));
      }
    },
    fade: {
      a: function (c, t) { return Promise.resolve(); },
      b: function (c, t) {
        var back = t.querySelector('.dk-back');
        return run(c, back, [{ opacity: 1 }, { opacity: 0 }], opt(PH.fade.b[0], 'ease-out'));
      }
    },
    seal: {
      a: function (c, t) {
        var wax = layer(c, t, 'dk-fx-wax', '<i class="dk-l"></i><i class="dk-r"></i>');
        var l = wax.querySelector('.dk-l'), r = wax.querySelector('.dk-r');
        var press = run(c, wax, [{ transform: 'scale(1)' }, { transform: 'scale(.9)', offset: .5 }, { transform: 'scale(1)' }], opt(PH.seal.a[0], EASE_INOUT));
        return press.then(function () {
          run(c, l, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'translate(-16px,46px) rotate(-38deg)' }], opt(PH.seal.a[1], EASE_IN));
          return run(c, r, [{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'translate(14px,52px) rotate(30deg)' }], opt(PH.seal.a[1], EASE_IN));
        });
      },
      b: function (c, t) {
        var back = t.querySelector('.dk-back');
        var doors = ['dk-l', 'dk-r'].map(function (side) {
          var d = layer(c, t, 'dk-fx-door ' + side);
          d.appendChild(back.cloneNode(true));
          return d;
        });
        t.style.perspective = '700px';
        back.style.opacity = '0';
        return Promise.all([
          run(c, doors[0], [{ opacity: 1, transform: 'none', filter: 'brightness(1)' }, { opacity: 1, offset: .8 }, { opacity: 0, transform: 'rotateY(-110deg)', filter: 'brightness(.6)' }], opt(PH.seal.b[0], 'cubic-bezier(.45,.05,.25,1)')),
          run(c, doors[1], [{ opacity: 1, transform: 'none', filter: 'brightness(1)' }, { opacity: 1, offset: .8 }, { opacity: 0, transform: 'rotateY(110deg)', filter: 'brightness(.6)' }], opt(PH.seal.b[0], 'cubic-bezier(.45,.05,.25,1)'))
        ]);
      }
    },
    bolt: {
      a: function (c, t) {
        var bolt = layer(c, t, 'dk-fx-bolt', '<svg viewBox="0 0 52 150" preserveAspectRatio="none" width="100%" height="100%"><path d="M30 0L12 62h14L16 110l30-70H30L40 0z"/></svg>');
        var flash = layer(c, t, 'dk-fx-flash');
        run(c, flash, [{ opacity: 0 }, { opacity: 0, offset: .12 }, { opacity: .96, offset: .18 }, { opacity: 0 }], opt(500, 'linear'));
        return run(c, bolt, [{ opacity: 1, clipPath: 'inset(0 0 100% 0)' }, { clipPath: 'inset(0 0 0 0)', offset: .22 }, { opacity: 1, offset: .4 }, { opacity: .25, offset: .5 }, { opacity: 1, offset: .6 }, { opacity: 0, clipPath: 'inset(0 0 0 0)' }], opt(PH.bolt.a[0], 'linear'));
      },
      b: function (c, t) {
        var back = t.querySelector('.dk-back');
        var slot = t.querySelector('.dk-faceslot');
        var hide = run(c, back, [{ opacity: 1 }, { opacity: 0 }], opt(PH.bolt.b[0], 'linear'));
        return hide.then(function () {
          return run(c, slot, [{ filter: 'brightness(2.4) saturate(.4)', transform: 'translateX(-3px)' }, { transform: 'translateX(3px)', offset: .12 }, { transform: 'translateX(-1.5px)', offset: .24 }, { transform: 'translateX(0)', offset: .36 }, { filter: 'none', transform: 'none' }], opt(PH.bolt.b[1], EASE_OUT));
        });
      }
    },
    forge: {
      a: function (c, t) {
        var back = t.querySelector('.dk-back');
        var heat = layer(c, c, 'dk-fx-heat');
        run(c, heat, [{ opacity: 0 }, { opacity: 1, offset: .2 }, { opacity: 0 }], opt(620, EASE_OUT));
        return run(c, back, [{ filter: 'sepia(0) saturate(1) hue-rotate(0deg) brightness(1)', opacity: 1 }, { filter: 'sepia(1) saturate(6) hue-rotate(-35deg) brightness(1.5)', opacity: 1 }], opt(PH.forge.a[0], EASE_INOUT));
      },
      b: function (c, t) {
        var back = t.querySelector('.dk-back');
        var slot = t.querySelector('.dk-faceslot');
        run(c, back, [{ filter: 'sepia(1) saturate(6) hue-rotate(-35deg) brightness(1.5)', opacity: 1 }, { filter: 'sepia(1) saturate(6) hue-rotate(-35deg) brightness(1.8) blur(2px)', opacity: 0 }], opt(130, EASE_INOUT));
        return run(c, slot, [{ filter: 'sepia(1) saturate(7) hue-rotate(-30deg) brightness(2) contrast(1.1)' }, { filter: 'sepia(.9) saturate(5) hue-rotate(-42deg) brightness(1.1) contrast(1.1)', offset: .4 }, { filter: 'none' }], opt(PH.forge.b[0], EASE_OUT));
      }
    }
  };

  function plainPlate(card) {
    var p = document.createElement('div');
    p.className = 'dk-plain dk-s-' + shapeOf(card.getAttribute('data-shape'));
    return p;
  }

  /** turnOver: play the deck's turn-over on one face-down card. */
  function turnOver(card, loadFace) {
    if (!card || card.getAttribute('data-state') !== 'down' || card.__dkTurning) return Promise.resolve();
    card.__dkTurning = true;
    var load = loadFace || loaderFor(card);
    var btn = card.querySelector('.dk-turn');
    if (btn) btn.disabled = true;
    var done = turnQueue.push(function () { state(card).dead = false; return playTurn(card, load); }).then(function () {
      card.__dkTurning = false;
    }, function () { card.__dkTurning = false; });
    card.__dkTurnDone = done;
    return done;
  }

  function playTurn(card, load) {
    var plan = planTurn(turnOf(card.getAttribute('data-turn')), {
      level: pageLevel(), shape: shapeOf(card.getAttribute('data-shape')), weak: deviceIsLimited(), crowded: crowded(card),
      staged: !!card.closest('[data-dk-stage]')
    });
    var turner = card.querySelector('.dk-turner');
    var slot = turner.querySelector('.dk-faceslot');
    var back = turner.querySelector('.dk-back');
    var spec = TURN_HALVES[plan.name] || TURN_HALVES.flip;
    var failed = false;
    var revealing = false;

    // A turn that cannot finish puts the card back exactly as it was: face
    // down, no inline style or effect layer left by either half, the button
    // live again and focus on it, and one plain sentence in the live region.
    function fail() {
      failed = true;
      state(card).dead = true;
      settle(card);
      [turner, back, slot].forEach(function (n) { n.removeAttribute('style'); });
      slot.textContent = '';
      var live = card.querySelector('.dk-live');
      if (live) live.textContent = FAIL_WORDS;
      var btn = card.querySelector('.dk-turn');
      if (btn) { btn.disabled = false; btn.focus({ preventScroll: true }); }
    }
    function landSlot() {
      var tmp = document.createDocumentFragment();
      while (slot.firstChild) tmp.appendChild(slot.firstChild);
      var plain = tmp.querySelector('.dk-plain');
      if (plain) plain.parentNode.removeChild(plain);
      land(card, tmp, true);
    }

    if (plan.kind === 'off') {
      return Promise.resolve().then(load).then(function (markup) {
        land(card, faceNodes(markup), true);
        return pictureReady(card);
      }, fail);
    }

    if (plan.kind === 'calm') {
      return Promise.resolve().then(load).then(function (markup) {
        slot.appendChild(faceNodes(markup));
        return pictureReady(slot);
      }).then(function () {
        turner.classList.add('dk-flat');
        return run(card, back, [{ opacity: 1 }, { opacity: 0 }], opt(CALM_MS, 'ease-out'));
      }).then(landSlot, fail);
    }

    // Full. A 3D turn keeps the slot behind the back; a flat one reveals it,
    // so the slot holds a plain plate until the face is there.
    turner.classList.add(spec.three ? 'dk-3d' : 'dk-flat');
    card.classList.add('dk-turning');
    slot.appendChild(plainPlate(card));

    return sequence({
      load: function () {
        return Promise.resolve().then(load).then(function (markup) {
          var late = revealing;
          slot.textContent = '';
          slot.appendChild(faceNodes(markup));
          if (late) run(card, slot, [{ opacity: 0 }, { opacity: 1 }], opt(CALM_MS, 'ease-out'));
          return pictureReady(slot).then(function () { return markup; });
        });
      },
      firstHalf: function () { return spec.a(card, turner); },
      sleep: sleep,
      wait: HALFWAY_WAIT,
      secondHalf: function () { revealing = true; return spec.b(card, turner); },
      fill: function () {},
      fail: fail
    }).then(function () { if (!failed) landSlot(); });
  }

  // ---- The fixed moves ----

  function travel(el, dx, dy, rot, ms, lift) {
    var s = lift ? 1.07 : 1;
    return run(el, el, [
      { transform: 'translate(0,0) rotate(0) scale(1)', easing: EASE_OUT },
      { transform: 'translate(0,' + (lift ? -8 : 0) + 'px) rotate(0) scale(' + s + ')', offset: .25, easing: EASE_INOUT },
      { transform: 'translate(' + dx + 'px,' + dy + 'px) rotate(' + (rot || 0) + 'deg) scale(' + s + ')', offset: .85, easing: EASE_OUT },
      { transform: 'translate(' + dx + 'px,' + dy + 'px) rotate(' + (rot || 0) + 'deg) scale(1)' }
    ], opt(ms, 'linear'));
  }

  function fadeIn(el) { return run(el, el, [{ opacity: 0 }, { opacity: 1 }], opt(CALM_MS, 'ease-out')); }

  var MOVE_FN = {
    draw: function (el, o, level) {
      if (level === 'off') return Promise.resolve();
      if (level === 'calm') return fadeIn(el);
      return travel(el, o.dx || 0, o.dy || 0, 0, MOVE_PHASES.draw[0], true);
    },
    deal: function (el, o, level) {
      if (level === 'off') return Promise.resolve();
      if (level === 'calm') return fadeIn(el);
      return travel(el, o.dx || 0, o.dy || 0, 12, MOVE_PHASES.deal[0], true);
    },
    play: function (el, o, level) {
      if (level === 'off') return Promise.resolve();
      if (level === 'calm') return fadeIn(el);
      return travel(el, o.dx || 0, o.dy || 0, 0, MOVE_PHASES.play[0], true);
    },
    shuffle: function (el, o, level) {
      if (level === 'off') return Promise.resolve();
      if (level === 'calm') return run(el, el, [{ opacity: 1 }, { opacity: .45, offset: .5 }, { opacity: 1 }], opt(400, EASE_INOUT));
      var sx = (o.index % 2 ? 1 : -1) * 34, sy = -4 - (o.index % 3) * 3, sr = (o.index % 2 ? 1 : -1) * 7;
      var t = 'translate(' + sx + 'px,' + sy + 'px) rotate(' + sr + 'deg)';
      var p = run(el, el, [{ transform: 'none' }, { transform: t }], opt(MOVE_PHASES.shuffle[1], EASE_OUT, (o.index % 3) * 30));
      return p.then(function () { return run(el, el, [{ transform: t }, { transform: 'none' }], opt(MOVE_PHASES.shuffle[2], EASE_INOUT)); });
    },
    used: function (el, o, level) {
      var end = function () { el.classList.add('dk-spent'); };
      if (level === 'off') { end(); return Promise.resolve(); }
      if (level === 'calm') return run(el, el, [{ opacity: 1 }, { opacity: .45, offset: .5 }, { opacity: 1 }], opt(400, EASE_INOUT)).then(end);
      return run(el, el, [
        { transform: 'none', filter: 'saturate(1) brightness(1)', easing: EASE_OUT },
        { transform: 'translate(0,-6px) scale(1.04)', filter: 'saturate(1) brightness(1)', offset: .18, easing: EASE_SLIDE },
        { transform: 'translate(' + (o.dx || 0) + 'px,' + (o.dy || 4) + 'px) rotate(9deg) scale(.96)', filter: 'saturate(.35) brightness(.9)' }
      ], opt(MOVE_PHASES.used[0], 'linear')).then(end);
    },
    gone: function (el, o, level) {
      var end = function () { el.hidden = true; };
      if (level === 'off') { end(); return Promise.resolve(); }
      if (level === 'calm') return run(el, el, [{ opacity: 1 }, { opacity: 0 }], opt(CALM_MS, 'ease-in')).then(end);
      el.classList.add('dk-gone-mask');
      for (var i = 0; i < 6; i++) {
        var m = layer(el, el, 'dk-fx-mote');
        m.style.left = (12 + i * 14) + '%';
        m.style.top = (30 + (i * 17) % 50) + '%';
        run(el, m, [{ opacity: .9, transform: 'none' }, { opacity: 0, transform: 'translate(' + ((i % 2 ? 1 : -1) * (6 + i * 3)) + 'px,-34px)' }], opt(320, EASE_OUT, 60 * i));
      }
      return run(el, el, [
        { filter: 'grayscale(0) brightness(1)', '--dk-g': '-20%', opacity: 1, transform: 'none' },
        { filter: 'grayscale(1) brightness(.8)', '--dk-g': '-20%', opacity: 1, offset: .28 },
        { filter: 'grayscale(1) brightness(.7)', '--dk-g': '110%', opacity: 0, transform: 'translateY(-6px)' }
      ], opt(MOVE_PHASES.gone[0], EASE_INOUT)).then(function () { el.classList.remove('dk-gone-mask'); end(); });
    },
    ready: function (el, o, level) {
      if (level === 'off') return Promise.resolve();
      var glow = layer(el, el, 'dk-fx-glow');
      var breath = [{ opacity: 0 }, { opacity: 1, offset: .35 }, { opacity: 0 }];
      if (level === 'calm') return run(el, glow, breath, { duration: dur(MOVE_PHASES.ready[0]), iterations: 2, easing: EASE_INOUT, fill: 'none' });
      run(el, el, [{ transform: 'none' }, { transform: 'translateY(-3px)', offset: .35 }, { transform: 'none' }], { duration: dur(MOVE_PHASES.ready[0]), iterations: 2, easing: EASE_INOUT, fill: 'none' });
      return run(el, glow, breath, { duration: dur(MOVE_PHASES.ready[0]), iterations: 2, easing: EASE_INOUT, fill: 'none' });
    }
  };

  /**
   * move: play one fixed move. opts.dx and opts.dy say how far the card goes
   * (the screen that asked knows where it is going); opts.done runs when the
   * move ends, before the card is cleaned, so the caller can place the card
   * where the move took it. A new move on the same card settles the old one.
   */
  function move(el, name, opts) {
    opts = opts || {};
    if (MOVES.indexOf(name) < 0 || !el) return Promise.resolve();
    // A card that is mid-turn is brought to its end state first (face up,
    // plain, nothing left over); the move never strips a turn half-way.
    var pre = Promise.resolve();
    if (el.__dkTurning && el.__dkTurnDone) {
      var st0 = state(el);
      st0.skip = true;
      st0.anims.slice().forEach(function (a) { try { a.finish(); } catch (e) { /* already ended */ } });
      pre = el.__dkTurnDone.then(function () { st0.skip = false; });
    }
    return pre.then(function () {
      state(el).dead = false;
      settle(el);
      return MOVE_FN[name](el, opts, pageLevel());
    }).then(function () {
      if (opts.done) opts.done(el);
      var st = state(el);
      st.anims.splice(0).forEach(function (a) { try { a.cancel(); } catch (e) { /* gone */ } });
      st.fx.splice(0).forEach(function (n) { if (n.parentNode) n.parentNode.removeChild(n); });
    });
  }

  /** deal: move up to seven cards, 60ms apart; later ones are simply there. */
  function deal(els, opts) {
    var plan = dealPlan(els.length);
    return Promise.all(plan.map(function (p) {
      if (!p.moves) return Promise.resolve();
      var o = {};
      Object.keys(opts || {}).forEach(function (k) { o[k] = opts[k]; });
      return sleep(pageLevel() === 'full' ? p.delay : 0).then(function () { return move(els[p.index], 'deal', o); });
    }));
  }

  /** shuffle: the cards of a pile split and rejoin. */
  function shuffle(els) {
    return Promise.all(els.map(function (el, i) { return move(el, 'shuffle', { index: i }); }));
  }

  // ---- Mounting ----

  // The Director's proof sheet (looks_page.templ) plays a sample per button:
  // one delegated handler on the sheet, bound the first time a card mounts.
  function bindProof(root) {
    if (!root || root.__dkProof) return;
    root.__dkProof = true;
    root.addEventListener('click', function (ev) {
      var b = ev.target.closest && ev.target.closest('[data-dk-move],[data-dk-reset]');
      var one = b && b.closest('[data-dk-one]');
      if (!one || !root.contains(one)) return;
      if (b.hasAttribute('data-dk-reset')) return resetSample(one);
      var cards = Array.prototype.slice.call(one.querySelectorAll('[data-dk-card]'));
      var name = b.getAttribute('data-dk-move');
      var o = { dx: parseInt(b.getAttribute('data-dx'), 10) || 0, dy: parseInt(b.getAttribute('data-dy'), 10) || 0 };
      if (name === 'shuffle') return shuffle(cards);
      if (name === 'deal') return deal(cards, o);
      move(cards[0], name, o);
    });
  }

  function resetSample(one) {
    var slot = one.querySelector('[data-dk-slot]');
    var tpl = one.querySelector('template.dk-down-tpl');
    if (slot && tpl) {
      slot.innerHTML = tpl.innerHTML; // the server's own markup for the card face down
      var c = slot.querySelector('[data-dk-card]');
      if (c) { c.__dkMounted = false; mount(c); }
      return;
    }
    one.querySelectorAll('[data-dk-card]').forEach(function (c) {
      settle(c);
      c.hidden = false;
      c.classList.remove('dk-spent', 'dk-gone-mask');
    });
  }

  function mount(card) {
    if (card.__dkMounted) return;
    card.__dkMounted = true;
    bindProof(card.closest('[data-dk-proof]'));
    var btn = card.querySelector('.dk-turn');
    if (btn) {
      card.__dkClick = function () { turnOver(card); };
      btn.addEventListener('click', card.__dkClick);
    }
  }

  function unmount(card) {
    var btn = card.querySelector('.dk-turn');
    if (btn && card.__dkClick) btn.removeEventListener('click', card.__dkClick);
    settle(card);
    card.__dkMounted = false;
  }

  window.DeckCards = {
    TURNS: TURNS, MOVES: MOVES, PH: PH, MOVE_PHASES: MOVE_PHASES, HALFWAY_WAIT: HALFWAY_WAIT, CROWD: CROWD, DEAL_MAX: DEAL_MAX, DEAL_GAP: DEAL_GAP,
    levelOf: levelOf, deviceIsLimited: deviceIsLimited, planTurn: planTurn, finishFor: finishFor,
    dealPlan: dealPlan, crowdedFor: crowdedFor, shapeOf: shapeOf, turnOf: turnOf, SHAPES: SHAPES, createQueue: createQueue, sequence: sequence,
    pictureSrcOk: pictureSrcOk, pictureSrcOf: pictureSrcOf, faceSrcOk: faceSrcOk, pictureReady: pictureReady, turnOver: turnOver, move: move, deal: deal, shuffle: shuffle
  };

  if (C.register) {
    C.register('deck-cards', {
      init: function (node) { mount(node); },
      destroy: function (node) { unmount(node); }
    });
  }
})();
