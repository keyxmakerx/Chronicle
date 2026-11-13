// account_signed_in.spec.mjs — "Where you're signed in" on My account lists
// each browser, ends one on request, and Sign out everywhere ends them all.
// Stands in for the operator check in #1909.
//
// Covers steps 1 to 3 and 5 with two browser contexts signed in as one
// account (the second pretends to be a phone). Step 4 (a Foundry world
// listed with Disconnect) needs a connected Foundry world; step 6 (the
// report-only script-rule log after a day of use) needs the live server's
// logs over time.
//
// RUN: tools/live-checks.sh test/live/account_signed_in.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, done, picture } from './_live.mjs';

after(done);

const PHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1';

test('a second browser is listed, can be signed out, and sign out everywhere ends all', async () => {
  const desk = await person({ tag: 'signedin', name: 'Mara Vell' });
  const phone = await person({
    email: desk.email, password: desk.password, register: false,
    context: { userAgent: PHONE, isMobile: true },
  });

  await desk.go('/account');
  const card = desk.page.locator('#signed-in');
  await card.waitFor();
  const rows = card.locator('ul > li');
  assert.equal(await rows.count(), 2, 'the card lists this browser and the phone');
  await picture(desk.page, 'account-signed-in', {
    check: '#1909', about: 'Where you\'re signed in: this browser first, the phone under it',
    locator: card, mask: ['#signed-in span.text-xs'],
  });

  const first = rows.nth(0);
  await first.getByText('This browser').waitFor();
  assert.equal(await first.getByRole('button', { name: /Sign out/ }).count(), 0, 'this browser has no Sign out button of its own');
  const second = rows.nth(1);
  const out = second.getByRole('button', { name: /^Sign out/ });
  assert.equal(await out.count(), 1, 'the other browser has a Sign out button');

  // The phone is still signed in until the button is pressed.
  assert.ok(!/\/login/.test((await phone.go('/account')).url()), 'phone is signed in before');

  await out.click();
  await card.getByText('Signed out.').waitFor();
  assert.equal(await card.locator('ul > li').count(), 1, 'the phone row disappears');

  await phone.go('/account');
  assert.match(phone.page.url(), /\/login/, 'the phone is asked to sign in at its next page load');

  // Sign out everywhere: a third browser goes too, and this one lands on sign-in.
  const third = await person({ email: desk.email, password: desk.password, register: false });
  await desk.go('/account');
  desk.page.once('dialog', (d) => d.accept());
  await Promise.all([
    desk.page.waitForURL(/\/login/),
    desk.page.getByRole('button', { name: 'Sign out everywhere' }).click(),
  ]);
  await third.go('/account');
  assert.match(third.page.url(), /\/login/, 'every other browser is signed out too');
  desk.assertClean();
});

test('the card shows no error line for a normal account', async () => {
  const who = await person({ tag: 'signedin2' });
  await who.go('/account');
  const card = who.page.locator('#signed-in');
  await card.waitFor();
  assert.equal(await card.getByRole('alert').count(), 0);
  assert.equal(await card.locator('ul > li').count(), 1);
});
