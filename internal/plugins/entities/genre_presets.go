// genre_presets.go defines entity type presets for different campaign genres.
// When a campaign is created with a genre selection, the corresponding preset
// types are seeded instead of (or in addition to) the standard defaults.
package entities

// GenrePreset defines a set of entity types for a campaign genre.
type GenrePreset struct {
	ID          string       // Genre identifier (e.g., "fantasy", "sci-fi").
	Name        string       // Display name (e.g., "Fantasy / D&D").
	Description string       // Short description for the UI.
	Icon        string       // FontAwesome icon.
	Types       []EntityType // Entity types to seed.
}

// GenrePresets returns all available genre presets.
func GenrePresets() []GenrePreset {
	return []GenrePreset{
		fantasyPreset(),
		sciFiPreset(),
		horrorPreset(),
		modernPreset(),
		historicalPreset(),
	}
}

// FindGenrePreset returns a genre preset by ID, or nil if not found.
func FindGenrePreset(id string) *GenrePreset {
	for _, p := range GenrePresets() {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

func fantasyPreset() GenrePreset {
	return GenrePreset{
		ID:          "fantasy",
		Name:        "Fantasy / D&D",
		Description: "Classic fantasy with characters, locations, factions, items, and lore.",
		Icon:        "fa-hat-wizard",
		Types: []EntityType{
			{Slug: "character", Name: "Character", NamePlural: "Characters", Icon: "fa-user", Color: "#3b82f6", SortOrder: 1, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "title", Label: "Title", Type: "text", Section: "Basics"},
					{Key: "race", Label: "Race", Type: "text", Section: "Basics"},
					{Key: "class", Label: "Class", Type: "text", Section: "Basics"},
					{Key: "level", Label: "Level", Type: "number", Section: "Basics"},
					{Key: "alignment", Label: "Alignment", Type: "text", Section: "Basics"},
				}},
			{Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#ef4444", SortOrder: 2, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "population", Label: "Population", Type: "text", Section: "Basics"},
					{Key: "region", Label: "Region", Type: "text", Section: "Basics"},
					{Key: "ruler", Label: "Ruler", Type: "text", Section: "Basics"},
				}},
			{Slug: "faction", Name: "Faction", NamePlural: "Factions", Icon: "fa-flag", Color: "#f59e0b", SortOrder: 3, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "leader", Label: "Leader", Type: "text", Section: "Basics"},
					{Key: "goals", Label: "Goals", Type: "text", Section: "Basics"},
					{Key: "alignment", Label: "Alignment", Type: "text", Section: "Basics"},
				}},
			{Slug: "item", Name: "Item", NamePlural: "Items", Icon: "fa-box", Color: "#8b5cf6", SortOrder: 4, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "rarity", Label: "Rarity", Type: "text", Section: "Basics"},
					{Key: "attunement", Label: "Requires Attunement", Type: "checkbox", Section: "Basics"},
				}},
			{Slug: "creature", Name: "Creature", NamePlural: "Creatures", Icon: "fa-dragon", Color: "#dc2626", SortOrder: 5, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "challenge_rating", Label: "Challenge Rating", Type: "text", Section: "Basics"},
					{Key: "habitat", Label: "Habitat", Type: "text", Section: "Basics"},
				}},
			{Slug: "quest", Name: "Quest", NamePlural: "Quests", Icon: "fa-scroll", Color: "#ec4899", SortOrder: 6, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "status", Label: "Status", Type: "select", Section: "Basics",
						Options: []string{"Active", "Completed", "Failed", "On Hold"}},
					{Key: "quest_giver", Label: "Quest Giver", Type: "text", Section: "Basics"},
					{Key: "reward", Label: "Reward", Type: "text", Section: "Basics"},
				}},
			{Slug: "lore", Name: "Lore", NamePlural: "Lore", Icon: "fa-book", Color: "#10b981", SortOrder: 7, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{}},
			{Slug: "shop", Name: "Shop", NamePlural: "Shops", Icon: "fa-store", Color: "#f97316", SortOrder: 8, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "shop_type", Label: "Shop Type", Type: "select", Section: "Basics",
						Options: []string{"General Store", "Blacksmith", "Apothecary", "Magic Shop", "Tavern", "Armorer", "Jeweler", "Tailor", "Stable", "Other"}},
					{Key: "shop_keeper", Label: "Shopkeeper", Type: "text", Section: "Basics"},
					{Key: "currency", Label: "Currency", Type: "text", Section: "Basics"},
				}},
		},
	}
}

