package connector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// keyPrefix starts every connection key, so a key pasted in the wrong place
// is recognisable and never mistaken for a sync API key (chron_).
const keyPrefix = "chai_"

// keyBytes is the key's randomness: 256 bits, so the key can be kept as a
// plain SHA-256 (no slow hash needed) and can't be guessed.
const keyBytes = 32

// keyLen is the length of a whole key: the prefix and the random part in
// unpadded base64url.
var keyLen = len(keyPrefix) + base64.RawURLEncoding.EncodedLen(keyBytes)

// Why a request was refused. The endpoint turns each into a plain message
// for the owner, who reads it in their AI app.
var (
	// ErrBadKey: no key, a malformed one, or one that opens no campaign at
	// this address. One error for all, so a refusal never says which.
	ErrBadKey = errors.New("this key doesn't open this campaign")
	// ErrKeyEnded: the key was disconnected or ran out.
	ErrKeyEnded = errors.New("this key was disconnected or has run out")
	// ErrOwnerGone: whoever made the key no longer owns the campaign.
	ErrOwnerGone = errors.New("whoever made this key no longer owns the campaign")
	// ErrNotOwner: Check found the user is not the campaign's owner.
	ErrNotOwner = errors.New("not the campaign's owner")
)

// Owners checks that a key's maker still owns its campaign, on every request.
type Owners interface {
	// Check returns the campaign's name when userID is its owner and the
	// campaign is not deleted or in the Trash; otherwise an error
	// (ErrNotOwner when they simply aren't the owner).
	Check(ctx context.Context, campaignID, userID string) (campaignName string, err error)
}

// Service makes, checks and revokes connection keys and keeps the call log.
type Service struct {
	repo   Repository
	owners Owners
	now    func() time.Time
	limits *limiter
	calls  atomic.Int64
	check  ChangesCheck
}

// ChangesCheck looks at what an AI asks for against the campaign and returns
// the problems it can fix itself; nil means none. An error only skips the
// check, since the owner's review checks everything again.
type ChangesCheck func(ctx context.Context, campaignID, changes string) ([]string, error)

// SetCheck adds a check that runs when an AI asks for changes. Optional.
func (s *Service) SetCheck(f ChangesCheck) { s.check = f }

// NewService creates the connector service.
func NewService(repo Repository, owners Owners) *Service {
	return &Service{repo: repo, owners: owners, now: time.Now, limits: newLimiter()}
}

