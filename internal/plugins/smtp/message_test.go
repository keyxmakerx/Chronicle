package smtp

import (
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	longHTML := "<p>" + strings.Repeat("a", 2000) + " 🎲</p>"
	tests := []struct {
		name      string
		subject   string
		plain     string
		html      string
		wantParts []string
	}{
		{"plain only, ascii", "Hello", "line one\nline two", "", []string{"line one\r\nline two"}},
		{"plain only, em dash", "Password Reset — Chronicle", "reset — now", "", []string{"reset — now"}},
		{"html with emoji and a long line", "You're invited 🎲", "Going ✓", longHTML, []string{"Going ✓", longHTML}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			from := mail.Address{Name: "Chrönicle", Address: "chronicle@example.com"}
			raw, err := buildMessage(from, []string{"a@example.com"}, tc.subject, tc.plain, tc.html, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(raw, "\r\n") {
				if len(line) > 998 {
					t.Fatalf("line %d is %d bytes", i, len(line))
				}
				for _, r := range line {
					if r > 127 {
						t.Fatalf("line %d has non-ASCII %q", i, line)
					}
				}
			}

			msg, err := mail.ReadMessage(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			dec := new(mime.WordDecoder)
			if got, _ := dec.DecodeHeader(msg.Header.Get("Subject")); got != tc.subject {
				t.Errorf("subject = %q, want %q", got, tc.subject)
			}
			if !strings.HasSuffix(msg.Header.Get("Message-ID"), "@example.com>") {
				t.Errorf("Message-ID = %q", msg.Header.Get("Message-ID"))
			}

			var bodies []string
			mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
			if mediaType == "multipart/alternative" {
				mr := multipart.NewReader(msg.Body, params["boundary"])
				for {
					p, err := mr.NextPart() // decodes quoted-printable itself
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					b, _ := io.ReadAll(p)
					bodies = append(bodies, strings.TrimSuffix(string(b), "\r\n"))
				}
			} else {
				b, _ := io.ReadAll(quotedprintable.NewReader(msg.Body))
				bodies = append(bodies, string(b))
			}
			if len(bodies) != len(tc.wantParts) {
				t.Fatalf("got %d parts, want %d", len(bodies), len(tc.wantParts))
			}
			for i := range bodies {
				if bodies[i] != tc.wantParts[i] {
					t.Errorf("part %d = %q, want %q", i, bodies[i], tc.wantParts[i])
				}
			}
		})
	}
}
