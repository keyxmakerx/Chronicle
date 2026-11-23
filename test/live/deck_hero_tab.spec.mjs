// deck_hero_tab.spec.mjs: the Cards part of a hero's page when a game system
// draws the page.
//
// A page a game system draws has no layout, so no layout block shows; its
// pieces share one tabbed panel instead. The Cards part offers itself there as
// a tab. This walk installs the Draw Steel package (the server fetches it from
// GitHub; with GitHub out of reach the walk is skipped), makes a hero page
// that package draws, and checks, with no Cards block on any layout:
//   - nothing dealt: no Cards tab for the hero's player, a Director or another
//     player;
//   - a card dealt face down: the hero's player has the tab, open on load,
//     counting 1, shows the card's back with the turn control, and a tap turns
//     it; a Director has the tab and no control; another player has the tab
//     (counts only) and no control;
//   - a card dealt while the tab is not the one showing arrives in it, and the
//     count follows;
//   - the Card decks add-on off: no tab for anyone.
//
// RUN: tools/live-checks.sh test/live/deck_hero_tab.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, siteAdmin, newCampaign, addMember, enableAddons, done, ADMIN } from './_live.mjs';

after(done);

const JSON_HEADERS = { 'Content-Type': 'application/json' };
const j = (who, url, method, body) => who.api(url, { method, body: body === undefined ? undefined : JSON.stringify(body), headers: JSON_HEADERS });

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

async function newHero(owner, cid, name) {
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

const tab = (who) => who.page.locator('[data-widget="page-tabs"] [data-pt-tab="hero_cards"]');
const pane = (who) => who.page.locator('[data-widget="page-tabs"] [data-pt-pane="hero_cards"]');

// The hero page, once the tabbed panel is on it.
async function openHero(who, cid, hero) {
  await who.go(`/campaigns/${cid}/entities/${hero}`);
  await who.page.locator('[data-widget="page-tabs"] [data-pt-tab]').first().waitFor();
}

test('the Cards part is a tab under a hero page a game system draws', async (t) => {
  const unreachable = await installDrawSteel();
  if (unreachable) { t.skip('this server could not fetch the Draw Steel package: ' + unreachable); return; }

  const owner = await person({ tag: 'dht-d', name: 'Director Dara' });
  const tam = await person({ tag: 'dht-t', name: 'Tam Player' });
  const ilse = await person({ tag: 'dht-i', name: 'Ilse Player' });
  const cid = await newCampaign(owner, 'Cards tab');
  await enableAddons(owner, cid, '*');
  await enableAddons(owner, cid, ['drawsteel']);
  await addMember(owner, cid, tam, 'player');
  await addMember(owner, cid, ilse, 'player');
  const hero = await newHero(owner, cid, 'Tam Hero');
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
  await openHero(tam, cid, hero);
  assert.ok(await tam.page.locator('[data-widget="page-tabs"]').count(), 'the page has no tabbed panel, so the game system did not draw it');
  assert.equal(await tam.page.locator('.dkh-strip').count(), 0, 'a Cards strip is on the page before anything is dealt');

  // Nothing dealt: no tab for anyone.
  for (const who of [tam, owner, ilse]) {
    await openHero(who, cid, hero);
    assert.equal(await tab(who).count(), 0, 'the Cards tab is offered with no cards to show');
  }

  // A card dealt face down.
  const dealt = await j(owner, `${A}/${omens}/deal-card`, 'POST', { cardId: card, to: 'hand', heroId: hero });
  assert.ok(dealt.ok, 'the deal failed: ' + JSON.stringify(dealt.body));

  // The hero's player: the tab, open on load, counting one, the back of the card.
  await openHero(tam, cid, hero);
  await tab(tam).waitFor();
  assert.match(await tab(tam).innerText(), /^Cards\s*1$/i);
  await tam.page.waitForFunction(() => document.querySelector('[data-pt-tab="hero_cards"]')?.getAttribute('aria-selected') === 'true');
  await pane(tam).locator('[data-widget="deck-hand"] .dkh-item').first().waitFor();
  assert.equal(await pane(tam).locator('.dkh-item').count(), 1);
  assert.equal(await pane(tam).locator('[data-pt-press]').count() + await tam.page.locator('[data-pt-press]:visible').count(), 0, 'the Cards tab offers the panel an action button');
  const html = await tam.page.content();
  assert.ok(!html.includes('Moonwellzz') && !html.includes('Wordsofthemoonwell'), 'the page holds the card before the tap');
  const turn = pane(tam).locator('.dk-turn').first();
  await turn.waitFor();
  const turnReq = tam.page.waitForRequest((r) => r.method() === 'POST' && /\/turn$/.test(r.url()));
  await turn.click();
  await turnReq;
  await pane(tam).locator('.dk-plate').first().waitFor();
  assert.match(await pane(tam).innerText(), /moonwellzz/i);

  // A Director reads it and has no control.
  await openHero(owner, cid, hero);
  await tab(owner).waitFor();
  assert.match(await tab(owner).innerText(), /^Cards\s*1$/i);
  await pane(owner).locator('.dkh-item').first().waitFor();
  assert.equal(await pane(owner).locator('[data-dkh-open], .dk-turn').count(), 0, 'a Director was offered a control on the hero’s cards');

  // Another player: counts and faces only, and no control.
  await openHero(ilse, cid, hero);
  await tab(ilse).waitFor();
  await pane(ilse).locator('.dkh-item').first().waitFor();
  assert.equal(await pane(ilse).locator('[data-dkh-open], .dk-turn').count(), 0, 'another player was offered a control on Tam’s cards');
  assert.ok(!(await ilse.page.content()).includes('Wordsofthemoonwell'), 'another player was sent the words of a card');

  // A card dealt while another tab is showing arrives in the Cards pane, and
  // the count follows.
  await openHero(tam, cid, hero);
  await tam.page.locator('[data-pt-tab="notes"]').click();
  assert.ok(await pane(tam).isHidden(), 'the Cards pane is still showing');
  const second = await j(owner, `${A}/${omens}/deal-card`, 'POST', { cardId: card, to: 'hand', heroId: hero });
  assert.ok(second.ok, 'the second deal failed: ' + JSON.stringify(second.body));
  await tam.page.waitForFunction(() => document.querySelector('[data-pt-tab="hero_cards"] [data-pt-n]')?.textContent === '2');
  await tab(tam).click();
  assert.equal(await pane(tam).locator('.dkh-item').count(), 2);
  const turnNew = pane(tam).locator('.dk-turn').first();
  await turnNew.waitFor();
  await turnNew.click();
  await pane(tam).locator('.dk-plate').nth(1).waitFor();

  // Card decks off: no tab for anyone.
  const list = (await owner.api(`/campaigns/${cid}/addons`)).body;
  const decksAddon = list.find((a) => (a.addon_slug || a.slug) === 'decks');
  assert.ok(decksAddon, 'the Card decks add-on is not listed');
  const off = await owner.api(`/campaigns/${cid}/addons/${decksAddon.addon_id}/toggle`, { method: 'PUT', form: { action: 'disable' } });
  assert.ok(off.ok, 'turning the add-on off answered ' + off.status);
  for (const who of [tam, owner, ilse]) {
    await openHero(who, cid, hero);
    assert.equal(await tab(who).count(), 0, 'the Cards tab is offered with the add-on off');
    assert.equal(await who.page.locator('.dkh-strip').count(), 0, 'a Cards strip is drawn with the add-on off');
  }

  tam.assertClean(); ilse.assertClean(); owner.assertClean();
});
