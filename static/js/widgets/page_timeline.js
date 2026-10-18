/**
 * page_timeline.js -- the time line block's merged beads and hover card.
 *
 * The server draws the lean line and a bead per event (internal/plugins/
 * calendar/page_timeline.templ), already filtered for the viewer. This
 * merges beads that would touch, since how close they sit depends on the
 * width the line gets: a run of beads closer than a bead's width becomes one
 * bead with a second ring, placed in the middle of the run, whose hover card
 * lists them all. Years are relabelled to suit. It reflows on resize.
 *
 * The card is the shared hover card (hovercard.js), so it behaves like every
 * other one: after a 250ms rest, on keyboard focus, closed by Escape. A click
 * follows a bead to its day on the calendar. The owner picks how it is drawn
 * (Customize › Hover cards › Popups on time lines): the classic card, or the
 * fading corner when <html> carries data-cz-tlpop="corner".
 */
(function () {
  'use strict';

  // Beads closer than this many pixels share one bead.
  var BEAD = 14;
  // Two year labels must be at least this many pixels apart to stay readable.
  var YEAR_GAP = 40;

  function fact(dot) {
    var d = dot.getAttribute('data-ptl-date') || '', end = dot.getAttribute('data-ptl-end');
    return end ? d + ' to ' + end : d;
  }

  // The hover card content for one bead, or for every event it stands for.
  function contentFor(dot) {
    var li = dot.parentNode, group = li._ptlGroup;
    if (group && group.length > 1) {
      return {
        kind: 'Time line',
        kindIcon: 'fa-layer-group',
        title: group.length + ' events',
        facts: group.map(function (g) {
          return [fact(g), (g.getAttribute('data-ptl-name') || '') + (g.hasAttribute('data-dm-only') ? ' (Director only)' : '')];
        }),
        link: { href: dot.getAttribute('href') || '', label: 'Open on the calendar' }
      };
    }
    var facts = [[dot.hasAttribute('data-ptl-end') ? 'Dates' : 'Date', fact(dot)]];
    if (dot.hasAttribute('data-ptl-repeats')) facts.push(['Repeats', 'From this date']);
    return {
      kind: dot.getAttribute('data-ptl-kind') || 'Event',
      kindIcon: (dot.getAttribute('data-ptl-icon') || 'fa-calendar-day').replace(/^fa-solid\s+/, ''),
      title: dot.getAttribute('data-ptl-name') || '',
      facts: facts,
      foot: dot.hasAttribute('data-dm-only') ? 'Director only' : '',
      link: { href: dot.getAttribute('href') || '', label: 'Open on the calendar' }
    };
  }

  // Groups the beads for the line's current width and relabels the years.
  function layout(el) {
    var inner = el.querySelector('.ptl-in'), items = Array.prototype.slice.call(el.querySelectorAll('.ptl-dots > li'));
    if (!inner || !items.length) return;
    var w = Math.max(1, inner.clientWidth), groups = [];
    items.forEach(function (li) {
      var at = parseFloat(li.getAttribute('data-ptl-at')) || 0, g = groups[groups.length - 1];
      if (g && (at - g.last) / 100 * w < BEAD) { g.items.push(li); g.last = at; } else groups.push({ items: [li], first: at, last: at });
    });
    var lastLab = -100, lastYear = null;
    groups.forEach(function (g, i) {
      var head = g.items[0], dot = head.querySelector('.ptl-ev'), at = (g.first + g.last) / 2, dots = g.items.map(function (li) { return li.querySelector('.ptl-ev'); });
      g.items.forEach(function (li, j) { li.hidden = j > 0; });
      head.style.left = at.toFixed(2) + '%';
      head._ptlGroup = dots;
      // A merged bead is amber or hollow only when every event on it is.
      var multi = dots.length > 1, every = function (attr) { return dots.every(function (d) { return d.hasAttribute(attr); }); };
      dot.classList.toggle('ptl-ev--multi', multi);
      dot.classList.toggle('ptl-ev--dm', every('data-dm-only'));
      dot.classList.toggle('ptl-ev--later', every('data-ptl-later'));
      dot.classList.toggle('ptl-ev--death', dots.some(function (d) { return d.hasAttribute('data-ptl-death'); }));
      dot.setAttribute('aria-label', dots.map(function (d) { return d.getAttribute('data-ptl-label'); }).join('; '));
      var year = head.getAttribute('data-ptl-year'), yr = dot.querySelector('.ptl-yr');
      var lab = (i === 0 || year !== lastYear) && (at - lastLab) / 100 * w >= YEAR_GAP;
      if (lab) {
        lastLab = at; lastYear = year;
        if (!yr) { yr = document.createElement('span'); yr.className = 'ptl-yr'; yr.setAttribute('aria-hidden', 'true'); dot.appendChild(yr); }
        yr.textContent = year;
      } else if (yr) yr.remove();
    });
  }

  Chronicle.register('page_timeline', {
    init: function (el) {
      // The server's classes and label are each bead's own; keep them so a
      // bead can leave a merged group when the line widens.
      el.querySelectorAll('.ptl-ev').forEach(function (d) {
        if (d.classList.contains('ptl-ev--later')) d.setAttribute('data-ptl-later', '');
        if (d.classList.contains('ptl-ev--death')) d.setAttribute('data-ptl-death', '');
        d.setAttribute('data-ptl-label', d.getAttribute('aria-label') || '');
      });
      layout(el);
      if (window.ResizeObserver) {
        var w = 0;
        el._ptlRO = new ResizeObserver(function () { if (el.clientWidth !== w) { w = el.clientWidth; layout(el); } });
        el._ptlRO.observe(el);
      }
      var hc = window.Chronicle && Chronicle.hovercard;
      var corner = document.documentElement.getAttribute('data-cz-tlpop') === 'corner';
      if (hc) el._ptlUnbind = hc.bind(el, '.ptl-ev', contentFor, { corner: corner });
    },
    destroy: function (el) {
      if (el._ptlUnbind) el._ptlUnbind();
      if (el._ptlRO) el._ptlRO.disconnect();
      el._ptlUnbind = null; el._ptlRO = null;
    }
  });
})();
