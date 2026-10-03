package dmscreen

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// hiddenLimit keeps the reveal list to what fits on one panel; the full list
// lives on the Characters page.
const hiddenLimit = 8

// Service builds the DM Screen and performs its few actions.
type Service interface {
	Build(ctx context.Context, campaignID string, v Viewer) (*View, error)
	Reveal(ctx context.Context, entityID, campaignID string, v Viewer) (string, error)
	SetDowntime(ctx context.Context, campaignID string, v Viewer, open bool) (DowntimeResult, error)
}

// DowntimeResult reports a downtime switch: the new state and how many
// waiting requests went through or failed when it opened.
type DowntimeResult struct {
	Open    bool `json:"open"`
	Applied int  `json:"applied"`
	Failed  int  `json:"failed"`
}

type service struct {
	src Sources
}

// NewService returns a Service reading from src.
func NewService(src Sources) Service {
	return &service{src: src}
}

// Build assembles the screen. A failing source is logged and its section
// left out, so one broken plugin never blanks the whole screen.
func (s *service) Build(ctx context.Context, campaignID string, v Viewer) (*View, error) {
	if v.Role < 2 {
		return nil, apperror.NewForbidden("the DM Screen is for the campaign's owner and scribes")
	}
	view := &View{CampaignID: campaignID}
	warn := func(section string, err error) {
		slog.Warn("dm screen: section unavailable", slog.String("section", section), slog.String("campaign_id", campaignID), slog.Any("error", err))
	}

	if s.src.Downtime != nil {
		open, pending, ok, err := s.src.Downtime.Downtime(ctx, campaignID, v)
		if err != nil {
			warn("downtime", err)
		} else if ok {
			view.Downtime = &DowntimeView{Open: open, CanToggle: v.IsOwner(), Pending: pending}
		}
	}
	if s.src.World != nil {
		w, err := s.src.World.World(ctx, campaignID, v)
		if err != nil {
			warn("world", err)
		}
		view.World = w
	}
	if s.src.Nights != nil {
		n, err := s.src.Nights.NextNight(ctx, campaignID, v)
		if err != nil {
			warn("nights", err)
		}
		view.Night = n
	}
	if s.src.Foundry != nil {
		last, connected := s.src.Foundry.FoundryPresence(campaignID)
		view.Foundry = FoundryView{Connected: connected, NeverSeen: last == nil && !connected, LastSeen: last}
	}

	var def *systems.DMScreenDef
	var sys systems.System
	if s.src.System != nil {
		if sys = s.src.System.EnabledSystem(ctx, campaignID); sys != nil {
			if info := sys.Info(); info != nil {
				view.SystemName = info.Name
				def = info.DMScreen
			}
		}
	}

	if s.src.Party != nil {
		heroes, err := s.src.Party.Heroes(ctx, campaignID, v)
		if err != nil {
			warn("party", err)
		}
		var meters []systems.DMScreenMeter
		if def != nil {
			meters = def.Party
		}
		view.PartyFilled = len(meters) > 0
		for _, h := range heroes {
			hv := HeroView{
				ID: h.ID, Name: h.Name, PlayerName: h.PlayerName,
				Meters: buildMeters(meters, h.Fields),
			}
			if def != nil {
				hv.Subtitle, _ = fieldText(h.Fields, def.HeroSubtitle)
				hv.Conditions = heroConditions(h.Fields[def.HeroConditions])
			}
			view.Party = append(view.Party, hv)
		}
	}

	if s.src.Hidden != nil {
		hidden, err := s.src.Hidden.HiddenCharacters(ctx, campaignID, v, hiddenLimit)
		if err != nil {
			warn("hidden", err)
		}
		for _, h := range hidden {
			view.Hidden = append(view.Hidden, HiddenView{ID: h.ID, Name: h.Name, TypeName: h.TypeName})
		}
	}

	// A package whose data failed to load has no provider; the Rules tab is
	// then simply left out.
	if sys != nil && def != nil && def.Conditions != nil {
		if dp := sys.DataProvider(); dp != nil {
			items, err := dp.List(def.Conditions.Category)
			if err != nil {
				warn("conditions", err)
			}
			view.Conditions = pickConditions(items, def.Conditions)
		}
	}
	return view, nil
}

// Reveal makes a hidden character visible to players and returns its name.
func (s *service) Reveal(ctx context.Context, entityID, campaignID string, v Viewer) (string, error) {
	if v.Role < 2 {
		return "", apperror.NewForbidden("only the campaign's owner and scribes can reveal characters")
	}
	if s.src.Hidden == nil {
		return "", apperror.NewNotFound("revealing is not available")
	}
	if entityID == "" {
		return "", apperror.NewBadRequest("entity ID is required")
	}
	return s.src.Hidden.Reveal(ctx, entityID, campaignID)
}

