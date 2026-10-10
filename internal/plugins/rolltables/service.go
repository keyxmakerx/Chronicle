package rolltables

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Limits keep one campaign's document bounded; it is a DM's working set of
// tables, not a content store.
const (
	MaxBodyBytes     = 512 * 1024
	MaxTables        = 100
	MaxTableNameLen  = 120
	MaxEntries       = 500
	MaxEntryNameLen  = 200
	MaxEntryBriefLen = 1000
	MaxWeight        = 100
	MaxCreditField   = 200
	MaxCreditNotice  = 600
	MaxCreditURL     = 300
	defaultWeight    = 1.0
)

// idPattern keeps table ids stable, URL-safe keys so a client can address a
// table without worrying about case or separators.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Service is the business-logic boundary for roll tables.
type Service interface {
	// Get returns the campaign's document; a campaign with no row reads as an
	// empty set of tables.
	Get(ctx context.Context, campaignID string) (Document, error)
	// Put validates the raw body, replaces the whole document with its
	// normalized form and returns what was stored.
	Put(ctx context.Context, campaignID string, body []byte, userID string) (Document, error)
}

type service struct {
	repo Repository
}

// NewService builds the service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Get(ctx context.Context, campaignID string) (Document, error) {
	data, err := s.repo.Get(ctx, campaignID)
	if err != nil {
		return Document{}, apperrorInternal(err)
	}
	if data == nil {
		return emptyDocument(), nil
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, apperrorInternal(err)
	}
	if doc.Tables == nil {
		doc.Tables = []Table{}
	}
	return doc, nil
}

func (s *service) Put(ctx context.Context, campaignID string, body []byte, userID string) (Document, error) {
	doc, err := normalize(body)
	if err != nil {
		return Document{}, err
	}
	// Store the re-marshalled document, never the raw body, so unknown fields
	// and odd formatting cannot reach the database.
	data, err := json.Marshal(doc)
	if err != nil {
		return Document{}, apperrorInternal(err)
	}
	if err := s.repo.Put(ctx, campaignID, data, userID); err != nil {
		return Document{}, apperrorInternal(err)
	}
	return doc, nil
}

// Wire shapes for decoding. Pointers distinguish "absent" from a zero value
// so a missing table list is rejected (it would otherwise wipe the set) and a
// missing weight takes the default. Unknown fields are ignored here, which is
// what drops them.
type inDocument struct {
	Tables *[]inTable `json:"tables"`
}

type inTable struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Credit  *Credit   `json:"credit"`
	Entries []inEntry `json:"entries"`
}

type inEntry struct {
	Name   string   `json:"name"`
	Brief  string   `json:"brief"`
	Weight *float64 `json:"weight"`
}

