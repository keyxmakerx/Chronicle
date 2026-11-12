/**
 * design_library_suggest.js — "Suggest for <system>" inside the saved-designs
 * library (design_library.js): choose one of the Owner's designs, an optional
 * note for the package maintainer, whether to be credited, and download one
 * JSON file the maintainer can paste into the package. Nothing is sent
 * anywhere; the server builds the file and the page only saves it.
 *
 * Typed text (the note) is put back with .value after a redraw, never through
 * markup, and ids read from the page are only compared against the loaded list.
 *
 * The pure helpers are on DesignSuggest.util so test/js can run them without a page.
 */
(function () {
  'use strict';

  var MAX_NOTE = 240;

  /* ---- Pure helpers ---- */
  // What the file will hold, as the fold lists it, so the Owner knows before exporting.
  function contents(opts) {
    var out = ['The design’s own choices: look, parts and materials', 'Its name, its saved note and the game system (' + opts.system + ')'];
    out.push(opts.credit ? 'Your note, and your name (' + (opts.you || 'your display name') + ')' : 'Your note. Your name is not included');
    out.push('No goods, prices or pictures');
    return out;
  }
  function exportedNote(filename, systemLabel) {
    return 'Exported ' + filename + '. Send it to the ' + systemLabel + ' package maintainer. If they add it, it shows under From ' + systemLabel +
      ' on every ' + systemLabel + ' campaign after their next release. It stays in My designs either way.';
  }
  function byId(list, id) { for (var i = 0; i < list.length; i++) if (list[i].id === id) return list[i]; return null; }
  // The body to send: the note trimmed and held to the cap, the credit as a boolean.
  function requestBody(note, credit) { return { note: String(note || '').trim().slice(0, MAX_NOTE), credit: !!credit }; }
  function fileText(suggestion) { return JSON.stringify(suggestion, null, 2) + '\n'; }

  /* ---- State ---- */
  var env = null, box = null;
  var Z = { id: '', note: '', credit: true, exported: '', err: '', busy: false };

  function esc(t) { return env.esc(t); }

  function html() {
    var mine = env.mine(), label = env.systemLabel(), sys = env.system();
    var h = '<div class="dl-card" role="group" aria-label="Suggest a design for ' + esc(label) + '"><h4>Suggest a design for ' + esc(label) + ' <button type="button" class="dl-link" data-dsg="close">Close</button></h4>';
    h += '<p class="dl-ok" role="status" aria-live="polite" data-dsg-done></p>';
    if (!mine.length) return h + '<p class="dl-ok">Save a design of your own first, then suggest it here.</p></div>';
    h += '<label class="dl-lab" for="dsg-design">Design to suggest</label><select id="dsg-design" class="dl-in">' +
      mine.map(function (m) { return '<option value="' + esc(m.id) + '">' + esc(m.name) + ' (' + esc(env.kindLabel(m.kind)) + ')</option>'; }).join('') + '</select>';
    h += '<label class="dl-lab" for="dsg-note">Note for the maintainer <span class="dl-sub">optional</span></label><textarea id="dsg-note" rows="2" cols="1" maxlength="' + MAX_NOTE + '"></textarea>';
    h += '<label class="dsg-credit"><input type="checkbox" id="dsg-credit"> Credit me as <b data-dsg-you></b></label>';
    h += '<ul class="dsg-list" aria-label="What is in the file">' + contents({ system: sys, credit: Z.credit, you: env.you() }).map(function (l) { return '<li>' + esc(l) + '</li>'; }).join('') + '</ul>';
    h += '<div class="dl-row"><button type="button" class="dl-btn pri" data-dsg="export"' + (Z.busy ? ' disabled' : '') + '>Export suggestion file</button></div>';
    return h + '<p class="dl-err" role="alert" data-dsg-err></p></div>';
  }
  var CSS = [
    '.dsg-list{margin:0;padding-left:18px;font-size:.76rem;color:var(--color-text-secondary,#6b7280);display:grid;gap:2px}',
    '.dsg-credit{display:flex;gap:8px;align-items:center;min-height:36px;font-size:.82rem;color:var(--color-text-body,#374151)}.dsg-credit b{color:var(--color-text-primary,#111827)}.dsg-credit input{width:18px;height:18px}',
    '.dsg-done{border-left:3px solid #2f855a;padding:6px 10px;border-radius:0 6px 6px 0;background:color-mix(in srgb,#2f855a 10%,var(--color-card-bg,#fff))}',
    '#dsg-note{min-height:3.6rem;font:inherit;font-size:.82rem}',
    '.dl-card h4{color:var(--color-text-primary,#111827)}'
  ].join('\n');
  function style() {
    if (document.getElementById('dsg-style')) return;
    var st = document.createElement('style'); st.id = 'dsg-style'; st.textContent = CSS; document.head.appendChild(st);
  }

  function read() {
    if (!box) return;
    var d = box.querySelector('#dsg-design'), n = box.querySelector('#dsg-note'), c = box.querySelector('#dsg-credit');
    if (d) Z.id = d.value;
    if (n) Z.note = n.value;
    if (c) Z.credit = c.checked;
  }
  function paint() {
    if (!box) return;
    if (!env.open()) { box.innerHTML = ''; return; }
    var mine = env.mine();
    if (!byId(mine, Z.id)) Z.id = (byId(mine, env.selected()) || mine[0] || {}).id || '';
    box.innerHTML = html();
    // Everything typed or chosen goes back as values and properties, never markup.
    var d = box.querySelector('#dsg-design'), n = box.querySelector('#dsg-note'), c = box.querySelector('#dsg-credit'), you = box.querySelector('[data-dsg-you]');
    if (d) d.value = Z.id;
    if (n) n.value = Z.note;
    if (c) c.checked = Z.credit;
    if (you) you.textContent = env.you() || 'your display name';
    paintMsgs();
  }
  function paintMsgs() {
    if (!box) return;
    var e = box.querySelector('[data-dsg-err]'), done = box.querySelector('[data-dsg-done]');
    if (e) e.textContent = Z.err;
    if (done) { done.textContent = Z.exported ? exportedNote(Z.exported, env.systemLabel()) : ''; done.className = Z.exported ? 'dl-ok dsg-done' : 'dl-ok'; }
  }

  /* ---- Acting ---- */
  function download(name, text) {
    try {
      var a = document.createElement('a');
      a.href = URL.createObjectURL(new Blob([text], { type: 'application/json' }));
      a.download = name;
      document.body.appendChild(a); a.click();
      setTimeout(function () { URL.revokeObjectURL(a.href); a.remove(); }, 500);
      return true;
    } catch (x) { return false; }
  }
  function exportFile() {
    read();
    var d = byId(env.mine(), Z.id); if (!d) { Z.err = 'Choose a design to suggest.'; paintMsgs(); return; }
    Z.busy = true; Z.err = ''; paint();
    env.call('POST', '/designs/' + encodeURIComponent(d.id) + '/suggestion', requestBody(Z.note, Z.credit)).then(function (r) {
      Z.busy = false;
      if (!download(r.filename, fileText(r.suggestion))) { Z.err = 'The file could not be saved from this browser.'; paint(); return; }
      Z.exported = r.filename; paint();
      var again = box.querySelector('[data-dsg="export"]'); if (again) again.focus();
    }).catch(function (e) { Z.busy = false; Z.err = (e && e.message) || 'Something went wrong. Try again.'; paint(); });
  }

  function onClick(e) {
    var b = e.target.closest && e.target.closest('button'); if (!b || !box.contains(b)) return;
    var act = b.getAttribute('data-dsg');
    if (act === 'close') env.close();
    else if (act === 'export') exportFile();
  }
  function onChange(e) {
    // Changing the design or the credit tick reads the form and redraws the list of what is in the file.
    if (e.target.id === 'dsg-design' || e.target.id === 'dsg-credit') { read(); Z.exported = ''; paint(); }
  }
  function bind(host) {
    if (host && !host._dsg) { host._dsg = 1; host.addEventListener('click', onClick); host.addEventListener('change', onChange); }
  }

  /* ---- Mounting ---- */
  // The host may not exist yet: the Suggest button needs the game system, which loads after the first draw.
  function attach(host, e) { style(); env = e; box = host || null; if (host) { bind(host); paint(); } }
  function redraw(host) { if (!host || !env) return; read(); box = host; bind(host); paint(); }
  function focus() { var d = box && box.querySelector('#dsg-design'); if (d) d.focus(); }

  window.DesignSuggest = {
    attach: attach, redraw: redraw, focus: focus,
    util: { contents: contents, exportedNote: exportedNote, requestBody: requestBody, fileText: fileText, MAX_NOTE: MAX_NOTE }
  };
})();