func sciFiPreset() GenrePreset {
	return GenrePreset{
		ID:          "sci-fi",
		Name:        "Sci-Fi / Space Opera",
		Description: "Spaceships, planets, species, corporations, and technology.",
		Icon:        "fa-rocket",
		Types: []EntityType{
			{Slug: "character", Name: "Character", NamePlural: "Characters", Icon: "fa-user", Color: "#3b82f6", SortOrder: 1, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "species", Label: "Species", Type: "text", Section: "Basics"},
					{Key: "homeworld", Label: "Homeworld", Type: "text", Section: "Basics"},
					{Key: "role", Label: "Role", Type: "text", Section: "Basics"},
					{Key: "affiliation", Label: "Affiliation", Type: "text", Section: "Basics"},
				}},
			{Slug: "planet", Name: "Planet", NamePlural: "Planets", Icon: "fa-globe", Color: "#10b981", SortOrder: 2, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "select", Section: "Basics",
						Options: []string{"Terrestrial", "Gas Giant", "Ice World", "Desert", "Ocean", "Jungle", "Artificial", "Other"}},
					{Key: "population", Label: "Population", Type: "text", Section: "Basics"},
					{Key: "system", Label: "Star System", Type: "text", Section: "Basics"},
					{Key: "atmosphere", Label: "Atmosphere", Type: "text", Section: "Basics"},
				}},
			{Slug: "starship", Name: "Starship", NamePlural: "Starships", Icon: "fa-shuttle-space", Color: "#6366f1", SortOrder: 3, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "class", Label: "Ship Class", Type: "text", Section: "Basics"},
					{Key: "manufacturer", Label: "Manufacturer", Type: "text", Section: "Basics"},
					{Key: "crew_capacity", Label: "Crew Capacity", Type: "number", Section: "Basics"},
					{Key: "armament", Label: "Armament", Type: "text", Section: "Basics"},
				}},
			{Slug: "species", Name: "Species", NamePlural: "Species", Icon: "fa-dna", Color: "#ec4899", SortOrder: 4, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "homeworld", Label: "Homeworld", Type: "text", Section: "Basics"},
					{Key: "lifespan", Label: "Lifespan", Type: "text", Section: "Basics"},
					{Key: "traits", Label: "Traits", Type: "text", Section: "Basics"},
				}},
			{Slug: "corporation", Name: "Corporation", NamePlural: "Corporations", Icon: "fa-building", Color: "#f59e0b", SortOrder: 5, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "industry", Label: "Industry", Type: "text", Section: "Basics"},
					{Key: "ceo", Label: "CEO", Type: "text", Section: "Basics"},
					{Key: "headquarters", Label: "Headquarters", Type: "text", Section: "Basics"},
				}},
			{Slug: "technology", Name: "Technology", NamePlural: "Technology", Icon: "fa-microchip", Color: "#8b5cf6", SortOrder: 6, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "tech_level", Label: "Tech Level", Type: "text", Section: "Basics"},
					{Key: "availability", Label: "Availability", Type: "text", Section: "Basics"},
				}},
			{Slug: "mission", Name: "Mission", NamePlural: "Missions", Icon: "fa-crosshairs", Color: "#ef4444", SortOrder: 7, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "status", Label: "Status", Type: "select", Section: "Basics",
						Options: []string{"Briefed", "In Progress", "Completed", "Failed", "Aborted"}},
					{Key: "client", Label: "Client", Type: "text", Section: "Basics"},
					{Key: "reward", Label: "Reward", Type: "text", Section: "Basics"},
				}},
			{Slug: "lore", Name: "Lore", NamePlural: "Lore", Icon: "fa-book", Color: "#64748b", SortOrder: 8, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{}},
		},
	}
}

