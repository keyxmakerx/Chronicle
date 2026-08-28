// callout_widget.test.mjs — the player call-to-action widget (C-RSVP-P10).
//
// THIS SUITE EXISTS BECAUSE OF THE DEFECT IT IS MODELLED ON. `collect_rsvps`
// was pinned by a Go test asserting the server OMITS it and a JS test feeding a
// fixture that CARRIES it; both stayed green for the feature's whole life while
// the product shipped a checkbox that could never be unchecked. So these tests
// deliberately consume the SERVER'S OWN MARKUP — the exact strings
// callout_handler.go's renderCallout emits — rather than a hand-built fixture.
// If the server stops shipping `hidden`, or renames a data attribute, the
// mismatch surfaces here instead of on somebody's phone.
//
// The widget owns exactly two things the server cannot do: filling in the
// browser's timezone, and dismissal. Both are tested, plus the degrade path for
// a browser that will not report a zone at all.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

import { El, el, makeDocument } from './daycard_dom.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const widgetPath = join(here, '..', '..', 'static', 'js', 'widgets', 'callout.js');

// --- the server's markup, transcribed from renderCallout ------------------
//
// Kept as a builder rather than a raw HTML string because the minimal DOM does
// not parse HTML. The ATTRIBUTES are what matter and they are copied verbatim
// from callout_handler.go; a rename there without a rename here fails the
// contract assertion at the bottom of this file.

function rsvpBar({ count = 1, link = '/campaigns/c1/proposals/p1' } = {}) {
  const kids = [
    el('i', { class: 'fa-solid fa-hourglass-half cta-ico' }),
    el('span', { class: 'cta-msg' }, [], 'Your table is waiting on your answer'),
  ];
  if (count > 1) kids.push(el('span', { class: 'cta-count' }, [], String(count)));
  if (link) kids.push(el('a', { class: 'cta-go', href: link }, [], 'Answer now'));
  kids.push(el('button', { type: 'button', class: 'cta-x', 'data-cta-dismiss': '' }));
  return el('div', { class: 'cta-bar', 'data-cta': 'rsvp', role: 'status' }, kids);
}

function timezoneBar() {
  return el('div', { class: 'cta-bar', 'data-cta': 'timezone', role: 'status' }, [
    el('i', { class: 'fa-solid fa-clock cta-ico' }),
    el('span', { class: 'cta-msg' }, [], 'Chronicle does not know your timezone…'),
    el('button', { type: 'button', class: 'cta-go', 'data-cta-tz-accept': '', hidden: true }, [
      el('span', { 'data-cta-zone': '' }, [], 'my timezone'),
    ]),
    el('a', { class: 'cta-alt', href: '/account' }, [], 'Choose'),
    el('button', { type: 'button', class: 'cta-x', 'data-cta-dismiss': '' }),
  ]);
}

// boot runs the widget IIFE in a vm with a host containing `bar`, and returns
// handles for driving it.
function boot(bar, opts = {}) {
  const host = el('div', { class: 'cta-host', 'data-widget': 'callout' }, bar ? [bar] : []);
  const doc = makeDocument(host);

  const store = new Map(Object.entries(opts.storage || {}));
  const sessionStorage = opts.storageThrows
    ? { getItem() { throw new Error('denied'); }, setItem() { throw new Error('denied'); } }
    : {
      getItem: (k) => (store.has(k) ? store.get(k) : null),
      setItem: (k, v) => store.set(k, String(v)),
    };

  const fetches = [];
  const notices = [];
  const registry = {};

  const Chronicle = {
    register: (name, def) => { registry[name] = def; },
    apiFetch: (url, init) => {
      fetches.push({ url, init });
      if (opts.fetchFails) return Promise.reject(new Error('offline'));
      return Promise.resolve({ ok: opts.fetchOk !== false });
    },
    notify: (msg, kind) => notices.push({ msg, kind }),
  };

  // Intl is stubbed rather than trusted: the CI box's real zone would make the
  // assertions machine-dependent.
  const Intl = opts.noIntl
    ? undefined
    : {
      DateTimeFormat: function () {
        return {
          resolvedOptions: () => ({
            timeZone: Object.prototype.hasOwnProperty.call(opts, 'zone') ? opts.zone : 'Europe/London',
          }),
        };
      },
    };

  const sandbox = {
    window: { sessionStorage },
    document: doc,
    Chronicle,
    Intl,
    console,
  };
  sandbox.globalThis = sandbox;
  vm.createContext(sandbox);
  vm.runInContext(readFileSync(widgetPath, 'utf8'), sandbox, { filename: 'callout.js' });

  assert.ok(registry.callout, 'the widget did not register itself as "callout"');
  registry.callout.init(host);

  return { host, bar, fetches, notices, store, doc };
}

