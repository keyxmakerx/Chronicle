# Glossary

<!-- ====================================================================== -->
<!-- Category: STATIC                                                         -->
<!-- Purpose: Defines TTRPG and Chronicle-specific terms so AI uses correct    -->
<!--          domain language consistently.                                   -->
<!-- Update: When new domain concepts are introduced.                         -->
<!-- ====================================================================== -->

## TTRPG Terms

- **TTRPG** -- Tabletop Role-Playing Game (D&D, Pathfinder, Draw Steel, etc.)
- **Campaign** -- An ongoing series of connected game sessions with shared narrative
- **Session** -- A single play meeting, typically 3-5 hours
- **PC (Player Character)** -- A character controlled by a player
- **NPC (Non-Player Character)** -- A character controlled by the Game Master
- **GM (Game Master)** -- The player who runs the game, controls NPCs, narrates the world.
  Also called DM (Dungeon Master) in D&D specifically.
- **World** -- The fictional setting where campaigns take place
- **Lore** -- Background information, history, and mythology of a world
- **Homebrew** -- Custom rules, items, or content created by the GM/players
- **SRD (System Reference Document)** -- Freely available game rules (e.g., D&D 5e SRD)
- **OGL (Open Game License)** -- License allowing use of game mechanics

## Game Systems (Modules)

- **D&D 5e** -- Dungeons & Dragons 5th Edition. Most popular TTRPG.
- **Pathfinder** -- D&D derivative with more mechanical depth. Uses OGL.
- **Draw Steel** -- Matt Colville's tactical TTRPG. Modern design.

## Chronicle-Specific Terms

- **Plugin** -- A self-contained feature application in `internal/plugins/`.
  Has its own handler, service, repository, and templates. Examples: auth,
  campaigns, entities, calendar, maps.
- **Module** -- A game system content pack in `internal/modules/`. Provides
  reference data, tooltips, and dedicated pages. Read-only. Examples: dnd5e,
  pathfinder, drawsteel.
- **Widget** -- A reusable UI building block in `internal/widgets/`. Mounts to a
  DOM element via `data-widget` attribute, fetches its own data. Examples:
  editor, title, tags, attributes, mentions.
- **Entity** -- The universal base type for all worldbuilding objects (characters,
  locations, items, etc.). Every "thing" in a campaign is an entity.
- **Entity Type** -- A configurable template that defines what fields an entity
  has. Can be customized per campaign. Default types: Character, Location,
  Organization, Item, Quest, etc.
- **Fragment** -- An HTML partial returned by an HTMX request (not a full page).
- **Layout** -- The outer HTML shell (head, nav, footer) that wraps page content.
- **Mount Point** -- A DOM element where a widget auto-mounts itself.
- **Entry** -- The rich text content body of an entity (stored as TipTap JSON).
- **Post** -- An additional rich text section attached to an entity.
- **Fields Data** -- The JSON blob storing an entity's type-specific field values.