// SetDowntime switches downtime from the screen. Only the owner (or a
// DM-granted co-DM) may, the same rule the Stashes page follows.
func (s *service) SetDowntime(ctx context.Context, campaignID string, v Viewer, open bool) (DowntimeResult, error) {
	if !v.IsOwner() {
		return DowntimeResult{}, apperror.NewForbidden("only the campaign owner can switch downtime")
	}
	if s.src.Downtime == nil {
		return DowntimeResult{}, apperror.NewNotFound("downtime is not available")
	}
	applied, failed, err := s.src.Downtime.SetDowntime(ctx, campaignID, v, open)
	if err != nil {
		return DowntimeResult{}, err
	}
	return DowntimeResult{Open: open, Applied: applied, Failed: failed}, nil
}

// buildMeters reads each declared meter from a hero's sheet fields. A meter
// whose current field is empty on this hero is skipped, so a class without
// a heroic resource simply shows one line fewer.
func buildMeters(defs []systems.DMScreenMeter, fields map[string]any) []MeterView {
	var out []MeterView
	for _, d := range defs {
		cur, ok := fieldText(fields, d.Current)
		if !ok {
			continue
		}
		label := d.Label
		if d.LabelField != "" {
			if l, ok := fieldText(fields, d.LabelField); ok {
				label = l
			}
		}
		if label == "" {
			continue
		}
		m := MeterView{Label: label, Current: cur}
		if d.Max != "" {
			if mx, ok := fieldText(fields, d.Max); ok {
				m.Max = mx
				cf, cerr := strconv.ParseFloat(cur, 64)
				mf, merr := strconv.ParseFloat(mx, 64)
				if cerr == nil && merr == nil && mf > 0 {
					m.HasMax = true
					ratio := math.Max(0, math.Min(1, cf/mf))
					m.Percent = int(math.Round(ratio * 100))
					m.Low = d.WarnBelow > 0 && cf/mf < d.WarnBelow
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// fieldText renders a sheet field as display text. Sheet values arrive as
// strings or JSON numbers; anything else (lists, objects) isn't a meter.
func fieldText(fields map[string]any, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok || raw == nil {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		return v, v != ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case bool:
		return "", false
	default:
		return "", false
	}
}

// maxHeroConditions bounds what one sheet field can put on the screen.
const maxHeroConditions = 12

// heroConditions reads the conditions a hero has from a sheet field that may
// hold a list, a JSON-encoded list, or a comma-separated string, with each
// entry a name or a {"name": …} object (the shape Foundry statuses sync as).
// Status ids like "frightened" or "dazed-1" come back as readable names.
func heroConditions(raw any) []string {
	var items []any
	switch v := raw.(type) {
	case []any:
		items = v
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		if strings.HasPrefix(v, "[") {
			if err := json.Unmarshal([]byte(v), &items); err != nil {
				return nil
			}
		} else {
			for _, part := range strings.Split(v, ",") {
				items = append(items, part)
			}
		}
	default:
		return nil
	}
	var out []string
	for _, it := range items {
		var name string
		switch c := it.(type) {
		case string:
			name = c
		case map[string]any:
			name, _ = c["name"].(string)
		}
		if name = humanizeID(name); name != "" {
			out = append(out, name)
		}
		if len(out) == maxHeroConditions {
			break
		}
	}
	return out
}

// humanizeID turns a status id ("taunted", "dazed_2") into a label.
func humanizeID(id string) string {
	id = strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(id))
	if id == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(id)
	return string(unicode.ToUpper(r)) + id[size:]
}

// refMarkup matches a system's inline cross-reference, {@category term} or
// {@category term|shown text}, which would otherwise print verbatim.
var refMarkup = regexp.MustCompile(`\{@[a-z]+ ([^}|]+)(?:\|([^}]+))?\}`)

// htmlTag matches tags in reference text that may carry HTML; the panel shows
// plain text, so tags are dropped rather than printed escaped.
var htmlTag = regexp.MustCompile(`<[^>]*>`)

// flattenMarkup replaces cross-references with the text they display and
// drops HTML tags.
func flattenMarkup(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	return refMarkup.ReplaceAllStringFunc(s, func(m string) string {
		parts := refMarkup.FindStringSubmatch(m)
		if parts[2] != "" {
			return parts[2]
		}
		return parts[1]
	})
}

// pickConditions keeps the entries the system marks as conditions, sorted
// by name, with the rule text from the entry's summary or description.
func pickConditions(items []systems.ReferenceItem, c *systems.DMScreenConditions) []ConditionView {
	var out []ConditionView
	for _, it := range items {
		if c.Property != "" && fmt.Sprint(it.Properties[c.Property]) != c.Value {
			continue
		}
		text := it.Description
		if text == "" {
			text = it.Summary
		}
		out = append(out, ConditionView{Name: it.Name, Text: flattenMarkup(text)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
