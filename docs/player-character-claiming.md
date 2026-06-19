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

1. Go to **Admin** → **Addons**
2. Find **Player Character Claiming** in the list
3. Click **Enable**
4. The addon is now active for your campaign

### 2. A "Player Characters" Sub-Type Appears

After enabling the addon:
- Visit **Customization Hub** → **Entities** → **Categories**
- You'll see a new category named **Player Characters** (if it wasn't already created)
  - This category is tagged `claimable=true` by default
  - Characters in this category can be claimed by players
  - It appears in the sidebar and the category dashboard

**Note:** If you already have a "Character" category and didn't enable the addon,
that category's claimable status depends on your setting. New campaigns default to
claimable for the Character category when the addon is on.

### 3. Add Player Characters

Create characters in the Player Characters category:
1. Go to **Player Characters** category
2. Click **New Character**
3. Fill in the name, description, and fields as normal
4. Save

**Alternatively:** Create characters in any category and edit the category
settings to mark it as claimable (see §Setup below).

## Player Workflow: Claiming a Character

Once a character is created and unclaimed:

1. **Player opens the character page**
   - They see a banner at the top: **"Claim this character"** (button)

2. **Player clicks the claim button**
   - The page updates to show: **"Claimed by <PlayerName>"**
   - The character is now linked to the player's account

3. **Result**
   - The character shows up on the player's dashboard
   - The character's detail page displays the owner's name
   - The owner's ID is recorded in the audit log

**Conflict resolution:** If a player tries to claim a character already claimed
by someone else, they'll see an error. Only the GM can reassign ownership.

## GM Workflow: Managing Ownership

### View All Owners

On any claimable category's dashboard (e.g., **Player Characters**):

1. Scroll down to the **Owner Roster** panel (Scribe+ only)
2. See a table of all characters and their current owners
3. The panel shows:
   - Character name
   - Current owner (or "unclaimed")
   - Reassign dropdown (all campaign members)
   - Unclaim button

### Reassign Ownership

1. Find the character in the **Owner Roster**
2. Click the dropdown next to the character's current owner
3. Select a new player from the list, or leave blank to unclaim
4. The change is saved immediately (PUT `/entities/:eid/owner`)
5. The audit log records the reassignment with the GM's name and the new owner's name

### Unclaim a Character

1. Find the character in the **Owner Roster**
2. Click the **Unclaim** button
3. The owner is cleared
4. The character becomes available for any player to claim again

## Setup & Configuration

### Enable Claiming for an Existing Category

If you have an existing "Character" category and want to enable claiming:

1. Go to **Customization Hub** → **Entities**
2. Find your Character category
3. Click **Edit** (or the quick-edit icon)
4. Check the box: **"Players can claim entities of this type"**
5. Save
6. Characters in this category are now claimable

### Disable Claiming for a Category

1. Go to **Customization Hub** → **Entities**
2. Find the category (e.g., "Player Characters")
3. Click **Edit**
4. Uncheck: **"Players can claim entities of this type"**
5. Save
6. The claim button will no longer appear on characters in this category
7. The Owner Roster will not appear on the dashboard

### Create a Custom Claimable Category

1. Go to **Categories** in the Customization Hub
2. Click **Add Category**
3. Name it (e.g., "Adventurers", "Company Members")
4. When the addon is enabled, a checkbox appears: **"Players can claim entities of this type"**
5. Check it to allow claiming for this new category
6. Save

The new category appears in the sidebar and on the main dashboard immediately.

## Audit & Accountability

All claiming and ownership changes are recorded in the **Activity Log**:

### Claiming Activity

When a player claims a character:
- **Action:** `entity.claimed`
- **Label:** "claimed"
- **Entry:** "Alice claimed Tyne"
- **Visible in:** Activity feed on the campaign dashboard

### Ownership Reassignment

When a GM reassigns ownership:
- **Action:** `entity.owner_changed`
- **Label:** "reassigned owner of"
- **Entry:** "GM Bob reassigned owner of Alice to Charlie" (or "unclaimed Alice")
- **Visible in:** Activity feed
- **Details:** New owner ID + display name (or "cleared" if unclaimed)

**Search & filter:** Click the activity action label to filter the feed.

## Disabling the Feature

### Temporarily Hide Claiming UI

If you disable the addon:
1. Go to **Admin** → **Addons**
2. Click **Disable** on Player Character Claiming
3. All claiming UI disappears:
   - Claim buttons on character pages are hidden
   - Owner Roster is hidden from dashboards
   - Claiming toggles disappear from category settings
4. Existing ownership records are preserved
5. Re-enabling the addon shows everything again

**Note:** Toggling off does NOT delete ownership data. Owners remain linked
to their characters; the UI just hides until you re-enable the addon.

### Permanently Remove Claiming

If you want to clear all ownership links (not recommended):
1. For each claimable category, go to the dashboard
2. Use the Owner Roster to unclaim all characters
3. Then disable the addon

Alternatively, contact the server administrator for a database migration
to bulk-clear `entity.owner_user_id` (if supported).

## Troubleshooting

### "Only character entities can be claimed"

You're trying to claim a non-character entity (e.g., a Location or Item).
Only entity types marked as claimable can be claimed. Check the category
settings in the Customization Hub.

### "Entity is already claimed by another player"

The character is already owned by someone else. Only the GM can reassign it.
Ask the GM to use the Owner Roster to reassign.

### Claim button doesn't appear

1. **Is the addon enabled?** Go to Admin → Addons and check.
2. **Is the entity type claimable?** Go to Customization Hub → Entities and
   check the "Players can claim" toggle for the category.
3. **Is the character already claimed?** If so, only the owner and the GM
   see the owner badge.

### Owner shows as "a player" instead of a name

The owner's account was deleted or is no longer in the campaign. The character
still has a link to a user ID, but the display name couldn't be resolved.
The GM can reassign the character to an active player via the Owner Roster.

## Best Practices

1. **Enable once at campaign start** — Enabling mid-campaign can surprise
   players with new UI. Enable during setup if you plan to use the feature.

2. **Create dedicated categories** — Use "Player Characters" for PCs and
   "NPCs" or "Companions" for non-claimed entities. This keeps the UI clear.

3. **Unclaim before deletion** — If you plan to delete a character, unclaim
   it first. This prevents orphaned ownership records.

4. **Use the audit trail** — Check the Activity Log to see who claimed what
   and when. It's useful for accountability and debugging.

5. **Educate your players** — Let them know they can self-claim characters.
   Not all campaigns expect this workflow; make the feature explicit.

## See Also

- **Architecture & Design:** `.ai/decisions.md` (ADR-039)
- **Technical Details:** `internal/plugins/entities/.ai.md` §"Player Character Claiming"
- **API Reference:** `docs/api/` (see POST /entities/:eid/claim and PUT /entities/:eid/owner)
