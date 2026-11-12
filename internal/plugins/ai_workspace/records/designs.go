package records

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// DesignsAPI is the saved-designs library as the `design` kind and the
// `designs` lookup need it, adapted in app wiring so this package does not
// import the plugin. The service is the authority: it checks the owner rule,
// the labels and, through the kind's own validator, the data. Sharing is not
// here on purpose; the owner shares from the Share sheet.
type DesignsAPI interface {
	// List is the campaign's library, newest first.
	List(ctx context.Context, campaignID string, a Actor) ([]DesignRef, error)
	// Check runs Save's checks without saving.
	Check(ctx context.Context, campaignID string, a Actor, v DesignValue) error
	Save(ctx context.Context, campaignID string, a Actor, v DesignValue) error
	// Rename changes the name and/or note; nil leaves one as it is.
	Rename(ctx context.Context, campaignID string, a Actor, id string, name, note *string) error
}

// DesignValue is a design to save: its kind of design, labels and data.
type DesignValue struct {
	Kind, Name, Note string
	Data             json.RawMessage
}

// DesignRef is a saved design as the lookups show it.
type DesignRef struct {
	ID, Kind, Name, Note, SystemID, Maker, Origin string
	Shared                                        bool
	Data                                          json.RawMessage
}

// DesignKind saves a new design in the campaign's library, or renames one.
// Matched by `name`. Only the owner has a library, as in the room builder, and
// a block can neither share nor remove a design.
type DesignKind struct {
	Svc    DesignsAPI
	Owners CampaignOwnerAPI
}

func (DesignKind) Name() string  { return "design" }
func (DesignKind) Label() string { return "Saved design" }
func (DesignKind) Brief() string {
	return "save a design in the library (a shop counter room's theme) or rename one"
}

func (DesignKind) Doc() string {
	return "A design in the campaign's library: a list of choices from known parts, saved to reuse or to share later. Keys: " +
		"`name` (at most 60 characters; it is how an update finds the design), `type` (room-theme: a shop counter room's theme), " +
		"`note` (at most 140 characters, optional) and `data` (the design itself, an object; for room-theme the same theme the `shop-room` kind's `counter` describes, written in full as `version`, `name`, `skeleton`, `theme` and `props`). " +
		"The server refuses any part it does not know, naming it. `action: update` renames (`rename`, the new name) or changes the `note`; the data of a saved design is not changed, save a new one instead. " +
		"A block never shares or removes a design: the owner does that from the room builder. Ask `what: designs` to see the library, each design with its block.\n\n" +
		"```\n---\nkind: design\nname: Cinderhold Forge\ntype: room-theme\nnote: Warm, for the smithy\ndata: {\"version\":1,\"name\":\"Cinderhold Forge\",\"skeleton\":{\"shape\":\"alcove\",\"opening\":\"window-left\",\"display\":\"pegboard\",\"counter\":\"straight\"},\"theme\":{\"wall\":\"stone\",\"wood\":\"#6b4527\",\"cloth\":\"#5a2024\",\"accent\":\"#ff9a50\",\"trim\":\"#c89a4a\",\"light\":\"forge\",\"warmth\":0.8,\"outside\":\"dusk\"},\"props\":[\"anvil\",\"sparks\"]}\n---\n```\n\n" +
		"```\n---\nkind: design\naction: update\nname: Cinderhold Forge\nrename: Cinderhold Smithy\n---\n```"
}

func (DesignKind) Title(r Record) string {
	if r.Name != "" {
		return "Design " + r.Name
	}
	return ""
}

// owner is the library's rule: the campaign's owner, never a co-DM.
func (k DesignKind) owner(ctx context.Context, campaignID string, a Actor) error {
	if a.Role >= permissions.RoleOwner && k.Owners != nil {
		if ok, err := k.Owners.IsOwner(ctx, campaignID, a.UserID); err == nil && ok {
			return nil
		}
	}
	return badRequestf("only the campaign's owner can save designs")
}

// designData reads the `data` key, a nested object in the front matter, as JSON.
func designData(r Record) (json.RawMessage, error) {
	v, ok := r.Fields["data"]
	if !ok || v == nil {
		return nil, badRequestf("data: write the design as an object")
	}
	if s, isStr := v.(string); isStr {
		if !json.Valid([]byte(s)) {
			return nil, badRequestf("data: write the design as an object, not text")
		}
		return json.RawMessage(s), nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, badRequestf("data: the design can't be read as an object")
	}
	return raw, nil
}

func (k DesignKind) find(ctx context.Context, campaignID string, a Actor, name string) (*DesignRef, error) {
	list, err := k.Svc.List(ctx, campaignID, a)
	if err != nil {
		return nil, err
	}
	var found *DesignRef
	for i := range list {
		if strings.EqualFold(list[i].Name, strings.TrimSpace(name)) {
			if found != nil {
				return nil, badRequestf("more than one design is called %s; rename one in the room builder first", quote(name))
			}
			found = &list[i]
		}
	}
	return found, nil
}

