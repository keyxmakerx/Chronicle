// Package vault_import turns a .zip of an Obsidian vault (or any folder of
// Markdown notes, including Notion and Trilium Markdown exports) into
// Chronicle pages.
//
// Everything in the zip is attacker input, so the package is built in layers
// that never trust the one before it: archive.go admits entries (paths, sizes,
// ratios), vault.go reads notes and resolves links without writing anything
// (that is the preview), and runner.go is the only code that touches the
// campaign, through the narrow interfaces in ports.go. The package never
// imports another plugin; internal/app adapts the pages, media and page-file
// services to those interfaces.
package vault_import
