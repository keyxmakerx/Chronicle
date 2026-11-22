// deck_moments.spec.mjs: a card that waits for its moment, walked as the people
// at a table do. A Director gives a card a printed test and a test of their
// own in the Cards tab; the player reads only the printed line. A hero's
// number moves; the Director's Table counts it and slides a notice in with the
// card; the player's browser is sent nothing of it. Draw another, then Deal
// puts the card in the hand face down. Let them draw invites the player, who
// sees the invitation, draws, and the moment closes. Not now closes one. A
// number moved back makes the notice go away. A test whose number was removed
// is marked as pointing at nothing. The walk is made on a phone and on a
// desktop, in light and dark, with no sideways scroll and no console error.
//
// Set LIVE_SHOTS to a folder to also save pictures there.
//
// RUN: tools/live-checks.sh test/live/deck_moments.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';
const JSON_HEADERS = { 'Content-Type': 'application/json' };

// What a player's browser must never be sent: the Director's own test, whether
// anything holds, or the notice itself.
const SECRETS = ['0.41729', 'fitsNow', 'whyNot', 'fitsSince', 'saidAt', 'whenJson', 'directorNote'];

const watchErrors = (who) => {
  const errors = [];
  who.page.on('console', (m) => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()); });
  who.page.on('pageerror', (e) => errors.push(String(e)));
  return errors;
};

// Every response body and every request URL the person's browser had, for the "never sent" checks.
const record = (who) => {
  const rec = { bodies: [], urls: [], from: [] };
  who.page.on('request', (r) => rec.urls.push(r.url()));
  who.page.on('response', async (r) => {
    try {
      const type = r.headers()['content-type'] || '';
      if (/text|json|javascript/.test(type)) { const b = await r.text(); rec.bodies.push(b); rec.from.push(r.url()); }
    } catch (e) { /* a redirect or a closed page has no body */ }
  });
  return rec;
};

const j = (who, url, method, body) => who.api(url, { method, body: body === undefined ? undefined : JSON.stringify(body), headers: JSON_HEADERS });

async function newHero(owner, cid, name) {
  await owner.go(`/campaigns/${cid}/entities/new`);
  const etid = await owner.page.locator('select#entity_type_id').evaluate((el) => (([...el.options].find((o) => o.value && /character/i.test(o.textContent)) || {}).value));
  const id = await owner.page.evaluate(async ({ cid, etid, name }) => {
    const form = new FormData();
    form.append('name', name); form.append('entity_type_id', etid); form.append('is_private', 'false');
    const res = await Chronicle.apiFetch(`/campaigns/${cid}/entities`, { method: 'POST', body: form });
    return (res.url.match(/entities\/([0-9a-f-]{36})/) || [])[1] || '';
  }, { cid, etid, name });
  assert.ok(id, 'the hero page was not made');
  await owner.api(`/campaigns/${cid}/entities/${id}/permissions`, { method: 'PUT', body: JSON.stringify({ visibility: 'default', is_private: false, permissions: [] }), headers: JSON_HEADERS });
  await owner.api(`/campaigns/${cid}/characters/playable`, { method: 'POST', form: { entity_id: id, playable: '1' } });
  return { id, etid };
}

// numberFields gives the character type the fields a test can read.
async function giveFields(owner, cid, etid, fields) {
  const t = await owner.api(`/campaigns/${cid}/entity-types/${etid}`, { method: 'PUT', body: {} });
  assert.ok(t.ok, 'could not read the character type');
  const put = await owner.api(`/campaigns/${cid}/entity-types/${etid}`, { method: 'PUT', body: { fields } });
  assert.ok(put.ok, 'type update answered ' + put.status);
}

const field = (key, label) => ({ key, label, type: 'number' });
const setHP = async (owner, cid, hero, hp) => {
  const r = await owner.api(`/campaigns/${cid}/entities/${hero}/fields`, { method: 'PUT', body: JSON.stringify({ fields_patch: { hp } }), headers: JSON_HEADERS });
  assert.ok(r.ok, 'the number was not set: ' + JSON.stringify(r.body));
};

