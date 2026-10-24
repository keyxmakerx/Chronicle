package app

// checkup_adapters.go wires the site check-up (admin) to the backup plugin
// and to GitHub, so the admin package imports neither.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
	"github.com/keyxmakerx/chronicle/internal/plugins/admin"
	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
)

// checkupBackup runs the backup plugin's read-back check.
type checkupBackup struct {
	svc   backup.Service
	store backup.SettingsStore
}

func (a checkupBackup) CheckBackup(ctx context.Context) (admin.BackupCheckResult, error) {
	res, err := backup.CheckBackup(ctx, a.svc, a.store, time.Now())
	if errors.Is(err, backup.ErrAlreadyRunning) {
		return admin.BackupCheckResult{}, admin.ErrCheckupBusy
	}
	if err != nil {
		return admin.BackupCheckResult{}, err
	}
	return admin.BackupCheckResult{OK: res.OK, MadeNow: res.MadeNow, BackupAt: res.BackupAt, Problem: res.Problem}, nil
}

// chronicleRepo is where Chronicle is published. Images are built from its
// main branch, so "behind main" is "a newer image is out".
const chronicleRepo = "keyxmakerx/Chronicle"

// revisionPattern is a git commit id, the only thing put into the URL.
var revisionPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// chronicleVersionChecker asks GitHub how many commits main has that the
// running build doesn't. GITHUB_TOKEN raises the rate limit when set.
type chronicleVersionChecker struct {
	client   *http.Client
	token    string
	revision string
	apiBase  string
}

func newChronicleVersionChecker() *chronicleVersionChecker {
	b := hostinfo.ReadBuild()
	return &chronicleVersionChecker{
		client:   &http.Client{Timeout: 15 * time.Second},
		token:    os.Getenv("GITHUB_TOKEN"),
		revision: b.Revision,
		apiBase:  "https://api.github.com",
	}
}

func (v *chronicleVersionChecker) CheckChronicle(ctx context.Context) admin.VersionCheckResult {
	if !revisionPattern.MatchString(v.revision) {
		return admin.VersionCheckResult{Problem: "this build doesn't record which version it is."}
	}
	url := fmt.Sprintf("%s/repos/%s/compare/%s...main", v.apiBase, chronicleRepo, v.revision)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return admin.VersionCheckResult{Problem: "the check couldn't be made."}
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if v.token != "" {
		req.Header.Set("Authorization", "Bearer "+v.token)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return admin.VersionCheckResult{Problem: "GitHub couldn't be reached."}
	}
	defer resp.Body.Close()
	return parseCompare(resp.StatusCode, io.LimitReader(resp.Body, 1<<20), v.revision)
}

// parseCompare reads GitHub's compare answer. Only ahead_by is used: the
// commits main has that this build lacks. A build GitHub doesn't know (a
// local commit) can't be compared; a build with uncommitted changes is
// compared by the commit it started from.
func parseCompare(status int, body io.Reader, revision string) admin.VersionCheckResult {
	switch {
	case status == http.StatusNotFound:
		return admin.VersionCheckResult{Problem: "this build isn't one Chronicle published."}
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return admin.VersionCheckResult{Problem: "GitHub asked us to slow down; it's tried again next week."}
	case status != http.StatusOK:
		return admin.VersionCheckResult{Problem: fmt.Sprintf("GitHub answered %d.", status)}
	}
	var cmp struct {
		AheadBy int `json:"ahead_by"`
	}
	if err := json.NewDecoder(body).Decode(&cmp); err != nil {
		return admin.VersionCheckResult{Problem: "GitHub's answer couldn't be read."}
	}
	res := admin.VersionCheckResult{Known: true, Behind: cmp.AheadBy}
	if cmp.AheadBy > 0 {
		res.CompareURL = fmt.Sprintf("https://github.com/%s/compare/%s...main", chronicleRepo, revision)
	}
	return res
}
