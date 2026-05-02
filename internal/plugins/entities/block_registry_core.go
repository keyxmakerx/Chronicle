// block_registry_core.go registers the core block types that are always
// available (no addon requirement). Template blocks wrap the templ components
// defined in show.templ. Dashboard blocks have nil renderers (rendered by
// the Go dashboard handler, not the block registry).
package entities

import "github.com/a-h/templ"

// IntPtr is a helper for inline *int literals in ConfigFieldMeta.
func IntPtr(n int) *int { return &n }

// RegisterCoreBlocks adds core block types to the registry. Called during
// NewBlockRegistry or app startup before plugins register their own blocks.
func RegisterCoreBlocks(r *BlockRegistry) {
	// ---------------------------------------------------------------
	// Template (entity page) blocks — Contexts: ["template"]
	// ---------------------------------------------------------------

	r.Register(BlockMeta{
		Type: "title", Label: "Title", Icon: "fa-heading",
		Description: "Entity name and actions",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTitle(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "image", Label: "Image", Icon: "fa-image",
		Description: "Header image with upload",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockImage(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "entry", Label: "Rich Text", Icon: "fa-align-left",
		Description: "Main content editor",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockEntry(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "attributes", Label: "Attributes", Icon: "fa-list",
		Description: "Custom field values",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockAttributes(ctx.CC, ctx.Entity, ctx.EntityType, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "details", Label: "Details", Icon: "fa-info-circle",
		Description: "Metadata and dates",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockDetails(ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: "tags", Label: "Tags", Icon: "fa-tags",
		Description: "Tag picker widget",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTags(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "relations", Label: "Relations", Icon: "fa-link",
		Description: "Entity relation links",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockRelations(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "divider", Label: "Divider", Icon: "fa-minus",
		Description: "Horizontal separator",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockDivider()
	})

	r.Register(BlockMeta{
		Type: "posts", Label: "Posts", Icon: "fa-layer-group",
		Description: "Sub-notes and additional content sections",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockPosts(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	// Player-facing per-user notes on entity pages. Each member sees
	// their own notes plus whatever others have shared with them
	// according to the audience field (private / dm_only / dm_scribe /
	// everyone / custom). Backed by internal/widgets/entity_notes; the
	// data layer enforces visibility, this block is just the mount
	// point. Gated by the "player-notes" addon so campaigns can disable
	// it cleanly without leaving an empty block lying around.
	r.Register(BlockMeta{
		Type: "entity_notes", Label: "Player Notes", Icon: "fa-sticky-note",
		Description: "Per-user notes with private / DM-only / shared / custom audiences",
		Addon:       "player-notes",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockEntityNotes(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "shop_inventory", Label: "Shop Inventory", Icon: "fa-store",
		Description: "Shop items with prices",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockShopInventory(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "inventory", Label: "Inventory", Icon: "fa-shield-halved",
		Description: "Character inventory — items with quantity, equipped, and attuned",
		Addon: "armory", Contexts: []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockInventory(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "transaction_log", Label: "Transaction Log", Icon: "fa-receipt",
		Description: "Purchase and sale history for shops",
		Addon: "armory", Contexts: []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTransactionLog(ctx.CC, ctx.Entity)
	})

	// text_block is shared between dashboard and template contexts.
	r.Register(BlockMeta{
		Type: "text_block", Label: "Text Block", Icon: "fa-align-left",
		Description: "Custom static HTML content",
		Contexts:    []string{"dashboard", "template"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "content", Label: "HTML Content", Type: "textarea"},
		},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTextBlock(ctx.Block.Config)
	})

	// Extension widget block — generic mount point for extension-provided JS widgets.
	r.Register(BlockMeta{
		Type: "ext_widget", Label: "Extension Widget", Icon: "fa-puzzle-piece",
		Description: "Widget provided by an extension",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockExtWidget(ctx.CC, ctx.Entity, ctx.Block)
	})

	// Cover image block — full-width banner/hero image for entity pages.
	r.Register(BlockMeta{
		Type: "cover_image", Label: "Cover Image", Icon: "fa-panorama",
		Description: "Full-width banner image",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockCoverImage(ctx.CC, ctx.Entity, ctx.CSRFToken, ctx.Block.Config)
	})

	// Local graph block — mini-graph showing entity's neighborhood.
	r.Register(BlockMeta{
		Type: "local_graph", Label: "Local Graph", Icon: "fa-diagram-project",
		Description: "Entity relationship neighborhood",
		Contexts:    []string{"template"},
	}, func(ctx BlockRenderContext) templ.Component {
		return blockLocalGraph(ctx.CC, ctx.Entity, ctx.Block.Config)
	})

	// Container layout types — rendered by the layout editor JS, not by
	// server-side templ. Registered here so they pass validation.
	r.Register(BlockMeta{
		Type: "two_column", Label: "2 Columns", Icon: "fa-columns",
		Description: "Side-by-side columns", Container: true,
		Contexts: []string{"template"},
	}, nil)

	r.Register(BlockMeta{
		Type: "three_column", Label: "3 Columns", Icon: "fa-table-columns",
		Description: "Three equal columns", Container: true,
		Contexts: []string{"template"},
	}, nil)

	r.Register(BlockMeta{
		Type: "tabs", Label: "Tabs", Icon: "fa-folder",
		Description: "Tabbed content sections", Container: true,
		Contexts: []string{"template"},
	}, nil)

	r.Register(BlockMeta{
		Type: "section", Label: "Section", Icon: "fa-caret-down",
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
		Type: "welcome_banner", Label: "Welcome Banner", Icon: "fa-flag",
		Description: "Campaign name & description hero",
		Contexts:    []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "category_grid", Label: "Category Grid", Icon: "fa-grid-2",
		Description: "Quick-nav entity type grid",
		Contexts:    []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "columns", Label: "Columns", Type: "number", Min: IntPtr(2), Max: IntPtr(6), Default: 4},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "recent_pages", Label: "Recent Pages", Icon: "fa-clock",
		Description: "Recently updated entities",
		Contexts:    []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Items to show", Type: "number", Min: IntPtr(4), Max: IntPtr(12), Default: 8},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "entity_list", Label: "Entity List", Icon: "fa-list",
		Description: "Filtered list by category",
		Contexts:    []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "entity_type_id", Label: "Entity Type", Type: "entity_type"},
			{Key: "limit", Label: "Items to show", Type: "number", Min: IntPtr(4), Max: IntPtr(20), Default: 8},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "pinned_pages", Label: "Pinned Pages", Icon: "fa-thumbtack",
		Description: "Hand-picked entity cards",
		Contexts:    []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "calendar_preview", Label: "Calendar", Icon: "fa-calendar-days",
		Description: "Upcoming calendar events",
		Addon: "calendar", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Events to show", Type: "number", Min: IntPtr(1), Max: IntPtr(20), Default: 5},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "timeline_preview", Label: "Timeline", Icon: "fa-timeline",
		Description: "Timeline list with event counts",
		Addon: "timeline", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Timelines to show", Type: "number", Min: IntPtr(1), Max: IntPtr(20), Default: 5},
		},
	}, nil)

	// NOTE: map_preview is registered in routes.go with both contexts
	// since it has a real templ renderer from the maps plugin.

	r.Register(BlockMeta{
		Type: "calendar_full", Label: "Full Calendar", Icon: "fa-calendar",
		Description: "Full interactive calendar grid",
		Addon: "calendar", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "timeline_full", Label: "Full Timeline", Icon: "fa-timeline",
		Description: "Full timeline D3 visualization",
		Addon: "timeline", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
			{Key: "timeline_id", Label: "Timeline ID", Type: "text"},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "relations_graph_full", Label: "Full Relations Graph", Icon: "fa-diagram-project",
		Description: "Large entity relations graph",
		Addon: "relations", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "height", Label: "Height (px)", Type: "number", Min: IntPtr(300), Max: IntPtr(1000), Default: 500},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "map_full", Label: "Full Map", Icon: "fa-map-location-dot",
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
		Type: "session_tracker", Label: "Sessions", Icon: "fa-dice-d20",
		Description: "Upcoming sessions with RSVP",
		Addon: "sessions", Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Sessions to show", Type: "number", Min: IntPtr(1), Max: IntPtr(20), Default: 5},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "activity_feed", Label: "Activity Feed", Icon: "fa-clock-rotate-left",
		Description: "Recent campaign activity log",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "limit", Label: "Entries to show", Type: "number", Min: IntPtr(1), Max: IntPtr(30), Default: 10},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "sync_status", Label: "Foundry Sync", Icon: "fa-plug",
		Description: "Foundry VTT sync status",
		Addon: "foundry", Contexts: []string{"dashboard"},
	}, nil)

	// Category dashboard blocks — only available in category dashboard context.
	r.Register(BlockMeta{
		Type: "category_header", Label: "Category Header", Icon: "fa-heading",
		Description: "Category name, icon, count, description",
		Contexts: []string{"dashboard"},
	}, nil)

	r.Register(BlockMeta{
		Type: "entity_grid", Label: "Entity Grid", Icon: "fa-grid-2",
		Description: "All entities as card grid",
		Contexts: []string{"dashboard"},
		ConfigFields: []ConfigFieldMeta{
			{Key: "columns", Label: "Columns", Type: "number", Min: IntPtr(2), Max: IntPtr(6), Default: 4},
		},
	}, nil)

	r.Register(BlockMeta{
		Type: "search_bar", Label: "Search Bar", Icon: "fa-search",
		Description: "Search input for filtering",
		Contexts: []string{"dashboard"},
	}, nil)
}
