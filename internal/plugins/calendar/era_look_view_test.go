// era_look_view_test.go pins the structure editor's Era look part: it
// mounts calendar_era_look.js with the look and every era oldest first,
// marking the ones players can't see yet, on a real-time calendar too.
package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"regexp"
	"testing"
)

func TestEraLookPart(t *testing.T) {
	cal, eras, _ := eraFixture()
	cal.Months = eraMonths()
	// Stored out of order: the part lists them oldest first.
	cal.Eras = []Era{eras[2], eras[0], eras[1]}
	cal.EraLook = EraLook{ColorsOn: true, Feel: EraFeelCustom, Intensity: 2, Speed: 0.5}
	for _, realTime := range []bool{false, true} {
		var buf bytes.Buffer
		data := StructureEditData{CampaignID: eraCampaignID, CampaignName: "C", Calendar: &cal, RealTime: realTime}
		if err := StructureEditFragment(data).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !bytes.Contains(buf.Bytes(), []byte(`data-widget="calendar_era_look"`)) || !bytes.Contains(buf.Bytes(), []byte(`id="era-look"`)) {
			t.Fatalf("realTime=%v: no Era look part:\n%s", realTime, out)
		}
		m := regexp.MustCompile(`data-config="([^"]*)"`).FindStringSubmatch(out)
		if m == nil {
			t.Fatal("no data-config")
		}
		var cfg eraLookConfig
		if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Look != cal.EraLook {
			t.Errorf("look = %+v", cfg.Look)
		}
		var names []string
		for _, e := range cfg.Eras {
			names = append(names, e.Name)
			if e.Secret != (e.Name == secretEraName) {
				t.Errorf("%s secret = %v", e.Name, e.Secret)
			}
		}
		if len(names) != 3 || names[0] != "Age of Dragons" || names[2] != secretEraName {
			t.Errorf("eras = %v, want oldest first", names)
		}
	}
}
