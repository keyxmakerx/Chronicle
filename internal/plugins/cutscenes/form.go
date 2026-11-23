package cutscenes

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// The editor posts one plain form. A list layer (on top, words) is a run of
// same-named fields, one value per row in document order, so a row has no
// checkbox (an unticked box sends nothing and would shift every row after
// it). A row left blank (no kind, no words) is the editor's spare row and is
// skipped, never stored.
const (
	fName      = "name"
	fBgKind    = "bg_kind"
	fBgSeconds = "bg_seconds"
	fBgLook    = "bg_look"
	fOvKind    = "ov_kind"
	fOvAt      = "ov_at"
	fOvSeconds = "ov_seconds"
	fOvColor   = "ov_color"
	fOvLook    = "ov_look"
	fTxSay     = "tx_say"
	fTxAt      = "tx_at"
	fTxSeconds = "tx_seconds"
	fTxStyle   = "tx_style"
	fEndKind   = "end_kind"
	fEndAfter  = "end_after"
)

// SceneFromForm reads the editor's form into a scene. It only reads: every
// rule (kinds, ranges, limits) is Normalize's, applied by the service. A
// number that is not a number is refused here, because "abc seconds" has no
// clamped meaning.
func SceneFromForm(f url.Values) (Scene, error) {
	var s Scene
	var err error
	s.Name = f.Get(fName)
	s.Background.Kind = f.Get(fBgKind)
	s.Background.Params.Look = f.Get(fBgLook)
	if s.Background.Seconds, err = number(f.Get(fBgSeconds), "the background's time"); err != nil {
		return s, err
	}
	kinds, ats, secs, colors, looks := f[fOvKind], f[fOvAt], f[fOvSeconds], f[fOvColor], f[fOvLook]
	for i, k := range kinds {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		o := Overlay{Kind: k, Params: Params{Color: nth(colors, i), Look: nth(looks, i)}}
		if o.At, err = number(nth(ats, i), fmt.Sprintf("layer on top %d's start", len(s.Overlays)+1)); err != nil {
			return s, err
		}
		if o.Seconds, err = number(nth(secs, i), fmt.Sprintf("layer on top %d's time", len(s.Overlays)+1)); err != nil {
			return s, err
		}
		s.Overlays = append(s.Overlays, o)
	}
	says, tats, tsecs, styles := f[fTxSay], f[fTxAt], f[fTxSeconds], f[fTxStyle]
	for i, say := range says {
		if strings.TrimSpace(say) == "" {
			continue
		}
		t := TextLine{Say: say, Style: nth(styles, i)}
		if t.At, err = number(nth(tats, i), fmt.Sprintf("words %d's start", len(s.Text)+1)); err != nil {
			return s, err
		}
		if t.Seconds, err = number(nth(tsecs, i), fmt.Sprintf("words %d's time on screen", len(s.Text)+1)); err != nil {
			return s, err
		}
		s.Text = append(s.Text, t)
	}
	s.Ending.Kind = f.Get(fEndKind)
	if s.Ending.Kind == EndingAuto {
		after, err := number(f.Get(fEndAfter), "the ending's wait")
		if err != nil {
			return s, err
		}
		s.Ending.After = &after
	}
	return s, nil
}

func nth(v []string, i int) string {
	if i < len(v) {
		return v[i]
	}
	return ""
}

// number reads seconds as typed; empty is 0, which Normalize reads as "not
// set" where that has a meaning.
func number(s, what string) (float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, apperror.NewBadRequest(fmt.Sprintf("%s must be a number of seconds", what))
	}
	return f, nil
}

// ShowFromForm reads the Show sheet: to (repeated) and rewatch.
func ShowFromForm(f url.Values) ShowInput {
	return ShowInput{To: f["to"], Rewatch: f.Get("rewatch")}
}