func horrorPreset() GenrePreset {
	return GenrePreset{
		ID:          "horror",
		Name:        "Horror / Investigation",
		Description: "Investigators, locations, clues, entities, and mysteries.",
		Icon:        "fa-ghost",
		Types: []EntityType{
			{Slug: "investigator", Name: "Investigator", NamePlural: "Investigators", Icon: "fa-user-secret", Color: "#3b82f6", SortOrder: 1, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "occupation", Label: "Occupation", Type: "text", Section: "Basics"},
					{Key: "sanity", Label: "Sanity", Type: "number", Section: "Basics"},
					{Key: "motivation", Label: "Motivation", Type: "text", Section: "Basics"},
				}},
			{Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#6b7280", SortOrder: 2, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "danger_level", Label: "Danger Level", Type: "select", Section: "Basics",
						Options: []string{"Safe", "Uneasy", "Dangerous", "Deadly", "Unknown"}},
					{Key: "last_visited", Label: "Last Visited", Type: "text", Section: "Basics"},
				}},
			{Slug: "clue", Name: "Clue", NamePlural: "Clues", Icon: "fa-magnifying-glass", Color: "#f59e0b", SortOrder: 3, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "found_at", Label: "Found At", Type: "text", Section: "Basics"},
					{Key: "significance", Label: "Significance", Type: "select", Section: "Basics",
						Options: []string{"Minor", "Important", "Critical", "Red Herring"}},
				}},
			{Slug: "entity", Name: "Entity", NamePlural: "Entities", Icon: "fa-skull", Color: "#dc2626", SortOrder: 4, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "threat_level", Label: "Threat Level", Type: "text", Section: "Basics"},
					{Key: "weakness", Label: "Weakness", Type: "text", Section: "Basics"},
				}},
			{Slug: "npc", Name: "NPC", NamePlural: "NPCs", Icon: "fa-user", Color: "#10b981", SortOrder: 5, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "occupation", Label: "Occupation", Type: "text", Section: "Basics"},
					{Key: "trustworthy", Label: "Trustworthy", Type: "select", Section: "Basics",
						Options: []string{"Yes", "No", "Unknown", "Suspicious"}},
					{Key: "connection", Label: "Connection to Mystery", Type: "text", Section: "Basics"},
				}},
			{Slug: "mystery", Name: "Mystery", NamePlural: "Mysteries", Icon: "fa-question", Color: "#8b5cf6", SortOrder: 6, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "status", Label: "Status", Type: "select", Section: "Basics",
						Options: []string{"Open", "Partially Solved", "Solved", "Cold Case"}},
				}},
			{Slug: "journal", Name: "Journal", NamePlural: "Journals", Icon: "fa-book", Color: "#64748b", SortOrder: 7, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{}},
		},
	}
}

func modernPreset() GenrePreset {
	return GenrePreset{
		ID:          "modern",
		Name:        "Modern / Urban",
		Description: "Contemporary setting with people, places, organizations, and cases.",
		Icon:        "fa-city",
		Types: []EntityType{
			{Slug: "character", Name: "Character", NamePlural: "Characters", Icon: "fa-user", Color: "#3b82f6", SortOrder: 1, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "occupation", Label: "Occupation", Type: "text", Section: "Basics"},
					{Key: "age", Label: "Age", Type: "number", Section: "Basics"},
					{Key: "affiliation", Label: "Affiliation", Type: "text", Section: "Basics"},
				}},
			{Slug: "location", Name: "Location", NamePlural: "Locations", Icon: "fa-map-pin", Color: "#ef4444", SortOrder: 2, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "address", Label: "Address", Type: "text", Section: "Basics"},
					{Key: "district", Label: "District / Area", Type: "text", Section: "Basics"},
				}},
			{Slug: "organization", Name: "Organization", NamePlural: "Organizations", Icon: "fa-building", Color: "#f59e0b", SortOrder: 3, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "leader", Label: "Leader", Type: "text", Section: "Basics"},
					{Key: "headquarters", Label: "Headquarters", Type: "text", Section: "Basics"},
				}},
			{Slug: "item", Name: "Item", NamePlural: "Items", Icon: "fa-box", Color: "#8b5cf6", SortOrder: 4, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "origin", Label: "Origin", Type: "text", Section: "Basics"},
				}},
			{Slug: "case", Name: "Case", NamePlural: "Cases", Icon: "fa-briefcase", Color: "#ec4899", SortOrder: 5, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "status", Label: "Status", Type: "select", Section: "Basics",
						Options: []string{"Open", "Active", "Closed", "Cold"}},
					{Key: "priority", Label: "Priority", Type: "select", Section: "Basics",
						Options: []string{"Low", "Medium", "High", "Critical"}},
				}},
			{Slug: "note", Name: "Note", NamePlural: "Notes", Icon: "fa-sticky-note", Color: "#10b981", SortOrder: 6, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{}},
		},
	}
}

