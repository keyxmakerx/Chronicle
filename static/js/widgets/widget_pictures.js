/**
 * widget_pictures.js: the small drawings the Customizer's widget list shows
 * beside each widget's name. Each is a wireframe of what the widget puts on a
 * page, drawn on one 56 by 40 grid with one ink and one accent mark, so the
 * long list reads as one family of pictures. The kinds are the server's
 * (internal/plugins/entities/block_pictures.go); an unknown or missing kind
 * draws "lines", as a package widget does.
 *
 * Classes the stylesheet colours from tokens: i ink, f faint ink, a accent,
 * s an ink stroke, sa an accent stroke, b the frame's own ground.
 */
(function () {
  'use strict';

  function bar(x, y, w, h, c) { return '<rect class="' + (c || 'i') + '" x="' + x + '" y="' + y + '" width="' + w + '" height="' + (h || 3) + '" rx="' + ((h || 3) / 2) + '"/>'; }
  function box(x, y, w, h, c, r) { return '<rect class="' + c + '" x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" rx="' + (r == null ? 2 : r) + '"/>'; }
  function dot(x, y, r, c) { return '<circle class="' + c + '" cx="' + x + '" cy="' + y + '" r="' + r + '"/>'; }
  function line(x1, y1, x2, y2, c) { return '<path class="' + (c || 's') + '" d="M' + x1 + ' ' + y1 + 'L' + x2 + ' ' + y2 + '"/>'; }

  var DRAW = {
    heading: function () { return bar(8, 11, 32, 6) + bar(8, 21, 18, 2, 'a') + bar(8, 27, 26, 3, 'f'); },
    lines: function () { return bar(8, 10, 40) + bar(8, 16.5, 40) + bar(8, 23, 40) + bar(8, 29.5, 26); },
    quote: function () { return bar(8, 9, 2.5, 22, 'a') + bar(14, 11, 34) + bar(14, 17.5, 34) + bar(14, 24, 24); },
    image: function () {
      return box(8, 7, 40, 26, 'f', 3) + '<path class="b" d="M8 30L19 19L27 26L33 21L48 31V33H8Z"/>' + dot(39, 14, 3, 'a');
    },
    card: function () { return box(10, 6, 36, 15, 'f', 3) + dot(17, 13.5, 2.5, 'a') + bar(10, 25, 24) + bar(10, 31, 32, 2.5, 'f'); },
    facts: function () { return bar(8, 11, 9, 3, 'f') + bar(21, 11, 27, 3, 'a') + bar(8, 18.5, 9, 3, 'f') + bar(21, 18.5, 20) + bar(8, 26, 9, 3, 'f') + bar(21, 26, 24); },
    sheet: function () {
      return box(8.75, 7.75, 10.5, 12.5, 's') + box(22.75, 7.75, 10.5, 12.5, 's') + box(36.75, 7.75, 10.5, 12.5, 's') +
        bar(11, 12.5, 6, 3, 'a') + bar(25, 12.5, 6) + bar(39, 12.5, 6) + bar(8, 25, 40, 3, 'f') + bar(8, 31, 28, 3, 'f');
    },
    links: function () { return dot(11, 12, 2.5, 'a') + bar(17, 10.5, 30) + dot(11, 20, 2.5, 'f') + bar(17, 18.5, 24) + dot(11, 28, 2.5, 'f') + bar(17, 26.5, 27); },
    grid: function () {
      return box(8, 8, 11.5, 11, 'a', 2) + box(22.25, 8, 11.5, 11, 'f', 2) + box(36.5, 8, 11.5, 11, 'f', 2) +
        box(8, 22, 11.5, 11, 'f', 2) + box(22.25, 22, 11.5, 11, 'f', 2) + box(36.5, 22, 11.5, 11, 'f', 2);
    },
    feed: function () {
      return dot(11, 11.5, 3, 'a') + bar(17, 9, 26) + bar(17, 14, 18, 2.5, 'f') + line(8, 20, 48, 20, 's thin') +
        dot(11, 27.5, 3, 'f') + bar(17, 25, 22) + bar(17, 30, 26, 2.5, 'f');
    },
    tree: function () {
      return line(28, 13, 28, 20) + line(17, 20, 39, 20) + line(17, 20, 17, 27) + line(39, 20, 39, 27) +
        dot(28, 10, 3.5, 'a') + dot(17, 30, 3.5, 'i') + dot(39, 30, 3.5, 'i');
    },
    graph: function () {
      return line(28, 20, 13, 11) + line(28, 20, 43, 10) + line(28, 20, 14, 31) + line(28, 20, 44, 29) +
        dot(13, 11, 2.5, 'i') + dot(43, 10, 2.5, 'i') + dot(14, 31, 2.5, 'i') + dot(44, 29, 2.5, 'i') + dot(28, 20, 4, 'a');
    },
    map: function () {
      return box(8, 7, 40, 26, 'f', 3) + '<path class="b-s" d="M8 26C16 22 22 30 31 25S43 19 48 22"/>' +
        '<path class="a" d="M30 10.5a4.5 4.5 0 0 1 4.5 4.5c0 3.3-4.5 7.5-4.5 7.5s-4.5-4.2-4.5-7.5a4.5 4.5 0 0 1 4.5-4.5Z"/>' + dot(30, 15, 1.6, 'b');
    },
    month: function () {
      var h = bar(8, 7, 40, 3.5), r, c;
      for (r = 0; r < 4; r++) for (c = 0; c < 7; c++) h += box(8 + c * 6, 14 + r * 6, 4, 4, r === 2 && c === 4 ? 'a' : 'f', 1);
      return h;
    },
    track: function () {
      return line(8, 22, 48, 22) + dot(13, 22, 2, 'i') + dot(23, 22, 2, 'i') + dot(43, 22, 2, 'i') + dot(34, 22, 3.5, 'a') +
        bar(28, 12, 12, 3) + bar(9, 28, 10, 2.5, 'f') + bar(37, 28, 11, 2.5, 'f');
    },
    table: function () {
      return box(8, 8, 40, 6, 'f', 1.5) + line(8, 21, 48, 21, 's thin') + line(8, 28, 48, 28, 's thin') + line(22, 14, 22, 33, 's thin') +
        bar(10, 16.5, 9, 2.5, 'a') + bar(25, 16.5, 18, 2.5) + bar(10, 23.5, 9, 2.5) + bar(25, 23.5, 14, 2.5) + bar(10, 30, 9, 2.5) + bar(25, 30, 20, 2.5);
    },
    counter: function () {
      return '<circle class="s ring" cx="19" cy="20" r="9"/><path class="sa ring" d="M19 11a9 9 0 0 1 8.56 11.78"/>' +
        bar(15, 18.5, 8, 3) + bar(33, 14, 15, 3.5) + bar(33, 22, 10, 3, 'f');
    },
    steps: function () {
      return box(8, 8, 6, 6, 'a', 1.5) + '<path class="b-s" d="M9.6 11.1l1.5 1.5 2.4-2.8"/>' + bar(18, 9.5, 26) +
        box(8.75, 17.75, 4.5, 4.5, 's', 1.2) + bar(18, 18.5, 22) + box(8.75, 26.75, 4.5, 4.5, 's', 1.2) + bar(18, 27.5, 28);
    },
    tags: function () {
      return box(8, 10, 14, 7, 'a', 3.5) + box(25.75, 10.75, 16.5, 5.5, 's', 2.75) + box(8.75, 22.75, 19.5, 5.5, 's', 2.75) + box(32.75, 22.75, 11.5, 5.5, 's', 2.75);
    },
    divider: function () { return bar(8, 10, 32, 3, 'f') + bar(8, 19, 40, 1.5, 'a') + bar(8, 27, 26, 3, 'f'); },
    search: function () {
      return box(8.75, 12.75, 38.5, 14.5, 's', 4) + '<circle class="sa" cx="16" cy="19.5" r="3"/>' + line(18.2, 21.7, 20.5, 24, 'sa') + bar(24, 18.5, 17, 2.5, 'f');
    }
  };

  // widgetPicture returns the drawing for a kind as an inline SVG string. The
  // kind only ever selects one of the fixed drawings above.
  function widgetPicture(kind) {
    var draw = Object.prototype.hasOwnProperty.call(DRAW, kind) ? DRAW[kind] : DRAW.lines;
    return '<svg class="wpic" viewBox="0 0 56 40" width="56" height="40" aria-hidden="true" focusable="false">' + draw() + '</svg>';
  }

  window.Chronicle = window.Chronicle || {};
  window.Chronicle.widgetPicture = widgetPicture;
  window.Chronicle._widgetPictureKinds = Object.keys(DRAW);
})();
