package cutscenes

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// Service owns every rule about scenes and showings. It never imports Echo:
// the handlers bind and render, the service decides.
//
// Two sight rules hold everywhere: only a Director (owner or co-DM) reads,
// writes or shows scenes, and a showing is readable only by the people it was
// for while its window is open, or by a Director. Anyone else is told it does
// not exist (404), never that it is not theirs.
type Service interface {
	List(ctx context.Context, campaignID string, v Viewer) ([]Scene, error)
	Get(ctx context.Context, campaignID, sceneID string, v Viewer) (*Scene, error)
	Create(ctx context.Context, campaignID string, v Viewer, in Scene) (*Scene, error)
	// Save replaces the whole scene: the editor posts every layer.
	Save(ctx context.Context, campaignID, sceneID string, v Viewer, in Scene) (*Scene, error)
	Delete(ctx context.Context, campaignID, sceneID string, v Viewer) error
	// Check returns the scene as Save would store it, for the editor's
	// Preview, without storing anything.
	Check(ctx context.Context, campaignID string, v Viewer, in Scene) (Scene, error)
	// FoundryPreviewOffered says whether the editor offers Preview in
	// Foundry: the campaign is connected to the Foundry module.
	FoundryPreviewOffered(ctx context.Context, campaignID string, v Viewer) bool
	// PreviewInFoundry plays the scene, checked as Check does, on the
	// Director's own Foundry screen. Nothing is stored and no player sees it.
	PreviewInFoundry(ctx context.Context, campaignID string, v Viewer, in Scene) (Scene, error)

	// Audience lists every member with where they are now, for the Show sheet.
	Audience(ctx context.Context, campaignID string, v Viewer) ([]AudienceRow, error)
	// Show makes a showing of the scene as it is now and puts it on the
	// chosen people's screens.
	Show(ctx context.Context, campaignID, sceneID string, v Viewer, in ShowInput) (*Showing, error)
	// Stop pulls a showing off every screen; its rewatch window stays.
	Stop(ctx context.Context, campaignID, showingID string, v Viewer) (*Showing, error)
	// Recent lists the campaign's newest showings with who watched.
	Recent(ctx context.Context, campaignID string, v Viewer) ([]ShowingReport, error)
	// LiveFor lists a scene's showings that may still be on screens.
	LiveFor(ctx context.Context, campaignID, sceneID string, v Viewer) ([]Showing, error)

	// Mine lists the caller's own showings still inside their window.
	Mine(ctx context.Context, campaignID string, v Viewer) ([]MineItem, error)
	// Read returns one showing's scene for the caller to play.
	Read(ctx context.Context, campaignID, showingID string, v Viewer) (*ShowingRead, error)
	// Watched marks the caller's first play of a showing.
	Watched(ctx context.Context, campaignID, showingID string, v Viewer) error
	// MemberShowings lists one member's showings still inside their window,
	// with the scenes, for the Foundry GM client. The sync API checks the
	// caller is a Director before it asks.
	MemberShowings(ctx context.Context, campaignID, userID string) ([]MemberShowing, error)
}

// Presence says where each member is now; implemented in internal/app over
// the websocket hub and the Foundry players report, so this plugin imports
// neither. A member missing from the map is away.
type Presence interface {
	Places(ctx context.Context, campaignID string) (map[string]Place, error)
}

// Members lists the campaign's people; implemented in internal/app over the
// campaigns service.
type Members interface {
	Members(ctx context.Context, campaignID string) ([]Member, error)
}

// Publisher puts a showing on screens and takes it off; implemented in
// internal/app over the websocket bus. It is told after the database has
// the showing, so a screen that asks right away can read it.
type Publisher interface {
	Show(campaignID string, s Showing)
	Stop(campaignID string, s Showing)
}

// FoundryPreviewer plays a Director's scene on their own Foundry screen;
// implemented in internal/app over the websocket hub and bus. Offered says
// whether the campaign uses the Foundry module at all; Preview hands the
// scene to the module's GM-held socket alone and reports whether one was
// connected to hear it.
type FoundryPreviewer interface {
	Offered(ctx context.Context, campaignID string) bool
	Preview(campaignID, directorID string, sc Scene) bool
}

// recentLimit is how many showings the Director's list holds.
const recentLimit = 20