function click(node) {
  (node._listeners || []).filter((l) => l.type === 'click').forEach((l) => l.fn({}));
}

// The minimal DOM records listeners; expose them the way this suite needs.
El.prototype.addEventListener = function (type, fn) {
  (this._listeners = this._listeners || []).push({ type, fn });
};

// --- the timezone half -----------------------------------------------------

test('the accept button stays hidden and unusable when the browser will not name a zone', () => {
  const bar = timezoneBar();
  const { fetches } = boot(bar, { zone: '' });
  const accept = bar.querySelector('[data-cta-tz-accept]');

  assert.equal(accept.hidden, true,
    'a visible "Use my timezone" with no zone behind it would submit an empty string ' +
    'and CLEAR the field it claims to set');
  assert.equal(fetches.length, 0);
  assert.ok(bar.querySelector('.cta-alt'),
    'with no accept button the /account link is the entire offer and must survive');
});

test('a browser with no Intl at all degrades instead of throwing', () => {
  const bar = timezoneBar();
  // The assertion is that boot() itself does not throw — an exception here
  // would take the dismiss handler down with it and strand the banner.
  const { fetches } = boot(bar, { noIntl: true });
  assert.equal(bar.querySelector('[data-cta-tz-accept]').hidden, true);
  assert.equal(fetches.length, 0);
  assert.ok(bar.querySelector('[data-cta-dismiss]'), 'the banner is still dismissible');
});

test('the browser zone is filled in and the button revealed', () => {
  const bar = timezoneBar();
  boot(bar, { zone: 'America/Chicago' });
  assert.equal(bar.querySelector('[data-cta-zone]').textContent, 'America/Chicago');
  assert.equal(bar.querySelector('[data-cta-tz-accept]').hidden, false);
});

test('accepting PUTs the zone the browser reported, to the account endpoint', async () => {
  const bar = timezoneBar();
  const { fetches, notices, host } = boot(bar, { zone: 'Asia/Tokyo' });
  click(bar.querySelector('[data-cta-tz-accept]'));
  await Promise.resolve(); await Promise.resolve();

  assert.equal(fetches.length, 1);
  assert.equal(fetches[0].url, '/account/timezone');
  assert.equal(fetches[0].init.method, 'PUT');
  // Field-by-field, not deepEqual: the body literal is constructed INSIDE the
  // vm realm, so it carries the sandbox's Object.prototype and deepStrictEqual
  // rejects it as not reference-equal (the cross-realm trap: a vm's Object and
  // Array prototypes are never reference-equal to the host's).
  assert.equal(fetches[0].init.body.timezone, 'Asia/Tokyo');
  assert.deepEqual(Object.keys(fetches[0].init.body), ['timezone'],
    'the PUT must send only the field it means to change');
  assert.ok(notices.some((n) => n.kind === 'success'));
  assert.equal(host.children.length, 0,
    'the banner is satisfied, not merely dismissed — it should go without waiting for the poll');
});

test('a refused save gives the button back and says so', async () => {
  const bar = timezoneBar();
  const { notices, host } = boot(bar, { zone: 'Asia/Tokyo', fetchOk: false });
  click(bar.querySelector('[data-cta-tz-accept]'));
  await Promise.resolve(); await Promise.resolve();

  assert.equal(bar.querySelector('[data-cta-tz-accept]').disabled, false,
    'a control that disables itself and then says nothing has told the player it worked');
  assert.ok(notices.some((n) => n.kind === 'error'));
  assert.equal(host.children.length, 1, 'nothing was saved, so the banner must stay');
});

