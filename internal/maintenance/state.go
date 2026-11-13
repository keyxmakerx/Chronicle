// Package maintenance holds the site-wide "back in a few minutes" mode used
// while a restore replaces the database. One State is shared by the global
// gate middleware (gate.go), the restore plugin that drives it, and the
// health probe that reports it.
//
// The state is mirrored to a small marker file so that a process restart in
// the middle of a restore comes back closed instead of serving a half-loaded
// database. The same file is what scripts/restore.sh reads to confirm that
// Chronicle itself, not a human, asked it to run against a live site.
package maintenance

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// MarkerName is the marker file's basename inside the backup directory.
const MarkerName = ".chronicle-maintenance"

// Phase is where a restore is in its life.
type Phase string

const (
	// PhaseIdle means no restore has run since boot (or the marker was cleared).
	PhaseIdle Phase = "idle"
	// PhaseRunning means the site is closed and the restore is working.
	PhaseRunning Phase = "running"
	// PhaseDone means the restore finished and the site is open again.
	PhaseDone Phase = "done"
	// PhaseFailed means the restore stopped with an error; the site stays closed.
	PhaseFailed Phase = "failed"
	// PhaseInterrupted means the process restarted while a restore was running.
	PhaseInterrupted Phase = "interrupted"
)

// Errors returned by State transitions.
var (
	ErrBusy       = errors.New("maintenance: a restore is already in progress")
	ErrNotActive  = errors.New("maintenance: not in maintenance")
	ErrStillBusy  = errors.New("maintenance: the restore is still running")
	errBadSession = errors.New("maintenance: no session to tie the restore to")
)

// Snapshot is a copy of the state, safe to hand to templates and JSON.
type Snapshot struct {
	// Active is true while the gate is closed.
	Active bool
	Phase  Phase
	// Step is a short plain-language line for what is happening now.
	Step     string
	Manifest string
	// Actor is the user id of the admin who started the restore.
	Actor      string
	StartedAt  time.Time
	FinishedAt time.Time
	// Safety is the basename of the manifest holding the copy of the
	// database taken just before it was replaced; empty if none was made.
	Safety string
	// DBTouched is true once the script began replacing the database.
	DBTouched bool
	ExitCode  int
	// Summary is the plain-language outcome.
	Summary string
	// Tail is the end of the script's output, kept for the failure page.
	Tail string
	// Left is true when an admin left maintenance without a finished restore.
	Left bool
}

// State is the shared maintenance state. The zero value is not usable; use
// NewState.
type State struct {
	markerPath string

	mu         sync.Mutex
	snap       Snapshot
	ownerHash  string
	leaveToken string
	runToken   string
	owner      OwnerHandler

	inflight atomic.Int64
	now      func() time.Time
}

// NewState builds a State and reads the marker. A marker left by a process
// that died mid-restore (state=running) comes back as interrupted; one left by
// a failed restore (state=failed) comes back as failed. Either way the gate
// starts closed. An unreadable or malformed marker is treated as closed too:
// the only way a marker exists is that a restore began, so serving traffic
// against an unknown database is the worse mistake.
func NewState(markerPath string) *State {
	s := &State{markerPath: markerPath, now: time.Now}
	s.snap.Phase = PhaseIdle
	s.loadMarker()
	return s
}

// MarkerExists reports whether a restore marker is on disk. main reads it
// before the app exists, to tolerate a database a failed restore left behind.
func MarkerExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// MarkerPath returns where the marker file lives.
func (s *State) MarkerPath() string { return s.markerPath }

// Active reports whether the gate is closed.
func (s *State) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap.Active
}

// Snapshot returns a copy of the current state.
func (s *State) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// RunToken is the secret handed to the restore script so it can prove, with
// the marker, that Chronicle started it. Empty when not in maintenance.
func (s *State) RunToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runToken
}

// LeaveToken is the one-time value the admin's page must send back to leave
// maintenance. It is shown only to the owning admin session.
func (s *State) LeaveToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leaveToken
}

