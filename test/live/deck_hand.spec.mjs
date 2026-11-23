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
// The Cards part of a hero's page when a game system draws the page is the last
// walk, in a campaign of its own with the same three people.
//
// Set LIVE_SHOTS to a folder to also save pictures there.
//
// RUN: tools/live-checks.sh test/live/deck_hand.spec.mjs

import { test, after, beforeEach } from 'node:test';
import assert from 'node:assert/strict';
import { person, siteAdmin, newCampaign, addMember, enableAddons, done, once, resetErrors, ADMIN } from './_live.mjs';

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

// One Director, two players with a claimed hero each and one campaign serve every
// walk in the file; each walk makes its own decks, so the people and heroes are
// signed up and made once.
const world = once(async () => {
  const owner = await person({ tag: 'dh-d', name: 'Director Dara', context: { reducedMotion: 'no-preference' } });
  const tam = await person({ tag: 'dh-t', name: 'Tam Player', context: { reducedMotion: 'no-preference' } });
  const ilse = await person({ tag: 'dh-i', name: 'Ilse Player', context: { reducedMotion: 'no-preference' } });
  const cid = await newCampaign(owner, 'Hands');
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
  await addCardsBlock(owner, cid, heroTam.etid);
  return { owner, tam, ilse, cid, heroTam, heroIlse, decks: [] };
});

beforeEach(async () => { const w = await world(); resetErrors(w.owner, w.tam, w.ilse); });

