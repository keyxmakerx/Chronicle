package foundry_vtt

import "time"

// CampaignToken is one row of foundry_module_campaign_tokens — the
// per-campaign signing version counter that gates manifest URL
// signatures. Schema-compatible with foundry_modules.CampaignToken
// (intentional: both plugins read the same table during the C-FMC-5b
// parallel period; C-FMC-5c renames the table to
// foundry_vtt_campaign_tokens and deletes foundry_modules).
type CampaignToken struct {
	CampaignID   string    `json:"campaign_id"`
	TokenVersion int       `json:"token_version"`
	RotatedAt    time.Time `json:"rotated_at"`
}

// OwnerTabData is the renderable payload for the per-campaign owner
// settings tab. Rendered by owner_tab.templ; populated by the
// handler's OwnerTabFragmentHandler.
type OwnerTabData struct {
	// CampaignID is the current campaign — used as the form action
	// target and in the install URL the operator copies.
	CampaignID string

	// InstallURL is the full Foundry-installable manifest URL,
	// including the signed token. Empty string if not yet minted
	// (handler always mints lazily on first load, so empty here
	// would indicate a downstream error).
	InstallURL string

	// CurrentPin is the campaign's saved pin string ("" for
	// latest-tracking campaigns). The pin selector dropdown
	// preselects this value.
	CurrentPin string

	// CurrentVersion is the version that will actually be served
	// to Foundry right now (after pin → install dir resolution).
	// May differ from CurrentPin when pin is empty (latest-tracking).
	CurrentVersion string

	// AvailableVersions is the list of versions the packages plugin
	// has extracted on disk for the foundry-module package. The pin
	// dropdown shows these as options. Empty when no foundry-module
	// package is registered or no version is installed.
	AvailableVersions []string

	// PackageRegistered is false when no foundry-module typed package
	// exists in the packages plugin's catalog. The templ uses this
	// to render a "set up the foundry module first" empty state
	// instead of the install URL UI.
	PackageRegistered bool

	// DescriptorPresent records whether the installed module's zip
	// shipped a chronicle-package.json. Surfaced in the templ as a
	// transparency cue ("descriptor: present" vs "using defaults")
	// so the operator can see at a glance which path is active.
	DescriptorPresent bool

	// CSRFToken is the form token for the pin / rotate-token POSTs.
	CSRFToken string
}

// PackageDescriptor is the parsed shape of chronicle-package.json
// (schema v1 — see https://chronicle.bnuuy.haus/schemas/foundry-package.v1.json).
// The hook reads this from the extracted install dir; falls back to
// defaultDescriptor() when no file is present. Schema versioning is
// enforced in descriptor.go's loadDescriptor.
type PackageDescriptor struct {
	// SchemaVersion is the descriptor's contract version. v1 is the
	// only recognized value; an unknown major version fails the
	// install loudly per the C-FMC-5b agreement.
	SchemaVersion int `json:"schemaVersion"`

	// Package identifies what kind of Chronicle package this is and
	// where the canonical Foundry manifest lives inside the extracted
	// zip.
	Package PackageDescriptorPackage `json:"package"`

	// Serving controls how Chronicle rewrites the served manifest
	// (which fields to override, with what URL shapes). Default
	// values match the operator's locked URL shape from C-FMC-5-R1.
	Serving PackageDescriptorServing `json:"serving"`
}

// PackageDescriptorPackage is the descriptor's "package" block.
type PackageDescriptorPackage struct {
	// ID is the upstream module's stable identifier (e.g. "chronicle-sync").
	// Informational; Chronicle doesn't currently enforce a match
	// against the packages plugin's package row.
	ID string `json:"id"`

	// Kind identifies the package category. Must be "foundry-module"
	// for v1; the hook fires only for this kind anyway.
	Kind string `json:"kind"`

	// ModuleJSONPath is the path to the manifest INSIDE the
	// extracted install dir. Default: "module.json". Modules with
	// a nested layout (e.g. "dist/module.json") override this.
	ModuleJSONPath string `json:"moduleJsonPath"`
}

// PackageDescriptorServing is the descriptor's "serving" block —
// the part that controls what Chronicle does at serve-time.
type PackageDescriptorServing struct {
	// RewriteFields lists which top-level fields of module.json the
	// serve handler overrides on every fetch. Default: ["manifest",
	// "download"]. Future Foundry releases can add "asset_manifest"
	// or similar without Chronicle code changes.
	RewriteFields []string `json:"rewriteFields"`

	// ManifestEndpoint is the URL template the rewriter uses for
	// the "manifest" field. {campaign_id} and {token} are
	// substituted at serve time. Default matches the locked URL
	// shape.
	ManifestEndpoint string `json:"manifestEndpoint"`

	// DownloadEndpoint is the URL template for the "download"
	// field. Same substitution semantics as ManifestEndpoint.
	DownloadEndpoint string `json:"downloadEndpoint"`

	// PerCampaignSignedToken indicates the rewriter should append
	// a token query param. Default true; reserved for future
	// modules that want public (unsigned) manifests.
	PerCampaignSignedToken bool `json:"perCampaignSignedToken"`

	// ZipContentRoot is the path INSIDE the zip where the module's
	// content begins. Empty = zip root. Reserved for v1; not yet
	// consumed by the serve handler (packages plugin's extraction
	// is the single point that honors this in future versions).
	ZipContentRoot string `json:"zipContentRoot"`
}

// currentSchemaVersion is the descriptor schema major this build
// recognizes. Bumping requires a coordinated Foundry + Chronicle
// release; the hook fails install loudly on an unrecognized major.
const currentSchemaVersion = 1
