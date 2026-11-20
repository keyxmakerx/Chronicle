// rules_page.test.mjs — the rules list's name filter. Rows whose data-find
// does not hold the typed text are hidden, and the count of rows left is what
// decides whether "Nothing matches" shows.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '..', '..', 'static', 'js', 'widgets', 'rules_page.js'), 'utf8');
const sandbox = { window: {} };
vm.runInNewContext(src, sandbox);
const { filterRows } = sandbox.window.ChronicleRulesPage;

function row(find) {
  return { hidden: false, getAttribute: (k) => (k === 'data-find' ? find : null) };
}

test('filterRows hides the rows that do not hold the text', () => {
  const cases = [
    ['empty shows all', '', [true, true, true], 3],
    ['spaces show all', '   ', [true, true, true], 3],
    ['case does not matter', 'ASH', [true, false, false], 1],
    ['matches the one line too', 'sea', [false, true, false], 1],
    ['nothing matches', 'zzz', [false, false, false], 0],
  ];
  for (const [name, q, shown, count] of cases) {
    const rows = [row('ashborn born of fire'), row('elf of the sea'), row('dwarf')];
    assert.equal(filterRows(rows, q), count, name);
    assert.deepEqual(rows.map((r) => !r.hidden), shown, name);
  }
});
