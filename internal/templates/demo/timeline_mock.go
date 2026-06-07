// timeline_mock.go — mock dataset for the /demo/timeline/tuner showcase.
//
// C-TIMELINE-V2-DESIGN-1-TUNER. The "FM Tuner" timeline design: a
// horizontally-centered etched-metal time axis with adaptive tick
// notches, swim-lanes above and below, era gradient bands behind
// everything, hover-revealed connection arcs, and an atmospheric
// backdrop that paints the cursor day's weather/celestial state.
//
// Mock-driven, mirroring the Almanac's mock discipline (calendar_mock.go):
// the dataset is BOTH the single source of truth the templ embeds as JSON
// AND the shape the JS init blocks read. No backend changes — the real
// port re-skins the production D3 widget post-design-selection.
//
// §A1 ("one dataset, multiple views") is demonstrated by reusing the
// Almanac mock's eras / tiers / categories vocabulary here, so the same
// worldbuilding reads as a calendar grid OR a timeline depending on the
// lens.
//
// Cross-refs:
//   - dispatches/chronicle/C-TIMELINE-V2-DESIGN-1-TUNER.md (this dispatch)
//   - decisions/2026-05-28-cal-timeline-v2-design.md (§A1/A2/A3/B2/C1)
//   - decisions/2026-06-05-rendering-canvas-css-exemption.md (binding CSS rule)

package demo

// TimelineMockData is the self-contained dataset for the Tuner timeline
// showcase. Single function (CalTimelineTunerMock) so templ + the JSON
// embed for the JS share one source of truth.
type TimelineMockData struct {
	Calendar   CalAlmanacCalendar   `json:"calendar"`
	Eras       []CalAlmanacEra      `json:"eras"`
	Tiers      []CalAlmanacTier     `json:"tiers"`
	Categories []CalAlmanacCategory `json:"categories"`
	// Entities — the actors a swim-lane can group by; each carries a
	// color so connection arcs can be entity-color-coded (§F).
	Entities []TimelineEntity `json:"entities"`
	// Events — standalone timeline entries positioned by start date.
	Events []TimelineEvent `json:"events"`
	// Connections — directed relationships between events (§F). Rendered
	// as hover-revealed curved arcs, entity- or type-color-coded.
	Connections []TimelineConnection `json:"connections"`
	// SpecialMoonDays — "Y-M-D" keys where the full sky-band backdrop
	// (sun + moons) renders, per the operator's restraint rule (§J2):
	// sun/moons are noise on every day, so they ONLY render here
	// (eclipses, supermoons, blood moons).
	SpecialMoonDays []string `json:"special_moon_days"`
	// DayWeather — "Y-M-D" -> weather-effect id; drives the axis glyphs
	// and the atmospheric backdrop (weather always renders if present).
	DayWeather map[string]string `json:"day_weather"`
	// CelestialEvents — "Y-M-D" -> celestial events; non-routine ones
	// (meteor/eclipse) render as axis glyphs + backdrop.
	CelestialEvents map[string][]CalAlmanacCelestial `json:"celestial_events"`
	// WeatherTypes — named-weather vocabulary (shared with Almanac), for
	// glyph labels + tooltips.
	WeatherTypes []CalAlmanacWeatherType `json:"weather_types"`
	// Current campaign date — the cursor needle's "today" home (Home key).
	CurrentYear  int `json:"current_year"`
	CurrentMonth int `json:"current_month"`
	CurrentDay   int `json:"current_day"`
	// Calendar geometry the JS uses to convert dates → an absolute day
	// index for axis positioning (mock approximates uniform months).
	DaysPerMonth  int `json:"days_per_month"`
	MonthsPerYear int `json:"months_per_year"`
}