// A Director, a player with a claimed hero, and a hero deck held by that hero
// with two cards that come up when the hero is hurt: each prints "at or below 5"
// and keeps "at or above 0.41729" to the Director.
async function table(tag, { width, height, scheme }) {
  const ctx = { viewport: { width, height }, colorScheme: scheme, reducedMotion: 'no-preference' };
  const owner = await person({ tag: tag + '-d', name: 'Director Dara', context: ctx });
  const tam = await person({ tag: tag + '-t', name: 'Tam Player', context: ctx });
  const cid = await newCampaign(owner, 'Moments ' + tag);
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, tam, 'player');
  const hero = await newHero(owner, cid, 'Orrin');
  await giveFields(owner, cid, hero.etid, [field('hp', 'Hit points'), field('stamina', 'Stamina')]);
  await tam.go(`/campaigns/${cid}`);
  const claim = await tam.api(`/campaigns/${cid}/entities/${hero.id}/claim`, { method: 'POST' });
  assert.ok(claim.ok, 'could not claim a hero: ' + JSON.stringify(claim.body));
  await setHP(owner, cid, hero.id, 20);

  const A = `/campaigns/${cid}/decks`;
  const deck = (await j(owner, A, 'POST', { name: 'Omens', kind: 'hero', settings: { knows: 'kinds', othersSee: 'face', arrives: 'down', draws: 'players', notice: 'slide' } })).body.id;
  const v = (await owner.api(`${A}/${deck}`)).body.version;
  const held = await j(owner, `${A}/${deck}`, 'PUT', { version: v, holderEntityId: hero.id });
  assert.ok(held.ok, 'the deck was not given to the hero: ' + JSON.stringify(held.body));
  const printed = { test: 'number', open: true, who: 'holder', field: 'hp', op: 'lte', value: 5 };
  const own = { test: 'number', open: false, who: 'holder', field: 'hp', op: 'gte', value: 0.41729 };
  const made = await j(owner, `${A}/${deck}/cards`, 'POST', { name: 'Stormcaller', words: 'The sky breaks', copies: 3, when: [printed, own] });
  assert.ok(made.ok, 'the card was not made: ' + JSON.stringify(made.body));
  const storm = made.body.cards.find((c) => c.name === 'Stormcaller').id;
  const made2 = await j(owner, `${A}/${deck}/cards`, 'POST', { name: 'Frostbite', words: 'The cold bites', copies: 3, when: [printed, own] });
  assert.ok(made2.ok, 'the second card was not made: ' + JSON.stringify(made2.body));

  await owner.go(`${A}/workshop/${deck}?tab=players`);
  const uid = await owner.page.evaluate(() => [...document.querySelectorAll('.dkw-prow')].find((r) => r.textContent.includes('Tam Player')).getAttribute('data-user'));
  const v2 = (await owner.api(`${A}/${deck}`)).body.version;
  const chose = await j(owner, `${A}/${deck}/players`, 'PUT', { version: v2, players: [{ userId: uid, drawsLeft: 3 }] });
  assert.ok(chose.ok, 'could not choose the player: ' + JSON.stringify(chose.body));
  return { owner, tam, cid, A, deck, storm, hero: hero.id, etid: hero.etid, scheme, width };
}

const noSideways = (page, what) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  .then((w) => assert.ok(w <= 0, `${what}: the page scrolls sideways by ${w}px`));

async function shot(who, name, w, within) {
  if (!SHOTS) return;
  await who.page.waitForTimeout(500);
  const path = `${SHOTS}/${name}-${w.scheme}-${w.width}.png`;
  if (!within) { await who.page.screenshot({ path }); return; }
  await who.page.locator(within).first().evaluate((el) => el.scrollIntoView({ block: 'center' }));
  await who.page.screenshot({ path });
}
const EDITOR = '[data-dkw-when]';

// openOpen draws a select's choices as a list, since a closed browser draws
// none in a picture; it is undone straight after.
async function listed(loc, fn) {
  await loc.evaluate((s) => { s.dataset.was = String(s.size || 0); s.size = s.options.length; });
  try { await fn(); } finally { await loc.evaluate((s) => { s.size = Number(s.dataset.was) || 0; }); }
}

const count = (who) => who.page.locator('[data-tt-count]');
const waitCount = (who, n) => who.page.waitForFunction((want) => {
  const c = document.querySelector('[data-tt-count]');
  return want === 0 ? !c || c.hidden : !c.hidden && c.textContent === String(want);
}, n);
// openTable opens the Table: the tab on a wide screen, the sheet's handle on a phone.
const openTable = async (who) => {
  const tab = who.page.locator('[data-tt-tab]');
  if (await tab.isVisible()) await tab.click(); else await who.page.locator('[data-tt-grab]').click();
};
// openWaiting opens a waiting moment from the drawer's first line, opening the Table if it is shut.
const openWaiting = async (who) => {
  const row = who.page.locator('[data-tt-wait] [data-tt-moment]').first();
  const shown = (await row.count()) > 0 && await row.evaluate((el) => { const r = el.getBoundingClientRect(); return r.bottom <= innerHeight && r.top >= 0 && r.right <= innerWidth; });
  if (!shown) await openTable(who);
  await row.click();
};
const notice = (who) => who.page.locator('#tt-moment');

