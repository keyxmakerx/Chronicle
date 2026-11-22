/**
 * hero_stage.js — the hero creator's "stage" look: the hero stands at the top
 * of the page (a banner on a phone, the rail on a computer) and shows its
 * name, its line and its figures while the player chooses.
 *
 * window.ChronicleHeroStage(Creator, kit) returns a subclass of the creator
 * that overrides only what this look draws or does differently. It is used
 * when the mount carries data-look="stage"; hero_creator.js builds it once and
 * hands over the helpers in kit. Everything else (steps, rules, drafts, own
 * entries, practice and try-out) is the creator's own and is not touched.
 *
 * Looking at an entry tries it on: the figures it would change show their new
 * value beside the old one, computed with the pick set and put back, so
 * nothing is saved or scrubbed by looking.
 *
 * Listed before hero_creator.js in the body-script registry, so it is defined
 * by the time a mount is built. Styles are hero_stage.css.
 */
(function () {
  'use strict';

  // The figures the banner and the rail show, in order.
  var FIGS = [['stamina', 'Stamina'], ['speed', 'Speed'], ['size', 'Size'], ['stab', 'Stability']];
  // The entry properties that can produce any of them; a system with none of
  // these in its plan has no figures to show.
  var FIG_KEYS = ['starting_stamina', 'speed', 'size', 'stability', 'stamina_bonus', 'speed_bonus', 'stability_bonus'];
  var DASH = '–';
  var ARROW = '→';

  window.ChronicleHeroStage = function (Creator, kit) {
    var esc = kit.esc, lower = kit.lower, I = kit.I, qa = kit.qa, dockedBar = kit.dockedBar, hasOwn = kit.hasOwn;

    function Stage(root) {
      Creator.call(this, root);
      this.inerted = [];
      this.low = false;
      this.borders = makeBorders(root);
    }
    Stage.prototype = Object.create(Creator.prototype);
    Stage.prototype.constructor = Stage;
    var base = Creator.prototype, P = Stage.prototype;

    // The borders library the page put on the mount, handed to the colouring
    // script. A missing or unreadable library, or a page without the script,
    // leaves every card plain.
    function makeBorders(root) {
      try {
        var lib = JSON.parse(root.getAttribute('data-borders') || 'null');
        if (lib && typeof lib === 'object' && typeof window.ChronicleHeroBorders === 'function') return window.ChronicleHeroBorders(lib, root);
      } catch (e) { /* plain cards */ }
      return null;
    }

    // --- The window ---

    // The creator fills the window, so the view follows the window, not the
    // mount's column.
    P.measure = function () {
      var w = document.documentElement.clientWidth || window.innerWidth, v = w >= 900 ? 'desk' : 'phone', changed = v !== this.view;
      this.view = v;
      return changed;
    };

    // The window less whatever the docked Table bar covers.
    P.room = function () {
      return { height: window.innerHeight - dockedBar(window.innerHeight), min: 240 };
    };

    // The page behind is covered, never scrolled into place.
    P.alignTop = function () { this.aligned = false; };

    // Everything else on the page is behind the creator, so it is out of reach
    // of the keyboard and screen readers while the look is open: at each level
    // from the mount up to the body, every sibling goes inert. Left usable:
    // the docked Table bar (a body child or fixed, with its ancestors), and the
    // site's toasts, which announce and offer Undo over the creator. Only what
    // this took is given back, so a page that set its own inert is left as it
    // was. It runs on every render and again when something joins a level the
    // walk crossed (a bar HTMX brings in late).
    var SKIP_TAGS = { SCRIPT: 1, STYLE: 1, LINK: 1, TEMPLATE: 1, NOSCRIPT: 1 };

    P.usable = function () {
      var out = [], toast = document.getElementById('chronicle-toasts'), bar = document.getElementById('tt-drawer');
      if (toast) out.push(toast);
      if (bar && (bar.parentElement === document.body || getComputedStyle(bar).position === 'fixed')) out.push(bar);
      return out;
    };

    P.lockSite = function (on) {
      var self = this;
      if (!on) {
        if (this.siteWatch) { this.siteWatch.disconnect(); this.siteWatch = null; }
        if (this.siteKick) { cancelAnimationFrame(this.siteKick); this.siteKick = 0; }
        this.inerted.forEach(function (el) { el.removeAttribute('inert'); });
        this.inerted = [];
        document.documentElement.removeAttribute('data-full-surface');
        return;
      }
      // Page shortcuts stand down while a surface covers the window.
      document.documentElement.setAttribute('data-full-surface', '');
      var keep = this.usable(), levels = [], node = this.root;
      while (node && node !== document.body && node.parentElement) {
        var par = node.parentElement, mine = node;
        levels.push(par);
        Array.prototype.forEach.call(par.children, function (sib) {
          if (sib === mine || SKIP_TAGS[sib.tagName]) return;
          var held = keep.some(function (k) { return sib === k || sib.contains(k); }), at = self.inerted.indexOf(sib);
          if (held) {
            if (at >= 0) { sib.removeAttribute('inert'); self.inerted.splice(at, 1); }
          } else if (at < 0 && !sib.hasAttribute('inert')) {
            sib.setAttribute('inert', '');
            self.inerted.push(sib);
          }
        });
        node = par;
      }
      if (!window.MutationObserver) return;
      if (!this.siteWatch) {
        this.siteWatch = new MutationObserver(function () {
          if (self.siteKick) return;
          self.siteKick = requestAnimationFrame(function () {
            self.siteKick = 0;
            if (self.siteWatch && document.body.contains(self.root)) self.lockSite(true);
          });
        });
      }
      this.siteWatch.disconnect();
      levels.forEach(function (el) { self.siteWatch.observe(el, { childList: true }); });
    };

    P.render = function () {
      base.render.call(this);
      this.lockSite(true);
      // What is on screen now is the figures' starting point for the next count.
      this.lastStats = this.stats();
    };

    // The Table bar slides to its resting place after its stylesheet arrives,
    // and a slide moves the bar without resizing it, which the base's
    // observer cannot see. Fit again when the bar's slide ends at rest.
    P.watch = function () {
      var self = this, pend = 0;
      base.watch.call(this);
      this.onBarSlide = function (ev) {
        var bar = document.getElementById('tt-drawer');
        if (!bar || ev.target !== bar || ev.propertyName !== 'transform') return;
        // Lifted or full, the phone sheet covers the creator on purpose, and
        // the creator keeps the height it had under the resting bar.
        var h = bar.getAttribute('data-h');
        if (h && h !== 'rest') return;
        if (pend) return;
        pend = requestAnimationFrame(function () { pend = 0; if (document.body.contains(self.root)) self.fit(); });
      };
      document.addEventListener('transitionend', this.onBarSlide, true);
    };

    P.destroy = function () {
      if (this.borders) { this.borders.destroy(); this.borders = null; }
      if (this.onBarSlide) { document.removeEventListener('transitionend', this.onBarSlide, true); this.onBarSlide = null; }
      this.lockSite(false);
      base.destroy.call(this);
    };

    // --- The top bar ---

    P.leaveWord = function () { return this.tryout || this.preload ? 'Leave' : 'Save and leave'; };

    // Nothing is kept as a draft in a try-out or on a copy or change, so there
    // is nothing to say about it. Before the first write nothing has failed.
    P.statusHTML = function () {
      if (this.tryout || this.preload) return '';
      var bad = this.kept === false;
      return '<span class="hc-sv' + (bad ? ' is-bad' : '') + '" data-saved>' + (bad ? 'Not saved: this browser won\'t keep it' : 'Saved on this device') + '</span>';
    };

    P.save = function () {
      var was = this.kept;
      base.save.call(this);
      if (this.kept === was) return;
      var el = this.$('[data-saved]');
      if (!el) return;
      el.className = 'hc-sv' + (this.kept === false ? ' is-bad' : '');
      el.textContent = this.kept === false ? 'Not saved: this browser won\'t keep it' : 'Saved on this device';
    };

    P.barHTML = function (back) {
      var desk = this.desk();
      return '<header class="hc-abar"><a class="hc-lv" href="' + esc(back) + '">' + I.back + '<span>' + this.leaveWord() + '</span></a>' +
        (desk ? '<div class="hc-abar__t"><h1>' + esc(this.title()) + '</h1><small>' + esc(this.plan.systemName || 'Game system') + '</small></div>'
          : '<h1 class="hc-sr">' + esc(this.title()) + '</h1>') + this.statusHTML() + '</header>';
    };

    // A phone seats the bar and the hero in one banner, but not on Name and
    // review, where the hero card in the page takes over. On a computer the
    // hero stands in the rail and the bar stands alone.
    P.topHTML = function (back) {
      if (this.desk()) return this.barHTML(back);
      return '<div class="hc-stgw">' + this.barHTML(back) + (this.isDetails() ? '' : this.stageHTML()) + '</div>';
    };

    // The banner holds the picks, so there is no chip row.
    P.chipsBlockHTML = function () { return ''; };
    P.cardIx = function () { return ''; };

    // An entry that names a border wears it; the script leaves a wrong name
    // or colour plain.
    P.cardAttrs = function (c) { return this.borders ? this.borders.attrs(c.look) : ''; };

    // A framed card is deeper-padded and lets its border overhang, so the
    // content passing the card's edge is not the only way words reach the
    // frame: the foot line must also clear the bottom padding.
    P.overflow = function (card) {
      var over = base.overflow.call(this, card), more = card.hasAttribute('data-trim') && card.querySelector('.hc-more');
      if (!more) return over;
      var pad = parseFloat(getComputedStyle(card).paddingBottom) || 0;
      return Math.max(over, more.offsetTop + more.offsetHeight - (card.clientHeight - pad));
    };

    // --- The hero ---

    // Decided once from the plan: does any entry produce a figure at all?
    P.hasFigures = function () {
      if (this.figs == null) {
        this.figs = this.plan.steps.some(function (st) {
          return (st.choices || []).some(function (c) {
            var p = c.properties || {};
            return FIG_KEYS.some(function (k) { return hasOwn(p, k) && p[k] != null && p[k] !== ''; });
          });
        });
      }
      return this.figs;
    };

    P.heroName = function () { return this.name.trim() || 'Unnamed hero'; };
    P.initial = function () {
      var n = this.name.trim();
      return n ? Array.from(n)[0].toUpperCase() : '?';
    };

    // The portrait goes to About your hero when the plan has that step.
    P.portrait = function (big) {
      var l = esc(this.initial());
      if (!big && this.hasAbout()) return '<button type="button" class="hc-por" data-go="' + this.plan.steps.length + '" aria-label="About your hero">' + l + '</button>';
      return '<span class="hc-por' + (big ? ' hc-por--big' : '') + '" aria-hidden="true">' + l + '</span>';
    };

    // The hero in a few words: ancestry and class as the party block words
    // them, else the first picks, with pronouns after. The review's card adds
    // the next pick the line does not already name.
    P.heroLine = function (fuller) {
      var self = this, line = this.summary(this.picks) || this.line(2), out = [line || 'Choose something to begin'];
      if (fuller && line) {
        var more = '';
        this.plan.steps.forEach(function (st, i) {
          var p = self.picked(i);
          if (!more && p && lower(line).indexOf(lower(p)) < 0) more = p;
        });
        if (more) out.push(more);
      }
      var pro = typeof this.about.pronouns === 'string' ? this.about.pronouns.trim() : '';
      if (pro) out.push(pro);
      return out.join(' · ');
    };

    P.figuresHTML = function () {
      if (!this.hasFigures()) return '';
      var s = this.stats();
      return '<div class="hc-sstw"><div class="hc-sst">' + FIGS.map(function (f) {
        var v = s[f[0]];
        return '<div class="hc-st"><span>' + f[1] + '</span><b data-stat="' + f[0] + '">' + (v == null ? DASH : esc(v)) + '</b><i class="hc-pv" data-pv="' + f[0] + '"></i></div>';
      }).join('') + '</div></div>';
    };

    P.stageInner = function () {
      return '<div class="hc-stg__id">' + this.portrait(false) + '<div class="hc-stg__tx"><p class="hc-stg__nm hc-disp">' + esc(this.heroName()) +
        '</p><p class="hc-stg__ln">' + esc(this.heroLine(false)) + '</p><p class="hc-stg__pv"></p></div></div>' + this.figuresHTML();
    };
    // A banner drawn while the page is giving way for the cards stands tucked,
    // so a redraw never flashes it open.
    P.stageHTML = function () { return '<div class="hc-stg' + (this.low ? ' is-tuck' : '') + '" data-stage>' + this.stageInner() + '</div>'; };

    P.railHead = function () { return this.stageHTML(); };
    // The hero stands at the head of the rail, so the old foot block is gone.
    P.sofarHTML = function () { return ''; };

    // The review's card is the hero.
    P.heroCard = function () {
      return '<div class="hc-hero">' + this.portrait(true) + '<div class="hc-who">' + (this.practice ? 'Practice hero' : 'New hero') + '</div><h2 class="hc-disp" data-hname>' + esc(this.heroName()) + '</h2>' +
        '<p class="hc-line">' + esc(this.heroLine(true)) + '</p>' + this.statsHTML(false) + '</div>';
    };

    P.aboutHeadHTML = function () {
      return '<h2 class="hc-disp hc-rev__t" tabindex="-1">About your hero <span class="hc-opt">Optional</span></h2>' +
        '<p class="hc-ahd">Fill in what you like now. The rest can wait for the sheet.</p>';
    };

    // --- Looking shows what would change ---

    // The entry in front of the player now, or null where there is none to
    // look at: not on a list step, while the own-entry editor is open, or
    // while a card is still flying.
    P.frontEntry = function () {
      if (!this.plan || this.busy || this.own || this.isExtra()) return null;
      if (this.desk() || this.mode === 'cards' || (this.sh && !this.sh.own)) return this.curEntry();
      return null;
    };

    // The figures as they would be with entry c picked at step st: the pick is
    // set, read and put back, and nothing is saved.
    P.tryOn = function (st, c) {
      var k = st.fieldKey, had = hasOwn(this.picks, k), was = this.picks[k], out, options = this.mine(k, c);
      this.picks[k] = { name: c.name, options: options };
      try { out = this.stats(); } finally { if (had) this.picks[k] = was; else delete this.picks[k]; }
      return out;
    };

    P.previewStage = function () {
      var box = this.$('[data-stage]');
      if (!box || !this.plan) return;
      var e = this.frontEntry(), st = this.plan.steps[this.step], pv = null, any = false;
      if (e && st && !this.isPicked(st, e)) pv = this.tryOn(st, e);
      var cur = this.stats();
      qa('[data-pv]', box).forEach(function (el) {
        var k = el.getAttribute('data-pv'), show = !!pv && pv[k] != null && pv[k] !== cur[k];
        var txt = show ? (cur[k] == null ? String(pv[k]) : ARROW + ' ' + pv[k]) : '';
        if (el.textContent !== txt) el.textContent = txt;
        el.parentNode.classList.toggle('is-pvonly', show && cur[k] == null);
        if (show) any = true;
      });
      var line = this.$('.hc-stg__pv', box), words = pv ? (any ? 'If you choose ' : 'Looking at ') + e.name : '';
      if (line && line.textContent !== words) line.textContent = words;
    };

    P.refreshStage = function (glow) {
      var box = this.$('[data-stage]');
      if (!box) return;
      box.innerHTML = this.stageInner();
      if (glow) this.wash();
      this.countStats();
    };

    // The line flashes once, under Lively and Gentle.
    P.wash = function () {
      if (this.still()) return;
      var l = this.$('.hc-stg__ln');
      if (!l) return;
      l.classList.remove('is-lit'); void l.offsetWidth; l.classList.add('is-lit');
    };

    P.countStats = function () {
      base.countStats.call(this);
      this.previewStage();
    };

    // Wherever the entry in front changes, the preview follows.
    P.after = function () {
      base.after.call(this);
      this.afterRoom();
      this.tuckFor(this.scroller());
      this.previewStage();
    };
    P.refreshList = function () { base.refreshList.call(this); this.afterRoom(); this.tuckFor(this.scroller()); };
    P.fit = function () {
      base.fit.call(this);
      // What the room alone asks of the intro line, before the cards ask more.
      this.roomShort = this.short;
      this.afterRoom();
    };

    // When making room changed the hand's height, the cards are fitted to it.
    P.afterRoom = function () {
      if (!this.makeRoom() || !this.$('[data-hand]')) return;
      this.fitCards(); this.scrollHandTo(this.curIdx(), false); this.tilt();
    };
    P.refreshUnder = function () { base.refreshUnder.call(this); this.previewStage(); };
    P.refreshRight = function () { base.refreshRight.call(this); this.previewStage(); };
    P.openSheet = function (i, own) { base.openSheet.call(this, i, own); this.previewStage(); };
    P.closeSheet = function (now, cb, refocus) {
      var r = base.closeSheet.call(this, now, cb, refocus);
      this.previewStage();
      return r;
    };
    P.showOwn = function (instant) { base.showOwn.call(this, instant); this.previewStage(); };
    P.endOwn = function () { base.endOwn.call(this); this.previewStage(); };
    // A purchase can change the figures or what an entry would give.
    P.refreshBuy = function (t) { base.refreshBuy.call(this, t); this.refreshStage(false); };

    // --- Choosing ---

    // The card flies to the hero's line, and the stage is redrawn when it lands.
    P.flightTarget = function () { return this.$('.hc-stg__ln'); };
    P.prepFlight = function () { /* the banner is always in view */ };
    P.landed = function () {
      // The last pick can end the lists: then the banner gives way to the card.
      if (this.isDetails()) {
        var w = this.$('.hc-stgw');
        if (w) w.outerHTML = this.topHTML(this.backHref());
        return;
      }
      this.refreshStage(true);
    };
    P.glowPick = function (key) {
      var p = this.picks[key];
      if (p && p.name) this.wash();
    };

    // --- Room for the cards ---

    // The name's line at the fit's floor: a name is never set below 20px, so
    // its line is that many px at the heading's own line height.
    var NAME_FLOOR = 20, NAME_LINE = 1.05;

    // The least height the hand needs for every card to read whole: a card's
    // padding and frame, its label, one line of name and the foot line, with
    // the gaps between them, plus the hand's own padding. Cards of one border
    // are alike, so one of each kind is read, not every card of a long step.
    P.handNeed = function (hand) {
      var seen = {}, need = 0, px = function (v) { return parseFloat(v) || 0; };
      qa('.hc-card', hand).forEach(function (c) {
        var more = c.querySelector('.hc-more'), kind = c.getAttribute('data-trim') || '';
        if (!more || seen[kind]) return;
        seen[kind] = true;
        var cs = getComputedStyle(c), top = c.querySelector('.hc-top'), h3 = c.querySelector('h3'), ratio = NAME_LINE;
        if (h3) { var hs = getComputedStyle(h3), r = px(hs.lineHeight) / px(hs.fontSize); if (r > 0) ratio = r; }
        need = Math.max(need, px(cs.paddingTop) + px(cs.paddingBottom) + px(cs.borderTopWidth) + px(cs.borderBottomWidth) +
          (top ? top.offsetHeight : 0) + more.offsetHeight + NAME_FLOOR * ratio + 2 * px(cs.rowGap));
      });
      if (!need) return 0;
      var hs2 = getComputedStyle(hand);
      return Math.ceil(need + px(hs2.paddingTop) + px(hs2.paddingBottom));
    };

    // A card must read whole, so as the room shrinks the page gives way in a
    // fixed order: the step's intro line, then the hero's figures, and only
    // then does the hand keep the height its cards need and the body scroll.
    // Each step is tried by measuring, never by fixed room sizes. Only on a
    // phone showing cards; anywhere else nothing is forced. Returns whether
    // anything changed.
    P.makeRoom = function () {
      var app = this.app();
      if (!app) return false;
      var hand = this.$('[data-hand]'), box = this.$('[data-stage]');
      var key = function (s) { return [s.short, s.low, hand ? hand.style.minHeight : '', app.classList.contains('is-scroll'), !!box && box.classList.contains('is-tuck')].join(); };
      var was = key(this);
      // Measuring needs the tuck to apply at once, not over its transition.
      app.classList.add('is-fitting');
      if (this.roomShort != null) this.short = this.roomShort;
      this.low = false;
      app.classList.remove('is-scroll');
      if (hand) hand.style.minHeight = '';
      if (hand && !this.desk()) {
        var need = this.handNeed(hand);
        app.classList.toggle('is-short', this.short);
        if (box) box.classList.remove('is-tuck');
        if (need && hand.clientHeight < need) { this.short = true; app.classList.add('is-short'); }
        if (need && hand.clientHeight < need) { this.low = true; if (box) box.classList.add('is-tuck'); }
        if (need && hand.clientHeight < need) { hand.style.minHeight = need + 'px'; app.classList.add('is-scroll'); }
      } else {
        app.classList.toggle('is-short', this.short);
        this.tuckFor(this.scroller());
      }
      app.classList.remove('is-fitting');
      return key(this) !== was;
    };

    // --- The tuck ---

    // What scrolls on the phone's body: the list view and About your hero (and
    // Name and review, which has no stage to tuck).
    P.scroller = function () { return this.$('.hc-list, .hc-rev'); };

    // Past 48px the stage folds to its name and line; it opens only at the very
    // top. A page too short to keep scrolling after the fold stays open, or the
    // fold would hand the scroll back and the two would flicker.
    P.tuckFor = function (sc) {
      var box = this.$('[data-stage]');
      if (!box || this.desk()) return;
      var tucked = box.classList.contains('is-tuck');
      // While the cards need the room the stage stays folded.
      if (this.low) { box.classList.add('is-tuck'); return; }
      // Nothing scrolls (the cards): the stage stands open.
      if (!sc) { if (tucked) box.classList.remove('is-tuck'); return; }
      if (!tucked && sc.scrollTop > 48 && sc.scrollHeight - sc.clientHeight > 48 + box.offsetHeight * 0.6) box.classList.add('is-tuck');
      else if (tucked && sc.scrollTop <= 0) box.classList.remove('is-tuck');
    };

    // --- The hand: a mouse can move it ---

    P.setupHand = function (hand) {
      base.setupHand.call(this, hand);
      var self = this, md = null, dragged = false, wheelAt = 0;
      function cards() { return qa('.hc-card[data-i]', hand); }
      function nearest() {
        var mid = hand.scrollLeft + hand.clientWidth / 2, best = 0, bd = 1e9;
        cards().forEach(function (c, i) { var d = Math.abs(c.offsetLeft + c.offsetWidth / 2 - mid); if (d < bd) { bd = d; best = i; } });
        return best;
      }
      function move(ev) {
        if (!md) return;
        var dx = ev.clientX - md.x, now = performance.now();
        if (!dragged && Math.abs(dx) < 5) return;
        if (!dragged) { dragged = true; hand.style.scrollSnapType = 'none'; hand.classList.add('is-dragging'); }
        md.v = (ev.clientX - md.lx) / Math.max(1, now - md.t); md.lx = ev.clientX; md.t = now; md.dx = dx;
        hand.scrollLeft = md.s - dx;
        ev.preventDefault();
      }
      function up() {
        window.removeEventListener('pointermove', move);
        window.removeEventListener('pointerup', up);
        window.removeEventListener('pointercancel', up);
        if (!md) return;
        var m = md; md = null;
        // The click that follows a drag arrives at once; after that a plain
        // click on a card counts again.
        setTimeout(function () { dragged = false; }, 0);
        if (!dragged) return;
        hand.classList.remove('is-dragging');
        // Let go on the nearest card; a short drag of 50px or a flick still
        // moves one card on.
        var n = cards().length, i = nearest(), dx = m.dx || 0;
        if (i === m.i0 && Math.abs(dx) > 50) i += dx < 0 ? 1 : -1;
        else if (i === m.i0 && Math.abs(m.v) > 0.35) i += m.v < 0 ? 1 : -1;
        i = Math.max(0, Math.min(n - 1, i));
        self.scrollHandTo(i, true);
        setTimeout(function () { hand.style.scrollSnapType = ''; }, 420);
      }
      hand.addEventListener('pointerdown', function (ev) {
        if (ev.pointerType !== 'mouse' || ev.button !== 0) return;
        md = { x: ev.clientX, s: hand.scrollLeft, t: performance.now(), v: 0, lx: ev.clientX, i0: nearest() };
        dragged = false;
        window.addEventListener('pointermove', move);
        window.addEventListener('pointerup', up);
        window.addEventListener('pointercancel', up);
      });
      // A drag is not a click on the card it ends over.
      hand.addEventListener('click', function (ev) { if (dragged) { ev.stopPropagation(); ev.preventDefault(); } }, true);
      hand.addEventListener('dragstart', function (ev) { ev.preventDefault(); });
      // The wheel steps one card at a time.
      hand.addEventListener('wheel', function (ev) {
        if (Math.abs(ev.deltaY) <= Math.abs(ev.deltaX)) return;
        ev.preventDefault();
        var now = performance.now();
        if (now - wheelAt < 260) return;
        wheelAt = now;
        var n = cards().length, i = Math.max(0, Math.min(n - 1, nearest() + (ev.deltaY > 0 ? 1 : -1)));
        self.scrollHandTo(i, true);
      }, { passive: false });
    };

    // --- Events ---

    P.wire = function () {
      var self = this, root = this.root;
      base.wire.call(this);
      // Scroll does not bubble; the capture sees every scroller under the root.
      root.addEventListener('scroll', function (ev) { if (ev.target.matches && ev.target.matches('.hc-list, .hc-rev')) self.tuckFor(ev.target); }, true);
      // The creator's own handler has already taken the new value.
      root.addEventListener('input', function (ev) {
        var t = ev.target;
        if (!t.hasAttribute) return;
        if (t.hasAttribute('data-name')) {
          var nm = self.heroName(), ini = self.initial();
          self.$$('.hc-stg__nm').forEach(function (el) { el.textContent = nm; });
          self.$$('.hc-por').forEach(function (el) { el.textContent = ini; });
        } else if (t.hasAttribute('data-about')) {
          var ln = self.heroLine(false);
          self.$$('.hc-stg__ln').forEach(function (el) { el.textContent = ln; });
        }
      });
    };

    return Stage;
  };
})();
