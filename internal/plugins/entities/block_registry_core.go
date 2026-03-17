// block_registry_core.go registers the core entity block types that are always
// available (no addon requirement). These wrap the templ components defined
// in show.templ.
package entities

import "github.com/a-h/templ"

// RegisterCoreBlocks adds core block types to the registry. Called during
// NewBlockRegistry or app startup before plugins register their own blocks.
func RegisterCoreBlocks(r *BlockRegistry) {
	r.Register(BlockMeta{
		Type: "title", Label: "Title", Icon: "fa-heading",
		Description: "Entity name and actions",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTitle(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "image", Label: "Image", Icon: "fa-image",
		Description: "Header image with upload",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockImage(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "entry", Label: "Rich Text", Icon: "fa-align-left",
		Description: "Main content editor",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockEntry(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "attributes", Label: "Attributes", Icon: "fa-list",
		Description: "Custom field values",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockAttributes(ctx.CC, ctx.Entity, ctx.EntityType, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "details", Label: "Details", Icon: "fa-info-circle",
		Description: "Metadata and dates",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockDetails(ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: "tags", Label: "Tags", Icon: "fa-tags",
		Description: "Tag picker widget",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTags(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "relations", Label: "Relations", Icon: "fa-link",
		Description: "Entity relation links",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockRelations(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "divider", Label: "Divider", Icon: "fa-minus",
		Description: "Horizontal separator",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockDivider()
	})

	r.Register(BlockMeta{
		Type: "posts", Label: "Posts", Icon: "fa-layer-group",
		Description: "Sub-notes and additional content sections",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockPosts(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "shop_inventory", Label: "Shop Inventory", Icon: "fa-store",
		Description: "Shop items with prices",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockShopInventory(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "inventory", Label: "Inventory", Icon: "fa-shield-halved",
		Description: "Character inventory — items with quantity, equipped, and attuned", Addon: "armory",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockInventory(ctx.CC, ctx.Entity, ctx.CSRFToken)
	})

	r.Register(BlockMeta{
		Type: "transaction_log", Label: "Transaction Log", Icon: "fa-receipt",
		Description: "Purchase and sale history for shops", Addon: "armory",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTransactionLog(ctx.CC, ctx.Entity)
	})

	r.Register(BlockMeta{
		Type: "text_block", Label: "Text Block", Icon: "fa-align-left",
		Description: "Custom static HTML content",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockTextBlock(ctx.Block.Config)
	})

	// Extension widget block — generic mount point for extension-provided JS widgets.
	// The widget slug is stored in block config and used as the data-widget attribute.
	// Not shown in the palette directly (extension widgets appear as individual block types).
	r.Register(BlockMeta{
		Type: "ext_widget", Label: "Extension Widget", Icon: "fa-puzzle-piece",
		Description: "Widget provided by an extension",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockExtWidget(ctx.CC, ctx.Entity, ctx.Block)
	})

	// Cover image block — full-width banner/hero image for entity pages.
	r.Register(BlockMeta{
		Type: "cover_image", Label: "Cover Image", Icon: "fa-panorama",
		Description: "Full-width banner image",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockCoverImage(ctx.CC, ctx.Entity, ctx.CSRFToken, ctx.Block.Config)
	})

	// Local graph block — mini-graph showing entity's neighborhood.
	r.Register(BlockMeta{
		Type: "local_graph", Label: "Local Graph", Icon: "fa-diagram-project",
		Description: "Entity relationship neighborhood",
	}, func(ctx BlockRenderContext) templ.Component {
		return blockLocalGraph(ctx.CC, ctx.Entity, ctx.Block.Config)
	})

	// Container layout types — rendered by the template editor JS, not by
	// server-side templ. Registered here so they pass validation.
	r.Register(BlockMeta{
		Type: "two_column", Label: "2 Columns", Icon: "fa-columns",
		Description: "Side-by-side columns", Container: true,
	}, nil)

	r.Register(BlockMeta{
		Type: "three_column", Label: "3 Columns", Icon: "fa-table-columns",
		Description: "Three equal columns", Container: true,
	}, nil)

	r.Register(BlockMeta{
		Type: "tabs", Label: "Tabs", Icon: "fa-folder",
		Description: "Tabbed content sections", Container: true,
	}, nil)

	r.Register(BlockMeta{
		Type: "section", Label: "Section", Icon: "fa-caret-down",
		Description: "Collapsible accordion", Container: true,
	}, nil)
}
