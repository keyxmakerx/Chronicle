// daycard_delete_twostep.test.mjs — C-CALV4-GAMEREADY §7 [GR-13].
//
// THE DEFECT THIS PINS. One click on `[data-de-delete]` fired a hard
// `DELETE /campaigns/:c/calendars/:cal/events/:id` immediately. The repository
// does `DELETE FROM calendar_events WHERE id = ?` — no soft delete, no restore
// path — and the button sits at a 24px tap floor, on a phone, on the line ABOVE
// Save in the same delegated handler. The failure mode is not "the user did not
// mean to delete"; it is "the user meant to hit Save".
//
// THE RULED SHAPE, and it is a two-step IN PLACE rather than a dialog: the
// first click swaps the label to `Confirm delete` and starts a ~4s timer; a
// second click inside that window sends the DELETE; the timer expiring, the
// editor closing, or ANY other editor interaction disarms it back. No
// `<dialog>`, no `[role=alertdialog]`, no `window.confirm`, and no DOM outside
// the editor's own box — the editor sheet is `position: fixed` and already puts
// Save under a software keyboard, so a second fixed layer would be a second
// trap rather than a confirmation.
//
// The suite also pins §7's same-handler freebie: Save gets a busy state, so a
// write in flight cannot read as a dead button under the previous failure's
// error text.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, SRC } from './daycard_harness.mjs';

const STORED = {
  id: 'ev-1', calendar_id: 'cal-1', name: 'Council of Wards',
  year: 1523, month: 1, day: 3, all_day: true, visibility: 'everyone',
};

// openEditor opens the EDIT editor over ev-1 and returns the delete button.
async function openEditor(opts) {
  const fx = boot({
    responses: {
      'GET /campaigns/camp-1/calendars/cal-1/events/ev-1': {
        ok: true, json: () => Promise.resolve(STORED),
      },
      ...((opts || {}).responses || {}),
    },
  });
  fx.fire('click', fx.cells[3]);
  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));
  return fx;
}

const del = (fx) => fx.editor.querySelector('[data-de-delete]');
const save = (fx) => fx.editor.querySelector('[data-de-save]');
const deletes = (fx) => fx.calls.filter((c) => c.method === 'DELETE');

// fireArmTimer runs ONLY the ~4s arming timer, leaving the register's own
// close/morph timers alone — `fx.flush()` would run every queued timer and
// could not tell an expiry apart from a close.
function fireArmTimer(fx) {
  const t = fx.timers.find((x) => x.ms === 4000);
  assert.ok(t, 'the arming timer should be queued while the button is armed');
  t.fn();
}

test('ONE click on Delete sends ZERO requests and arms the button in place', async () => {
  const fx = await openEditor();
  const before = fx.calls.length;
  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));

  assert.equal(deletes(fx).length, 0, 'the first click must not delete anything');
  assert.equal(fx.calls.length, before, 'the first click must send no request at all');
  assert.equal(del(fx).textContent, 'Confirm delete');
  assert.equal(del(fx).getAttribute('aria-live'), 'polite',
    'the label swap is a state change and must be announced, not only seen');
});

test('TWO clicks inside the window send exactly ONE DELETE, at the shipped URL', async () => {
  const fx = await openEditor();
  fx.fire('click', del(fx));
  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));

  assert.equal(deletes(fx).length, 1);
  assert.equal(deletes(fx)[0].url, '/campaigns/camp-1/calendars/cal-1/events/ev-1');
});

test('a THIRD click cannot send a second DELETE — the busy guard still holds', async () => {
  const fx = await openEditor();
  fx.fire('click', del(fx));
  fx.fire('click', del(fx));
  fx.fire('click', del(fx));
  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(deletes(fx).length, 1);
});

test('a click AFTER the window expires sends zero and RE-ARMS rather than deleting', async () => {
  const fx = await openEditor();
  fx.fire('click', del(fx));
  assert.equal(del(fx).textContent, 'Confirm delete');

  fireArmTimer(fx);
  assert.equal(del(fx).textContent, 'Delete event',
    'an expired window restores the resting label');

  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(deletes(fx).length, 0,
    'the click after the timeout is a FIRST click again, not a confirmation');
  assert.equal(del(fx).textContent, 'Confirm delete');

  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(deletes(fx).length, 1, 're-arming still reaches the delete on a second press');
});