type service struct {
	repo     Repository
	members  Members
	presence Presence
	pub      Publisher
	foundry  FoundryPreviewer
	now      func() time.Time
}

// Option configures the service.
type Option func(*service)

// WithPresence adds the "where are they" signal to the Show sheet; without it
// everyone reads as away.
func WithPresence(p Presence) Option { return func(s *service) { s.presence = p } }

// WithPublisher wires the live channel; without it a showing is only stored,
// and players find it in their Watch again list.
func WithPublisher(p Publisher) Option { return func(s *service) { s.pub = p } }

// WithFoundryPreview wires Preview in Foundry; without it the editor does not
// offer it.
func WithFoundryPreview(p FoundryPreviewer) Option { return func(s *service) { s.foundry = p } }

// WithClock replaces time.Now, for tests.
func WithClock(now func() time.Time) Option { return func(s *service) { s.now = now } }

// NewService builds the service.
func NewService(repo Repository, members Members, opts ...Option) Service {
	s := &service{repo: repo, members: members, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

// notFound is what anyone who may not see a thing is told.
func notFound() error { return apperror.NewNotFound("page not found") }

func mustDirect(v Viewer) error {
	if !v.Director || v.UserID == "" {
		return notFound()
	}
	return nil
}

func (s *service) List(ctx context.Context, campaignID string, v Viewer) ([]Scene, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	return s.repo.ListScenes(ctx, campaignID)
}

func (s *service) Get(ctx context.Context, campaignID, sceneID string, v Viewer) (*Scene, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	return s.repo.GetScene(ctx, campaignID, sceneID)
}

func (s *service) Check(_ context.Context, _ string, v Viewer, in Scene) (Scene, error) {
	if err := mustDirect(v); err != nil {
		return Scene{}, err
	}
	return Normalize(in)
}

func (s *service) FoundryPreviewOffered(ctx context.Context, campaignID string, v Viewer) bool {
	return mustDirect(v) == nil && s.foundry != nil && s.foundry.Offered(ctx, campaignID)
}

// PreviewInFoundry checks the scene first, so Foundry never plays something a
// save would refuse, and says plainly when no Foundry is there to play it.
func (s *service) PreviewInFoundry(ctx context.Context, campaignID string, v Viewer, in Scene) (Scene, error) {
	sc, err := s.Check(ctx, campaignID, v, in)
	if err != nil {
		return Scene{}, err
	}
	if s.foundry == nil || !s.foundry.Offered(ctx, campaignID) {
		return Scene{}, apperror.NewConflict("This campaign isn't connected to Foundry. Connect it under Foundry VTT in the campaign's settings.")
	}
	if !s.foundry.Preview(campaignID, v.UserID, sc) {
		return Scene{}, apperror.NewConflict("Foundry isn't connected right now. Open your world in Foundry as the GM, then try again.")
	}
	return sc, nil
}

func (s *service) Create(ctx context.Context, campaignID string, v Viewer, in Scene) (*Scene, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	sc, err := Normalize(in)
	if err != nil {
		return nil, err
	}
	sc.ID = uuid.NewString()
	if err := s.repo.CreateScene(ctx, campaignID, v.UserID, sc); err != nil {
		return nil, err
	}
	return s.repo.GetScene(ctx, campaignID, sc.ID)
}

func (s *service) Save(ctx context.Context, campaignID, sceneID string, v Viewer, in Scene) (*Scene, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetScene(ctx, campaignID, sceneID); err != nil {
		return nil, err
	}
	sc, err := Normalize(in)
	if err != nil {
		return nil, err
	}
	sc.ID = sceneID
	if err := s.repo.UpdateScene(ctx, campaignID, sc); err != nil {
		return nil, err
	}
	return s.repo.GetScene(ctx, campaignID, sceneID)
}

func (s *service) Delete(ctx context.Context, campaignID, sceneID string, v Viewer) error {
	if err := mustDirect(v); err != nil {
		return err
	}
	return s.repo.DeleteScene(ctx, campaignID, sceneID)
}

func (s *service) Audience(ctx context.Context, campaignID string, v Viewer) ([]AudienceRow, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	members, err := s.members.Members(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	places := map[string]Place{}
	if s.presence != nil {
		// A failed read loses only the "where are they" words; the sheet
		// still lists everyone, as away.
		if p, err := s.presence.Places(ctx, campaignID); err != nil {
			slog.Warn("cutscenes: reading presence", slog.String("campaign_id", campaignID), slog.Any("error", err))
		} else {
			places = p
		}
	}
	out := make([]AudienceRow, 0, len(members))
	for _, m := range members {
		out = append(out, AudienceRow{Member: m, Place: places[m.UserID], You: m.UserID == v.UserID})
	}
	// Players first (they are who a scene is usually for), the Directors
	// after, each by name; the asker last of all.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.You != b.You {
			return !a.You
		}
		if a.Director != b.Director {
			return !a.Director
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out, nil
}

// rewatchHours reads the Show sheet's window; "" is the default.
func rewatchHours(key string) (int, error) {
	if key == "" {
		key = DefaultRewatch
	}
	for _, c := range RewatchChoices {
		if c.Key == key {
			return c.Hours, nil
		}
	}
	return 0, apperror.NewBadRequest("pick how long they can watch it again: 1 hour, tonight or 3 days")
}

func (s *service) Show(ctx context.Context, campaignID, sceneID string, v Viewer, in ShowInput) (*Showing, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	sc, err := s.repo.GetScene(ctx, campaignID, sceneID)
	if err != nil {
		return nil, err
	}
	hours, err := rewatchHours(in.Rewatch)
	if err != nil {
		return nil, err
	}
	members, err := s.members.Members(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(members))
	for _, m := range members {
		names[m.UserID] = m.Name
	}
	seen := map[string]bool{}
	var viewers []ShowingViewer
	for _, id := range in.To {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		name, ok := names[id]
		if !ok {
			// A stale sheet can name someone who has since left; showing to
			// them would leak the scene to a non-member.
			return nil, apperror.NewBadRequest("someone you picked is no longer in this campaign; open the sheet again")
		}
		seen[id] = true
		viewers = append(viewers, ShowingViewer{UserID: id, Name: name})
	}
	if len(viewers) == 0 {
		return nil, apperror.NewBadRequest("pick at least one person to show it to")
	}
	if len(viewers) > MaxViewers {
		return nil, apperror.NewBadRequest(fmt.Sprintf("a showing can be for at most %d people", MaxViewers))
	}
	// The stored scene is shown as it is, re-checked so an old row that no
	// longer passes never reaches a screen.
	snap, err := Normalize(*sc)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Truncate(time.Second)
	sh := Showing{
		ID: uuid.NewString(), CampaignID: campaignID, SceneID: sc.ID, Scene: snap, ShownBy: v.UserID,
		ShownAt: now, RewatchUntil: now.Add(time.Duration(hours) * time.Hour), Viewers: viewers,
	}
	if err := s.repo.CreateShowing(ctx, sh); err != nil {
		return nil, err
	}
	if s.pub != nil {
		s.pub.Show(campaignID, sh)
	}
	return &sh, nil
}

func (s *service) Stop(ctx context.Context, campaignID, showingID string, v Viewer) (*Showing, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	sh, err := s.repo.GetShowing(ctx, campaignID, showingID)
	if err != nil {
		return nil, err
	}
	if sh.StoppedAt == nil {
		now := s.now().UTC().Truncate(time.Second)
		if err := s.repo.StopShowing(ctx, campaignID, showingID, now); err != nil {
			return nil, err
		}
		sh.StoppedAt = &now
		if s.pub != nil {
			s.pub.Stop(campaignID, *sh)
		}
	}
	s.name(ctx, campaignID, []*Showing{sh})
	return sh, nil
}

// name fills viewers' names from the member list; a former member reads as
// "someone who left".
func (s *service) name(ctx context.Context, campaignID string, list []*Showing) {
	members, err := s.members.Members(ctx, campaignID)
	if err != nil {
		slog.Warn("cutscenes: reading members", slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	names := make(map[string]string, len(members))
	for _, m := range members {
		names[m.UserID] = m.Name
	}
	for _, sh := range list {
		for i := range sh.Viewers {
			if n, ok := names[sh.Viewers[i].UserID]; ok {
				sh.Viewers[i].Name = n
			} else {
				sh.Viewers[i].Name = "someone who left"
			}
		}
	}
}

func (s *service) Recent(ctx context.Context, campaignID string, v Viewer) ([]ShowingReport, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	list, err := s.repo.RecentShowings(ctx, campaignID, recentLimit)
	if err != nil {
		return nil, err
	}
	ptrs := make([]*Showing, len(list))
	for i := range list {
		ptrs[i] = &list[i]
	}
	s.name(ctx, campaignID, ptrs)
	now := s.now()
	out := make([]ShowingReport, 0, len(list))
	for _, sh := range list {
		out = append(out, ReportOf(sh, now))
	}
	return out, nil
}

func (s *service) LiveFor(ctx context.Context, campaignID, sceneID string, v Viewer) ([]Showing, error) {
	if err := mustDirect(v); err != nil {
		return nil, err
	}
	list, err := s.repo.RecentShowings(ctx, campaignID, recentLimit)
	if err != nil {
		return nil, err
	}
	now := s.now()
	var out []Showing
	for _, sh := range list {
		if sh.SceneID == sceneID && sh.Live(now) {
			out = append(out, sh)
		}
	}
	ptrs := make([]*Showing, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	s.name(ctx, campaignID, ptrs)
	return out, nil
}

func (s *service) Mine(ctx context.Context, campaignID string, v Viewer) ([]MineItem, error) {
	if v.UserID == "" {
		return nil, notFound()
	}
	list, err := s.repo.ViewerShowings(ctx, campaignID, v.UserID, s.now())
	if err != nil {
		return nil, err
	}
	out := make([]MineItem, 0, len(list))
	for _, sh := range list {
		watched := len(sh.Viewers) > 0 && sh.Viewers[0].WatchedAt != nil
		out = append(out, MineItem{ShowingID: sh.ID, Name: sh.Scene.Name, ShownAt: sh.ShownAt, RewatchUntil: sh.RewatchUntil, Watched: watched, Stopped: sh.StoppedAt != nil})
	}
	return out, nil
}

// readable returns the showing when v may play it now: a Director always, a
// viewer while the window is open. Everyone else gets the same 404 as a
// missing id, so asking says nothing about whether it exists.
func (s *service) readable(ctx context.Context, campaignID, showingID string, v Viewer) (*Showing, error) {
	if v.UserID == "" {
		return nil, notFound()
	}
	sh, err := s.repo.GetShowing(ctx, campaignID, showingID)
	if err != nil {
		if apperror.SafeCode(err) == 404 {
			return nil, notFound()
		}
		return nil, err
	}
	if v.Director {
		return sh, nil
	}
	if !s.now().Before(sh.RewatchUntil) {
		return nil, notFound()
	}
	for _, vw := range sh.Viewers {
		if vw.UserID == v.UserID {
			return sh, nil
		}
	}
	return nil, notFound()
}

func (s *service) Read(ctx context.Context, campaignID, showingID string, v Viewer) (*ShowingRead, error) {
	sh, err := s.readable(ctx, campaignID, showingID, v)
	if err != nil {
		return nil, err
	}
	return &ShowingRead{ShowingID: sh.ID, RewatchUntil: sh.RewatchUntil, Scene: sh.Scene}, nil
}

func (s *service) Watched(ctx context.Context, campaignID, showingID string, v Viewer) error {
	sh, err := s.readable(ctx, campaignID, showingID, v)
	if err != nil {
		return err
	}
	// A Director previewing someone else's showing is not one of its viewers;
	// the update then matches no row and changes nothing.
	return s.repo.MarkWatched(ctx, campaignID, sh.ID, v.UserID, s.now())
}

func (s *service) MemberShowings(ctx context.Context, campaignID, userID string) ([]MemberShowing, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, apperror.NewBadRequest("name the member: ?user=<chronicle user id>")
	}
	list, err := s.repo.ViewerShowings(ctx, campaignID, userID, s.now())
	if err != nil {
		return nil, err
	}
	out := make([]MemberShowing, 0, len(list))
	for _, sh := range list {
		out = append(out, MemberShowing{ShowingID: sh.ID, Name: sh.Scene.Name, ShownAt: sh.ShownAt, RewatchUntil: sh.RewatchUntil, Scene: sh.Scene})
	}
	return out, nil
}
