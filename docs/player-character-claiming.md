# Player Character Claiming: Operator Guide

Player Character Claiming links player-owned characters to campaign members.
Players can claim unclaimed characters; Scribes and the owner can assign,
reassign and clear ownership. The Characters page shows who owns what, and
every change is in the audit log. It is optional and enabled per campaign.

## Enable

The campaign owner switches it on:

1. Open the campaign, then **Game & features** (under Manage;
   `/campaigns/<id>/extensions`).
2. Under **Features**, find **Player Character Claiming** and click **Enable**
   (it shows **On** when active).

### The "Player Characters" category

Enabling creates a **Player Characters** category nested under the default
**Characters** category, claimable by default. It is not created if the
campaign already has one or has a game system; then the system's character sheet
and fields live on **Characters** itself, which is the claimable type. See it
under **Customize** > **Manage Categories** (or **Manage** beside **Categories**
on the campaign page).

A campaign with a separate system character category (e.g. **Heroes**) beside
**Characters** shows "Your characters are split between ..." on the addon's
setup page. Answering the question there moves its pages into **Characters**,
which takes over the sheet and fields. The old category is turned off, not
deleted, and claims stay.

A category with no explicit claimable setting counts as claimable when its
preset is a character or its slug ends in `-character`. The checkbox below
overrides that either way.

Scribes and the owner create characters with **New Page** in the Player
Characters category, or in any category marked claimable.

## Player workflow

1. The player opens an unclaimed character and sees "This character is
   unclaimed." with a **Claim character** button.
2. Clicking it reloads the page with **Claimed by <PlayerName>**.
3. The character appears in the **Yours** band of the **Characters** page (and
   in **The Party** band if its category is on the party list), the page shows
   the owner's name to everyone who can see it, and the claim is logged.

A player can claim only a character they can see; claiming one you already own
changes nothing. Claiming one owned by someone else shows an error; only a
Scribe or the owner can reassign.

## GM workflow: the Players box

On the **Characters** page, Scribes and the owner get a **Players** button in
the Party head:

- One row per player, with their character as a chip. The **x** on a chip
  unclaims it (owner cleared, claimable again).
- **Give a character** opens a finder; pick a result. With nothing typed it
  lists only free characters. Reassigning is the same: pick the new player.
- **Free to pick** holds playable characters with no owner. **x** removes one
  from the list; **Add a character** adds one.
- A player with no character sees **Pick your character** above the Party,
  listing the free characters they can see.

## Claimable categories

Under **Customize** > **Manage Categories**:

- **Enable on an existing category:** **Edit** its card, check **Players can
  claim entities of this type**, save.
- **Disable:** uncheck it and save. The claim button disappears and its
  characters leave the Players box.
- **New claimable category:** in the add-category form, check the same box
  (visible when the addon is on) and click **Add Category**.

## Audit

Owner only, at `/campaigns/<id>/activity`:

| Action | Label | Example |
|---|---|---|
| `entity.claimed` | claimed | "Alice claimed Tyne" |
| `entity.owner_changed` | reassigned owner of | "GM Bob reassigned owner of Tyne" |

The new owner's ID and display name (or `cleared`) are stored with the entry
but not shown in the feed.

## Disabling

Click **On** beside Player Character Claiming under **Game & features**. Then:

- Claim buttons, the Players button and box, and the category claim toggles are
  hidden, and claim and reassign requests are refused.
- A character that already has an owner still shows "Claimed by ...".
- Ownership records are kept; re-enabling shows everything again.

There is no bulk-clear screen. To clear ownership, click **x** on each chip in
the Players box before disabling, or have the server administrator clear
`entities.owner_user_id` directly.

## Troubleshooting

| Message or symptom | Cause and fix |
|---|---|
| "only character entities can be claimed" | The entity's type is not claimable. Check the category's claim checkbox. |
| "entity is already claimed by another player" | Ask a Scribe or the owner to reassign it from the Players box. |
| "player character claiming is not enabled for this campaign" | The addon is off. |
| No claim button | Check, in order: addon shows **On**; the category's claim checkbox; the character is not already claimed (the banner shows the owner); the player can see the page. |
| Owner shows as "a player" | The owner's account was deleted or left the campaign; the user ID has no name. Reassign from the Players box. |

Enable the addon at campaign start so players are not surprised by new UI, and
keep PCs in a dedicated category apart from NPCs and Companions.

## See also

- Design: `.ai/decisions.md` (ADR-039). Technical details:
  `internal/plugins/entities/.ai.md` §"Player Character Claiming".
- Routes (under `/campaigns/:id`): `POST /entities/:eid/claim` (Player and up,
  must be able to see the page) and `PUT /entities/:eid/owner` (Scribe and up;
  body `{"owner_user_id": "<user id>" | null}`). They are campaign page routes,
  not part of the public API in `docs/api/openapi.yaml`.
