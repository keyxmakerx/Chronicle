// Package calendar - place_weather.go gives each place with a climate of its
// own its own weather, and keeps where the party is.
//
// A place's climate is a field on its page, owned by the entities plugin; a
// page with none takes its nearest parent's, and a page with no climate
// above it keeps the calendar's world climate. The page that holds the
// climate is the place's region, and weather is stored per region
// (calendar_place_weather_days). The calendar's own day weather stays the
// weather everywhere else.
//
// The party's marks say where the party is from a day on. A player sees one
// weather a day: the weather of the region the party was in that day, never
// another place's, and only for today and earlier, as before. A region with
// a climate but no reading that day shows nothing rather than the world's
// weather, which would be the wrong climate.
package calendar

import (
	"context"
	"fmt"
	"sort"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// PlaceDirectory is the campaign's places as the calendar reads them. It
// belongs to the entities plugin and is wired in internal/app; reads are
// unfiltered, so every answer that reaches a viewer goes through Viewable.
type PlaceDirectory interface {
	// ClimatePlaces is every page of the campaign with a climate of its own.
	ClimatePlaces(ctx context.Context, campaignID string) ([]ClimatePlace, error)
	// RegionOf is the page whose climate covers entityID: the page itself or
	// its nearest parent with a climate. Nil when none does, or the page is
	// not in the campaign.
	RegionOf(ctx context.Context, campaignID, entityID string) (*ClimatePlace, error)
	// Viewable is the subset of ids the viewer may open.
	Viewable(ctx context.Context, campaignID string, ids []string, v permissions.Viewer) map[string]bool
	// PlaceName is a page's name when it is a live page of the campaign.
	PlaceName(ctx context.Context, campaignID, entityID string) (string, bool)
}

// ClimatePlace is a page with a climate of its own.
type ClimatePlace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Climate     string `json:"climate"`
	ClimateName string `json:"climate_name"`
	ParentID    string `json:"parent_id,omitempty"`
}

// ClimateRef names a climate.
type ClimateRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PartyMarkView is a party mark with the names the Director sees.
type PartyMarkView struct {
	PartyMark
	PlaceName string `json:"place_name,omitempty"`
	// RegionID is the place whose weather the party has from this day; empty
	// for the world's weather.
	RegionID string `json:"region_id,omitempty"`
}

// PlaceWeatherYear is the Director's view of every place's weather for a
// year: the world climate, the places with their own, the party's marks and
// each place's readings.
type PlaceWeatherYear struct {
	World  ClimateRef              `json:"world"`
	Places []ClimatePlace          `json:"places"`
	Party  []PartyMarkView         `json:"party"`
	Days   map[string][]DayWeather `json:"days"`
}

// PlaceToday is today's weather at one region of the main calendar.
type PlaceToday struct {
	Date    string      `json:"date"`
	Reading *DayWeather `json:"reading,omitempty"`
	// PartyHere is set when the party is in this region today.
	PartyHere bool `json:"party_here,omitempty"`
}

// PartyNow is where the party is today, as one viewer may know it.
type PartyNow struct {
	// PlaceName and RegionName are empty when there is none or the viewer may
	// not open that page.
	PlaceName   string `json:"place_name,omitempty"`
	RegionName  string `json:"region_name,omitempty"`
	ClimateName string `json:"climate_name"`
	// Since is the day of the mark in force; nil when the party was never
	// placed.
	Since *DayDate `json:"since,omitempty"`
}

