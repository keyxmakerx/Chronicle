// Walks every campaign GET route as a given viewer and reports any seeded
// marker found in a response body. Also walks as the owner so every marker is
// proven reachable: a marker the owner's walk never sees means that seed is
// not covered, which is reported instead of passing silently.

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const SNAPSHOT = join(here, '..', '..', 'internal', 'wire', 'routes_snapshot.txt');

// Routes that sign out, stream forever, or return files; a GET there tells
// nothing about visibility.
const SKIP = /\/(logout|export|download|events\/stream|stream)$|\/ws$|oidc|\/media\/.*\/file|\.zip$/;

export function campaignGetRoutes() {
  const out = [];
  for (const line of readFileSync(SNAPSHOT, 'utf8').split('\n')) {
    const [method, path] = line.split('\t');
    if (method !== 'GET' || !path) continue;
    if (!/:id\b|:cid\b/.test(path) || !/campaigns/.test(path)) continue;
    if (SKIP.test(path) || path.includes('*')) continue;
    out.push(path);
  }
  return [...new Set(out)];
}

function expand(path, cid, pool, cap = 24) {
  const params = [...new Set(path.match(/:[A-Za-z]+/g) || [])].filter((p) => p !== ':id' && p !== ':cid');
  const base = path.replace(/:(id|cid)\b/g, cid);
  if (!params.length) return [base];
  const out = [];
  const pick = (p) => {
    // Numeric-looking parameters get small ints; the rest get seeded ids.
    return /^:(y|m|d|etid|typeID|tagId|vid|nid|rid|size|mappingID|markerID|fid|did|lid|iid|gid|bid|pid|encid|encounterID|eraID)$/.test(p) ? [...pool.ints, ...pool.ids.slice(0, 4)] : [...pool.ids, ...pool.ints.slice(0, 2), 'a'];
  };
  const lists = params.map(pick);
  const rec = (i, cur) => {
    if (out.length >= cap) return;
    if (i === params.length) { out.push(cur); return; }
    for (const v of lists[i]) { rec(i + 1, cur.replace(new RegExp(params[i] + '(?![A-Za-z])'), encodeURIComponent(v))); if (out.length >= cap) return; }
  };
  rec(0, base);
  return out;
}

const VARIANTS = [
  { name: 'html', headers: { Accept: 'text/html' } },
  { name: 'json', headers: { Accept: 'application/json' } },
  { name: 'htmx', headers: { Accept: 'text/html', 'HX-Request': 'true' } },
];

// Returns {hits: [{route, variant, marker, status}], seen:Set(markers), requests}
export async function crawl(client, cid, ids, markers, { log = () => {}, term = 'ZQ', controls = [] } = {}) {
  const controlSeen = new Set();
  const pool = { ids: [...new Set(ids)], ints: ['1', '2', '3', '4', '5'] };
  const keys = Object.keys(markers);
  const hits = [];
  const seen = new Set();
  let requests = 0;
  let before0 = client.jar.size;
  const urls = new Set();
  for (const r of campaignGetRoutes()) for (const u of expand(r, cid, pool)) urls.add(u);
  // Search, lookup and feed routes answer nothing without a query.
  const Q = `q=${term}&query=${term}&term=${term}&search=${term}&name=${term}&since=0&limit=500`;
  for (const u of [...urls]) {
    if (/search|find|lookup|suggest|changes|feed|entities$|notes$|timeline|sidebar|results/.test(u)) urls.add(u + (u.includes('?') ? '&' : '?') + Q);
  }
  for (const e of [`/campaigns/${cid}/entities?q=${term}`, `/campaigns/${cid}/search?q=${term}`]) urls.add(e);
  const lostAt = [];
  for (const u of urls) {
    // A GET that ends the session would turn the rest of the walk into a
    // signed-out walk; note it, sign back in, and carry on.
    if (client.creds && !(await client.signedIn())) {
      lostAt.push(u);
      await client.login(client.creds.email, client.creds.password);
    }
    for (const v of VARIANTS) {
      let res;
      try { res = await client.get(u, { headers: v.headers, follow: false }); } catch { continue; }
      requests++;
      const before = client.jar.size;
      if (client.jar.size < before0) { log(`${client.label}: cookie dropped by GET ${u}`); before0 = client.jar.size; }
      if (res.status >= 300 && res.status < 400) continue;
      for (const k of controls) if (res.text.includes(k)) controlSeen.add(k);
      for (const k of keys) {
        if (res.text.includes(k)) {
          seen.add(k);
          hits.push({ route: u.replace(cid, ':id'), variant: v.name, marker: k, status: res.status });
        }
      }
    }
  }
  if (client.creds && !(await client.signedIn())) { lostAt.push('(end of walk)'); await client.login(client.creds.email, client.creds.password); }
  log(`${client.label}: ${urls.size} urls x ${VARIANTS.length} = ${requests} requests`);
  return { hits, seen, requests, lostAt, controlSeen };
}
