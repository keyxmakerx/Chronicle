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
  const o = await crawl(s.owner, s.ids.cid, s.ids.all, s.markers, { term: 'ZQ' + RUN, controls: s.controls });
  let unreachable = Object.entries(s.markers).filter(([k]) => !o.seen.has(k));
  // The author's own walk must find their private notes, or the check on the others proves nothing.
  const own = await crawl(s.player, s.ids.cid, s.ids.all, s.playerOnly, { term: 'ZQ' + RUN });
  for (const [k, v] of Object.entries(s.playerOnly)) if (!own.seen.has(k)) { console.log('  UNREACHABLE (even the author never sees it):', v); unreachable.push([k, v]); }
  console.log(`owner walk: ${o.requests} requests, saw ${o.seen.size}/${names} markers`);
  for (const [, v] of unreachable) console.log('  UNREACHABLE (no page shows it to the owner):', v);

  let leaks = 0;
  const viewers = { player: s.player, 'other player': s.player2, scribe: s.scribe, signedout: s.guest };
  for (const [who, client] of Object.entries(viewers)) {
    const r = await crawl(client, s.ids.cid, s.ids.all, { ...s.markers, ...s.playerOnly }, { term: 'ZQ' + RUN, controls: s.controls });
    if (r.lostAt.length) console.log(`  (${who}'s session ended during the walk at: ${r.lostAt.join(', ')}; signed back in)`);
    const byMarker = new Map();
    for (const h of r.hits) {
      if (who === 'scribe' && s.scribeMay.has(h.marker)) continue;
      if (who === 'player' && s.playerOnly[h.marker]) continue; // the author reading their own note
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
      if (socks[who].messages.some((x) => x.includes(k))) { leaks++; console.log(`  LEAK  ${who} socket received: ${what}`); }
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