func historicalPreset() GenrePreset {
	return GenrePreset{
		ID:          "historical",
		Name:        "Historical / Period",
		Description: "Historical figures, places, events, artifacts, and cultures.",
		Icon:        "fa-landmark",
		Types: []EntityType{
			{Slug: "figure", Name: "Figure", NamePlural: "Figures", Icon: "fa-user", Color: "#3b82f6", SortOrder: 1, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "title", Label: "Title / Rank", Type: "text", Section: "Basics"},
					{Key: "born", Label: "Born", Type: "text", Section: "Basics"},
					{Key: "died", Label: "Died", Type: "text", Section: "Basics"},
					{Key: "nationality", Label: "Nationality", Type: "text", Section: "Basics"},
				}},
			{Slug: "place", Name: "Place", NamePlural: "Places", Icon: "fa-map-pin", Color: "#ef4444", SortOrder: 2, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "type", Label: "Type", Type: "text", Section: "Basics"},
					{Key: "era", Label: "Era", Type: "text", Section: "Basics"},
					{Key: "region", Label: "Region", Type: "text", Section: "Basics"},
				}},
			{Slug: "event", Name: "Event", NamePlural: "Events", Icon: "fa-calendar", Color: "#ec4899", SortOrder: 3, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "date", Label: "Date", Type: "text", Section: "Basics"},
					{Key: "location", Label: "Location", Type: "text", Section: "Basics"},
					{Key: "significance", Label: "Significance", Type: "text", Section: "Basics"},
				}},
			{Slug: "artifact", Name: "Artifact", NamePlural: "Artifacts", Icon: "fa-gem", Color: "#8b5cf6", SortOrder: 4, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "era", Label: "Era", Type: "text", Section: "Basics"},
					{Key: "origin", Label: "Origin", Type: "text", Section: "Basics"},
					{Key: "current_location", Label: "Current Location", Type: "text", Section: "Basics"},
				}},
			{Slug: "culture", Name: "Culture", NamePlural: "Cultures", Icon: "fa-globe", Color: "#f59e0b", SortOrder: 5, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "region", Label: "Region", Type: "text", Section: "Basics"},
					{Key: "era", Label: "Era", Type: "text", Section: "Basics"},
					{Key: "language", Label: "Language", Type: "text", Section: "Basics"},
				}},
			{Slug: "dynasty", Name: "Dynasty", NamePlural: "Dynasties", Icon: "fa-crown", Color: "#f97316", SortOrder: 6, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{
					{Key: "founder", Label: "Founder", Type: "text", Section: "Basics"},
					{Key: "period", Label: "Period", Type: "text", Section: "Basics"},
					{Key: "capital", Label: "Capital", Type: "text", Section: "Basics"},
				}},
			{Slug: "note", Name: "Note", NamePlural: "Notes", Icon: "fa-book", Color: "#10b981", SortOrder: 7, IsDefault: true, Enabled: true,
				Fields: []FieldDefinition{}},
		},
	}
}
