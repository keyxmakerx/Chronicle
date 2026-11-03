// Package transformations keeps layers on a hero: a vampire's bite, rising
// as a revenant, a curse, a polymorph. A layer names the sheet fields it
// changes and leaves the hero's own fields alone, so taking it off puts the
// hero back exactly as they were.
//
// This package stores and serves layers; it knows no game's rules. What a
// change means on a sheet (a damage weakness, a trait) is drawn by the game
// system's sheet, and ready-made layers come from the system's data. Only
// the Director puts a layer on or takes one off; anyone who can see the
// page sees the layers on it.
package transformations