// CheckLeaveToken compares in constant time.
func (s *State) CheckLeaveToken(got string) bool {
	s.mu.Lock()
	want := s.leaveToken
	s.mu.Unlock()
	if want == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// Enter closes the gate for a restore started by actor from the browser
// session whose hash is ownerHash. It writes the marker first and refuses to
// close the gate if that fails, because a restore that cannot be recorded
// could not be told apart from a normal boot after a crash.
func (s *State) Enter(ownerHash, actor, manifest string) error {
	if !validHash(ownerHash) {
		return errBadSession
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snap.Active {
		return ErrBusy
	}
	run, err := randomHex(16)
	if err != nil {
		return err
	}
	leave, err := randomHex(16)
	if err != nil {
		return err
	}
	next := Snapshot{
		Active:    true,
		Phase:     PhaseRunning,
		Step:      "Getting ready",
		Manifest:  manifest,
		Actor:     actor,
		StartedAt: s.now(),
	}
	prevOwner, prevLeave, prevRun := s.ownerHash, s.leaveToken, s.runToken
	s.ownerHash, s.leaveToken, s.runToken = ownerHash, leave, run
	if err := s.writeMarkerLocked("running", next); err != nil {
		s.ownerHash, s.leaveToken, s.runToken = prevOwner, prevLeave, prevRun
		return fmt.Errorf("writing maintenance marker: %w", err)
	}
	s.snap = next
	return nil
}

// SetStep updates the plain-language progress line.
func (s *State) SetStep(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snap.Active && s.snap.Phase == PhaseRunning {
		s.snap.Step = step
	}
}

// NoteSafety records the safety copy's manifest name and persists it, so a
// restart mid-restore can still point at it.
func (s *State) NoteSafety(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.snap.Active {
		return
	}
	s.snap.Safety = name
	_ = s.writeMarkerLocked("running", s.snap)
}

// NoteDBTouched records that the database is being replaced; from here a
// stop leaves it in an unknown state.
func (s *State) NoteDBTouched() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.snap.Active {
		return
	}
	s.snap.DBTouched = true
	_ = s.writeMarkerLocked("running", s.snap)
}

// Succeed ends the restore, removes the marker and opens the gate.
func (s *State) Succeed(summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Phase = PhaseDone
	s.snap.Step = ""
	s.snap.Summary = summary
	s.snap.FinishedAt = s.now()
	s.snap.ExitCode = 0
	s.snap.Active = false
	s.clearLocked()
}

// Fail ends the restore with an error and keeps the gate closed. The marker
// is rewritten as failed, so a restart stays closed too.
func (s *State) Fail(summary, tail string, exitCode int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snap.Phase = PhaseFailed
	s.snap.Step = ""
	s.snap.Summary = summary
	s.snap.Tail = tail
	s.snap.ExitCode = exitCode
	s.snap.FinishedAt = s.now()
	s.snap.Active = true
	_ = s.writeMarkerLocked("failed", s.snap)
	s.writeTailLocked(tail)
}

// Leave opens the gate after a failed or interrupted restore, on the admin's
// say-so. It refuses while the restore is still running.
func (s *State) Leave() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.snap.Active {
		return ErrNotActive
	}
	if s.snap.Phase == PhaseRunning {
		return ErrStillBusy
	}
	s.snap.Active = false
	s.snap.Left = true
	s.snap.FinishedAt = s.now()
	s.clearLocked()
	return nil
}

// clearLocked removes the marker, the tail file and the secrets.
func (s *State) clearLocked() {
	s.ownerHash, s.leaveToken, s.runToken = "", "", ""
	if s.markerPath != "" {
		_ = os.Remove(s.markerPath)
		_ = os.Remove(s.markerPath + ".log")
	}
}

// IsOwner reports whether the request carries the session that started the
// restore. The bypass is keyed to the hash of that session's token, never to
// a cookie value an outsider could pick, and only while the gate is closed.
func (s *State) IsOwner(r *http.Request) bool {
	h := SessionHash(r)
	if h == "" {
		return false
	}
	s.mu.Lock()
	want := s.ownerHash
	active := s.snap.Active
	s.mu.Unlock()
	if !active || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(h)) == 1
}

