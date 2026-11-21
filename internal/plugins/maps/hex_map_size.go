package maps

import "math"

// HexMapWidth is the width, in map pixels, of a blank hex map made for AI
// Import. It matches the blank grid maps made there, so a grid size means the
// same number of pixels on both.
const HexMapWidth = 1000

// HexMapSize chooses the picture size and grid size of a blank map whose own
// hex field holds at least cols by rows hexes, with as little extra as the
// whole-number grid size allows.
//
// The field's column count depends only on the grid size (the hex width is a
// fixed share of the map width), so the grid size is the largest one that still
// gives cols columns, and the height is then the shortest that gives rows rows.
// Both are read back through NewHexGeometry rather than re-derived, so the two
// cannot drift apart. The grid size stays within the range the map settings
// accept; when even the smallest cannot give cols columns (more than the
// viewer's limit) the smallest is used.
func HexMapSize(cols, rows int) (width, height, gridSize int) {
	width = HexMapWidth
	gridSize = gridSizeMin
	for g := gridSizeMax; g >= gridSizeMin; g-- {
		if NewHexGeometry(float64(g), HexMapWidth, 1).Cols >= cols {
			gridSize = g
			break
		}
	}
	r := float64(gridSize) * HexMapWidth / 1000 / 2
	// Rows is ceil((height+r)/(1.5r)), so it reaches rows once height passes
	// 1.5r(rows-1)-r; start just there and let the geometry settle the float
	// edge in either direction.
	height = max(1, int(math.Floor(1.5*r*float64(rows-1)-r))+1)
	for NewHexGeometry(float64(gridSize), HexMapWidth, float64(height)).Rows < rows {
		height++
	}
	for height > 1 && NewHexGeometry(float64(gridSize), HexMapWidth, float64(height-1)).Rows >= rows {
		height--
	}
	return width, height, gridSize
}
