// deck_hand.spec.mjs: a player's hand (the Hand leaf of the Table, the opened
// card, the Cards part of a hero's page) walked as the people at a table do.
//
// A Director deals a card from a second browser; the player sees it arrive
// face down and new without reloading; the page source and every response the
// player's browser received hold none of the card's name, words, kind or
// picture before the tap; the tap turns it and the face shows; showing it to
// the table puts the face on another player's screen; asking to play lands on
// the Director's row, and Let them puts the card on the table for both; a card
// taken back rings the bell. The same walk is made on a phone and on a desktop,
// in light and dark, by mouse and by keyboard, with no sideways scroll and no
// console error.
//
// Set LIVE_SHOTS to a folder to also save pictures there.
//
// RUN: tools/live-checks.sh test/live/deck_hand.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, addMember, enableAddons, done } from './_live.mjs';

after(done);

const SHOTS = process.env.LIVE_SHOTS || '';
const JSON_HEADERS = { 'Content-Type': 'application/json' };
const PNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==';

const watchErrors = (who) => {
  const errors = [];
  who.page.on('console', (m) => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()); });
  who.page.on('pageerror', (e) => errors.push(String(e)));
  return errors;
};

// Every response body the person's browser received, for the "never sent" checks.
const recordBodies = (who) => {
  const bodies = [];
  who.page.on('response', async (r) => {
    try {
      const type = r.headers()['content-type'] || '';
      if (/text|json|javascript/.test(type)) bodies.push(await r.text());
    } catch (e) { /* a redirect or a closed page has no body */ }
  });
  return bodies;
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

async function addCardsBlock(owner, cid, etid) {
  const got = await owner.api(`/campaigns/${cid}/entity-types/${etid}/layout`);
  const layout = got.body.layout && got.body.layout.rows && got.body.layout.rows.length ? got.body.layout
    : { rows: [{ id: 'row-1', columns: [{ id: 'col-1-1', width: 12, blocks: [{ id: 'blk-title', type: 'title' }, { id: 'blk-entry', type: 'entry' }] }] }] };
  layout.rows.push({ id: 'row-cards', columns: [{ id: 'row-cards-col', width: 12, blocks: [{ id: 'blk-cards', type: 'hero_cards' }] }] });
  const put = await owner.api(`/campaigns/${cid}/entity-types/${etid}/layout`, { method: 'PUT', body: JSON.stringify({ layout }), headers: JSON_HEADERS });
  assert.ok(put.ok, 'could not put the Cards part on the layout: ' + JSON.stringify(put.body));
}

// A table of one Director and two players, two heroes (claimed), a hero deck
// that lets others see shown faces, and a party deck with a card that asks first.
async function table(tag, { width, height, scheme }) {
  const ctx = { viewport: { width, height }, colorScheme: scheme, reducedMotion: 'no-preference' };
  const owner = await person({ tag: tag + '-d', name: 'Director Dara', context: { reducedMotion: 'no-preference' } });
  const tam = await person({ tag: tag + '-t', name: 'Tam Player', context: ctx });
  const ilse = await person({ tag: tag + '-i', name: 'Ilse Player', context: ctx });
  const cid = await newCampaign(owner, 'Hands ' + tag);
  await enableAddons(owner, cid, '*');
  await addMember(owner, cid, tam, 'player');
  await addMember(owner, cid, ilse, 'player');
  const heroTam = await newHero(owner, cid, 'Tam Hero');
  const heroIlse = await newHero(owner, cid, 'Ilse Hero');
  for (const [who, hero] of [[tam, heroTam], [ilse, heroIlse]]) {
    await who.go(`/campaigns/${cid}`);
    const claim = await who.api(`/campaigns/${cid}/entities/${hero.id}/claim`, { method: 'POST' });
    assert.ok(claim.ok, 'could not claim a hero: ' + JSON.stringify(claim.body));
  }

  const A = `/campaigns/${cid}/decks`;
  const omens = (await j(owner, A, 'POST', { name: 'Omens', kind: 'hero', settings: { knows: 'count', othersSee: 'face', arrives: 'down' } })).body.id;
  await j(owner, `${A}/${omens}`, 'PUT', { version: 1, look: { kinds: [{ key: 'bane', name: 'Zzbane', colour: 'rose', sign: 'star' }] } });
  const pic = await owner.page.evaluate(async ({ cid, PNG }) => {
    const bytes = Uint8Array.from(atob(PNG), (c) => c.charCodeAt(0));
    const form = new FormData();
    form.append('campaign_id', cid); form.append('usage_type', 'attachment');
    form.append('file', new Blob([bytes], { type: 'image/png' }), 'moon.png');
    const res = await Chronicle.apiFetch('/media/upload', { method: 'POST', body: form });
    return (await res.json()).id;
  }, { cid, PNG });
  const made = await j(owner, `${A}/${omens}/cards`, 'POST', { name: 'Moonwellzz', words: 'Wordsofthemoonwell', kindKey: 'bane', corner: '7', copies: 2 });
  const moon = made.body.cards.find((c) => c.name === 'Moonwellzz').id;
  await j(owner, `${A}/${omens}/cards/${moon}`, 'PUT', { picture: pic });
  const fates = (await j(owner, A, 'POST', { name: 'Fates', kind: 'party', settings: { arrives: 'up', fate: 'play' } })).body.id;
  const fav = (await j(owner, `${A}/${fates}/cards`, 'POST', { name: 'Favourowed', words: 'A debt is called in', copies: 1, askFirst: true })).body.cards[0].id;

  // Both players are chosen for both decks.
  const ids = {};
  await owner.go(`${A}/workshop/${omens}?tab=players`);
  for (const [n, k] of [['Tam Player', 'tam'], ['Ilse Player', 'ilse']]) {
    ids[k] = await owner.page.evaluate((name) => [...document.querySelectorAll('.dkw-prow')].find((r) => r.textContent.includes(name)).getAttribute('data-user'), n);
  }
  for (const d of [omens, fates]) {
    const v = (await owner.api(`${A}/${d}`)).body.version;
    const put = await j(owner, `${A}/${d}/players`, 'PUT', { version: v, players: [{ userId: ids.tam }, { userId: ids.ilse }] });
    assert.ok(put.ok, 'could not choose the players: ' + JSON.stringify(put.body));
  }
  await addCardsBlock(owner, cid, heroTam.etid);
  return { owner, tam, ilse, cid, A, omens, fates, moon, fav, pic, heroTam: heroTam.id, heroIlse: heroIlse.id, scheme, width };
}

const noSideways = (page, what) => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  .then((w) => assert.ok(w <= 0, `${what}: the page scrolls sideways by ${w}px`));

const hand = (who) => who.page.locator('#tt-drawer [data-dkh-root]').first();

async function openHand(who, cid) {
  await who.go(`/campaigns/${cid}?table=hand`);
  await hand(who).waitFor();
}

async function shot(who, name, w) {
  if (SHOTS) await who.page.waitForTimeout(500);
  if (SHOTS) await who.page.screenshot({ path: `${SHOTS}/hand-${name}-${w.scheme}-${w.width}.png` });
}

async function walk(tag, view, { keyboard = false } = {}) {
  const w = await table(tag, view);
  const { owner, tam, ilse, cid, A } = w;
  const tamErrors = watchErrors(tam), ilseErrors = watchErrors(ilse);
  const bodies = recordBodies(tam);
  const ilseBodies = recordBodies(ilse);

  // Empty: an honest line, no sideways scroll.
  await openHand(tam, cid);
  await openHand(ilse, cid);
  assert.match(await hand(tam).innerText(), /No cards yet\. Your Director deals them\./);
  await noSideways(tam.page, 'the empty Hand leaf');
  await shot(tam, 'empty', view);

  // A Director deals one face down to Tam's hero, from a second browser.
  const dealt = await j(owner, `${A}/${w.omens}/deal-card`, 'POST', { cardId: w.moon, to: 'hand', heroId: w.heroTam });
  assert.ok(dealt.ok, 'the deal failed: ' + JSON.stringify(dealt.body));
  const slot = hand(tam).locator('[data-dkh-slot]');
  await slot.first().waitFor();
  assert.equal(await slot.count(), 1);
  assert.ok(await hand(tam).locator('.dkh-chip-new').isVisible(), 'the new card is not marked new');
  assert.match(await hand(tam).innerText(), /A new card from Omens\. Tap it to turn it over\./);
  assert.equal(await ilse.page.locator('#tt-drawer [data-dkh-slot]').count(), 0, 'Ilse was shown Tam’s face-down card as one of her own');
  await noSideways(tam.page, 'a new card');
  await shot(tam, 'new-card', view);

  // Before the tap: none of the card in the page or anything the browser received.
  const secrets = ['Moonwellzz', 'Wordsofthemoonwell', 'Zzbane', w.pic];
  const html = await tam.page.content();
  for (const s of secrets) {
    assert.ok(!html.includes(s), `the page holds ${s} before the tap`);
    assert.ok(!bodies.some((b) => b.includes(s)), `a response held ${s} before the tap`);
    assert.ok(!ilseBodies.some((b) => b.includes(s)), `Ilse's browser was sent ${s}`);
  }
  assert.equal(await hand(tam).locator('template, .dk-face-tpl').count(), 0, 'a template is in the hand before the tap');
  assert.deepEqual(await hand(tam).locator('.dk-turn').evaluateAll((bs) => bs.map((b) => b.getAttribute('data-face-src') || '')), ['']);

  // The tap turns it, by mouse or by keyboard.
  const turnReq = tam.page.waitForRequest((r) => r.method() === 'POST' && /\/turn$/.test(r.url()));
  const turn = hand(tam).locator('.dk-turn').first();
  if (keyboard) { await turn.focus(); await tam.page.keyboard.press('Enter'); } else { await turn.click(); }
  await turnReq;
  await hand(tam).locator('.dk-plate').first().waitFor();
  assert.match(await hand(tam).innerText(), /moonwellzz/i);
  await noSideways(tam.page, 'a turned card');
  await shot(tam, 'face', view);
  // Turning is private: Ilse reads nothing of it.
  assert.ok(!(await ilse.page.content()).includes('Moonwellzz'), 'the face reached Ilse before it was shown');

  // Open it: the card at full size with its controls, Keep it closes.
  const open = hand(tam).locator('[data-dkh-open]').first();
  await open.waitFor();
  if (keyboard) { await open.focus(); await tam.page.keyboard.press('Enter'); } else { await open.click(); }
  const over = tam.page.locator('[data-dkh-over]');
  await over.waitFor();
  assert.match(await over.innerText(), /wordsofthemoonwell/i);
  const names = await over.locator('.dkh-acts button').allInnerTexts();
  assert.deepEqual(names.map((n) => n.trim()).slice(0, 2), ['Play it', 'Show the table']);
  const heights = await over.locator('.dkh-acts button').evaluateAll((bs) => bs.map((b) => b.getBoundingClientRect().height));
  if (view.width < 640) for (const h of heights) assert.ok(h >= 44, `a control is ${h}px tall on a phone`);
  await noSideways(tam.page, 'the opened card');
  await shot(tam, 'opened', view);
  await over.getByRole('button', { name: 'Keep it' }).click();
  await over.waitFor({ state: 'detached' });

  // Show the table: another player's screen shows the face without a reload.
  await open.click();
  await over.getByRole('button', { name: 'Show the table' }).click();
  await over.locator('[data-dkh-state]').getByText('Shown to the table').waitFor();
  assert.equal(await over.getByRole('button', { name: 'Show the table' }).count(), 0, 'a shown card can be shown again');
  await ilse.page.locator('#tt-drawer').getByText('Moonwellzz').first().waitFor();
  await over.getByRole('button', { name: 'Keep it' }).click();

  // The Director takes it back: Tam is told on the bell, and the card is named
  // because it was face up to Tam.
  const dd = await owner.api(`${A}/${w.omens}`);
  const copy = dd.body.copies.find((c) => c.zone === 'hand').id;
  assert.ok((await j(owner, `${A}/${w.omens}/copies/${copy}/take-back`, 'POST', {})).ok);
  await hand(tam).locator('[data-dkh-slot]').first().waitFor({ state: 'detached' });
  const bell = (await tam.api('/notifications')).body;
  const text = JSON.stringify(bell);
  assert.match(text, /A card was taken back from Omens: Moonwellzz/);

  // The party's deck: a card that asks first, from the party's hand.
  const fd = await j(owner, `${A}/${w.fates}/deal-card`, 'POST', { cardId: w.fav, to: 'hand' });
  assert.ok(fd.ok, 'the party deal failed: ' + JSON.stringify(fd.body));
  await tam.page.locator('#tt-drawer').getByText("The party's hand").first().waitFor();
  await ilse.page.locator('#tt-drawer').getByText("The party's hand").first().waitFor();
  await shot(tam, 'party', view);
  await hand(tam).locator('[data-dkh-open]').first().click();
  await over.getByRole('button', { name: 'Ask to play' }).click();
  await over.getByText('Waiting for your Director').first().waitFor();
  assert.equal(await over.getByRole('button', { name: 'Ask to play' }).count(), 0, 'the card can be asked twice');
  await shot(tam, 'waiting', view);
  await over.getByRole('button', { name: 'Keep it' }).click();

  // The Director sees who asks, and lets them.
  await owner.go(`${A}/workshop/${w.fates}`);
  await owner.page.getByText('asks to play this').first().waitFor();
  const asked = JSON.stringify((await owner.api('/notifications')).body);
  assert.match(asked, /asks to play Favourowed from Fates/);
  await owner.page.getByRole('button', { name: 'Let them' }).click();
  await owner.page.getByText('asks to play this').first().waitFor({ state: 'detached' });
  for (const p of [tam, ilse]) {
    await p.page.locator('#tt-drawer').getByText('On the table').first().waitFor();
    assert.match(await p.page.locator('#tt-drawer').innerText(), /favourowed/i);
    // A play the Director let through is never called a refusal.
    assert.doesNotMatch(await p.page.locator('#tt-drawer').innerText(), /turned that down/i);
  }

  // The Cards part of Tam's hero page: Tam's own strip, and Ilse's read-only one.
  await tam.go(`/campaigns/${cid}/entities/${w.heroTam}`);
  await tam.page.locator('.dkh-strip').first().waitFor();
  await tam.page.locator('.dkh-strip').first().scrollIntoViewIfNeeded();
  await shot(tam, 'hero-part', view);
  await ilse.go(`/campaigns/${cid}/entities/${w.heroTam}`);
  await ilse.page.locator('.dkh-strip').first().waitFor();
  assert.equal(await ilse.page.locator('.dkh-strip [data-dkh-open], .dkh-strip .dk-turn').count(), 0, 'another player was offered a control on Tam’s cards');

  assert.deepEqual(tamErrors, [], 'Tam saw console errors');
  assert.deepEqual(ilseErrors, [], 'Ilse saw console errors');
  tam.assertClean(); ilse.assertClean(); owner.assertClean();
}

test('on a phone, in light: deal, turn, open, show, take back, ask and be let', async () => {
  await walk('dhp', { width: 390, height: 844, scheme: 'light' });
});

test('on a desktop, in dark: the same walk', async () => {
  await walk('dhd', { width: 1280, height: 900, scheme: 'dark' });
});

test('by keyboard: turning and opening need no mouse', async () => {
  await walk('dhk', { width: 1280, height: 900, scheme: 'light' }, { keyboard: true });
});

test('a stranger to the table and a signed-out visitor are told nothing is there', async () => {
  const owner = await person({ tag: 'dhs-d', name: 'Director Dara' });
  const stranger = await person({ tag: 'dhs-s', name: 'Stranger Sol' });
  const cid = await newCampaign(owner, 'Hands closed');
  await enableAddons(owner, cid, '*');
  const url = `/campaigns/${cid}/decks/hand`;
  assert.equal((await stranger.api(url)).status, 403, 'a non-member read the hand');
  const out = await owner.page.context().browser().newContext();
  const anon = await out.newPage();
  const res = await anon.goto(new URL(url, owner.page.url()).href);
  assert.ok(res.status() !== 200 || /login|sign in/i.test(await anon.content()), 'a signed-out visitor read the hand');
  await out.close();
});
