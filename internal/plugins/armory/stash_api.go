// stash_api.go is the stash surface for the sync API. Foundry holds one GM key
// but speaks for whichever player is at the table, so every call can name the
// member it acts for and is then answered under THAT member's rules.
//
// Why a named member is safe: only a caller who is Owner or co-DM may name
// someone else, and such a caller can already do anything here. Naming a player
// can only narrow what the call may do (their stashes, their characters, their
// need for approval). A name that is not a current member is refused as not
// found, so the call cannot be steered at an outsider.
package armory

import (
	"context"
	"strconv"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// MemberDirectory answers the membership questions acting-as needs.
// Implemented over the campaigns service in internal/app.
type MemberDirectory interface {
	// MemberRole returns the member's campaign role (permissions.Role* level);
	// ok is false when the user is not a current member.
	MemberRole(ctx context.Context, campaignID, userID string) (role int, ok bool, err error)
	// IsDmGranted reports whether the user holds a co-DM grant.
	IsDmGranted(ctx context.Context, campaignID, userID string) (bool, error)
}

// StashAPI adapts StashService to the sync API's wire shapes. It holds no
// rules of its own beyond choosing who the caller acts as.
type StashAPI struct {
	svc     StashService
	members MemberDirectory
}

// NewStashAPI creates the sync API facade.
func NewStashAPI(svc StashService, members MemberDirectory) *StashAPI {
	return &StashAPI{svc: svc, members: members}
}

// ActorFor builds the Actor a call runs as: the named member when actingUserID
// is set, else the caller. Roles are real campaign roles, promoted to Owner
// visibility only by a real co-DM grant, exactly as the web layer's
// VisibilityRole does. A non-member is NotFound.
//
// Only a caller who is themselves Owner or co-DM may name someone else: the
// same routes also answer browser sessions, where the caller can be a player,
// and naming the GM must never lift a player to the GM's rights.
func (a *StashAPI) ActorFor(ctx context.Context, campaignID, keyUserID, actingUserID string) (Actor, error) {
	caller, err := a.memberActor(ctx, campaignID, keyUserID)
	if err != nil {
		return Actor{}, err
	}
	if actingUserID == "" || actingUserID == keyUserID {
		return caller, nil
	}
	if !caller.IsOwner() {
		return Actor{}, forbidden()
	}
	return a.memberActor(ctx, campaignID, actingUserID)
}

// memberActor resolves one user's real role in the campaign.
func (a *StashAPI) memberActor(ctx context.Context, campaignID, uid string) (Actor, error) {
	if uid == "" {
		return Actor{}, notFound("member")
	}
	role, ok, err := a.members.MemberRole(ctx, campaignID, uid)
	if err != nil {
		return Actor{}, err
	}
	if !ok {
		return Actor{}, notFound("member")
	}
	if role < permissions.RoleOwner {
		granted, err := a.members.IsDmGranted(ctx, campaignID, uid)
		if err != nil {
			return Actor{}, err
		}
		if granted {
			role = permissions.RoleOwner
		}
	}
	return Actor{UserID: uid, Role: role}, nil
}

// --- wire shapes (camelCase JSON) ---

// APIItem is one item line.
type APIItem struct {
	ItemID   string `json:"itemId"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// APICharacter is the acting member's view of one character.
type APICharacter struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	MoneyKey string    `json:"moneyKey"`
	Money    float64   `json:"money"`
	Items    []APIItem `json:"items"`
}

// APIStash is a stash the acting member can see.
type APIStash struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Money float64   `json:"money"`
	Items []APIItem `json:"items"`
}

// APIDestination is a place a move can go.
type APIDestination struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// APIStashView is the answer to GET /stashes/view.
type APIStashView struct {
	DowntimeOpen bool             `json:"downtimeOpen"`
	Character    APICharacter     `json:"character"`
	Stashes      []APIStash       `json:"stashes"`
	Destinations []APIDestination `json:"destinations"`
	CanApprove   bool             `json:"canApprove"`
}

// APIEndpoint names one side of a move.
type APIEndpoint struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// APIMove is one move row.
type APIMove struct {
	ID          int64       `json:"id"`
	Kind        string      `json:"kind"`
	ItemID      string      `json:"itemId,omitempty"`
	Quantity    int         `json:"quantity,omitempty"`
	Amount      float64     `json:"amount,omitempty"`
	From        APIEndpoint `json:"from"`
	To          APIEndpoint `json:"to"`
	Status      string      `json:"status"`
	Reason      string      `json:"reason,omitempty"`
	RequestedBy string      `json:"requestedBy"`
	DecidedBy   string      `json:"decidedBy,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
}

// APIMoveLine is a move with names resolved (and redacted) for the actor.
type APIMoveLine struct {
	APIMove
	ItemName      string `json:"itemName,omitempty"`
	FromName      string `json:"fromName"`
	ToName        string `json:"toName"`
	RequesterName string `json:"requesterName"`
	Summary       string `json:"summary"`
}

// APIMoveRequest is the body of POST /stashes/moves.
type APIMoveRequest struct {
	ActingUserID string
	Kind         string
	ItemID       string
	Quantity     int
	Amount       string
	From         APIEndpoint
	To           APIEndpoint
}

// APIMoveResult answers a move, an approval or a decline.
type APIMoveResult struct {
	Status string  `json:"status"`
	Move   APIMove `json:"move"`
}

// APIHistory is a list of move lines.
type APIHistory struct {
	History []APIMoveLine `json:"history"`
}

// APIRequests is the pending-request list.
type APIRequests struct {
	Requests []APIMoveLine `json:"requests"`
}

// APIDowntime is the downtime switch; Applied and Failed appear after a
// change that opened it and swept the waiting requests.
type APIDowntime struct {
	Open    bool `json:"open"`
	Applied int  `json:"applied,omitempty"`
	Failed  int  `json:"failed,omitempty"`
}

func units(c Cents) float64 { return float64(c) / 100 }

func apiItems(in []HeldItem) []APIItem {
	out := make([]APIItem, 0, len(in))
	for _, it := range in {
		out = append(out, APIItem{ItemID: it.ItemID, Name: it.Name, Quantity: it.Quantity})
	}
	return out
}

func apiMove(m Move) APIMove {
	return APIMove{
		ID: m.ID, Kind: m.Kind, ItemID: m.ItemEntityID, Quantity: m.Quantity, Amount: units(m.Amount),
		From:   APIEndpoint{Kind: m.From.Kind, ID: m.From.ID},
		To:     APIEndpoint{Kind: m.To.Kind, ID: m.To.ID},
		Status: m.Status, Reason: m.Reason, RequestedBy: m.RequestedBy, DecidedBy: m.DecidedBy,
		CreatedAt: m.CreatedAt,
	}
}

func apiLines(in []MoveLine) []APIMoveLine {
	out := make([]APIMoveLine, 0, len(in))
	for _, l := range in {
		// Purchase requests share the web history but are not moves: their ids
		// are not move ids and they have no kind, so the API leaves them out.
		if l.Kind == "" {
			continue
		}
		out = append(out, APIMoveLine{
			APIMove: apiMove(l.Move), ItemName: l.ItemName, FromName: l.FromName, ToName: l.ToName,
			RequesterName: l.RequesterName, Summary: MoveSummary(l),
		})
	}
	return out
}

// --- operations ---

// View returns what the acting member sees for one character: it is the web
// panel and stashes page computed for that member, so the same rules and the
// same redaction apply.
func (a *StashAPI) View(ctx context.Context, campaignID, keyUserID, actingUserID, characterID string) (*APIStashView, error) {
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	panel, err := a.svc.CharacterPanel(ctx, campaignID, actor, characterID)
	if err != nil {
		return nil, err
	}
	if panel == nil {
		return nil, notFound("character")
	}
	page, err := a.svc.StashesPage(ctx, campaignID, actor)
	if err != nil {
		return nil, err
	}
	from := Endpoint{Kind: EndpointCharacter, ID: panel.Character.ID}
	dests, err := a.svc.Destinations(ctx, campaignID, actor, MoveKindItem, from)
	if err != nil {
		return nil, err
	}

	view := &APIStashView{
		DowntimeOpen: panel.DowntimeOpen,
		CanApprove:   page.CanApprove,
		Character: APICharacter{
			ID: panel.Character.ID, Name: panel.Character.Name, MoneyKey: panel.Character.MoneyKey,
			Money: units(panel.Money), Items: apiItems(panel.Items),
		},
		Stashes:      make([]APIStash, 0, len(page.Stashes)),
		Destinations: make([]APIDestination, 0, len(dests)),
	}
	for _, st := range page.Stashes {
		items := make([]APIItem, 0, len(st.Items))
		for _, it := range st.Items {
			items = append(items, APIItem(it))
		}
		view.Stashes = append(view.Stashes, APIStash{
			ID: strconv.Itoa(st.ID), Name: st.Name, Money: units(st.Money), Items: items,
		})
	}
	for _, d := range dests {
		view.Destinations = append(view.Destinations, APIDestination{Kind: d.Endpoint.Kind, ID: d.Endpoint.ID, Name: d.Label})
	}
	return view, nil
}

// Move applies or queues a move as the acting member.
func (a *StashAPI) Move(ctx context.Context, campaignID, keyUserID string, req APIMoveRequest) (*APIMoveResult, error) {
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, req.ActingUserID)
	if err != nil {
		return nil, err
	}
	out, err := a.svc.Move(ctx, campaignID, actor, MoveInput{
		Kind: req.Kind, ItemEntityID: req.ItemID, Quantity: req.Quantity, Amount: req.Amount,
		From: Endpoint{Kind: req.From.Kind, ID: req.From.ID},
		To:   Endpoint{Kind: req.To.Kind, ID: req.To.ID},
	})
	if err != nil {
		return nil, err
	}
	return &APIMoveResult{Status: out.Move.Status, Move: apiMove(out.Move)}, nil
}

