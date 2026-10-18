// Package fieldrules holds two homebrew tools a campaign builds for itself in
// the system book's Homebrew tools: worked-out rules (a field's value follows
// another field through a table, such as a guild rank from echelon) and local
// words (what a place or an era calls a coin, a rank or any other word).
//
// The rules and words are the campaign's data, never a game system's, so they
// survive a package update and work under any system. Pages ask this plugin
// what a rule gives and which local words apply; deciding who sees a page is
// theirs. The service decides who may change a rule or a word and which
// words a viewer is shown; the handler adds no rule of its own.
package fieldrules
