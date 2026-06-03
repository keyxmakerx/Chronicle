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
	// Refinement (post-PR-#385) — operator-requested vocabularies.
	Eras         []CalAlmanacEra         `json:"eras"`           // colored bands behind the grid
	WeatherTypes []CalAlmanacWeatherType `json:"weather_types"`  // named weather vocabulary
	MoonPhases   []CalAlmanacMoonPhase   `json:"moon_phases"`    // named phase vocabulary (per-moon)
	DayWeather   map[string]string       `json:"day_weather"`    // "Y-M-D" -> weather-type ID
	DayNotes     map[string]string       `json:"day_notes"`      // "Y-M-D" -> free-text note
	Recurring    []CalAlmanacRecurring   `json:"recurring"`      // weekly/monthly templates
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
	// Icon — name of an inline SVG glyph rendered by the templ's
	// calAlmanacIcon helper (sword, mask, hearth, etc.). No external
	// font/icon dependency.
	Icon string `json:"icon"`
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
	// RecurringRef — if non-empty, this event was generated from the
	// recurring template with this ID. Lets the popover offer
	// "edit this instance only" vs "edit the series."
	RecurringRef string `json:"recurring_ref,omitempty"`
}

// CalAlmanacEra — a named historical span rendered as a colored band
// above the weekday header. Eras can stretch many years; the demo
// shows the current era as the active band + 2 adjacent for context.
type CalAlmanacEra struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	StartYear  int    `json:"start_year"`
	EndYear    int    `json:"end_year"`     // 0 = ongoing
	Color      string `json:"color"`        // OKLCH literal
	Description string `json:"description,omitempty"`
}

// CalAlmanacWeatherType — a named weather condition the operator
// authored once, then references on specific days. Matches Calendaria's
// "Select Weather" vocabulary (clear/cloudy/sakura bloom/arcane winds).
type CalAlmanacWeatherType struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"` // "standard" / "severe" / "environmental" / "fantasy"
	Icon     string `json:"icon"`     // inline SVG glyph name
	Color    string `json:"color"`    // OKLCH literal; tints the chip
	TempC    int    `json:"temp_c"`   // °C; informational
}

// CalAlmanacMoonPhase — a named span of a moon's phase cycle. Each
// moon owns a list of phases keyed by start_pct/end_pct (0..100),
// matching the Calendaria moon-phases editor. Operator can name a
// phase (e.g. "The Silver Crown") so it reads like worldbuilding,
// not a procedural percentage.
type CalAlmanacMoonPhase struct {
	MoonID   int    `json:"moon_id"`
	Name     string `json:"name"`
	StartPct int    `json:"start_pct"` // 0..100
	EndPct   int    `json:"end_pct"`
	Glyph    string `json:"glyph"`     // unicode moon glyph (rendered as fallback over the SVG)
}

