// player-crawl: seed a campaign with marked hidden content, then walk it as a
// player, a scribe and a signed-out visitor and fail if any marker shows up.
//
// RUN against a scratch instance (it registers accounts and makes a campaign):
//   CRAWL_BASE=http://localhost:8099 node tools/player-crawl/run.mjs
// Exit 0: no leaks and every seed reachable by the owner. 1: a leak or an
// unreachable seed. 2: the harness itself failed.

import { seed, liveEdits, RUN } from './seed.mjs';
import { watch } from './ws.mjs';
import { crawl } from './crawl.mjs';

const BASE = process.env.CRAWL_BASE || 'http://localhost:8080';
const idShape = (s) => s.replace(/[0-9a-f]{8}-[0-9a-f-]{27}/g, ':uid');

try {
  const s = await seed(BASE, (...a) => console.log(...a));
  const names = Object.keys(s.markers).length;
  console.log(`\nseeded ${names} markers in ${s.ids.cid}; ${s.skipped.length} step(s) not seeded`);
  const o = await crawl(s.owner, s.ids.cid, s.ids.all, s.markers, { term: 'ZQ' + RUN, controls: s.controls, extraUrls: s.extraUrls });
  let unreachable = Object.entries(s.markers).filter(([k]) => !o.seen.has(k));
  // The author's own walk must find their private notes, or the check on the
  // others proves nothing. It walks the extra URLs too: a cut scene's answer
  // is the player's own only on their list of scenes shown to them.
  const own = await crawl(s.player, s.ids.cid, s.ids.all, s.playerOnly, { term: 'ZQ' + RUN, extraUrls: s.extraUrls });
  for (const [k, v] of Object.entries(s.playerOnly)) if (!own.seen.has(k)) { console.log('  UNREACHABLE (even the author never sees it):', v); unreachable.push([k, v]); }
  console.log(`owner walk: ${o.requests} requests, saw ${o.seen.size}/${names} markers`);
  for (const [, v] of unreachable) console.log('  UNREACHABLE (no page shows it to the owner):', v);

  let leaks = 0;
  // Shop goods stay out of lists even for the Director: they show in their
  // shop, on their own page and in the Director's Shop goods tab, nowhere else.
  for (const k of Object.keys(s.markers).filter((x) => /SHOPGOOD/.test(x))) {
    const ok = [s.ids.shop, s.ids.shopB, s.ids.goodVis, s.ids.goodHid, s.ids.goodFlip].filter(Boolean);
    const bad = o.hits.filter((h) => h.marker === k && !ok.some((id) => h.route.includes(id)) && !/\/armory\?goods=1/.test(h.route)
      // The AI export names a shop's goods only as its "sells" relations; the goods get no page of their own there.
      && !/\/ai-export\/generate/.test(h.route));
    if (bad.length) {
      leaks++;
      console.log(`  LEAK  the Director's own walk finds ${s.markers[k]} outside its shop:`);
      for (const h of bad.slice(0, 6)) console.log(`        ${h.variant.padEnd(4)} ${idShape(h.route)}`);
    }
  }
  // The owner previewing as a guest rides the owner's own session with the
  // guest cookie on. The server lowers each read to a visitor's, so this walk
  // must see what the signed-out stranger sees, under the same leak checks.
  // The cookie's value is the campaign it was turned on in, as the server
  // writes it; any other value is no preview at all. It is dropped straight
  // after the walk so no later owner step runs with it (writes ignore it anyway).
  const GUEST_COOKIE = 'chronicle_view_as_guest';
  const viewers = { player: s.player, 'other player': s.player2, scribe: s.scribe, signedout: s.guest, 'owner previewing as a guest': s.owner };
  const walks = {};
  for (const [who, client] of Object.entries(viewers)) {
    const previewing = who === 'owner previewing as a guest';
    if (previewing) client.jar.set(GUEST_COOKIE, s.ids.cid);
    const r = await crawl(client, s.ids.cid, s.ids.all, { ...s.markers, ...s.playerOnly }, { term: 'ZQ' + RUN, controls: s.controls, extraUrls: s.extraUrls });
    if (previewing) client.jar.delete(GUEST_COOKIE);
    walks[who] = r;
    if (r.lostAt.length) console.log(`  (${who}'s session ended during the walk at: ${r.lostAt.join(', ')}; signed back in)`);
    const byMarker = new Map();
    for (const h of r.hits) {
      if (who === 'scribe' && s.scribeMay.has(h.marker)) continue;
      if (who === 'player' && s.playerOnly[h.marker]) continue; // the author reading their own note
      // A shop good listed openly shows in its own shop and on its own page;
      // anywhere else (a list, search, Auto-link, the Items gallery) is a leak.
      if ((s.mayShow[h.marker] || []).some((id) => h.route.includes(id))) continue;
      const e = byMarker.get(h.marker) || new Set();
      e.add(`${h.variant.padEnd(4)} ${idShape(h.route)}`);
      byMarker.set(h.marker, e);
    }
    console.log(`\n${who}: ${r.requests} requests, ${byMarker.size} marker(s) leaked; saw ${r.controlSeen.size}/${s.controls.length} visible-content controls`);
    if (r.controlSeen.size < s.controls.length) { leaks++; console.log(`  BLIND: ${who} never saw the visible text it should, so a clean result proves nothing`); }
    for (const [k, routes] of byMarker) {
      leaks++;
      console.log(`  LEAK  ${s.markers[k]}`);
      for (const rt of [...routes].slice(0, 6)) console.log(`        ${rt}`);
      if (routes.size > 6) console.log(`        ... and ${routes.size - 6} more`);
    }
  }
  // A card's picture is a file: while its card is face down no one but a
  // Director may be sent it, at full size or as a thumbnail, whatever the
  // walk's markers say. The Director's own request proves the file is there.
  for (const u of s.pictureUrls || []) {
    const mine = await s.owner.get(u, { follow: false });
    if (mine.status !== 200) { leaks++; console.log(`  PICTURE NOT REACHABLE by the Director: ${u} -> ${mine.status}`); }
    // The owner previewing as a guest is left out: /media sits outside the campaign's
    // routes, which are what the preview cookie lowers, and the owner is a Director.
    for (const [who, client] of Object.entries(viewers)) {
      if (who === 'owner previewing as a guest') continue;
      const got = await client.get(u, { follow: false });
      if (got.status === 200) { leaks++; console.log(`  LEAK  ${who} was sent a face-down card's picture: ${u}`); }
    }
  }
  // The write probes (the turn request, starting a deck from a source): sent by people who may not, the first by people who may not turn that copy, it must be
  // refused as if nothing were there, carry no marker, and leave the copy face
  // down (checked from the Director's side afterwards).
  for (const pr of s.postProbes || []) {
    for (const who of pr.who) {
      const client = viewers[who];
      const res = await client.post(pr.path, { body: { csrf_token: client.csrf() }, headers: { Accept: 'text/html, application/json' }, follow: false });
      const found = Object.keys({ ...s.markers, ...s.playerOnly }).filter((k) => res.text.includes(k));
      console.log(`write probe ${pr.path.replace(s.C, '')} as ${who}: ${res.status}`);
      if (res.status === 200 || found.length) { leaks++; console.log(`  LEAK  ${who} was answered ${res.status} to a write probe (${pr.path.replace(s.C, '')})${found.length ? ' with ' + found.map((k) => s.markers[k]).join(', ') : ''}`); }
    }
  }
  // The moment and the card are as the Director left them: the answers and the
  // say route, asked by people who may not, changed nothing.
  if (s.ids.moment) {
    const mo = await s.owner.get(`${s.C}/decks/moments`, { headers: { Accept: 'application/json' } });
    const still = (JSON.parse(mo.text).moments || []).find((x) => x.id === s.ids.moment);
    if (!still || still.state !== 'waiting') { leaks++; console.log('  LEAK  a moment was answered by a request from someone who may not answer it'); }
    const dk = await s.owner.get(`${s.C}/decks/${s.ids.whenDeck}`, { headers: { Accept: 'application/json' } });
    const card = (JSON.parse(dk.text).cards || []).find((c) => c.id === s.ids.whenCard);
    if (!card || card.saidAt) { leaks++; console.log('  LEAK  a card was said so by a request from someone who may not say it'); }
  }
  // The Director's notice fragment is the Director's alone: everyone else is told
  // the page is not there, and the Director's own read proves the probe is not blind.
  if (s.ids.momentPanel) {
    const ask = (client) => client.get(s.ids.momentPanel, { headers: { Accept: 'text/html', 'HX-Request': 'true' }, follow: false });
    const mine = await ask(s.owner);
    console.log(`moments panel as owner: ${mine.status}`);
    if (mine.status !== 200) { leaks++; console.log('  LEAK  the Director was refused the moments panel (the probe is blind)'); }
    for (const who of ['player', 'other player', 'scribe']) {
      const res = await ask(viewers[who]);
      console.log(`moments panel as ${who}: ${res.status}`);
      const found = Object.keys(s.markers).filter((k) => res.text.includes(k));
      if (res.status !== 404 || found.length) { leaks++; console.log(`  LEAK  ${who} was answered ${res.status} for the moments panel${found.length ? ' with ' + found.map((k) => s.markers[k]).join(', ') : ''}`); }
    }
  }
  if ((s.postProbes || []).length) {
    const d = await s.owner.get(`${s.C}/decks/${s.ids.handDeck}`, { headers: { Accept: 'application/json' } });
    const copy = (JSON.parse(d.text).copies || []).find((c) => c.id === s.ids.handCopy);
    if (!copy || copy.faceUp) { leaks++; console.log('  LEAK  the copy was turned over by a request from someone who may not turn it'); }
  }
  // The preview must not open anything the stranger cannot: any page that
  // answers 2xx to the previewing owner and not to the stranger is a leak of
  // the owner's real access. The other direction (a page the stranger gets and
  // the preview refuses) is only noted.
  {
    const ok = (n) => n < 300;
    const strangerWalk = walks.signedout, previewWalk = walks['owner previewing as a guest'];
    let wider = 0, narrower = 0;
    for (const [key, st] of previewWalk.statuses) {
      const other = strangerWalk.statuses.get(key);
      if (other === undefined) continue;
      if (ok(st) && !ok(other)) { wider++; leaks++; console.log(`  PREVIEW OPENS WHAT A STRANGER CANNOT  ${idShape(key)}  preview ${st}, stranger ${other}`); }
      else if (!ok(st) && ok(other)) narrower++;
    }
    console.log(`\nguest preview vs stranger: ${wider} page(s) wider, ${narrower} narrower (narrower is only noted)`);
  }
  // The live channel: viewers hold a socket while the owner edits hidden things.
  const socks = {
    player: await watch(s.player, BASE, s.ids.cid),
    scribe: await watch(s.scribe, BASE, s.ids.cid),
    owner: await watch(s.owner, BASE, s.ids.cid),
  };
  for (const [w, k] of Object.entries(socks)) if (!k.opened) { console.log(`\nsocket: ${w} could not connect`); leaks++; }
  const live = await liveEdits(s);
  const skippedLive = live.__skipped || []; delete live.__skipped;
  const control = live.__control; delete live.__control;
  const scribeMayLive = new Set(live.__scribeMay || []); delete live.__scribeMay;
  await new Promise((r) => setTimeout(r, 2500));
  const ownerBlob = socks.owner.messages.join('\n');
  console.log(`\nlive channel: owner socket got ${socks.owner.messages.length} message(s)`);
  for (const who of ['player', 'scribe']) {
    const got = socks[who].messages.some((x) => x.includes(control));
    console.log(`  ${who} socket got ${socks[who].messages.length} message(s); visible-page control ${got ? 'arrived' : 'DID NOT ARRIVE (live check is blind)'}`);
    if (!got) leaks++;
  }
  for (const [k, what] of Object.entries(live)) {
    if (!ownerBlob.includes(k)) console.log(`  not carried live to the owner (nothing to compare): ${what}`);
    for (const who of ['player', 'scribe']) {
      if (who === 'scribe' && scribeMayLive.has(k)) continue;
      if (socks[who].messages.some((x) => x.includes(k))) { leaks++; console.log(`  LEAK  ${who} socket received: ${what}`); }
    }
  }
  // A moment is told to Directors only: the owner's socket must have heard one
  // (or the check is blind), and neither the player's nor the scribe's may have.
  if (!skippedLive.includes('a moment comes')) {
    const heard = (k) => socks[k].messages.some((x) => x.includes('"decks.moment"'));
    console.log(`  moment message: owner ${heard('owner') ? 'heard' : 'DID NOT HEAR (live check is blind)'}`);
    if (!heard('owner')) leaks++;
    for (const who of ['player', 'scribe']) {
      if (heard(who)) { leaks++; console.log(`  LEAK  ${who} socket received a moment message`); }
    }
  }
  for (const k of Object.values(socks)) k.close();
  if (skippedLive.length) console.log('  live edits not done: ' + skippedLive.join(', '));
  process.exitCode = leaks || unreachable.length || s.skipped.length ? 1 : 0;
  if (s.skipped.length) console.log('\nnot seeded:\n  ' + s.skipped.join('\n  '));
} catch (e) {
  console.error('player-crawl: ' + e.stack);
  process.exitCode = 2;
}
