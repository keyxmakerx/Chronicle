// cal_visibility.js — THE ONE visibility mapper (C-CALV4-DAYCARD, R2-2a,
// [DC-10] SIGNED).
//
// WHY THIS FILE EXISTS. The mapping between an editor's three-way visibility
// MODE and Chronicle's stored `{visibility, visibility_rules}` pair was already
// duplicated once — calendar_permissions.js:20-34 and the V2 event drawer carry
// the same fifteen lines — and this slice needed it a third time. A third copy
// is a defect in waiting: the three would drift on the first change to the
// model, and the drift would present as "some surfaces set dm_only and some
// don't", which is a permission bug wearing a UI bug's clothes. So it is
// EXTRACTED here and both live consumers read it.
//
// THE MAPPING, and it mirrors the W5a resolver rather than inventing anything:
//
//   public   → visibility "everyone", no rules
//   gmonly   → visibility "dm_only"   (a hard GM gate; an allow-list does NOT
//                                      admit anyone past it)
//   specific → visibility "everyone" + {allowed_users, denied_users}
//
// dm_only ADDITIONALLY requires CanAuthorDmOnly() server-side
// (handler.go's CreateEventAPI/UpdateEventAPI downgrade it to "everyone" for
// anyone without the co-DM capability). This module does not know the viewer,
// so it does not enforce that — the PRODUCER does, by not rendering the control
// ([DC-9]: the client must not offer what the server will silently ignore).
//
// ONLY USER-KIND RULES PERSIST. The stored model is user-scoped
// (allowed_users / denied_users); a role- or tag-kind chip has nowhere to go,
// and the composed tag+member audience does not exist on main at all — there is
// no member_tags table, the shipped people primitive is campaign_groups, and
// the composed audience is W-G's. Dropping a non-user chip silently is the
// shipped behaviour and is preserved verbatim rather than "improved".
(function () {
  'use strict';

  // buildVisibilityPayload maps the editor mode + chip rules to a write body.
  function buildVisibilityPayload(mode, chipRules) {
    if (mode === 'gmonly') return { visibility: 'dm_only', visibility_rules: null };
    if (mode === 'public') return { visibility: 'everyone', visibility_rules: null };
    // specific
    var allowed = [], denied = [];
    (chipRules || []).forEach(function (r) {
      if (!r || r.kind !== 'user' || !r.target) return;
      if (r.mode === 'allow') allowed.push(r.target);
      else if (r.mode === 'deny') denied.push(r.target);
    });
    var rules = {};
    if (allowed.length) rules.allowed_users = allowed;
    if (denied.length) rules.denied_users = denied;
    var json = (allowed.length || denied.length) ? JSON.stringify(rules) : null;
    return { visibility: 'everyone', visibility_rules: json };
  }

  // rulesToChips converts a stored {allowed_users,denied_users} JSON string into
  // the chip-row rule array an editor renders.
  function rulesToChips(rulesStr) {
    if (!rulesStr) return [];
    var r;
    try { r = JSON.parse(rulesStr); } catch (e) { return []; }
    if (!r || typeof r !== 'object') return [];
    var chips = [];
    (r.allowed_users || []).forEach(function (u) { chips.push({ mode: 'allow', kind: 'user', target: u, label: u }); });
    (r.denied_users || []).forEach(function (u) { chips.push({ mode: 'deny', kind: 'user', target: u, label: u }); });
    return chips;
  }

  // modeFor is the INVERSE, and the day card's editor needs it: a stored record
  // arrives as {visibility, visibility_rules} and the editor opens on a mode.
  // It is here rather than in the card so the round trip cannot be asymmetric —
  // buildVisibilityPayload(modeFor(x), rulesToChips(x.rules)) must reproduce x,
  // and that is asserted in test/js/calendar_permissions.test.mjs.
  function modeFor(visibility, rulesStr) {
    if (visibility === 'dm_only') return 'gmonly';
    if (rulesToChips(rulesStr).length) return 'specific';
    return 'public';
  }

  var api = {
    buildVisibilityPayload: buildVisibilityPayload,
    rulesToChips: rulesToChips,
    modeFor: modeFor,
  };

  if (typeof window !== 'undefined') {
    window.ChronicleCalVisibility = api;
    // The test alias, matching the house convention (__calPerm, __calDayCard).
    window.__calVis = api;
  }
})();
