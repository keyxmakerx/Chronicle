package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestReadingWords(t *testing.T) {
	s := func(v string) *string { return &v }
	f := func(v float64) *float64 { return &v }
	tests := []struct {
		name string
		w    *calendar.DayWeather
		want string
	}{
		{"no reading", nil, ""},
		{"label and temperature, rounded", &calendar.DayWeather{PresetLabel: s("Light rain"), TemperatureCelsius: f(5.6)}, "Light rain, 6°"},
		{"a description when there is no label", &calendar.DayWeather{Description: s("Ash on the wind")}, "Ash on the wind"},
		{"below zero", &calendar.DayWeather{PresetLabel: s("Snow"), TemperatureCelsius: f(-3.4)}, "Snow, -3°"},
		{"nothing but a reading", &calendar.DayWeather{}, "Weather recorded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := readingWords(tt.w); got != tt.want {
				t.Errorf("readingWords = %q, want %q", got, tt.want)
			}
		})
	}
}