// History returns the move history of one character or one stash, as the
// acting member may see it. Exactly one of characterID and stashID is needed.
func (a *StashAPI) History(ctx context.Context, campaignID, keyUserID, actingUserID, characterID, stashID string) (*APIHistory, error) {
	if (characterID == "") == (stashID == "") {
		return nil, apperror.NewBadRequest("give either characterId or stashId")
	}
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	var lines []MoveLine
	if characterID != "" {
		lines, err = a.svc.CharacterHistory(ctx, campaignID, actor, characterID)
	} else {
		sid, convErr := strconv.Atoi(stashID)
		if convErr != nil {
			return nil, notFound("stash")
		}
		lines, err = a.svc.StashHistory(ctx, campaignID, actor, sid)
	}
	if err != nil {
		return nil, err
	}
	return &APIHistory{History: apiLines(lines)}, nil
}

// Requests lists the pending requests; the acting member must be able to
// answer them.
func (a *StashAPI) Requests(ctx context.Context, campaignID, keyUserID, actingUserID string) (*APIRequests, error) {
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	lines, err := a.svc.PendingRequests(ctx, campaignID, actor)
	if err != nil {
		return nil, err
	}
	return &APIRequests{Requests: apiLines(lines)}, nil
}

// Approve answers a pending request with a yes, as the acting member.
func (a *StashAPI) Approve(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (*APIMoveResult, error) {
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	m, err := a.svc.Approve(ctx, campaignID, actor, moveID)
	if err != nil {
		return nil, err
	}
	return &APIMoveResult{Status: m.Status, Move: apiMove(*m)}, nil
}

// Decline answers a pending request with a no, as the acting member.
func (a *StashAPI) Decline(ctx context.Context, campaignID, keyUserID, actingUserID string, moveID int64) (*APIMoveResult, error) {
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	m, err := a.svc.Decline(ctx, campaignID, actor, moveID)
	if err != nil {
		return nil, err
	}
	return &APIMoveResult{Status: m.Status, Move: apiMove(*m)}, nil
}

// Downtime reports the switch. It is campaign-wide and not secret, so no
// acting member is needed.
func (a *StashAPI) Downtime(ctx context.Context, campaignID string) (*APIDowntime, error) {
	open, err := a.svc.IsDowntimeOpen(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return &APIDowntime{Open: open}, nil
}

// SetDowntime opens or closes downtime; only an approver may.
func (a *StashAPI) SetDowntime(ctx context.Context, campaignID, keyUserID, actingUserID string, open bool) (*APIDowntime, error) {
	actor, err := a.ActorFor(ctx, campaignID, keyUserID, actingUserID)
	if err != nil {
		return nil, err
	}
	res, err := a.svc.SetDowntime(ctx, campaignID, actor, open)
	if err != nil {
		return nil, err
	}
	return &APIDowntime{Open: open, Applied: res.Applied, Failed: res.Failed}, nil
}
