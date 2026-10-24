package admin

import (
	"fmt"
	"strings"
	"time"
)

// The site check-up: four questions a host should be able to answer "yes" to,
// shown behind a shield in the Site admin top bar. The weekly run answers the
// slow ones (is the backup restorable, is there a newer Chronicle); the rest
// are read live each time the shield loads, so fixing one turns it green at
// once. See the CheckupService for when it nags.

// CheckState is one check-up item's verdict.
type CheckState string

// Check-up verdicts. Bad is reserved for a backup that failed: it is the
// one thing that turns the shield red and puts the strip up at once.
const (
	CheckOK   CheckState = "ok"
	CheckWarn CheckState = "warn"
	CheckBad  CheckState = "bad"
)

// CheckItem is one row of the check-up sheet. A row offers at most one fix:
// a link (FixHref) or, for a one-click change, a form post (FixPost with
// FixVals as hx-vals JSON).
type CheckItem struct {
	Key      string
	Title    string
	Detail   string
	State    CheckState
	FixLabel string
	FixHref  string
	FixPost  string
	FixVals  string
	// FixExternal opens FixHref in a new tab (a page outside the site).
	FixExternal bool
}

// BackupCheckResult is the weekly read-back of a backup, as the backup
// plugin reports it through the CheckupBackup adapter.
type BackupCheckResult struct {
	OK       bool      `json:"ok"`
	MadeNow  bool      `json:"madeNow,omitempty"`
	BackupAt time.Time `json:"backupAt,omitempty"`
	Problem  string    `json:"problem,omitempty"`
}

// VersionCheckResult says how far the running Chronicle is behind the newest
// published one. Known is false when that could not be found out; Problem
// then says why, and the check-up treats it as a shrug, not a fault.
type VersionCheckResult struct {
	Known      bool   `json:"known"`
	Behind     int    `json:"behind,omitempty"`
	CompareURL string `json:"compareUrl,omitempty"`
	Problem    string `json:"problem,omitempty"`
}

// CheckupRecord is the stored outcome of the last full check-up run.
type CheckupRecord struct {
	At        time.Time           `json:"at"`
	Backup    *BackupCheckResult  `json:"backup,omitempty"`
	Chronicle *VersionCheckResult `json:"chronicle,omitempty"`
}

// checkupInputs is everything the check-up rows are built from: the last run
// plus the live settings. A field left at its zero "known" value is a figure
// that could not be read; its row says nothing rather than something false.
type checkupInputs struct {
	Now     time.Time
	Record  *CheckupRecord
	Running bool

	// BackupKnown is false when the backup state could not be read.
	BackupKnown, DailyOn, LastRunFailed bool

	// Registration is "open", "invite" or "closed"; "" when unreadable.
	Registration string

	// EmailReady is whether outgoing email is set up, so a player can reset
	// a forgotten password by themselves.
	EmailReady bool

	// PackageUpdates is how many game systems have an update ready; -1 when
	// unknown.
	PackageUpdates int
}

// Checkup is the built check-up, ready to render.
type Checkup struct {
	Items     []CheckItem
	CheckedAt time.Time // zero when no full run has happened yet
	NextAt    time.Time
	Running   bool
	Now       time.Time
}

// Fix destinations. Kept here so the rows and their tests agree.
const (
	checkupBackupHref   = "/admin/backup"
	checkupEmailHref    = "/admin/smtp"
	checkupPackagesHref = "/admin/packages?tab=updates"
	checkupSignupPost   = "/admin/security/registration"
	// checkupUpdateGuide is the deployment guide's upgrade section: updating
	// Chronicle itself is a server step, so the fix is the instructions.
	checkupUpdateGuide = "https://github.com/keyxmakerx/Chronicle/blob/main/docs/deployment.md#6-upgrade--redeploy"
)

