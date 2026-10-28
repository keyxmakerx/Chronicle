/**
 * crafting_projects.js -- the Director's Projects tab (window.CraftingProjects)
 *
 * Every hero's project in progress, the respite button that gives each one a
 * roll, and the projects finished lately, as the signed Crafting projects
 * mockup. A row's actions open as a strip under it, because the table
 * scrolls sideways on a phone and would clip a popover. Every change goes
 * to the server with the project's version, so two Directors never
 * silently overwrite each other; the tab reloads what the server answers.
 */
window.CraftingProjects = (function () {
  'use strict';
  var L = window.CraftingLib, esc = L.esc;
  var MORE = '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><circle cx="3" cy="8" r="1.5" fill="currentColor"/><circle cx="8" cy="8" r="1.5" fill="currentColor"/><circle cx="13" cy="8" r="1.5" fill="currentColor"/></svg>';

  function plural(n, one, many) { return n + ' ' + (n === 1 ? one : many); }
  /* How long ago, in the words a person uses. */
  function ago(iso, now) {
    var t = Date.parse(iso);
    if (!t) return '';
    var m = Math.max(0, Math.round(((now || Date.now()) - t) / 60000));
    if (m < 2) return 'just now';
    if (m < 60) return m + ' minutes ago';
    var h = Math.round(m / 60);
    if (h < 24) return plural(h, 'hour', 'hours') + ' ago';
    return plural(Math.round(h / 24), 'day', 'days') + ' ago';
  }
  /* The line beside the respite button. */
  function respiteLine(page, logged) {
    if (logged) return 'Logged ' + plural(logged, 'time', 'times') + ' just now' + (page.today ? ', on ' + page.today : '') + '. Every project in progress got one roll each time.';
    var r = page.lastRespite, when = r ? (r.dayLabel ? 'on ' + r.dayLabel : ago(r.at)) : '';
    return (r ? 'The last one was ' + when + '. ' : 'No respite logged yet. ') +
      'Each project in progress gets one roll. The button in the DM Screen’s Homebrew drawer does the same; one press is one respite.';
  }

  function mount(el, app) {
    var page = null, open = null, logged = 0, busy = false, dead = false;
    function isOpen(kind, id) { return !!(open && open.kind === kind && open.id === id); }
    function find(id) { for (var i = 0; i < page.active.length; i++) if (page.active[i].id === id) return page.active[i]; return null; }
    function focus(sel) { setTimeout(function () { var f = el.querySelector(sel); if (f) f.focus(); }, 0); }

    function row(p) {
      var id = esc(p.id), cell;
      if (isOpen('points', p.id)) {
        cell = '<div class="crp-pts"><label class="sr-only" for="crp-pts-' + id + '">Project points</label><input type="number" class="cra-in cra-sm" id="crp-pts-' + id + '" min="0" max="' + (p.goal - 1) + '" value="' + p.points + '">' +
          '<span class="cra-fine">of ' + p.goal + '</span><button type="button" class="btn btn-primary btn-sm" data-act="savepts" data-id="' + id + '">Set</button>' +
          '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + id + '">Cancel</button></div>';
      } else cell = L.progress(p.points, p.goal, '', p.name);
      var ready = p.rollsReady + (p.rollsReady && p.waits ? ' <span class="crp-pill is-warn" title="' + esc(p.waits) + '">can’t be worked today</span>' : '');
      var h = '<tr><td class="is-who">' + esc(p.hero) + '</td><td>' + esc(p.name) + '<div class="cra-fine">' + esc(p.kind || '') + '</div></td><td>' + cell + '</td><td>' + ready + '</td>' +
        '<td class="is-r"><button type="button" class="crp-ib" data-act="menu" data-id="' + id + '" aria-expanded="' + isOpen('menu', p.id) + '" aria-label="More for ' + esc(p.hero) + '’s ' + esc(p.name) + '">' + MORE + '</button></td></tr>';
      if (isOpen('menu', p.id)) h += '<tr><td colspan="5" class="is-strip"><div class="crp-strip">' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="give" data-id="' + id + '">Give a roll</button>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="editpts" data-id="' + id + '">Set the points</button>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="askcancel" data-id="' + id + '">Cancel the project</button></div></td></tr>';
      if (isOpen('confirm', p.id)) h += '<tr><td colspan="5" class="is-strip"><div class="crp-confirm"><p>Cancel ' + esc(p.hero) + '’s ' + esc(p.name) + '? What it took stays spent' +
        (p.took ? ' (' + esc(p.took) + ')' : '') + '. Give anything back with <b>Give an item</b> on their page.</p>' +
        '<button type="button" class="btn btn-secondary btn-sm" data-act="close" data-id="' + id + '">Keep it</button><button type="button" class="btn btn-danger btn-sm" data-act="cancel" data-id="' + id + '">Cancel the project</button></div></td></tr>';
      return h;
    }

    function render() {
      if (!page) return;
      var h = '<div class="crp-tab"><div class="crp-resp"><div><b>Log a respite</b><span class="cra-fine">' + esc(respiteLine(page, logged)) + '</span></div>' +
        '<button type="button" class="btn btn-primary btn-sm" data-act="respite"' + (busy ? ' disabled' : '') + '>Log a respite</button></div>' +
        '<div class="crp-twrap"><table class="crp-table" aria-label="Projects in progress"><thead><tr><th scope="col">Hero</th><th scope="col">Project</th><th scope="col">Progress</th><th scope="col">Rolls ready</th><th scope="col"><span class="sr-only">Actions</span></th></tr></thead><tbody>' +
        (page.active.length ? page.active.map(row).join('') : '<tr><td colspan="5" class="cra-fine">No projects in progress. Players start them from the Crafting block on their character’s page.</td></tr>') +
        '</tbody></table></div><section><h3 class="cra-ph">Finished lately</h3><ul class="crp-done">' +
        (page.finished.length ? page.finished.map(function (p) {
          return '<li><span>' + esc(p.hero) + ' made ' + esc(p.makes) + '</span>' +
            (p.undelivered ? '<span class="crp-pill is-warn">Didn’t reach the pack. Give it with Give an item.</span>' : '') +
            '<span class="cra-fine">' + esc(ago(p.finishedAt)) + '</span></li>';
        }).join('') : '<li class="cra-fine">Nothing finished yet.</li>') + '</ul></section></div>';
      el.innerHTML = h;
    }

    function load() {
      return app.api('GET', '/projects').then(function (got) {
        if (dead) return;
        page = got; render();
      }, function (err) {
        if (dead) return;
        el.innerHTML = '<div class="cra-empty cra-bad" role="alert">Projects couldn’t load: ' + esc(err.message) + '. <button type="button" class="cra-link" data-act="retry">Try again</button></div>';
      });
    }
    /* After a change, reload and say what happened; a refusal says why. */
    function then(p, msg) {
      busy = true; render();
      return p.then(function (got) {
        var said = typeof msg === 'function' ? msg(got) : msg;
        busy = false; open = null;
        return load().then(function () { app.toast(said); });
      }, function (err) {
        busy = false;
        app.toast(err.message.charAt(0).toUpperCase() + err.message.slice(1));
        open = null;
        return load();
      });
    }

    el.addEventListener('click', function (e) {
      var b = e.target.closest('[data-act]');
      if (!b || !el.contains(b)) return;
      var act = b.getAttribute('data-act'), id = b.getAttribute('data-id'), p = id ? find(id) : null, path = id ? '/projects/' + encodeURIComponent(id) : '';
      if (act === 'retry') { load(); return; }
      if (busy) return;
      switch (act) {
        case 'respite':
          then(app.api('POST', '/respites'), function (got) { logged++; return 'Respite logged. ' + plural(got.projects, 'project', 'projects') + ' got a roll.'; });
          break;
        case 'menu': open = isOpen('menu', id) ? null : {kind: 'menu', id: id}; render(); focus('.crp-strip button'); break;
        case 'close': open = null; render(); focus('[data-act="menu"][data-id="' + id + '"]'); break;
        case 'give': if (p) then(app.api('POST', path + '/rolls'), p.hero + ' has a roll for ' + p.name + '.'); break;
        case 'editpts': open = {kind: 'points', id: id}; render(); focus('#crp-pts-' + id); break;
        case 'savepts':
          if (!p) break;
          var v = Number(el.querySelector('#crp-pts-' + id).value);
          if (!isFinite(v) || v < 0 || v % 1) { app.toast('Points are a whole number, 0 or more.'); break; }
          if (v >= p.goal) { app.toast('Set it below the goal of ' + p.goal + '; the item comes from a roll.'); break; }
          then(app.api('PUT', path + '/points', {points: v, version: p.version}), p.name + ' is at ' + v + ' of ' + p.goal + '.');
          break;
        case 'askcancel': open = {kind: 'confirm', id: id}; render(); focus('[data-act="cancel"]'); break;
        case 'cancel': if (p) then(app.api('DELETE', path), p.name + ' was cancelled. What it took stays spent.'); break;
      }
    });
    el.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && open) { var id = open.id; open = null; render(); focus('[data-act="menu"][data-id="' + id + '"]'); }
      if (e.key === 'Enter' && e.target.matches('.crp-pts input')) { var s = el.querySelector('[data-act="savepts"]'); if (s) s.click(); }
    });

    el.innerHTML = '<div class="cra-loading" aria-hidden="true"><i></i><i></i><i></i></div>';
    return {
      // Fresh each time the tab opens: players start and roll in between.
      shown: function () { load(); },
      destroy: function () { dead = true; el.innerHTML = ''; }
    };
  }
  return {mount: mount, respiteLine: respiteLine, ago: ago};
})();
