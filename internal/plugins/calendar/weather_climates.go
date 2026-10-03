// Package calendar - weather_climates.go lists the climates a calendar can
// pick for its world. The ids and names mirror the weather generator in
// static/js/widgets/chronicle_gen.js (a JS test pins the two together), so
// the Generate sheet can start from the stored id without a lookup table.
package calendar

import (
	"math"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

const (
	// DefaultWeatherClimate and DefaultWeatherContinuity are what a calendar
	// with no stored setting reports.
	DefaultWeatherClimate    = "temperate"
	DefaultWeatherContinuity = 0.55
)

// WeatherClimate is one selectable climate. Magic climates are listed apart
// from the natural ones in the settings form.
type WeatherClimate struct {
	ID    string
	Name  string
	Magic bool
}

// WeatherClimates is the ordered list the settings form renders.
var WeatherClimates = []WeatherClimate{
	{ID: "temperate", Name: "Temperate"},
	{ID: "cold-coast", Name: "Cold coast"},
	{ID: "desert", Name: "Desert"},
	{ID: "tropical", Name: "Tropical"},
	{ID: "highland", Name: "Highland"},
	{ID: "mediterranean", Name: "Warm coast"},
	{ID: "tundra", Name: "Tundra"},
	{ID: "fey-wilds", Name: "Fey wilds", Magic: true},
	{ID: "ashlands", Name: "Ashlands", Magic: true},
	{ID: "gloomfen", Name: "Gloomfen", Magic: true},
}

// validClimate reports whether id is one of WeatherClimates.
func validClimate(id string) bool {
	for _, c := range WeatherClimates {
		if c.ID == id {
			return true
		}
	}
	return false
}

// climateName returns the display name for id, or id itself if unknown.
func climateName(id string) string {
	for _, c := range WeatherClimates {
		if c.ID == id {
			return c.Name
		}
	}
	return id
}

// validateWeatherSettings is the one rule for a climate and a continuity,
// shared by the service's save and the settings form's preview so the
// preview never accepts what the save would refuse.
func validateWeatherSettings(s WeatherSettings) error {
	if !validClimate(s.Climate) {
		return apperror.NewBadRequest("choose one of the listed climates")
	}
	if math.IsNaN(s.Continuity) || s.Continuity < 0 || s.Continuity > 1 {
		return apperror.NewBadRequest("how long weather lasts must be between 0 and 1")
	}
	return nil
}

// roundContinuity keeps the two decimals the column stores.
func roundContinuity(c float64) float64 {
	return math.Round(c*100) / 100
}

// continuityWords names a "how long weather lasts" value in the words the
// Generate sheet uses (continuityWords in calendar_weather_sheet.js);
// continuityWordsJS is the same rule for the settings page's live readout.
func continuityWords(v float64) string {
	switch {
	case v < 0.25:
		return "Changes daily"
	case v < 0.5:
		return "Changeable"
	case v < 0.75:
		return "Some spells"
	}
	return "Long spells"
}

const continuityWordsJS = "c < 0.25 ? 'Changes daily' : c < 0.5 ? 'Changeable' : c < 0.75 ? 'Some spells' : 'Long spells'"
