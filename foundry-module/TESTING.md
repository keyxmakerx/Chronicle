# Foundry VTT Sync Module - E2E Testing Checklist

Manual testing checklist for the Chronicle-Foundry bidirectional sync module.
Requires a running Chronicle instance and Foundry VTT with the chronicle-sync module installed.

## Prerequisites

- [ ] Chronicle running with API key created for target campaign
- [ ] Foundry VTT world with chronicle-sync module enabled
- [ ] Module settings configured: API URL, API key, campaign ID
- [ ] Sync enabled in module settings

## Connection & Status

- [ ] Status indicator shows green dot when connected
- [ ] Status indicator shows yellow during reconnection
- [ ] Status indicator shows red when disconnected
- [ ] WebSocket auto-reconnects after network interruption (wait 30s)
- [ ] Message queue drains after reconnection

## Journal Sync (Entities)

### Chronicle -> Foundry
- [ ] Create entity in Chronicle -> JournalEntry appears in Foundry
- [ ] Update entity name -> JournalEntry name updates
- [ ] Update entity entry (rich text) -> JournalEntry pages update
- [ ] Toggle entity privacy -> JournalEntry ownership changes
- [ ] Delete entity -> JournalEntry removed

### Foundry -> Chronicle
- [ ] Create JournalEntry -> Entity appears in Chronicle
- [ ] Update JournalEntry name -> Entity name updates
- [ ] Edit JournalEntry page content -> Entity entry updates
- [ ] Delete JournalEntry -> Entity deleted in Chronicle

### Multi-Page Sync
- [ ] Entity with h1/h2 headings creates multiple Foundry journal pages
- [ ] Entity without headings creates single "Content" page
- [ ] Multi-page Foundry journal concatenates into single Chronicle entry
- [ ] Updating entity content adds/removes/updates pages correctly
- [ ] Page titles match heading text (HTML stripped)
- [ ] Pre-heading content creates "Overview" page

### Permission Sync
- [ ] Private entity (is_private=true) creates journal with default ownership NONE
- [ ] Public entity (is_private=false) creates journal with default ownership OBSERVER
- [ ] Custom visibility entity fetches permissions and maps role grants to ownership
- [ ] Custom visibility with player view grant → default OBSERVER
- [ ] Custom visibility with no player grant → default NONE
- [ ] Changing journal ownership in Foundry pushes is_private to Chronicle
- [ ] Changing journal ownership pushes visibility/permissions to Chronicle API
- [ ] Permission API failure falls back to binary is_private mapping

