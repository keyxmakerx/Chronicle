package app

import (
	"context"
	"fmt"
	"math"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// placeDirectoryAdapter gives the calendar the campaign's places
// (calendar.PlaceDirectory) through the entities service, never its
// repository. Its reads are unfiltered; Viewable is the one visibility
// question, asked with the entities plugin's own predicate.
type placeDirectoryAdapter struct{ pages entities.EntityService }

var _ calendar.PlaceDirectory = placeDirectoryAdapter{}

func (a placeDirectoryAdapter) ClimatePlaces(ctx context.Context, campaignID string) ([]calendar.ClimatePlace, error) {
	got, err := a.pages.ClimatePlaces(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]calendar.ClimatePlace, len(got))
	for i, p := range got {
		out[i] = calendar.ClimatePlace{ID: p.ID, Name: p.Name, Climate: p.Climate, ParentID: p.ParentID}
	}
	return out, nil
}

func (a placeDirectoryAdapter) RegionOf(ctx context.Context, campaignID, entityID string) (*calendar.ClimatePlace, error) {
	e, err := a.pages.GetByID(ctx, entityID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if e == nil || e.CampaignID != campaignID {
		return nil, nil
	}
	region, climate, err := a.pages.ClimateRegion(ctx, e)
	if err != nil || region == nil {
		return nil, err
	}
	out := &calendar.ClimatePlace{ID: region.ID, Name: region.Name, Climate: climate}
	if region.ParentID != nil {
		out.ParentID = *region.ParentID
	}
	return out, nil
}

func (a placeDirectoryAdapter) Viewable(ctx context.Context, campaignID string, ids []string, v permissions.Viewer) map[string]bool {
	if v.IsSystem() {
		out := make(map[string]bool, len(ids))
		for _, id := range ids {
			out[id] = true
		}
		return out
	}
	got, err := a.pages.FilterViewableEntityIDs(ctx, campaignID, ids, v.Role(), v.UserID())
	if err != nil {
		return map[string]bool{} // fail closed: name nothing
	}
	return got
}

func (a placeDirectoryAdapter) PlaceName(ctx context.Context, campaignID, entityID string) (string, bool) {
	e, err := a.pages.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		return "", false
	}
	return e.Name, true
}

// fieldClimateAdapter gives pages' climate fields the calendar's climates and
// weather (entities.FieldClimate). With the calendar switched off a field
// still lists the climates, but there is no world climate and no weather.
type fieldClimateAdapter struct {
	svc    calendar.CalendarService
	addons questAddonChecker
}

var _ entities.FieldClimate = fieldClimateAdapter{}

func (fieldClimateAdapter) Climates() []entities.ClimateChoice {
	out := make([]entities.ClimateChoice, len(calendar.WeatherClimates))
	for i, c := range calendar.WeatherClimates {
		out[i] = entities.ClimateChoice{ID: c.ID, Name: c.Name, Magic: c.Magic}
	}
	return out
}

func (a fieldClimateAdapter) places(ctx context.Context, campaignID string) (calendar.PlaceWeatherService, bool) {
	if on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, calendar.PluginSlug); err != nil || !on {
		return nil, false
	}
	pw, ok := a.svc.(calendar.PlaceWeatherService)
	return pw, ok
}

func (a fieldClimateAdapter) World(ctx context.Context, campaignID string, role int, userID string) (entities.ClimateChoice, bool) {
	pw, ok := a.places(ctx, campaignID)
	if !ok {
		return entities.ClimateChoice{}, false
	}
	w, ok := pw.WorldClimate(ctx, campaignID, permissions.RequestViewer(role, userID))
	return entities.ClimateChoice{ID: w.ID, Name: w.Name}, ok
}

func (a fieldClimateAdapter) Today(ctx context.Context, campaignID, regionID string, role int, userID string) *entities.ClimateToday {
	pw, ok := a.places(ctx, campaignID)
	if !ok {
		return nil
	}
	t, err := pw.PlaceToday(ctx, campaignID, regionID, permissions.RequestViewer(role, userID))
	if err != nil || t == nil {
		return nil
	}
	return &entities.ClimateToday{Date: t.Date, Words: readingWords(t.Reading), PartyHere: t.PartyHere}
}

// readingWords is "Light rain, 6°": the reading's label and temperature.
func readingWords(w *calendar.DayWeather) string {
	if w == nil {
		return ""
	}
	label := ""
	switch {
	case w.PresetLabel != nil && *w.PresetLabel != "":
		label = *w.PresetLabel
	case w.Description != nil && *w.Description != "":
		label = *w.Description
	default:
		label = "Weather recorded"
	}
	if w.TemperatureCelsius != nil {
		return fmt.Sprintf("%s, %d°", label, int(math.Round(*w.TemperatureCelsius)))
	}
	return label
}
