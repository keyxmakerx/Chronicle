/**
 * table_leaf.js — a box on the players' screen (data-widget="table-leaf",
 * data-tt-mount="rolls|tonight|dice") that the Table draws one of its own
 * sections into, so those leaves have one implementation: table.js, loaded
 * ahead of this file, does the drawing through Chronicle.tableLeaf. The
 * screen mounts its cards' widgets when a card is first shown and tears them
 * down when it redraws.
 */
(function () {
  'use strict';
  var C = window.Chronicle;
  if (!C || !C.register) return;
  C.register('table-leaf', {
    init: function (el) { if (C.tableLeaf) C.tableLeaf.mount(el); },
    destroy: function (el) { if (C.tableLeaf) C.tableLeaf.unmount(el); }
  });
})();
