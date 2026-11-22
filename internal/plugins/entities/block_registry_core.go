// block_registry_core.go registers the core block types that are always
// available (no addon requirement). Template blocks wrap the templ components
// defined in show.templ. Dashboard blocks have nil renderers (rendered by
// the Go dashboard handler, not the block registry).
package entities

import (
	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// IntPtr is a helper for inline *int literals in ConfigFieldMeta.
func IntPtr(n int) *int { return &n }

// RegisterCoreBlocks adds core block types to the registry. Called during
// NewBlockRegistry or app startup before plugins register their own blocks.
func RegisterCoreBlocks(r *BlockRegistry) {
	// The blocks other plugins draw can also sit on a category's home, as can
	// the older home blocks the campaigns plugin draws.
	registerPlaceableCategoryBlocks()
	registerHomeCategoryBlocks()

	// ---------------------------------------------------------------
	// Template (entity page) blocks — Contexts: ["template"]
	// ---------------------------------------------------------------

	r.Register(BlockMeta{
		Type: "title", Label: "Title", Icon: "fa-heading", Picture: PictureHeading,
		TileSizes: []string{"2x1", "4x1"},
		Description: "The page's name and actions",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTitle(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "image", Label: "Image", Icon: "fa-image", Picture: PictureImage,
		TileSizes: []string{"1x2", "2x2", "4x2"},
		Description: "Header image with upload",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockImage(ctx.CC, ctx.Entity, ctx.EntityType, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "entry", Label: "Rich text", Icon: "fa-align-left", Picture: PictureLines,
		TileSizes: []string{"2x2", "4x2"},
		Description: "Main content editor",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockEntry(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "attributes", Label: "Attributes", Icon: "fa-list", Picture: PictureFacts, ShowsSheet: true,
		TileSizes: []string{"2x2", "2x1", "4x2"},
		Description: "Custom field values",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockAttributes(ctx.CC, ctx.Entity, ctx.EntityType, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "character_surface", Label: "Character sheet", Icon: "fa-id-badge", Picture: PictureSheet, ShowsSheet: true,
		Description: "Dynamic character sheet — expandable boxes built from the page's portrait, fields, and description. The default for player characters.",
		Contexts:    []string{"template"},
		Singleton:   true,
	}, func(ctx BlockRenderContext) templ.Component {
		return blockCharacterSurface(ctx.CC, ctx.Entity, ctx.EntityType, ctx.CSRFToken, ctx.UserID)
	})

	registerContentsBlock(r)

	r.Register(BlockMeta{
		Type: "details", Label: "Details", Icon: "fa-info-circle", Picture: PictureFacts,
		TileSizes: []string{"2x1", "2x2"},
		Description: "Metadata and dates",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockDetails(ctx.CC, ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: "tags", Label: "Tags", Icon: "fa-tags", Picture: PictureTags,
		TileSizes: []string{"1x1", "2x1"},
		Description: "Tag picker widget",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTags(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "relations", Label: "Relations", Icon: "fa-link", Picture: PictureLinks,
		TileSizes: []string{"2x2", "4x2"},
		Description: "Links to related pages",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockRelations(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "divider", Label: "Divider", Icon: "fa-minus", Picture: PictureDivider,
		Description: "Horizontal separator",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockDivider()
	})

	r.Register(BlockMeta{
		Type: "posts", Label: "Posts", Icon: "fa-layer-group", Picture: PictureFeed,
		Description: "Sub-notes and additional content sections",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockPosts(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	// Player-facing per-user notes on entity pages, visible per the
	// audience field (private / dm_only / dm_scribe / everyone / custom).
	// Backed by internal/widgets/entity_notes, which enforces visibility;
	// this block is just the mount point. Gated by the "player-notes" addon.
	r.Register(BlockMeta{
		Type: "entity_notes", Label: "Player notes", Icon: "fa-sticky-note", Picture: PictureFeed,
		Description: "Per-user notes with private / DM-only / shared / custom audiences",
		Addon:       "player-notes",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		// Every entity_notes route needs a signed-in member (RolePlayer), so
		// a visitor (signed out, or on a look-around pass) gets no block
		// rather than one that fails to load and offers New note.
		if ctx.UserID == "" || ctx.CC == nil || ctx.CC.MemberRole < campaigns.RolePlayer {
			return templ.NopComponent
		}
		return blockEntityNotes(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "shop_inventory", Label: "Shop", Icon: "fa-store", Picture: PictureGrid,
		Description: "A shop players step into to browse and buy",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockShopInventory(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	// How a page shows its magic: an item's glow, sound and strike, or the
	// look a Remnant's page passes to the items made from it. Any category
	// may place it; for players it hides itself when there is no look.
	r.Register(BlockMeta{
		Type: "item_look", Label: "Look", Icon: "fa-wand-magic-sparkles", Picture: PictureImage,
		Description: "How an item glows, sounds and strikes, or the look a page passes to items made from it",
		Addon: addonArmory, Contexts: []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockItemLook(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	// The "permissions" block is retired (ADR-057): editing lives only in
	// edit mode via form.templ's inline widget mount. A stored
	// layout may still reference the unregistered type; RenderBlock drops
	// an unregistered block type silently, so no migration is needed. See
	// TestStoredRowPermRendersWithoutPermissionsBlock.

	r.Register(BlockMeta{
		Type: "inventory", Label: "Inventory", Icon: "fa-shield-halved", Picture: PictureTable,
		Description: "Character inventory — items with quantity, equipped, and attuned",
		Addon: addonArmory, Contexts: []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockInventory(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "transaction_log", Label: "Transaction log", Icon: "fa-receipt", Picture: PictureTable,
		Description: "Purchase and sale history for shops",
		Addon: addonArmory, Contexts: []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTransactionLog(ctx.CC, ctx.Entity)
	})

	// Page pieces drawn under every layout; see page_extras.go.
	r.Register(BlockMeta{
		Type: BlockSubPages, Label: "Sub-pages", Icon: "fa-sitemap", Picture: PictureLinks,
		Description: "This page's sub-pages, with a button to add one",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockSubPages(ctx.CC, ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: BlockWritingPrompts, Label: "Writing prompts", Icon: "fa-lightbulb", Picture: PictureLines,
		Description: "Prompts to help write this page (shown to scribes and the owner)",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockWritingPrompts(ctx.CC, ctx.EntityType)
	})

	r.Register(BlockMeta{
		Type: BlockBacklinks, Label: "Backlinks", Icon: "fa-link", Picture: PictureLinks,
		Description: "Pages that mention this one",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockBacklinksMount(ctx.CC, ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: BlockSystemPanels, Label: "Game system panels", Icon: "fa-dice-d20", Picture: PictureSheet, ShowsSheet: true,
		Description: "Panels the game system adds to NPC pages",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return entitySystemPanels(ctx.CC, ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: BlockGameSystemSheet, Label: "Game system sheet", Icon: "fa-scroll", Picture: PictureSheet, ShowsSheet: true,
		Description: "The sheet your game system draws for this page, like a Draw Steel hero's",
		Contexts:    []string{"template"},
		Singleton:   true,
	}, func(ctx BlockRenderContext) templ.Component {
		return blockGameSystemSheet(ctx.CC, ctx.Entity, ctx.EntityType, ctx.CSRFToken, ctx.UserID)
	})

	// Items & Money (BlockCharacterItems) is registered by app beside the
	// page panel it shows, which names the addon that gates it.

	// text_block is shared between dashboard and template contexts.
	r.Register(BlockMeta{
		Type: "text_block", Label: "Text block", Icon: "fa-align-left", Picture: PictureLines,
		TileSizes: []string{"1x1", "2x1", "2x2", "4x2"},
		Description: "Your own words: notes, links or instructions",
		Contexts:    []string{"dashboard", "template"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "content", Label: "Text", Type: "textarea", Hint: "Type plain text. A blank line starts a new paragraph."},
		},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTextBlock(ctx.Block.Config)
	})

	// Extension widget block — generic mount point for extension-provided JS widgets.
	r.Register(BlockMeta{
		Type: "ext_widget", Label: "Extension widget", Icon: "fa-puzzle-piece", Picture: PictureLines,
		Description: "Widget provided by an extension",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockExtWidget(ctx.CC, ctx.Entity, ctx.Block)
	})

	// Cover image block — full-width banner/hero image for entity pages.
	r.Register(BlockMeta{
		Type: "cover_image", Label: "Cover image", Icon: "fa-panorama", Picture: PictureImage,
		Description: "Full-width banner image",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockCoverImage(ctx.CC, ctx.Entity, ctx.CSRFToken, ctx.Block.Config)
	})

	// Local graph block — mini-graph showing entity's neighborhood.
	r.Register(BlockMeta{
		Type: "local_graph", Label: "Local graph", Icon: "fa-diagram-project", Picture: PictureGraph,
		Description: "The pages around this one, as a graph",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockLocalGraph(ctx.CC, ctx.Entity, ctx.Block.Config)
	})

	// Container layout types — rendered by the layout editor JS, not by
	// server-side templ. Registered here so they pass validation.
	r.Register(BlockMeta{
		Type: "two_column", Label: "Two columns", Icon: "fa-columns", Picture: PictureGrid,
		Description: "Side-by-side columns", Container: true,
		Contexts: []string{"template"},
	}, nil)

	r.Register(BlockMeta{
		Type: "three_column", Label: "Three columns", Icon: "fa-table-columns", Picture: PictureGrid,
		Description: "Three equal columns", Container: true,
		Contexts: []string{"template"},
	}, nil)

	r.Register(BlockMeta{
		Type: "tabs", Label: "Tabs", Icon: "fa-folder", Picture: PictureLines,
		Description: "Tabbed content sections", Container: true,
		Contexts: []string{"template"},
	}, nil)

	r.Register(BlockMeta{
		Type: "section", Label: "Section", Icon: "fa-caret-down", Picture: PictureLines,
		Description: "Collapsible accordion", Container: true,
		Contexts: []string{"template"},
	}, nil)

	// ---------------------------------------------------------------
	// Dashboard blocks — Contexts: ["dashboard"]
	// Rendered by the Go dashboard handler, not the block registry.
	// Registered here so the unified /block-types API and validation
	// are driven by a single source of truth.
	// ---------------------------------------------------------------

	r.Register(BlockMeta{
		Type: "welcome_banner", Label: "Welcome banner", Icon: "fa-flag", Picture: PictureHeading,
		Description: "Campaign name & description hero",
		Contexts:    []string{"dashboard"},
	}, nil)

	// Three-card row matching the legacy hardcoded dashboard's All Pages /
	// Members / Categories shortcuts. Lifted into a block so the default
	// dashboard can be expressed entirely as a layout JSON, which lets the
	// customization editor edit the same structure the live page renders.
	r.Register(BlockMeta{
		Type: "quick_actions", Label: "Quick actions", Icon: "fa-bolt", Picture: PictureTags,
		Description: "All pages / Members / Categories shortcut cards",
		Contexts:    []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "category_grid", Label: "Category grid", Icon: "fa-table-cells-large", Picture: PictureGrid,
		Description: "A grid of the categories, to jump to one",
		Contexts:    []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "columns", Label: "Columns", Type: "number", Min: IntPtr(2), Max: IntPtr(6), Default: 4},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "recent_pages", Label: "Recent pages", Icon: "fa-clock", Picture: PictureLinks,
		TileSizes: []string{"2x2", "4x2"},
		Description: "Pages changed lately",
		Contexts:    []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Items to show", Type: "number", Min: IntPtr(4), Max: IntPtr(12), Default: 8},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "entity_list", Label: "Page list", Icon: "fa-list", Picture: PictureLinks,
		Description: "Filtered list by category",
		Contexts:    []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "entity_type_id", Label: "Category", Type: "entity_type"},
			{Key: "limit", Label: "Items to show", Type: "number", Min: IntPtr(4), Max: IntPtr(20), Default: 8},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "pinned_pages", Label: "Pinned pages", Icon: "fa-thumbtack", Picture: PictureLinks,
		Description: "Pages pinned for everyone; on a campaign home or a page, each viewer's starred pages too",
		Contexts:    []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "calendar_preview", Label: "Calendar", Icon: "fa-calendar-days", Picture: PictureCalendar,
		Description: "Upcoming calendar events",
		Addon: "calendar", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Events to show", Type: "number", Min: IntPtr(1), Max: IntPtr(20), Default: 5},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "timeline_preview", Label: "Timeline", Icon: "fa-timeline", Picture: PictureTimeline,
		Description: "Timeline list with event counts",
		Addon: "timeline", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Timelines to show", Type: "number", Min: IntPtr(1), Max: IntPtr(20), Default: 5},
		},
	}, nil)

	// NOTE: map_editor (per-entity full editor) is registered in routes.go
	// because its renderer needs the maps service injected via closure.
	// map_full (dashboard map embed) lives below in this file.

	r.Register(BlockMeta{
		Type: "calendar_full", Label: "Full calendar", Icon: "fa-calendar", Picture: PictureCalendar,
		Description: "Full interactive calendar grid",
		Addon: "calendar", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "timeline_full", Label: "Full timeline", Icon: "fa-timeline", Picture: PictureTimeline,
		Description: "Full timeline D3 visualization",
		Addon: "timeline", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
			{Key: "timeline_id", Label: "Timeline ID", Type: "text"},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "relations_graph_full", Label: "Full relations graph", Icon: "fa-diagram-project", Picture: PictureGraph,
		Description: "A large graph of how pages relate",
		Addon: "relations", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "map_full", Label: "Full map", Icon: "fa-map-location-dot", Picture: PictureMap,
		Description: "Full map with drawings & tokens",
		Addon: "maps", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
			// "map" type renders as a dropdown of the campaign's maps in
			// the layout editor (vs. the prior plain-text UUID input).
			{Key: "map_id", Label: "Map", Type: "map"},
		},
	}, nil)

	r.Register(BlockMeta{
		// Coming up replaced this block; a saved one draws Coming up.
		Type: "session_tracker", Label: "Coming up", Icon: "fa-dice-d20", Picture: PictureCalendar,
		Description: "Replaced by Coming up",
		Addon: "sessions", Contexts: []string{"dashboard"}, Retired: true,
	}, nil)

	r.Register(BlockMeta{
		Type: "coming_up", Label: "Coming up", Icon: "fa-dice-d20", Picture: PictureCalendar,
		TileSizes: []string{"2x2", "2x1"},
		Description: "Game nights, what you owe the table, next events",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "start_here", Label: "Start here", Icon: "fa-flag", Picture: PictureCard,
		Description: "One page a new reader should open first",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			// "page" renders as a page search in the Customizer.
			{Key: "entity_id", Label: "Page", Type: "page", Hint: "Players see it only if they can open the page."},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "brochure", Label: "Brochure", Icon: "fa-book-open", Picture: PictureCard,
		Description: "A small box that opens a new-player page as a folded brochure",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "entity_id", Label: "Page", Type: "page", Hint: "Its title and first line make the cover; each heading starts a new panel. Players see it only if they can open the page."},
			{Key: "content", Label: "Words on the box", Type: "textarea", Hint: "Optional, shown on the small box. Leave empty to use the page's own first line."},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "at_a_glance", Label: "At a glance", Icon: "fa-list-ul", Picture: PictureFacts,
		Description: "The game system, next game, last session and page count",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "party", Label: "The party", Icon: "fa-users", Picture: PictureGrid,
		Description: "The players' heroes: name, ancestry, class, level and who plays them",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "your_hero", Label: "Your hero", Icon: "fa-user-shield", Picture: PictureCard,
		Description: "The viewer's own hero, or a way to choose one",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "getting_started", Label: "Getting started", Icon: "fa-list-check", Picture: PictureSteps,
		Description: "A new player's steps: choose a hero, say when you are free. Goes away when done",
		Addon: "sessions", Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "when_we_play", Label: "When we play", Icon: "fa-calendar-check", Picture: PictureCalendar,
		Description: "The times most of the table is free",
		Addon: "sessions", Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "when_we_could_play", Label: "When we could play", Icon: "fa-calendar-week", Picture: PictureCalendar,
		Description: "The next weeks: set game nights and roughly when the table tends to be free",
		Addon: "sessions", Contexts: []string{"dashboard", "template"},
	}, placeableBlockRenderer("when_we_could_play"))

	r.Register(BlockMeta{
		Type: "next_game", Label: "Next game", Icon: "fa-hourglass-half", Picture: PictureCounter,
		Description: "How many days until the next set game night, and your own answer",
		Addon: "sessions", Contexts: []string{"dashboard", "template"},
	}, placeableBlockRenderer("next_game"))

	r.Register(BlockMeta{
		Type: "today_in_world", Label: "Today in the world", Icon: "fa-sun", Picture: PictureCounter,
		Description: "The date in your world, today's weather and moons, and the next event on the calendar",
		Addon: addonCalendar, Contexts: []string{"dashboard", "template"},
	}, placeableBlockRenderer("today_in_world"))

	r.Register(BlockMeta{
		Type: "open_quests", Label: "Open quests", Icon: "fa-scroll", Picture: PictureSteps,
		Description: "The three open quests that most need attention: due soonest, then newest",
		Contexts: []string{"dashboard", "template"},
	}, placeableBlockRenderer("open_quests"))

	r.Register(BlockMeta{
		Type: "world_clocks", Label: "The world's clocks", Icon: "fa-stopwatch", Picture: PictureCounter,
		Description: "Trackers you show to everyone, drawn small as rings",
		Contexts: []string{"dashboard", "template"},
	}, placeableBlockRenderer("world_clocks"))

	r.Register(BlockMeta{
		Type: "since_last_here", Label: "Since you were last here", Icon: "fa-envelope-open-text", Picture: PictureFeed,
		Description: "What is new for each member since they last pressed Mark as seen: recaps, pages, quests and game nights they may open",
		Contexts: []string{"dashboard", "template"},
	}, placeableBlockRenderer("since_last_here"))

	r.Register(BlockMeta{
		Type: "where_we_play", Label: "Where we play", Icon: "fa-location-dot", Picture: PictureFacts,
		Description: "The Foundry address and voice channel you type in",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "foundry_url", Label: "Foundry address", Type: "text", Hint: "Where players open the game, like foundry.example.com."},
			{Key: "voice", Label: "Voice", Type: "text", Hint: "Where you talk, like \"Discord, the Ember Coast channel\"."},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "house_rules", Label: "House rules", Icon: "fa-scroll", Picture: PictureLines,
		Description: "The house chapters of the rulebook, as players can read them",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "last_time", Label: "Last time", Icon: "fa-clock-rotate-left", Picture: PictureLines,
		Description: "The latest session's recap, as players can read it",
		Addon: "sessions", Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "table_agreement", Label: "Table agreement", Icon: "fa-handshake", Picture: PictureLines,
		Description: "What the table agrees on: safety, pacing, absences",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "content", Label: "Text", Type: "textarea", Hint: "Players read this as you type it. Starter headings: Safety and lines, Pacing, Missing a night, Table talk."},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "making_a_hero", Label: "Making a hero", Icon: "fa-hammer", Picture: PictureLines,
		Description: "What a new player needs to know before building a hero",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "content", Label: "Text", Type: "textarea", Hint: "Starter headings: What is allowed, Starting level, Where to build it, Ask me first."},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "run_by", Label: "Run by", Icon: "fa-crown", Picture: PictureCard,
		Description: "Who runs the table: the owner and the people who help run it, by name",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "setup_steps", Label: "Setup steps", Icon: "fa-list-check", Picture: PictureSteps,
		Description: "The owner's \"Getting your table ready\" line, ticked from what exists",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "activity_feed", Label: "Activity feed", Icon: "fa-clock-rotate-left", Picture: PictureFeed,
		Description: "Recent campaign activity log",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Entries to show", Type: "number", Min: IntPtr(1), Max: IntPtr(30), Default: 10},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "sync_status", Label: "Foundry sync", Icon: "fa-plug", Picture: PictureFacts,
		Description: "Foundry VTT sync status",
		Addon: "foundry", Contexts: []string{"dashboard"},
	}, nil)

	// Category dashboard blocks — only available in category dashboard context.
	r.Register(BlockMeta{
		Type: "category_header", Label: "Category header", Icon: "fa-heading", Picture: PictureHeading,
		Description: "Category name, icon, count, description",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "entity_grid", Label: "Page grid", Icon: "fa-table-cells-large", Picture: PictureGrid,
		Description: "Pages as a grid of cards",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "columns", Label: "Columns", Type: "number", Min: IntPtr(2), Max: IntPtr(6), Default: 4},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "search_bar", Label: "Search bar", Icon: "fa-search", Picture: PictureSearch,
		Description: "Search input for filtering",
		Contexts: []string{"dashboard"},
	}, nil)

	// Every home block can also sit on an ordinary page. Done last so each
	// keeps the label, icon, addon, settings and tile sizes registered above.
	for _, t := range pageHomeBlockTypes {
		r.offerOnPages(t)
	}
}
