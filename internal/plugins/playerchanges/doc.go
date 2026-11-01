// Package playerchanges is "What players changed": a record of every change a
// player makes to a page (a sheet field, a page's title or text, a removed
// page), by hand or through their AI, and the owner's page for searching it
// and undoing a change.
//
// The last month lives one row per change and answers at once. Older months
// are packed into one compressed blob per campaign and month and kept as
// long as the site's Long-term trash setting said when they were packed; a
// search that reaches back unpacks them. Recording never fails the write it
// describes: errors are logged and the write stands.
package playerchanges
