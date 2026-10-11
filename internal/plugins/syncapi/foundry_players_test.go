package syncapi

import (
	"strings"
	"testing"
	"time"
)

var playersNow = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func tptr(t time.Time) *time.Time { return &t }

func TestCleanPlayers_Rejects(t *testing.T) {
	tooMany := make([]reportedPlayer, foundryPlayersMax+1)
	for i := range tooMany {
		tooMany[i] = reportedPlayer{FoundryUserID: strings.Repeat("a", 1) + string(rune('A'+i%26)) + strings.Repeat("x", i)}
	}
	cases := []struct {
		name string
		in   []reportedPlayer
	}{
		{"over the cap", tooMany},
		{"empty id", []reportedPlayer{{FoundryUserID: ""}}},
		{"blank id", []reportedPlayer{{FoundryUserID: "   "}}},
		{"id with a space inside", []reportedPlayer{{FoundryUserID: "ab cd"}}},
		{"id with a control char", []reportedPlayer{{FoundryUserID: "ab\ncd"}}},
		{"id not ASCII", []reportedPlayer{{FoundryUserID: "café"}}},
		{"id over 64", []reportedPlayer{{FoundryUserID: strings.Repeat("a", foundryPlayerIDMax+1)}}},
		{"one bad id spoils the report", []reportedPlayer{{FoundryUserID: "ok"}, {FoundryUserID: ""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cleanPlayers(tc.in, nil, playersNow); err == nil {
				t.Fatal("expected a bad request")
			}
		})
	}
}

func TestCleanPlayers_AcceptsAtLimits(t *testing.T) {
	in := make([]reportedPlayer, foundryPlayersMax)
	for i := range in {
		in[i] = reportedPlayer{FoundryUserID: "u" + strings.Repeat("x", i%10) + string(rune('a'+i/26)) + string(rune('a'+i%26))}
	}
	out, err := cleanPlayers(in, nil, playersNow)
	if err != nil || len(out) != foundryPlayersMax {
		t.Fatalf("got %d rows, err %v", len(out), err)
	}
	long := strings.Repeat("a", foundryPlayerIDMax)
	out, err = cleanPlayers([]reportedPlayer{{FoundryUserID: long}}, nil, playersNow)
	if err != nil || len(out) != 1 || out[0].FoundryUserID != long {
		t.Fatalf("64-char id: %+v, %v", out, err)
	}
}

func TestCleanPlayers_Fields(t *testing.T) {
	members := map[string]bool{"m1": true}
	recent := playersNow.Add(-time.Hour)
	cases := []struct {
		name  string
		in    reportedPlayer
		check func(t *testing.T, p FoundryPlayer)
	}{
		{"id is trimmed", reportedPlayer{FoundryUserID: "  abc  "}, func(t *testing.T, p FoundryPlayer) {
			if p.FoundryUserID != "abc" {
				t.Fatalf("id %q", p.FoundryUserID)
			}
		}},
		{"name control chars stripped and trimmed", reportedPlayer{FoundryUserID: "a", Name: " Re\x1b[31mn\r\n\x7f\u0085 "}, func(t *testing.T, p FoundryPlayer) {
			if p.Name != "Re[31mn" {
				t.Fatalf("name %q", p.Name)
			}
		}},
		{"name truncated to 100", reportedPlayer{FoundryUserID: "a", Name: strings.Repeat("é", 150)}, func(t *testing.T, p FoundryPlayer) {
			if n := len([]rune(p.Name)); n != foundryPlayerNameMax {
				t.Fatalf("name runes %d", n)
			}
		}},
		{"failure truncated to 200", reportedPlayer{FoundryUserID: "a", LastFailure: strings.Repeat("x", 500)}, func(t *testing.T, p FoundryPlayer) {
			if len(p.LastFailure) != foundryPlayerFailureMax {
				t.Fatalf("failure len %d", len(p.LastFailure))
			}
		}},
		{"failure control chars stripped", reportedPlayer{FoundryUserID: "a", LastFailure: "no\nway"}, func(t *testing.T, p FoundryPlayer) {
			if p.LastFailure != "noway" {
				t.Fatalf("failure %q", p.LastFailure)
			}
		}},
		{"member in campaign is kept", reportedPlayer{FoundryUserID: "a", MemberID: "m1"}, func(t *testing.T, p FoundryPlayer) {
			if p.MemberUserID != "m1" {
				t.Fatalf("member %q", p.MemberUserID)
			}
		}},
		{"stranger is dropped but the row stays", reportedPlayer{FoundryUserID: "a", Name: "Eve", MemberID: "stranger"}, func(t *testing.T, p FoundryPlayer) {
			if p.MemberUserID != "" || p.Name != "Eve" {
				t.Fatalf("got %+v", p)
			}
		}},
		{"future time dropped", reportedPlayer{FoundryUserID: "a", LastChangeAt: tptr(playersNow.Add(time.Hour)), LastFailedAt: tptr(playersNow.Add(6 * time.Minute))}, func(t *testing.T, p FoundryPlayer) {
			if p.LastChangeAt != nil || p.LastFailedAt != nil {
				t.Fatalf("times kept: %+v", p)
			}
		}},
		{"slight clock skew kept", reportedPlayer{FoundryUserID: "a", LastChangeAt: tptr(playersNow.Add(4 * time.Minute))}, func(t *testing.T, p FoundryPlayer) {
			if p.LastChangeAt == nil {
				t.Fatal("skew within 5 minutes dropped")
			}
		}},
		{"older than retention dropped", reportedPlayer{FoundryUserID: "a", LastChangeAt: tptr(playersNow.Add(-foundryPlayersRetention - time.Second))}, func(t *testing.T, p FoundryPlayer) {
			if p.LastChangeAt != nil {
				t.Fatal("stale time kept")
			}
		}},
		{"zero time dropped", reportedPlayer{FoundryUserID: "a", LastChangeAt: tptr(time.Time{})}, func(t *testing.T, p FoundryPlayer) {
			if p.LastChangeAt != nil {
				t.Fatal("zero time kept")
			}
		}},
		{"recent time kept in UTC", reportedPlayer{FoundryUserID: "a", LastChangeAt: tptr(recent.In(time.FixedZone("x", 3600)))}, func(t *testing.T, p FoundryPlayer) {
			if p.LastChangeAt == nil || !p.LastChangeAt.Equal(recent) || p.LastChangeAt.Location() != time.UTC {
				t.Fatalf("got %v", p.LastChangeAt)
			}
		}},
		{"failed count capped", reportedPlayer{FoundryUserID: "a", FailedCount: 5000000}, func(t *testing.T, p FoundryPlayer) {
			if p.FailedCount != 100000 {
				t.Fatalf("count %d", p.FailedCount)
			}
		}},
		{"negative failed count becomes zero", reportedPlayer{FoundryUserID: "a", FailedCount: -3}, func(t *testing.T, p FoundryPlayer) {
			if p.FailedCount != 0 {
				t.Fatalf("count %d", p.FailedCount)
			}
		}},
		{"reported at is server time", reportedPlayer{FoundryUserID: "a", Online: true}, func(t *testing.T, p FoundryPlayer) {
			if !p.ReportedAt.Equal(playersNow) || !p.Online {
				t.Fatalf("got %+v", p)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cleanPlayers([]reportedPlayer{tc.in}, members, playersNow)
			if err != nil || len(out) != 1 {
				t.Fatalf("out %+v err %v", out, err)
			}
			tc.check(t, out[0])
		})
	}
}

func TestCleanPlayers_DedupeKeepsFirst(t *testing.T) {
	out, err := cleanPlayers([]reportedPlayer{
		{FoundryUserID: "a", Name: "First"},
		{FoundryUserID: " a ", Name: "Second"},
		{FoundryUserID: "b", Name: "Other"},
	}, nil, playersNow)
	if err != nil || len(out) != 2 || out[0].Name != "First" || out[1].FoundryUserID != "b" {
		t.Fatalf("got %+v, %v", out, err)
	}
}

func TestBuildPlayerRows(t *testing.T) {
	members := []MemberRef{
		{UserID: "gm", Name: "GM", Role: "Owner", Owner: true},
		{UserID: "ann", Name: "Ann", Role: "Player"},
		{UserID: "bob", Name: "Bob", Role: "Player"},
		{UserID: "cat", Name: "Cat", Role: "Scribe"},
	}
	cases := []struct {
		name       string
		players    []FoundryPlayer
		wantStatus map[string]PlayerStatus // keyed by member id, or "foundry:<name>" for unlinked
		check      func(t *testing.T, rows []PlayerRow)
	}{
		{
			name:    "unlinked foundry user explains and says how to fix",
			players: []FoundryPlayer{{FoundryUserID: "f1", Name: "Stranger"}},
			check: func(t *testing.T, rows []PlayerRow) {
				r := rows[0]
				if r.Status != PlayerUnlinked || r.FoundryName != "Stranger" || r.MemberUserID != "" || r.Why == "" || len(r.Fix) == 0 {
					t.Fatalf("got %+v", r)
				}
			},
		},
		{
			name:    "link to a non-member reads as unlinked",
			players: []FoundryPlayer{{FoundryUserID: "f1", Name: "X", MemberUserID: "nobody"}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerUnlinked {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name: "linked with a recent failure newer than the last change is refused",
			players: []FoundryPlayer{{FoundryUserID: "f1", Name: "Ann F", MemberUserID: "ann",
				LastChangeAt: tptr(playersNow.Add(-3 * time.Hour)), LastFailedAt: tptr(playersNow.Add(-time.Hour)), LastFailure: "403 forbidden"}},
			check: func(t *testing.T, rows []PlayerRow) {
				r := rows[0]
				if r.Status != PlayerRefused || r.MemberName != "Ann" || r.MemberRole != "Player" ||
					!strings.Contains(r.Why, "403 forbidden") || !strings.Contains(r.Why, "Ann") || len(r.Fix) != 2 {
					t.Fatalf("got %+v", r)
				}
			},
		},
		{
			name: "failure with no change ever is refused, and no failure text adds no 'told' line",
			players: []FoundryPlayer{{FoundryUserID: "f1", MemberUserID: "ann",
				LastFailedAt: tptr(playersNow.Add(-time.Hour))}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerRefused || strings.Contains(rows[0].Why, "Foundry was told") {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name: "success after the failure is working",
			players: []FoundryPlayer{{FoundryUserID: "f1", MemberUserID: "ann",
				LastFailedAt: tptr(playersNow.Add(-2 * time.Hour)), LastChangeAt: tptr(playersNow.Add(-time.Hour))}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerWorking || rows[0].Why != "" || len(rows[0].Fix) != 0 {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name: "failure older than the window with no change since is only linked",
			players: []FoundryPlayer{{FoundryUserID: "f1", MemberUserID: "ann",
				LastFailedAt: tptr(playersNow.Add(-recentWindow - time.Hour))}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerIdle {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name: "a tie between failure and change still reads as refused",
			players: []FoundryPlayer{{FoundryUserID: "f1", MemberUserID: "ann",
				LastFailedAt: tptr(playersNow.Add(-time.Hour)), LastChangeAt: tptr(playersNow.Add(-time.Hour))}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerRefused {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name:    "no foundry users: non-owner members listed, owner is not",
			players: nil,
			check: func(t *testing.T, rows []PlayerRow) {
				if len(rows) != 3 {
					t.Fatalf("got %d rows: %+v", len(rows), rows)
				}
				for i, id := range []string{"ann", "bob", "cat"} {
					r := rows[i]
					if r.MemberUserID != id || r.Status != PlayerNoFoundry || r.Why == "" || len(r.Fix) == 0 || r.FoundryName != "" {
						t.Fatalf("row %d: %+v", i, r)
					}
				}
			},
		},
		{
			name:    "linked with nothing synced is only linked, never working",
			players: []FoundryPlayer{{FoundryUserID: "f1", MemberUserID: "ann"}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerIdle || !strings.Contains(rows[0].Why, "Ann") {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name: "a change older than the window is only linked",
			players: []FoundryPlayer{{FoundryUserID: "f1", MemberUserID: "ann",
				LastChangeAt: tptr(playersNow.Add(-recentWindow - time.Hour))}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].Status != PlayerIdle {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name:    "owner linked to a foundry user is listed as that user",
			players: []FoundryPlayer{{FoundryUserID: "f0", Name: "Gamemaster", MemberUserID: "gm", Online: true, LastChangeAt: tptr(playersNow.Add(-time.Minute))}},
			check: func(t *testing.T, rows []PlayerRow) {
				if rows[0].MemberUserID != "gm" || rows[0].Status != PlayerWorking || !rows[0].Online {
					t.Fatalf("got %+v", rows[0])
				}
			},
		},
		{
			name: "order: linked, then unlinked, then members without foundry",
			players: []FoundryPlayer{
				{FoundryUserID: "f9", Name: "Stranger"},
				{FoundryUserID: "f1", Name: "Ann F", MemberUserID: "ann"},
			},
			check: func(t *testing.T, rows []PlayerRow) {
				got := []PlayerStatus{}
				for _, r := range rows {
					got = append(got, r.Status)
				}
				want := []PlayerStatus{PlayerIdle, PlayerUnlinked, PlayerNoFoundry, PlayerNoFoundry}
				if len(got) != len(want) {
					t.Fatalf("got %v", got)
				}
				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("got %v want %v", got, want)
					}
				}
				if rows[2].MemberUserID != "bob" || rows[3].MemberUserID != "cat" {
					t.Fatalf("members without foundry: %+v", rows[2:])
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, buildPlayerRows(tc.players, members, playersNow))
		})
	}
}

func TestLatestReport(t *testing.T) {
	if latestReport(nil) != nil {
		t.Fatal("empty list should have no report time")
	}
	got := latestReport([]FoundryPlayer{{ReportedAt: playersNow.Add(-time.Hour)}, {ReportedAt: playersNow}, {ReportedAt: playersNow.Add(-time.Minute)}})
	if got == nil || !got.Equal(playersNow) {
		t.Fatalf("got %v", got)
	}
}

func TestCarryForward(t *testing.T) {
	older, newer := tptr(playersNow.Add(-2*time.Hour)), tptr(playersNow.Add(-time.Hour))
	cases := []struct {
		name      string
		now, old  FoundryPlayer
		change    *time.Time
		failed    *time.Time
		failure   string
		failCount int
	}{
		{name: "a new session reporting nothing keeps the earlier times",
			now:    FoundryPlayer{},
			old:    FoundryPlayer{LastChangeAt: older, LastFailedAt: older, LastFailure: "HTTP 403 Forbidden", FailedCount: 2},
			change: older, failed: older, failure: "HTTP 403 Forbidden", failCount: 2},
		{name: "newer reported times win",
			now:    FoundryPlayer{LastChangeAt: newer, LastFailedAt: newer, LastFailure: "HTTP 500", FailedCount: 1},
			old:    FoundryPlayer{LastChangeAt: older, LastFailedAt: older, LastFailure: "HTTP 403 Forbidden", FailedCount: 2},
			change: newer, failed: newer, failure: "HTTP 500", failCount: 1},
		{name: "a newer stored failure keeps its text and count",
			now:    FoundryPlayer{LastChangeAt: newer, LastFailedAt: older, LastFailure: "HTTP 500", FailedCount: 1},
			old:    FoundryPlayer{LastFailedAt: newer, LastFailure: "HTTP 403 Forbidden", FailedCount: 4},
			change: newer, failed: newer, failure: "HTTP 403 Forbidden", failCount: 4},
		{name: "nothing known stays nothing",
			now: FoundryPlayer{}, old: FoundryPlayer{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := carryForward(tc.now, tc.old)
			if !sameTime(got.LastChangeAt, tc.change) || !sameTime(got.LastFailedAt, tc.failed) ||
				got.LastFailure != tc.failure || got.FailedCount != tc.failCount {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
