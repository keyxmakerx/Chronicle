/**
 * Family tree
 *
 * The entity page's "Family tree" block (internal/plugins/entities/
 * family_tree_block.templ) is one quiet line of circles at rest: the page's
 * parents, the page, its partners and its children. A click on the word Family
 * or on a circle turns that line into the tree over the page, growing out of
 * the line's own rect, radius and fill (a container transform) and folding back
 * into it; a click on a circle in the tree docks that person's card, which
 * grows out of the circle the same way.
 *
 *   - Resting on a circle (or keyboard focus) for 250 ms Peeks a small card
 *     that sits where it hides no other circle and none of the lit lines.
 *   - The sheet has a visible Close and Escape closes it. A click outside does
 *     not (a panel closes only on purpose).
 *   - "Make centre" re-roots the tree on a relative whose own relations were
 *     read; a relative at the edge of what was read offers only "Open page".
 *
 * The server decides what a viewer may see: the JSON carries no relative a
 * player cannot open and no Director-only relation. The only permission this
 * file applies is the Director's own "view as player" preview, which hides
 * what is marked dm (html[data-view-as-player]); the server never sends that
 * to real players. Text reaches the DOM only through textContent. Motion
 * follows html[data-motion] (calm fades, off shows the end state) and a device
 * asking for reduced motion is always off.
 *
 * The Director's Edit tab (only when the server sent data-dm="1", and never in
 * the "view as player" preview) turns the empty places into "Add a parent /
 * child / partner" circles and puts a remove mark on each linked relative.
 * Edits change a copy of the model; the resting strip keeps drawing the saved
 * one. The save bar counts the difference between the two and Save sends only
 * that difference (pages to create, links to add or remove) in one request;
 * the server writes both directions of every pair and answers with the family
 * as it now stands, so the page shows what really saved. A failed save keeps
 * the edits on screen and says which links did not save.
 *
 * The buttons on the shell are inline IIFEs (the block can arrive by an
 * HTMX swap) that call window.ChronicleFamilyTree; everything the script draws
 * is wired when it is created, and the one document listener (Escape) lives
 * only while the sheet is open.
 */