test('a network failure is handled the same way as a refusal', async () => {
  const bar = timezoneBar();
  const { notices } = boot(bar, { zone: 'Asia/Tokyo', fetchFails: true });
  click(bar.querySelector('[data-cta-tz-accept]'));
  await Promise.resolve(); await Promise.resolve(); await Promise.resolve();

  assert.equal(bar.querySelector('[data-cta-tz-accept]').disabled, false);
  assert.ok(notices.some((n) => n.kind === 'error'));
});

// --- dismissal -------------------------------------------------------------

test('dismissing removes the banner and remembers it', () => {
  const bar = timezoneBar();
  const { host, store } = boot(bar);
  click(bar.querySelector('[data-cta-dismiss]'));

  assert.equal(host.children.length, 0);
  assert.equal(store.get('chronicle:cta-dismissed:timezone'), '1');
});

test('an already-dismissed banner never re-renders', () => {
  const bar = rsvpBar();
  const { host } = boot(bar, { storage: { 'chronicle:cta-dismissed:rsvp': '1' } });
  assert.equal(host.children.length, 0,
    'the poll re-renders innerHTML every 60s, so a dismissed banner must be removed ' +
    'again on each swap or dismissal lasts one minute');
});

// THE FAILURE MODE THAT MAKES A BANNER UNTRUSTWORTHY. If dismissal were keyed
// globally, closing the standing timezone nag would also swallow a
// time-limited RSVP that arrived ten minutes later — and the player would never
// know they had been asked.
test('dismissal is per kind: closing the timezone ask does not swallow a later RSVP', () => {
  const tz = timezoneBar();
  const first = boot(tz);
  click(tz.querySelector('[data-cta-dismiss]'));
  assert.equal(first.host.children.length, 0);

  const rsvp = rsvpBar();
  const second = boot(rsvp, { storage: Object.fromEntries(first.store) });
  assert.equal(second.host.children.length, 1,
    'an RSVP was swallowed by an unrelated dismissal');
});

test('storage being unavailable shows the banner rather than hiding it', () => {
  const bar = rsvpBar();
  const { host } = boot(bar, { storageThrows: true });
  assert.equal(host.children.length, 1,
    'private browsing must not silently suppress a request the table is waiting on');
  // And dismissing must not throw, even though it cannot be remembered.
  click(bar.querySelector('[data-cta-dismiss]'));
  assert.equal(host.children.length, 0);
});

test('an empty host is not an error', () => {
  const { host } = boot(null);
  assert.equal(host.children.length, 0);
});

// --- the contract with the server -----------------------------------------
//
// The hooks the widget reaches for must be the ones the Go renderer emits. This
// is the assertion that would have caught the collect_rsvps defect: it reads
// BOTH sides and compares them, instead of trusting a local fixture.

test('every hook the widget queries is one the server actually renders', () => {
  const widget = readFileSync(widgetPath, 'utf8');
  const server = readFileSync(
    join(here, '..', '..', 'internal', 'plugins', 'sessions', 'callout_handler.go'), 'utf8');

  const hooks = [...widget.matchAll(/\[data-(cta[\w-]*)\]/g)].map((m) => m[1]);
  assert.ok(hooks.length >= 4, 'expected the widget to query several data-cta hooks');

  for (const h of new Set(hooks)) {
    assert.ok(server.includes('data-' + h),
      `the widget queries [data-${h}] but callout_handler.go never renders it — ` +
      'this is the two-halves-pinned-separately defect, again');
  }

  // And the two kind values the widget branches on must be the ones the server
  // stamps, or the RSVP banner would be treated as a timezone one.
  assert.ok(server.includes('data-cta="rsvp"'));
  assert.ok(server.includes('data-cta="timezone"'));
  assert.ok(widget.includes("!== 'timezone'"),
    'the widget no longer branches on the timezone kind');
});
