package vault_import

import (
	"fmt"
	"sync"
	"time"
)

// Job states.
const (
	StateRunning = "running"
	StateDone    = "done"
	StateFailed  = "failed"
)

// maxProblems bounds the per-page problem list kept and shown; the count stays
// exact past it.
const maxProblems = 100

// Problem is one thing that did not import, in words for the owner.
type Problem struct {
	Where string
	What  string
}

// Summary is the result of an import.
type Summary struct {
	Pages          int // pages created, folders included
	PagesFailed    int // pages that could not be created
	TextsFailed    int // pages created but whose text could not be saved
	LinksMade      int
	LinksPlain     int
	Pictures       int
	PictureFails   int
	FilesAttached  int
	FilesSkipped   int
	Problems       []Problem
	MoreProblems   int
	VaultName      string
	SourceFileName string
}

func (sm *Summary) problem(where, what string) {
	if len(sm.Problems) >= maxProblems {
		sm.MoreProblems++
		return
	}
	sm.Problems = append(sm.Problems, Problem{Where: where, What: what})
}

// Job is one running or finished import.
type Job struct {
	ID         string
	CampaignID string
	UserID     string

	mu       sync.Mutex
	state    string
	phase    string
	done     int
	total    int
	rootID   string
	rootName string
	partial  bool
	errMsg   string
	summary  *Summary
	started  time.Time
	finished time.Time
	file     string
}

func newJob(campaignID, userID, file string, total int, now time.Time) *Job {
	return &Job{ID: newID(), CampaignID: campaignID, UserID: userID, state: StateRunning,
		phase: "Starting", total: total, started: now, file: file}
}

func (j *Job) set(f func(j *Job)) {
	j.mu.Lock()
	f(j)
	j.mu.Unlock()
}

// JobView is a consistent copy of a job's progress, safe to render and poll.
type JobView struct {
	ID         string
	CampaignID string
	State      string
	Phase      string
	Done       int
	Total      int
	Percent    int
	// Label is the sentence over the progress bar.
	Label    string
	RootID   string
	RootName string
	// Partial is true on a failure that left an Imported folder behind.
	Partial bool
	Error   string
	Summary *Summary
	File    string
}

// View snapshots the job.
func (j *Job) View() JobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := JobView{ID: j.ID, CampaignID: j.CampaignID, State: j.state, Phase: j.phase, Done: j.done, Total: j.total,
		RootID: j.rootID, RootName: j.rootName, Partial: j.partial, Error: j.errMsg, File: j.file}
	if j.summary != nil {
		sm := *j.summary
		sm.Problems = append([]Problem(nil), j.summary.Problems...)
		v.Summary = &sm
	}
	if j.total > 0 {
		v.Percent = j.done * 100 / j.total
	}
	switch {
	case j.state == StateDone:
		v.Percent = 100
		v.Label = "Done"
	case j.state == StateFailed:
		v.Label = "Stopped"
	case j.done > 0:
		v.Label = fmt.Sprintf("Importing %d of %d…", j.done, j.total)
	default:
		v.Label = j.phase + "…"
	}
	return v
}