async function walk(tag, view, { keyboard = false } = {}) {
  const w = await table(tag, view);
  const { owner, tam, cid, A, deck, storm, hero } = w;
  const ownerErrors = watchErrors(owner), tamErrors = watchErrors(tam);
  const sent = record(tam);

  // The editor: a printed test and an own test in the Cards tab.
  await owner.go(`${A}/workshop/${deck}?tab=cards&card=${storm}`);
  const rows = owner.page.locator('[data-dkw-when] [data-when-row]');
  await rows.first().waitFor();
  assert.equal(await rows.count(), 2);
  assert.deepEqual(await rows.locator('.dkw-mark').allTextContents(), ['On the card', 'Only you']);
  const section = await owner.page.locator('[data-dkw-when]').textContent();
  assert.match(section, /Every condition must hold\. Up to four\./);
  assert.equal(await rows.first().locator('select[aria-label="Which number"]').inputValue(), 'hp');
  assert.equal(await rows.first().locator('select[aria-label="How it compares"]').inputValue(), 'lte');
  assert.match(await owner.page.locator('#dkw-preview').innerText(), /The second condition is yours alone, so the card shows only the first\./);
  // The leaf beside the card is drawn only where there is room for it, so the printed line is read there.
  if (view.width >= 900) assert.match(await owner.page.locator('#dkw-preview').innerText(), /Hit points is at or below 5/);
  assert.doesNotMatch(await owner.page.locator('#dkw-preview').textContent(), /0\.41729/, 'the preview prints the Director’s own test');
  assert.match(await owner.page.locator('[data-dkw-now]').innerText(), /Fits now\.|Waiting: /);
  await noSideways(owner.page, 'the card editor');
  await shot(owner, 'editor-two', view, EDITOR);
  await listed(owner.page.locator('.dkw-addsel'), () => shot(owner, 'editor-add-menu', view, EDITOR));
  await listed(rows.first().locator('select.dw-blank').nth(2), () => shot(owner, 'editor-blank-menu', view, EDITOR));

  // A changed blank saves at once through the card update.
  const saving = owner.page.waitForResponse((r) => r.request().method() === 'PUT' && /\/cards\//.test(r.url()));
  await rows.first().locator('select[aria-label="How it compares"]').selectOption('lt');
  assert.ok((await saving).ok(), 'a changed blank was not saved');
  await owner.page.waitForFunction(() => /is below 5/.test(document.querySelector('#dkw-preview').textContent));
  await rows.first().locator('select[aria-label="How it compares"]').selectOption('lte');
  await owner.page.waitForFunction(() => /is at or below 5/.test(document.querySelector('#dkw-preview').textContent));
  // Text that is not a number is refused in the row and nothing is sent.
  const sentPuts = [];
  owner.page.on('request', (r) => { if (r.method() === 'PUT' && /\/cards\//.test(r.url())) sentPuts.push(r.url()); });
  await rows.first().locator('input[aria-label="The number"]').fill('lots');
  await rows.first().locator('input[aria-label="The number"]').press('Tab');
  await owner.page.getByText('Type a number.').waitFor();
  assert.deepEqual(sentPuts, []);
  await owner.page.waitForFunction(() => !!document.querySelector('[data-when-row="0"] [aria-label="The number"]'));
  const typed = owner.page.waitForResponse((r) => r.request().method() === 'PUT' && /\/cards\//.test(r.url()));
  await rows.first().locator('input[aria-label="The number"]').fill('6');
  await rows.first().locator('input[aria-label="The number"]').press('Tab');
  assert.ok((await typed).ok(), 'a typed number was not saved');
  await owner.page.waitForFunction(() => /is at or below 6/.test(document.querySelector('#dkw-preview').textContent));

  // The cards grid marks a card that fits now or waits.
  await owner.go(`${A}/workshop/${deck}?tab=cards`);
  await owner.page.locator('.dkw-fitmark').first().waitFor();
  assert.ok(await owner.page.locator('#dkw-grid').innerText().then((t) => /Waiting/.test(t)), 'the grid does not mark a waiting card');
  await noSideways(owner.page, 'the cards grid');
  await shot(owner, 'cards-grid-marks', view, '#dkw-grid');

  // The Director's Table: nothing is waiting while Orrin is well.
  await owner.go(`/campaigns/${cid}`);
  await tam.go(`/campaigns/${cid}?table=hand`);
  await tam.page.locator('#tt-drawer [data-dkh-root]').first().waitFor();
  await waitCount(owner, 0);
  assert.equal(await notice(owner).count(), 0);

  // The number falls: the Director's count is 1 and the notice slides in with the card.
  await owner.page.locator('#main-content').click({ position: { x: 5, y: 5 }, force: true }).catch(() => {});
  await setHP(owner, cid, hero, 4);
  await waitCount(owner, 1);
  await notice(owner).waitFor();
  const slid = await notice(owner).innerText();
  assert.match(slid, /Orrin.s Hit points is 4\./);
  assert.match(await owner.page.locator('#tt-live').textContent(), /A card.s moment has come in Omens\./, 'the slide-in was not said once in the live region');
  assert.match(slid, /Stormcaller|Frostbite/);
  const buttons = (await notice(owner).locator('[data-dkm-act]').allInnerTexts()).map((t) => t.trim());
  assert.deepEqual(buttons, ['Deal to Orrin', 'Draw another', 'Let Orrin draw', 'Not now']);
  assert.notEqual(await owner.page.evaluate(() => document.activeElement && document.activeElement.closest('#tt-moment') ? 'in' : 'out'), 'in', 'the slide-in took focus');
  await noSideways(owner.page, 'the notice');
  await shot(owner, 'notice-slid-in', view);

  // The player's browser was sent nothing of it.
  await tam.page.waitForTimeout(600);
  const html = await tam.page.content();
  for (const s of SECRETS) {
    assert.ok(!html.includes(s), `Tam's page holds ${s}`);
    assert.ok(!sent.bodies.some((b) => b.includes(s)), `a response to Tam held ${s}: ${sent.from.filter((u, i) => sent.bodies[i].includes(s)).join(' ')}`);
  }
  assert.ok(!sent.urls.some((u) => /decks\/moments/.test(u)), 'Tam’s browser asked for a moment');
  assert.equal(await tam.page.locator('#tt-moment').count(), 0);
  assert.equal(await tam.page.locator('[data-tt-count]').evaluate((c) => c.hidden), true, 'Tam’s count moved');

  // Closing the notice (by Escape or the button) keeps the count.
  if (keyboard) { await notice(owner).locator('[data-dkm-close]').focus(); await owner.page.keyboard.press('Escape'); } else { await notice(owner).locator('[data-dkm-close]').click(); }
  await notice(owner).waitFor({ state: 'detached' });
  await waitCount(owner, 1);

  // Waiting on you: the drawer's first line, opening one puts focus on its heading.
  await openTable(owner);
  const wait = owner.page.locator('[data-tt-wait]');
  await wait.waitFor();
  assert.match(await wait.textContent(), /Waiting on you/);
  await noSideways(owner.page, 'Waiting on you');
  await shot(owner, 'drawer-waiting', view);
  await wait.locator('[data-tt-moment]').first().click();
  await notice(owner).locator('#dkm-h').waitFor();
  await owner.page.waitForFunction(() => document.activeElement && document.activeElement.id === 'dkm-h');
  await shot(owner, 'notice-one-card', view);

  // Draw another, then Deal: the card arrives in the hand face down.
  const another = owner.page.waitForResponse((r) => /\/another$/.test(r.url()));
  await notice(owner).locator('[data-dkm-act="another"]').click();
  assert.ok((await another).ok(), 'Draw another was refused');
  await notice(owner).locator('[data-dkm-act="deal"]').waitFor();
  const deal = owner.page.waitForResponse((r) => /\/deal$/.test(r.url()));
  await notice(owner).locator('[data-dkm-act="deal"]').click();
  assert.ok((await deal).ok(), 'Deal was refused');
  const slot = tam.page.locator('#tt-drawer [data-dkh-slot]');
  await slot.first().waitFor();
  assert.equal(await slot.locator('.dk-turn').count(), 1, 'the dealt card did not arrive face down');
  assert.ok(!(await tam.page.content()).match(/Stormcaller|Frostbite/), 'the face was sent before the tap');
  await slot.locator('.dk-turn').click();
  await tam.page.locator('#tt-drawer .dk-plate').first().waitFor();
  await tam.page.locator('#tt-drawer [data-dkh-open]').first().click();
  const over = tam.page.locator('[data-dkh-over]');
  await over.waitFor();
  const face = await over.textContent();
  assert.match(face, /Hit points is at or below [56]/, 'the printed line is not on the face');
  assert.doesNotMatch(face, /0\.41729/);
  await over.getByRole('button', { name: 'Keep it' }).click();
  await over.waitFor({ state: 'detached' });
  // The deck still has a card that fits, so the Director sees the next moment.
  await owner.page.keyboard.press('Escape');

  // A card that fits when it is made is a moment: each of these comes with the number still at 4.
  const newOmen = async (name) => {
    const r = await j(owner, `${A}/${deck}/cards`, 'POST', { name, words: 'More omens', copies: 2, when: [{ test: 'number', open: true, who: 'holder', field: 'hp', op: 'lte', value: 5 }] });
    assert.ok(r.ok, 'the card was not made: ' + JSON.stringify(r.body));
  };

  // A moment again: Let Orrin draw invites the player, who draws and closes it.
  await newOmen('Omen two');
  await waitCount(owner, 1);
  await openWaiting(owner);
  await notice(owner).locator('[data-dkm-act="invite"]').click();
  const invitation = tam.page.locator('#tt-drawer').getByText('Your Director invites you to draw.');
  await invitation.waitFor();
  assert.equal(await tam.page.locator('#tt-drawer [data-dkh-draws]').innerText().then((t) => /left/i.test(t)), false, 'an invited draw shows draws left');
  await noSideways(tam.page, 'the invitation');
  await shot(tam, 'hand-invited', view);
  await tam.page.locator('#tt-drawer [data-dkh-draw]').click();
  await tam.page.locator('#tt-drawer [data-dkh-slot]').nth(1).waitFor();
  await waitCount(owner, 0);
  await notice(owner).waitFor({ state: 'detached', timeout: 5000 }).catch(() => assert.fail('the notice stayed after the invited draw'));

  // Not now closes one; a number moved back makes a new one go away.
  await newOmen('Omen three');
  await waitCount(owner, 1);
  await openWaiting(owner);
  await notice(owner).locator('[data-dkm-act="dismiss"]').click();
  await waitCount(owner, 0);
  await newOmen('Omen four');
  await waitCount(owner, 1);
  await setHP(owner, cid, hero, 30);
  await waitCount(owner, 0);

  // A test whose number was taken away points at nothing.
  const old = await j(owner, `${A}/${deck}/cards`, 'POST', { name: 'Old omen', words: 'Long gone', copies: 1, when: [{ test: 'number', open: true, who: 'holder', field: 'stamina', op: 'lte', value: 3 }] });
  assert.ok(old.ok, 'the card was not made: ' + JSON.stringify(old.body));
  const oldId = old.body.cards.find((c) => c.name === 'Old omen').id;
  await giveFields(owner, cid, w.etid, [field('hp', 'Hit points')]);
  await owner.go(`${A}/workshop/${deck}?tab=cards&card=${oldId}`);
  await owner.page.getByText('This no longer points at anything.').waitFor();
  await noSideways(owner.page, 'a broken condition');
  await shot(owner, 'editor-broken', view, EDITOR);

  assert.deepEqual(ownerErrors, [], 'the Director saw console errors');
  assert.deepEqual(tamErrors, [], 'Tam saw console errors');
  tam.assertClean(); owner.assertClean();
}

test('on a phone, in light: the editor, the notice, deal, invite, not now', async () => {
  await walk('dmp', { width: 390, height: 844, scheme: 'light' });
});

test('on a desktop, in dark: the same walk', async () => {
  await walk('dmd', { width: 1280, height: 900, scheme: 'dark' });
});

test('by keyboard, in light: the notice closes with Escape', async () => {
  await walk('dmk', { width: 1280, height: 900, scheme: 'light' }, { keyboard: true });
});

test('a player and a stranger are told no notice is there', async () => {
  const owner = await person({ tag: 'dms-d', name: 'Director Dara' });
  const tam = await person({ tag: 'dms-t', name: 'Tam Player' });
  const stranger = await person({ tag: 'dms-s', name: 'Stranger Sol' });
  const cid = await newCampaign(owner, 'Moments closed');
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, tam, 'player');
  const url = `/campaigns/${cid}/decks/moments/panel`;
  assert.equal((await owner.api(url)).status, 200, 'the Director was refused');
  assert.equal((await tam.api(url)).status, 404, 'a player read the notice');
  assert.equal((await stranger.api(url)).status, 403, 'a non-member read the notice');
});
