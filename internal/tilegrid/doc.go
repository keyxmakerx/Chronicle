// Package tilegrid is the one definition of a tile section, shared by every
// layout that draws one: entity page layouts, the campaign dashboard and
// category dashboards.
//
// A tile section is a layout row of kind "tiles". It holds an ordered list of
// widgets, each in a size its widget type declares ("WxH" on a 4-column grid),
// instead of the 12-unit columns of an ordinary row. The three layout models
// store the same Tile; this package owns the parts that must not drift
// between them: size parsing, the stored-size fallback, structural
// validation, and the grid markup.
package tilegrid
