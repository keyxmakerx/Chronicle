package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/changesource"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/playerchanges"
)

// fieldObservers tells every observer about a field write, and passes page
// saves and removals on to those that want them. The entities service takes
// one observer; this lets the Armory's money history and "What players
// changed" both hear about writes.
type fieldObservers []entities.FieldChangeObserver

func (f fieldObservers) FieldsChanged(ctx context.Context, e *entities.Entity, oldFields, newFields map[string]any) error {
	for _, o := range f {
		if err := o.FieldsChanged(ctx, e, oldFields, newFields); err != nil {
			slog.Warn("field observer failed", slog.String("entity_id", e.ID), slog.Any("error", err))
		}
	}
	return nil
}

func (f fieldObservers) TextChanged(ctx context.Context, before, after *entities.Entity, userID string) error {
	for _, o := range f {
		if po, ok := o.(entities.PageChangeObserver); ok {
			if err := po.TextChanged(ctx, before, after, userID); err != nil {
				slog.Warn("page observer failed", slog.String("entity_id", after.ID), slog.Any("error", err))
			}
		}
	}
	return nil
}

func (f fieldObservers) PageTrashed(ctx context.Context, e *entities.Entity, userID string) error {
	for _, o := range f {
		if po, ok := o.(entities.PageChangeObserver); ok {
			if err := po.PageTrashed(ctx, e, userID); err != nil {
				slog.Warn("page observer failed", slog.String("entity_id", e.ID), slog.Any("error", err))
			}
		}
	}
	return nil
}

// playerChangeMembers is what the recorder needs from campaigns.
type playerChangeMembers interface {
	GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
}

// playerChangeTypes is what the recorder needs from entities to label fields.
type playerChangeTypes interface {
	GetEntityTypeByID(ctx context.Context, id int) (*entities.EntityType, error)
}

// playerChangeRecorder turns page writes into "What players changed" rows.
// Only members whose role is Player are recorded: the owner and scribes
// already keep the campaign, and a page's history says what they did.
type playerChangeRecorder struct {
	svc     playerchanges.Service
	members playerChangeMembers
	types   playerChangeTypes
}

// recordedKinds are the write sources a player makes by hand or through
// their AI. Stash, shop and craft writes are the side effects of a move the
// Armory's own history already shows.
var recordedKinds = map[string]bool{
	changesource.KindWeb: true, changesource.KindAI: true,
	changesource.KindFoundry: true, changesource.KindExtension: true,
}

// player returns the player's name when userID is a Player in the campaign.
func (r *playerChangeRecorder) player(ctx context.Context, campaignID, userID string) (string, bool) {
	if userID == "" {
		return "", false
	}
	m, err := r.members.GetMember(ctx, campaignID, userID)
	if err != nil || m == nil || m.Role != campaigns.RolePlayer {
		return "", false
	}
	return m.DisplayName, true
}

// source names where a write came from for the row.
func source(ctx context.Context) (src, app string) {
	s, _ := changesource.From(ctx)
	switch s.Kind {
	case changesource.KindAI:
		return playerchanges.SourceAI, s.Label
	case changesource.KindFoundry:
		return playerchanges.SourceWeb, "Foundry"
	}
	return playerchanges.SourceWeb, s.Label
}

func (r *playerChangeRecorder) FieldsChanged(ctx context.Context, e *entities.Entity, oldFields, newFields map[string]any) error {
	s, ok := changesource.From(ctx)
	if !ok || !recordedKinds[s.Kind] {
		return nil
	}
	name, ok := r.player(ctx, e.CampaignID, s.UserID)
	if !ok {
		return nil
	}
	labels := r.labels(ctx, e)
	src, app := source(ctx)
	keys := map[string]bool{}
	for k := range oldFields {
		keys[k] = true
	}
	for k := range newFields {
		keys[k] = true
	}
	for k := range keys {
		if strings.HasPrefix(k, "_") {
			continue // internal bookkeeping, not something the player typed
		}
		was, now := playerchanges.ValueJSON(oldFields[k]), playerchanges.ValueJSON(newFields[k])
		if was == now {
			continue
		}
		label := labels[k]
		if label == "" {
			label = keyLabel(k)
		}
		if err := r.svc.Record(ctx, playerchanges.Change{
			CampaignID: e.CampaignID, EntityID: e.ID, PageName: e.Name, UserID: s.UserID, UserName: name,
			Kind: playerchanges.KindField, FieldKey: k, FieldLabel: label,
			OldText: playerchanges.TextOf(oldFields[k]), NewText: playerchanges.TextOf(newFields[k]),
			OldJSON: was, NewJSON: now, Source: src, App: app,
		}); err != nil {
			return err
		}
	}
	return nil
}

