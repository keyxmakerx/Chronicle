package entities

// Add-on slugs that gate core blocks. The entities plugin cannot import the
// owning plugins, so the slugs are declared here once, in a file the
// plugin-isolation guard reads as a const registry.
const (
	addonArmory = "armory"
)