(function () {
  'use strict';

  var DUR = 300;
  var EASE = 'cubic-bezier(.2,.8,.2,1)';
  var REST_MS = 250;
  var SVGNS = 'http://www.w3.org/2000/svg';
  var STEP = 84;
  var STEP_PHONE = 74;
  var PITCH = 104;
  var PARTNER_GAP = 8;
  var TOP = 56;
  var PHONE_W = 560;

  // ---- Pure helpers (unit-tested in test/js/family_tree.test.mjs) ----

  /** motionOf: how much motion the page allows ('full' | 'calm' | 'off'). */
  function motionOf(attr, reduced) {
    if (reduced) return 'off';
    return attr === 'calm' || attr === 'off' ? attr : 'full';
  }

  /** hashOf: a stable number from an id, so a person keeps one hue. */
  function hashOf(id) {
    var h = 7;
    for (var i = 0; i < id.length; i++) h = (h * 33 + id.charCodeAt(i)) >>> 0;
    return h;
  }

  /** hueOf: the ring and fill hue of a person's circle. */
  function hueOf(id) { return hashOf(String(id)) % 360; }

  /** initialsOf: up to two capitals from a name's first and last words. */
  function initialsOf(name) {
    var w = String(name || '').trim().split(/\s+/).filter(Boolean);
    if (!w.length) return '?';
    var a = Array.from(w[0])[0] || '';
    var b = w.length > 1 ? (Array.from(w[w.length - 1])[0] || '') : '';
    return (a + b).toUpperCase();
  }

  /** safeUrl: a picture or page address the block will use: ours, or http(s). */
  function safeUrl(u) {
    return typeof u === 'string' && /^(\/(?!\/)|https?:\/\/)/.test(u) ? u : '';
  }

  function cap(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

  /** greats: "Great-" repeated for generation gen (3 is great-, 4 great-great-). */
  function greats(gen) { return gen > 2 ? 'Great-' + new Array(gen - 2).join('great-') : ''; }

  /**
   * relLabel: what a relative is to the centre, in words that need no gender
   * (the pages hold none). dir is 'up', 'down', 'partner' or 'sib'; kind is the
   * link's kind (birth, adopted, guardian).
   */
  function relLabel(dir, gen, kind) {
    if (dir === 'partner') return 'Partner';
    if (dir === 'sib') return 'Sibling';
    if (dir === 'up') {
      if (kind === 'guardian') return 'Guardian';
      var up = gen > 1 ? greats(gen) + 'grandparent' : 'parent';
      return kind === 'adopted' ? 'Adoptive ' + up.toLowerCase() : cap(up);
    }
    if (dir === 'down') {
      if (kind === 'guardian') return 'Ward';
      var dn = gen > 1 ? greats(gen) + 'grandchild' : 'child';
      return kind === 'adopted' ? 'Adopted ' + dn.toLowerCase() : cap(dn);
    }
    return 'Relative';
  }

  /** rowLabel: the label at the left of a generation's row. */
  function rowLabel(dir, gen) {
    if (dir === 'up') return gen > 1 ? greats(gen) + 'grandparents' : 'Parents';
    return gen > 1 ? greats(gen) + 'grandchildren' : 'Children';
  }

  /** The model: people by id, in the order the server sent them. */
  function buildModel(data) {
    var P = {};
    var order = [];
    ((data && data.people) || []).forEach(function (p) {
      P[p.id] = {
        id: p.id, name: String(p.name || ''), sub: String(p.sub || ''), img: safeUrl(p.img), url: safeUrl(p.url),
        hue: hueOf(p.id), hidden: !!p.dm, read: !!p.read,
        par: (p.parents || []).map(function (e) { return { id: e.id, t: e.kind || 'birth', dm: !!e.dm }; }),
        partners: (p.partners || []).slice()
      };
      order.push(p.id);
    });
    return { P: P, order: order, page: data && data.page };
  }

  function visible(m, dm, id) { var p = m.P[id]; return !!p && (!p.hidden || dm); }
  function edgeVisible(m, dm, e) { return (!e.dm || dm) && visible(m, dm, e.id); }
  function parentsOf(m, dm, id) { return m.P[id].par.filter(function (e) { return edgeVisible(m, dm, e); }); }
  function kidsOf(m, dm, id) {
    return m.order.filter(function (x) {
      return visible(m, dm, x) && m.P[x].par.some(function (e) { return e.id === id && edgeVisible(m, dm, e); });
    });
  }
  function partnersOf(m, dm, id) { return m.P[id].partners.filter(function (x) { return visible(m, dm, x); }); }
  function sibsOf(m, dm, id) {
    var mine = m.P[id].par.filter(function (e) { return (!e.dm || dm) && e.t !== 'guardian'; }).map(function (e) { return e.id; });
    return m.order.filter(function (x) {
      return x !== id && visible(m, dm, x) && m.P[x].par.some(function (e) {
        return e.t !== 'guardian' && (!e.dm || dm) && mine.indexOf(e.id) > -1;
      });
    });
  }

  /** How many relatives the viewer can reach from the page through what they see. */
  function reachable(m, dm, page) {
    var seen = {}; seen[page] = 1;
    var q = [page];
    while (q.length) {
      var u = q.shift();
      parentsOf(m, dm, u).map(function (e) { return e.id; })
        .concat(kidsOf(m, dm, u), partnersOf(m, dm, u))
        .forEach(function (id) { if (!seen[id]) { seen[id] = 1; q.push(id); } });
    }
    return Object.keys(seen).length - 1;
  }

  /**
   * layoutTree: where every circle and line goes. The centre sits in the middle
   * row; ancestors are rows above it and descendants rows below, two each way
   * to begin with (xu / xd open further ones). A row wider than the room joins
   * its overflow into a "more" bubble until it is opened.
   */
  function layoutTree(m, o) {
    var dm = o.dm, c = o.centre, phone = !!o.phone, edit = !!o.edit;
    var step = phone ? STEP_PHONE : STEP;
    var cap_ = Math.max(3, Math.floor((o.width - 56) / step));
    var maxU = 2 + (o.xu || 0), maxD = 2 + (o.xd || 0);
    var seen = {}; seen[c] = 1;
    var upL = [], cur = [{ id: c }];
    for (;;) {
      var nx = [];
      cur.forEach(function (n) {
        parentsOf(m, dm, n.id).forEach(function (e) { if (!seen[e.id]) { seen[e.id] = 1; nx.push({ id: e.id, t: e.t, via: n.id }); } });
      });
      if (!nx.length) break;
      upL.push(nx); cur = nx;
    }
    var further = 0;
    upL.slice(maxU).forEach(function (l) { further += l.length; });
    upL = upL.slice(0, maxU);
    var dnL = [], dcur = [{ id: c }], dseen = {}; dseen[c] = 1;
    for (;;) {
      var nd = [];
      dcur.forEach(function (n) {
        kidsOf(m, dm, n.id).forEach(function (k) {
          if (!dseen[k]) {
            dseen[k] = 1;
            var e = m.P[k].par.filter(function (x) { return x.id === n.id; })[0];
            nd.push({ id: k, t: e.t, via: n.id });
          }
        });
      });
      if (!nd.length) break;
      dnL.push(nd); dcur = nd;
    }
    var after = 0;
    dnL.slice(maxD).forEach(function (l) { after += l.length; });
    dnL = dnL.slice(0, maxD);

    var rows = [];
    function mkrow(key, label, items, dir, gen) {
      var shown = items, bub = 0;
      if (!(o.openRows || {})[key] && items.length > cap_) { shown = items.slice(0, cap_ - 1); bub = items.length - (cap_ - 1); }
      var ents = shown.map(function (n) { return { kind: 'dot', id: n.id, rel: relLabel(dir, gen, n.t), t: n.t, via: n.via, dir: dir }; });
      if (bub) ents.push({ kind: 'bub', key: 'b:' + key, n: bub, label: 'more', act: key });
      return { key: key, label: label, ents: ents, dir: dir };
    }
    upL.slice().reverse().forEach(function (l, i) { var g = upL.length - i; rows.push(mkrow('u' + g, rowLabel('up', g), l, 'up', g)); });
    if (further) rows[0].ents.push({ kind: 'bub', key: 'b:fu', n: further, label: 'further back', act: 'fu' });
    if (edit && !upL.length) rows.push({ key: 'u1', label: rowLabel('up', 1), ents: [], dir: 'up' });
    var mid = [], sibs = o.sib ? sibsOf(m, dm, c) : [];
    sibs.forEach(function (id) { mid.push({ kind: 'dot', id: id, rel: relLabel('sib'), sib: 1 }); });
    mid.push({ kind: 'dot', id: c, rel: 'Centre', me: 1 });
    partnersOf(m, dm, c).forEach(function (id) { mid.push({ kind: 'dot', id: id, rel: 'Partner', via: c, dir: 'partner' }); });
    var midRow = { key: 'm', label: m.P[c].name, ents: mid, dir: 'mid', nsib: sibs.length };
    rows.push(midRow);
    dnL.forEach(function (l, i) { rows.push(mkrow('d' + (i + 1), rowLabel('down', i + 1), l, 'down', i + 1)); });
    if (after) rows[rows.length - 1].ents.push({ kind: 'bub', key: 'b:fd', n: after, label: 'further down', act: 'fd' });
    if (edit && !dnL.length) rows.push({ key: 'd1', label: rowLabel('down', 1), ents: [], dir: 'down' });
    if (edit) {
      // The empty places of the Edit tab: always offered, because a person can
      // have more than two parent links (born to, adoptive, guardian) and any
      // number of partners.
      var upRow = rows.filter(function (r) { return r.key === 'u1'; })[0];
      var dnRow = rows.filter(function (r) { return r.key === 'd1'; })[0];
      if (upRow) upRow.ents.push({ kind: 'slot', key: 's:parent', slot: 'parent', label: 'Add a parent' });
      if (dnRow) dnRow.ents.push({ kind: 'slot', key: 's:child', slot: 'child', label: 'Add a child' });
      midRow.ents.push({ kind: 'slot', key: 's:partner', slot: 'partner', label: 'Add a partner' });
    }

    var maxN = 0;
    rows.forEach(function (r) { maxN = Math.max(maxN, r.ents.length); });
    var w = Math.max(o.width, maxN * step + 64), cx = w / 2, dots = {};
    rows.forEach(function (r, ri) {
      var y = TOP + ri * PITCH, n = r.ents.length;
      r.y = y;
      r.ents.forEach(function (e, j) {
        var x = cx + (j - (n - 1) / 2) * step;
        if (r.key === 'm') { var ci = r.nsib; x = cx + (j - ci) * step + (j > ci ? PARTNER_GAP : 0) - (j < ci ? PARTNER_GAP : 0); }
        e.x = x; e.y = y; e.row = r.key;
      });
    });
    var links = [], slots = {};
    rows.forEach(function (r) { r.ents.forEach(function (e) { if (e.kind === 'dot' && !dots[e.id]) dots[e.id] = e; if (e.kind === 'slot') slots[e.key] = e; }); });
    Object.keys(dots).forEach(function (id) {
      var d = dots[id];
      m.P[id].par.filter(function (e) { return edgeVisible(m, dm, e); }).forEach(function (e) {
        var pd = dots[e.id];
        if (pd && pd.y < d.y) links.push({ from: pd, to: d, t: e.t, dm: e.dm, a: e.id, b: id });
      });
    });
    rows.forEach(function (r) {
      for (var i = 0; i < r.ents.length - 1; i++) {
        var a = r.ents[i], b = r.ents[i + 1];
        if (a.kind === 'dot' && b.kind === 'dot' && m.P[a.id].partners.indexOf(b.id) > -1) links.push({ from: a, to: b, t: 'partner', a: a.id, b: b.id });
      }
    });
    return { w: w, h: TOP + (rows.length - 1) * PITCH + 80, rows: rows, dots: dots, slots: slots, links: links, centreY: rows.indexOf(midRow) * PITCH + TOP, cap: cap_ };
  }

  /** pathD: a rounded elbow from a parent down to a child, or a loop under partners. */
  function pathD(l) {
    var a = l.from, b = l.to, dx = b.x - a.x, sg = dx < 0 ? -1 : 1;
    if (l.t === 'partner') {
      var ya = a.y + 58, yb = b.y + 58, yy = Math.max(ya, yb) + 12, r = Math.min(8, Math.abs(dx) / 2);
      return 'M' + a.x + ' ' + ya + 'V' + (yy - r) + 'Q' + a.x + ' ' + yy + ' ' + (a.x + sg * r) + ' ' + yy + 'H' + (b.x - sg * r) + 'Q' + b.x + ' ' + yy + ' ' + b.x + ' ' + (yy - r) + 'V' + yb;
    }
    var y1 = a.y + (a.me ? 62 : 58), y2 = b.y - (b.me ? 34 : 26), ym = (y1 + y2) / 2;
    if (!dx) return 'M' + a.x + ' ' + y1 + 'V' + y2;
    var r2 = Math.min(12, Math.abs(dx) / 2, ym - y1, y2 - ym);
    return 'M' + a.x + ' ' + y1 + 'V' + (ym - r2) + 'Q' + a.x + ' ' + ym + ' ' + (a.x + sg * r2) + ' ' + ym + 'H' + (b.x - sg * r2) + 'Q' + b.x + ' ' + ym + ' ' + b.x + ' ' + (ym + r2) + 'V' + y2;
  }

  /** Rects for the peek: first candidate with no overlap wins, else the cheapest. */
  function pickPeekSpot(cands, others, pts, bounds, G) {
    var best = null;
    cands.forEach(function (c) {
      var x = Math.max(bounds.left + G, Math.min(bounds.right - G - c.w, c.x));
      var y = Math.max(bounds.top + G, Math.min(bounds.bottom - G - c.h, c.y));
      var cost = c.cost, k = 0;
      others.forEach(function (o) { if (x < o.r && o.l < x + c.w && y < o.b && o.t < y + c.h) cost += 1e6; });
      pts.forEach(function (q) { if (q[0] > x && q[0] < x + c.w && q[1] > y && q[1] < y + c.h) k++; });
      cost += k * 1000;
      if (c.x < bounds.left + G - 0.5 || c.x + c.w > bounds.right - G + 0.5 || c.y < bounds.top + G - 0.5 || c.y + c.h > bounds.bottom - G + 0.5) cost += 100;
      if (!best || cost < best.cost) best = { cost: cost, x: x, y: y, flat: c.flat, w: c.fw, i: c.i };
    });
    return best;
  }

  // ---- Editing: pure changes to a model (unit-tested) ----

  var NEW_PREFIX = 'new:';

  function cloneModel(m) {
    var P = {};
    m.order.forEach(function (id) {
      var p = m.P[id];
      P[id] = {
        id: p.id, name: p.name, sub: p.sub, img: p.img, url: p.url, hue: p.hue, hidden: p.hidden, read: p.read,
        par: p.par.map(function (e) { return { id: e.id, t: e.t, dm: e.dm }; }), partners: p.partners.slice()
      };
    });
    return { P: P, order: m.order.slice(), page: m.page };
  }

  function addPerson(m, p) {
    if (m.P[p.id]) return m.P[p.id];
    m.P[p.id] = {
      id: p.id, name: String(p.name || ''), sub: String(p.sub || ''), img: safeUrl(p.img), url: safeUrl(p.url),
      hue: hueOf(p.id), hidden: !!p.hidden, read: !!p.read, par: [], partners: []
    };
    m.order.push(p.id);
    return m.P[p.id];
  }

  /** newPersonId: the next free ref for a page that does not exist yet. */
  function newPersonId(m) {
    var n = 1;
    while (m.P[NEW_PREFIX + n]) n++;
    return NEW_PREFIX + n;
  }

  /** personFromCandidate: a picker result as a person who is not read yet. */
  function personFromCandidate(c, urlBase) {
    return { id: c.id, name: c.name, sub: c.sub, img: c.img, url: urlBase ? urlBase + c.id : '', hidden: !!c.dm, read: false };
  }

  function addParentEdge(m, child, parent, kind) {
    var c = m.P[child];
    if (!c || !m.P[parent] || child === parent) return false;
    var cur = c.par.filter(function (e) { return e.id === parent; })[0];
    if (cur) { cur.t = kind; return true; }
    c.par.push({ id: parent, t: kind, dm: false });
    return true;
  }
  function removeParentEdge(m, child, parent) {
    var c = m.P[child];
    if (c) c.par = c.par.filter(function (e) { return e.id !== parent; });
  }
  function addPartnerPair(m, a, b) {
    var pa = m.P[a], pb = m.P[b];
    if (!pa || !pb || a === b) return false;
    if (pa.partners.indexOf(b) < 0) pa.partners.push(b);
    if (pb.partners.indexOf(a) < 0) pb.partners.push(a);
    return true;
  }
  function removePartnerPair(m, a, b) {
    if (m.P[a]) m.P[a].partners = m.P[a].partners.filter(function (x) { return x !== b; });
    if (m.P[b]) m.P[b].partners = m.P[b].partners.filter(function (x) { return x !== a; });
  }

  /** ancestorsOf / descendantsOf: every page above or below, through any link, for the picker. */
  function ancestorsOf(m, id) {
    var seen = {}, q = [id];
    while (q.length) {
      var u = q.shift();
      (m.P[u] ? m.P[u].par : []).forEach(function (e) { if (!seen[e.id] && e.id !== id) { seen[e.id] = 1; q.push(e.id); } });
    }
    return seen;
  }
  function descendantsOf(m, id) {
    var seen = {}, q = [id];
    while (q.length) {
      var u = q.shift();
      m.order.forEach(function (x) {
        if (!seen[x] && x !== id && m.P[x].par.some(function (e) { return e.id === u; })) { seen[x] = 1; q.push(x); }
      });
    }
    return seen;
  }

  /**
   * pickerExcluded: the pages the picker must not offer for a slot, so a link
   * that is already there, the page itself, or one that would make a page its
   * own ancestor cannot be picked. The server refuses the last one as well.
   */
  function pickerExcluded(m, centre, kind) {
    var ex = {}; ex[centre] = 1;
    var c = m.P[centre];
    if (!c) return ex;
    if (kind === 'parent') {
      c.par.forEach(function (e) { ex[e.id] = 1; });
      Object.keys(descendantsOf(m, centre)).forEach(function (k) { ex[k] = 1; });
    } else if (kind === 'child') {
      m.order.forEach(function (x) { if (m.P[x].par.some(function (e) { return e.id === centre; })) ex[x] = 1; });
      Object.keys(ancestorsOf(m, centre)).forEach(function (k) { ex[k] = 1; });
    } else {
      c.partners.forEach(function (k) { ex[k] = 1; });
    }
    return ex;
  }

  /**
   * diffModels: what changed from the saved model to the edited one, as the
   * request the server takes. A link added and removed again is no change; a
   * page that was made up and then left unlinked is never created.
   */
  function diffModels(base, cur) {
    var edits = [], used = {};
    function note(id) { if (id.indexOf(NEW_PREFIX) === 0) used[id] = 1; }
    cur.order.forEach(function (id) {
      var c = cur.P[id], b = base.P[id];
      c.par.forEach(function (e) {
        var be = b && b.par.filter(function (x) { return x.id === e.id; })[0];
        if (be && be.t === e.t) return;
        if (be) edits.push({ op: 'remove', edge: 'parent', a: id, b: e.id });
        edits.push({ op: 'add', edge: 'parent', a: id, b: e.id, link: e.t });
        note(id); note(e.id);
      });
      if (b) {
        b.par.forEach(function (e) {
          if (!c.par.some(function (x) { return x.id === e.id; })) edits.push({ op: 'remove', edge: 'parent', a: id, b: e.id });
        });
      }
      c.partners.forEach(function (o) {
        if (id < o && !(b && b.partners.indexOf(o) > -1)) { edits.push({ op: 'add', edge: 'partner', a: id, b: o }); note(id); note(o); }
      });
      if (b) {
        b.partners.forEach(function (o) {
          if (id < o && cur.P[o] && c.partners.indexOf(o) < 0) edits.push({ op: 'remove', edge: 'partner', a: id, b: o });
        });
      }
    });
    var newPages = Object.keys(used).map(function (ref) { return { ref: ref, name: cur.P[ref].name }; });
    return { newPages: newPages, edits: edits };
  }

  /** renameId: a page made up on screen got its real id from the server. */
  function renameId(m, from, to) {
    var p = m.P[from];
    if (!p || m.P[to]) return;
    p.id = to; p.hue = hueOf(to);
    m.P[to] = p; delete m.P[from];
    m.order = m.order.map(function (x) { return x === from ? to : x; });
    m.order.forEach(function (id) {
      var q = m.P[id];
      q.par.forEach(function (e) { if (e.id === from) e.id = to; });
      q.partners = q.partners.map(function (x) { return x === from ? to : x; });
    });
  }

  /** describeEdit: one change in words, for the save bar's report of what did not save. */
  function describeEdit(m, ed) {
    function nm(id) { return m.P[id] ? m.P[id].name : 'a page'; }
    var add = ed.op === 'add';
    if (ed.edge === 'partner') return (add ? 'adding ' : 'removing ') + nm(ed.a) + ' and ' + nm(ed.b) + ' as partners';
    var how = ed.link === 'adopted' ? 'an adoptive parent' : ed.link === 'guardian' ? 'a guardian' : 'a parent';
    return (add ? 'adding ' : 'removing ') + nm(ed.b) + (add ? ' as ' + how : ' as a parent') + ' of ' + nm(ed.a);
  }

  /**
   * saveReport: the sentence the save bar shows after a save that did not fully
   * land, from the server's answer. It says which links did not save, whether
   * the rest were taken back, and anything left half-written.
   */
  function saveReport(m, edits, res) {
    var bad = [], half = [], made = (res.created || []).map(function (c) { return c.name; });
    (res.failed || []).forEach(function (f) {
      if (f.index < 0 || !edits[f.index]) { bad.push('creating a new page'); return; }
      var t = describeEdit(m, edits[f.index]);
      (f.halfWritten ? half : bad).push(t);
    });
    var stuck = (res.written || []).filter(function (i) {
      return !(res.failed || []).some(function (f) { return f.index === i; });
    }).map(function (i) { return edits[i] ? describeEdit(m, edits[i]) : ''; }).filter(Boolean);
    var out = res.rolledBack ? 'Nothing was saved. ' : 'Some changes were saved and some were not. ';
    if (bad.length) out += 'Could not do: ' + bad.join('; ') + '. ';
    if (half.length) out += 'Left half-written, finish or remove it in Relations: ' + half.join('; ') + '. ';
    if (stuck.length) out += 'Saved and not taken back: ' + stuck.join('; ') + '. ';
    if (made.length) out += 'The new page' + (made.length > 1 ? 's ' : ' ') + made.join(', ') + (made.length > 1 ? ' were' : ' was') + ' created and will be used when you save again. ';
    out += res.rolledBack ? 'Your changes are still here; press Save to try again.' : 'What did not save is still here; press Save to try again.';
    return out;
  }

  // ---- DOM ----

  function mk(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function svgEl(name, attrs) {
    var e = document.createElementNS(SVGNS, name);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    return e;
  }
  function xIcon() {
    var s = svgEl('svg', { 'class': 'fm-ic', viewBox: '0 0 16 16', 'aria-hidden': 'true' });
    s.appendChild(svgEl('path', { d: 'M6 4l4 4-4 4' }));
    return s;
  }

  // jsonOf reads the block's data from data-json. A script tag would be
  // stripped by a boosted swap; one is still read for older markup.
  function jsonOf(root) {
    var raw = root.getAttribute('data-json');
    if (raw != null) return raw;
    var s = root.querySelector('script[type="application/json"]');
    return s ? s.textContent : '{}';
  }

  function rootOf(el) { return el && el.closest ? el.closest('.fm') : null; }

  function Family(root) {
    var self = this;
    var data = {};
    try { data = JSON.parse(jsonOf(root)); } catch (e) { data = {}; }
    var M = buildModel(data);
    var PAGE = M.page;
    // BASE is the family as saved; M is the family as edited. They differ only
    // while the Director has unsaved changes.
    var BASE = cloneModel(M);
    var E = { on: false, hist: [], pick: null, busy: false, note: '', noteBad: false };
    var dmServer = root.getAttribute('data-dm') === '1';
    var editUrl = root.getAttribute('data-edit-url') || '', pickUrl = root.getAttribute('data-pick-url') || '', pagesUrl = root.getAttribute('data-pages-url') || '';
    var mq = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;
    var S = { open: false, centre: PAGE, trail: [PAGE], sel: null, sib: false, xu: 0, xd: 0, openRows: {}, hov: null, phone: false };
    var L = null, els = {}, lastW = 0, pendingLay = false, peekT = 0, peekFor = null;
    var morphing = false, opener = { id: null }, anims = [], curDone = null, afterClose = false, dkAnim = [], linkTimer = 0;

    var q = function (s) { return root.querySelector(s); };
    var anchor = q('[data-fm-anchor]'), strip = q('[data-fm-strip-box]'), stripTr = q('[data-fm-strip]'), sheet = q('[data-fm-sheet]'),
      hdc = q('[data-fm-hdc]'), tv = q('[data-fm-tv]'), inner = q('[data-fm-in]'), rowsEl = q('[data-fm-rows]'),
      lkBase = q('[data-fm-lk]'), lkLit = q('[data-fm-lit]'), body = q('[data-fm-body]'), dock = q('[data-fm-dock]'),
      dkc = q('[data-fm-dkc]'), peek = q('[data-fm-peek]'), openBtn = q('[data-fm-open]'), closeBtn = q('[data-fm-close]'),
      sibBtn = q('[data-fm-sib]'), trailEl = q('[data-fm-trail]'), cntEl = q('[data-fm-cnt]'), shownEl = q('[data-fm-shown]'),
      liveEl = q('[data-fm-live]'), editBtn = q('[data-fm-edit]'), saveBar = q('[data-fm-savebar]'), saveMsg = q('[data-fm-savemsg]'),
      undoBtn = q('[data-fm-undo]'), saveBtn = q('[data-fm-save]'), legend = q('[data-fm-legend]');
    var gEl = mk('div'), dEl = mk('div');
    rowsEl.appendChild(gEl); rowsEl.appendChild(dEl);
    self.root = root;

    function dm() { return dmServer && !document.documentElement.hasAttribute('data-view-as-player'); }
    function level() { return motionOf(document.documentElement.getAttribute('data-motion'), mq && mq.matches); }
    function say(t) { liveEl.textContent = t; }
    function name(id) { return M.P[id].name; }
    function vis(id) { return visible(M, dm(), id); }

    // ---- Circles ----
    function circle(id) {
      var p = M.P[id] || BASE.P[id], pw = mk('span', 'fm-pw'), pic = mk('span', 'fm-pic ' + (p.img ? 'fm-img' : 'fm-ini'));
      pic.style.setProperty('--h', p.hue);
      if (p.img) {
        var im = mk('img'); im.setAttribute('src', p.img); im.setAttribute('alt', ''); im.setAttribute('loading', 'lazy'); im.setAttribute('decoding', 'async'); im.setAttribute('draggable', 'false');
        im.onerror = function () { im.remove(); pic.classList.remove('fm-img'); pic.classList.add('fm-ini'); pic.appendChild(mk('i', null, initialsOf(p.name))); };
        pic.appendChild(im);
      } else {
        pic.appendChild(mk('i', null, initialsOf(p.name)));
      }
      pw.appendChild(pic);
      if (p.hidden && dm()) pw.appendChild(mk('span', 'fm-dmtag cb-dm', 'DM'));
      pw.appendChild(mk('span', 'fm-halo'));
      return pw;
    }
    function kindCls(p, t) { return (t === 'adopted' ? ' fm-ad' : '') + (t === 'guardian' ? ' fm-gd' : '') + (p.hidden && dm() ? ' fm-dmp' : ''); }

    // ---- The line of descent at rest ----
    function stripItem(id, cls, rel, t) {
      var p = M.P[id] || BASE.P[id], b = mk('button', 'fm-hs-it' + cls + kindCls(p, t));
      b.type = 'button'; b.setAttribute('data-id', id); b.setAttribute('data-rel', rel);
      b.setAttribute('aria-label', p.name + ', ' + rel);
      b.appendChild(circle(id)); b.appendChild(mk('span', 'fm-nm', p.name));
      b.addEventListener('click', function (e) { onStripClick(b, e); });
      wirePeek(b);
      return b;
    }
    function renderStrip() {
      var c = PAGE, d = dm(), pe = parentsOf(BASE, d, c), pa = partnersOf(BASE, d, c), ks = kidsOf(BASE, d, c);
      var f = document.createDocumentFragment();
      pe.forEach(function (e) { f.appendChild(stripItem(e.id, '', relLabel('up', 1, e.t), e.t)); });
      if (pe.length) f.appendChild(mk('span', 'fm-hs-gap'));
      f.appendChild(stripItem(c, ' fm-me', 'This page', null));
      if (pa.length) { f.appendChild(mk('span', 'fm-hs-gap')); pa.forEach(function (id) { f.appendChild(stripItem(id, '', 'Partner')); }); }
      if (ks.length) {
        f.appendChild(mk('span', 'fm-hs-gap'));
        var cap_ = ks.length > 6 ? 5 : 6;
        ks.slice(0, cap_).forEach(function (id) {
          var e = BASE.P[id].par.filter(function (x) { return x.id === c; })[0];
          f.appendChild(stripItem(id, '', relLabel('down', 1, e && e.t), e && e.t));
        });
        if (ks.length > cap_) {
          var more = mk('button', 'fm-hs-it'); more.type = 'button';
          more.setAttribute('aria-label', (ks.length - cap_) + ' more children, open the tree');
          var pw = mk('span', 'fm-pw'); pw.style.setProperty('--d', '30px');
          pw.appendChild(mk('span', 'fm-pic fm-bub', '+' + (ks.length - cap_)));
          more.appendChild(pw); more.appendChild(mk('span', 'fm-nm', 'more'));
          more.addEventListener('click', function (e) { setOpen(true, more, null, e.detail === 0); });
          f.appendChild(more);
        }
      }
      if (d && !pe.length && !pa.length && !ks.length) {
        var none = mk('span', 'fm-hs-none', 'No family linked yet. Open Family and press Edit to add some.');
        none.setAttribute('data-dm-only', '');
        f.appendChild(none);
      }
      stripTr.textContent = '';
      stripTr.appendChild(f);
      var t = reachable(BASE, d, c), cut = data.cutGenerations || data.cutPeople, n = d ? pendingCount() : 0;
      if (n) cntEl.textContent = n === 1 ? '1 change not saved' : n + ' changes not saved';
      else cntEl.textContent = cut ? 'At least ' + t + ' relatives' : t + (t === 1 ? ' relative' : ' relatives');
    }
    function onStripClick(b, e) {
      var id = b.getAttribute('data-id');
      if (S.open) { if (id && id !== PAGE) select(id); return; }
      setOpen(true, b, id, e.detail === 0);
    }

    // ---- The tree ----
    function dotEl(e) {
      var key = e.kind === 'dot' ? e.id : e.key, cls = '', b;
      if (e.kind === 'bub') {
        b = mk('button', 'fm-b'); b.type = 'button'; b.setAttribute('data-bub', e.act);
        b.setAttribute('aria-label', e.n + ' ' + e.label + ', show them');
        var pw = mk('span', 'fm-pw'); pw.appendChild(mk('span', 'fm-pic fm-bub', '+' + e.n));
        b.appendChild(pw); b.appendChild(mk('span', 'fm-nm', e.label));
        b.addEventListener('click', function () {
          if (e.act === 'fu') S.xu++; else if (e.act === 'fd') S.xd++; else S.openRows[e.act] = true;
          render({ glide: true }); say('Showing more');
        });
        return { key: key, cls: '', sig: 'b' + e.act + e.n, node: b };
      }
      if (e.kind === 'slot') {
        b = mk('button', 'fm-b fm-slot'); b.type = 'button'; b.setAttribute('data-slot', e.slot);
        b.setAttribute('aria-label', e.label + ' to ' + name(S.centre));
        var spw = mk('span', 'fm-pw'); spw.appendChild(mk('span', 'fm-pic fm-plus', '+'));
        b.appendChild(spw); b.appendChild(mk('span', 'fm-nm', e.label));
        b.addEventListener('click', function () { openPick(e.slot, b); });
        return { key: key, cls: '', sig: 's' + e.slot + S.centre, node: b };
      }
      var p = M.P[e.id], rm = null;
      cls = (e.me ? ' fm-me' : '') + kindCls(p, e.t);
      b = mk('button', 'fm-b'); b.type = 'button'; b.setAttribute('data-id', e.id); b.setAttribute('data-rel', e.rel);
      b.setAttribute('aria-label', p.name + ', ' + e.rel);
      b.appendChild(circle(e.id)); b.appendChild(mk('span', 'fm-nm', p.name));
      b.addEventListener('click', function () { if (S.sel === e.id && e.id !== S.centre) return; select(e.id); });
      wirePeek(b);
      if (editing() && e.via && !e.me && !e.sib) {
        // Taking a link off never deletes a page; it is one more change for the save bar.
        rm = mk('button', 'fm-rm'); rm.type = 'button';
        rm.setAttribute('aria-label', 'Remove ' + p.name + ' from this family'); rm.setAttribute('title', 'Remove');
        rm.appendChild(svgEl('svg', { 'class': 'fm-ic', viewBox: '0 0 16 16', 'aria-hidden': 'true' })).appendChild(svgEl('path', { d: 'M4 4l8 8M12 4l-8 8' }));
        rm.addEventListener('click', function (ev) { ev.stopPropagation(); removeFrom(e); });
      }
      return { key: key, cls: cls, sig: 'p' + e.id + e.rel + cls + (p.hidden && dm() ? 'h' : '') + (rm ? 'r' + e.dir + e.via : ''), node: b, rm: rm };
    }

    function render(opts) {
      opts = opts || {};
      if (!S.open) return;
      var Wp = tv.clientWidth;
      if (!Wp) { pendingLay = true; return; }
      pendingLay = false; lastW = Wp;
      L = layoutTree(M, { dm: dm(), centre: S.centre, phone: S.phone, width: Wp, xu: S.xu, xd: S.xd, openRows: S.openRows, sib: S.sib, edit: editing() });
      inner.style.width = L.w + 'px'; inner.style.height = L.h + 'px';
      [lkBase, lkLit].forEach(function (s) { s.setAttribute('width', L.w); s.setAttribute('height', L.h); });
      gEl.textContent = '';
      L.rows.forEach(function (r) {
        var gl = mk('div', 'fm-gl'); gl.style.top = r.y + 'px'; gl.style.width = L.w + 'px'; gEl.appendChild(gl);
        var rl = mk('span', 'fm-rl', r.label); rl.style.top = (r.y - 40) + 'px'; gEl.appendChild(rl);
      });
      var want = {};
      L.rows.forEach(function (r) { r.ents.forEach(function (e) { var d = dotEl(e); d.e = e; want[d.key] = d; }); });
      Object.keys(els).forEach(function (k) {
        if (!want[k]) {
          var el = els[k]; delete els[k];
          el.classList.add('fm-gone');
          if (level() === 'off') el.remove(); else setTimeout(function () { el.remove(); }, 220);
        }
      });
      Object.keys(want).forEach(function (k) {
        var d = want[k], e = d.e, el = els[k], top = e.y - (e.me ? 28 : 20);
        if (!el) {
          el = mk('div'); els[k] = el; el._sig = '';
          el.style.transform = 'translate(' + (e.x - 44) + 'px,' + top + 'px)';
          dEl.appendChild(el);
          if (opts.first !== true && level() !== 'off') { el.classList.add('fm-new'); (function (x) { setTimeout(function () { x.classList.remove('fm-new'); }, 700); })(el); }
        }
        if (el._sig !== d.sig) {
          el._sig = d.sig;
          el.className = 'fm-dot' + d.cls + (el.classList.contains('fm-new') ? ' fm-new' : '');
          el.textContent = ''; el.appendChild(d.node);
          if (d.rm) el.appendChild(d.rm);
        }
        el.style.transform = 'translate(' + (e.x - 44) + 'px,' + top + 'px)';
        el.classList.remove('fm-gone');
      });
      drawBase();
      var glide = level() === 'full' && opts.glide;
      clearTimeout(linkTimer);
      if (glide) {
        lkBase.style.transition = 'none'; lkBase.style.opacity = '0'; lkLit.style.display = 'none';
        linkTimer = setTimeout(function () { lkBase.style.transition = ''; lkBase.style.opacity = '1'; lkLit.style.display = ''; lightUp(); }, 420);
      } else { lkBase.style.opacity = '1'; lkLit.style.display = ''; lightUp(); }
      if (opts.fade && level() === 'calm' && tv.animate) tv.animate([{ opacity: 0.2 }, { opacity: 1 }], { duration: 220 });
      shownEl.textContent = Object.keys(L.dots).length + ' shown of ' + (reachable(M, dm(), PAGE) + 1);
      if (E.pick) placePick();
    }
    function pathNode(l, lit) {
      var p = svgEl('path', { d: pathD(l) });
      var c = (l.t === 'guardian' ? 'fm-gd ' : '') + (l.t === 'adopted' ? 'fm-ad ' : '') + (l.dm ? 'fm-dml' : '');
      if (c.trim()) p.setAttribute('class', c.trim());
      if (lit) p.setAttribute('pathLength', '1');
      return p;
    }
    function drawBase() {
      lkBase.textContent = '';
      L.links.forEach(function (l) { lkBase.appendChild(pathNode(l, false)); });
    }
    function lightUp() {
      var ids = {};
      if (S.hov) ids[S.hov] = 1;
      if (S.sel) ids[S.sel] = 1;
      lkLit.textContent = '';
      if (L) L.links.forEach(function (l) { if (ids[l.a] || ids[l.b]) lkLit.appendChild(pathNode(l, true)); });
      Object.keys(els).forEach(function (k) { els[k].classList.toggle('fm-on', !!ids[k]); els[k].classList.toggle('fm-sel', k === S.sel); });
    }
    function scrollToCentre() {
      if (!L) return;
      tv.scrollTop = Math.max(0, L.centreY - tv.clientHeight / 2);
      tv.scrollLeft = Math.max(0, (L.w - tv.clientWidth) / 2);
    }

    // ---- Trail and the docked card ----
    function renderTrail() {
      var tr = S.trail, from = Math.max(0, tr.length - 4);
      trailEl.textContent = '';
      if (from > 0) { var dots = mk('span', 'fm-q', '…'); dots.style.cursor = 'default'; trailEl.appendChild(dots); }
      tr.forEach(function (id, i) {
        if (i < from) return;
        if (i < tr.length - 1) {
          var b = mk('button', 'fm-q', name(id)); b.type = 'button';
          b.addEventListener('click', function () { setCentre(tr[i]); });
          trailEl.appendChild(b);
          var ic = svgEl('svg', { 'class': 'fm-ic', viewBox: '0 0 16 16', 'aria-hidden': 'true' });
          ic.appendChild(svgEl('path', { d: 'M6 4l4 4-4 4' }));
          trailEl.appendChild(ic);
        } else trailEl.appendChild(mk('b', null, name(id)));
      });
    }
    function factRow(k, v) { var r = mk('div', 'fm-fact'); r.appendChild(mk('span', null, k)); r.appendChild(mk('span', null, v)); return r; }
    function circleRect(id) { var el = els[id], pic = el && el.querySelector('.fm-pic'); return pic ? pic.getBoundingClientRect() : null; }
    function dkStop() { dkAnim.forEach(function (a) { try { a.cancel(); } catch (e) { /* already finished */ } }); dkAnim = []; dkc.style.width = ''; dkc.style.height = ''; body.classList.remove('fm-dk-keep'); }
    function bodyGeom(el, bg) {
      var br = body.getBoundingClientRect(), r = el.getBoundingClientRect(), cs = getComputedStyle(el);
      return { l: r.left - br.left, t: r.top - br.top, w: r.width, h: r.height, r: parseFloat(cs.borderTopLeftRadius) || 0, bg: bg || cs.backgroundColor, sh: cs.boxShadow };
    }
    function circGeom(rect, el) {
      var br = body.getBoundingClientRect();
      return { l: rect.left - br.left, t: rect.top - br.top, w: rect.width, h: rect.height, r: rect.width / 2, bg: getComputedStyle(el).backgroundColor, sh: 'none' };
    }
    function renderDock(grow, shrink) {
      var id = S.sel, ok = !!id && vis(id), was = body.classList.contains('fm-docked');
      dkStop();
      if (!ok) {
        if (was && shrink && S.open && level() === 'full') {
          body.classList.add('fm-dk-keep'); body.classList.remove('fm-docked');
          var from = bodyGeom(dock), to = circGeom(shrink.rect, shrink.el);
          dkc.style.width = dock.clientWidth + 'px'; dkc.style.height = dock.clientHeight + 'px';
          dkAnim = morphAnim(dock, [dkc], from, to, false, dkStop);
        } else if (was && S.open && level() === 'calm') {
          body.classList.add('fm-dk-keep'); body.classList.remove('fm-docked');
          var a = dock.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 140 }); dkAnim = [a]; a.onfinish = dkStop;
        } else body.classList.remove('fm-docked');
        return;
      }
      var p = M.P[id], d = L && L.dots[id], rel = d ? d.rel : 'Relative', dmv = dm();
      var pe = parentsOf(M, dmv, id).map(function (e) { return name(e.id) + (e.t === 'adopted' ? ' (adopted)' : e.t === 'guardian' ? ' (guardian)' : ''); });
      var ks = kidsOf(M, dmv, id).map(name), pt = partnersOf(M, dmv, id).map(name);
      dkc.textContent = '';
      var x = mk('button', 'fm-dkx'); x.type = 'button'; x.setAttribute('aria-label', 'Close this card');
      var xs = svgEl('svg', { 'class': 'fm-ic', viewBox: '0 0 16 16', 'aria-hidden': 'true' }); xs.appendChild(svgEl('path', { d: 'M4 4l8 8M12 4l-8 8' })); x.appendChild(xs);
      x.addEventListener('click', deselect);
      dkc.appendChild(x);
      var top = mk('div', 'fm-dk-top'); top.appendChild(circle(id));
      var who = mk('div'); who.appendChild(mk('b', null, p.name));
      who.appendChild(mk('span', 'fm-r', rel + (id === S.centre ? '' : ' of ' + name(S.centre))));
      top.appendChild(who); dkc.appendChild(top);
      if (p.sub) dkc.appendChild(mk('p', null, p.sub));
      if (pe.length) dkc.appendChild(factRow('Born to', pe.join(', ')));
      if (pt.length) dkc.appendChild(factRow('Partner', pt.join(', ')));
      if (ks.length) dkc.appendChild(factRow('Children', ks.slice(0, 4).join(', ') + (ks.length > 4 ? ' and ' + (ks.length - 4) + ' more' : '')));
      if (!p.read) dkc.appendChild(mk('p', 'fm-note', 'Their own family is on their page.'));
      if (p.hidden && dmv) {
        var n = mk('div', 'fm-dmnote'); n.setAttribute('data-dm-only', '');
        n.appendChild(mk('span', 'fm-dmtag cb-dm', 'DM'));
        n.appendChild(mk('span', null, 'Hidden from players. They see no card and no gap.'));
        dkc.appendChild(n);
      }
      var act = mk('div', 'fm-act');
      if (id !== S.centre && p.read) {
        var mc = mk('button', 'fm-cmd', 'Make centre'); mc.type = 'button';
        mc.addEventListener('click', function () { setCentre(id); });
        act.appendChild(mc);
      }
      if (p.url) {
        var a2 = mk('a', 'fm-lnk', 'Open page'); a2.setAttribute('href', p.url); a2.appendChild(xIcon());
        act.appendChild(a2);
      }
      dkc.appendChild(act);
      body.classList.add('fm-docked');
      if (grow && S.open && !morphing) {
        if (level() === 'full') {
          var to2 = bodyGeom(dock), from2 = circGeom(grow.rect, grow.el);
          dkc.style.width = dock.clientWidth + 'px'; dkc.style.height = dock.clientHeight + 'px';
          dkAnim = morphAnim(dock, [dkc], from2, to2, true, dkStop);
        } else if (level() === 'calm') {
          var a3 = dock.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 180 }); dkAnim = [a3]; a3.onfinish = dkStop;
        }
      }
    }
    function select(id) {
      if (!id || !vis(id)) return;
      var cr = circleRect(id);
      S.sel = id; hidePeek();
      renderDock(cr ? { rect: cr, el: els[id].querySelector('.fm-pic') } : null);
      render({ glide: true }); say(name(id) + ' selected');
    }
    function deselect() {
      if (!S.sel) return;
      var id = S.sel, cr = circleRect(id), pic = els[id] && els[id].querySelector('.fm-pic');
      S.sel = null;
      renderDock(null, cr && pic ? { rect: cr, el: pic } : null);
      render({ glide: true });
    }
    function setCentre(id) {
      if (id === S.centre || !vis(id) || !M.P[id].read) return;
      var i = S.trail.indexOf(id);
      if (i > -1) S.trail = S.trail.slice(0, i + 1); else S.trail.push(id);
      S.centre = id; S.xu = 0; S.xd = 0; S.openRows = {};
      renderTrail(); renderDock(); render({ glide: true, fade: true }); scrollToCentre();
      say('Tree centred on ' + name(id));
    }

    // ---- The Peek ----
    function hidePeek() { clearTimeout(peekT); peekFor = null; peek.classList.remove('fm-on'); }
    function peekFill(el) {
      var id = el.getAttribute('data-id'), p = M.P[id];
      peek.textContent = '';
      var pk = mk('span', 'fm-pk-pw'); pk.appendChild(circle(id)); peek.appendChild(pk);
      var t = mk('div', 'fm-pk-t'); t.appendChild(mk('b', null, p.name)); t.appendChild(document.createTextNode(el.getAttribute('data-rel') || ''));
      if (p.sub) { t.appendChild(document.createElement('br')); t.appendChild(document.createTextNode(p.sub)); }
      peek.appendChild(t);
      peek.classList.toggle('fm-dmpk', !!(p.hidden && dm()));
    }
    function showPeek(el) {
      var id = el.getAttribute('data-id');
      if (!id || !M.P[id]) return;
      var ar = anchor.getBoundingClientRect(), pr = (el.querySelector('.fm-pw') || el).getBoundingClientRect();
      peekFill(el);
      if (el.classList.contains('fm-hs-it')) {
        peek.classList.remove('fm-beside', 'fm-flat'); peek.style.width = '';
        var cx = pr.left + pr.width / 2 - ar.left;
        cx = Math.max(100, Math.min(ar.width - 100, cx));
        peek.style.left = cx + 'px'; peek.style.top = (pr.top - ar.top - 12) + 'px'; peek.classList.add('fm-on');
        return;
      }
      placePeek(el, pr, ar);
      peek.classList.add('fm-on');
    }
    // In the tree the card never covers another person's circle or the hovered person's own lit lines:
    // it sits beside the circle (right, then left), else in the band under or over its row, and stays
    // inside the visible scroll area. The first candidate with no overlap wins; if none is clear, the cheapest.
    function placePeek(el, pr, ar) {
      var tr = tv.getBoundingClientRect(), others = [], pts = [], G = 6;
      var mine = el.closest('.fm-dot');
      rowsEl.querySelectorAll('.fm-dot:not(.fm-gone) .fm-pic').forEach(function (c) {
        if (c.closest('.fm-dot') !== mine) { var r = c.getBoundingClientRect(); others.push({ l: r.left - 3, t: r.top - 3, r: r.right + 3, b: r.bottom + 3 }); }
      });
      var sr = lkLit.getBoundingClientRect();
      Array.prototype.slice.call(lkLit.querySelectorAll('path')).forEach(function (pa) {
        var n = pa.getTotalLength();
        for (var d = 0; d <= n; d += 5) { var pt = pa.getPointAtLength(d); pts.push([sr.left + pt.x, sr.top + pt.y]); }
      });
      var rowBot = pr.bottom, rowTop = pr.top, cyy = pr.top + pr.height / 2;
      rowsEl.querySelectorAll('.fm-dot:not(.fm-gone) .fm-pic').forEach(function (c) {
        var r = c.getBoundingClientRect();
        if (Math.abs(r.top + r.height / 2 - cyy) < 24) { rowBot = Math.max(rowBot, r.bottom); rowTop = Math.min(rowTop, r.top); }
      });
      var cands = [];
      [[false, 0], [true, 0], [true, 176]].forEach(function (v, vi) {
        peek.classList.add('fm-beside'); peek.classList.toggle('fm-flat', v[0]); peek.style.width = v[1] ? v[1] + 'px' : '';
        var w = peek.offsetWidth, h = peek.offsetHeight, cy = pr.top + pr.height / 2, cxm = pr.left + pr.width / 2, bandY = rowBot + 4, upY = rowTop - 4 - h;
        var base = v[1] ? 1 : 0;
        if (!v[0]) {
          cands.push({ x: pr.right + 14, y: cy - h / 2, w: w, h: h, cost: base, flat: false, fw: 0 });
          cands.push({ x: pr.left - 14 - w, y: cy - h / 2, w: w, h: h, cost: 1 + base, flat: false, fw: 0 });
        } else {
          for (var x = tr.left + G; x <= tr.right - G - w + 0.5; x += 10) {
            var dd = Math.abs(x + w / 2 - cxm) / 1000;
            cands.push({ x: x, y: bandY, w: w, h: h, cost: 2 + dd + base, flat: true, fw: v[1] });
            cands.push({ x: x, y: upY, w: w, h: h, cost: 50 + dd + base, flat: true, fw: v[1] });
          }
        }
      });
      // Candidates that sit on the hovered circle itself are never clear.
      cands.forEach(function (c) { if (c.x < pr.right && pr.left < c.x + c.w && c.y < pr.bottom && pr.top < c.y + c.h) c.cost += 1e6; });
      var best = pickPeekSpot(cands, others, pts, tr, G);
      peek.classList.toggle('fm-flat', best.flat);
      peek.style.width = best.w ? best.w + 'px' : '';
      peek.style.left = (best.x - ar.left) + 'px'; peek.style.top = (best.y - ar.top) + 'px';
    }
    function peekLater(el) {
      var id = el.getAttribute('data-id');
      if (!id || peekFor === id) return;
      hidePeek(); peekFor = id;
      peekT = setTimeout(function () { showPeek(el); }, level() === 'off' ? 0 : REST_MS);
    }
    function wirePeek(b) {
      b.addEventListener('mouseenter', function () { peekLater(b); if (b.closest('.fm-dot') && b.getAttribute('data-id') !== S.hov) { S.hov = b.getAttribute('data-id'); lightUp(); } });
      b.addEventListener('mouseleave', function () { hidePeek(); if (S.hov) { S.hov = null; lightUp(); } });
      b.addEventListener('focus', function () { if (b.closest('.fm-dot')) { S.hov = b.getAttribute('data-id'); lightUp(); } peekLater(b); });
      b.addEventListener('blur', function () { hidePeek(); if (S.hov) { S.hov = null; lightUp(); } });
    }
    tv.addEventListener('scroll', function () { hidePeek(); inner.style.setProperty('--sl', tv.scrollLeft + 'px'); });

    // ---- The Director's Edit tab ----
    function editing() { return E.on && dm() && !!editUrl; }
    function pendingCount() { return diffModels(BASE, M).edits.length; }
    function barShown() { return dm() && !!saveBar && (E.on || pendingCount() > 0 || !!E.note); }
    function updateBar() {
      if (!saveBar) return;
      var n = pendingCount(), show = barShown();
      saveBar.hidden = !show;
      if (legend) legend.hidden = show;
      if (!show) return;
      var msg = E.busy ? 'Saving…' : E.note || (n ? (n === 1 ? '1 change not saved' : n + ' changes not saved') : 'No changes yet');
      saveMsg.textContent = msg;
      saveBar.classList.toggle('fm-err', !!E.noteBad && !E.busy);
      saveMsg.setAttribute('aria-live', E.noteBad ? 'assertive' : 'polite');
      undoBtn.disabled = !E.hist.length || E.busy;
      saveBtn.disabled = !n || E.busy;
    }
    function editUi() {
      if (editBtn) {
        var on = editing();
        editBtn.setAttribute('aria-pressed', on ? 'true' : 'false');
        editBtn.textContent = on ? 'Done' : 'Edit';
      }
      updateBar();
    }
    function snapshot() { return JSON.stringify({ P: M.P, order: M.order }); }
    function pushHist() { E.hist.push(snapshot()); E.note = ''; E.noteBad = false; }
    function afterEdit() {
      closePick(); renderStrip(); updateBar(); renderDock(); render({ glide: true });
    }
    function removeFrom(e) {
      pushHist();
      if (e.dir === 'up') removeParentEdge(M, e.via, e.id);
      else if (e.dir === 'down') removeParentEdge(M, e.id, e.via);
      else if (e.dir === 'partner') removePartnerPair(M, S.centre, e.id);
      var nm = name(e.id);
      if (S.sel === e.id) S.sel = null;
      afterEdit(); say(nm + ' removed from this family');
    }
    function undoEdit() {
      var snap = E.hist.pop();
      if (!snap || E.busy) return;
      var o = JSON.parse(snap);
      M = { P: o.P, order: o.order, page: PAGE };
      E.note = ''; E.noteBad = false;
      if (!vis(S.sel || PAGE)) S.sel = null;
      afterEdit(); say('Last change undone');
    }

    // The picker grows from the empty place it was opened from. It offers only what
    // the server says the Director can open, a bounded page at a time, narrowed by typing.
    function closePick(restore) {
      var pk = E.pick;
      if (!pk) return;
      clearTimeout(pk.timer); pk.seq++;
      pk.el.remove(); E.pick = null;
      if (restore) { var b = rowsEl.querySelector('[data-slot="' + pk.kind + '"]'); if (b) b.focus(); }
    }
    function placePick() {
      var pk = E.pick, slot = pk && L && L.slots['s:' + pk.kind];
      if (!slot) return;
      var w = S.phone ? 230 : 250, EST = 330;
      var left = Math.max(8, Math.min(L.w - w - 8, slot.x - w / 2));
      var below = !(slot.y > L.h - EST - 70 && slot.y > EST + 80);
      pk.el.style.left = left + 'px';
      pk.el.style.setProperty('--px', (slot.x - left) + 'px');
      if (below) {
        pk.el.style.bottom = ''; pk.el.style.top = (slot.y + 62) + 'px';
        pk.el.style.setProperty('--py', '-40px');
        inner.style.height = Math.max(L.h, slot.y + 62 + EST + 14) + 'px';
      } else {
        pk.el.style.top = ''; pk.el.style.bottom = (L.h - (slot.y - 24)) + 'px';
        pk.el.style.setProperty('--py', '100%');
      }
    }
    function openPick(kind, slotBtn) {
      closePick(); hidePeek();
      var noun = { parent: 'a parent', child: 'a child', partner: 'a partner' }[kind];
      var el = mk('div', 'fm-pick'); el.setAttribute('role', 'dialog'); el.setAttribute('aria-label', 'Add ' + noun);
      var h = mk('div', 'fm-pick-h'); h.appendChild(mk('span', null, 'Add ' + noun));
      var x = mk('button', 'fm-pkx'); x.type = 'button'; x.setAttribute('aria-label', 'Close the picker');
      x.appendChild(svgEl('svg', { 'class': 'fm-ic', viewBox: '0 0 16 16', 'aria-hidden': 'true' })).appendChild(svgEl('path', { d: 'M4 4l8 8M12 4l-8 8' }));
      h.appendChild(x); el.appendChild(h);
      var pk = { kind: kind, link: 'birth', el: el, seq: 0, timer: 0 };
      if (kind !== 'partner') {
        var seg = mk('div', 'fm-seg'); seg.setAttribute('role', 'group'); seg.setAttribute('aria-label', 'Link type');
        [['birth', 'Born to'], ['adopted', 'Adopted'], ['guardian', kind === 'parent' ? 'Guardian' : 'Ward']].forEach(function (t) {
          var b = mk('button', null, t[1]); b.type = 'button'; b.setAttribute('data-v', t[0]); b.setAttribute('aria-pressed', t[0] === 'birth' ? 'true' : 'false');
          b.addEventListener('click', function () {
            pk.link = t[0];
            Array.prototype.forEach.call(seg.children, function (o) { o.setAttribute('aria-pressed', o === b ? 'true' : 'false'); });
          });
          seg.appendChild(b);
        });
        el.appendChild(seg);
      }
      var inp = mk('input'); inp.type = 'text'; inp.setAttribute('placeholder', 'Search pages'); inp.setAttribute('aria-label', 'Search pages');
      inp.setAttribute('autocomplete', 'off'); inp.setAttribute('maxlength', '200');
      el.appendChild(inp);
      var list = mk('div', 'fm-pl'); list.setAttribute('aria-live', 'polite'); el.appendChild(list);
      var mkBtn = mk('button', 'fm-mk', '+ Create a new page'); mkBtn.type = 'button'; el.appendChild(mkBtn);
      pk.input = inp; pk.list = list; pk.mkBtn = mkBtn; pk.cands = []; pk.ready = false;
      E.pick = pk;
      inner.appendChild(el);
      placePick();

      x.addEventListener('click', function () { closePick(true); });
      inp.addEventListener('input', function () { labelCreate(); clearTimeout(pk.timer); pk.timer = setTimeout(load, 220); });
      inp.addEventListener('keydown', function (ev) {
        if (ev.key !== 'Enter') return;
        ev.preventDefault();
        var shown = pickList();
        if (shown.length && inp.value.trim()) commit(shown[0]);
        else if (!shown.length && pk.ready) createFromText();
      });
      mkBtn.addEventListener('click', createFromText);
      labelCreate();
      load();
      inp.focus({ preventScroll: true });
      var bot = parseFloat(el.style.top || 0) + 330;
      if (bot > tv.scrollTop + tv.clientHeight) tv.scrollTop = bot - tv.clientHeight;
      say('Choose ' + noun);

      function labelCreate() {
        var t = inp.value.trim();
        mkBtn.textContent = t ? '+ Create a new page “' + t + '”' : '+ Create a new page';
      }
      function pickList() {
        var ex = pickerExcluded(M, S.centre, kind);
        return pk.cands.filter(function (c) { return !ex[c.id]; });
      }
      function fill() {
        list.textContent = '';
        if (!pk.ready) { list.appendChild(mk('div', 'fm-none', 'Searching…')); return; }
        if (pk.failed) { list.appendChild(mk('div', 'fm-none', 'Pages could not be loaded. Type to try again.')); return; }
        var shown = pickList();
        if (!shown.length) {
          var t = inp.value.trim();
          list.appendChild(mk('div', 'fm-none', t.length === 1 ? 'Type one more letter to search.' : 'No page matches. Create one below.'));
          return;
        }
        shown.forEach(function (c) {
          var b = mk('button'); b.type = 'button'; b.setAttribute('data-pid', c.id);
          b.appendChild(mk('span', 'fm-pl-n', c.name));
          if (c.dm) b.appendChild(mk('span', 'fm-dmtag cb-dm', 'DM'));
          b.appendChild(mk('small', null, c.sub));
          b.addEventListener('click', function () { commit(c); });
          list.appendChild(b);
        });
        if (pk.more) list.appendChild(mk('div', 'fm-none', 'More pages match. Type to narrow the list.'));
      }
      function load() {
        var seq = ++pk.seq, t = inp.value.trim();
        pk.ready = false; pk.failed = false; fill();
        var done = function (data, failed) {
          if (E.pick !== pk || seq !== pk.seq) return;
          pk.ready = true; pk.failed = failed;
          pk.cands = failed ? [] : ((data && data.pages) || []).map(function (c) { return { id: String(c.id), name: String(c.name || ''), sub: String(c.sub || ''), img: safeUrl(c.img), dm: !!c.dm }; });
          pk.more = !failed && !!(data && data.more);
          fill();
        };
        if (!window.Chronicle || !window.Chronicle.apiFetch || !pickUrl) { done(null, true); return; }
        window.Chronicle.apiFetch(pickUrl + (t ? '?q=' + encodeURIComponent(t) : '')).then(function (r) {
          if (!r.ok) throw new Error('status ' + r.status);
          return r.json();
        }).then(function (d) { done(d, false); }, function () { done(null, true); });
      }
      function createFromText() {
        var t = inp.value.trim();
        if (!t) { inp.focus(); list.textContent = ''; list.appendChild(mk('div', 'fm-none', 'Type the new page’s name first.')); return; }
        commit(null, t);
      }
    }
    function commit(c, newName) {
      var pk = E.pick; if (!pk) return;
      var kind = pk.kind, link = pk.link, centre = S.centre, other;
      pushHist();
      if (c) {
        addPerson(M, personFromCandidate(c, pagesUrl));
        other = c.id;
      } else {
        other = newPersonId(M);
        addPerson(M, { id: other, name: newName, sub: 'New page', img: '', url: '', hidden: false, read: true });
      }
      if (kind === 'parent') addParentEdge(M, centre, other, link);
      else if (kind === 'child') addParentEdge(M, other, centre, link);
      else addPartnerPair(M, centre, other);
      var nm = M.P[other].name;
      afterEdit(); say(nm + ' added');
    }

    // Save sends only what differs from the saved family, in one request, and shows what the server says is now true.
    function save() {
      if (E.busy || !dm() || !editUrl) return;
      var d = diffModels(BASE, M);
      if (!d.edits.length) return;
      E.busy = true; closePick(); updateBar();
      var done = function (res) { E.busy = false; settle(d, res); };
      if (!window.Chronicle || !window.Chronicle.apiFetch) { done(null); return; }
      window.Chronicle.apiFetch(editUrl, { method: 'POST', body: { newPages: d.newPages, edits: d.edits } }).then(function (r) {
        return r.json().then(function (j) { return { ok: r.ok, status: r.status, j: j }; }, function () { return { ok: false, status: r.status, j: null }; });
      }).then(done, function () { done(null); });
    }
    function reconcileState() {
      S.trail = S.trail.filter(function (id) { return !!M.P[id] && vis(id); });
      if (!S.trail.length) S.trail = [PAGE];
      if (!M.P[S.centre] || !vis(S.centre)) S.centre = S.trail[S.trail.length - 1];
      if (S.sel && !(M.P[S.sel] && vis(S.sel))) S.sel = null;
    }
    function settle(d, res) {
      var j = res && res.j;
      if (!j || !j.tree) {
        // Refused before anything was written, or no answer at all.
        E.note = res ? (j && j.message ? j.message : 'The save was refused.') + ' Your changes are still here.'
          : 'Could not confirm the save. Reload the page to see what saved; your changes are still here.';
        E.noteBad = true; updateBar(); say(E.note);
        return;
      }
      (j.created || []).forEach(function (c) {
        renameId(M, c.ref, c.id);
        if (M.P[c.id] && pagesUrl) M.P[c.id].url = pagesUrl + c.id;
        E.hist = E.hist.map(function (h) { return h.split(JSON.stringify(c.ref)).join(JSON.stringify(c.id)); });
        S.trail = S.trail.map(function (x) { return x === c.ref ? c.id : x; });
        if (S.centre === c.ref) S.centre = c.id;
        if (S.sel === c.ref) S.sel = null;
      });
      data = j.tree;
      var truth = buildModel(j.tree);
      BASE = cloneModel(truth);
      if (j.saved) {
        M = truth; E.hist = [];
        E.note = 'Saved. The family is updated on this page.'; E.noteBad = false;
      } else {
        E.note = saveReport(M, d.edits, j); E.noteBad = true;
      }
      reconcileState();
      renderStrip(); renderTrail(); renderDock(); updateBar(); render({ glide: true });
      say(j.saved ? 'Saved' : 'Some changes did not save');
    }

    // ---- Open and close: the container transform ----
    function relA(r) { var a = anchor.getBoundingClientRect(); return { l: r.left - a.left, t: r.top - a.top, w: r.width, h: r.height }; }
    function bgOf(el) {
      var c = getComputedStyle(el).backgroundColor;
      if (c && c !== 'rgba(0, 0, 0, 0)' && c !== 'transparent') return c;
      var n = root.parentElement;
      while (n) { var b = getComputedStyle(n).backgroundColor; if (b && b !== 'rgba(0, 0, 0, 0)' && b !== 'transparent') return b; n = n.parentElement; }
      return '#fff';
    }
    function openerGeom() {
      var pic = opener.id ? strip.querySelector('.fm-hs-it[data-id="' + opener.id + '"] .fm-pic') : null;
      if (pic) { var r = pic.getBoundingClientRect(), g = relA(r); g.r = r.width / 2; g.bg = bgOf(pic); g.sh = 'none'; return g; }
      var r2 = strip.getBoundingClientRect(), g2 = relA(r2), cs = getComputedStyle(strip);
      g2.r = parseFloat(cs.borderTopLeftRadius) || 0; g2.bg = bgOf(strip); g2.sh = cs.boxShadow;
      return g2;
    }
    function kfOf(g) { return { left: g.l + 'px', top: g.t + 'px', width: g.w + 'px', height: g.h + 'px', borderRadius: g.r + 'px', backgroundColor: g.bg, boxShadow: g.sh }; }
    // The box itself runs from the opener's rect, radius and fill to its own; its content fades in after 40%.
    function morphAnim(box, conts, from, to, fwd, done) {
      var a = box.animate([kfOf(from), kfOf(to)], { duration: DUR, easing: EASE, fill: 'both' }), list = [a];
      var op = fwd ? [{ opacity: 0, offset: 0 }, { opacity: 0, offset: 0.4 }, { opacity: 1, offset: 1 }] : [{ opacity: 1, offset: 0 }, { opacity: 0, offset: 0.4 }, { opacity: 0, offset: 1 }];
      conts.forEach(function (c) { list.push(c.animate(op, { duration: DUR, easing: 'linear', fill: 'both' })); });
      a.onfinish = function () { list.forEach(function (x) { try { x.cancel(); } catch (e) { /* already finished */ } }); done(); };
      return list;
    }
    function stopAll() { anims.forEach(function (a) { try { a.cancel(); } catch (e) { /* already finished */ } }); anims = []; }
    function snap() { if (curDone) { var f = curDone; curDone = null; stopAll(); f(); } }
    function sheetGeom() { var cs = getComputedStyle(sheet); return { l: 0, t: 0, w: sheet.offsetWidth, h: sheet.offsetHeight, r: parseFloat(cs.borderTopLeftRadius) || 0, bg: cs.backgroundColor, sh: cs.boxShadow }; }
    function playSheet(on, from) {
      var lv = level(), to = sheetGeom();
      morphing = true; hdc.style.width = to.w + 'px';
      function done() { curDone = null; stopAll(); morphing = false; hdc.style.width = ''; if (!on) finishClose(); }
      curDone = done;
      if (lv === 'calm') {
        var a = sheet.animate(on ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 1 }, { opacity: 0 }], { duration: on ? 200 : 150, fill: 'both' });
        anims = [a]; a.onfinish = done; return;
      }
      anims = morphAnim(sheet, [hdc], on ? from : to, on ? to : from, on, done);
    }
    function finishClose() {
      sheet.classList.remove('fm-show');
      Object.keys(els).forEach(function (k) { els[k].remove(); }); els = {};
      body.classList.remove('fm-docked', 'fm-dk-keep'); dkStop();
      strip.classList.remove('fm-away');
      if (afterClose) { afterClose = false; openBtn.focus(); }
    }
    function onKey(e) {
      if (e.key !== 'Escape' || !root.isConnected) return;
      if (peek.classList.contains('fm-on')) hidePeek();
      if (!S.open) return;
      if (E.pick) { closePick(true); return; }
      if (S.sel) { deselect(); return; }
      setOpen(false, null, null, false, true);
    }
    function setOpen(on, fromEl, selId, viaKey, refocus) {
      if (on === S.open) return;
      snap();
      S.open = on; openBtn.setAttribute('aria-expanded', on ? 'true' : 'false'); hidePeek(); closePick();
      if (on) {
        var item = fromEl && fromEl.closest ? fromEl.closest('.fm-hs-it[data-id]') : null;
        opener = { id: item ? item.getAttribute('data-id') : null };
        var from = level() === 'off' ? null : openerGeom();
        sheet.classList.add('fm-show'); renderTrail();
        S.sel = null; if (selId && selId !== S.centre) S.sel = selId;
        render({ first: true }); renderDock(); render({ first: true }); scrollToCentre();
        strip.classList.add('fm-away');
        if (from) playSheet(true, from);
        say('Family tree open');
        document.addEventListener('keydown', onKey);
        if (viaKey) setTimeout(function () { closeBtn.focus(); }, 0);
      } else {
        document.removeEventListener('keydown', onKey);
        S.hov = null; say('Family tree closed'); afterClose = !!refocus; S.sel = null;
        if (level() === 'off') { finishClose(); return; }
        var to = openerGeom();
        if (level() === 'full') setTimeout(function () { strip.classList.remove('fm-away'); }, DUR * 0.55);
        playSheet(false, to);
      }
    }

    // ---- Size, and the Director's preview as a player ----
    function relay() {
      var phone = root.clientWidth > 0 && root.clientWidth < PHONE_W;
      if (phone !== S.phone) { S.phone = phone; root.classList.toggle('fm-phone', phone); }
      if (!S.open) return;
      if (!tv.clientWidth) { pendingLay = true; return; }
      if (pendingLay || Math.abs(tv.clientWidth - lastW) > 1) { var first = pendingLay; render({ first: first }); if (first) scrollToCentre(); }
    }
    var ro = null, mo = null;
    if (window.ResizeObserver) { ro = new ResizeObserver(relay); ro.observe(root); ro.observe(tv); }
    // Taking the Director's eye off keeps a hidden relative out of the strip, the trail and the card at once.
    function onPreview() {
      if (!dm()) { E.on = false; closePick(); }
      editUi();
      S.trail = S.trail.filter(vis);
      if (!S.trail.length) S.trail = [PAGE];
      if (!vis(S.centre)) S.centre = S.trail[S.trail.length - 1];
      if (S.sel && !vis(S.sel)) S.sel = null;
      renderStrip(); renderTrail(); renderDock(); render({ glide: true });
    }
    if (window.MutationObserver) { mo = new MutationObserver(onPreview); mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-view-as-player'] }); }

    // ---- What the shell's buttons call ----
    self.toggle = function (el, e) { setOpen(!S.open, openBtn, null, !!e && e.detail === 0); };
    self.close = function () { setOpen(false, null, null, false, true); };
    self.edit = function () {
      if (!dm() || !editUrl) return;
      E.on = !E.on; closePick();
      if (!E.on && !E.noteBad && !pendingCount()) E.note = '';
      editUi(); render({ glide: true }); say(E.on ? 'Editing the family' : 'Editing done');
    };
    self.undo = undoEdit;
    self.save = save;
    self.siblings = function () {
      S.sib = !S.sib; sibBtn.setAttribute('aria-pressed', S.sib ? 'true' : 'false'); render({ glide: true });
    };
    self.destroy = function () {
      document.removeEventListener('keydown', onKey);
      if (ro) ro.disconnect();
      if (mo) mo.disconnect();
      clearTimeout(peekT); clearTimeout(linkTimer);
      closePick();
    };

    relay();
    renderStrip();
    editUi();
  }

  function instanceOf(el) { var r = rootOf(el); return r && r.__fm; }

  var API = {
    toggle: function (el, e) { var f = instanceOf(el); if (f) f.toggle(el, e); },
    close: function (el) { var f = instanceOf(el); if (f) f.close(el); },
    siblings: function (el) { var f = instanceOf(el); if (f) f.siblings(el); },
    edit: function (el) { var f = instanceOf(el); if (f) f.edit(el); },
    undo: function (el) { var f = instanceOf(el); if (f) f.undo(el); },
    save: function (el) { var f = instanceOf(el); if (f) f.save(el); }
  };
  if (typeof window !== 'undefined') window.ChronicleFamilyTree = API;

  // The block mounts as a widget so its script loads on sight (ADR-063) and
  // draws the strip once it is there; the shell's buttons stay inline.
  if (typeof window !== 'undefined' && window.Chronicle && window.Chronicle.register) {
    window.Chronicle.register('family-tree', {
      init: function (el) {
        if (el.__fm) return;
        try { el.__fm = new Family(el); } catch (err) { if (window.console) console.error('[family-tree]', err); }
      },
      destroy: function (el) { if (el.__fm) { el.__fm.destroy(); el.__fm = null; } }
    });
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      motionOf: motionOf, hueOf: hueOf, initialsOf: initialsOf, safeUrl: safeUrl, relLabel: relLabel, rowLabel: rowLabel,
      buildModel: buildModel, visible: visible, parentsOf: parentsOf, kidsOf: kidsOf, partnersOf: partnersOf, sibsOf: sibsOf,
      reachable: reachable, layoutTree: layoutTree, pathD: pathD, pickPeekSpot: pickPeekSpot,
      cloneModel: cloneModel, addPerson: addPerson, newPersonId: newPersonId, personFromCandidate: personFromCandidate,
      addParentEdge: addParentEdge, removeParentEdge: removeParentEdge, addPartnerPair: addPartnerPair,
      removePartnerPair: removePartnerPair, ancestorsOf: ancestorsOf, descendantsOf: descendantsOf,
      pickerExcluded: pickerExcluded, diffModels: diffModels, renameId: renameId, describeEdit: describeEdit,
      saveReport: saveReport
    };
  }
})();
