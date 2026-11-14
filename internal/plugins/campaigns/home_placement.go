package campaigns

// HomeHasBlock reports whether a home layout places blockType in a column or
// a tile section. Home blocks that a visitor may see only where the owner
// chose to show them ask it of the Guests home, so placement is the consent:
// the fragment routes are reachable on any public campaign, and the layout is
// the only thing that says the owner wanted the block there.
func HomeHasBlock(l *DashboardLayout, blockType string) bool {
	if l == nil {
		return false
	}
	for _, r := range l.Rows {
		for _, col := range r.Columns {
			for _, b := range col.Blocks {
				if b.Type == blockType {
					return true
				}
			}
		}
		for _, t := range r.Tiles {
			if t.Type == blockType {
				return true
			}
		}
	}
	return false
}

// GuestHomeHasBlock reports whether the campaign's Guests home places
// blockType.
func GuestHomeHasBlock(c *Campaign, blockType string) bool {
	home, _ := c.HomeLayout(HomeGuest)
	return HomeHasBlock(home, blockType)
}
