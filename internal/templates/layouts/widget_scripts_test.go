package layouts

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// widgetScriptsJSON matches the manifest element base.templ emits.
var widgetScriptsJSON = regexp.MustCompile(`(?s)<script id="chronicle-widget-scripts" type="application/json">(.*?)</script>`)

// TestBaseEmitsWidgetScripts pins the manifest boot.js reads: one inert JSON
// element, always a JSON object, escaped so a URL can never close the
// script element.
func TestBaseEmitsWidgetScripts(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want map[string][]string
	}{
		{
			name: "no manifest set",
			ctx:  context.Background(),
			want: map[string][]string{},
		},
		{
			name: "manifest set",
			ctx: SetWidgetScripts(context.Background(), map[string][]string{
				"timeline-viz": {"/static/plugins/chart/js/timeline_viz.js?v=1", "/x</script>.js"},
			}),
			want: map[string][]string{
				"timeline-viz": {"/static/plugins/chart/js/timeline_viz.js?v=1", "/x</script>.js"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderBase(t, tt.ctx)
			m := widgetScriptsJSON.FindAllStringSubmatch(out, -1)
			if len(m) != 1 {
				t.Fatalf("found %d manifest elements, want 1", len(m))
			}
			if strings.Contains(m[0][1], "</script") {
				t.Fatalf("manifest body is not escaped: %s", m[0][1])
			}
			var got map[string][]string
			if err := json.Unmarshal([]byte(m[0][1]), &got); err != nil {
				t.Fatalf("manifest is not JSON: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("manifest = %v, want %v", got, tt.want)
			}
		})
	}
}