// normalize validates the body and returns the canonical document.
func normalize(body []byte) (Document, error) {
	if len(body) > MaxBodyBytes {
		return Document{}, errInvalid(fmt.Sprintf("the tables are too large; keep them under %d KiB", MaxBodyBytes/1024))
	}
	var in inDocument
	if err := json.Unmarshal(bytes.TrimSpace(body), &in); err != nil {
		return Document{}, errInvalid("the body must be a JSON object like {\"tables\":[...]}")
	}
	if in.Tables == nil {
		return Document{}, errInvalid("the body must include a \"tables\" list (it may be empty)")
	}
	if len(*in.Tables) > MaxTables {
		return Document{}, errInvalid(fmt.Sprintf("a campaign can have at most %d tables", MaxTables))
	}

	out := Document{Tables: make([]Table, 0, len(*in.Tables))}
	seen := make(map[string]bool, len(*in.Tables))
	for i, t := range *in.Tables {
		label := fmt.Sprintf("table %d", i+1)
		if !idPattern.MatchString(t.ID) {
			return Document{}, errInvalid(label + ": the id must start with a lowercase letter and use only lowercase letters, digits and dashes (up to 64 characters)")
		}
		if seen[t.ID] {
			return Document{}, errInvalid(label + ": the id \"" + t.ID + "\" is used more than once")
		}
		seen[t.ID] = true

		name := strings.TrimSpace(t.Name)
		if n := utf8.RuneCountInString(name); n < 1 || n > MaxTableNameLen {
			return Document{}, errInvalid(fmt.Sprintf("%s: the name must be 1 to %d characters", label, MaxTableNameLen))
		}
		if len(t.Entries) < 1 || len(t.Entries) > MaxEntries {
			return Document{}, errInvalid(fmt.Sprintf("%s: a table needs 1 to %d entries", label, MaxEntries))
		}

		entries := make([]Entry, 0, len(t.Entries))
		for j, e := range t.Entries {
			elabel := fmt.Sprintf("%s, entry %d", label, j+1)
			ename := strings.TrimSpace(e.Name)
			if n := utf8.RuneCountInString(ename); n < 1 || n > MaxEntryNameLen {
				return Document{}, errInvalid(fmt.Sprintf("%s: the name must be 1 to %d characters", elabel, MaxEntryNameLen))
			}
			if utf8.RuneCountInString(e.Brief) > MaxEntryBriefLen {
				return Document{}, errInvalid(fmt.Sprintf("%s: the brief can be at most %d characters", elabel, MaxEntryBriefLen))
			}
			w := defaultWeight
			if e.Weight != nil {
				w = *e.Weight
			}
			if math.IsNaN(w) || w < 0 || w > MaxWeight {
				return Document{}, errInvalid(fmt.Sprintf("%s: the weight must be a number from 0 to %d", elabel, MaxWeight))
			}
			entries = append(entries, Entry{Name: ename, Brief: e.Brief, Weight: w})
		}
		credit, err := normalizeCredit(t.Credit, label)
		if err != nil {
			return Document{}, err
		}
		out.Tables = append(out.Tables, Table{ID: t.ID, Name: name, Credit: credit, Entries: entries})
	}
	return out, nil
}

// normalizeCredit trims a table's credit and checks its links. A credit with no
// source is dropped, since a credit line can't name nothing. Links must be
// https so a stored credit can never become a script link when shown.
func normalizeCredit(c *Credit, label string) (*Credit, error) {
	if c == nil {
		return nil, nil
	}
	out := Credit{
		Source:     strings.TrimSpace(c.Source),
		Author:     strings.TrimSpace(c.Author),
		Page:       strings.TrimSpace(c.Page),
		URL:        strings.TrimSpace(c.URL),
		Licence:    strings.TrimSpace(c.Licence),
		LicenceURL: strings.TrimSpace(c.LicenceURL),
		Notice:     strings.TrimSpace(c.Notice),
	}
	if out.Source == "" {
		return nil, nil
	}
	for _, f := range []struct{ name, v string }{{"source", out.Source}, {"author", out.Author}, {"page", out.Page}, {"licence", out.Licence}} {
		if utf8.RuneCountInString(f.v) > MaxCreditField {
			return nil, errInvalid(fmt.Sprintf("%s: the credit's %s can be at most %d characters", label, f.name, MaxCreditField))
		}
	}
	if utf8.RuneCountInString(out.Notice) > MaxCreditNotice {
		return nil, errInvalid(fmt.Sprintf("%s: the credit's notice can be at most %d characters", label, MaxCreditNotice))
	}
	for _, f := range []struct{ name, v string }{{"url", out.URL}, {"licenceUrl", out.LicenceURL}} {
		if f.v == "" {
			continue
		}
		if len(f.v) > MaxCreditURL || !httpsURL(f.v) {
			return nil, errInvalid(fmt.Sprintf("%s: the credit's %s must be an https link of at most %d characters", label, f.name, MaxCreditURL))
		}
	}
	return &out, nil
}

// httpsURL reports whether s is an absolute https URL with a host and no
// whitespace or quote characters.
func httpsURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	return !strings.ContainsAny(s, " \t\r\n\"'<>")
}