// TimelineEntity — an actor that events involve; the entity-grouping
// swim-lane mode (§D) makes one lane per entity, and connection arcs
// can be tinted by the shared entity's color (§F).
type TimelineEntity struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`  // "npc" / "location" / "item" / "faction"
	Color string `json:"color"` // OKLCH literal
}

// TimelineEvent — a single timeline entry. Multi-day events set the
// End* fields (0 = single-day). Tier governs card size (§E); Category
// governs the icon + accent. Entities lists the involved entity ids.
type TimelineEvent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Year        int      `json:"year"`
	Month       int      `json:"month"`
	Day         int      `json:"day"`
	EndYear     int      `json:"end_year"`
	EndMonth    int      `json:"end_month"`
	EndDay      int      `json:"end_day"`
	Tier        string   `json:"tier"`
	Category    string   `json:"category"`
	Entities    []string `json:"entities"`
}

// TimelineConnection — a directed relationship between two events. Type
// governs stroke style (solid-arrow / dashed / dotted / solid); EntityID
// (optional) names the shared entity whose color tints the arc in
// entity-color mode (§F).
type TimelineConnection struct {
	Source   string `json:"source"`   // event id
	Target   string `json:"target"`   // event id
	Type     string `json:"type"`     // "caused" / "related" / "mentioned" / "co-occurs"
	Label    string `json:"label"`    // human-readable relationship
	EntityID string `json:"entity_id,omitempty"`
}

// CalTimelineTunerMock returns the full Tuner showcase dataset. The data
// spans ~3000 years so every zoom level (millennia → days) has something
// to render, with a dense cluster around the current campaign year (1492
// DR) for the default/zoomed-in views.
func CalTimelineTunerMock() TimelineMockData {
	return TimelineMockData{
		Calendar: CalAlmanacCalendar{
			Name:        "Calendar of Harptos",
			Mode:        "fantasy",
			EpochName:   "DR",
			HoursPerDay: 24,
		},
		CurrentYear:   1492,
		CurrentMonth:  4, // Tarsakh
		CurrentDay:    14,
		DaysPerMonth:  30,
		MonthsPerYear: 12,
		// Eras reuse the Almanac vocabulary (§A1) but stretch the ranges so
		// zoom-out testing has gradient bands across the full canvas.
		Eras: []CalAlmanacEra{
			{"era-first", "First Age", -3000, -1, "oklch(0.50 0.10 240)", "Ages-long dawn of the realm, before mortal record."},
			{"era-kings", "Age of Kings", 0, 1486, "oklch(0.62 0.16 75)", "The long lineage of mortal kings."},
			{"era-reckoning", "Reckoning", 1487, 0, "oklch(0.62 0.18 22)", "Current era. Began with the falling Spire."},
		},
		Tiers: []CalAlmanacTier{
			{"major", "Major", "oklch(0.65 0.20 22)"},
			{"standard", "Standard", "oklch(0.62 0.18 240)"},
			{"detail", "Detail", "oklch(0.55 0.04 260)"},
		},
		Categories: []CalAlmanacCategory{
			{"battle", "Battle", "oklch(0.62 0.20 25)", "sword"},
			{"diplomacy", "Diplomacy", "oklch(0.68 0.14 200)", "compass"},
			{"discovery", "Discovery", "oklch(0.72 0.16 145)", "spark"},
			{"founding", "Founding", "oklch(0.78 0.16 75)", "spire"},
			{"arc", "Character arc", "oklch(0.70 0.18 320)", "mask"},
		},
		Entities: []TimelineEntity{
			{"ent-aragorn", "Aragorn", "npc", "oklch(0.66 0.17 145)"},
			{"ent-frodo", "Frodo", "npc", "oklch(0.70 0.16 250)"},
			{"ent-ring", "The Ring", "item", "oklch(0.72 0.19 50)"},
			{"ent-saruman", "Saruman", "npc", "oklch(0.62 0.18 25)"},
			{"ent-galadriel", "Galadriel", "npc", "oklch(0.78 0.10 200)"},
			{"ent-gondor", "Gondor", "faction", "oklch(0.70 0.14 90)"},
			{"ent-moria", "Moria", "location", "oklch(0.55 0.05 280)"},
		},
		// Events: a few deep-history anchors for zoom-out, then a dense
		// cluster across 1480-1495 for the default + zoomed-in views.
		Events: []TimelineEvent{
			{ID: "ev-dawnforge", Name: "The Dawnforge", Description: "The first city is raised from raw stone.", Year: -2400, Month: 1, Day: 1, Tier: "major", Category: "founding", Entities: []string{"ent-gondor"}},
			{ID: "ev-firstking", Name: "Crowning of the First King", Description: "The Age of Kings begins.", Year: 12, Month: 6, Day: 3, Tier: "major", Category: "founding", Entities: []string{"ent-gondor"}},
			{ID: "ev-moria-delving", Name: "Delving of Moria", Description: "The deep halls are opened.", Year: 740, Month: 9, Day: 12, Tier: "standard", Category: "discovery", Entities: []string{"ent-moria"}},
			{ID: "ev-spirefall", Name: "The Falling Spire", Description: "The cataclysm that opened the Reckoning.", Year: 1486, Month: 12, Day: 30, Tier: "major", Category: "battle", Entities: []string{"ent-saruman", "ent-gondor"}},
			{ID: "ev-ring-found", Name: "The Ring Resurfaces", Description: "An old evil is rediscovered.", Year: 1489, Month: 3, Day: 8, Tier: "major", Category: "discovery", Entities: []string{"ent-frodo", "ent-ring"}},
			{ID: "ev-council", Name: "Council of the Wise", Description: "The fellowship is decided.", Year: 1490, Month: 7, Day: 21, Tier: "standard", Category: "diplomacy", Entities: []string{"ent-aragorn", "ent-frodo", "ent-galadriel", "ent-ring"}},
			{ID: "ev-moria-fall", Name: "Shadow in Moria", Description: "The fellowship passes the deep dark.", Year: 1491, Month: 1, Day: 15, EndYear: 1491, EndMonth: 1, EndDay: 18, Tier: "standard", Category: "battle", Entities: []string{"ent-aragorn", "ent-frodo", "ent-moria"}},
			{ID: "ev-lorien", Name: "Sojourn in the Golden Wood", Description: "Rest and counsel from Galadriel.", Year: 1491, Month: 2, Day: 2, EndYear: 1491, EndMonth: 2, EndDay: 20, Tier: "detail", Category: "diplomacy", Entities: []string{"ent-galadriel", "ent-frodo"}},
			{ID: "ev-saruman-rise", Name: "Saruman's Betrayal", Description: "The white wizard turns.", Year: 1491, Month: 4, Day: 30, Tier: "major", Category: "arc", Entities: []string{"ent-saruman"}},
			{ID: "ev-gondor-muster", Name: "Muster of Gondor", Description: "The realm calls its banners.", Year: 1492, Month: 2, Day: 10, EndYear: 1492, EndMonth: 2, EndDay: 28, Tier: "standard", Category: "diplomacy", Entities: []string{"ent-aragorn", "ent-gondor"}},
			{ID: "ev-tarsakh-skirmish", Name: "Skirmish at the Ford", Description: "First blood of the spring campaign.", Year: 1492, Month: 4, Day: 12, Tier: "detail", Category: "battle", Entities: []string{"ent-aragorn"}},
			{ID: "ev-coronation", Name: "The Return of the King", Description: "Aragorn is crowned.", Year: 1492, Month: 4, Day: 14, Tier: "major", Category: "founding", Entities: []string{"ent-aragorn", "ent-gondor"}},
			{ID: "ev-ring-end", Name: "The Ring Unmade", Description: "The long shadow ends.", Year: 1492, Month: 4, Day: 14, Tier: "major", Category: "arc", Entities: []string{"ent-frodo", "ent-ring"}},
			{ID: "ev-rebuild", Name: "Rebuilding Begins", Description: "The realm turns to peace.", Year: 1492, Month: 6, Day: 1, Tier: "detail", Category: "founding", Entities: []string{"ent-gondor"}},
		},
		Connections: []TimelineConnection{
			{Source: "ev-spirefall", Target: "ev-ring-found", Type: "caused", Label: "loosed the old evil", EntityID: "ent-ring"},
			{Source: "ev-ring-found", Target: "ev-council", Type: "caused", Label: "prompted the council", EntityID: "ent-ring"},
			{Source: "ev-council", Target: "ev-moria-fall", Type: "caused", Label: "set the road", EntityID: "ent-frodo"},
			{Source: "ev-moria-fall", Target: "ev-lorien", Type: "co-occurs", Label: "fellowship's path", EntityID: "ent-frodo"},
			{Source: "ev-lorien", Target: "ev-coronation", Type: "related", Label: "Galadriel's counsel", EntityID: "ent-galadriel"},
			{Source: "ev-saruman-rise", Target: "ev-tarsakh-skirmish", Type: "caused", Label: "stirred the borders", EntityID: "ent-saruman"},
			{Source: "ev-gondor-muster", Target: "ev-coronation", Type: "caused", Label: "secured the throne", EntityID: "ent-gondor"},
			{Source: "ev-coronation", Target: "ev-ring-end", Type: "co-occurs", Label: "the same dawn", EntityID: "ent-aragorn"},
			{Source: "ev-ring-end", Target: "ev-rebuild", Type: "caused", Label: "made peace possible", EntityID: "ent-gondor"},
			{Source: "ev-firstking", Target: "ev-spirefall", Type: "mentioned", Label: "the long lineage ends"},
		},
		// Restraint demo: blood-sun coincidence + the coronation dawn.
		SpecialMoonDays: []string{"1492-4-14", "1491-4-30"},
		DayWeather: map[string]string{
			"1492-4-12": "rain",
			"1492-4-13": "thunderstorm",
			"1491-1-15": "snow",
			"1491-1-16": "snow",
			"1492-2-10": "fog",
			"1492-6-1":  "cloudy",
		},
		CelestialEvents: map[string][]CalAlmanacCelestial{
			"1492-4-14": {{Type: "eclipse-solar", Name: "The Crowning Eclipse", StartTime: 11, Duration: 2}},
			"1489-3-8":  {{Type: "meteor-shower", Name: "Falling Stars of Ches", StartTime: -1, Duration: 6}},
			"1491-4-30": {{Type: "eclipse-lunar", Name: "Blood of Selûne", StartTime: 22, Duration: 3}},
		},
		WeatherTypes: []CalAlmanacWeatherType{
			{"w-clear", "Clear", "standard", "sun", "oklch(0.85 0.14 80)", 18},
			{"w-cloudy", "Cloudy", "standard", "cloud", "oklch(0.74 0.02 240)", 14},
			{"w-rain", "Rain", "standard", "rain", "oklch(0.62 0.12 240)", 11},
			{"w-fog", "Fog", "standard", "fog", "oklch(0.70 0.02 240)", 9},
			{"w-storm", "Thunderstorm", "severe", "storm", "oklch(0.52 0.20 285)", 8},
			{"w-snow", "Snow", "severe", "snowflake", "oklch(0.85 0.04 240)", -4},
		},
	}
}
