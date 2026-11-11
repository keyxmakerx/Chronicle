// Seeds one campaign with marked content and returns the marker table.
// Every marker is a unique string that only the owner may ever see; the crawl
// fails if one shows up for anyone else. Each seed step is independent: a step
// that cannot be done on this build is reported and skipped, so a changed form
// shows as "not seeded" instead of a silently weaker test.

import { Client } from './client.mjs';

export const RUN = Math.random().toString(36).slice(2, 7).toUpperCase();
export const mk = (key) => `ZQ${RUN}${key}`;

// Edits hidden things while viewers hold a socket open. Each edit carries a
// fresh marker; the live channel must not carry it to a viewer.
export async function liveEdits(s, log = console.log) {
  const { owner, ids, C } = s;
  const out = {};
  const mkm = (key, what) => { const k = mk(key); out[k] = what; return k; };
  const tryit = async (name, fn) => { try { await fn(); } catch (e) { log('live edit not done', name, '-', e.message); out.__skipped = [...(out.__skipped || []), name]; } };
  const J = (res, w) => { if (res.status >= 400) throw new Error(`${w} -> ${res.status}`); return res; };
  const doc = (x) => JSON.stringify({ type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: x }] }] });
  await tryit('public control', async () => {
    const t = mkm('LIVEPUBLIC', 'live control: a visible map marker');
    delete out[t]; out.__control = t;
    J(await owner.post(`${C}/maps/${ids.map}/markers`, { json: { name: t, x: 9, y: 9, icon: 'fa-flag', color: '#00ff00', visibility: 'everyone' } }), 'marker');
  });
  await tryit('hidden page text', async () => {
    const t = mkm('LIVEPAGE', 'live: edit of a hidden page');
    J(await owner.put(`${C}/entities/${ids.hidden}/entry`, { json: { entry: doc(t), entry_html: `<p>${t}</p>` } }), 'entry');
  });
  await tryit('hidden page rename', async () => {
    const t = mkm('LIVERENAME', 'live: rename of a hidden page');
    J(await owner.put(`${C}/entities/${ids.hidden}`, { json: { name: t } }), 'rename');
  });
  await tryit('hidden page secret', async () => {
    const t = mkm('LIVESECRET', 'live: inline secret saved on a visible page');
    J(await owner.put(`${C}/entities/${ids.open}/entry`, { json: { entry: doc('x'), entry_html: `<p>open <span data-secret="true">${t}</span></p>` } }), 'entry');
  });
  await tryit('dm-only calendar event', async () => {
    const t = mkm('LIVECAL', 'live: Director-only calendar event');
    J(await owner.post(`${C}/calendars/${ids.cal}/events`, { json: { name: t, year: 1000, month: 3, day: 3, visibility: 'dm_only' } }), 'event');
  });
  await tryit('dm-only timeline event', async () => {
    const t = mkm('LIVETL', 'live: Director-only timeline event');
    J(await owner.post(`${C}/timelines/${ids.tl}/standalone-events`, { json: { name: t, year: 1000, month: 3, day: 4, visibility: 'dm_only' } }), 'event');
  });
  await tryit('dm-only marker', async () => {
    const t = mkm('LIVEMARKER', 'live: Director-only map marker');
    J(await owner.post(`${C}/maps/${ids.map}/markers`, { json: { name: t, x: 5, y: 5, icon: 'fa-skull', color: '#f00', visibility: 'dm_only' } }), 'marker');
  });
  await tryit('gm layer drawing', async () => {
    const t = mkm('LIVEDRAW', 'live: drawing on the gm layer');
    const r = await owner.get(`${C}/maps/${ids.map}/layers`, { headers: { Accept: 'application/json' } });
    const lid = JSON.parse(r.text).find((l) => l.layer_type === 'gm')?.id;
    J(await owner.post(`${C}/maps/${ids.map}/drawings`, { json: { layer_id: lid, drawing_type: 'text', points: [{ x: 1, y: 1 }], stroke_color: '#000', stroke_width: 2, text_content: t, visibility: 'dm_only' } }), 'drawing');
  });
  await tryit('dm-only post', async () => {
    const t = mkm('LIVEPOST', 'live: Director-only post');
    J(await owner.post(`${C}/entities/${ids.open}/posts`, { json: { name: t, entry: JSON.parse(doc(t)), entryHtml: `<p>${t}</p>`, isPrivate: true } }), 'post');
  });
  await tryit('dm-only tag', async () => {
    const t = mkm('LIVETAG', 'live: Director-only tag');
    J(await owner.post(`${C}/tags`, { json: { name: t, color: '#0000ff', dmOnly: true } }), 'tag');
  });
  await tryit('dm-only note', async () => {
    const t = mkm('LIVENOTE', 'live: Director-only entity note');
    J(await owner.post(`${C}/entities/${ids.open}/notes`, { json: { audience: 'dm_only', title: t, body: JSON.parse(doc(t)), bodyHtml: `<p>${t}</p>` } }), 'note');
  });
  return out;
}

