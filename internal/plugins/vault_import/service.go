package vault_import

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// uploadTTL is how long a previewed zip waits for the owner to press Import.
const uploadTTL = 30 * time.Minute

// maxConcurrentImports bounds how many imports run at once across the server;
// each holds a zip open and writes many rows.
const maxConcurrentImports = 2

// jobKeep is how long a finished import's result stays readable.
const jobKeep = 24 * time.Hour

// Deps is everything the service needs from the rest of Chronicle.
type Deps struct {
	Pages    PageStore
	Pictures PictureStore
	Files    FileAttacher
	ToJSON   EditorJSON
	Audit    AuditLogger
	// Lifecycle ends running imports when the server shuts down.
	Lifecycle context.Context
	Limits    Limits
	TempDir   string
	Now       func() time.Time
}

// Service previews and runs imports. It holds uploads and job progress in
// memory: a restart forgets them, and a half-finished import is then marked by
// the folder it left behind (see runner.go), never by this state.
type Service struct {
	d Deps

	mu      sync.Mutex
	uploads map[string]*upload
	jobs    map[string]*Job
	active  map[string]string // campaign -> running job id
	running int
}

type upload struct {
	id, campaignID, userID string
	path, fileName         string
	vault                  *Vault
	timer                  *time.Timer
}

// NewService builds the service and clears any zips a previous run left in its
// temp folder.
func NewService(d Deps) *Service {
	if d.Limits.MaxEntries == 0 {
		d.Limits = DefaultLimits
	}
	if d.TempDir == "" {
		d.TempDir = filepath.Join(os.TempDir(), "chronicle-vault-import")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Lifecycle == nil {
		d.Lifecycle = context.Background()
	}
	_ = os.RemoveAll(d.TempDir)
	return &Service{d: d, uploads: map[string]*upload{}, jobs: map[string]*Job{}, active: map[string]string{}}
}

// Limits returns the limits in force, for the page to state them.
func (s *Service) Limits() Limits { return s.d.Limits }

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SaveUpload copies an uploaded zip to a private temp file, refusing more than
// the upload limit, and returns its path. The caller hands the path to Preview.
func (s *Service) SaveUpload(r io.Reader) (string, error) {
	if err := os.MkdirAll(s.d.TempDir, 0o700); err != nil {
		return "", apperror.NewInternal(fmt.Errorf("creating import folder: %w", err))
	}
	f, err := os.CreateTemp(s.d.TempDir, "upload-*.zip")
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("creating import file: %w", err))
	}
	n, err := io.Copy(f, io.LimitReader(r, s.d.Limits.MaxUploadBytes+1))
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", apperror.NewBadRequest("The upload was interrupted. Try again.")
	}
	if n > s.d.Limits.MaxUploadBytes {
		_ = os.Remove(f.Name())
		return "", apperror.NewBadRequest(fmt.Sprintf("That zip is over %d MB. Import a smaller part of the vault.", s.d.Limits.MaxUploadBytes>>20))
	}
	return f.Name(), nil
}

// Preview checks and reads the zip at zipPath and reports what an import would
// do. Nothing is written to the campaign. On any error the file is removed.
func (s *Service) Preview(campaignID, userID, zipPath, fileName string) (*Preview, error) {
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(zipPath)
		}
	}()
	arc, err := OpenArchive(zipPath, fileName, s.d.Limits)
	if err != nil {
		return nil, s.asBadRequest(err)
	}
	defer func() { _ = arc.Close() }()
	var attachable AttachCheck
	if s.d.Files != nil {
		attachable = s.d.Files.CanAttach
	}
	v, err := Analyze(arc, attachable)
	if err != nil {
		slog.Warn("vault import: analysing the zip failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return nil, apperror.NewBadRequest("Some notes in that zip could not be read. Check the zip and try again.")
	}
	if len(v.Notes) == 0 {
		return nil, apperror.NewBadRequest("There are no Markdown notes (.md files) in that zip.")
	}

	up := &upload{id: newID(), campaignID: campaignID, userID: userID, path: zipPath, fileName: fileName, vault: v}
	s.mu.Lock()
	for id, old := range s.uploads {
		if old.campaignID == campaignID && old.userID == userID {
			s.dropUploadLocked(id)
		}
	}
	s.uploads[up.id] = up
	up.timer = time.AfterFunc(uploadTTL, func() {
		s.mu.Lock()
		s.dropUploadLocked(up.id)
		s.mu.Unlock()
	})
	s.mu.Unlock()
	keep = true

	return buildPreview(up), nil
}

