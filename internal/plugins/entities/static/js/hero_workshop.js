/**
 * hero_workshop.js — the hero creator, standing in the Customizer's Hero
 * creator page: the real creator in a frame, with the campaign's real cards,
 * where pressing a card picks the card to dress instead of choosing it for a
 * hero.
 *
 * window.ChronicleHeroWorkshop(Stage, kit) returns a subclass of the stage
 * look, in the way hero_stage.js subclasses the creator. hero_creator.js builds
 * it once, for a mount that carries data-workshop, from the same kit. The page
 * (hero_looks.js) hands the creator a hooks object on the mount element before
 * mounting it:
 *
 *   mount.heroWorkshop = {
 *     view      'desk' or 'phone': the view the frame shows, not the window's
 *     height    the frame's height in px, as the creator measures its room
 *     borders   the page's ChronicleHeroBorders instance, shared with the picker
 *     loaded(creator)  the plan is read and about to be drawn for the first time
 *     press(i)  a card was pressed: the card at index i of the step is to be dressed
 *     follow(i) the creator's focus moved to card i (the arrow keys)
 *     drawn()   the cards were drawn again
 *   }
 *
 * Nothing in the frame makes a hero, writes an entry, keeps a draft or leaves
 * the page: every place the creator does one of those is closed here, and the
 * creator for players is not touched.
 */
(function () {
  'use strict';

  var NO_HOOKS = {
    view: 'desk', height: 640, borders: null,
    loaded: function () {}, press: function () {}, follow: function () {}, drawn: function () {}
  };

  window.ChronicleHeroWorkshop = function (Stage, kit) {
    function Workshop(root) {
      Stage.call(this, root);
      // The page's own object is kept, not copied: it changes the view and the
      // height after the creator is made, and the creator reads them live.
      var h = root.heroWorkshop || {};
      Object.keys(NO_HOOKS).forEach(function (k) { if (!(k in h)) h[k] = NO_HOOKS[k]; });
      this.hooks = h;
      // The page owns the colouring script, so a card and a sample in the
      // panel share each coloured picture; the one the stage look made from
      // the mount's library is not used.
      if (this.borders) this.borders.destroy();
      this.borders = this.hooks.borders || null;
    }
    Workshop.prototype = Object.create(Stage.prototype);
    Workshop.prototype.constructor = Workshop;
    var P = Workshop.prototype;

    // --- The frame, not the window ---

    // The view is the one the page's switch (or a phone) asks for.
    P.measure = function () {
      var v = this.hooks.view === 'phone' ? 'phone' : 'desk', changed = v !== this.view;
      this.view = v;
      return changed;
    };

    P.room = function () { return { height: Number(this.hooks.height) || NO_HOOKS.height, min: 240 }; };

    // The workshop is part of a page the owner is working in: nothing around
    // it is made inert and no page shortcut stands down.
    P.lockSite = function () { /* the page stays usable */ };

    // --- Nothing is kept ---

    P.restore = function () { /* no hero draft is read */ };
    P.save = function () { this.kept = true; };
    P.takeHandoff = function () { return ''; };
    P.statusHTML = function () { return ''; };

    // The page owns the borders; the stage look would destroy them.
    P.destroy = function () {
      this.borders = null;
      Stage.prototype.destroy.call(this);
    };

    // --- Nothing is made, written or left ---

    // The bar names the creator and its game system; there is no way out of
    // the frame and nothing it could say was saved.
    P.barHTML = function () {
      var esc = kit.esc, desk = this.desk();
      return '<header class="hc-abar">' + (desk
        ? '<div class="hc-abar__t"><h1>' + esc(this.title()) + '</h1><small>' + esc(this.plan.systemName || 'Game system') + '</small></div>'
        : '<h1 class="hc-sr">' + esc(this.title()) + '</h1>') + '</header>';
    };

    // No hero is made from here, and no entry is written.
    P.create = function () { /* nothing is made */ };
    P.persist = function () { return Promise.reject(new Error('the workshop makes no hero')); };
    P.keepHere = function () { /* nothing is kept */ };
    P.keepDraftsTo = function () { return Promise.resolve(); };
    P.openOwn = function () { /* no entry is written here */ };
    P.ownEnabled = function () { return false; };

    // After the plan is read and before the first draw the page records each
    // card's look as served; no step offers "make your own".
    P.ownFirst = function () {
      Stage.prototype.ownFirst.call(this);
      this.plan.steps.forEach(function (st) { st.canMake = false; });
      this.hooks.loaded(this);
    };

    // --- A press picks the card to dress ---

    P.choose = function (i) { this.hooks.press(i == null ? this.curIdx() : i); };
    P.openSheet = function (i, own) { if (!own) this.hooks.press(i); };

    P.setFocus = function (i, scroll) {
      Stage.prototype.setFocus.call(this, i, scroll);
      this.hooks.follow(i);
    };
    P.after = function () { Stage.prototype.after.call(this); this.hooks.drawn(); };
    P.refreshList = function () { Stage.prototype.refreshList.call(this); this.hooks.drawn(); };

    // On a computer a click moves the creator's focus and, after it, picks
    // the card; a phone's tap goes through openSheet.
    P.wire = function () {
      var self = this;
      Stage.prototype.wire.call(this);
      this.root.addEventListener('click', function (ev) {
        var gc = ev.target.closest && ev.target.closest('.hc-gcard[data-i]:not(.hc-gcard--nh), .hc-mid .hc-lrow[data-i]');
        if (gc && self.plan && self.desk() && self.root.contains(gc)) self.hooks.press(+gc.getAttribute('data-i'));
      });
    };

    return Workshop;
  };
})();
