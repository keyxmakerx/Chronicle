package htmlconv

import (
	"strings"
	"testing"
)

func TestConvert_Link_PreservesEntityPreview(t *testing.T) {
	got, err := Convert(`<p><a href="/x" data-mention-id="ent-42" data-entity-preview="/x/preview">Lyra</a></p>`)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	text := firstChildOfDoc(t, got)["content"].([]any)[0].(map[string]any)
	attrs := text["marks"].([]any)[0].(map[string]any)["attrs"].(map[string]any)
	if attrs["data-entity-preview"] != "/x/preview" {
		t.Errorf("attrs.data-entity-preview = %v, want /x/preview", attrs["data-entity-preview"])
	}
}

// The editor saves a picture as a figure; converting it back must give the
// chronicleImage node the editor reads, never plain text.
func TestConvert_Picture(t *testing.T) {
	const id = "0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70"
	cases := []struct {
		name, html  string
		wantWidth   float64
		wantAlign   string
		wantCaption string
		wantGM      bool
	}{
		{"figure with width, side and caption",
			`<figure class="ce-img ce-img--w40 ce-img--right"><img src="/media/` + id + `" alt="Mira"><figcaption>Mira Kell</figcaption></figure>`,
			40, "right", "Mira Kell", false},
		{"gm only figure",
			`<figure class="ce-img ce-img--w100 ce-img--center ce-img--gm"><img src="/media/` + id + `" alt=""></figure>`,
			100, "center", "", true},
		{"bare image on this site", `<img src="/media/` + id + `" alt="x">`, 100, "center", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Convert(c.html)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			n := firstChildOfDoc(t, got)
			if n["type"] != "chronicleImage" {
				t.Fatalf("type = %v, want chronicleImage\n%s", n["type"], got)
			}
			a := n["attrs"].(map[string]any)
			if a["mediaId"] != id || a["width"] != c.wantWidth || a["align"] != c.wantAlign || a["caption"] != c.wantCaption || a["gmOnly"] != c.wantGM {
				t.Errorf("attrs = %v", a)
			}
		})
	}
}

func TestConvert_PictureFromOtherSiteIsNotKept(t *testing.T) {
	for _, in := range []string{
		`<figure class="ce-img"><img src="https://evil.example/x.png"></figure>`,
		`<p>before <img src="https://evil.example/x.png"> after</p>`,
		`<img src="/media/not-an-id">`,
	} {
		got, err := Convert(in)
		if err != nil {
			t.Fatalf("Convert(%q): %v", in, err)
		}
		if strings.Contains(got, "chronicleImage") || strings.Contains(got, "evil.example") {
			t.Errorf("Convert(%q) kept an outside picture: %s", in, got)
		}
	}
}
