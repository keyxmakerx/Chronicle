# Player Character Claiming — Operator Guide

This guide walks you through enabling and using the Player Character Claiming feature in Chronicle.

## Overview

Player Character Claiming allows you to:
- Link player-owned characters to campaign members
- Let players claim unclaimed characters themselves
- See who owns which character on the Characters dashboard
- Reassign ownership between players or clear ownership entirely
- Track all claiming activity in the audit log

The feature is **optional** and must be explicitly enabled per campaign.

## Quick Start

### 1. Enable the Addon

The addon is switched on per campaign, by the campaign owner:

1. Open your campaign and click **Game & features** (on the campaign page,
   under Manage; the address is `/campaigns/<id>/extensions`)
2. Under **Features**, find **Player Character Claiming**
3. Click **Enable** (the button turns to **On** when active)
4. The addon is now active for this campaign only

### 2. A "Player Characters" Sub-Type Appears

After enabling the addon:
- Open the campaign's **Customize** page and click **Manage Categories**
  (or use **Manage** beside **Categories** on the campaign page)
- Enabling creates a category named **Player Characters**, nested under the
  default **Characters** category, unless the campaign already has one or has a
  game system's own character category (that one is nested under
  **Characters** instead and serves as the claimable type)
  - The new category is claimable by default
  - Characters in this category can be claimed by players

**Note:** A category with no explicit claimable setting counts as claimable
when its preset is a character or its slug ends in `-character`; the checkbox
described under Setup below overrides that either way.

### 3. Add Player Characters

Scribes and the owner create characters in the Player Characters category:
1. Go to the **Player Characters** category
2. Click **New Page**
3. Fill in the name, description, and fields as normal
4. Save

**Alternatively:** Create characters in any category and edit the category
settings to mark it as claimable (see §Setup below).

## Player Workflow: Claiming a Character

Once a character is created and unclaimed:

1. **Player opens the character page**
   - They see a banner at the top: "This character is unclaimed." with a
     **Claim character** button

2. **Player clicks the claim button**
   - The page reloads and the banner reads **"Claimed by <PlayerName>"**
   - The character is now linked to the player's account

3. **Result**
   - The character appears in the **Yours** band at the top of the
     **Characters** page, and in **The Party** band if its category is on the
     party list
   - The character's page shows the owner's name to everyone who can see it
   - The claim is recorded in the activity log

A player can only claim a character they can see. Claiming a character you
already own changes nothing.

**Conflict resolution:** If a player tries to claim a character already claimed
by someone else, they'll see an error. Only a Scribe or the owner can reassign
ownership.

## GM Workflow: Managing Ownership

### View All Owners

On any claimable category's dashboard (e.g., **Player Characters**):

1. Look at the **Player Character Roster** panel near the top of the dashboard
   (Scribe and owner only)
2. See a table of the characters in that category and their current owners
3. The panel shows:
   - Character name
   - Current owner ("Unclaimed", or "Unknown player" if the owner left the
     campaign)
   - Reassign dropdown (all campaign members)
   - Unclaim button (only on a claimed character)

### Reassign Ownership

1. Find the character in the **Player Character Roster**
2. Open the dropdown in the **Reassign** column
3. Select a new player from the list, or **Unclaimed** to clear the owner
4. The change is saved immediately and the page reloads
5. The activity log records the reassignment under the acting member's name

### Unclaim a Character

1. Find the character in the **Player Character Roster**
2. Click the **Unclaim** button
3. The owner is cleared
4. The character becomes available for any player to claim again

## Setup & Configuration

### Enable Claiming for an Existing Category

If you have an existing "Character" category and want to enable claiming:

1. Open **Customize** → **Manage Categories**
2. Find your Character category
3. Click **Edit** on its card
4. Check the box: **"Players can claim entities of this type"**
5. Save
6. Characters in this category are now claimable

### Disable Claiming for a Category

1. Open **Customize** → **Manage Categories**
2. Find the category (e.g., "Player Characters")
3. Click **Edit**
4. Uncheck: **"Players can claim entities of this type"**
5. Save
6. The claim button will no longer appear on characters in this category
7. The Player Character Roster will not appear on the dashboard

