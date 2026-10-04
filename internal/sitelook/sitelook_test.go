package sitelook

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      Settings
		want    Settings
		wantErr bool
	}{
		{"defaults", Settings{}, Settings{Configured: true, Background: BackgroundPlain}, false},
		{"default name is stored empty", Settings{Name: " Chronicle "}, Settings{Configured: true, Background: BackgroundPlain}, false},
		{"custom name trimmed", Settings{Name: "  Dragon Hold "}, Settings{Configured: true, Name: "Dragon Hold", Background: BackgroundPlain}, false},
		{"name at limit", Settings{Name: strings.Repeat("é", 40)}, Settings{Configured: true, Name: strings.Repeat("é", 40), Background: BackgroundPlain}, false},
		{"name too long", Settings{Name: strings.Repeat("a", 41)}, Settings{}, true},
		{"name with newline", Settings{Name: "a\nb"}, Settings{}, true},
		{"welcome too long", Settings{Welcome: strings.Repeat("a", 81)}, Settings{}, true},
		{"welcome control char", Settings{Welcome: "hi\x00"}, Settings{}, true},
		{"welcome kept raw for templ to escape", Settings{Welcome: "<b>hi</b>"}, Settings{Configured: true, Welcome: "<b>hi</b>", Background: BackgroundPlain}, false},
		{"unknown look", Settings{Look: "neon"}, Settings{}, true},
		{"known look", Settings{Look: "ember"}, Settings{Configured: true, Look: "ember", Background: BackgroundPlain}, false},
		{"look background needs a look", Settings{Background: BackgroundLook}, Settings{}, true},
		{"look background", Settings{Look: "arcane", Background: BackgroundLook}, Settings{Configured: true, Look: "arcane", Background: BackgroundLook}, false},
		{"picture background needs a picture", Settings{Background: BackgroundPicture}, Settings{}, true},
		{"picture background", Settings{Background: BackgroundPicture, Picture: "2026/10/abc.jpg"},
			Settings{Configured: true, Background: BackgroundPicture, Picture: "2026/10/abc.jpg"}, false},
		{"stale picture dropped", Settings{Background: BackgroundPlain, Picture: "2026/10/abc.jpg"}, Settings{Configured: true, Background: BackgroundPlain}, false},
		{"unknown background", Settings{Background: "moving"}, Settings{}, true},
		{"svg logo refused", Settings{Logo: "2026/10/abc.svg"}, Settings{}, true},
		{"traversal logo refused", Settings{Logo: "../etc/passwd.png"}, Settings{}, true},
		{"css-breaking picture refused", Settings{Background: BackgroundPicture, Picture: "2026/10/a');x.png"}, Settings{}, true},
		{"move kept over the look's colours", Settings{Look: "arcane", Background: BackgroundLook, Move: true}, Settings{Configured: true, Look: "arcane", Background: BackgroundLook, Move: true}, false},
		{"move kept over a picture", Settings{Background: BackgroundPicture, Picture: "2026/10/abc.jpg", Move: true},
			Settings{Configured: true, Background: BackgroundPicture, Picture: "2026/10/abc.jpg", Move: true}, false},
		{"move dropped over a plain background", Settings{Background: BackgroundPlain, Move: true}, Settings{Configured: true, Background: BackgroundPlain}, false},
		{"move dropped when no background is chosen", Settings{Move: true}, Settings{Configured: true, Background: BackgroundPlain}, false},
		{"favicon flag kept", Settings{LogoAsFavicon: true, Logo: "2026/10/l.webp"}, Settings{Configured: true, LogoAsFavicon: true, Logo: "2026/10/l.webp", Background: BackgroundPlain}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Validate(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPictureName(t *testing.T) {
	tests := []struct {
		in    string
		valid bool
	}{
		{"", true},
		{"2026/09/3f2a.png", true},
		{"2026/09/3f2a.JPG", true},
		{"2026/09/3f2a.webp", true},
		{"2026/09/3f2a.gif", false},
		{"2026/09/3f2a.svg", false},
		{"2026/09/3f2a", false},
		{"/2026/09/a.png", false},
		{"2026//a.png", false},
		{"2026/.hidden/a.png", false},
		{"..\\a.png", false},
		{"a b.png", false},
		{"a\".png", false},
		{strings.Repeat("a", 256) + ".png", false},
	}
	for _, tc := range tests {
		_, err := PictureName("logo", tc.in)
		if (err == nil) != tc.valid {
			t.Errorf("PictureName(%q): err=%v, want valid=%v", tc.in, err, tc.valid)
		}
	}
}

func TestDisplayNameAndInitial(t *testing.T) {
	tests := []struct{ name, display, initial string }{
		{"", "Chronicle", "C"},
		{"dragon hold", "dragon hold", "D"},
		{"élan", "élan", "É"},
	}
	for _, tc := range tests {
		s := Settings{Name: tc.name}
		if s.DisplayName() != tc.display || s.Initial() != tc.initial {
			t.Errorf("%q: got %q/%q, want %q/%q", tc.name, s.DisplayName(), s.Initial(), tc.display, tc.initial)
		}
	}
}

func TestLooksAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range Looks {
		if seen[l.ID] {
			t.Errorf("duplicate look %q", l.ID)
		}
		seen[l.ID] = true
		for _, c := range []string{l.Accent, l.HeaderFrom, l.HeaderTo} {
			if len(c) != 7 || c[0] != '#' {
				t.Errorf("look %q has colour %q that is not #RRGGBB", l.ID, c)
			}
		}
	}
	if len(Looks) != 8 {
		t.Errorf("got %d looks, want 8", len(Looks))
	}
}

func encodePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.White)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func encodeJPEG(w, h int) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil)
	return b.Bytes()
}

func TestCheckImage(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><script>alert(1)</script></svg>`)
	big := append(encodePNG(64, 64), make([]byte, LogoMaxBytes)...)
	tests := []struct {
		name     string
		kind     string
		data     []byte
		wantMIME string
		wantErr  bool
	}{
		{"square png logo", KindLogo, encodePNG(64, 64), "image/png", false},
		{"squarish jpeg logo", KindLogo, encodeJPEG(100, 90), "image/jpeg", false},
		{"wide logo refused", KindLogo, encodePNG(200, 64), "", true},
		{"tall logo refused", KindLogo, encodePNG(64, 200), "", true},
		{"svg logo refused", KindLogo, svg, "", true},
		{"html disguised as png refused", KindLogo, []byte("<html><script>x</script></html>"), "", true},
		{"gif refused", KindLogo, []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), "", true},
		{"oversize logo refused", KindLogo, big, "", true},
		{"empty refused", KindLogo, nil, "", true},
		{"wide picture", KindPicture, encodeJPEG(300, 120), "image/jpeg", false},
		{"square picture refused", KindPicture, encodePNG(100, 100), "", true},
		{"tall picture refused", KindPicture, encodePNG(100, 200), "", true},
		{"unknown kind", "banner", encodePNG(64, 64), "", true},
		{"truncated png refused", KindLogo, encodePNG(64, 64)[:20], "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mime, err := CheckImage(tc.kind, tc.data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if mime != tc.wantMIME {
				t.Errorf("mime = %q, want %q", mime, tc.wantMIME)
			}
		})
	}
}
