package sanitize

import "testing"

func TestCountSecrets(t *testing.T) {
	tests := []struct {
		name                         string
		in                           string
		passages, pictures, diagrams int
	}{
		{"empty", "", 0, 0, 0},
		{"plain page", "<p>Hello <strong>world</strong></p>", 0, 0, 0},
		{"one passage", `<p>A <span data-secret="true">hidden</span> B</p>`, 1, 0, 0},
		{"two passages across lines", "<p><span data-secret=\"true\">a\nb</span></p><p><span class=\"x\" data-secret=\"true\">c</span></p>", 2, 0, 0},
		{"gm picture", `<figure class="ce-img ce-img--gm"><img src="/m/a.png"><figcaption>c</figcaption></figure>`, 0, 1, 0},
		{"player picture not counted", `<figure class="ce-img"><img src="/m/a.png"></figure>`, 0, 0, 0},
		{"gm diagram", `<pre class="ce-diagram ce-diagram--gm">graph TD; A--&gt;B</pre>`, 0, 0, 1},
		{"player diagram not counted", `<pre class="ce-diagram">graph TD; A--&gt;B</pre>`, 0, 0, 0},
		{"roller not counted", `<div class="ce-roll" data-table="t"></div>`, 0, 0, 0},
		{
			"mixed",
			`<span data-secret="true">x</span><figure class="ce-img--gm"><img src="a"></figure><pre class="ce-diagram--gm">g</pre><span data-secret="true">y</span><div class="ce-roll"></div>`,
			2, 1, 1,
		},
		{"secret inside gm picture counted once", `<figure class="ce-img--gm"><figcaption><span data-secret="true">x</span></figcaption></figure>`, 0, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, pic, d := CountSecrets(tt.in)
			if p != tt.passages || pic != tt.pictures || d != tt.diagrams {
				t.Errorf("CountSecrets() = (%d,%d,%d), want (%d,%d,%d)", p, pic, d, tt.passages, tt.pictures, tt.diagrams)
			}
		})
	}
}
