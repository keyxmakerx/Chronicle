// token_maker.spec.mjs — a creature token made with the token maker from a
// page's picture: the buttons, the panel, framing, frames and colours, Save,
// the discard question, and Calm motion. Stands in for the operator check in
// #1637.
//
// Covers 1.1 to 1.7 (the hover card's corner token is checked from the card's
// own preview data), 2 and 3. The page is a Location with the token field
// added to it, since a Draw Steel creature page needs the package.
//
// Left for a person: 1.4's "that preview starts moving after a moment" is
// only checked as "the frame is offered and can be hovered" (a moving
// frame's motion is not something a still check can judge); 4 needs a real
// phone and swiping; 5 needs the Draw Steel package's creature editor
// (keyxmakerx/Chronicle-Draw-Steel #149); 6 needs a connected Foundry world.
//
// RUN: tools/live-checks.sh test/live/token_maker.spec.mjs

import { test, after } from 'node:test';
import assert from 'node:assert/strict';
import { person, newCampaign, newPage, done, picture } from './_live.mjs';

after(done);

let w;
const maker = (page) => page.locator('.tkm-p');

// A generated picture, so the check needs no file: a gold head on a blue field.
async function givePicture(owner, cid, eid) {
  return owner.page.evaluate(async ({ cid, eid }) => {
    const c = document.createElement('canvas'); c.width = c.height = 400;
    const x = c.getContext('2d');
    x.fillStyle = '#2b3a55'; x.fillRect(0, 0, 400, 400);
    x.fillStyle = '#d8a657'; x.beginPath(); x.arc(200, 170, 110, 0, 7); x.fill();
    x.fillStyle = '#8a2c2c'; x.fillRect(110, 260, 180, 140);
    const blob = await new Promise((r) => c.toBlob(r, 'image/png'));
    const fd = new FormData(); fd.append('file', blob, 'grimgar.png'); fd.append('usage_type', 'entity_image'); fd.append('campaign_id', cid); fd.append('entity_id', eid);
    const up = await Chronicle.apiFetch('/media/upload', { method: 'POST', body: fd });
    const media = await up.json();
    const set = await Chronicle.apiFetch(`/campaigns/${cid}/entities/${eid}/image`, { method: 'PUT', body: { image_path: media.id } });
    // The page's type declares the token field in Draw Steel; here the page does.
    const field = await Chronicle.apiFetch(`/campaigns/${cid}/entities/${eid}/field-overrides`, { method: 'PUT', body: { added: [{ key: 'token_image', label: 'Token picture', type: 'string', section: 'Appearance' }] } });
    return [up.status, set.status, field.status];
  }, { cid, eid });
}

test('a page with a picture offers Make token beside Change, and the panel opens', async () => {
  // Full motion needs a device that has not asked for less.
  const owner = await person({ tag: 'own', name: 'Mara Vell', context: { reducedMotion: 'no-preference' } });
  const cid = await newCampaign(owner, 'Token Check');
  const eid = await newPage(owner, cid, 'Location', 'Grimgar Ironhide');
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  // Without a picture there is no token button.
  assert.equal(await owner.page.locator('[data-token-maker]').count(), 0, 'Make token shows with no picture');
  assert.deepEqual(await givePicture(owner, cid, eid), [201, 200, 200]);
  w = { owner, cid, eid };

  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  const chips = owner.page.locator('.fx-chips');
  await chips.waitFor();
  assert.deepEqual((await chips.locator('button').allInnerTexts()).map((t) => t.trim()), ['Make token', 'Change']);
  await picture(owner.page, 'token-page-picture-buttons', { check: '#1637', about: 'Page picture with Make token and Change', locator: owner.page.locator('.fx-chips').locator('..'), mask: [] });

  await chips.getByRole('button', { name: 'Make token' }).click();
  const panel = maker(owner.page);
  await panel.waitFor();
  await owner.page.locator('.tkm-fr').first().waitFor();
  assert.ok(await owner.page.locator('[data-tkm-groups] button').count() >= 3, 'no frame groups');
  assert.ok(await owner.page.locator('.tkm-pv').count() >= 6, 'no still previews of the frames');
  assert.equal(await owner.page.getByText(/could not be loaded/i).count(), 0, 'the maker showed a load error');
  assert.equal(await owner.page.locator('.tkm-p .tkm-title').innerText(), 'Token for Grimgar Ironhide');
  await owner.page.waitForFunction(() => document.querySelector('.tkm-surface') && !document.querySelector('[data-tkm-wait]:not([hidden])'));
  await picture(owner.page, 'token-maker-open', { check: '#1637', about: 'The token maker open on a creature picture', mask: ['.tkm-bg'] });
  owner.assertClean();
});