// Track marks a request as in flight; call the returned func when it ends.
func (s *State) Track() func() {
	s.inflight.Add(1)
	return func() { s.inflight.Add(-1) }
}

// WaitIdle blocks until no tracked request is running or max elapses, and
// reports whether it went idle. The restore script starts only after this, so
// a slow request does not write into the database as it is being dropped.
func (s *State) WaitIdle(max time.Duration) bool {
	deadline := time.Now().Add(max)
	for {
		if s.inflight.Load() <= 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// sessionCookieNames mirrors the auth plugin's session cookie names (bare for
// plain HTTP, __Host- prefixed behind TLS); internal/app pins them equal.
var sessionCookieNames = []string{"__Host-chronicle_session", "chronicle_session"}

// SessionCookieNames returns the cookie names the gate reads a session from.
func SessionCookieNames() []string {
	return append([]string(nil), sessionCookieNames...)
}

// SessionHash returns the hex SHA-256 of the request's session token, or ""
// when it has none. The raw token is never stored.
func SessionHash(r *http.Request) string {
	for _, name := range sessionCookieNames {
		if ck, err := r.Cookie(name); err == nil && ck.Value != "" {
			sum := sha256.Sum256([]byte(ck.Value))
			return hex.EncodeToString(sum[:])
		}
	}
	return ""
}

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// --- marker file ---

// The marker is key=value lines so scripts/restore.sh can read it with sed.
func (s *State) writeMarkerLocked(state string, snap Snapshot) error {
	if s.markerPath == "" {
		return nil
	}
	lines := []string{
		"state=" + state,
		"manifest=" + oneLine(snap.Manifest),
		"actor=" + oneLine(snap.Actor),
		"owner=" + s.ownerHash,
		"leave_token=" + s.leaveToken,
		"token=" + s.runToken,
		"started=" + snap.StartedAt.UTC().Format(time.RFC3339),
		"safety=" + oneLine(snap.Safety),
		"db_touched=" + boolDigit(snap.DBTouched),
		fmt.Sprintf("exit=%d", snap.ExitCode),
		"summary=" + oneLine(snap.Summary),
	}
	if err := os.MkdirAll(filepath.Dir(s.markerPath), 0o750); err != nil {
		return err
	}
	tmp := s.markerPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.markerPath)
}

func (s *State) writeTailLocked(tail string) {
	if s.markerPath == "" {
		return
	}
	_ = os.WriteFile(s.markerPath+".log", []byte(tail), 0o600)
}

func (s *State) loadMarker() {
	if s.markerPath == "" {
		return
	}
	raw, err := os.ReadFile(s.markerPath)
	if err != nil {
		// A missing marker, or a parent that is not a directory, means no
		// restore was recorded.
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return
		}
		// Present but unreadable: stay closed with no owner.
		s.snap = Snapshot{Active: true, Phase: PhaseInterrupted,
			Summary: "Chronicle found a restore marker it could not read."}
		return
	}
	kv := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			kv[k] = v
		}
	}
	started, _ := time.Parse(time.RFC3339, kv["started"])
	exit := 0
	_, _ = fmt.Sscanf(kv["exit"], "%d", &exit)
	snap := Snapshot{
		Active:    true,
		Manifest:  kv["manifest"],
		Actor:     kv["actor"],
		StartedAt: started,
		Safety:    kv["safety"],
		DBTouched: kv["db_touched"] == "1",
		ExitCode:  exit,
		Summary:   kv["summary"],
	}
	if validHash(kv["owner"]) {
		s.ownerHash = kv["owner"]
	}
	s.leaveToken = kv["leave_token"]
	s.runToken = ""
	switch kv["state"] {
	case "failed":
		snap.Phase = PhaseFailed
		if tail, err := os.ReadFile(s.markerPath + ".log"); err == nil {
			snap.Tail = string(tail)
		}
	default:
		snap.Phase = PhaseInterrupted
		snap.Summary = "Chronicle was restarted while this restore was running, so it did not finish."
	}
	s.snap = snap
}

func oneLine(v string) string {
	v = strings.ReplaceAll(v, "\r", " ")
	return strings.ReplaceAll(v, "\n", " ")
}

func boolDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