// PlaceWeatherService is the per-place half of the calendar's weather. The
// writes and PlaceWeatherYear are for the Director: their routes require
// CanAuthorDmOnly, as the day-weather writes do.
type PlaceWeatherService interface {
	PlaceWeatherYear(ctx context.Context, calendarID, campaignID string, year int) (*PlaceWeatherYear, error)
	SetPlaceDayWeather(ctx context.Context, calendarID, campaignID, placeID string, days []DayWeatherInput) error
	ClearPlaceDayWeather(ctx context.Context, calendarID, campaignID, placeID string, dates []DayDate) error
	LockPlaceDayWeather(ctx context.Context, calendarID, campaignID, placeID string, dates []DayDate, locked bool) (int, error)
	// SetPartyPlace marks the party at placeID from day d; "" is nowhere
	// with a climate of its own.
	SetPartyPlace(ctx context.Context, calendarID, campaignID string, d DayDate, placeID string) error
	ClearPartyPlace(ctx context.Context, calendarID, campaignID string, d DayDate) error
	// ListPartyDayWeather is the party's weather by day, as players see it,
	// with the Director also getting days to come.
	ListPartyDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error)
	// PlaceToday is today's weather in a region on the main calendar; nil
	// when a player asks about a region the party is not in today, or there
	// is no calendar.
	PlaceToday(ctx context.Context, campaignID, regionID string, v permissions.Viewer) (*PlaceToday, error)
	// WorldClimate is the main calendar's climate; false without one.
	WorldClimate(ctx context.Context, campaignID string, v permissions.Viewer) (ClimateRef, bool)
	// PartyNow is where the party is on the calendar's today.
	PartyNow(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*PartyNow, error)
}

var _ PlaceWeatherService = (*calendarService)(nil)

// SetPlaceWeather wires per-place weather. Reached by type assertion, like
// the other setters; unset, the calendar keeps one weather.
func (s *calendarService) SetPlaceWeather(repo PlaceWeatherRepository, dir PlaceDirectory) {
	s.placeRepo, s.places = repo, dir
}

func (s *calendarService) placesOn() bool { return s.placeRepo != nil && s.places != nil }

func errNoPlaceWeather() error {
	return apperror.NewBadRequest("weather for each place is not available here")
}

// worldClimate is the calendar's own climate, the default for every place
// with none.
func (s *calendarService) worldClimate(ctx context.Context, calendarID string) ClimateRef {
	id := DefaultWeatherClimate
	if st, err := s.weatherRepo.GetSettings(ctx, calendarID); err == nil && st != nil && validClimate(st.Climate) {
		id = st.Climate
	}
	return ClimateRef{ID: id, Name: climateName(id)}
}

