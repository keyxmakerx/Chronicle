// pane.test.mjs — Phase 1.5 R3: the participation-role enum the attach-entity
// picker uses MUST match the entity-ties data model exactly (so the Phase-2
// port wires straight through). The pane DOM behaviour needs a real DOM and is
// guarded structurally in Go + the operator's visual gate.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { boot, arr } from './harness.mjs';

test('participation roles == the entity-ties enum (order + values)', () => {
  const w = boot();
  assert.deepEqual(arr(w.__calParticipationRoles), ['involved', 'present', 'affected', 'mentioned']);
});