// hashKey is the stored form of a key.
func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newKey() (string, error) {
	b := make([]byte, keyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return keyPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Connect makes a new key for the campaign and revokes any earlier one, so a
// campaign has one connection at a time. The key is returned once and never
// kept. Only Safe and Permitted can be chosen: lookups read the same in
// Permitted and Everything, so a third choice would mean nothing. canAsk is
// the Change setting.
func (s *Service) Connect(ctx context.Context, campaignID, userID string, privacy aiexport.PrivacyMode, canAsk bool) (string, *Connection, error) {
	if privacy != aiexport.PrivacyModeSafe && privacy != aiexport.PrivacyModePermitted {
		return "", nil, apperror.NewBadRequest("pick what your AI may see")
	}
	if _, err := s.owners.Check(ctx, campaignID, userID); err != nil {
		return "", nil, apperror.NewForbidden("only the campaign's owner can connect an AI")
	}
	raw, err := newKey()
	if err != nil {
		return "", nil, apperror.NewInternal(fmt.Errorf("ai connector: making key: %w", err))
	}
	now := s.now().UTC().Truncate(time.Second)
	c := &Connection{
		CampaignID: campaignID,
		UserID:     userID,
		KeyHash:    hashKey(raw),
		KeyHint:    raw[len(raw)-4:],
		Privacy:    privacy,
		CanAsk:     canAsk,
		CreatedAt:  now,
		ExpiresAt:  now.Add(KeyLifetime),
	}
	id, err := s.repo.Replace(ctx, c)
	if err != nil {
		return "", nil, err
	}
	c.ID = id
	return raw, c, nil
}

// Current is the campaign's connection, or nil when there is none (an
// expired one is still returned, so the page can say it ran out).
func (s *Service) Current(ctx context.Context, campaignID string) (*Connection, error) {
	return s.repo.Current(ctx, campaignID)
}

// Disconnect revokes the campaign's connection; the key stops working at once.
func (s *Service) Disconnect(ctx context.Context, campaignID string) error {
	return s.repo.RevokeAll(ctx, campaignID, s.now().UTC().Truncate(time.Second))
}

// Authenticate checks a key against the campaign in the address. It returns
// the connection and the campaign's name, or ErrBadKey, ErrKeyEnded or
// ErrOwnerGone.
func (s *Service) Authenticate(ctx context.Context, campaignID, raw string) (*Connection, string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) != keyLen || !strings.HasPrefix(raw, keyPrefix) || campaignID == "" {
		return nil, "", ErrBadKey
	}
	hash := hashKey(raw)
	c, err := s.repo.FindByHash(ctx, hash)
	if err != nil {
		return nil, "", err
	}
	if c == nil || subtle.ConstantTimeCompare([]byte(c.KeyHash), []byte(hash)) != 1 || c.CampaignID != campaignID {
		return nil, "", ErrBadKey
	}
	if !c.Live(s.now()) {
		return nil, "", ErrKeyEnded
	}
	name, err := s.owners.Check(ctx, c.CampaignID, c.UserID)
	if err != nil {
		if !errors.Is(err, ErrNotOwner) {
			slog.Warn("ai connector: owner check failed", slog.Int64("connection", c.ID), slog.Any("error", err))
		}
		return nil, "", ErrOwnerGone
	}
	return c, name, nil
}

// Allow spends one call of the connection's budget, or says how long to wait.
func (s *Service) Allow(connID int64) (bool, time.Duration) {
	return s.limits.allow(connID, s.now())
}

// Seen stamps the connection as used, with the name the AI app gave itself
// when it connected ("" keeps the stored one).
func (s *Service) Seen(ctx context.Context, c *Connection, clientName string) {
	if err := s.repo.MarkUsed(ctx, c.ID, s.now(), cleanClientName(clientName)); err != nil {
		slog.Warn("ai connector: marking used", slog.Int64("connection", c.ID), slog.Any("error", err))
	}
}

// Record logs one tool call for "What it did". A failure is logged, never
// returned: the AI's answer doesn't wait on the log.
func (s *Service) Record(ctx context.Context, c *Connection, tool, summary, note string, ok bool) {
	call := &Call{
		ConnectionID: c.ID, CampaignID: c.CampaignID, At: s.now().UTC(),
		Tool: oneLine(tool, 40), Summary: oneLine(summary, summaryMax), Note: oneLine(note, noteMax), OK: ok,
	}
	if err := s.repo.AddCall(ctx, call); err != nil {
		slog.Warn("ai connector: logging call", slog.Int64("connection", c.ID), slog.Any("error", err))
		return
	}
	if s.calls.Add(1)%50 == 0 {
		if err := s.repo.PruneCalls(ctx, c.CampaignID, callsKept); err != nil {
			slog.Warn("ai connector: pruning calls", slog.Any("error", err))
		}
	}
}

// Activity is the campaign's newest calls, for the page.
func (s *Service) Activity(ctx context.Context, campaignID string) ([]Call, error) {
	return s.repo.RecentCalls(ctx, campaignID, CallsShown)
}

// ErrCannotAsk: the connection's Change setting is Only look.
var ErrCannotAsk = errors.New("this connection may only look things up")

// Ask queues what an AI asked for, for the owner to review. Nothing in it is
// applied here or anywhere else until the owner presses Apply on the review
// screen. A request the review couldn't use is refused now, with the reason,
// so the AI can fix it instead of leaving the owner a broken one.
func (s *Service) Ask(ctx context.Context, c *Connection, summary, changes string) (*Request, error) {
	if !c.CanAsk {
		return nil, ErrCannotAsk
	}
	counts, blocks, err := checkChanges(changes)
	if err != nil {
		return nil, err
	}
	if s.check != nil {
		problems, err := s.check(ctx, c.CampaignID, changes)
		if err != nil {
			slog.Warn("ai-connector: changes check failed", slog.String("campaign_id", c.CampaignID), slog.Any("error", err))
		} else if len(problems) > 0 {
			return nil, unusable(problems)
		}
	}
	app := c.ClientName
	if app == "" {
		app = "Your AI"
	}
	q := &Request{
		CampaignID: c.CampaignID, ConnectionID: c.ID, AppName: cleanClientName(app),
		Summary: oneLine(summary, summaryMax), Counts: counts, Blocks: blocks, Body: changes,
		Status: RequestWaiting, CreatedAt: s.now().UTC().Truncate(time.Second),
	}
	id, err := s.repo.AddRequest(ctx, q, requestsWaiting)
	if err != nil {
		return nil, err
	}
	q.ID = id
	return q, nil
}

// Waiting is the campaign's requests waiting for the owner, newest first.
func (s *Service) Waiting(ctx context.Context, campaignID string) ([]Request, error) {
	return s.repo.Requests(ctx, campaignID, RequestWaiting, requestsWaiting)
}

// WaitingRequest is one waiting request of the campaign, with its blocks, or
// an error the owner can read.
func (s *Service) WaitingRequest(ctx context.Context, campaignID string, id int64) (*Request, error) {
	q, err := s.repo.Request(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, apperror.NewNotFound("that request isn't here any more")
	}
	if q.Status != RequestWaiting {
		return nil, apperror.NewBadRequest("you already answered that request")
	}
	return q, nil
}

// Answered marks a request reviewed and applied; outcome says what was.
// Answering one twice changes nothing.
func (s *Service) Answered(ctx context.Context, campaignID string, id int64, outcome string) error {
	return s.finish(ctx, campaignID, id, RequestAnswered, outcome)
}

// TurnDown answers a request No as a whole, applying nothing.
func (s *Service) TurnDown(ctx context.Context, campaignID string, id int64) error {
	return s.finish(ctx, campaignID, id, RequestTurnedDown, "nothing changed")
}

func (s *Service) finish(ctx context.Context, campaignID string, id int64, status, outcome string) error {
	if _, err := s.repo.FinishRequest(ctx, campaignID, id, status, oneLine(outcome, noteMax), s.now()); err != nil {
		return err
	}
	if err := s.repo.PruneRequests(ctx, campaignID, requestsKept); err != nil {
		slog.Warn("ai connector: pruning requests", slog.Any("error", err))
	}
	return nil
}

// checkChanges reads a request's blocks as the review screen will, and
// counts what they would do. Lookups and part labels belong to copy and
// paste, so they are refused, as is any block that would not parse.
func checkChanges(changes string) (counts string, blocks int, err error) {
	if strings.TrimSpace(changes) == "" {
		return "", 0, errors.New("changes is empty")
	}
	if len(changes) > changesMax {
		return "", 0, fmt.Errorf("changes is too long (at most %d KB); send it in smaller requests", changesMax/1024)
	}
	var add, change, remove int
	var problems []string
	for _, p := range importer.Parse(changes) {
		n := p.SourceIndex + 1
		kind := strings.ToLower(strings.TrimSpace(p.FrontMatter.Kind))
		switch {
		case p.Status == importer.StatusParseError:
			problems = append(problems, fmt.Sprintf("block %d: %s", n, strings.TrimRight(p.ParseError, ".")))
			continue
		case kind == records.KindLookup:
			problems = append(problems, fmt.Sprintf("block %d is a lookup; use the lookup tools instead", n))
			continue
		case kind == importer.KindPart:
			problems = append(problems, fmt.Sprintf("block %d is a part label; send the whole answer in one request", n))
			continue
		}
		switch strings.ToLower(strings.TrimSpace(p.FrontMatter.Action)) {
		case importer.ActionDelete:
			remove++
		case importer.ActionUpdate:
			change++
		default:
			add++
		}
	}
	if len(problems) > 0 {
		return "", 0, unusable(problems)
	}
	blocks = add + change + remove
	if blocks == 0 {
		return "", 0, errors.New("changes has no blocks")
	}
	var parts []string
	for _, x := range []struct {
		n    int
		what string
	}{{add, "to add"}, {change, "to change"}, {remove, "to remove"}} {
		if x.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", x.n, x.what))
		}
	}
	return strings.Join(parts, ", "), blocks, nil
}

