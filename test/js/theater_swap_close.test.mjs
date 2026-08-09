// theater_swap_close.test.mjs — C-CALV4-THEATER (R2-3), [TH-14] RE-SIGNED
// 2026-08-08. The counterpart the re-sign made REQUIRED.
//
// WHY THIS FILE EXISTS, IN THE RULING'S OWN TERMS. [TH-14] originally required
// the scaffold to sit OUTSIDE any HTMX-swappable region, and the build stopped
// there — correctly, because the premise was refuted rather than the builder
// wrong. `calendar_widget_type.go:152` wraps the ENTIRE output of
// EntityCalendarBlock in `widgetbindings.BlockHost`, and
// `internal/plugins/widgetbindings/picker.templ` targets THAT wrapper with
// `hx-swap="outerHTML"` on three live paths (bind, create, unbind). The
// swappable region is the whole component, one level above anything this slice
// can emit, so no position it owns satisfied the constraint.
//
// The re-sign REPLACED constraint 3: the scaffold may sit inside the swappable
// region, because opener and scaffold share that subtree and therefore die and
// revive together, and the module re-initialises on `htmx:afterSettle`. There
// is no state in which a live opener points at a dead scaffold — which is the
// "opens once, then stops" failure the original constraint was written to
// prevent.
//
// THE ONE REAL RISK IT LEAVES is a `<dialog>` that is OPEN when its subtree is
// swapped: removed from the DOM mid-modal, it strands the top layer, leaves the
// document scroll-locked and drops focus on a detached node. Narrow — while the
// theater is open the picker behind it is inert, so the swap must be driven
// from elsewhere — but real. This suite is its pin.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot } from './theater_harness.mjs';

test('swapping the host while the theater is OPEN closes it and returns focus', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  assert.equal(fx.dialog.open, true);
  assert.equal(fx.lockedNow(), true);
  assert.equal(fx.activeElement(), fx.closeBtn);

  // picker.templ's swap target IS the BlockHost wrapper, and the scaffold is
  // inside it.
  fx.fire('htmx:beforeSwap', { target: fx.host });

  assert.equal(fx.dialog.open, false, 'the dialog was swapped away while still in the top layer');
  assert.equal(fx.lockedNow(), false, 'the scroll lock survived the swap — the page behind stays frozen with no modal to close');
  assert.equal(fx.activeElement(), fx.opener, 'focus was left on a node that is about to be detached');
  assert.equal(fx.opener.getAttribute('aria-expanded'), 'false');
});

test('the swap close is IMMEDIATE — it does not wait for a transition on a node that is about to vanish', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fire('htmx:beforeSwap', { target: fx.host });

  assert.equal(fx.timers.length, 0,
    'the swap close armed the register watchdog; it would fire against a detached element after the swap had already replaced it');
});

test('a swap that does not contain the theater leaves it alone', () => {
  // HTMX swaps happen all over an entity page. Closing a modal because some
  // unrelated fragment settled would be its own bug.
  const fx = boot();
  fx.fireOn('click', fx.opener);

  fx.fire('htmx:beforeSwap', { target: fx.embed });
  assert.equal(fx.dialog.open, true, 'an unrelated swap closed the theater');
  assert.equal(fx.lockedNow(), true);
});

test('a swap while the theater is CLOSED is a no-op', () => {
  const fx = boot();
  assert.doesNotThrow(() => fx.fire('htmx:beforeSwap', { target: fx.host }));
  assert.equal(fx.dialog.open, false);
  assert.equal(fx.lockedNow(), false);
  assert.equal(fx.activeElement(), null, 'a closed theater moved focus on a swap');
});

test('a beforeSwap with no usable target is survived rather than thrown on', () => {
  const fx = boot();
  fx.fireOn('click', fx.opener);
  assert.doesNotThrow(() => fx.fire('htmx:beforeSwap', {}));
  assert.doesNotThrow(() => fx.fire('htmx:beforeSwap', { target: null }));
  assert.equal(fx.dialog.open, true);
});

test('after the swap re-renders the component, the theater opens again', () => {
  // The other half of the re-sign's argument: scaffold and opener are reborn
  // together and htmx:afterSettle rewires both, so the affordance is not a
  // one-shot. A fresh boot IS the re-rendered subtree — the swap replaces the
  // nodes, so the guard attribute goes with them.
  const fx = boot();
  fx.fireOn('click', fx.opener);
  fx.fire('htmx:beforeSwap', { target: fx.host });
  assert.equal(fx.dialog.open, false);

  const rebuilt = boot();
  rebuilt.fire('htmx:afterSettle', {});
  rebuilt.fireOn('click', rebuilt.opener);
  assert.equal(rebuilt.dialog.open, true, 'the theater opened once and then stopped opening');
  assert.equal(rebuilt.boxOpen(), true);
});
