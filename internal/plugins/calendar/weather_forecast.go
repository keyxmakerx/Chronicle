// Package calendar - weather_forecast.go builds the forecast players are
// shown in place of the real weather: the stored reading for a coming day,
// blurred more the further ahead it looks. It is a port of forecastWeather in
// static/js/widgets/chronicle_gen.js, simplified so it needs no climate
// model and is fully deterministic: the same calendar, day and lead always
// give the same entry, so a player cannot refresh their way to the truth and
// a forecast does not change under them between page loads.
//
// A forecast entry is built only from a coarse category and numbers. It never
// carries the reading's preset id, label, colour, description or wind, since
// those would leak a day the owner has kept hidden (an own kind such as
// "Fire rain" names itself).
package calendar

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
)

// forecastAccuracy is the generator's default forecast accuracy (0.7); it
// sets how quickly confidence falls with lead time.
const forecastAccuracy = 0.7

// Forecast categories: the coarse sky a forecast talks about.
const (
	fcDry   = "dry"
	fcCloud = "cloud"
	fcWet   = "wet"
	fcSnow  = "snow"
	fcStorm = "storm"
	fcFog   = "fog"
)

// forecastCategoryByIcon maps a reading's icon to its category; an icon not
// listed (or none) reads as cloud, the least committal sky.
var forecastCategoryByIcon = map[string]string{
	"clear": fcDry, "cloud": fcCloud, "rain": fcWet,
	"snow": fcSnow, "storm": fcStorm, "fog": fcFog,
}

// forecastIconByCategory is the only icon a forecast sends: the category's
// own glyph, never the real reading's icon.
var forecastIconByCategory = map[string]string{
	fcDry: "clear", fcCloud: "cloud", fcWet: "rain",
	fcSnow: "snow", fcStorm: "storm", fcFog: "fog",
}

// forecastNeighbours are the skies a wrong forecast can land on: the ones
// that plausibly stand in for the truth.
var forecastNeighbours = map[string][]string{
	fcDry:   {fcCloud},
	fcCloud: {fcDry, fcWet, fcFog},
	fcWet:   {fcCloud, fcStorm},
	fcSnow:  {fcCloud, fcWet},
	fcStorm: {fcWet, fcCloud},
	fcFog:   {fcCloud, fcDry},
}

// ForecastEntry is one day of a player's forecast. TempLow and TempHigh are
// null when the real reading carries no temperature.
type ForecastEntry struct {
	Year         int     `json:"year"`
	Month        int     `json:"month"`
	Day          int     `json:"day"`
	Lead         int     `json:"lead"`
	Confidence   float64 `json:"confidence"`
	Icon         string  `json:"icon"`
	Words        string  `json:"words"`
	TempLow      *int    `json:"temp_low"`
	TempHigh     *int    `json:"temp_high"`
	PrecipChance int     `json:"precip_chance"`
}

// forecastConfidence is how sure the forecast is, lead days out (lead 1 is
// tomorrow): it decays exponentially and is held between 0.18 and 0.97 so
// tomorrow is never certain and the far end is never pure noise.
func forecastConfidence(lead int) float64 {
	k := 0.34 - 0.24*forecastAccuracy
	return math.Min(0.97, math.Max(0.18, 0.97*math.Exp(-float64(lead-1)*k)))
}

// forecastSeed derives the entry's RNG seed from the calendar, the day and
// the lead, so each is stable and independent of the others.
func forecastSeed(calendarID string, date DayDate, lead int) int64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s|%d|%d|%d|%d", calendarID, date.Year, date.Month, date.Day, lead)
	return int64(h.Sum64())
}

// roundTo10 rounds to the nearest ten, the precision a chance of rain is
// given in.
func roundTo10(x float64) int { return int(math.Round(x/10)) * 10 }

// buildForecastEntry blurs the real reading for date, lead days ahead. The
// draws from the RNG happen in a fixed order (category, then a wrong sky if
// the forecast misses, then temperature) so each result is reproducible.
func buildForecastEntry(calendarID string, date DayDate, lead int, real DayWeather) ForecastEntry {
	conf := forecastConfidence(lead)
	rng := rand.New(rand.NewSource(forecastSeed(calendarID, date, lead))) //nolint:gosec // a reproducible blur, not a secret

	truth := fcCloud
	if real.Icon != nil {
		if c, ok := forecastCategoryByIcon[*real.Icon]; ok {
			truth = c
		}
	}
	shown := truth
	if rng.Float64() >= conf {
		n := forecastNeighbours[truth]
		shown = n[rng.Intn(len(n))]
	}

	// A wet sky is sure in proportion to confidence when it is the truth,
	// and only as likely as the forecast is wrong when it is not; a dry-ish
	// sky keeps a small chance of rain that grows as confidence falls.
	var chance int
	switch shown {
	case fcWet, fcSnow, fcStorm:
		if shown == truth {
			chance = roundTo10(conf * 100)
		} else {
			chance = roundTo10((1 - conf) * 100)
		}
	default:
		chance = roundTo10((1 - conf) * 40)
	}

	e := ForecastEntry{
		Year: date.Year, Month: date.Month, Day: date.Day, Lead: lead,
		Confidence:   math.Round(conf*100) / 100,
		Icon:         forecastIconByCategory[shown],
		Words:        forecastWords(shown, conf, chance),
		PrecipChance: chance,
	}
	if real.TemperatureCelsius != nil {
		center := *real.TemperatureCelsius + rng.NormFloat64()*(1-conf)*3*0.8
		spread := 1 + (1-conf)*3*1.4
		lo, hi := int(math.Round(center-spread)), int(math.Round(center+spread))
		e.TempLow, e.TempHigh = &lo, &hi
	}
	return e
}

// forecastWordTable holds, per category, the phrase for high, middling and
// low confidence (mirrors forecastWords in chronicle_gen.js).
var forecastWordTable = map[string][3]string{
	fcDry:   {"Dry and bright", "Probably dry", "Mostly dry"},
	fcCloud: {"Grey and dry", "Probably grey", "Cloudy, maybe"},
	fcWet:   {"Rain", "Rain likely", "Chance of rain"},
	fcSnow:  {"Snow", "Snow likely", "Chance of snow"},
	fcStorm: {"Storms", "Storms likely", "Chance of storms"},
	fcFog:   {"Fog", "Fog likely", "Chance of fog"},
}

// forecastWords names a forecast in plain words: firm below 0.8 confidence
// gives way to hedged, and under 0.35 the forecast admits it cannot call it.
func forecastWords(category string, conf float64, chance int) string {
	if conf < 0.35 {
		if chance >= 50 {
			return "Unsettled"
		}
		return "Hard to call"
	}
	names, ok := forecastWordTable[category]
	if !ok {
		return "Unsettled"
	}
	switch {
	case conf >= 0.8:
		return names[0]
	case conf >= 0.55:
		return names[1]
	}
	return names[2]
}

// addDays steps a date n days forward through the calendar's own months and
// leap rule, so a forecast crosses a month end or a year end the way the
// calendar does. It reports false when the calendar has no day to land on
// (every month empty), rather than looping forever. Geometry must be loaded.
func (c *Calendar) addDays(date DayDate, n int) (DayDate, bool) {
	y, m, d := date.Year, date.Month, date.Day
	for i := 0; i < n; i++ {
		d++
		for guard := 0; d > c.MonthDays(m-1, y); guard++ {
			if guard > 2*len(c.Months)+2 {
				return DayDate{}, false
			}
			m++
			d = 1
			if m > len(c.Months) {
				m = 1
				y++
			}
		}
	}
	return DayDate{Year: y, Month: m, Day: d}, true
}