// buildCheckup turns the inputs into the sheet's four rows.
func buildCheckup(in checkupInputs) Checkup {
	c := Checkup{Running: in.Running, Now: in.Now, NextAt: nextCheckupAt(in.Now)}
	if in.Record != nil {
		c.CheckedAt = in.Record.At
	}
	c.Items = append(c.Items, backupItem(in))
	if it, ok := signupItem(in.Registration); ok {
		c.Items = append(c.Items, it)
	}
	c.Items = append(c.Items, resetItem(in.EmailReady), updatesItem(in))
	return c
}

func backupItem(in checkupInputs) CheckItem {
	it := CheckItem{Key: "backup", Title: "Backup made and test-restored", State: CheckOK}
	var check *BackupCheckResult
	if in.Record != nil {
		check = in.Record.Backup
	}
	switch {
	case in.LastRunFailed:
		it.State, it.Detail = CheckBad, "The last backup failed. Your worlds are not backed up until one works."
	case check != nil && !check.OK:
		it.State, it.Detail = CheckBad, check.Problem
	case in.BackupKnown && !in.DailyOn:
		it.State, it.Detail = CheckWarn, "Daily backups are off."
		if check != nil && check.MadeNow {
			it.Detail += " The check-up made its own " + agoLabel(in.Now, check.BackupAt) + ", and it read back fine."
		} else if check != nil {
			it.Detail += " The newest backup, from " + agoLabel(in.Now, check.BackupAt) + ", read back fine."
		}
		it.FixLabel, it.FixHref = "Turn on", checkupBackupHref
		return it
	case check == nil:
		it.State, it.Detail = CheckWarn, "Not checked yet. Press Check now below, or wait for Sunday's check-up."
		return it
	default:
		made := "Made " + agoLabel(in.Now, check.BackupAt)
		if check.MadeNow {
			made = "The check-up made one " + agoLabel(in.Now, check.BackupAt)
		}
		it.Detail = made + ", then read back in full: nothing missing or damaged."
		return it
	}
	it.FixLabel, it.FixHref = "See why", checkupBackupHref
	return it
}

func signupItem(mode string) (CheckItem, bool) {
	it := CheckItem{Key: "signup", Title: "Who can sign up", State: CheckOK}
	switch mode {
	case "open":
		it.State, it.Detail = CheckWarn, "Anyone with the address can make an account."
		it.FixLabel, it.FixPost, it.FixVals = "Make invite-only", checkupSignupPost, `{"registration_mode":"invite"}`
	case "invite":
		it.Detail = "Invite-only."
	case "closed":
		it.Detail = "Closed: nobody new can sign up."
	default:
		return it, false
	}
	return it, true
}

func resetItem(emailReady bool) CheckItem {
	it := CheckItem{Key: "reset", Title: "Players can get back in", State: CheckOK,
		Detail: "Email is set up, so a forgotten password can be reset."}
	if !emailReady {
		it.State, it.Detail = CheckWarn, "Email isn't set up, so a forgotten password is a dead end."
		it.FixLabel, it.FixHref = "Set up email", checkupEmailHref
	}
	return it
}

func updatesItem(in checkupInputs) CheckItem {
	it := CheckItem{Key: "update", Title: "Chronicle and game systems up to date", State: CheckOK}
	var ver *VersionCheckResult
	if in.Record != nil {
		ver = in.Record.Chronicle
	}
	var news []string
	if ver != nil && ver.Known && ver.Behind > 0 {
		news = append(news, "A newer Chronicle is out ("+countUnit(ver.Behind, "change")+").")
	}
	if in.PackageUpdates > 0 {
		news = append(news, countUnit(in.PackageUpdates, "game-system update")+" ready.")
	}
	switch {
	case in.PackageUpdates > 0:
		it.State, it.FixLabel, it.FixHref = CheckWarn, "See updates", checkupPackagesHref
	case len(news) > 0:
		it.State, it.FixLabel, it.FixHref, it.FixExternal = CheckWarn, "How to update", checkupUpdateGuide, true
	}
	if len(news) > 0 {
		it.Detail = strings.Join(news, " ")
		return it
	}
	it.Detail = "Nothing new to install."
	if ver != nil && !ver.Known && ver.Problem != "" {
		it.Detail += " Chronicle's own version couldn't be checked: " + ver.Problem
	}
	return it
}

