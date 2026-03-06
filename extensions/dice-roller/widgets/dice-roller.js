/**
 * Dice Roller Widget — Chronicle Extension
 *
 * A simple dice roller that registers as a Chronicle widget via
 * Chronicle.register(). Mounts to elements with data-widget="dice-roller".
 *
 * Supports standard TTRPG dice: d4, d6, d8, d10, d12, d20, d100.
 * Rolls are displayed inline with a brief history.
 */
(function () {
  'use strict';

  var DICE = [
    { label: 'd4', sides: 4 },
    { label: 'd6', sides: 6 },
    { label: 'd8', sides: 8 },
    { label: 'd10', sides: 10 },
    { label: 'd12', sides: 12 },
    { label: 'd20', sides: 20 },
    { label: 'd100', sides: 100 }
  ];

  var MAX_HISTORY = 10;

  function roll(sides) {
    return Math.floor(Math.random() * sides) + 1;
  }

  Chronicle.register('dice-roller', {
    init: function (el) {
      var history = [];

      // Build UI.
      var container = document.createElement('div');
      container.className = 'dice-roller-widget p-4 bg-surface-raised rounded-lg border border-edge';

      // Header.
      var header = document.createElement('div');
      header.className = 'flex items-center gap-2 mb-3';
      header.innerHTML = '<i class="fa-solid fa-dice-d20 text-accent"></i><span class="font-semibold text-fg">Dice Roller</span>';
      container.appendChild(header);

      // Dice buttons row.
      var btnRow = document.createElement('div');
      btnRow.className = 'flex flex-wrap gap-1.5 mb-3';

      DICE.forEach(function (d) {
        var btn = document.createElement('button');
        btn.className = 'px-2.5 py-1.5 text-xs font-medium bg-surface border border-edge rounded hover:bg-surface-hover hover:border-accent transition-colors text-fg';
        btn.textContent = d.label;
        btn.addEventListener('click', function () {
          var result = roll(d.sides);
          addResult(d.label, result, d.sides);
        });
        btnRow.appendChild(btn);
      });
      container.appendChild(btnRow);

      // Result display.
      var resultDiv = document.createElement('div');
      resultDiv.className = 'text-center py-3 mb-3 bg-surface rounded border border-edge min-h-[3rem] flex items-center justify-center';
      resultDiv.innerHTML = '<span class="text-fg-muted text-sm italic">Click a die to roll</span>';
      container.appendChild(resultDiv);

      // History list.
      var historyDiv = document.createElement('div');
      historyDiv.className = 'space-y-1 max-h-32 overflow-y-auto';
      container.appendChild(historyDiv);

      function addResult(diceLabel, result, sides) {
        // Highlight nat 1 and nat max.
        var isNat1 = result === 1;
        var isNatMax = result === sides;
        var colorClass = isNat1 ? 'text-red-500' : isNatMax ? 'text-green-500' : 'text-fg';

        resultDiv.innerHTML = '<span class="text-2xl font-bold ' + colorClass + '">' + result + '</span>' +
          '<span class="text-fg-muted text-sm ml-2">(' + diceLabel + ')</span>';

        // Add to history.
        history.unshift({ dice: diceLabel, result: result, isNat1: isNat1, isNatMax: isNatMax });
        if (history.length > MAX_HISTORY) history.pop();

        // Render history.
        historyDiv.innerHTML = '';
        history.forEach(function (h) {
          var row = document.createElement('div');
          row.className = 'flex justify-between text-xs text-fg-muted px-2 py-0.5';
          var rClass = h.isNat1 ? 'text-red-400' : h.isNatMax ? 'text-green-400' : '';
          row.innerHTML = '<span>' + h.dice + '</span><span class="font-medium ' + rClass + '">' + h.result + '</span>';
          historyDiv.appendChild(row);
        });
      }

      el.appendChild(container);
      el._diceRollerContainer = container;
    },

    destroy: function (el) {
      if (el._diceRollerContainer) {
        el._diceRollerContainer.remove();
        delete el._diceRollerContainer;
      }
    }
  });
})();
