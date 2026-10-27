package designlab

import "testing"

func TestNewBenchSky(t *testing.T) {
	tests := []struct {
		name               string
		campaign, calendar string
		want               BenchSky
	}{
		{"both ids", "camp_1", "cal-2", BenchSky{CampaignID: "camp_1", CalendarID: "cal-2"}},
		{"neither", "", "", BenchSky{}},
		{"campaign only", "camp_1", "", BenchSky{}},
		{"calendar only", "", "cal-2", BenchSky{}},
		{"path in an id", "../admin", "cal-2", BenchSky{}},
		{"markup in an id", "camp_1", `"><script>`, BenchSky{}},
		{"too long", "camp_1", string(make([]byte, 65)), BenchSky{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewBenchSky(tt.campaign, tt.calendar); got != tt.want {
				t.Errorf("NewBenchSky(%q, %q) = %+v, want %+v", tt.campaign, tt.calendar, got, tt.want)
			}
		})
	}
}
