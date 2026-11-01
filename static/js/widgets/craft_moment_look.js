/**
 * craft_moment_look.js -- a crafting moment as the Director dressed it in
 * How it looks, played once on a player's roll (window widget
 * craft_moment_look)
 *
 * The Crafting block mounts it over a roll's result with el._look =
 * {spots, words, makes, liquid}: the look the server sent for the moment that
 * happened. It plays on the same bench the rules editor previews on
 * (CraftRulesStage over ItemLookFX), so the player sees what the Director
 * saw. The block only mounts it when motion is on; the words are in the
 * result's own text either way. Its scripts load on sight, so a page pays
 * for the effects only when a dressed moment plays.
 */
(function () {
  'use strict';
  var STYLE = 'craft-moment-look-style';
  function injectStyle() {
    if (document.getElementById(STYLE)) return;
    var s = document.createElement('style');
    s.id = STYLE;
    s.textContent = '.cml{max-width:560px;border-radius:10px;overflow:hidden;background:#15110e;margin:0 0 10px}.cml canvas{display:block;width:100%;aspect-ratio:16/9}';
    document.head.appendChild(s);
  }
  Chronicle.register('craft_moment_look', {
    init: function (el) {
      var o = el._look, B = window.CraftRulesStage;
      if (!o || !B) { el.hidden = true; return; }
      injectStyle();
      el.classList.add('cml');
      el.innerHTML = '<canvas role="img"></canvas>';
      var cv = el.firstChild;
      cv.setAttribute('aria-label', o.words ? 'The bench as the Director set this moment: ' + o.words : 'The bench as the Director set this moment');
      var st = new B.Stage(cv);
      st.set({spots: o.spots || {}, words: o.words || '', makes: !!o.makes});
      if (o.liquid) st.liquid = o.liquid;
      st.play();
      el._stage = st;
    },
    destroy: function (el) {
      if (el._stage) el._stage.destroy();
      el._stage = null;
    }
  });
})();