// The decks of one walk: a hero deck that lets others see shown faces, and a
// party deck with a card that asks first. The players are shown at `view`'s size
// and in its colour scheme.
async function table(tag, { width, height, scheme }) {
  const base = await world();
  const { owner, tam, ilse, cid } = base;
  void tag;
  for (const who of [tam, ilse]) {
    await who.page.setViewportSize({ width, height });
    await who.page.emulateMedia({ colorScheme: scheme });
  }
  const A = `/campaigns/${cid}/decks`;
  // An earlier walk's decks would put their cards in this walk's hands.
  for (const old of base.decks.splice(0)) await owner.api(`${A}/${old}`, { method: 'DELETE' });

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
  base.decks.push(omens, fates);

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
  return { owner, tam, ilse, cid, A, omens, fates, moon, fav, pic, heroTam: base.heroTam.id, heroIlse: base.heroIlse.id, scheme, width };
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
  // The bell keeps earlier walks' lines, so a new one is counted, not just found.
  const bellHas = async (who, line) => JSON.stringify((await who.api('/notifications')).body).split(line).length - 1;
  const TAKEN = 'A card was taken back from Omens: Moonwellzz';
  const takenBefore = await bellHas(tam, TAKEN);
  const dd = await owner.api(`${A}/${w.omens}`);
  const copy = dd.body.copies.find((c) => c.zone === 'hand').id;
  assert.ok((await j(owner, `${A}/${w.omens}/copies/${copy}/take-back`, 'POST', {})).ok);
  await hand(tam).locator('[data-dkh-slot]').first().waitFor({ state: 'detached' });
  assert.equal(await bellHas(tam, TAKEN), takenBefore + 1, 'the bell did not say the card was taken back');

  // The party's deck: a card that asks first, from the party's hand.
  const fd = await j(owner, `${A}/${w.fates}/deal-card`, 'POST', { cardId: w.fav, to: 'hand' });
  assert.ok(fd.ok, 'the party deal failed: ' + JSON.stringify(fd.body));
  await tam.page.locator('#tt-drawer').getByText("The party's hand").first().waitFor();
  await ilse.page.locator('#tt-drawer').getByText("The party's hand").first().waitFor();
  await shot(tam, 'party', view);
  const askedBefore = await bellHas(owner, 'asks to play Favourowed from Fates');
  await hand(tam).locator('[data-dkh-open]').first().click();
  await over.getByRole('button', { name: 'Ask to play' }).click();
  await over.getByText('Waiting for your Director').first().waitFor();
  assert.equal(await over.getByRole('button', { name: 'Ask to play' }).count(), 0, 'the card can be asked twice');
  await shot(tam, 'waiting', view);
  await over.getByRole('button', { name: 'Keep it' }).click();

  // The Director sees who asks, and lets them.
  await owner.go(`${A}/workshop/${w.fates}`);
  await owner.page.getByText('asks to play this').first().waitFor();
  assert.equal(await bellHas(owner, 'asks to play Favourowed from Fates'), askedBefore + 1, 'the bell did not say who asks to play');
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

test('on a phone, a hand of twelve scrolls inside its row, says its count, and an offline tap answers in view', async () => {
  const view = { width: 390, height: 844, scheme: 'dark' };
  const w = await table('dhc', view);
  const { owner, tam, cid, A } = w;
  const errors = watchErrors(tam);
  const crowd = await j(owner, `${A}/${w.omens}/cards`, 'POST', { name: 'Crowdzz', words: 'One of many', copies: 12 });
  assert.ok(crowd.ok, 'the twelve-copy card was not made: ' + JSON.stringify(crowd.body));
  const card = crowd.body.cards.find((c) => c.name === 'Crowdzz').id;
  for (let i = 0; i < 12; i++) {
    const dealt = await j(owner, `${A}/${w.omens}/deal-card`, 'POST', { cardId: card, to: 'hand', heroId: w.heroTam });
    assert.ok(dealt.ok, 'a deal failed: ' + JSON.stringify(dealt.body));
  }
  await openHand(tam, cid);
  await tam.page.waitForFunction(() => document.querySelectorAll('#tt-drawer [data-dkh-slot]').length === 12);
  const row = tam.page.locator('#tt-drawer [data-dkh-held]').first();
  const read = () => row.evaluate((r) => ({ sw: r.scrollWidth, cw: r.clientWidth, more: r.getAttribute('data-dkh-more'), label: r.parentNode.querySelector('.dkh-lab').textContent.replace(/\s+/g, ' ').trim() }));
  await tam.page.waitForFunction(() => /· 12/.test(document.querySelector('#tt-drawer .dkh-lab').textContent));
  let r = await read();
  assert.ok(r.sw > r.cw, 'the row of twelve does not scroll inside itself');
  assert.equal(r.more, 'end', 'the trailing edge does not fade');
  assert.equal(r.label, 'Your hand · 12');
  await noSideways(tam.page, 'a hand of twelve');
  await shot(tam, 'twelve', view);
  await row.evaluate((el) => { el.scrollLeft = el.scrollWidth; });
  await tam.page.waitForFunction(() => document.querySelector('#tt-drawer [data-dkh-held]').getAttribute('data-dkh-more') === 'start');
  await row.evaluate((el) => { el.scrollLeft = 0; });
  await tam.page.waitForFunction(() => document.querySelector('#tt-drawer [data-dkh-held]').getAttribute('data-dkh-more') === 'end');

  // Offline, a tap on a face-down card says so under the row, inside the screen.
  await tam.page.context().setOffline(true);
  await row.locator('.dk-turn').nth(2).click();
  const err = tam.page.locator('#tt-drawer [data-dkh-err]:not([hidden])');
  await err.waitFor();
  assert.match(await err.innerText(), /That did not go through\. Check your connection and try again\./);
  assert.equal(await err.getAttribute('role'), 'alert');
  const box = await err.evaluate((e) => { const b = e.getBoundingClientRect(); return { top: b.top, bottom: b.bottom, h: innerHeight, under: e.previousElementSibling === document.querySelector('#tt-drawer [data-dkh-held]') }; });
  assert.ok(box.under, 'the line is not right under the hand');
  assert.ok(box.top >= 0 && box.bottom <= box.h, `the line is off screen: ${box.top}..${box.bottom} of ${box.h}`);
  await shot(tam, 'twelve-offline', view);
  await tam.page.context().setOffline(false);
  assert.equal(await tam.page.locator('#tt-drawer [data-dkh-err]').count(), 1, 'the alert was duplicated');
  await noSideways(tam.page, 'the offline line');
  assert.deepEqual(errors.filter((e) => !/Failed to fetch|ERR_INTERNET_DISCONNECTED/i.test(e)), [], 'Tam saw console errors');
});

test('a stranger to the table and a signed-out visitor are told nothing is there', async () => {
  const { owner, cid } = await world();
  const stranger = await person({ tag: 'dhs-s', name: 'Stranger Sol' });
  const url = `/campaigns/${cid}/decks/hand`;
  assert.equal((await stranger.api(url)).status, 403, 'a non-member read the hand');
  const out = await owner.page.context().browser().newContext();
  const anon = await out.newPage();
  const res = await anon.goto(new URL(url, owner.page.url()).href);
  assert.ok(res.status() !== 200 || /login|sign in/i.test(await anon.content()), 'a signed-out visitor read the hand');
  await out.close();
});

// The Cards part as a tab on a hero page a game system draws. The walk installs
// the Draw Steel package (the server fetches it from GitHub; with GitHub out of
// reach it is skipped). With no Cards block on any layout: nothing dealt, no
// Cards tab for anyone; a card dealt face down gives the hero's player the tab,
// open on load, counting 1, with the card's back and the turn control; a
// Director has the tab and no control; another player has the tab (counts only)
// and no control; a card dealt while the tab is not showing arrives in it and
// the count follows; with the Card decks add-on off, no tab for anyone.
// The package is added once for the whole server; the admin must have
// confirmed their password within the last minutes first.
async function installDrawSteel() {
  const admin = await siteAdmin();
  await admin.go('/admin/packages');
  const reauth = await admin.api('/account/reauth', { method: 'POST', form: { password: ADMIN.password } });
  assert.ok(reauth.ok, 'the admin could not confirm their password: ' + JSON.stringify(reauth.body));
  const have = await admin.api('/admin/packages', { headers: { Accept: 'application/json' } });
  if (!/Chronicle-Draw-Steel/i.test(typeof have.body === 'string' ? have.body : JSON.stringify(have.body))) {
    const added = await admin.api('/admin/packages', { method: 'POST', form: { repo_url: 'https://github.com/keyxmakerx/Chronicle-Draw-Steel', type: 'system' } });
    const said = JSON.stringify(added.body).slice(0, 300);
    // GitHub out of reach or rate limited is the run's surroundings, not a
    // fault in the page: the walk is skipped and says why.
    if (!added.ok && /GitHub (didn't respond|had a temporary problem)|GitHub's rate limit/.test(said)) {
      await admin.ctx.close();
      return said;
    }
    assert.ok(added.ok, 'the Draw Steel package was not added: ' + said);
  }
  // Adding still succeeds when GitHub gives no answer (the server tries again
  // later), so the package's own version list is what says the releases
  // arrived. Draw Steel has releases; none listed means GitHub was out of
  // reach. Listed but none installed is a real install failure, and the walk
  // fails at the switch below with the server's words.
  const page = await admin.api('/admin/packages');
  const html = typeof page.body === 'string' ? page.body : JSON.stringify(page.body);
  const id = (html.match(/[?&](?:amp;)?pkg=([0-9a-f-]{36})/) || [])[1];
  assert.ok(id, 'the Draw Steel package is not on the packages page');
  const versions = await admin.api(`/admin/packages/${id}/versions`);
  const listed = typeof versions.body === 'string' ? versions.body : JSON.stringify(versions.body);
  if (!/font-mono font-bold[^>]*>\s*\d+\.\d+\.\d+/.test(listed)) {
    await admin.ctx.close();
    return 'the server added the package but got no releases from GitHub';
  }
  admin.assertClean();
  await admin.ctx.close();
  return '';
}

async function newSystemHero(owner, cid, name) {
  await owner.go(`/campaigns/${cid}/entities/new`);
  const etid = await owner.page.locator('select#entity_type_id').evaluate((el) => (([...el.options].find((o) => o.value && /character|hero/i.test(o.textContent)) || {}).value));
  const id = await owner.page.evaluate(async ({ cid, etid, name }) => {
    const form = new FormData();
    form.append('name', name); form.append('entity_type_id', etid); form.append('is_private', 'false');
    const res = await Chronicle.apiFetch(`/campaigns/${cid}/entities`, { method: 'POST', body: form });
    return (res.url.match(/entities\/([0-9a-f-]{36})/) || [])[1] || '';
  }, { cid, etid, name });
  assert.ok(id, 'the hero page was not made');
  await owner.api(`/campaigns/${cid}/entities/${id}/permissions`, { method: 'PUT', body: JSON.stringify({ visibility: 'default', is_private: false, permissions: [] }), headers: JSON_HEADERS });
  await owner.api(`/campaigns/${cid}/characters/playable`, { method: 'POST', form: { entity_id: id, playable: '1' } });
  return id;
}

const dsTab = (who) => who.page.locator('[data-widget="page-tabs"] [data-pt-tab="hero_cards"]');
const dsPane = (who) => who.page.locator('[data-widget="page-tabs"] [data-pt-pane="hero_cards"]');

// The hero page, once the tabbed panel is on it.
async function openDsHero(who, cid, hero) {
  await who.go(`/campaigns/${cid}/entities/${hero}`);
  await who.page.locator('[data-widget="page-tabs"] [data-pt-tab]').first().waitFor();
}

test('the Cards part is a tab under a hero page a game system draws', async (t) => {
  const unreachable = await installDrawSteel();
  if (unreachable) { t.skip('this server could not fetch the Draw Steel package: ' + unreachable); return; }

  // The same three people, in a campaign of their own: the game system draws
  // every hero page of the campaign it is on, which would change the walks above.
  const { owner, tam, ilse } = await world();
  for (const who of [owner, tam, ilse]) {
    await who.page.setViewportSize({ width: 1280, height: 800 });
    await who.page.emulateMedia({ colorScheme: 'light' });
  }
  const cid = await newCampaign(owner, 'Cards tab');
  await enableAddons(owner, cid, '*');
  await enableAddons(owner, cid, ['drawsteel']);
  await addMember(owner, cid, tam, 'player');
  await addMember(owner, cid, ilse, 'player');
  const hero = await newSystemHero(owner, cid, 'Tam Hero');
  await tam.go(`/campaigns/${cid}`);
  const claim = await tam.api(`/campaigns/${cid}/entities/${hero}/claim`, { method: 'POST' });
  assert.ok(claim.ok, 'could not claim the hero: ' + JSON.stringify(claim.body));

  const A = `/campaigns/${cid}/decks`;
  const omens = (await j(owner, A, 'POST', { name: 'Omens', kind: 'hero', settings: { knows: 'count', othersSee: 'face', arrives: 'down' } })).body.id;
  const made = await j(owner, `${A}/${omens}/cards`, 'POST', { name: 'Moonwellzz', words: 'Wordsofthemoonwell', copies: 3 });
  const card = made.body.cards.find((c) => c.name === 'Moonwellzz').id;
  await owner.go(`${A}/workshop/${omens}?tab=players`);
  const ids = {};
  for (const [n, k] of [['Tam Player', 'tam'], ['Ilse Player', 'ilse']]) {
    ids[k] = await owner.page.evaluate((name) => [...document.querySelectorAll('.dkw-prow')].find((r) => r.textContent.includes(name)).getAttribute('data-user'), n);
  }
  const v = (await owner.api(`${A}/${omens}`)).body.version;
  const put = await j(owner, `${A}/${omens}/players`, 'PUT', { version: v, players: [{ userId: ids.tam }, { userId: ids.ilse }] });
  assert.ok(put.ok, 'could not choose the players: ' + JSON.stringify(put.body));

  // The page is drawn by the game system: one tabbed panel, and no layout block.
  await openDsHero(tam, cid, hero);
  assert.ok(await tam.page.locator('[data-widget="page-tabs"]').count(), 'the page has no tabbed panel, so the game system did not draw it');
  assert.equal(await tam.page.locator('.dkh-strip').count(), 0, 'a Cards strip is on the page before anything is dealt');

  // Nothing dealt: no tab for anyone.
  for (const who of [tam, owner, ilse]) {
    await openDsHero(who, cid, hero);
    assert.equal(await dsTab(who).count(), 0, 'the Cards tab is offered with no cards to show');
  }

  // A card dealt face down.
  const dealt = await j(owner, `${A}/${omens}/deal-card`, 'POST', { cardId: card, to: 'hand', heroId: hero });
  assert.ok(dealt.ok, 'the deal failed: ' + JSON.stringify(dealt.body));

  // The hero's player: the tab, open on load, counting one, the back of the card.
  await openDsHero(tam, cid, hero);
  await dsTab(tam).waitFor();
  assert.match(await dsTab(tam).innerText(), /^Cards\s*1$/i);
  await tam.page.waitForFunction(() => document.querySelector('[data-pt-tab="hero_cards"]')?.getAttribute('aria-selected') === 'true');
  await dsPane(tam).locator('[data-widget="deck-hand"] .dkh-item').first().waitFor();
  assert.equal(await dsPane(tam).locator('.dkh-item').count(), 1);
  assert.equal(await dsPane(tam).locator('[data-pt-press]').count() + await tam.page.locator('[data-pt-press]:visible').count(), 0, 'the Cards tab offers the panel an action button');
  const html = await tam.page.content();
  assert.ok(!html.includes('Moonwellzz') && !html.includes('Wordsofthemoonwell'), 'the page holds the card before the tap');
  const turn = dsPane(tam).locator('.dk-turn').first();
  await turn.waitFor();
  const turnReq = tam.page.waitForRequest((r) => r.method() === 'POST' && /\/turn$/.test(r.url()));
  await turn.click();
  await turnReq;
  await dsPane(tam).locator('.dk-plate').first().waitFor();
  assert.match(await dsPane(tam).innerText(), /moonwellzz/i);

  // A Director reads it and has no control.
  await openDsHero(owner, cid, hero);
  await dsTab(owner).waitFor();
  assert.match(await dsTab(owner).innerText(), /^Cards\s*1$/i);
  await dsPane(owner).locator('.dkh-item').first().waitFor();
  assert.equal(await dsPane(owner).locator('[data-dkh-open], .dk-turn').count(), 0, 'a Director was offered a control on the hero’s cards');

  // Another player: counts and faces only, and no control.
  await openDsHero(ilse, cid, hero);
  await dsTab(ilse).waitFor();
  await dsPane(ilse).locator('.dkh-item').first().waitFor();
  assert.equal(await dsPane(ilse).locator('[data-dkh-open], .dk-turn').count(), 0, 'another player was offered a control on Tam’s cards');
  assert.ok(!(await ilse.page.content()).includes('Wordsofthemoonwell'), 'another player was sent the words of a card');

  // A card dealt while another tab is showing arrives in the Cards pane, and
  // the count follows.
  await openDsHero(tam, cid, hero);
  await tam.page.locator('[data-pt-tab="notes"]').click();
  assert.ok(await dsPane(tam).isHidden(), 'the Cards pane is still showing');
  const second = await j(owner, `${A}/${omens}/deal-card`, 'POST', { cardId: card, to: 'hand', heroId: hero });
  assert.ok(second.ok, 'the second deal failed: ' + JSON.stringify(second.body));
  await tam.page.waitForFunction(() => document.querySelector('[data-pt-tab="hero_cards"] [data-pt-n]')?.textContent === '2');
  await dsTab(tam).click();
  assert.equal(await dsPane(tam).locator('.dkh-item').count(), 2);
  const turnNew = dsPane(tam).locator('.dk-turn').first();
  await turnNew.waitFor();
  await turnNew.click();
  await dsPane(tam).locator('.dk-plate').nth(1).waitFor();

  // Card decks off: no tab for anyone.
  const list = (await owner.api(`/campaigns/${cid}/addons`)).body;
  const decksAddon = list.find((a) => (a.addon_slug || a.slug) === 'decks');
  assert.ok(decksAddon, 'the Card decks add-on is not listed');
  const off = await owner.api(`/campaigns/${cid}/addons/${decksAddon.addon_id}/toggle`, { method: 'PUT', form: { action: 'disable' } });
  assert.ok(off.ok, 'turning the add-on off answered ' + off.status);
  for (const who of [tam, owner, ilse]) {
    await openDsHero(who, cid, hero);
    assert.equal(await dsTab(who).count(), 0, 'the Cards tab is offered with the add-on off');
    assert.equal(await who.page.locator('.dkh-strip').count(), 0, 'a Cards strip is drawn with the add-on off');
  }

  tam.assertClean(); ilse.assertClean(); owner.assertClean();
});