### Create a Custom Claimable Category

1. Open **Customize** → **Manage Categories**
2. Use the add-category form at the top
3. Name it (e.g., "Adventurers", "Company Members")
4. When the addon is enabled, a checkbox appears: **"Players can claim entities of this type"**
5. Check it to allow claiming for this new category
6. Click **Add Category**

## Audit & Accountability

All claiming and ownership changes are recorded in the campaign's activity log
(owner only; open **/campaigns/<id>/activity**):

### Claiming Activity

When a player claims a character:
- **Action:** `entity.claimed`
- **Label:** "claimed"
- **Entry:** "Alice claimed Tyne"
- **Visible in:** The activity log

### Ownership Reassignment

When a GM reassigns ownership:
- **Action:** `entity.owner_changed`
- **Label:** "reassigned owner of"
- **Entry:** "GM Bob reassigned owner of Tyne"
- **Visible in:** The activity log
- **Details:** The new owner's ID and display name (or `cleared`) are stored
  with the entry but not shown in the feed

## Disabling the Feature

### Temporarily Hide Claiming UI

If you disable the addon:
1. Go to **Game & features** for the campaign
2. Click **On** beside Player Character Claiming
3. The claiming controls disappear:
   - Claim buttons on character pages are hidden
   - The Player Character Roster is hidden from dashboards
   - Claiming toggles disappear from category settings
   - The claim and reassign requests are refused
   - A character that already has an owner still shows "Claimed by ..."

4. Existing ownership records are preserved
5. Re-enabling the addon shows everything again

**Note:** Toggling off does NOT delete ownership data. Owners remain linked
to their characters; the UI just hides until you re-enable the addon.

### Permanently Remove Claiming

If you want to clear all ownership links (not recommended):
1. For each claimable category, go to the dashboard
2. Use the Player Character Roster to unclaim all characters
3. Then disable the addon (the roster is hidden once it is off)

There is no bulk-clear screen; the server administrator can clear the
`owner_user_id` column of the `entities` table directly.

## Troubleshooting

### "Only character entities can be claimed"

You're trying to claim a non-character entity (e.g., a Location or Item).
Only entity types marked as claimable can be claimed. Check the category
settings under Customize → Manage Categories.

### "Entity is already claimed by another player"

The character is already owned by someone else. Only the GM can reassign it.
Ask the GM to use the Player Character Roster to reassign.

### Claim button doesn't appear

1. **Is the addon enabled?** Open Game & features and check Player Character
   Claiming shows **On**.
2. **Is the entity type claimable?** Open Customize → Manage Categories and
   check the "Players can claim" toggle for the category.
3. **Is the character already claimed?** If so, the banner shows who owns it
   instead of the button.
4. **Can the player see the page?** A player can't claim a page hidden from them.

### Owner shows as "a player" instead of a name

The owner's account was deleted or is no longer in the campaign. The character
still has a link to a user ID, but the display name couldn't be resolved.
The GM can reassign the character to an active player via the Player Character Roster.

## Best Practices

1. **Enable once at campaign start** — Enabling mid-campaign can surprise
   players with new UI. Enable during setup if you plan to use the feature.

2. **Create dedicated categories** — Use "Player Characters" for PCs and
   "NPCs" or "Companions" for non-claimed entities. This keeps the UI clear.

3. **Use the audit trail** — Check the activity log to see who claimed what
   and when. It's useful for accountability and debugging.

4. **Educate your players** — Let them know they can self-claim characters.
   Not all campaigns expect this workflow; make the feature explicit.

## See Also

- **Architecture & Design:** `.ai/decisions.md` (ADR-039)
- **Technical Details:** `internal/plugins/entities/.ai.md` §"Player Character Claiming"
- **Routes** (under `/campaigns/:id`): `POST /entities/:eid/claim` (any
  member who can see the page) and `PUT /entities/:eid/owner` (Scribe and up;
  body `{"owner_user_id": "<user id>" | null}`). They are campaign page routes,
  not part of the public API in `docs/api/openapi.yaml`.
