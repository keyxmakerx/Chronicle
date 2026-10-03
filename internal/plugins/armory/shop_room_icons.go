package armory

// shopRoomIcons is the set of Font Awesome icon names a shop room item may
// carry. static/js/widgets/shop_room_icons.js must list exactly the same
// names: the widget only offers these, and the server rejects the rest so a
// forged layout cannot smuggle an arbitrary class name into the page.
var shopRoomIcons = []string{
	"hammer", "shield-halved", "vest", "screwdriver-wrench", "link", "fire",
	"oil-can", "boxes-stacked", "box", "flask", "vial", "kit-medical", "leaf",
	"seedling", "mortar-pestle", "jar", "spa", "bandage", "wheat-awn",
	"jar-wheat", "bottle-droplet", "ring", "gem", "crown", "eye", "binoculars",
	"key", "mask", "compass", "scroll", "handcuffs", "skull", "spider",
	"wand-sparkles", "book", "eye-slash", "bag-shopping", "hourglass",
	"book-open", "star", "moon", "feather", "lightbulb", "bread-slice", "tent",
	"cheese", "shirt", "fish", "bucket", "apple-whole", "broom",
	"scale-balanced", "bell", "sword", "dagger", "axe", "bow",
}