func (k DesignKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if err := k.owner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return Plan{Error: "a design needs a `name`"}
	}
	switch r.Action {
	case ActionDelete:
		return Plan{Error: "a block can't remove a design; the owner removes it from the room builder"}
	case ActionUpdate:
		d, err := k.find(ctx, campaignID, a, name)
		if err != nil {
			return Plan{Error: planError(err)}
		}
		if d == nil {
			return Plan{Error: "there is no design called " + quote(name)}
		}
		if r.Has("data") || r.Has("type") {
			return Plan{Error: "the data of a saved design is not changed; save a new design instead"}
		}
		n, note := k.labels(r)
		if n == nil && note == nil {
			return Plan{Summary: "changes nothing in the design " + quote(d.Name)}
		}
		if err := k.Svc.Check(ctx, campaignID, a, DesignValue{Kind: d.Kind, Name: orCurrent(n, d.Name), Note: orCurrent(note, d.Note), Data: d.Data}); err != nil {
			return Plan{Error: lookError("the design", err)}
		}
		return Plan{Summary: "updates the design " + quote(d.Name) + k.said(n, note)}
	}
	v, err := k.value(r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if d, err := k.find(ctx, campaignID, a, name); err != nil {
		return Plan{Error: planError(err)}
	} else if d != nil {
		return Plan{Error: "a design called " + quote(name) + " is already saved; use action: update to rename it"}
	}
	if err := k.Svc.Check(ctx, campaignID, a, v); err != nil {
		return Plan{Error: lookError("the design", err)}
	}
	return Plan{Summary: fmt.Sprintf("saves the %s design %s", v.Kind, quote(v.Name))}
}

// value is a new design from a create block.
func (k DesignKind) value(r Record) (DesignValue, error) {
	kind := strings.ToLower(r.Str("type"))
	if kind == "" {
		return DesignValue{}, badRequestf("type: say what kind of design this is, for example room-theme")
	}
	data, err := designData(r)
	if err != nil {
		return DesignValue{}, err
	}
	return DesignValue{Kind: kind, Name: strings.TrimSpace(r.Name), Note: r.Str("note"), Data: data}, nil
}

// labels reads an update's new name (`rename`) and note; nil means unchanged.
func (DesignKind) labels(r Record) (name, note *string) {
	if r.Has("rename") {
		s := r.Str("rename")
		name = &s
	}
	if r.Has("note") {
		s := r.Str("note")
		if isNone(s) {
			s = ""
		}
		note = &s
	}
	return name, note
}

func (DesignKind) said(name, note *string) string {
	var parts []string
	if name != nil {
		parts = append(parts, "name "+quote(*name))
	}
	if note != nil {
		parts = append(parts, "note")
	}
	return ": " + strings.Join(parts, ", ")
}

func orCurrent(p *string, cur string) string {
	if p != nil {
		return *p
	}
	return cur
}

func (k DesignKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if err := k.owner(ctx, campaignID, a); err != nil {
		return err
	}
	if r.Action == ActionUpdate {
		d, err := k.find(ctx, campaignID, a, r.Name)
		if err != nil {
			return err
		}
		if d == nil {
			return apperror.NewBadRequest("there is no design called " + quote(r.Name))
		}
		name, note := k.labels(r)
		return k.Svc.Rename(ctx, campaignID, a, d.ID, name, note)
	}
	v, err := k.value(r)
	if err != nil {
		return err
	}
	return k.Svc.Save(ctx, campaignID, a, v)
}

// Export is empty: the `designs` lookup prints the library as blocks.
func (DesignKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// designBlock writes a saved design in the block's own keys.
func designBlock(d DesignRef) string {
	var b strings.Builder
	b.WriteString("---\nkind: design\nname: " + quote(d.Name) + "\ntype: " + d.Kind + "\n")
	if d.Note != "" {
		b.WriteString("note: " + quote(d.Note) + "\n")
	}
	b.WriteString("data: " + string(d.Data) + "\n---\n")
	return b.String()
}

// DesignsLookup answers `what: designs`: the campaign's library, each design
// with its block. It is the owner's, like the library.
type DesignsLookup struct {
	Svc    DesignsAPI
	Owners CampaignOwnerAPI
}

func (DesignsLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "designs", Aliases: []string{"design", "design-library"}, Keys: "none",
		Brief: "the campaign's saved designs (shop counter room themes): name, kind, game system, maker, whether shared, and each as a block (owner only)",
		Tool: aiplug.Tool{
			Name: "designs", Title: "Saved designs", Did: "Listed the saved designs", Owner: true,
			Desc: "The campaign's library of saved designs: each with its kind, game system, maker and whether it is shared, and a block to send back changed.",
		},
	}
}

func (l DesignsLookup) Answer(ctx context.Context, campaignID string, a Actor, _ Record) (string, string, error) {
	chip := "Saved designs"
	if err := (DesignKind{Svc: l.Svc, Owners: l.Owners}).owner(ctx, campaignID, a); err != nil {
		return chip, "", badRequestf("saved designs are the campaign owner's")
	}
	list, err := l.Svc.List(ctx, campaignID, a)
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Saved designs (%d)\n\n", len(list))
	if len(list) == 0 {
		b.WriteString("Nothing is saved yet.\n")
	}
	for _, d := range list {
		share := "not shared"
		if d.Shared {
			share = "shared"
		}
		fmt.Fprintf(&b, "### %s\n\n%s, game system %s, made by %s, %s.\n\n```\n%s```\n\n", d.Name, d.Kind, nonEmpty(d.SystemID, "none"), nonEmpty(d.Maker, "unknown"), share, designBlock(d))
	}
	return chip, b.String(), nil
}

func nonEmpty(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}