// Worst is the shield's colour: bad beats warn beats ok.
func (c Checkup) Worst() CheckState {
	worst := CheckOK
	for _, it := range c.Items {
		if it.State == CheckBad {
			return CheckBad
		}
		if it.State == CheckWarn {
			worst = CheckWarn
		}
	}
	return worst
}

// Open returns the rows that are not ok.
func (c Checkup) Open() []CheckItem {
	var out []CheckItem
	for _, it := range c.Items {
		if it.State != CheckOK {
			out = append(out, it)
		}
	}
	return out
}

// Title is the sheet's heading.
func (c Checkup) Title() string {
	switch c.Worst() {
	case CheckOK:
		return "All good"
	case CheckBad:
		return "A backup failed"
	}
	n := len(c.Open())
	if n == 1 {
		return "1 thing needs you"
	}
	return fmt.Sprintf("%d things need you", n)
}

// Subtitle says when the check-up last ran and, when all is well, when it
// runs next.
func (c Checkup) Subtitle() string {
	if c.Running {
		return "Checking now. This makes a backup if there isn't one from today, so it can take a few minutes."
	}
	checked := "Not checked yet."
	if !c.CheckedAt.IsZero() {
		checked = "Checked " + agoLabel(c.Now, c.CheckedAt) + "."
	}
	if c.Worst() == CheckOK {
		return checked + " Next check-up " + nextCheckupLabel(c.Now, c.NextAt) + "."
	}
	return checked + " Nothing here stops you using the site."
}

// StripText is the reminder strip's sentence for open rows.
func (c Checkup) StripText() string {
	if c.Worst() == CheckBad {
		return "A backup failed. Your worlds are not backed up until one works."
	}
	open := c.Open()
	names := make([]string, len(open))
	for i, it := range open {
		names[i] = lowerFirst(it.Title)
	}
	lead := countUnit(len(open), "check-up item") + " have been waiting a week: "
	if len(open) == 1 {
		lead = "1 check-up item has been waiting a week: "
	}
	return lead + strings.Join(names, ", ") + "."
}

// lowerFirst puts a row title mid-sentence; Chronicle keeps its capital.
func lowerFirst(s string) string {
	if s == "" || strings.HasPrefix(s, "Chronicle") {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// The weekly check-up runs Sunday at 04:00 server time: after the default
// 03:00 daily backup, so it reads that backup back instead of making its own.
const (
	checkupWeekday = time.Sunday
	checkupHour    = 4
)

// lastCheckupBoundary is the most recent Sunday 04:00 at or before now.
func lastCheckupBoundary(now time.Time) time.Time {
	days := (int(now.Weekday()) - int(checkupWeekday) + 7) % 7
	b := time.Date(now.Year(), now.Month(), now.Day()-days, checkupHour, 0, 0, 0, now.Location())
	if b.After(now) {
		b = b.AddDate(0, 0, -7)
	}
	return b
}

// nextCheckupAt is the next Sunday 04:00 after now.
func nextCheckupAt(now time.Time) time.Time {
	return lastCheckupBoundary(now).AddDate(0, 0, 7)
}

// checkupDue reports whether the weekly run should start. A missed run (the
// server was down at 04:00) is made up as soon as it is back. A site that has
// never run one waits for the hour itself, so a fresh install or an update
// doesn't start a backup the moment it boots.
func checkupDue(now time.Time, last *CheckupRecord) bool {
	b := lastCheckupBoundary(now)
	if last == nil {
		return now.Sub(b) < time.Hour
	}
	return last.At.Before(b)
}

// nextCheckupLabel phrases the next run: "Sunday at 04:00", or "today at
// 04:00" when that Sunday is today.
func nextCheckupLabel(now, next time.Time) string {
	day := next.Weekday().String()
	if y, m, d := now.Date(); next.Year() == y && next.Month() == m && next.Day() == d {
		day = "today"
	}
	return day + " at " + next.Format("15:04")
}
