package playerchanges

import (
	"fmt"
	"hash/fnv"
	"html"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// monthsLabel names the long range: "6 months", "1 year".
func monthsLabel(m int) string {
	switch m {
	case 12:
		return "1 year"
	case 24:
		return "2 years"
	case 1:
		return "1 month"
	}
	return fmt.Sprintf("%d months", m)
}

// countLine says how many changes the answer holds and over what time.
func countLine(v ResultsView) string {
	n := "1 change"
	if v.Count != 1 {
		n = fmt.Sprintf("%d changes", v.Count)
	}
	if v.Range == RangeLong {
		return n + " in the last " + monthsLabel(v.KeepMonths)
	}
	return n + " this month"
}

// dayLine is when a change happened in the owner's zone: "Today, 9:42 pm",
// "Yesterday, 10:02 pm", "2 August, 9:10 pm", with the year once it is not
// this one.
func dayLine(t time.Time, zone *time.Location, now time.Time) string {
	lt, ln := t.In(zone), now.In(zone)
	clock := strings.ToLower(lt.Format("3:04 pm"))
	y1, m1, d1 := lt.Date()
	y2, m2, d2 := ln.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "Today, " + clock
	case lt.Add(24*time.Hour).Format("2006-01-02") == ln.Format("2006-01-02"):
		return "Yesterday, " + clock
	case y1 == y2:
		return lt.Format("2 January") + ", " + clock
	}
	return lt.Format("2 January 2006") + ", " + clock
}

// byLine says who made the change: the player, or their AI by app name.
func byLine(c Change) string {
	who := c.UserName
	if who == "" {
		who = "A player who has left"
	}
	if c.ByAI() {
		if c.App != "" {
			return who + "'s AI (" + c.App + ")"
		}
		return who + "'s AI"
	}
	return who
}

func blankAs(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

// initial is the first letter of a page's name, for its badge.
func initial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return "?"
}

// avatarHues are the badge colours, picked by page id so a page keeps its
// colour from search to search.
var avatarHues = []int{239, 199, 32, 152, 280, 345, 96, 12}

func avatarStyle(id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return fmt.Sprintf("background:hsl(%d 60%% 45%%)", avatarHues[int(h.Sum32())%len(avatarHues)])
}

// mark escapes s and wraps each typed word it holds in <mark>, so a search
// shows where it matched. Matching ignores case.
func mark(s string, words []string) string {
	if len(words) == 0 || s == "" {
		return html.EscapeString(s)
	}
	lower := strings.ToLower(s)
	if len(lower) != len(s) {
		// Lower-casing changed byte lengths, so positions would not line up.
		return html.EscapeString(s)
	}
	hit := make([]bool, len(s))
	for _, w := range words {
		w = strings.ToLower(w)
		if w == "" {
			continue
		}
		for i := 0; ; {
			j := strings.Index(lower[i:], w)
			if j < 0 {
				break
			}
			for k := i + j; k < i+j+len(w) && k < len(hit); k++ {
				hit[k] = true
			}
			i += j + len(w)
		}
	}
	var b strings.Builder
	in := false
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		if hit[i] != in {
			if in {
				b.WriteString("</mark>")
			} else {
				b.WriteString("<mark>")
			}
			in = hit[i]
		}
		b.WriteString(html.EscapeString(s[i : i+size]))
		i += size
	}
	if in {
		b.WriteString("</mark>")
	}
	return b.String()
}
