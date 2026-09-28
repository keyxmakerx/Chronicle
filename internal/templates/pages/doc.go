// The one hand-written Go file in a package of .templ sources: without it,
// a checkout that has not run `templ generate` (Dependabot's) sees no Go
// files here, and `go mod tidy` fails looking for the package as a module.
// tools/check-templ-packages.sh keeps every such package carrying one.

package pages
