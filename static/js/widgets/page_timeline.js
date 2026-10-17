/**
 * page_timeline.js -- the time line block's hover card.
 *
 * The server draws the line and its dots (internal/plugins/calendar/
 * page_timeline.templ), already filtered for the viewer. This only opens
 * the shared hover card (hovercard.js) on a dot, from what the dot carries,
 * so the card behaves like every other one: after a 250ms rest, on keyboard
 * focus, closed by Escape. A click follows the dot to its day on the
 * calendar.
 */
(function () {
  'use strict';

  // The hover card content for one dot.
  function contentFor(dot) {
    var facts = [['Date', dot.getAttribute('data-ptl-date') || '']];
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

  Chronicle.register('page_timeline', {
    init: function (el) {
      var hc = window.Chronicle && Chronicle.hovercard;
      if (!hc) return;
      el._ptlUnbind = hc.bind(el, '.ptl-ev', contentFor);
    },
    destroy: function (el) {
      if (el._ptlUnbind) el._ptlUnbind();
      el._ptlUnbind = null;
    }
  });
})();
