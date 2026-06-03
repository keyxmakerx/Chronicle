// calendar_mock.go — fantasy-calendar mock data for /demo/calendar.
//
// Shaped to mirror internal/plugins/calendar/model.go so the chosen
// design ports to the real plugin cleanly: same Calendar / Month /
// Weekday / Moon / Season / Festival / Event field set, just without
// the DB-only fields (CalendarID, CreatedAt, etc.).
//
// "Harptos-like" base: 12 months, intercalary days, 10-day "tenday"
// week, two moons, four seasons, three festivals, ~15 events spread
// across tiers, categories, multi-day spans, and time-of-day.

package demo

// CalAlmanacMockData is the full self-contained dataset for the
// Almanac showcase calendar. Single function so templ + JSON-embed for
// the JS share one source of truth.
type CalAlmanacMockData struct {
	Calendar  CalAlmanacCalendar  `json:"calendar"`
	Months    []CalAlmanacMonth   `json:"months"`
	Weekdays  []CalAlmanacWeekday `json:"weekdays"`
	Moons     []CalAlmanacMoon    `json:"moons"`
	Seasons   []CalAlmanacSeason  `json:"seasons"`
	Festivals []CalAlmanacFestival `json:"festivals"`
	Tiers     []CalAlmanacTier    `json:"tiers"`
	Categories []CalAlmanacCategory `json:"categories"`
	Events    []CalAlmanacEvent   `json:"events"`
	// CurrentMonth + Year are what the grid renders on initial load.
	CurrentYear  int `json:"current_year"`
	CurrentMonth int `json:"current_month"` // 1-indexed
	CurrentDay   int `json:"current_day"`
	// SkyTime is a 0..1 fraction of the day (0=midnight, 0.25=dawn,
	// 0.5=noon, 0.75=dusk). The templ reads it to position the
	// initial sun + render the initial gradient server-side, so the
	// page is meaningfully screenshottable before JS runs.
	SkyTime float64 `json:"sky_time"`
}

type CalAlmanacCalendar struct {
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	EpochName   string `json:"epoch_name"`
	HoursPerDay int    `json:"hours_per_day"`
}

type CalAlmanacMonth struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Days          int    `json:"days"`
	IsIntercalary bool   `json:"is_intercalary"`
}

type CalAlmanacWeekday struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Short     string `json:"short"`
	IsRestDay bool   `json:"is_rest_day"`
}

type CalAlmanacMoon struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"` // OKLCH literal
}

type CalAlmanacSeason struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Start int    `json:"start"` // month index, 1-indexed
	Color string `json:"color"`
}

type CalAlmanacFestival struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Month int    `json:"month"`
	Day   int    `json:"day"`
	Color string `json:"color"`
}

type CalAlmanacTier struct {
	ID    string `json:"id"` // "major"/"standard"/"detail"
	Name  string `json:"name"`
	Color string `json:"color"`
}

type CalAlmanacCategory struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type CalAlmanacEvent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Year        int    `json:"year"`
	Month       int    `json:"month"`
	Day         int    `json:"day"`
	EndMonth    int    `json:"end_month"` // 0 = single-day
	EndDay      int    `json:"end_day"`
	Hour        int    `json:"hour"`        // 0..23; -1 = all-day
	Tier        string `json:"tier"`
	Category    string `json:"category"`
	Visibility  string `json:"visibility"` // "public"/"specific"
	AllowUsers  []string `json:"allow_users,omitempty"`
	DenyUsers   []string `json:"deny_users,omitempty"`
}

