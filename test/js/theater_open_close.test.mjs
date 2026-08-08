// theater_open_close.test.mjs — C-CALV4-THEATER (R2-3). The open/close state
// machine, the register on every path, and the reduced-motion short circuit.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './theater_harness.mjs';

test('the Expand control opens the theater, modal, locked and revealed', () => {
  const fx = boot();
  assert.equal(fx.dialog.open, false, 'the theater ships open');
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');

  fx.fireOn('click', fx.opener);

  assert.equal(fx.dialog.open, true, 'showModal() was never called');
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'true');
  assert.equal(fx.boxOpen(), true, 'the register reveal never engaged on .tbox');
  // <dialog> gives inertness and focus containment for free; it does NOT give
  // scroll-lock, so the module owns that half ([TH-7]: the page behind must not
  // scroll while the theater is open).
  assert.equal(fx.lockedNow(), true, 'the page behind is not scroll-locked');
});

test('the reveal runs on the inner box and the layout flush precedes the class', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);

  // THE SUBJECT IS .tbox, NOT THE DIALOG. An element that was display:none has
  // no before-change style to transition FROM, @starting-style is a standing
  // refusal, so the reveal runs on a child that is rendered the whole time.
  const box = fx.dialog.querySelector('[data-theater-box]');
  assert.equal(box.classList.contains('tbopen'), true);
  assert.equal(fx.dialog.classList.contains('tbopen'), false, 'the dialog itself must not carry the reveal class');
  // The reveal class lands AFTER the dialog is open — i.e. after the layout
  // flush the module forces between the two. Reversing them is the silent
  // failure the sheet's comment warns about.
  assert.equal(fx.dialog.hasAttribute('open'), true);
});

test('the close button closes THROUGH the register, not around it', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('click', fx.closeBtn);

  // The class comes off first; the dialog is still open, waiting for the
  // transition to end. A close that called close() immediately would skip the
  // 160ms the register owns.
  assert.equal(fx.boxOpen(), false, 'the reveal class was not removed');
  assert.equal(fx.dialog.open, true, 'the dialog closed before the register ran');
  assert.equal(fx.timers.length, 1, 'no watchdog was armed — a browser that never fires transitionend would strand the theater open');

  fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
  assert.equal(fx.dialog.open, false, 'transitionend did not finish the close');
  assert.equal(fx.lockedNow(), false, 'the scroll lock outlived the theater');
  assert.equal(fx.timers.length, 0, 'the watchdog was not cleared, so finish() will run twice');
});

test('Escape is cancelable and runs the register — the whole reason <dialog> was chosen', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);

  const { defaultPrevented } = fx.fireOn('cancel', fx.dialog);
  assert.equal(defaultPrevented, true, 'the UA close was not prevented, so the register is skipped on the Escape path');
  assert.equal(fx.boxOpen(), false);
  assert.equal(fx.dialog.open, true, 'the dialog closed synchronously');

  fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
  assert.equal(fx.dialog.open, false);
});

test('a backdrop click closes, and a click inside the theater does not', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);

  // A click whose target is a descendant — anything in the Block — is not a
  // backdrop click and must not close the theater.
  fx.fireOn('click', fx.dialog, { target: fx.theaterBlock });
  assert.equal(fx.dialog.open, true, 'a click on the Block closed the theater');
  assert.equal(fx.boxOpen(), true);

  fx.fireOn('click', fx.dialog, { target: fx.dialog });
  assert.equal(fx.boxOpen(), false, 'the backdrop click did not run the register');
  fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
  assert.equal(fx.dialog.open, false);
});

test('a transition ending anywhere else cannot finish the close early', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('click', fx.closeBtn);

  // The Block inside the theater is a whole calendar surface; a transition on
  // one of its own elements must not be mistaken for the register's.
  fx.fireOn('transitionend', fx.dialog, { target: fx.theaterBlock });
  assert.equal(fx.dialog.open, true, 'a foreign transitionend finished the close');

  fx.fireOn('transitionend', fx.dialog, { target: fx.dialog.querySelector('[data-theater-box]') });
  assert.equal(fx.dialog.open, false);
});

test('the watchdog closes a theater whose transitionend never arrives', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fireOn('click', fx.closeBtn);
  assert.equal(fx.dialog.open, true);

  fx.flush();
  assert.equal(fx.dialog.open, false, 'a browser that never fires transitionend strands the dialog open');
  assert.equal(fx.lockedNow(), false);
});

