// _setup.mjs — runs once before the live checks, on a fresh instance.
//
// Registers the shared site admin. Only the first account on a fresh
// instance is made a site admin, so this must run before any check signs
// anyone up; tools/live-checks.sh runs it first, on its own.

import assert from 'node:assert/strict';
import { person, ADMIN, BASE, done } from './_live.mjs';

try {
  const admin = await person({ ...ADMIN, tag: 'admin' });
  const res = await admin.page.goto(BASE + '/admin', { waitUntil: 'networkidle' });
  assert.equal(res.status(), 200, `${ADMIN.email} is not a site admin: /admin answered ${res.status()}. Run the checks on a fresh database.`);
  console.log(`live setup: ${ADMIN.email} is the site admin on ${BASE}`);
} finally {
  await done();
}
