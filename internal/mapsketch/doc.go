// Package mapsketch reads the line-by-line text a map is sketched in, for two
// consumers that must agree on the words: AI Import's map records (a new map
// on the maps plugin) and the page map block (a document stored on a page).
//
// It sits outside both plugins, like tilegrid, so neither imports the other
// and the grammar has one definition. It knows nothing of HTTP, SQL or the
// maps plugin's drawing types; callers turn the parsed lines into whatever
// they store.
package mapsketch