// CalAlmanacMock returns the in-memory mock dataset. Pure function;
// no state, no DB. Repeating this in code (instead of embedding a
// JSON file) lets templ render directly from the same struct the JS
// gets via a JSON marshal — single source of truth.
func CalAlmanacMock() CalAlmanacMockData {
	return CalAlmanacMockData{
		Calendar: CalAlmanacCalendar{
			Name:        "Calendar of Harptos",
			Mode:        "fantasy",
			EpochName:   "DR",
			HoursPerDay: 24,
		},
		CurrentYear:  1492,
		CurrentMonth: 4, // Tarsakh — spring, festival-rich
		CurrentDay:   14,
		SkyTime:      0.52, // shortly past noon, sun high
		Months: []CalAlmanacMonth{
			{1, "Hammer", 30, false},
			{2, "Alturiak", 30, false},
			{3, "Ches", 30, false},
			{4, "Tarsakh", 30, false},
			{5, "Mirtul", 30, false},
			{6, "Kythorn", 30, false},
			{7, "Flamerule", 30, false},
			{8, "Eleasis", 30, false},
			{9, "Eleint", 30, false},
			{10, "Marpenoth", 30, false},
			{11, "Uktar", 30, false},
			{12, "Nightal", 30, false},
		},
		Weekdays: []CalAlmanacWeekday{
			{1, "First-day", "1st", false},
			{2, "Second-day", "2nd", false},
			{3, "Third-day", "3rd", false},
			{4, "Fourth-day", "4th", false},
			{5, "Fifth-day", "5th", false},
			{6, "Sixth-day", "6th", false},
			{7, "Seventh-day", "7th", false},
			{8, "Eighth-day", "8th", false},
			{9, "Ninth-day", "9th", false},
			{10, "Tenth-day", "10th", true}, // rest day
		},
		Moons: []CalAlmanacMoon{
			{1, "Selûne", 30.4, 0.0, "oklch(0.92 0.05 95)"},     // pale gold
			{2, "Shar", 28.0, 0.5, "oklch(0.42 0.06 280)"},      // dark indigo
		},
		Seasons: []CalAlmanacSeason{
			{1, "Winter", 12, "oklch(0.55 0.10 240)"},
			{2, "Spring", 3, "oklch(0.66 0.16 145)"},
			{3, "Summer", 6, "oklch(0.78 0.16 75)"},
			{4, "Autumn", 9, "oklch(0.62 0.18 50)"},
		},
		Festivals: []CalAlmanacFestival{
			{1, "Midwinter", 1, 30, "oklch(0.78 0.10 220)"},
			{2, "Greengrass", 4, 30, "oklch(0.70 0.18 135)"},
			{3, "Midsummer", 7, 30, "oklch(0.80 0.18 80)"},
		},
		Tiers: []CalAlmanacTier{
			{"major", "Major", "oklch(0.65 0.20 22)"},      // crimson (high contrast)
			{"standard", "Standard", "oklch(0.62 0.18 240)"}, // sky-blue
			{"detail", "Detail", "oklch(0.55 0.04 260)"},   // muted
		},
		Categories: []CalAlmanacCategory{
			{"session", "Session", "oklch(0.65 0.16 145)"},     // emerald
			{"festival", "Festival", "oklch(0.78 0.16 75)"},    // amber
			{"travel", "Travel", "oklch(0.68 0.14 200)"},       // teal
			{"npc", "NPC arc", "oklch(0.70 0.18 320)"},         // magenta
			{"world", "World event", "oklch(0.62 0.18 30)"},    // orange-red
			{"downtime", "Downtime", "oklch(0.65 0.06 260)"},   // muted blue
		},
		Events: []CalAlmanacEvent{
			// Major / world events
			{"e1", "The Burning of the Spire", "A celestial tower falls; tremors felt in every city. World turns on this day.",
				1492, 4, 7, 0, 0, -1, "major", "world", "public", nil, nil},
			{"e2", "Greengrass Festival", "Annual fertility festival across the realm. Mead, music, and renewals.",
				1492, 4, 30, 0, 0, -1, "major", "festival", "public", nil, nil},
			{"e3", "Caravan to Waterdeep", "Multi-day overland trek with the merchants of Daggerford.",
				1492, 4, 12, 4, 16, -1, "standard", "travel", "public", nil, nil},

			// Standard / session-y events
			{"e4", "Session 14: The Crypt Below", "Party descends to find the source of the tremors. Live 7pm.",
				1492, 4, 14, 0, 0, 19, "standard", "session", "public", nil, nil},
			{"e5", "Session 15: Audience with the Lord", "Court politics in the citadel; reveal of the Burning Spire's cause.",
				1492, 4, 21, 0, 0, 19, "standard", "session", "public", nil, nil},
			{"e6", "Session 16: The Long Road", "Travel + side hooks; light combat session.",
				1492, 4, 28, 0, 0, 19, "standard", "session", "public", nil, nil},

			// NPC arcs (some private)
			{"e7", "Marisha returns", "NPC re-appears in Daggerford with news from the north.",
				1492, 4, 9, 0, 0, 14, "standard", "npc", "specific", []string{"alice"}, nil},
			{"e8", "The Black Letter", "Sealed letter delivered to the party. DM eyes only — reveal timing TBD.",
				1492, 4, 18, 0, 0, 22, "detail", "npc", "specific", nil, []string{"bob", "carol"}},

			// Downtime / detail
			{"e9", "Crafting: Sun-blade", "Rolan finishes the inscription on his sun-blade.",
				1492, 4, 5, 0, 0, 10, "detail", "downtime", "public", nil, nil},
			{"e10", "Library research", "Aedric searches the temple library for references to the Spire.",
				1492, 4, 10, 0, 0, 14, "detail", "downtime", "public", nil, nil},
			{"e11", "Selûne full", "Lunar phase: Selûne is full. +1 to ritual rolls under moonlight.",
				1492, 4, 23, 0, 0, -1, "detail", "world", "public", nil, nil},
			{"e12", "Shar new", "Lunar phase: Shar is new. -1 to shadow-magic resists.",
				1492, 4, 1, 0, 0, -1, "detail", "world", "public", nil, nil},

			// Adjacent-month spill so the prior + next month preview cells show content
			{"e13", "Spring rains begin", "Weather shifts; travel difficulty +1 for a tenday.",
				1492, 3, 28, 4, 8, -1, "standard", "world", "public", nil, nil},
			{"e14", "Session 17: A Quiet Tenday", "Downtime session; player goals.",
				1492, 5, 5, 0, 0, 19, "standard", "session", "public", nil, nil},
			{"e15", "The Spire re-ignites", "Major world beat. Locks in only after the party returns from the crypt.",
				1492, 5, 12, 0, 0, -1, "major", "world", "public", nil, nil},
		},
	}
}
