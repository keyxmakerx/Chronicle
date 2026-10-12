package pageblocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits. They bound the stored document (and so a request body) and keep the
// widget's layout sane; the widget enforces the same numbers as a courtesy.
const (
	MaxReadAloudChars = 4000
	MaxColumns        = 8
	MaxRows           = 60
	MaxLabelChars     = 80
	MaxCellChars      = 500
	MaxSteps          = 30
	MaxStepTitleChars = 120
	MaxStepTextChars  = 1000
	MaxIDChars        = 64

	// MaxBodyBytes is above the worst legal table (60 rows x 8 cells x 500
	// four-byte characters) so a legal save is never refused, and low enough
	// that a junk body is.
	MaxBodyBytes = 1 << 20
)

// validKind reports whether kind is one this plugin stores.
func validKind(kind string) bool {
	switch kind {
	case KindReadAloud, KindTable, KindSteps:
		return true
	}
	return false
}

// validToken reports whether s is a short safe id: letters, digits, _ and -.
// Layout block ids and the ids the editor makes both fit; anything else is
// refused so an id can never carry markup or path characters.
func validToken(s string) bool {
	if s == "" || len(s) > MaxIDChars {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// cleanText checks plain text against a character limit. multiline allows
// line breaks and tabs; every other control character is refused, so stored
// text is always safe to show with textContent and to log.
func cleanText(field, s string, max int, multiline bool) (string, error) {
	if !utf8.ValidString(s) {
		return "", errInvalid(field + " is not valid text")
	}
	if utf8.RuneCountInString(s) > max {
		return "", errInvalid(fmt.Sprintf("%s is too long (at most %d characters)", field, max))
	}
	for _, r := range s {
		if !unicode.IsControl(r) {
			continue
		}
		if multiline && (r == '\n' || r == '\t') {
			continue
		}
		return "", errInvalid(field + " contains a character that is not allowed")
	}
	return s, nil
}

// normalizeNewlines turns CRLF into LF so a Windows paste is stored the same as a
// typed line break.
func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// decodeStrict unmarshals raw into v and refuses unknown fields, so a typo or
// a stale client fails loudly instead of silently dropping data.
func decodeStrict(raw json.RawMessage, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errInvalid("data is required")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errInvalid("data is not in the right shape for this block")
	}
	return nil
}

// emptyDoc is what a block with no saved row reads as.
func emptyDoc(kind string) any {
	switch kind {
	case KindReadAloud:
		return ReadAloud{}
	case KindTable:
		return Table{Columns: []Column{}, Rows: []Row{}}
	case KindSteps:
		return Steps{Steps: []Step{}}
	}
	return nil
}

// validateDoc checks raw as a document of kind and returns the cleaned value.
func validateDoc(kind string, raw json.RawMessage) (any, error) {
	switch kind {
	case KindReadAloud:
		var d ReadAloud
		if err := decodeStrict(raw, &d); err != nil {
			return nil, err
		}
		t, err := cleanText("text", normalizeNewlines(d.Text), MaxReadAloudChars, true)
		if err != nil {
			return nil, err
		}
		return ReadAloud{Text: t}, nil
	case KindTable:
		return validateTable(raw)
	case KindSteps:
		return validateSteps(raw)
	}
	return nil, errInvalid("unknown block kind")
}

func validateTable(raw json.RawMessage) (Table, error) {
	var d Table
	if err := decodeStrict(raw, &d); err != nil {
		return Table{}, err
	}
	if len(d.Columns) < 1 || len(d.Columns) > MaxColumns {
		return Table{}, errInvalid(fmt.Sprintf("a table needs 1 to %d columns", MaxColumns))
	}
	if len(d.Rows) > MaxRows {
		return Table{}, errInvalid(fmt.Sprintf("a table holds at most %d rows", MaxRows))
	}
	colIDs := make(map[string]bool, len(d.Columns))
	cols := make([]Column, 0, len(d.Columns))
	for _, c := range d.Columns {
		if !validToken(c.ID) || colIDs[c.ID] {
			return Table{}, errInvalid("column ids must be short, unique and plain")
		}
		colIDs[c.ID] = true
		label, err := cleanText("column name", c.Label, MaxLabelChars, false)
		if err != nil {
			return Table{}, err
		}
		cols = append(cols, Column{ID: c.ID, Label: label, DMOnly: c.DMOnly})
	}
	rowIDs := make(map[string]bool, len(d.Rows))
	rows := make([]Row, 0, len(d.Rows))
	for _, r := range d.Rows {
		if !validToken(r.ID) || rowIDs[r.ID] {
			return Table{}, errInvalid("row ids must be short, unique and plain")
		}
		rowIDs[r.ID] = true
		cells := make(map[string]string, len(r.Cells))
		for id, text := range r.Cells {
			// A cell of a column that is gone is dropped, not an error: the
			// editor may still hold it after a column was removed.
			if !colIDs[id] {
				continue
			}
			t, err := cleanText("cell", normalizeNewlines(text), MaxCellChars, true)
			if err != nil {
				return Table{}, err
			}
			if t != "" {
				cells[id] = t
			}
		}
		rows = append(rows, Row{ID: r.ID, Cells: cells})
	}
	return Table{Columns: cols, Rows: rows}, nil
}

func validateSteps(raw json.RawMessage) (Steps, error) {
	var d Steps
	if err := decodeStrict(raw, &d); err != nil {
		return Steps{}, err
	}
	if len(d.Steps) > MaxSteps {
		return Steps{}, errInvalid(fmt.Sprintf("a list holds at most %d steps", MaxSteps))
	}
	seen := make(map[string]bool, len(d.Steps))
	out := make([]Step, 0, len(d.Steps))
	for _, s := range d.Steps {
		if !validToken(s.ID) || seen[s.ID] {
			return Steps{}, errInvalid("step ids must be short, unique and plain")
		}
		seen[s.ID] = true
		title, err := cleanText("step title", s.Title, MaxStepTitleChars, false)
		if err != nil {
			return Steps{}, err
		}
		text, err := cleanText("step text", normalizeNewlines(s.Text), MaxStepTextChars, true)
		if err != nil {
			return Steps{}, err
		}
		out = append(out, Step{ID: s.ID, Title: title, Text: text, Done: s.Done})
	}
	return Steps{Steps: out}, nil
}

// hasDMOnly reports whether the table holds a DM-only column.
func hasDMOnly(t Table) bool {
	for _, c := range t.Columns {
		if c.DMOnly {
			return true
		}
	}
	return false
}

// stripDMOnly returns the table as a viewer without DM visibility must see
// it: DM-only columns and their cells removed. It copies, so the stored
// document is never altered.
func stripDMOnly(t Table) Table {
	if !hasDMOnly(t) {
		return t
	}
	hidden := map[string]bool{}
	cols := make([]Column, 0, len(t.Columns))
	for _, c := range t.Columns {
		if c.DMOnly {
			hidden[c.ID] = true
			continue
		}
		cols = append(cols, c)
	}
	rows := make([]Row, 0, len(t.Rows))
	for _, r := range t.Rows {
		cells := make(map[string]string, len(r.Cells))
		for id, text := range r.Cells {
			if !hidden[id] {
				cells[id] = text
			}
		}
		rows = append(rows, Row{ID: r.ID, Cells: cells})
	}
	return Table{Columns: cols, Rows: rows}
}