// labels maps the page's field keys to the names its sheet shows.
func (r *playerChangeRecorder) labels(ctx context.Context, e *entities.Entity) map[string]string {
	out := map[string]string{}
	if et, err := r.types.GetEntityTypeByID(ctx, e.EntityTypeID); err == nil && et != nil {
		for _, f := range et.Fields {
			out[f.Key] = f.Label
		}
	}
	if o := e.FieldOverrides; o != nil {
		for _, f := range o.Added {
			out[f.Key] = f.Label
		}
		for k, m := range o.Modified {
			if m.Label != nil && *m.Label != "" {
				out[k] = *m.Label
			}
		}
	}
	return out
}

func (r *playerChangeRecorder) TextChanged(ctx context.Context, before, after *entities.Entity, userID string) error {
	if s, ok := changesource.From(ctx); ok && !recordedKinds[s.Kind] {
		return nil
	}
	name, ok := r.player(ctx, after.CampaignID, userID)
	if !ok {
		return nil
	}
	src, app := source(ctx)
	base := playerchanges.Change{
		CampaignID: after.CampaignID, EntityID: after.ID, PageName: after.Name, UserID: userID, UserName: name,
		Kind: playerchanges.KindText, Source: src, App: app,
	}
	if before.Name != after.Name {
		c := base
		c.FieldKey, c.OldText, c.NewText = "name", before.Name, after.Name
		c.OldJSON, c.NewJSON = playerchanges.ValueJSON(before.Name), playerchanges.ValueJSON(after.Name)
		if err := r.svc.Record(ctx, c); err != nil {
			return err
		}
	}
	was, now := plainText(before.EntryHTML), plainText(after.EntryHTML)
	if was == now {
		return nil
	}
	c := base
	c.OldJSON, c.NewJSON = was, now
	c.OldText, c.NewText = textDiff(was, now)
	return r.svc.Record(ctx, c)
}

func (r *playerChangeRecorder) PageTrashed(ctx context.Context, e *entities.Entity, userID string) error {
	name, ok := r.player(ctx, e.CampaignID, userID)
	if !ok {
		return nil
	}
	src, app := source(ctx)
	return r.svc.Record(ctx, playerchanges.Change{
		CampaignID: e.CampaignID, EntityID: e.ID, PageName: e.Name, UserID: userID, UserName: name,
		Kind: playerchanges.KindRemoved, Source: src, App: app,
	})
}

// keyLabel names a field the sheet has no label for from its key:
// "hero_tokens" reads "Hero tokens".
func keyLabel(k string) string {
	k = strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(k))
	if k == "" {
		return k
	}
	return strings.ToUpper(k[:1]) + k[1:]
}

func plainText(html *string) string {
	if html == nil {
		return ""
	}
	return strings.TrimSpace(entities.HTMLToPlainLines(*html))
}

// textDiff names the words a text lost and gained, each run of them joined
// with " … " so the line reads as excerpts.
func textDiff(older, newer string) (removed, added string) {
	var gone, came []string
	var run strings.Builder
	op := byte(' ')
	flush := func() {
		if t := strings.Join(strings.Fields(run.String()), " "); t != "" {
			if op == '-' {
				gone = append(gone, t)
			} else if op == '+' {
				came = append(came, t)
			}
		}
		run.Reset()
	}
	for _, p := range entities.DiffWords(older, newer) {
		if p.Op != op {
			// A space between two removed (or added) words keeps the run going.
			if p.Op == ' ' && strings.TrimSpace(p.Text) == "" {
				run.WriteString(p.Text)
				continue
			}
			flush()
			op = p.Op
		}
		run.WriteString(p.Text)
	}
	flush()
	return strings.Join(gone, " … "), strings.Join(came, " … ")
}

// playerChangePages lets Undo read and write the pages a change was on.
type playerChangePages struct {
	entities entities.EntityService
	safety   entities.PageSafetyService
}

// page returns the campaign's page, or nil when it is gone or elsewhere.
func (p *playerChangePages) page(ctx context.Context, campaignID, entityID string) (*entities.Entity, error) {
	e, err := p.entities.GetByID(ctx, entityID)
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if e == nil || e.CampaignID != campaignID {
		return nil, nil
	}
	return e, nil
}

func (p *playerChangePages) FieldValue(ctx context.Context, campaignID, entityID, key string) (any, bool, error) {
	e, err := p.page(ctx, campaignID, entityID)
	if err != nil || e == nil {
		return nil, false, err
	}
	return e.FieldsData[key], true, nil
}

func (p *playerChangePages) SetField(ctx context.Context, campaignID, entityID, key string, value any, ownerID string) error {
	e, err := p.page(ctx, campaignID, entityID)
	if err != nil {
		return err
	}
	if e == nil {
		return playerchanges.ErrPageGone
	}
	wctx := changesource.With(entities.WithActor(ctx, ownerID), changesource.Source{Kind: changesource.KindWeb, UserID: ownerID})
	return p.entities.MergeFields(wctx, entityID, map[string]any{key: value})
}

func (p *playerChangePages) RestorePage(ctx context.Context, campaignID, entityID string) error {
	_, err := p.safety.RestoreFromTrash(ctx, campaignID, entityID)
	return err
}

func (p *playerChangePages) TextDiff(older, newer string) (string, string) {
	return textDiff(older, newer)
}
