package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// MapsAPI is the maps service as pins need it, in this package's own
// types (adapted in app wiring so this package does not import the
// plugin). The service's marker calls do not check the map's campaign, so a
// map is only ever found through ListMaps(campaignID) and a pin only
// through ListPins on that map.
type MapsAPI interface {
	ListMaps(ctx context.Context, campaignID string) ([]MapRef, error)
	ListPins(ctx context.Context, campaignID, mapID string, role int, userID string) ([]PinRef, error)
	CreatePin(ctx context.Context, mapID, userID string, in PinInput) error
	UpdatePin(ctx context.Context, pinID string, in PinInput, canAuthorDmOnly bool) error
	DeletePin(ctx context.Context, pinID string, canAuthorDmOnly bool, actorID string, role int) error
}

// MapRef is a map's identity.
type MapRef struct{ ID, CampaignID, Name string }

// PinRef is a pin as listed.
type PinRef struct {
	ID, Name string
	X, Y     float64
}

// PinInput is a pin's fields; on update an unset field is left as it is
// (nil, or "" for strings), and on create "" takes the service default.
type PinInput struct {
	Name, Icon, Color, Category, Visibility string
	Description, EntityID                   *string
	X, Y                                    *float64
}

// PinKind adds, moves and removes map pins, matched by map and pin name.
type PinKind struct {
	Svc      MapsAPI
	Entities EntityLookup
}

func (PinKind) Name() string  { return "pin" }
func (PinKind) Label() string { return "Map pin" }
func (PinKind) Doc() string {
	return "A pin on a map, matched by `map` (the map's name) and `name`. Keys: `x` and `y` (0 to 100, percent across and down the map), optional `icon`, `color`, `category` (location, danger, treasure, quest, note), `page` (a page name to link), `visibility` (`everyone` or `dm_only`), `rename_to`. The body is the pin's description.\n\n```\n---\nkind: pin\nmap: Grimvale\nname: Old Mill\nx: 42.5\ny: 61\ncategory: location\n---\nAbandoned since the flood.\n```"
}

func (k PinKind) findMap(ctx context.Context, campaignID, name string) (*MapRef, error) {
	ms, err := k.Svc.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewBadRequest("could not read the maps")
	}
	for i := range ms {
		if sameName(ms[i].Name, name) && ms[i].CampaignID == campaignID {
			return &ms[i], nil
		}
	}
	return nil, badRequestf("no map called %s", quote(name))
}

func (k PinKind) find(ctx context.Context, campaignID string, a Actor, r Record) (*MapRef, *PinRef, error) {
	m, err := k.findMap(ctx, campaignID, r.Str("map"))
	if err != nil {
		return nil, nil, err
	}
	pins, err := k.Svc.ListPins(ctx, campaignID, m.ID, a.Role, a.UserID)
	if err != nil {
		return m, nil, badRequestf("could not read the pins on %s", m.Name)
	}
	var hit *PinRef
	for i := range pins {
		if sameName(pins[i].Name, r.Name) {
			if hit != nil {
				return m, nil, badRequestf("%s has more than one pin called %s", m.Name, quote(r.Name))
			}
			hit = &pins[i]
		}
	}
	return m, hit, nil
}

func (k PinKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Name == "" || r.Str("map") == "" {
		return Plan{Error: "a pin needs `map` and `name`"}
	}
	for _, key := range []string{"x", "y"} {
		f, ok, err := r.Float(key)
		if err != nil {
			return Plan{Error: err.Error()}
		}
		if ok && (f < 0 || f > 100) {
			return Plan{Error: key + " must be between 0 and 100"}
		}
		if !ok && r.Action == ActionCreate {
			return Plan{Error: "a new pin needs x and y"}
		}
	}
	vis := r.Str("visibility")
	if vis != "" && vis != "everyone" && vis != "dm_only" {
		return Plan{Error: "visibility must be everyone or dm_only"}
	}
	if vis == "dm_only" && !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can add hidden pins"}
	}
	m, pin, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: err.Error()}
	}
	switch r.Action {
	case ActionCreate:
		p := Plan{Summary: "on the " + m.Name + " map"}
		if pin != nil {
			p.Warnings = append(p.Warnings, "A pin with this name is already on this map; this adds another")
		}
		return p
	case ActionUpdate:
		if pin == nil {
			return Plan{Error: "no pin called " + quote(r.Name) + " on " + m.Name}
		}
		return Plan{Summary: "changes the pin on the " + m.Name + " map"}
	default:
		if pin == nil {
			return Plan{Error: "no pin called " + quote(r.Name) + " on " + m.Name}
		}
		return Plan{Summary: "on the " + m.Name + " map"}
	}
}

func (k PinKind) linkedPage(ctx context.Context, campaignID string, r Record) (*string, error) {
	name := r.Str("page")
	if name == "" {
		return nil, nil
	}
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return nil, badRequestf("no page called %s to link", quote(name))
	}
	return &e.ID, nil
}

func (k PinKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	m, pin, err := k.find(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if r.Action == ActionDelete {
		return k.Svc.DeletePin(ctx, pin.ID, a.CanAuthorDmOnly(), a.UserID, a.Role)
	}
	page, err := k.linkedPage(ctx, campaignID, r)
	if err != nil {
		return err
	}
	in := PinInput{Name: r.Str("rename_to"), Icon: r.Str("icon"), Color: r.Str("color"),
		Category: r.Str("category"), Visibility: r.Str("visibility"), EntityID: page}
	if desc := strings.TrimSpace(r.Body); desc != "" {
		in.Description = &desc
	}
	if x, ok, _ := r.Float("x"); ok {
		in.X = &x
	}
	if y, ok, _ := r.Float("y"); ok {
		in.Y = &y
	}
	if r.Action == ActionCreate {
		in.Name = r.Name
		return k.Svc.CreatePin(ctx, m.ID, a.UserID, in)
	}
	return k.Svc.UpdatePin(ctx, pin.ID, in, a.CanAuthorDmOnly())
}

func (k PinKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	ms, err := k.Svc.ListMaps(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, m := range ms {
		pins, err := k.Svc.ListPins(ctx, campaignID, m.ID, a.Role, a.UserID)
		if err != nil {
			return "", err
		}
		if len(pins) == 0 {
			continue
		}
		b.WriteString("### Map: " + m.Name + "\n\n")
		for _, p := range pins {
			fmt.Fprintf(&b, "- %s (x %.1f, y %.1f)\n", p.Name, p.X, p.Y)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
