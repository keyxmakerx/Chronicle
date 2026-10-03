package syncapi

// The calendar's history kind spells the calendar plugin's slug. It is a
// label on the sync history wire (the Foundry module sends the same word),
// not a reference to that plugin, so it is declared once here, a file the
// plugin-isolation guard reads as a const registry.
const (
	historyKindCalendar = "calendar"
)
