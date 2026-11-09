package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ownerA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newTestState(t *testing.T) (*State, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), MarkerName)
	return NewState(p), p
}

// TestStateTransitions walks idle to each end state and checks what the
// gate and the marker say after every move.
func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name       string
		run        func(t *testing.T, s *State)
		wantActive bool
		wantPhase  Phase
		wantMarker bool
	}{
		{"fresh state is idle and open", func(*testing.T, *State) {}, false, PhaseIdle, false},
		{"enter closes the gate and writes the marker", func(t *testing.T, s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
		}, true, PhaseRunning, true},
		{"success opens the gate and removes the marker", func(t *testing.T, s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.Succeed("ok")
		}, false, PhaseDone, false},
		{"failure keeps the gate closed and the marker", func(t *testing.T, s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.Fail("bad", "tail", 3)
		}, true, PhaseFailed, true},
		{"leaving after failure opens the gate", func(t *testing.T, s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.Fail("bad", "tail", 3)
			_ = s.Leave()
		}, false, PhaseFailed, false},
		{"leaving while running is refused", func(t *testing.T, s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			if err := s.Leave(); !errors.Is(err, ErrStillBusy) {
				t.Errorf("Leave while running = %v, want ErrStillBusy", err)
			}
		}, true, PhaseRunning, true},
		{"a second enter is refused", func(t *testing.T, s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			if err := s.Enter(ownerA, "u2", "other.txt"); !errors.Is(err, ErrBusy) {
				t.Errorf("second Enter = %v, want ErrBusy", err)
			}
		}, true, PhaseRunning, true},
		{"a session hash that is not a hash is refused", func(t *testing.T, s *State) {
			if err := s.Enter("short", "u1", "m.txt"); err == nil {
				t.Error("Enter accepted a bad owner hash")
			}
		}, false, PhaseIdle, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, marker := newTestState(t)
			tc.run(t, s)
			snap := s.Snapshot()
			if snap.Active != tc.wantActive || snap.Phase != tc.wantPhase {
				t.Errorf("active=%v phase=%s, want active=%v phase=%s", snap.Active, snap.Phase, tc.wantActive, tc.wantPhase)
			}
			_, err := os.Stat(marker)
			if (err == nil) != tc.wantMarker {
				t.Errorf("marker present=%v, want %v", err == nil, tc.wantMarker)
			}
		})
	}
}

// TestEnterFailsClosedWhenMarkerCannotBeWritten pins that a restore that
// cannot be recorded never starts.
func TestEnterFailsClosedWhenMarkerCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewState(filepath.Join(blocker, MarkerName)) // parent is a file
	if err := s.Enter(ownerA, "u1", "m.txt"); err == nil {
		t.Fatal("Enter succeeded without a writable marker")
	}
	if s.Active() {
		t.Error("gate closed although the restore never started")
	}
}

// TestRestartWithMarker covers a process restart: what comes back depends on
// what the marker says was happening.
func TestRestartWithMarker(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(s *State)
		wantActive bool
		wantPhase  Phase
		check      func(t *testing.T, s Snapshot)
	}{
		{"no marker boots open", func(*State) {}, false, PhaseIdle, nil},
		{"a restore that was running comes back closed as interrupted", func(s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.NoteSafety("chronicle_pre_restore_manifest_1.txt")
			s.NoteDBTouched()
		}, true, PhaseInterrupted, func(t *testing.T, sn Snapshot) {
			if sn.Safety != "chronicle_pre_restore_manifest_1.txt" || !sn.DBTouched || sn.Manifest != "m.txt" {
				t.Errorf("snapshot lost details: %+v", sn)
			}
		}},
		{"a failed restore comes back closed with its output", func(s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.Fail("it broke", "last lines", 3)
		}, true, PhaseFailed, func(t *testing.T, sn Snapshot) {
			if sn.Tail != "last lines" || sn.Summary != "it broke" || sn.ExitCode != 3 {
				t.Errorf("snapshot lost details: %+v", sn)
			}
		}},
		{"a finished restore leaves nothing behind", func(s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.Succeed("ok")
		}, false, PhaseIdle, nil},
		{"leaving clears the marker so the next boot is open", func(s *State) {
			_ = s.Enter(ownerA, "u1", "m.txt")
			s.Fail("x", "y", 1)
			_ = s.Leave()
		}, false, PhaseIdle, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			first, marker := newTestState(t)
			tc.prepare(first)
			second := NewState(marker)
			snap := second.Snapshot()
			if snap.Active != tc.wantActive || snap.Phase != tc.wantPhase {
				t.Errorf("after restart active=%v phase=%s, want active=%v phase=%s", snap.Active, snap.Phase, tc.wantActive, tc.wantPhase)
			}
			if tc.check != nil {
				tc.check(t, snap)
			}
		})
	}
}

// TestRestartKeepsOwnerAndLeaveToken pins that the same admin can still see
// the page and leave maintenance after a restart, and nobody else can.
func TestRestartKeepsOwnerAndLeaveToken(t *testing.T) {
	first, marker := newTestState(t)
	_ = first.Enter(ownerA, "u1", "m.txt")
	leave := first.LeaveToken()
	first.Fail("x", "y", 1)

	second := NewState(marker)
	if !second.CheckLeaveToken(leave) {
		t.Error("leave token lost across restart")
	}
	if second.CheckLeaveToken("") || second.CheckLeaveToken(leave+"x") {
		t.Error("wrong leave token accepted")
	}
	if second.RunToken() != "" {
		t.Error("the script's run token must not survive a restart")
	}
}

// TestMarkerIsShellReadable pins the format scripts/restore.sh parses.
func TestMarkerIsShellReadable(t *testing.T) {
	s, marker := newTestState(t)
	if err := s.Enter(ownerA, "u1", "m.txt"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(marker)
	for _, want := range []string{"state=running\n", "manifest=m.txt\n", "token=" + s.RunToken() + "\n"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("marker missing %q:\n%s", want, raw)
		}
	}
	if info, _ := os.Stat(marker); info.Mode().Perm() != 0o600 {
		t.Errorf("marker mode = %v, want 0600", info.Mode().Perm())
	}
}
