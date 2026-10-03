// Package calendar - era_look_view.go builds the config the structure
// editor's Era look part hands to calendar_era_look.js.
package calendar

import (
	"encoding/json"
	"sort"
)

// eraLookEraConfig is one era as the Era look part shows it: its look
// fields, plus whether players can't see it yet (an eye on its row).
type eraLookEraConfig struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Color  string  `json:"color"`
	Color2 *string `json:"color_2"`
	Style  string  `json:"style"`
	Feel   *string `json:"feel"`
	Secret bool    `json:"secret"`
}

type eraLookConfig struct {
	Look EraLook            `json:"look"`
	Eras []eraLookEraConfig `json:"eras"`
}

// eraLookConfigJSON is the part's config: the calendar's look and its eras
// oldest first. The structure page is Owner only, so every era is here.
func eraLookConfigJSON(cal *Calendar) string {
	cfg := eraLookConfig{Look: cal.EraLook, Eras: []eraLookEraConfig{}}
	eras := append([]Era(nil), cal.Eras...)
	sort.SliceStable(eras, func(i, j int) bool {
		return cal.AbsoluteDay(eras[i].StartYear, eras[i].StartMonth, eras[i].StartDay) <
			cal.AbsoluteDay(eras[j].StartYear, eras[j].StartMonth, eras[j].StartDay)
	})
	for i := range eras {
		e := &eras[i]
		cfg.Eras = append(cfg.Eras, eraLookEraConfig{
			ID: e.ID, Name: e.Name, Color: e.Color, Color2: e.Color2,
			Style: eraStyleOrDefault(e.Style), Feel: e.Feel, Secret: cal.EraIsSecret(e),
		})
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return `{"look":{},"eras":[]}`
	}
	return string(b)
}
