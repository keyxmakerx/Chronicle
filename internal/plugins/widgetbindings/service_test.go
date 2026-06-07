// service_test.go — C-WIDGET-BINDING-P1-SPINE acceptance coverage:
// binding CRUD, default-vs-bound resolution, the precedence chain
// (own > entity-type template > default), the DIRECTIONAL cascade guard
// (a type-template must NOT override an entity's own binding — Foundry #9818),
// the orphan guard (render-time + delete-hook + sweep), campaign-scope
// enforcement on BOTH host and resolved instance, and the source layer.
package widgetbindings

import (
	"context"
	"errors"
	"testing"
)

// --- in-memory repo (no DB; service logic is what we exercise) ---

type memRepo struct {
	rows map[string]*WidgetBinding // keyed by binding id
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]*WidgetBinding{}} }

func key(campaignID, hostType, hostID, widgetType string) string {
	return campaignID + "|" + hostType + "|" + hostID + "|" + widgetType
}

func (m *memRepo) Upsert(_ context.Context, b *WidgetBinding) error {
	// Enforce the unique (campaign, host_type, host_id, widget_type) like the DB.
	for id, ex := range m.rows {
		if key(ex.CampaignID, ex.HostType, ex.HostID, ex.WidgetType) == key(b.CampaignID, b.HostType, b.HostID, b.WidgetType) {
			ex.InstanceID = b.InstanceID
			m.rows[id] = ex
			return nil
		}
	}
	cp := *b
	m.rows[b.ID] = &cp
	return nil
}
func (m *memRepo) GetForHost(_ context.Context, campaignID, hostType, hostID, widgetType string) (*WidgetBinding, error) {
	for _, b := range m.rows {
		if b.CampaignID == campaignID && b.HostType == hostType && b.HostID == hostID && b.WidgetType == widgetType {
			cp := *b
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *memRepo) DeleteForHostWidget(_ context.Context, campaignID, hostType, hostID, widgetType string) error {
	for id, b := range m.rows {
		if b.CampaignID == campaignID && b.HostType == hostType && b.HostID == hostID && b.WidgetType == widgetType {
			delete(m.rows, id)
		}
	}
	return nil
}
func (m *memRepo) ListByCampaign(_ context.Context, campaignID string) ([]WidgetBinding, error) {
	var out []WidgetBinding
	for _, b := range m.rows {
		if b.CampaignID == campaignID {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (m *memRepo) ListForInstance(_ context.Context, campaignID, widgetType, instanceID string) ([]WidgetBinding, error) {
	var out []WidgetBinding
	for _, b := range m.rows {
		if b.CampaignID == campaignID && b.WidgetType == widgetType && b.InstanceID == instanceID {
			out = append(out, *b)
		}
	}
	return out, nil
}
func (m *memRepo) DeleteByID(_ context.Context, campaignID, id string) error {
	if b, ok := m.rows[id]; ok && b.CampaignID == campaignID {
		delete(m.rows, id)
	}
	return nil
}

// --- fake widget type ---

// fakeWidget models a widget type over a set of known instances, each tagged
// with the campaign it belongs to (so InstanceExists can enforce campaign
// scope exactly like the real calendar type). defaultByCampaign supplies the
// "today's behavior" default; existsErr forces a transient validation error.
type fakeWidget struct {
	slug             string
	instanceCampaign map[string]string // instanceID -> campaignID
	defaultByCamp    map[string]string // campaignID -> default instanceID
	existsErr        error
}

func (f *fakeWidget) Slug() string { return f.slug }
func (f *fakeWidget) InstanceExists(_ context.Context, campaignID, instanceID string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	c, ok := f.instanceCampaign[instanceID]
	if !ok {
		return false, nil // genuine not-found → orphan
	}
	return c == campaignID, nil // cross-campaign → false (security)
}
func (f *fakeWidget) DefaultInstance(_ context.Context, host HostRef) (string, bool, error) {
	id, ok := f.defaultByCamp[host.CampaignID]
	return id, ok, nil
}
func (f *fakeWidget) ListInstances(context.Context, string, int) ([]InstanceRef, error) {
	return nil, ErrNotImplemented
}
func (f *fakeWidget) CreateInstance(context.Context, string, any) (string, error) {
	return "", ErrNotImplemented
}

func newSvc(wt WidgetType) (Service, *memRepo) {
	repo := newMemRepo()
	reg := NewRegistry()
	reg.Register(wt)
	return NewService(repo, reg), repo
}

const wt = "calendar"

func entityHost(camp, id, typeID string) HostRef {
	return HostRef{CampaignID: camp, Type: HostTypeEntity, ID: id, EntityTypeID: typeID}
}

// --- tests ---

func TestBindAndResolveOwn(t *testing.T) {
	svc, _ := newSvc(&fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-A": "camp-1"}})
	host := entityHost("camp-1", "ent-1", "5")
	if err := svc.Bind(context.Background(), host, wt, "cal-A"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	res, err := svc.Resolve(context.Background(), host, wt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.InstanceID != "cal-A" || res.Source != SourceOwn {
		t.Errorf("want cal-A/own, got %s/%s", res.InstanceID, res.Source)
	}
}

func TestResolveDefaultWhenUnbound(t *testing.T) {
	svc, _ := newSvc(&fakeWidget{slug: wt, defaultByCamp: map[string]string{"camp-1": "cal-default"}})
	res, err := svc.Resolve(context.Background(), entityHost("camp-1", "ent-1", "5"), wt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.InstanceID != "cal-default" || res.Source != SourceDefault {
		t.Errorf("unbound should resolve to the default; got %s/%s", res.InstanceID, res.Source)
	}
}

func TestPrecedence_OwnOverTypeOverDefault(t *testing.T) {
	fw := &fakeWidget{
		slug:             wt,
		instanceCampaign: map[string]string{"cal-own": "camp-1", "cal-type": "camp-1"},
		defaultByCamp:    map[string]string{"camp-1": "cal-default"},
	}
	svc, _ := newSvc(fw)
	ctx := context.Background()
	host := entityHost("camp-1", "ent-1", "42")
	typeHost := HostRef{CampaignID: "camp-1", Type: HostTypeEntityType, ID: "42"}

	// Only a type-template binding exists → inherited.
	if err := svc.Bind(ctx, typeHost, wt, "cal-type"); err != nil {
		t.Fatalf("bind type: %v", err)
	}
	res, _ := svc.Resolve(ctx, host, wt)
	if res.InstanceID != "cal-type" || res.Source != SourceEntityType {
		t.Fatalf("with only a type binding, expect inherited cal-type/entity_type; got %s/%s", res.InstanceID, res.Source)
	}

	// Add the entity's OWN binding → it must win over the type template.
	if err := svc.Bind(ctx, host, wt, "cal-own"); err != nil {
		t.Fatalf("bind own: %v", err)
	}
	res, _ = svc.Resolve(ctx, host, wt)
	if res.InstanceID != "cal-own" || res.Source != SourceOwn {
		t.Fatalf("own must win; got %s/%s", res.InstanceID, res.Source)
	}
}

// Directional cascade guard (Foundry #9818): a type template must NOT override
// an entity's own binding.
func TestDirectionalCascade_TypeDoesNotOverrideOwn(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-own": "camp-1", "cal-type": "camp-1"}}
	svc, _ := newSvc(fw)
	ctx := context.Background()
	host := entityHost("camp-1", "ent-1", "7")
	_ = svc.Bind(ctx, host, wt, "cal-own")
	_ = svc.Bind(ctx, HostRef{CampaignID: "camp-1", Type: HostTypeEntityType, ID: "7"}, wt, "cal-type")
	res, _ := svc.Resolve(ctx, host, wt)
	if res.Source != SourceOwn || res.InstanceID != "cal-own" {
		t.Errorf("entity-own must not be overridden by type-template; got %s/%s", res.InstanceID, res.Source)
	}
}

// Security: cannot bind an instance from another campaign.
func TestBind_RejectsCrossCampaignInstance(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-other": "camp-OTHER"}}
	svc, _ := newSvc(fw)
	err := svc.Bind(context.Background(), entityHost("camp-1", "ent-1", "5"), wt, "cal-other")
	if !errors.Is(err, ErrInstanceNotInCampaign) {
		t.Errorf("cross-campaign bind must be rejected; got %v", err)
	}
}

// Security: a binding whose instance is in another campaign must not resolve
// (render-time guard on the resolved instance, the #1 leakage vector) — it
// falls through to the default and sweeps the dead binding.
func TestResolve_DropsCrossCampaignInstance(t *testing.T) {
	fw := &fakeWidget{
		slug:             wt,
		instanceCampaign: map[string]string{"cal-x": "camp-1"},
		defaultByCamp:    map[string]string{"camp-1": "cal-default"},
	}
	svc, repo := newSvc(fw)
	ctx := context.Background()
	host := entityHost("camp-1", "ent-1", "5")
	_ = svc.Bind(ctx, host, wt, "cal-x")
	// The instance "moves" out of the campaign (or is deleted) after binding.
	fw.instanceCampaign["cal-x"] = "camp-OTHER"
	res, _ := svc.Resolve(ctx, host, wt)
	if res.Source != SourceDefault || res.InstanceID != "cal-default" {
		t.Errorf("orphaned/cross-campaign binding must fall back to default; got %s/%s", res.InstanceID, res.Source)
	}
	// And the dead binding was swept by the render-time guard.
	if b, _ := repo.GetForHost(ctx, "camp-1", HostTypeEntity, "ent-1", wt); b != nil {
		t.Errorf("render-time guard should have swept the orphan binding")
	}
}

// Orphan guard must NOT sweep on a transient validation error.
func TestResolve_DoesNotSweepOnTransientError(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-x": "camp-1"}}
	svc, repo := newSvc(fw)
	ctx := context.Background()
	host := entityHost("camp-1", "ent-1", "5")
	_ = svc.Bind(ctx, host, wt, "cal-x")
	fw.existsErr = errors.New("db blip")
	res, _ := svc.Resolve(ctx, host, wt)
	if res.Resolved() {
		t.Errorf("transient error should not win, got %+v", res)
	}
	if b, _ := repo.GetForHost(ctx, "camp-1", HostTypeEntity, "ent-1", wt); b == nil {
		t.Errorf("must NOT sweep the binding on a transient error")
	}
}

// Delete hook removes all bindings pointing at a deleted instance.
func TestOnInstanceDeleted(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-A": "camp-1"}}
	svc, repo := newSvc(fw)
	ctx := context.Background()
	_ = svc.Bind(ctx, entityHost("camp-1", "ent-1", "5"), wt, "cal-A")
	_ = svc.Bind(ctx, entityHost("camp-1", "ent-2", "5"), wt, "cal-A")
	n, err := svc.OnInstanceDeleted(ctx, "camp-1", wt, "cal-A")
	if err != nil || n != 2 {
		t.Fatalf("delete hook should clean 2; got %d (%v)", n, err)
	}
	if rows, _ := repo.ListByCampaign(ctx, "camp-1"); len(rows) != 0 {
		t.Errorf("bindings should be gone; got %d", len(rows))
	}
}

// Periodic sweep removes bindings whose instance no longer validates.
func TestSweep(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-A": "camp-1", "cal-B": "camp-1"}}
	svc, repo := newSvc(fw)
	ctx := context.Background()
	_ = svc.Bind(ctx, entityHost("camp-1", "ent-1", "5"), wt, "cal-A")
	_ = svc.Bind(ctx, entityHost("camp-1", "ent-2", "5"), wt, "cal-B")
	delete(fw.instanceCampaign, "cal-B") // cal-B deleted out from under its binding
	n, err := svc.Sweep(ctx, "camp-1")
	if err != nil || n != 1 {
		t.Fatalf("sweep should remove 1 orphan; got %d (%v)", n, err)
	}
	if b, _ := repo.GetForHost(ctx, "camp-1", HostTypeEntity, "ent-2", wt); b != nil {
		t.Errorf("orphaned cal-B binding should be swept")
	}
	if b, _ := repo.GetForHost(ctx, "camp-1", HostTypeEntity, "ent-1", wt); b == nil {
		t.Errorf("healthy cal-A binding must survive the sweep")
	}
}

// Unbind removes the host's binding (CRUD).
func TestUnbind(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-A": "camp-1"}}
	svc, repo := newSvc(fw)
	ctx := context.Background()
	host := entityHost("camp-1", "ent-1", "5")
	_ = svc.Bind(ctx, host, wt, "cal-A")
	if err := svc.Unbind(ctx, host, wt); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if b, _ := repo.GetForHost(ctx, "camp-1", HostTypeEntity, "ent-1", wt); b != nil {
		t.Errorf("binding should be removed after unbind")
	}
}

// Modularity: host_type entity_type AND dashboard are storable + resolvable
// with no schema/enum change — only registry registration + rows.
func TestModularity_AllHostTypesStorable(t *testing.T) {
	fw := &fakeWidget{slug: wt, instanceCampaign: map[string]string{"cal-d": "camp-1", "cal-t": "camp-1"}}
	svc, repo := newSvc(fw)
	ctx := context.Background()
	for _, h := range []HostRef{
		{CampaignID: "camp-1", Type: HostTypeEntityType, ID: "42"},
		{CampaignID: "camp-1", Type: HostTypeDashboard, ID: "camp-1:player"},
	} {
		if err := svc.Bind(ctx, h, wt, "cal-t"); err != nil {
			t.Errorf("host_type %q must be bindable: %v", h.Type, err)
		}
	}
	if rows, _ := repo.ListByCampaign(ctx, "camp-1"); len(rows) != 2 {
		t.Errorf("both entity_type + dashboard bindings should persist; got %d", len(rows))
	}
	// A dashboard host resolves its own binding.
	res, _ := svc.Resolve(ctx, HostRef{CampaignID: "camp-1", Type: HostTypeDashboard, ID: "camp-1:player"}, wt)
	if res.Source != SourceOwn || res.InstanceID != "cal-t" {
		t.Errorf("dashboard host should resolve its own binding; got %s/%s", res.InstanceID, res.Source)
	}
}

func TestBind_RejectsUnknownWidgetTypeAndHostType(t *testing.T) {
	svc, _ := newSvc(&fakeWidget{slug: wt})
	ctx := context.Background()
	if err := svc.Bind(ctx, entityHost("camp-1", "e", "1"), "nope", "x"); !errors.Is(err, ErrUnknownWidgetType) {
		t.Errorf("unknown widget type must be rejected; got %v", err)
	}
	if err := svc.Bind(ctx, HostRef{CampaignID: "camp-1", Type: "bogus", ID: "x"}, wt, "x"); !errors.Is(err, ErrInvalidHostType) {
		t.Errorf("invalid host type must be rejected; got %v", err)
	}
}
