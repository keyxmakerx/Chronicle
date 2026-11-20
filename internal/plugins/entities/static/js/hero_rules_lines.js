/**
 * hero_rules_lines.js — the quiet lines that say what a Director's hero-choice
 * rules did to a pick list, shared by the hero builder and the new-hero
 * creator so both surfaces word them the same way.
 *
 * Input is a step's `rules` object from the creation plan (limited, required,
 * prefilled, notHereCount, notHere, waiting, suggested). Output is escaped
 * markup using the caller's class prefix ("hb" or "hc"): `<p class="P-note">`
 * lines and the `P-nh` "n not here" foot, which folds open names and reasons.
 *
 * Loaded before the scripts that use it; exposes window.ChronicleHeroRules.
 */
(function () {
  'use strict';

  function esc(v) { return window.Chronicle && Chronicle.escapeHtml ? Chronicle.escapeHtml(v) : String(v == null ? '' : v).replace(/[&<>"']/g, function (c) { return '&#' + c.charCodeAt(0) + ';'; }); }
  // The campaign's own word for its game master, already HTML-safe.
  function gm() { return window.Chronicle && Chronicle.gmTerm ? Chronicle.gmTerm() : 'GM'; }
  function lower(s) { return String(s || '').toLowerCase(); }

  var LOCK = 'everyone starts with this';

  function waitingText(name) { return esc(name) + ' is waiting for your ' + gm() + '. Pick another to replace it.'; }

  function suggestedText(label, g) {
    return 'Your ' + gm() + ' asked for another ' + esc(lower(label)) + ' instead of ' + esc(g.entryName) + (g.message ? ': “' + esc(g.message) + '”' : '.');
  }

  // The notes above a list: a waiting pick, or a Director's suggestion when
  // nothing waits, and the short-list line on a required step with a choice.
  function notes(st, waitName, p) {
    var r = st.rules || {}, out = '';
    if (waitName) out += '<p class="' + p + '-note">' + waitingText(waitName) + '</p>';
    else if (r.suggested) out += '<p class="' + p + '-note">' + suggestedText(st.label, r.suggested) + '</p>';
    if (r.required && !r.prefilled) out += '<p class="' + p + '-note">Your ' + gm() + ' has set a short list for this.</p>';
    return out;
  }

  // "1 not here" at the list's foot, folding open the names and the reasons.
  function notHere(r, open, p) {
    if (!r || !r.notHereCount) return '';
    var names = r.notHere || [];
    var body = open ? '<ul class="' + p + '-nh-list">' + names.map(function (n) {
      return '<li><b>' + esc(n.name) + '</b> isn’t here.' + (n.reason ? ' Your ' + gm() + ' says: “' + esc(n.reason) + '”' : '') + '</li>';
    }).join('') + (r.notHereCount > names.length ? '<li>and ' + (r.notHereCount - names.length) + ' more</li>' : '') + '</ul>' : '';
    return '<div class="' + p + '-nh"><button type="button" class="' + p + '-link" data-act="nothere" aria-expanded="' + !!open + '">' +
      r.notHereCount + ' not here</button>' + body + '</div>';
  }

  window.ChronicleHeroRules = { LOCK: LOCK, waitingText: waitingText, suggestedText: suggestedText, notes: notes, notHere: notHere };
})();