test('frames, colour and shape change the big token and the three map-size tokens; Save keeps it', async () => {
  const { owner, cid, eid } = w;
  const { page } = owner;
  const sig = () => page.evaluate(() => {
    const px = (c) => { try { return c.toDataURL().length + ':' + c.toDataURL().slice(-60); } catch (e) { return 'x'; } };
    return { big: px(document.querySelector('.tkm-surface')), small: [...document.querySelectorAll('[data-tkm-map] canvas')].map(px) };
  });

  // Scrolling zooms the picture.
  const zoom = page.locator('[data-tkm-zoom]');
  const before = await zoom.inputValue();
  const box = await page.locator('.tkm-surface').boundingBox();
  await page.mouse.move(box.x + 30, box.y + 30);
  await page.mouse.wheel(0, -400);
  await page.waitForFunction((v) => document.querySelector('[data-tkm-zoom]').value !== v, before);

  // Elements, then rest on Ring of fire.
  await page.locator('[data-tkm-groups] button', { hasText: 'Elements' }).click();
  const fire = page.locator('.tkm-fr', { hasText: 'Ring of fire' });
  await fire.waitFor();
  await fire.hover();
  await picture(page, 'token-maker-ring-of-fire', { check: '#1637', about: 'Elements frames, resting on Ring of fire', mask: ['.tkm-bg'] });

  let last = await sig();
  const changed = async (what) => {
    await page.waitForFunction(async (prev) => {
      const px = (c) => { try { return c.toDataURL().length + ':' + c.toDataURL().slice(-60); } catch (e) { return 'x'; } };
      return px(document.querySelector('.tkm-surface')) !== prev;
    }, last.big, { timeout: 5000 }).catch(() => {});
    const now = await sig();
    assert.notEqual(now.big, last.big, `the big token did not change after ${what}`);
    assert.ok(now.small.some((s, i) => s !== last.small[i]), `the map-size tokens did not change after ${what}`);
    last = now;
  };
  await fire.click();
  assert.equal(await fire.getAttribute('aria-pressed'), 'true');
  await changed('picking Ring of fire');
  await page.locator('[data-tkm-colour="#b02a24"]').click();
  assert.equal(await page.locator('[data-tkm-colour="#b02a24"]').getAttribute('aria-pressed'), 'true');
  await changed('picking red');
  await page.locator('[data-tkm-shape] button', { hasText: 'Square' }).click();
  await changed('picking Square');
  await picture(page, 'token-maker-fire-red-square', { check: '#1637', about: 'Ring of fire, red, square', mask: ['.tkm-bg'] });

  // Save.
  const saveBtn = page.locator('[data-tkm-save]');
  await saveBtn.click();
  await page.getByText('Token saved.').waitFor();
  await panel_gone(page);
  assert.equal(await page.locator('.tkm-p .tkm-msg.is-error, .tkm-msg:not(:empty)').filter({ hasText: /not saved|could not/i }).count(), 0);

  // The token is on the page's token field, and the page picture is unchanged.
  const fields = (await owner.api(`/campaigns/${cid}/entities/${eid}/preview`)).body;
  const text = JSON.stringify(fields);
  assert.match(text, /token|badge/i, 'the preview carries no token');
  const picOk = await page.locator('.fx-chips').locator('..').locator('img').getAttribute('src');
  assert.ok(picOk, 'the page picture is gone');

  // A link to the page shows the framed token on the corner of the hover card.
  const other = await newPage(owner, cid, 'Location', 'Ravenhold');
  const url = `/campaigns/${cid}/entities/${eid}`;
  const doc = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hunted by ' }, { type: 'text', text: 'Grimgar Ironhide', marks: [{ type: 'link', attrs: { href: url, 'data-mention-id': eid, 'data-entity-preview': url + '/preview' } }] }] }] };
  await owner.api(`/campaigns/${cid}/entities/${other}/entry`, { method: 'PUT', body: { entry: JSON.stringify(doc), entry_html: '' } });
  await owner.go(`/campaigns/${cid}/entities/${other}`);
  await page.locator('a[data-mention-id]').first().hover();
  await page.locator('#chc .chc__badge').waitFor();
  await picture(page, 'token-hover-card-badge', { check: '#1637', about: 'Hover card with the new framed token on the corner of the picture', locator: page.locator('#chc') });
  owner.assertClean();
});