export async function seed(base, log = console.log) {
  const owner = new Client(base, 'owner');
  const pw = 'crawl-pass-12345';
  const ownerEmail = `owner-${RUN}@crawl.test`.toLowerCase();
  const playerEmail = `player-${RUN}@crawl.test`.toLowerCase();
  const scribeEmail = `scribe-${RUN}@crawl.test`.toLowerCase();
  await owner.register(ownerEmail, 'Owner Olive', pw);

  let r = await owner.post('/campaigns', { body: { name: 'Crawl ' + RUN, csrf_token: owner.csrf() } });
  const cid = r.url.match(/campaigns\/([0-9a-f-]{36})/)?.[1];
  if (!cid) throw new Error('campaign not created: ' + r.url);
  const C = `/campaigns/${cid}`;

  const player = new Client(base, 'player');
  await player.register(playerEmail, 'Pat Player', pw);
  const player2 = new Client(base, 'player2');
  const player2Email = `player2-${RUN}@crawl.test`.toLowerCase();
  await player2.register(player2Email, 'Quinn Player', pw);
  const scribe = new Client(base, 'scribe');
  await scribe.register(scribeEmail, 'Sam Scribe', pw);
  const guest = new Client(base, 'guest');
  for (const [em, role] of [[playerEmail, 'player'], [player2Email, 'player'], [scribeEmail, 'scribe']]) {
    r = await owner.post(`${C}/members`, { body: { email: em, role, csrf_token: owner.csrf() } });
    if (r.status >= 400) throw new Error(`add ${role}: ${r.status}`);
  }

  const ids = { cid, C };
  const markers = {}; // marker string -> description
  const playerOnly = {}; // marker -> description: private to the first player; nobody else may see it
  const skipped = [];
  const scribeMay = new Set(); // markers a Scribe is allowed to see (GM-tier by design)
  const m = (key, what, opts = {}) => { const s = mk(key); markers[s] = what; if (opts.scribe) scribeMay.add(s); return s; };
  const step = async (name, fn) => {
    try { await fn(); log('seeded  ', name); }
    catch (e) { skipped.push(`${name}: ${e.message}`); log('NOT SEEDED', name, '-', e.message); }
  };
  const must = (res, what) => {
    if (res.status >= 400) throw new Error(`${what} -> ${res.status} ${res.text.slice(0, 160).replace(/\s+/g, ' ')}`);
    return res;
  };

  // Categories: the first two the campaign starts with.
  r = await owner.get(`${C}/entities/new`);
  const typeOpts = [...r.text.matchAll(/<option value="(\d+)">([^<]+)</g)].map((x) => ({ id: +x[1], name: x[2] }));
  const typeId = (n) => typeOpts.find((t) => t.name === n)?.id;
  ids.types = typeOpts;

  // New campaigns start with most add-ons off; their pages answer 404 until on.
  r = await owner.get(`${C}/addons`, { headers: { Accept: 'application/json' } });
  try {
    for (const a of JSON.parse(r.text) || []) {
      if (a.enabled || a.addon_category === 'system') continue;
      await owner.put(`${C}/addons/${a.addon_id}/toggle`, { body: { action: 'enable', csrf_token: owner.csrf() } });
    }
  } catch { skipped.push('addons: could not list/enable'); }

  const create = async (name, type, isPrivate) => {
    const res = await owner.post(`${C}/entities`, {
      body: { name, entity_type_id: typeId(type), is_private: isPrivate ? 'true' : 'false', csrf_token: owner.csrf() },
    });
    const id = res.url.match(/entities\/([0-9a-f-]{36})/)?.[1];
    if (!id) throw new Error('create ' + name + ' -> ' + res.status + ' ' + res.url);
    // A new page takes the campaign's default (private); say what we mean.
    const p = await owner.put(`${C}/entities/${id}/permissions`, { json: { visibility: 'default', is_private: !!isPrivate, permissions: [] } });
    if (p.status >= 400) throw new Error('set visibility of ' + name + ' -> ' + p.status);
    return id;
  };
  const putJSON = (path, json) => owner.put(path, { json });
  const postJSON = (path, json) => owner.post(path, { json });
  const doc = (...paras) => JSON.stringify({ type: 'doc', content: paras.map((p) => ({ type: 'paragraph', content: p })) });
  const t = (text, secret) => (secret ? { type: 'text', text, marks: [{ type: 'secret' }] } : { type: 'text', text });

  // A visible page everything else hangs off.
  const open = await create('Open page ' + RUN, 'Location', false);
  ids.open = open;

  // 1. Hidden page: name and text.
  await step('hidden page', async () => {
    const name = m('HIDNAME', 'hidden page name');
    const body = m('HIDBODY', 'hidden page text');
    const hid = await create(name, 'Location', true);
    ids.hidden = hid;
    must(await putJSON(`${C}/entities/${hid}/entry`, { entry: doc([t(body)]), entry_html: `<p>${body}</p>` }), 'entry');
  });

  // 2. Visible page with an inline secret and a hidden-from-players passage.
  await step('inline secret', async () => {
    const vis = m('PUBTEXT', '(control) visible text must appear');
    const sec = m('INLINESECRET', 'inline secret inside a visible page');
    must(await putJSON(`${C}/entities/${open}/entry`, {
      entry: doc([t(vis + ' ', false), t(sec, true)]),
      entry_html: `<p>${vis} <span data-secret="true">${sec}</span></p>`,
    }), 'entry');
  });
  delete markers[mk('PUBTEXT')]; // the control is checked separately

  // 3. Mention of the hidden page, on the same visible page as the secret.
  await step('mention of hidden page', async () => {
    if (!ids.hidden) throw new Error('needs hidden page');
    const mention = m('MENTIONFAKE', 'stored name inside a mention of the hidden page');
    const vis = mk('PUBTEXT'), sec = mk('INLINESECRET');
    const link = { type: 'text', text: mention, marks: [{ type: 'link', attrs: { href: `${C}/entities/${ids.hidden}`, 'data-mention-id': ids.hidden } }] };
    must(await putJSON(`${C}/entities/${open}/entry`, {
      entry: JSON.stringify({ type: 'doc', content: [
        { type: 'paragraph', content: [t(vis + ' '), t(sec, true)] },
        { type: 'paragraph', content: [t('see '), link] },
      ] }),
      entry_html: `<p>${vis} <span data-secret="true">${sec}</span></p><p>see <a data-mention-id="${ids.hidden}" href="${C}/entities/${ids.hidden}">${mention}</a></p>`,
    }), 'entry');
  });

  // 4. Relation to the hidden page.
  await step('relation to hidden page', async () => {
    if (!ids.hidden) throw new Error('needs hidden page');
    must(await postJSON(`${C}/entities/${open}/relations`, { targetEntityId: ids.hidden, relationType: m('RELTYPE', 'relation label to hidden page'), reverseRelationType: 'rev' }), 'relation');
  });

  // 5. Director-only tag on the visible page.
  await step('Director-only tag', async () => {
    const tag = m('DMTAG', 'Director-only tag name');
    r = must(await postJSON(`${C}/tags`, { name: tag, color: '#ff0000', dmOnly: true }), 'create tag');
    const tid = JSON.parse(r.text)?.id;
    if (!tid) throw new Error('no tag id: ' + r.text.slice(0, 120));
    must(await putJSON(`${C}/entities/${open}/tags`, { tagIds: [tid] }), 'set tags');
  });

  // 6. Post, note.
  await step('Director-only post', async () => {
    const name = m('DMPOSTNAME', 'Director-only post name');
    const body = m('DMPOSTBODY', 'Director-only post text');
    must(await postJSON(`${C}/entities/${open}/posts`, { name, entry: JSON.parse(doc([t(body)])), entryHtml: `<p>${body}</p>`, isPrivate: true }), 'post');
  });
  await step('Director-only entity note', async () => {
    const title = m('DMNOTETITLE', 'Director-only entity note title');
    const body = m('DMNOTEBODY', 'Director-only entity note text');
    must(await postJSON(`${C}/entities/${open}/notes`, { audience: 'dm_only', title, body: JSON.parse(doc([t(body)])), bodyHtml: `<p>${body}</p>` }), 'note');
  });


  const idOf = (res) => { try { return JSON.parse(res.text)?.id; } catch { return undefined; } };
  const hidden = (...a) => a.filter(Boolean);
  ids.extra = [];

  // 7. Hidden NPC (a Director-only Character page).
  await step('hidden NPC', async () => {
    const name = m('NPCNAME', 'hidden NPC name');
    const body = m('NPCBODY', 'hidden NPC text');
    const id = await create(name, 'Character', true);
    ids.npc = id;
    must(await putJSON(`${C}/entities/${id}/entry`, { entry: doc([t(body)]), entry_html: `<p>${body}</p>` }), 'entry');
  });

  // 8. Calendar: a Director-only event and an unannounced future one.
  await step('calendar events', async () => {
    r = must(await postJSON(`${C}/calendars`, { mode: 'fantasy', name: 'Cal ' + RUN, current_year: 1000, hours_per_day: 24, minutes_per_hour: 60, seconds_per_minute: 60 }), 'calendar');
    const cal = idOf(r) || JSON.parse(r.text)?.calendar?.id;
    if (!cal) throw new Error('no calendar id ' + r.text.slice(0, 100));
    ids.cal = cal;
    r = must(await postJSON(`${C}/calendars/${cal}/events`, { name: m('CALDMEVENT', 'Director-only calendar event name'), description: m('CALDMEVENTDESC', 'Director-only calendar event text'), year: 1000, month: 1, day: 3, visibility: 'dm_only' }), 'dm event');
    ids.extra.push(idOf(r));
    r = must(await postJSON(`${C}/calendars/${cal}/events`, { name: m('CALFUTURE', 'unannounced future calendar event name'), description: m('CALFUTUREDESC', 'unannounced future calendar event text'), year: 1000, month: 2, day: 20, visibility: 'everyone', announced: 'on_day' }), 'future event');
    ids.extra.push(idOf(r));
  });

  // 9. Timeline: a Director-only event connected to a visible one.
  await step('timeline events', async () => {
    const f = { name: 'Line ' + RUN, description: '' };
    if (ids.cal) f.calendar_id = ids.cal;
    r = await owner.post(`${C}/timelines`, { body: { ...f, csrf_token: owner.csrf() } });
    const tid = r.url.match(/timelines\/([0-9a-f-]{36})/)?.[1];
    if (!tid) throw new Error('no timeline: ' + r.status + ' ' + r.url);
    ids.tl = tid;
    r = must(await postJSON(`${C}/timelines/${tid}/standalone-events`, { name: 'Open event ' + RUN, year: 1000, month: 1, day: 1, visibility: 'everyone' }), 'open event');
    const a = idOf(r);
    r = must(await postJSON(`${C}/timelines/${tid}/standalone-events`, { name: m('TLDMEVENT', 'Director-only timeline event name'), description: m('TLDMEVENTDESC', 'Director-only timeline event text'), year: 1000, month: 1, day: 2, visibility: 'dm_only' }), 'dm event');
    const b = idOf(r);
    ids.extra.push(a, b);
    must(await postJSON(`${C}/timelines/${tid}/connections`, { source_id: a, target_id: b, source_type: 'standalone', target_type: 'standalone', label: m('TLCONNLABEL', 'label on a connection to a Director-only timeline event'), style: 'solid' }), 'connection');
  });

  // 10. Maps: a Director-only layer with a drawing and marker.
  await step('map layer, drawing, marker', async () => {
    r = must(await postJSON(`${C}/maps`, { name: 'Map ' + RUN, image_width: 800, image_height: 600 }), 'map');
    const mid = idOf(r) || JSON.parse(r.text)?.map?.id;
    if (!mid) throw new Error('no map id ' + r.text.slice(0, 100));
    ids.map = mid;
    r = must(await postJSON(`${C}/maps/${mid}/layers`, { name: m('GMLAYER', 'gm layer name'), layer_type: 'gm', sort_order: 5, is_visible: true, opacity: 1 }), 'layer');
    const lid = idOf(r);
    ids.extra.push(lid);
    must(await postJSON(`${C}/maps/${mid}/drawings`, { layer_id: lid, drawing_type: 'text', points: [{ x: 10, y: 10 }], stroke_color: '#000', stroke_width: 2, text_content: m('GMDRAWTEXT', 'text of a drawing on the gm layer'), visibility: 'dm_only' }), 'drawing');
    must(await postJSON(`${C}/maps/${mid}/markers`, { name: m('DMMARKER', 'Director-only map marker name'), description: m('DMMARKERDESC', 'Director-only map marker text'), x: 40, y: 40, icon: 'fa-skull', color: '#f00', visibility: 'dm_only' }), 'marker');
  });

  // 11. Session recap with a Director-only passage.
  await step('session recap', async () => {
    r = await owner.post(`${C}/sessions`, { body: { name: 'Night ' + RUN, summary: m('SESSIONSUMMARY', 'session summary (owner wrote it for everyone? checked below)'), scheduled_date: '2026-12-01', csrf_token: owner.csrf() } });
    const sid = r.url.match(/sessions\/([0-9a-f-]{36})/)?.[1] || (r.text.match(/sessions\/([0-9a-f-]{36})/) || [])[1];
    if (!sid) throw new Error('no session: ' + r.status + ' ' + r.url);
    ids.session = sid;
    const rec = mk('RECAPSECRET'); // recaps drop secret passages on save, so this is not a marker
    const pubrec = mk('RECAPPUB');
    must(await putJSON(`${C}/sessions/${sid}/recap`, { recap_html: `<p>${pubrec} <span data-secret="true">${rec}</span></p>`, entry_html: `<p>${pubrec} <span data-secret="true">${rec}</span></p>`, entry: doc([t(pubrec + ' '), t(rec, true)]) }), 'recap');
    delete markers[mk('SESSIONSUMMARY')];
  });
  // The session summary is shown to players by design; it is only a control.
  delete markers[mk('SESSIONSUMMARY')];

  // 12. Director-only field value: a gm_only field on the Location category.
  await step('Director-only field value', async () => {
    const etid = typeId('Location');
    r = must(await putJSON(`${C}/entity-types/${etid}`, { fields: [{ key: 'gmsecret', label: 'Secret', type: 'text', section: 'Basics', gm_only: true }, { key: 'pubfield', label: 'Public', type: 'text', section: 'Basics' }] }), 'entity type');
    const val = m('GMFIELD', 'Director-only field value');
    must(await putJSON(`${C}/entities/${open}/fields`, { fields_data: { gmsecret: val, pubfield: mk('PUBFIELD') } }), 'fields');
  });

  // 13. A drawing left on the gm layer with the default visibility.
  await step('drawing on gm layer, default visibility', async () => {
    if (!ids.map) throw new Error('needs map');
    r = await owner.get(`${C}/maps/${ids.map}/layers`, { headers: { Accept: 'application/json' } });
    const lid = JSON.parse(r.text).find((l) => l.layer_type === 'gm')?.id;
    must(await postJSON(`${C}/maps/${ids.map}/drawings`, { layer_id: lid, drawing_type: 'text', points: [{ x: 20, y: 20 }], stroke_color: '#000', stroke_width: 2, text_content: m('GMDRAWDEFAULT', 'drawing on the gm layer with default visibility') }), 'drawing');
  });

  // 14. Armory: an item page held in the owner's stash, a stash, an inventory.
  await step('armory stash and item', async () => {
    const item = m('ARMORYITEM', 'name of an item page only the owner holds');
    const itemId = await create(item, 'Item', true);
    ids.item = itemId;
    r = must(await owner.post(`${C}/armory/stashes`, { body: { name: m('STASHNAME', 'owner stash name', { scribe: true }), location: m('STASHLOC', 'owner stash location', { scribe: true }), csrf_token: owner.csrf() } }), 'stash');
    r = await owner.get(`${C}/armory/stashes`);
    const sid = (r.text.match(/armory\/stashes\/(\d+)/) || [])[1];
    if (sid) must(await owner.post(`${C}/armory/stashes/${sid}/items`, { body: { item_id: itemId, quantity: '1', csrf_token: owner.csrf() } }), 'stash item');
    else throw new Error('stash id not found on page (stash created, item not added)');
  });
  await step('armory inventory', async () => {
    must(await postJSON(`${C}/armory/instances`, { name: mk('INVNAME'), description: mk('INVDESC') }), 'instance');
  });

  // 15. Make the campaign public so the signed-out walk sees what a stranger can.
  await step('public campaign', async () => {
    must(await owner.put(C, { json: { name: 'Crawl ' + RUN, description: '', is_public: true } }), 'update campaign');
    r = await guest.get(C, { follow: false });
    if (r.status !== 200) throw new Error('a stranger still cannot open it: ' + r.status);
  });

  // 16. One player's private notes must not reach another player.
  await step("a player's private notes", async () => {
    const title = mk('PNOTETITLE'), body = mk('PNOTEBODY');
    playerOnly[title] = "a player's private note title";
    playerOnly[body] = "a player's private note text";
    r = await player.post(`${C}/notes`, { json: { title, content: [{ type: 'text', value: body }] } });
    if (r.status >= 400) throw new Error('player note -> ' + r.status + ' ' + r.text.slice(0, 120));
    const et = mk('PENOTETITLE'), eb = mk('PENOTEBODY');
    playerOnly[et] = "a player's private note on a page (title)";
    playerOnly[eb] = "a player's private note on a page (text)";
    r = await player.post(`${C}/entities/${open}/notes`, { json: { audience: 'private', title: et, body: JSON.parse(doc([t(eb)])), bodyHtml: `<p>${eb}</p>` } });
    if (r.status >= 400) throw new Error('player page note -> ' + r.status + ' ' + r.text.slice(0, 120));
  });

  ids.all = hidden(cid, ids.open, ids.item, ids.hidden, ids.npc, ids.cal, ids.tl, ids.map, ids.session, ...ids.extra);

  return { owner, player, player2, scribe, guest, playerOnly, ids, markers, scribeMay, controls: [mk('PUBTEXT'), mk('PUBFIELD')], skipped, C, emails: { ownerEmail, playerEmail, scribeEmail } };
}