test('under reduced motion the close is INSTANT and waits on nothing', () => {
  // The [TH-3] rules live inside calendar-bench.css's single no-preference
  // wrapper, so under prefers-reduced-motion they DO NOT EXIST — no
  // transitionend will ever fire, and a close path that waited on one hangs the
  // theater open. The short circuit is load-bearing rather than polite.
  const fx = boot({ reduced: true });
  fx.fireOn('click', fx.opener);
  assert.equal(fx.dialog.open, true);

  fx.fireOn('click', fx.closeBtn);
  assert.equal(fx.dialog.open, false, 'the theater waited for a transition that will never fire');
  assert.equal(fx.lockedNow(), false);
  assert.equal(fx.timers.length, 0, 'a timer was armed under reduced motion — nothing is being waited for');
});

test('the wiring guard is idempotent, so a re-init is not a second listener', () => {
  const fx = boot();
  // The scaffold sits inside the region picker.templ swaps, so initAll runs
  // again on every htmx:afterSettle. Re-binding an already-wired scaffold must
  // be a no-op: a second click handler would open an already-open theater and a
  // second cancel handler would run two closes against one dialog.
  fx.fire('htmx:afterSettle', {});
  fx.fire('htmx:load', {});

  assert.equal((fx.opener._on.click || []).length, 1, 'the opener collected a second click listener');
  assert.equal((fx.dialog._on.cancel || []).length, 1, 'the dialog collected a second cancel listener');
  assert.equal(fx.dialog.getAttribute('data-theater-wired'), '1');
});

test('the module returns immediately when its mount is absent', () => {
  // The registry mounts this on EVERY page ([TH-12]); on all but one there is
  // nothing to bind and it must cost nothing and throw nothing.
  const fx = boot();
  const host = fx.dialog.parentNode;
  host.removeChild(fx.dialog);
  assert.doesNotThrow(() => fx.fire('htmx:afterSettle', {}));
});

test('opening twice is a no-op rather than a second showModal', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  const before = fx.focused.length;
  fx.fireOn('click', fx.opener);
  assert.equal(fx.dialog.open, true);
  assert.equal(fx.focused.length, before, 're-opening moved focus again, which would yank a reader out of the surface they were in');
});

test('the layout flush happens BEFORE the reveal class, in that order', () => {
  // Mechanism, not decoration. Without the forced read the class lands in the
  // same style change as the dialog becoming visible, there is no before-change
  // style to transition from, and the reveal silently does not run — while
  // every end-state assertion above stays green. A reader who deletes the
  // `void b.offsetHeight` as a stray now meets this instead of a still theater.
  const fx = boot();
  fx.fireOn('click', fx.opener);

  const ops = fx.boxEl._ops || [];
  const read = ops.indexOf('read:offsetHeight');
  const painted = ops.findIndex((o) => o.startsWith('class:') && o.includes('tbopen'));
  assert.ok(read >= 0, 'the module never forced a layout flush on the reveal box');
  assert.ok(painted >= 0, 'the reveal class was never added');
  assert.ok(read < painted, 'the reveal class was added before the layout flush, so the transition has no start value to run from');
});

test('the module binds EVERY scaffold, and each opener drives its own theater', () => {
  // [TH-15] SIGNED. `EntityCalendarBlock` is a per-block registry entry, so a
  // page can carry more than one (calendar, entity) pair. A `querySelector`
  // mount would bind the first and hand every Expand button on the page the
  // same theater — a defect that is invisible on a one-embed page, which is
  // exactly why it is pinned here.
  const fx = boot({ twin: true });
  assert.ok(fx.twin, 'the twin fixture did not build');
  assert.notEqual(fx.scaffoldID, fx.twin.scaffoldID, 'the two scaffolds share an id');

  fx.fireOn('click', fx.twin.opener);
  assert.equal(fx.twin.dialog.open, true, 'the SECOND opener did not open the second theater');
  assert.equal(fx.dialog.open, false, 'the second opener drove the FIRST theater');

  fx.fireOn('click', fx.twin.closeBtn);
  fx.fireOn('transitionend', fx.twin.dialog, { target: fx.twin.dialog.querySelector('[data-theater-box]') });
  assert.equal(fx.twin.dialog.open, false);
  assert.equal(fx.activeElement(), fx.twin.opener, 'focus returned to the wrong opener');
});
