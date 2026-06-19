# Security Audit — Player Character Claiming (PC-CLAIM-2 / PC-CLAIM-3)

- **Date:** 2026-06-19
- **Reviewer:** Claude Code (`/security-review`)
- **Base commit:** `7f3adc0` (main, after #481 PC-CLAIM-2 and #482 PC-CLAIM-3 merged)
- **Scope:** The Player Character Claiming surface introduced by PC-CLAIM-2/3:
  - `ClaimEntity` (`POST /campaigns/:id/entities/:eid/claim`, RolePlayer)
  - `AssignOwner` (`PUT /campaigns/:id/entities/:eid/owner`, RoleScribe)
  - the `player-character-claiming` addon gate (claim path + PC sub-type creation)
  - `owner_user_id` validation across the web and Foundry/`syncapi` paths
  - the per-type `claimable` flag
  - `entity.claimed` / `entity.owner_changed` audit integrity

## Executive Summary

The claiming surface is, on the whole, well-built: IDOR protection, role
gating, already-claimed/non-claimable rejection, cross-campaign protection,
`owner_user_id` membership validation, the Owner-only `claimable` flag, and
non-spoofable audit identity were all verified correct (see *Verified Secure*).

The audit found **one Medium** issue — the addon feature-gate was enforced in
the UI but **not** on the claim/owner API endpoints, so the lowest-privilege
role could drive the feature with a hand-rolled request while it was disabled —
plus **two Low** hardening gaps on the same endpoints. All three are fixed in
this branch; one Informational defense-in-depth item is left as a recommendation.

**Findings by severity:**

| ID  | Severity | Title | Status |
|-----|----------|-------|--------|
| M-1 | Medium | Addon gate not enforced on the claim/owner API paths | **Fixed** |
| L-1 | Low | `AssignOwner` accepted non-claimable entity types | **Fixed** |
| L-2 | Low | `ClaimEntity` had no view/ACL check → hidden-character name disclosure | **Fixed** |
| I-1 | Info | `entityService.Create` trusts the caller for `owner_user_id` membership | Recommendation |

---

## MEDIUM Severity

### M-1: Player Character Claiming addon gate is enforced in the UI but not on the API

**Location:** `internal/plugins/entities/service.go` — `ClaimEntity`, `AssignOwner`;
routes `internal/plugins/entities/routes.go:89,91`.

**Description.** PC-CLAIM-3 gates all four *UI* surfaces on the
`player-character-claiming` addon (the claim button, the GM roster, the per-type
toggle, the banner) via `Handler.isAddonEnabled`. However, the state-changing
**endpoints** were not gated:

- `ClaimEntity` (`POST …/claim`, **RolePlayer**) → `service.ClaimEntity` checked
  *idempotency*, *already-claimed*, and *claimable-type*, but **never** checked
  whether the addon was enabled for the campaign.
- `AssignOwner` (`PUT …/owner`, **RoleScribe**) → `service.AssignOwner` did the
  same with no addon check.

Neither route carries the `addons.RequireAddon(...)` middleware that gates every
other addon-backed feature (maps, calendar, npcs, timeline, sessions, media).
The repo's own convention — and PC-CLAIM-2's own `CreateEntityType` gate — is to
enforce the addon server-side; these two endpoints were the gap.

Because `isClaimableType` returns true for any type with
`preset_category == "character"` or a slug ending in `-character` (the legacy
heuristic), essentially **every** campaign has a claimable-by-heuristic character
type *even if the addon was never enabled*. So with the feature toggled off, a
plain campaign member (Player) could still:

```
POST /campaigns/<cid>/entities/<eid>/claim
```

and set themselves as `owner_user_id` of a character entity. A Scribe could
likewise reassign/clear ownership via `PUT …/owner`.

**Impact.** Authorization / feature-boundary bypass reachable by the
lowest-privilege role. `owner_user_id` does **not** confer view/edit through the
permission system (`CheckEntityAccess` / `GetEffectivePermission` ignore it), so
this is not a direct read/write escalation. The concrete effects are:
(1) the feature operates while the Owner has it disabled; (2) denial — the
rightful claimant/GM is then blocked by the 409 "already claimed", and per
product design must *re-enable* the addon to manage owners; (3) it is the
enabler for the disclosure in **L-2**.

**Fix.** Enforce the addon at the service layer (the single choke point for all
callers), mirroring `CreateEntityType`'s existing gate:

```go
// ClaimEntity / AssignOwner, after loading the entity:
if !s.isAddonEnabled(ctx, entity.CampaignID, AddonPlayerCharacterClaiming) {
    return nil, apperror.NewForbidden("player character claiming is not enabled for this campaign")
}
```

Tests: `TestClaimEntity_RejectsWhenAddonDisabled` (403, no DB write),
`TestClaimEntity_AllowedWhenAddonEnabled` (positive control),
`TestAssignOwner_AddonAndTypeGates`.

**Residual risk (accepted).** `isAddonEnabled` is *fail-open* (a transient
addon-lookup error allows the action), matching the existing `RequireAddon`
middleware and `CreateEntityType` convention. Closing this would require making
the whole convention fail-closed; out of scope for this change.

---

## LOW Severity

### L-1: `AssignOwner` accepted non-claimable entity types

**Location:** `internal/plugins/entities/service.go` — `AssignOwner`.

**Description.** `ClaimEntity` guards on `isClaimableType`, but `AssignOwner`
(Scribe+) applied no type guard. A Scribe could therefore `PUT …/owner` against
a **Location**, **Item**, or any non-character entity and stamp an
`owner_user_id` on it. Since `ListByOwner` (the "My Characters" query) is neither
visibility- nor type-filtered, that arbitrary entity would then appear in the
target player's "My Characters" list.

**Impact.** Low — requires the trusted Scribe role and confers no permission
escalation, but it is an inconsistency with the claim path and pollutes the
player landing page with non-character entities.

**Fix.** Mirror the claim-path guard, but only when *assigning* (clearing a
stale owner remains allowed regardless of the type's current flag):

```go
if ownerUserID != nil {
    et, err := s.types.FindByID(ctx, entity.EntityTypeID)
    if err != nil { return nil, apperror.NewInternal(...) }
    if !isClaimableType(et) {
        return nil, apperror.NewBadRequest("only character entities can be assigned an owner")
    }
}
```

Covered by `TestAssignOwner_AddonAndTypeGates` (assign→400 for a Location;
clear→allowed) and the updated `TestAssignOwner_SetAndClear`.

### L-2: `ClaimEntity` performed no view/ACL check on the target entity

**Location:** `internal/plugins/entities/handler.go` — `ClaimEntity`;
`internal/plugins/entities/repository.go:1166` (`ListByOwner`).

**Description.** `ClaimEntity` verified the entity belonged to the URL campaign
(IDOR) but never checked the caller could actually *see* the entity. The claim
button only renders on entities a player can view, but a hand-rolled `POST` with
a known entity UUID let a member claim an entity they otherwise cannot see — e.g.
an unclaimed, claimable, **private/dm-only** character. Because `ListByOwner`
intentionally skips visibility filtering, the now-"owned" hidden entity's name,
type, and icon then surface on the claimant's "My Characters" page (the entity
*show* page still blocks them, so this is name/metadata disclosure, not body
content).

**Impact.** Low — intra-campaign horizontal disclosure of a hidden character's
name/type, gated on the attacker already knowing the (random UUID) entity id,
which players cannot normally enumerate. Also enables silently locking a hidden
character to an unauthorized owner.

**Fix.** Add the same view gate the `Show` handler applies, using the *real*
member role (not the view-as-player override), before delegating to the service:

```go
access, err := h.service.CheckEntityAccess(ctx, entityID, int(cc.MemberRole), userID)
if err != nil { return err }
if !access.CanView { return apperror.NewNotFound("entity not found") }
```

Legitimate claims (initiated from a visible list/show page) are unaffected — you
can only claim what you can see.

---

## INFORMATIONAL

### I-1: `entityService.Create` trusts the caller to validate `owner_user_id` membership

**Location:** `internal/plugins/entities/service.go:329-340`.

`Create` accepts `OwnerUserID` and persists it after only a trim, with a comment
delegating cross-campaign membership validation to the call site. Today this is
**safe**: the only path that sets owner at creation is `syncapi.CreateEntity`,
which validates membership via `GetMember`
(`internal/plugins/syncapi/api_handler.go:309-314`); the batch-sync `create`
branch (`:605`) does not pass an owner; and the web create handler does not set
one. The risk is purely latent — a future caller that forwards `OwnerUserID`
without validating would silently create a cross-campaign/orphaned claim.

**Recommendation (defense-in-depth).** Either validate `owner_user_id` against
campaign membership inside `Create` (inject a member checker), or convert the
comment into an enforced invariant. Not fixed here to avoid widening the service
constructor; flagged for the backlog.

---

## Verified Secure (no change required)

The following properties named in the review scope were tested and found correct
as merged:

- **IDOR — entity must belong to the URL campaign.** Both handlers reject a
  mismatch with 404 (`handler.go:3182` claim, `:3227` owner).
- **Role enforcement.** Claim is `RequireRole(RolePlayer)`; owner is
  `RequireRole(RoleScribe)` (`routes.go:89,91`), under the auth +
  `RequireCampaignAccess` group.
- **Already-claimed.** `ClaimEntity` returns 409 when owned by another user and
  is a no-op for the same user (`service.go:917-923`).
- **Non-claimable (claim path).** `isClaimableType` guard returns 400
  (`service.go:930-932`); explicit `claimable=FALSE` overrides the heuristic.
- **Cross-campaign claim/assign.** Blocked by the IDOR check, and `AssignOwner`
  validates `owner_user_id` against the **URL campaign's** members, so an owner
  from another campaign cannot be assigned (`handler.go:3243-3257`).
- **`owner_user_id` membership validation (Foundry/syncapi).**
  `syncapi.CreateEntity` validates via `GetMember`
  (`api_handler.go:309-314`); `AssignOwner` validates via `ListMembers`
  (`handler.go:3243-3257`).
- **Per-type `claimable` is Owner-only.** Every `…/entity-types*` mutation is
  `RequireRole(RoleOwner)` (`routes.go:106-135`); the `syncapi` entity-type DTOs
  do not expose `claimable` or `preset_category`, so no Player/Scribe or API path
  can flip it. PC sub-type creation is addon-gated in `CreateEntityType`
  (`service.go:1267-1271`).
- **Audit integrity.** `entity.claimed` / `entity.owner_changed` are written via
  `logAudit` / `logAuditWithDetails`, which set the actor from the server-side
  session (`auth.GetUserID(c)`, `handler.go:252,271`) and use server-set action
  constants. The client cannot spoof the acting user or the action; the only
  client-influenced detail (`new_owner_id/name`) is membership-validated first.

---

## Changes in this branch

| File | Change |
|------|--------|
| `internal/plugins/entities/service.go` | M-1: addon gate in `ClaimEntity` + `AssignOwner`; L-1: claimable-type guard in `AssignOwner` (assign only) |
| `internal/plugins/entities/handler.go` | L-2: `CheckEntityAccess` view gate in `ClaimEntity` |
| `internal/plugins/entities/service_test.go` | Gate tests + updated `TestAssignOwner_SetAndClear` |

**Verification:** `templ generate`, `go build ./...`, `go vet`,
`go test ./... -short` (all packages green), and
`golangci-lint run ./internal/plugins/entities/...` (0 issues).
