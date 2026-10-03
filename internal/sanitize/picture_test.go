package sanitize

import (
	"strings"
	"testing"
)

// Pictures inside editor text (static/js/widgets/editor_image.js) are saved as
// <figure class="ce-img ..."><img src="/media/<id>"><figcaption>. These pin
// that the sanitizer keeps that shape and that GM-only pictures never reach
// players.

const pictureID = "0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70"

func TestHTML_KeepsEditorPicture(t *testing.T) {
	in := `<figure class="ce-img ce-img--w40 ce-img--right"><img src="/media/` + pictureID + `" alt="Mira"><figcaption>Mira Kell</figcaption></figure>`
	out := HTML(in)
	for _, want := range []string{
		`<figure class="ce-img ce-img--w40 ce-img--right">`,
		`src="/media/` + pictureID + `"`,
		`alt="Mira"`,
		`<figcaption>Mira Kell</figcaption>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sanitized picture lost %q\n got: %s", want, out)
		}
	}
}

func TestHTML_PictureCannotCarryScript(t *testing.T) {
	cases := []string{
		`<figure class="ce-img" onclick="x()"><img src="/media/` + pictureID + `" onerror="x()"></figure>`,
		`<figure class="ce-img"><img src="javascript:alert(1)"></figure>`,
		`<figure class="ce-img" style="background:url(x)"><img src="/media/` + pictureID + `"></figure>`,
	}
	for _, in := range cases {
		out := HTML(in)
		for _, bad := range []string{"onclick", "onerror", "javascript:", "style="} {
			if strings.Contains(out, bad) {
				t.Errorf("HTML(%q) kept %q: %s", in, bad, out)
			}
		}
	}
}

func TestStripSecretsHTML_DropsGMPicture(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "gm picture with caption",
			in:   `<p>a</p><figure class="ce-img ce-img--w40 ce-img--left ce-img--gm"><img src="/media/` + pictureID + `" alt="x"><figcaption>The traitor</figcaption></figure><p>b</p>`,
			want: `<p>a</p><p>b</p>`,
		},
		{
			name: "two pictures, only the gm one goes",
			in:   `<figure class="ce-img ce-img--gm"><img src="/media/a"></figure><figure class="ce-img ce-img--w100 ce-img--center"><img src="/media/b"></figure>`,
			want: `<figure class="ce-img ce-img--w100 ce-img--center"><img src="/media/b"></figure>`,
		},
		{
			name: "picture for everyone stays",
			in:   `<figure class="ce-img ce-img--w50 ce-img--center"><img src="/media/b"></figure>`,
			want: `<figure class="ce-img ce-img--w50 ce-img--center"><img src="/media/b"></figure>`,
		},
		{
			name: "class name that only contains the word stays",
			in:   `<figure class="ce-img ce-img--gmx"><img src="/media/b"></figure>`,
			want: `<figure class="ce-img ce-img--gmx"><img src="/media/b"></figure>`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripSecretsHTML(c.in); got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestStripSecretsHTML_GMPictureSurvivesSanitizeThenStrips(t *testing.T) {
	// The stored HTML is the sanitizer's output; the strip must still match it.
	stored := HTML(`<p>x</p><figure class="ce-img ce-img--w30 ce-img--right ce-img--gm"><img src="/media/` + pictureID + `" alt="secret map"><figcaption>Hidden vault</figcaption></figure>`)
	got := StripSecretsHTML(stored)
	if strings.Contains(got, pictureID) || strings.Contains(got, "Hidden vault") {
		t.Errorf("GM-only picture reached players: %s", got)
	}
}

func TestStripSecretsJSON_DropsGMPicture(t *testing.T) {
	in := `{"type":"doc","content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"a"}]},` +
		`{"type":"chronicleImage","attrs":{"mediaId":"` + pictureID + `","caption":"The traitor","gmOnly":true}},` +
		`{"type":"chronicleImage","attrs":{"mediaId":"keep","gmOnly":false}}]}`
	out := StripSecretsJSON(in)
	if strings.Contains(out, pictureID) || strings.Contains(out, "The traitor") {
		t.Errorf("GM-only picture kept: %s", out)
	}
	if !strings.Contains(out, `"mediaId":"keep"`) {
		t.Errorf("picture for everyone dropped: %s", out)
	}
}
