package designs

import (
	_ "embed"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// galleryTermsText is the editable maturity list (gallery_terms.txt). It is
// embedded so the filter works from the compiled binary.
//
//go:embed gallery_terms.txt
var galleryTermsText string

// barrier replaces an allowlisted word in the token stream: it matches
// nothing and stops words either side of it from being glued together.
const barrier = "|"

// maturityList is the parsed word list and allowlist.
type maturityList struct {
	phrases []string // lowercase terms, words separated by one space
	// multi are the phrases of two or more words; they are also tried before
	// the allowlist blanks a word, so "dick pic" is caught though "dick" alone
	// is a name.
	multi []string
	// Terms with no doubled letter can also be tested against text whose every
	// repeated letter was squashed to one ("fuuuck"); a term with a double
	// ("pussy") would be damaged by that squash and is left out of it.
	squashPhrases []string
	allow         map[string]bool
}

// parseMaturity reads the list file: "[terms]" and "[allow]" sections, one
// entry per line, "#" for notes.
func parseMaturity(text string) *maturityList {
	l := &maturityList{allow: map[string]bool{}}
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			section = strings.Trim(line, "[]")
		case section == "allow":
			l.allow[line] = true
		case section == "terms":
			l.phrases = append(l.phrases, line)
			flat := strings.ReplaceAll(line, " ", "")
			if strings.Contains(line, " ") {
				l.multi = append(l.multi, line)
			}
			if !hasDoubleLetter(flat) {
				l.squashPhrases = append(l.squashPhrases, line)
			}
		}
	}
	return l
}

func hasDoubleLetter(s string) bool {
	var prev rune
	for _, r := range s {
		if r == prev {
			return true
		}
		prev = r
	}
	return false
}

// leetMap undoes the usual letter-for-symbol swaps.
var leetMap = map[rune]rune{'0': 'o', '1': 'i', '3': 'e', '4': 'a', '5': 's', '7': 't', '@': 'a', '$': 's'}

// foldText lowercases, strips accents, undoes leetspeak and turns everything
// that is not a letter into a space, so the rest of the filter only ever sees
// words made of letters. With joinPunct, punctuation is dropped instead, so
// "s.e.x" and "fu-ck" read as one word; real spaces still separate words.
func foldText(s string, joinPunct bool) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if m, ok := leetMap[r]; ok {
			r = m
		}
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(r)
		case joinPunct && !unicode.IsSpace(r):
		default:
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// collapseRuns shortens a run of one repeated letter of minRun or more to keep
// letters. Spaces are never collapsed.
func collapseRuns(s string, minRun, keep int) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); {
		j := i
		for j < len(rs) && rs[j] == rs[i] {
			j++
		}
		n := j - i
		if rs[i] != ' ' && n >= minRun {
			n = keep
		}
		for k := 0; k < n; k++ {
			b.WriteRune(rs[i])
		}
		i = j
	}
	return b.String()
}

// mergeSingles glues runs of three or more one-letter words ("s e x") into one
// word; two is left alone because "a" and "i" are real words.
func mergeSingles(tokens []string) []string {
	var out []string
	for i := 0; i < len(tokens); {
		j := i
		for j < len(tokens) && len([]rune(tokens[j])) == 1 && tokens[j] != barrier {
			j++
		}
		if j-i >= 3 {
			out = append(out, strings.Join(tokens[i:j], ""))
			i = j
			continue
		}
		out = append(out, tokens[i])
		i++
	}
	return out
}

// hits reports whether tokens contain a listed phrase as whole words.
func hits(tokens, phrases []string) bool {
	padded := " " + strings.Join(tokens, " ") + " "
	for _, p := range phrases {
		if strings.Contains(padded, " "+p+" ") {
			return true
		}
	}
	return false
}

// flags reports whether text contains a listed term after normalising. It
// reads the text three ways: words split on any separator; with runs of single
// letters merged ("s e x"); and with punctuation dropped ("s.e.x"). Terms are
// only ever matched as whole words, never inside a longer one, and never across
// a plain space between whole words ("open island" does not read as a term), so
// the list cannot hit the Scunthorpe problem whatever the allowlist holds.
func (l *maturityList) flags(text string) bool {
	for _, joinPunct := range []bool{false, true} {
		folded := foldText(text, joinPunct)
		for _, v := range []struct {
			s       string
			phrases []string
		}{
			{collapseRuns(folded, 3, 2), l.phrases},
			{collapseRuns(folded, 2, 1), l.squashPhrases},
		} {
			raw := strings.Fields(v.s)
			if hits(raw, l.multi) {
				return true
			}
			tokens := append([]string{}, raw...)
			for i, t := range tokens {
				if l.allow[t] {
					tokens[i] = barrier
				}
			}
			if hits(tokens, v.phrases) || hits(mergeSingles(tokens), v.phrases) {
				return true
			}
		}
	}
	return false
}

// defaultMaturity is the list the server ships with.
var defaultMaturity = parseMaturity(galleryTermsText)