// climatePlaces is the directory's places with their climate names, by name.
func (s *calendarService) climatePlaces(ctx context.Context, campaignID string) ([]ClimatePlace, error) {
	got, err := s.places.ClimatePlaces(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list climate places: %w", err)
	}
	out := make([]ClimatePlace, 0, len(got))
	for _, p := range got {
		if !validClimate(p.Climate) {
			continue
		}
		p.ClimateName = climateName(p.Climate)
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// regionMark is a party mark resolved to the region whose weather it gives.
type regionMark struct {
	date   DayDate
	region string
}

// partyRegions resolves the calendar's marks, keeping only those on or
// before upTo when it is set: a mark after today is the Director's plan,
// which players must not see through the weather. regions holds each
// region the marks reach.
func (s *calendarService) partyRegions(ctx context.Context, cal *Calendar, upTo *DayDate) ([]regionMark, map[string]ClimatePlace, error) {
	marks, err := s.placeRepo.ListPartyMarks(ctx, cal.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list party marks: %w", err)
	}
	regions := map[string]ClimatePlace{}
	resolved := map[string]string{}
	out := make([]regionMark, 0, len(marks))
	for _, m := range marks {
		if upTo != nil && !dayOnOrBefore(m.Year, m.Month, m.Day, upTo.Year, upTo.Month, upTo.Day) {
			continue
		}
		region := ""
		if m.PlaceID != "" {
			r, seen := resolved[m.PlaceID]
			if !seen {
				if p, err := s.places.RegionOf(ctx, cal.CampaignID, m.PlaceID); err == nil && p != nil && validClimate(p.Climate) {
					p.ClimateName = climateName(p.Climate)
					regions[p.ID] = *p
					r = p.ID
				}
				resolved[m.PlaceID] = r
			}
			region = r
		}
		out = append(out, regionMark{date: DayDate{m.Year, m.Month, m.Day}, region: region})
	}
	return out, regions, nil
}

// regionAt is the region the party is in on a day: the latest mark on or
// before it, "" (the world) before the first.
func regionAt(marks []regionMark, y, m, d int) string {
	region := ""
	for _, mk := range marks {
		if !dayOnOrBefore(mk.date.Year, mk.date.Month, mk.date.Day, y, m, d) {
			break
		}
		region = mk.region
	}
	return region
}

// partyWeather merges the world's readings with the regions' by where the
// party was each day. A day in a region shows that region's reading, or
// nothing; a day elsewhere shows the world's. tag adjusts each region
// reading taken (naming or redacting it for the viewer). Pure, so the rule
// is pinned by table tests.
func partyWeather(world []DayWeather, marks []regionMark, place map[string][]DayWeather, tag func(region string, w *DayWeather)) []DayWeather {
	if len(marks) == 0 {
		return world
	}
	byPlace := map[string]map[DayDate]DayWeather{}
	for region, days := range place {
		m := make(map[DayDate]DayWeather, len(days))
		for _, w := range days {
			m[DayDate{w.Year, w.Month, w.Day}] = w
		}
		byPlace[region] = m
	}
	out := make([]DayWeather, 0, len(world))
	seen := map[DayDate]bool{}
	take := func(region string, w DayWeather) {
		if tag != nil {
			tag(region, &w)
		}
		out = append(out, w)
	}
	for _, w := range world {
		key := DayDate{w.Year, w.Month, w.Day}
		seen[key] = true
		region := regionAt(marks, w.Year, w.Month, w.Day)
		if region == "" {
			out = append(out, w)
			continue
		}
		if pw, ok := byPlace[region][key]; ok {
			take(region, pw)
		}
	}
	for region, days := range byPlace {
		for key, pw := range days {
			if seen[key] || regionAt(marks, key.Year, key.Month, key.Day) != region {
				continue
			}
			take(region, pw)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return !dayOnOrBefore(b.Year, b.Month, b.Day, a.Year, a.Month, a.Day)
	})
	return out
}

// overlayParty turns stored world readings into the party's weather for v.
// world is already cut to what v may see; place readings are cut the same
// way. A player gets no place id and sees the region's name only when they
// may open its page; the generator's zone fields, which carry the place's id
// and name, are dropped for them.
func (s *calendarService) overlayParty(ctx context.Context, cal *Calendar, world []DayWeather, year, month int, v permissions.Viewer) ([]DayWeather, error) {
	if !s.placesOn() {
		return world, nil
	}
	director := v.SkipsPerUserRules()
	today := DayDate{cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay}
	var upTo *DayDate
	if !director {
		upTo = &today
	}
	marks, regions, err := s.partyRegions(ctx, cal, upTo)
	if err != nil {
		return nil, err
	}
	if len(regions) == 0 {
		return partyWeather(world, marks, nil, nil), nil
	}
	ids := make([]string, 0, len(regions))
	for id := range regions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	place, err := s.placeRepo.ListPlaceDays(ctx, cal.ID, ids, year, month)
	if err != nil {
		return nil, fmt.Errorf("list place weather: %w", err)
	}
	if !director {
		for id, days := range place {
			kept := days[:0:0]
			for _, d := range days {
				if dayOnOrBefore(d.Year, d.Month, d.Day, today.Year, today.Month, today.Day) {
					kept = append(kept, d)
				}
			}
			place[id] = kept
		}
	}
	seeable := map[string]bool{}
	if director {
		for _, id := range ids {
			seeable[id] = true
		}
	} else {
		seeable = s.places.Viewable(ctx, cal.CampaignID, ids, v)
	}
	tag := func(region string, w *DayWeather) {
		if !director {
			w.Locked = nil
			w.ZoneID, w.ZoneName = nil, nil
		}
		if seeable[region] {
			name := regions[region].Name
			w.Place = &name
		}
	}
	return partyWeather(world, marks, place, tag), nil
}

// ListPartyDayWeather is ListDayWeather with the party's weather for every
// viewer, the Director included: what the world shows today.
func (s *calendarService) ListPartyDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	world, err := s.listWorldDays(ctx, cal, year, month, v)
	if err != nil {
		return nil, err
	}
	return s.overlayParty(ctx, cal, world, year, month, v)
}

// PlaceWeatherYear is everything the Director's calendar draws for places.
func (s *calendarService) PlaceWeatherYear(ctx context.Context, calendarID, campaignID string, year int) (*PlaceWeatherYear, error) {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	if year < -maxDayWeatherYear || year > maxDayWeatherYear {
		return nil, apperror.NewBadRequest(fmt.Sprintf("year must be between %d and %d", -maxDayWeatherYear, maxDayWeatherYear))
	}
	out := &PlaceWeatherYear{World: s.worldClimate(ctx, cal.ID), Places: []ClimatePlace{}, Party: []PartyMarkView{}, Days: map[string][]DayWeather{}}
	if !s.placesOn() {
		return out, nil
	}
	if out.Places, err = s.climatePlaces(ctx, campaignID); err != nil {
		return nil, err
	}
	marks, err := s.placeRepo.ListPartyMarks(ctx, cal.ID)
	if err != nil {
		return nil, fmt.Errorf("list party marks: %w", err)
	}
	for _, m := range marks {
		mv := PartyMarkView{PartyMark: m}
		if m.PlaceID != "" {
			mv.PlaceName, _ = s.places.PlaceName(ctx, campaignID, m.PlaceID)
			if r, err := s.places.RegionOf(ctx, campaignID, m.PlaceID); err == nil && r != nil && validClimate(r.Climate) {
				mv.RegionID = r.ID
			}
		}
		out.Party = append(out.Party, mv)
	}
	ids := make([]string, len(out.Places))
	for i, p := range out.Places {
		ids[i] = p.ID
	}
	days, err := s.placeRepo.ListPlaceDays(ctx, cal.ID, ids, year, 0)
	if err != nil {
		return nil, fmt.Errorf("list place weather: %w", err)
	}
	for id, list := range days {
		out.Days[id] = list
	}
	return out, nil
}

// placeForWeather checks placeID is a page of the campaign with a climate of
// its own: only such a place has weather stored for it.
func (s *calendarService) placeForWeather(ctx context.Context, cal *Calendar, placeID string) error {
	if !s.placesOn() {
		return errNoPlaceWeather()
	}
	r, err := s.places.RegionOf(ctx, cal.CampaignID, placeID)
	if err != nil {
		return fmt.Errorf("find place: %w", err)
	}
	if r == nil || r.ID != placeID || !validClimate(r.Climate) {
		return apperror.NewBadRequest("that place has no climate of its own; set one on its page first")
	}
	return nil
}

// placeCalendar is a write's calendar with its geometry, after the place
// check.
func (s *calendarService) placeCalendar(ctx context.Context, calendarID, campaignID, placeID string) (*Calendar, error) {
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	if err := s.placeForWeather(ctx, cal, placeID); err != nil {
		return nil, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}
	return cal, nil
}

// SetPlaceDayWeather stores one place's readings, judged exactly as the
// world's (checkDayWeather).
func (s *calendarService) SetPlaceDayWeather(ctx context.Context, calendarID, campaignID, placeID string, days []DayWeatherInput) error {
	if len(days) > maxDayWeatherBatch {
		return apperror.NewBadRequest(fmt.Sprintf("at most %d days can be set at once", maxDayWeatherBatch))
	}
	cal, err := s.placeCalendar(ctx, calendarID, campaignID, placeID)
	if err != nil {
		return err
	}
	if err := checkDayWeather(cal, days); err != nil {
		return err
	}
	return s.publishAfter(s.placeRepo.SetPlaceDays(ctx, calendarID, placeID, days), PubWeatherChanged, campaignID, calendarID)
}

func (s *calendarService) checkDates(cal *Calendar, dates []DayDate, verb string) error {
	if len(dates) > maxDayWeatherBatch {
		return apperror.NewBadRequest(fmt.Sprintf("at most %d days can be %s at once", maxDayWeatherBatch, verb))
	}
	for _, d := range dates {
		if err := validateDayDate(cal, d.Year, d.Month, d.Day); err != nil {
			return err
		}
	}
	return nil
}

// ClearPlaceDayWeather deletes one place's readings on the given days.
func (s *calendarService) ClearPlaceDayWeather(ctx context.Context, calendarID, campaignID, placeID string, dates []DayDate) error {
	cal, err := s.placeCalendar(ctx, calendarID, campaignID, placeID)
	if err != nil {
		return err
	}
	if err := s.checkDates(cal, dates, "cleared"); err != nil {
		return err
	}
	return s.publishAfter(s.placeRepo.ClearPlaceDays(ctx, calendarID, placeID, dates), PubWeatherChanged, campaignID, calendarID)
}

// LockPlaceDayWeather sets or clears the lock on one place's readings.
func (s *calendarService) LockPlaceDayWeather(ctx context.Context, calendarID, campaignID, placeID string, dates []DayDate, locked bool) (int, error) {
	cal, err := s.placeCalendar(ctx, calendarID, campaignID, placeID)
	if err != nil {
		return 0, err
	}
	if err := s.checkDates(cal, dates, "locked"); err != nil {
		return 0, err
	}
	n, err := s.placeRepo.LockPlaceDays(ctx, calendarID, placeID, dates, locked)
	if err != nil {
		return 0, fmt.Errorf("lock place weather: %w", err)
	}
	return n, nil
}

// SetPartyPlace marks where the party is from day d. Any live page of the
// campaign may be named; its weather is its region's.
func (s *calendarService) SetPartyPlace(ctx context.Context, calendarID, campaignID string, d DayDate, placeID string) error {
	if !s.placesOn() {
		return errNoPlaceWeather()
	}
	cal, err := s.calendarInCampaign(ctx, calendarID, campaignID)
	if err != nil {
		return err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return err
	}
	if err := validateDayDate(cal, d.Year, d.Month, d.Day); err != nil {
		return err
	}
	if placeID != "" {
		if _, ok := s.places.PlaceName(ctx, campaignID, placeID); !ok {
			return apperror.NewBadRequest("that place is not a page of this campaign")
		}
	}
	return s.publishAfter(s.placeRepo.SetPartyMark(ctx, calendarID, PartyMark{Year: d.Year, Month: d.Month, Day: d.Day, PlaceID: placeID}),
		PubWeatherChanged, campaignID, calendarID)
}

// ClearPartyPlace takes back the mark on day d; the mark before it applies
// again.
func (s *calendarService) ClearPartyPlace(ctx context.Context, calendarID, campaignID string, d DayDate) error {
	if !s.placesOn() {
		return errNoPlaceWeather()
	}
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.publishAfter(s.placeRepo.ClearPartyMark(ctx, calendarID, d), PubWeatherChanged, campaignID, calendarID)
}

// PlaceToday reads today's weather in a region on the main calendar. A
// player gets it only for the region the party is in today, so a place's
// page never tells them the weather somewhere the party is not.
func (s *calendarService) PlaceToday(ctx context.Context, campaignID, regionID string, v permissions.Viewer) (*PlaceToday, error) {
	if !s.placesOn() || regionID == "" {
		return nil, nil
	}
	cal, err := s.GetDefaultCalendarForViewer(ctx, campaignID, v)
	if err != nil || cal == nil {
		return nil, nil //nolint:nilerr // no calendar this viewer can see: no weather line
	}
	today := DayDate{cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay}
	marks, _, err := s.partyRegions(ctx, cal, &today)
	if err != nil {
		return nil, err
	}
	here := regionAt(marks, today.Year, today.Month, today.Day) == regionID
	if !v.SkipsPerUserRules() && !here {
		return nil, nil
	}
	out := &PlaceToday{Date: dateWords(cal, today), PartyHere: here}
	days, err := s.placeRepo.ListPlaceDays(ctx, cal.ID, []string{regionID}, today.Year, today.Month)
	if err != nil {
		return nil, fmt.Errorf("list place weather: %w", err)
	}
	for _, d := range days[regionID] {
		if d.Day == today.Day {
			r := d
			if !v.SkipsPerUserRules() {
				r.Locked, r.ZoneID, r.ZoneName = nil, nil, nil
			}
			out.Reading = &r
		}
	}
	return out, nil
}

// dateWords is "Firstfrost 8": the month's name and the day.
func dateWords(cal *Calendar, d DayDate) string {
	if d.Month >= 1 && d.Month <= len(cal.Months) {
		return fmt.Sprintf("%s %d", cal.Months[d.Month-1].Name, d.Day)
	}
	return fmt.Sprintf("%d-%d", d.Month, d.Day)
}

// WorldClimate is the main calendar's climate as v may see the calendar.
func (s *calendarService) WorldClimate(ctx context.Context, campaignID string, v permissions.Viewer) (ClimateRef, bool) {
	cal, err := s.calRepo.GetDefaultByCampaignID(ctx, campaignID)
	if err != nil || cal == nil || cal.CampaignID != campaignID || !calendarVisibleToViewer(*cal, v) {
		return ClimateRef{}, false
	}
	return s.worldClimate(ctx, cal.ID), true
}

// PartyNow says where the party is on the calendar's today, naming only
// pages v may open. A player is never told about a mark after today.
func (s *calendarService) PartyNow(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*PartyNow, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	world := s.worldClimate(ctx, cal.ID)
	out := &PartyNow{ClimateName: world.Name}
	if !s.placesOn() {
		return out, nil
	}
	marks, err := s.placeRepo.ListPartyMarks(ctx, cal.ID)
	if err != nil {
		return nil, fmt.Errorf("list party marks: %w", err)
	}
	var cur *PartyMark
	for i := range marks {
		m := marks[i]
		if dayOnOrBefore(m.Year, m.Month, m.Day, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay) {
			cur = &m
		}
	}
	if cur == nil {
		return out, nil
	}
	out.Since = &DayDate{cur.Year, cur.Month, cur.Day}
	if cur.PlaceID == "" {
		return out, nil
	}
	ids := []string{cur.PlaceID}
	region, err := s.places.RegionOf(ctx, campaignID, cur.PlaceID)
	if err != nil {
		return nil, fmt.Errorf("find region: %w", err)
	}
	if region != nil && validClimate(region.Climate) {
		out.ClimateName = climateName(region.Climate)
		ids = append(ids, region.ID)
	} else {
		region = nil
	}
	seeable := s.places.Viewable(ctx, campaignID, ids, v)
	if v.SkipsPerUserRules() {
		seeable = map[string]bool{cur.PlaceID: true}
		if region != nil {
			seeable[region.ID] = true
		}
	}
	if seeable[cur.PlaceID] {
		out.PlaceName, _ = s.places.PlaceName(ctx, campaignID, cur.PlaceID)
	}
	if region != nil && seeable[region.ID] {
		out.RegionName = region.Name
	}
	return out, nil
}

// forecastRegion is the region the party is in today, "" for the world.
func (s *calendarService) forecastRegion(ctx context.Context, cal *Calendar, today DayDate) (string, error) {
	if !s.placesOn() {
		return "", nil
	}
	marks, _, err := s.partyRegions(ctx, cal, &today)
	if err != nil {
		return "", err
	}
	return regionAt(marks, today.Year, today.Month, today.Day), nil
}

// forecastRows is one month's stored readings for the forecast: the region's
// own when the party is in one, else the world's.
func (s *calendarService) forecastRows(ctx context.Context, cal *Calendar, region string, year, month int) ([]DayWeather, error) {
	if region == "" {
		rows, err := s.weatherRepo.ListDays(ctx, cal.ID, year, month)
		if err != nil {
			return nil, fmt.Errorf("list day weather: %w", err)
		}
		return rows, nil
	}
	byPlace, err := s.placeRepo.ListPlaceDays(ctx, cal.ID, []string{region}, year, month)
	if err != nil {
		return nil, fmt.Errorf("list place weather: %w", err)
	}
	return byPlace[region], nil
}

// DayWeatherLister is the day-weather read every "today's weather" reader
// already has.
type DayWeatherLister interface {
	ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error)
}

// PartyDays is the weather the world shows: the party's, for every viewer.
// The header, the DM Screen, Today in the world and Foundry read today's
// weather through it, so the Director sees there what players see. A
// service without places reads plain day weather.
func PartyDays(ctx context.Context, svc DayWeatherLister, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error) {
	if pw, ok := svc.(interface {
		ListPartyDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]DayWeather, error)
	}); ok {
		return pw.ListPartyDayWeather(ctx, calendarID, campaignID, year, month, v)
	}
	return svc.ListDayWeather(ctx, calendarID, campaignID, year, month, v)
}