### Edge Cases
- [ ] Rapid successive edits don't create duplicate entities
- [ ] Sync guard prevents infinite loops (edit in A, syncs to B, doesn't re-sync to A)
- [ ] Monk's Enhanced Journal: content syncs correctly if module active

## Map Sync

### Chronicle -> Foundry
- [ ] Create drawing in Chronicle -> Drawing appears on Foundry scene
- [ ] Move/resize drawing -> Foundry drawing updates
- [ ] Delete drawing -> Foundry drawing removed
- [ ] Create token -> Token appears on Foundry scene
- [ ] Move token (PATCH position) -> Token position updates in Foundry
- [ ] Delete token -> Token removed from Foundry

### Foundry -> Chronicle
- [ ] Create drawing on scene -> Drawing syncs to Chronicle map
- [ ] Move drawing -> Chronicle drawing position updates
- [ ] Create token on scene -> Token syncs to Chronicle
- [ ] Move token -> Token position updates (debounced)
- [ ] Delete drawing/token -> Removed in Chronicle

### Coordinate Conversion
- [ ] Verify pixel-to-percentage conversion is accurate
- [ ] Drawing at (0,0) maps correctly
- [ ] Drawing at scene edge maps correctly

## Fog of War

### Chronicle -> Foundry
- [ ] Create fog region in Chronicle -> Semi-transparent polygon drawing appears on scene
- [ ] Create multiple fog regions -> All render correctly as overlay drawings
- [ ] Reset fog in Chronicle -> All fog drawings cleared from scene
- [ ] Fog region reconciliation: add/remove regions correctly on re-fetch

### Foundry -> Chronicle
- [ ] Draw a dark polygon (black fill, alpha > 0.5) -> Pushes as fog region to Chronicle
- [ ] Delete a fog drawing in Foundry -> Fog region deleted in Chronicle
- [ ] Non-fog polygon (light color or low alpha) -> Syncs as regular drawing, not fog

## Calendar Sync

### Chronicle -> Foundry
- [ ] Advance date in Chronicle -> Calendaria/SimpleCalendar date updates
- [ ] Create calendar event -> Event appears in calendar module
- [ ] Update event -> Calendar module event updates
- [ ] Delete event -> Calendar module event removed

### Foundry -> Chronicle
- [ ] Change date in Calendaria/SimpleCalendar -> Chronicle date updates
- [ ] Create event in calendar module -> Chronicle event created
- [ ] Update event -> Chronicle event updates
- [ ] Delete event -> Chronicle event removed

### Adapter Compatibility
- [ ] Test with Calendaria module active
- [ ] Test with SimpleCalendar module active (note 0-indexed months/days)
- [ ] Test with neither module -> Calendar sync gracefully disabled

## Shop Widget

- [ ] Right-click JournalEntry linked to Shop entity -> "Open Chronicle Shop" option appears
- [ ] Shop window opens with correct shop name
- [ ] Inventory loads from relations API (items with price/quantity metadata)
- [ ] Items show name, price, quantity
- [ ] Out-of-stock items visually distinct
- [ ] Drag item from shop -> Drop on character sheet -> Foundry Item created
- [ ] Real-time refresh: update shop entity in Chronicle -> Shop window updates
- [ ] Multiple shop windows can be open simultaneously
- [ ] Closing shop window cleans up properly

## Scene-to-Map Linking

- [ ] Right-click scene in nav bar -> "Link to Chronicle Map" option visible (GM only)
- [ ] Dialog shows all Chronicle maps for the campaign
- [ ] Selecting a map links the scene (sets flag)
- [ ] Unlinking clears the flag
- [ ] Auto-link: if campaign has exactly one map, scene auto-links on initial sync
- [ ] Multi-map warning: if campaign has multiple maps, log warning with instructions
- [ ] Linked scene shows correct map ID in flag inspector

## Initial Sync

- [ ] Fresh connection triggers initial sync (GET /sync/pull)
- [ ] Existing entities create proper sync mappings
- [ ] Existing maps/drawings/tokens have mappings created
- [ ] lastSyncTime updates after successful initial sync

## Permission & Security

- [ ] API key with read-only permission can't write via sync
- [ ] API key scoped to campaign A can't access campaign B data
- [ ] Disabled calendar addon -> Calendar API returns 404
- [ ] Disabled maps addon -> Maps API returns 404
- [ ] Private entities hidden from non-owner API keys
- [ ] Rate limiting enforced (60 req/min default)

## Character Sync (Actor ↔ Entity)

### Prerequisites
- [ ] Game system matches a Chronicle system (dnd5e or pf2e)
- [ ] "Sync Characters" enabled in module settings
- [ ] Character entity type exists in Chronicle campaign

### Chronicle -> Foundry
- [ ] Create character entity in Chronicle -> Actor (type: character) created in Foundry
- [ ] Update character entity fields -> Actor system data updates (ability scores, HP)
- [ ] Update character entity name -> Actor name updates
- [ ] Delete character entity -> Actor unlinked (flags removed) but NOT deleted

### Foundry -> Chronicle
- [ ] Create character Actor in Foundry -> Entity created in Chronicle with mapped fields
- [ ] Update Actor ability scores -> Chronicle entity fields_data updates
- [ ] Update Actor HP -> Chronicle entity hp_current/hp_max update
- [ ] Update Actor name -> Chronicle entity name updates
- [ ] Delete Actor -> Chronicle entity deleted

### Dashboard - Characters Tab
- [ ] Characters tab visible in sync dashboard
- [ ] System badge shows matched system name
- [ ] Synced actors show green check with "Synced" label
- [ ] Unlinked actors show "Not linked" with Push button
- [ ] Push button creates Chronicle entity and links actor
- [ ] Empty state shown when no character actors exist
- [ ] Disabled state shown when syncCharacters is off
- [ ] No-system state shown when game system doesn't match

### System Adapters
- [ ] D&D 5e: All 6 ability scores sync (str, dex, con, int, wis, cha)
- [ ] D&D 5e: HP current/max syncs bidirectionally
- [ ] D&D 5e: AC, speed, level, class, race, alignment, proficiency_bonus push to Chronicle
- [ ] PF2e: Ability mods sync to Chronicle (str_mod through cha_mod)
- [ ] PF2e: HP syncs bidirectionally
- [ ] PF2e: Only HP and name sync back from Chronicle (derived values protected)
- [ ] PF2e: ancestry, heritage, class, level, perception, speed push to Chronicle

### Edge Cases
- [ ] Actor sync disabled when no system adapter available
- [ ] Sync guard prevents infinite loops (change in A doesn't re-trigger back to A)
- [ ] Only character-type actors processed (NPCs, vehicles ignored)
- [ ] Only current user's changes pushed (other users' changes ignored)
- [ ] Pre-existing actors can be manually pushed via dashboard Push button

## Error Recovery

- [ ] Invalid API key shows clear error message
- [ ] Network timeout during sync doesn't corrupt state
- [ ] Partial sync failure (one entity fails) doesn't block others
- [ ] Module gracefully handles Chronicle server restart
