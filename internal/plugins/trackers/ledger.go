package trackers

// The value of a tracker is never stored. It is worked out here from the log,
// in order, so that striking a line changes the value as if the line had
// never been written, and the log can show a running balance.

// bounds returns the lowest and highest value a kind allows. hasMax is false
// for a count with no top.
func bounds(k Kind) (lo int, hi int, hasMax bool) {
	switch k.Shape {
	case ShapeSteps:
		return 0, len(k.Steps) - 1, true
	case ShapeClock:
		if k.Max != nil {
			return 0, *k.Max, true
		}
		return 0, 0, true
	}
	if k.Min != nil {
		lo = *k.Min
	}
	if k.Max != nil {
		return lo, *k.Max, true
	}
	return lo, 0, false
}

// clamp keeps v inside the kind's bounds.
func clamp(k Kind, v int) int {
	lo, hi, hasMax := bounds(k)
	// A count with no Min has no floor (Kind.Min nil is unbounded, and the
	// widget lets the step-down through); bounds' 0 is only where it starts.
	hasMin := k.Shape != ShapeCount || k.Min != nil
	if hasMin && v < lo {
		return lo
	}
	if hasMax && v > hi {
		return hi
	}
	return v
}

// startValue is the value of a tracker with no log: the lowest it allows.
func startValue(k Kind) int {
	lo, _, _ := bounds(k)
	return clamp(k, lo)
}

// apply is one line's effect. Each line is clamped as it lands, so a step
// back at the bottom is lost rather than owed: the log reads as the table
// saw it.
func apply(k Kind, v int, e Entry) int {
	switch e.Op {
	case OpSet:
		return clamp(k, e.Amount)
	default:
		return clamp(k, v+e.Amount)
	}
}

// fold returns the value after entries (oldest first) and the value right
// after each line. A struck line's "after" is the value it left unchanged.
func fold(k Kind, entries []Entry) (int, []int) {
	v := startValue(k)
	after := make([]int, len(entries))
	for i, e := range entries {
		if e.StruckAt == nil {
			v = apply(k, v, e)
		}
		after[i] = v
	}
	return v, after
}

// nowText is what value v means: the step's text (or its name when it has
// none) for steps, else the highest band at or below v.
func nowText(k Kind, v int) string {
	if k.Shape == ShapeSteps {
		if v < 0 || v >= len(k.Steps) {
			return ""
		}
		if k.Steps[v].Text != "" {
			return k.Steps[v].Text
		}
		return k.Steps[v].Name
	}
	text, best, found := "", 0, false
	for _, b := range k.Bands {
		if b.At <= v && (!found || b.At >= best) {
			text, best, found = b.Text, b.At, true
		}
	}
	return text
}