async function panel_gone(page) { await page.locator('.tkm-p').waitFor({ state: 'detached' }); }

test('leaving with changes asks before discarding; a second Escape returns to the maker', async () => {
  const { owner, cid, eid } = w;
  const { page } = owner;
  await owner.go(`/campaigns/${cid}/entities/${eid}`);
  await page.getByRole('button', { name: 'Make token' }).click();
  await page.locator('.tkm-fr').first().waitFor();
  await page.waitForFunction(() => !document.querySelector('[data-tkm-wait]:not([hidden])'));
  const frames = page.locator('.tkm-fr');
  await frames.nth(2).click();
  await page.keyboard.press('Escape');
  const ask = page.getByText('Discard this token?');
  await ask.waitFor();
  await picture(page, 'token-maker-discard-question', { check: '#1637', about: 'The discard question after Escape with unsaved changes', mask: ['.tkm-bg'] });
  await page.keyboard.press('Escape');
  await ask.waitFor({ state: 'hidden' });
  assert.ok(await page.locator('.tkm-p').isVisible(), 'the second Escape closed the maker');
  assert.equal(await frames.nth(2).getAttribute('aria-pressed'), 'true', 'the work was lost');
  await page.locator('[data-tkm-close]').last().click();
  await ask.waitFor();
  await page.getByRole('button', { name: 'Discard the token' }).click();
  await panel_gone(page);
  owner.assertClean();
});

test('Calm motion fades the maker in and a moving frame holds still with a note', async () => {
  const { owner, cid, eid } = w;
  const { page } = owner;
  const set = (v) => owner.api('/account/view-prefs', { method: 'PUT', body: { motion: v } });
  assert.ok((await set('calm')).ok, 'could not set Motion to Calm');
  try {
    await owner.go(`/campaigns/${cid}/entities/${eid}`);
    await page.getByRole('button', { name: 'Make token' }).click();
    await page.locator('.tkm').first().waitFor();
    await page.waitForFunction(() => document.querySelector('.tkm') && document.querySelector('.tkm').classList.contains('is-calm'), null, { timeout: 5000 });
    await page.locator('.tkm-fr').first().waitFor();
    await page.locator('[data-tkm-groups] button', { hasText: 'Elements' }).click();
    await page.locator('.tkm-fr', { hasText: 'Ring of fire' }).click();
    const note = await page.locator('[data-tkm-note]').innerText();
    assert.match(note, /still|hold|calm|motion/i, 'no note says the frame holds still: ' + note);
    await page.locator('[data-tkm-close]').last().click();
    const discard = page.getByRole('button', { name: 'Discard the token' });
    if (await discard.count()) await discard.click();
  } finally {
    await set('owner');
  }
  owner.assertClean();
});
