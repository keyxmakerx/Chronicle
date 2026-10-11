package campaigns

// remapSidebarIDs points an imported sidebar at the new campaign's
// categories and pages. A category item or hidden page whose original did
// not come back is dropped: its id would otherwise name nothing, or a
// category in some other campaign. The import report already lists what
// failed to come back, so dropping it here adds no second entry.
func remapSidebarIDs(cfg *SidebarConfig, idMap *IDMap) {
	items := cfg.Items[:0]
	for _, it := range cfg.Items {
		if it.Type == SidebarTypeCategory {
			newID, ok := idMap.EntityTypeIDs[it.TypeID]
			if !ok {
				continue
			}
			it.TypeID = newID
		}
		items = append(items, it)
	}
	cfg.Items = items

	hidden := cfg.HiddenEntityIDs[:0]
	for _, id := range cfg.HiddenEntityIDs {
		if newID, ok := idMap.EntityIDs[id]; ok {
			hidden = append(hidden, newID)
		}
	}
	cfg.HiddenEntityIDs = hidden
}
