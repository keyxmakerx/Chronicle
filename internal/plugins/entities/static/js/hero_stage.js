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

    function Stage(root) { Creator.call(this, root); this.inerted = []; }
    Stage.prototype = Object.create(Creator.prototype);
    Stage.prototype.constructor = Stage;
    var base = Creator.prototype, P = Stage.prototype;

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

    // The site's own bar and sidebar are behind the creator: out of reach of
    // the keyboard and screen readers while it is open. Only what this took is
    // given back, so a page that set its own inert is left as it was.
    P.lockSite = function (on) {
      var self = this;
      if (on) {
        ['sidebar', 'app-topbar'].forEach(function (id) {
          var el = document.getElementById(id);
          if (el && !el.hasAttribute('inert') && self.inerted.indexOf(el) < 0) { el.setAttribute('inert', ''); self.inerted.push(el); }
        });
        return;
      }
      this.inerted.forEach(function (el) { el.removeAttribute('inert'); });
      this.inerted = [];
    };

    P.render = function () {
      base.render.call(this);
      this.lockSite(true);
      // What is on screen now is the figures' starting point for the next count.
      this.lastStats = this.stats();
    };

    P.destroy = function () {
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
    P.stageHTML = function () { return '<div class="hc-stg" data-stage>' + this.stageInner() + '</div>'; };

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
      this.tuckFor(this.scroller());
      this.previewStage();
    };
    P.refreshList = function () { base.refreshList.call(this); this.tuckFor(this.scroller()); };
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