func (s *Service) asBadRequest(err error) error {
	if IsArchiveError(err) {
		return apperror.NewBadRequest(err.Error())
	}
	slog.Warn("vault import: opening the zip failed", slog.Any("error", err))
	return apperror.NewBadRequest("That zip could not be read. Zip the vault folder again and retry.")
}

func (s *Service) dropUploadLocked(id string) {
	up, ok := s.uploads[id]
	if !ok {
		return
	}
	if up.timer != nil {
		up.timer.Stop()
	}
	_ = os.Remove(up.path)
	delete(s.uploads, id)
}

// Preview is what the owner sees before anything is written.
type Preview struct {
	UploadID    string
	FileName    string
	VaultName   string
	Stats       Stats
	Total       int
	Outline     []OutlineLine
	OutlineMore int
	Skipped     []SkippedEntry
	SkippedMore int
}

const (
	outlineLimit = 30
	skippedLimit = 12
)

func buildPreview(up *upload) *Preview {
	v := up.vault
	p := &Preview{UploadID: up.id, FileName: up.fileName, VaultName: v.Name, Stats: v.Stats, Total: len(v.Pages)}
	p.Outline, p.OutlineMore = v.Outline(outlineLimit)
	p.Skipped = v.Stats.SkippedFiles
	if len(p.Skipped) > skippedLimit {
		p.SkippedMore = len(p.Skipped) - skippedLimit
		p.Skipped = p.Skipped[:skippedLimit]
	}
	return p
}

// ErrBusy is returned when an import for the campaign is already running, or
// the server is at its limit.
var ErrBusy = errors.New("an import is already running")

// Start begins importing a previewed upload in the background and returns the
// job to poll. The upload must belong to this campaign and this person.
func (s *Service) Start(campaignID, userID, uploadID string) (*JobView, error) {
	s.mu.Lock()
	up, ok := s.uploads[uploadID]
	if !ok || up.campaignID != campaignID || up.userID != userID {
		s.mu.Unlock()
		return nil, apperror.NewBadRequest("That upload has expired. Choose the zip again.")
	}
	if id, busy := s.active[campaignID]; busy {
		s.mu.Unlock()
		slog.Info("vault import: refused, one already running", slog.String("campaign_id", campaignID), slog.String("job", id))
		return nil, apperror.NewConflict("An import is already running for this campaign. Wait for it to finish.")
	}
	if s.running >= maxConcurrentImports {
		s.mu.Unlock()
		return nil, apperror.NewConflict("The server is busy with other imports. Try again in a few minutes.")
	}
	job := newJob(campaignID, userID, up.fileName, len(up.vault.Pages), s.d.Now())
	s.jobs[job.ID] = job
	s.active[campaignID] = job.ID
	s.running++
	// The job owns the zip from here; the expiry timer no longer applies.
	if up.timer != nil {
		up.timer.Stop()
	}
	delete(s.uploads, uploadID)
	s.mu.Unlock()

	go s.run(job, up)
	v := job.View()
	return &v, nil
}

// Job returns the progress of a job that belongs to this campaign.
func (s *Service) Job(campaignID, jobID string) (JobView, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[jobID]
	if !ok || j.CampaignID != campaignID {
		return JobView{}, false
	}
	return j.View(), true
}

// ActiveJob returns the running import for a campaign, so a reloaded page can
// pick the progress bar back up.
func (s *Service) ActiveJob(campaignID string) (JobView, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.active[campaignID]
	if !ok {
		return JobView{}, false
	}
	return s.jobs[id].View(), true
}

// finish releases the slot a job held and schedules its result for removal.
func (s *Service) finish(j *Job) {
	s.mu.Lock()
	if s.active[j.CampaignID] == j.ID {
		delete(s.active, j.CampaignID)
	}
	s.running--
	s.mu.Unlock()
	time.AfterFunc(jobKeep, func() {
		s.mu.Lock()
		delete(s.jobs, j.ID)
		s.mu.Unlock()
	})
}

// renderMarkdown is the single path every imported text takes to storage:
// Markdown to HTML, the shared sanitiser, the editor's own link and picture
// markup, the sanitiser again, then the editor's document format.
func (s *Service) renderMarkdown(src, campaignID string) (htmlOut, editorJSON string, err error) {
	h, err := markdownToHTML(src)
	if err != nil {
		return "", "", err
	}
	htmlOut = sanitize.HTML(finishHTML(h, campaignID))
	editorJSON, err = s.d.ToJSON(htmlOut)
	if err != nil {
		return "", "", fmt.Errorf("could not convert the note to the editor format: %w", err)
	}
	return htmlOut, editorJSON, nil
}