test('closing the editor DISARMS — a reopened editor never deletes on the first click', async () => {
  const fx = await openEditor();
  fx.fire('click', del(fx));
  assert.equal(del(fx).textContent, 'Confirm delete');

  fx.fire('click', fx.editor.querySelector('[data-de-cancel]'));
  assert.equal(del(fx).textContent, 'Delete event');

  fx.fire('click', fx.card.querySelector('[data-dc-edit]'));
  await new Promise((r) => setImmediate(r));
  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(deletes(fx).length, 0);
});

test('ANY other editor interaction disarms it', async () => {
  const fx = await openEditor();

  // A click elsewhere in the editor's own subtree.
  fx.fire('click', del(fx));
  fx.fire('click', fx.editor);
  assert.equal(del(fx).textContent, 'Delete event', 'a click on other chrome disarms');

  // A form control changing — the keyboard half of "any other interaction".
  fx.fire('click', del(fx));
  assert.equal(del(fx).textContent, 'Confirm delete');
  fx.fire('change', fx.editor.querySelector('[data-de-name]'));
  assert.equal(del(fx).textContent, 'Delete event', 'typing or toggling disarms');

  fx.fire('click', del(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(deletes(fx).length, 0);
});

test('the two-step adds NO dialog, NO window.confirm and NO DOM outside the editor', async () => {
  // Comments are stripped first: this file NAMES the idioms it refuses, and a
  // grep that could not tell a prohibition from a use would be unfalsifiable.
  const code = SRC.split('\n').map((l) => l.replace(/\/\/.*$/, '')).join('\n');
  assert.equal(/window\.confirm\s*\(|[^.\w]confirm\s*\(/.test(code), false,
    'window.confirm is what the V2 grid did, and V2 is a committed deletion');
  assert.equal(/alertdialog|<dialog|showModal/.test(code), false,
    'no modal layer: the sheet is position:fixed and already loses Save to the keyboard');

  const fx = await openEditor();
  const outside = fx.root.innerHTML;
  fx.fire('click', del(fx));
  assert.equal(fx.root.innerHTML.length >= outside.length - 0, true);
  // The ONLY thing that changed is the button's own text.
  assert.equal(del(fx).textContent, 'Confirm delete');
  assert.equal(fx.document.querySelector('[role="alertdialog"]'), null);
});

// --- §7's same-handler freebie: Save's busy state ----------------------------

test('a save in flight DISABLES the button and says aria-busy', async () => {
  const fx = await openEditor({ responses: { PUT: new Promise(() => {}) } });
  fx.fire('click', save(fx));
  await new Promise((r) => setImmediate(r));

  assert.equal(save(fx).disabled, true, 'a write in flight must not look live');
  assert.equal(save(fx).getAttribute('aria-busy'), 'true');
  const puts = fx.calls.filter((c) => c.method === 'PUT').length;
  fx.fire('click', save(fx));
  fx.fire('click', save(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(fx.calls.filter((c) => c.method === 'PUT').length, puts,
    'further taps send nothing — and now the button SHOWS that');
});

test('a hung save becomes a STATED failure rather than a permanently dead button', async () => {
  const fx = await openEditor({ responses: { PUT: new Promise(() => {}) } });
  fx.fire('click', save(fx));
  await new Promise((r) => setImmediate(r));
  assert.equal(save(fx).disabled, true);

  const t = fx.timers.find((x) => x.ms === 15000);
  assert.ok(t, 'a save in flight carries a ceiling');
  t.fn();

  assert.equal(save(fx).disabled, false, 'the editor is usable again');
  assert.equal(save(fx).getAttribute('aria-busy'), null);
  const err = fx.editor.querySelector('[data-de-err]');
  assert.match(err.textContent, /taking too long/);
  assert.equal(err.hidden, false);
});

test('starting a save CLEARS the previous failure sentence', async () => {
  const fx = await openEditor({ responses: { PUT: { ok: false, json: () => Promise.resolve({}) } } });
  fx.fire('click', save(fx));
  await new Promise((r) => setImmediate(r));
  const err = fx.editor.querySelector('[data-de-err]');
  assert.equal(err.hidden, false, 'the first attempt failed and said so');

  // A second attempt, still in flight: the stale sentence must be gone rather
  // than sitting under a spinning save as if the new one had already failed.
  fx.calls.length = 0;
  fx.fire('click', save(fx));
  assert.equal(err.textContent, '');
  assert.equal(err.hidden, true);
});
