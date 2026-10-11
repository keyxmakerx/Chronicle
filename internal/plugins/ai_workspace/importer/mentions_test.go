package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

const (
	idLyra  = "11111111-1111-4111-8111-111111111111"
	idHid   = "22222222-2222-4222-8222-222222222222"
	idSaved = "33333333-3333-4333-8333-333333333333"
)

func linkFixture() (*PageLinks, *fakeCreator) {
	fc := &fakeCreator{
		existing: map[string]*entities.Entity{
			"lyra":   {ID: idLyra, CampaignID: "camp", Name: "Lyra", Slug: "lyra"},
			"secret": {ID: idHid, CampaignID: "camp", Name: "Secret", Slug: "secret"},
		},
		hidden: map[string]bool{idHid: true},
	}
	return NewPageLinks(context.Background(), fc, "camp", Viewer{Role: 1, UserID: "u"}), fc
}

func TestMarkdownToHTML_PageLinks(t *testing.T) {
	anchor := `<a data-mention-id="` + idLyra + `" href="/campaigns/camp/entities/` + idLyra +
		`" data-entity-preview="/campaigns/camp/entities/` + idLyra + `/preview"`
	tests := []struct {
		name     string
		in       string
		want     []string // substrings that must appear
		wantNot  []string
		noOption bool
	}{
		{"plain name", "See @[Lyra] now.", []string{anchor, ">@Lyra</a>"}, nil, false},
		{"slug match, other case", "See @[lyra].", []string{anchor, ">@Lyra</a>"}, nil, false},
		{"shown text", "Ask @[Lyra|the bard].", []string{anchor, ">the bard</a>"}, []string{"@the bard"}, false},
		{"unknown stays plain", "Meet @[Nobody].", []string{"Meet Nobody."}, []string{"@[", "<a"}, false},
		{"unknown with text", "Meet @[Nobody|a stranger].", []string{"Meet a stranger."}, []string{"<a"}, false},
		{"hidden page stays plain", "See @[Secret].", []string{"See Secret."}, []string{"<a", idHid}, false},
		{"inline code literal", "Write `@[Lyra]` to link.", []string{"<code>@[Lyra]</code>"}, []string{"<a"}, false},
		{"code block literal", "```\n@[Lyra]\n```", []string{"@[Lyra]"}, []string{"<a"}, false},
		{"inside an existing link", "[go @[Lyra]](https://example.com)", []string{"go @[Lyra]"}, []string{"data-mention-id"}, false},
		{"text escaped once", "@[Lyra|a & b]", []string{">a &amp; b</a>"}, []string{"&amp;amp;"}, false},
		{"without option literal", "See @[Lyra].", []string{"@[Lyra]"}, []string{"<a"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := linkFixture()
			var got string
			var err error
			if tc.noOption {
				got, err = MarkdownToHTML(tc.in)
			} else {
				got, err = MarkdownToHTML(tc.in, WithPageLinks(p))
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("output %q missing %q", got, w)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(got, w) {
					t.Errorf("output %q should not contain %q", got, w)
				}
			}
		})
	}
}

func TestPageLinks_SavedPageWinsAndNamesRoundTrip(t *testing.T) {
	p, _ := linkFixture()
	p.Add(idSaved, "Old Mill (Imported)", "Old Mill")
	got, _ := MarkdownToHTML("@[Old Mill]", WithPageLinks(p))
	if !strings.Contains(got, `data-mention-id="`+idSaved+`"`) || !strings.Contains(got, ">@Old Mill (Imported)</a>") {
		t.Errorf("got %q", got)
	}
}

func TestLinkNames_And_Warnings(t *testing.T) {
	md := "@[Lyra] and @[Ghost|g] and @[Later] and `@[InCode]` and @[Ghost]"
	names := LinkNames(md)
	if strings.Join(names, ",") != "Ghost,Later,Lyra" {
		t.Errorf("names = %v", names)
	}
	p, _ := linkFixture()
	warns := LinkWarnings(p, md, map[string]bool{"later": true})
	if len(warns) != 1 || !strings.Contains(warns[0], "@[Ghost]") {
		t.Errorf("warnings = %v", warns)
	}
}