// CalAlmanacRecurring — a recurring-event template. The mock has one
// example (weekly session) that the templ expands into per-week
// instances within the focused month. Real plugin would persist these
// + per-instance overrides.
type CalAlmanacRecurring struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	StartMonth    int    `json:"start_month"`
	StartDay      int    `json:"start_day"`
	IntervalDays  int    `json:"interval_days"` // e.g. 7 = weekly (on the tenday this is "every other week")
	Hour          int    `json:"hour"`
	Tier          string `json:"tier"`
	Category      string `json:"category"`
	// Overrides — per-instance edits. Key is "Y-M-D"; value is the
	// replacement name (showcase scope — production would override
	// any field).
	Overrides map[string]string `json:"overrides,omitempty"`
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
			{"session", "Session", "oklch(0.65 0.16 145)", "dice"},      // emerald — d20 die
			{"festival", "Festival", "oklch(0.78 0.16 75)", "flame"},   // amber — bonfire
			{"travel", "Travel", "oklch(0.68 0.14 200)", "compass"},    // teal — wayfinder
			{"npc", "NPC arc", "oklch(0.70 0.18 320)", "mask"},         // magenta — face/mask
			{"world", "World event", "oklch(0.62 0.18 30)", "spire"},   // orange-red — tower
			{"downtime", "Downtime", "oklch(0.65 0.06 260)", "hearth"}, // muted blue — fireside
		},
		Eras: []CalAlmanacEra{
			{"era-first", "First Age", -3000, 0, "oklch(0.50 0.10 240)", "Ages-long dawn of the realm."},
			{"era-kings", "Age of Kings", 1, 1487, "oklch(0.62 0.16 75)", "The lineage of mortal kings."},
			{"era-reckoning", "Reckoning", 1488, 0, "oklch(0.62 0.18 22)", "Current era. Began with the falling Spire."},
		},
		WeatherTypes: []CalAlmanacWeatherType{
			// Standard
			{"w-clear", "Clear", "standard", "sun", "oklch(0.85 0.14 80)", 18},
			{"w-cloudy", "Cloudy", "standard", "cloud", "oklch(0.74 0.02 240)", 14},
			{"w-rain", "Rain", "standard", "rain", "oklch(0.62 0.12 240)", 11},
			{"w-fog", "Fog", "standard", "fog", "oklch(0.70 0.02 240)", 9},
			// Severe
			{"w-storm", "Thunderstorm", "severe", "storm", "oklch(0.52 0.20 285)", 8},
			{"w-blizzard", "Blizzard", "severe", "snowflake", "oklch(0.85 0.04 240)", -12},
			// Environmental — operator's authored vocabulary, not procedural.
			{"w-sakura", "Sakura Bloom", "environmental", "petal", "oklch(0.80 0.12 350)", 16},
			{"w-ashfall", "Ashfall", "environmental", "ember", "oklch(0.60 0.04 30)", 4},
			// Fantasy
			{"w-arcane", "Arcane Winds", "fantasy", "swirl", "oklch(0.72 0.22 290)", -2},
			{"w-leysurge", "Ley Surge", "fantasy", "swirl", "oklch(0.65 0.20 195)", 10},
			{"w-acidrain", "Acid Rain", "fantasy", "rain", "oklch(0.70 0.18 145)", 8},
		},
		MoonPhases: []CalAlmanacMoonPhase{
			// Selûne (moon 1) — operator's naming convention from the
			// Calendaria mockups. 8 phases, each 12.5% of the 30-day cycle.
			{1, "The Dark Sister", 0, 12, "●"},
			{1, "The Growing — early", 12, 25, "◐"},
			{1, "The Growing — middle", 25, 37, "◐"},
			{1, "The Growing — late", 37, 50, "◐"},
			{1, "The Silver Crown", 50, 62, "○"},
			{1, "The Fading — early", 62, 75, "◑"},
			{1, "The Fading — middle", 75, 87, "◑"},
			{1, "The Fading — late", 87, 100, "◑"},
			// Shar (moon 2) — fewer named phases; mostly procedural.
			{2, "Shar — hidden", 0, 25, "●"},
			{2, "Shar — quarter", 25, 75, "◑"},
			{2, "Shar — full dark", 75, 100, "●"},
		},
		DayWeather: map[string]string{
			"1492-4-1":  "w-arcane",   // operator's Calendaria reference
			"1492-4-2":  "w-cloudy",
			"1492-4-3":  "w-cloudy",
			"1492-4-4":  "w-rain",
			"1492-4-5":  "w-clear",
			"1492-4-6":  "w-clear",
			"1492-4-7":  "w-storm",    // The Spire falls — storm
			"1492-4-8":  "w-fog",
			"1492-4-9":  "w-fog",
			"1492-4-10": "w-clear",
			"1492-4-11": "w-clear",
			"1492-4-12": "w-rain",
			"1492-4-13": "w-rain",
			"1492-4-14": "w-clear",    // today
			"1492-4-15": "w-clear",
			"1492-4-17": "w-leysurge", // fantasy weather highlight
			"1492-4-18": "w-cloudy",
			"1492-4-22": "w-acidrain",
			"1492-4-23": "w-clear",    // Selûne full
			"1492-4-25": "w-sakura",   // environmental highlight
			"1492-4-26": "w-sakura",
			"1492-4-30": "w-clear",    // Greengrass
		},
		DayNotes: map[string]string{
			"1492-4-7":  "World-breaking day. Note the players' reactions; tremors echo for the rest of the tenday.",
			"1492-4-14": "Today. Session 14 prep: reveal the lich in the crypt only after the third combat round, NOT on entry.",
			"1492-4-17": "Ley Surge — Rolan's sun-blade reacts. He gets a free attune-shift this day.",
			"1492-4-23": "Selûne full. Marisha asks for a ritual — DM keep an eye on which player rolls best Insight.",
		},
		Recurring: []CalAlmanacRecurring{
			{
				ID: "rec-session",
				Name: "Weekly Session",
				Description: "Recurring D&D session — Seventh-day evenings at the Sigil & Lantern.",
				StartMonth: 4, StartDay: 7,
				IntervalDays: 7,
				Hour: 19,
				Tier: "standard",
				Category: "session",
				Overrides: map[string]string{
					"1492-4-14": "Session 14: The Crypt Below",
					"1492-4-21": "Session 15: Audience with the Lord",
					"1492-4-28": "Session 16: The Long Road",
				},
			},
		},
		Events: []CalAlmanacEvent{
			// Major / world events
			{"e1", "The Burning of the Spire", "A celestial tower falls; tremors felt in every city. World turns on this day.",
				1492, 4, 7, 0, 0, -1, "major", "world", "public", nil, nil, ""},
			{"e2", "Greengrass Festival", "Annual fertility festival across the realm. Mead, music, and renewals.",
				1492, 4, 30, 0, 0, -1, "major", "festival", "public", nil, nil, ""},
			{"e3", "Caravan to Waterdeep", "Multi-day overland trek with the merchants of Daggerford.",
				1492, 4, 12, 4, 16, -1, "standard", "travel", "public", nil, nil, ""},

			// (e4/e5/e6 sessions removed — the Recurring template
			// "rec-session" above now generates them via overrides so
			// they demonstrate the recurring + per-instance pattern.)

			// NPC arcs (some private)
			{"e7", "Marisha returns", "NPC re-appears in Daggerford with news from the north.",
				1492, 4, 9, 0, 0, 14, "standard", "npc", "specific", []string{"alice"}, nil, ""},
			{"e8", "The Black Letter", "Sealed letter delivered to the party. DM eyes only — reveal timing TBD.",
				1492, 4, 18, 0, 0, 22, "detail", "npc", "specific", nil, []string{"bob", "carol"}, ""},

			// Downtime / detail
			{"e9", "Crafting: Sun-blade", "Rolan finishes the inscription on his sun-blade.",
				1492, 4, 5, 0, 0, 10, "detail", "downtime", "public", nil, nil, ""},
			{"e10", "Library research", "Aedric searches the temple library for references to the Spire.",
				1492, 4, 10, 0, 0, 14, "detail", "downtime", "public", nil, nil, ""},
			{"e11", "Selûne full", "Lunar phase: Selûne is full. +1 to ritual rolls under moonlight.",
				1492, 4, 23, 0, 0, -1, "detail", "world", "public", nil, nil, ""},
			{"e12", "Shar new", "Lunar phase: Shar is new. -1 to shadow-magic resists.",
				1492, 4, 1, 0, 0, -1, "detail", "world", "public", nil, nil, ""},

			// Adjacent-month spill so the prior + next month preview cells show content
			{"e13", "Spring rains begin", "Weather shifts; travel difficulty +1 for a tenday.",
				1492, 3, 28, 4, 8, -1, "standard", "world", "public", nil, nil, ""},
			{"e14", "Session 17: A Quiet Tenday", "Downtime session; player goals.",
				1492, 5, 5, 0, 0, 19, "standard", "session", "public", nil, nil, "rec-session"},
			{"e15", "The Spire re-ignites", "Major world beat. Locks in only after the party returns from the crypt.",
				1492, 5, 12, 0, 0, -1, "major", "world", "public", nil, nil, ""},
		},
	}
}