// unusable refuses a request with its first few problems, for the AI to fix.
func unusable(problems []string) error {
	if len(problems) > 5 {
		problems = append(problems[:5:5], fmt.Sprintf("and %d more", len(problems)-5))
	}
	return errors.New("the owner couldn't use it: " + strings.Join(problems, "; "))
}

// cleanClientName keeps the plain characters of an app's name, cut short.
func cleanClientName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == ' ', r == '-', r == '_', r == '.', r == '(', r == ')', r == '/':
			b.WriteRune(r)
		}
		if b.Len() >= clientNameMax {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

// oneLine flattens s to one line of at most n bytes, cut on a rune boundary.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := n - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// limiter is a per-connection budget of calls per minute and per day, kept in
// memory: a restart forgets it, which only ever gives a connection a fresh
// budget, never more than one window's worth at once.
type limiter struct {
	mu      sync.Mutex
	windows map[int64]*window
}

type window struct {
	minuteStart, dayStart time.Time
	minute, day           int
}

func newLimiter() *limiter { return &limiter{windows: map[int64]*window{}} }

func (l *limiter) allow(id int64, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.windows) > 10000 {
		for k, w := range l.windows {
			if now.Sub(w.dayStart) > 24*time.Hour {
				delete(l.windows, k)
			}
		}
	}
	w := l.windows[id]
	if w == nil {
		w = &window{minuteStart: now, dayStart: now}
		l.windows[id] = w
	}
	if now.Sub(w.minuteStart) >= time.Minute {
		w.minuteStart, w.minute = now, 0
	}
	if now.Sub(w.dayStart) >= 24*time.Hour {
		w.dayStart, w.day = now, 0
	}
	if w.day >= callsPerDay {
		return false, w.dayStart.Add(24 * time.Hour).Sub(now)
	}
	if w.minute >= callsPerMinute {
		return false, w.minuteStart.Add(time.Minute).Sub(now)
	}
	w.minute++
	w.day++
	return true, 0
}
