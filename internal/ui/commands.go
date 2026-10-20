package ui

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const (
	logPageSize  = 500
	pollInterval = 2 * time.Second
	// wholeHistory is a log limit no repository reaches. git parses --max-count into a 32-bit int.
	wholeHistory = math.MaxInt32
)

// RepositoryFocused switches the review to Repository, unless it is already on screen.
type RepositoryFocused struct {
	Repository gitrepo.Repository
}

// FocusRejected clears the review and shows Reason, so nothing of the previous repository stays on screen.
type FocusRejected struct {
	Reason string
}

// pollTickMsg reloads the status, which reloads the selected diff and the log, so edits made outside perusal show up.
type pollTickMsg struct{}

func nextPoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// The messages below carry dir, the repository they came from, so a result that lands after a switch is dropped.

type statusLoadedMsg struct {
	dir    string
	status gitrepo.Status
}

type diffLoadedMsg struct {
	dir      string
	path     string
	fileDiff diff.FileDiff
}

// logLoadedMsg carries the limit it was read with, so a reload still in flight when the limit grows is dropped.
type logLoadedMsg struct {
	dir      string
	limit    int
	commits  []gitrepo.Commit
	loadedAt time.Time
}

type commitDiffLoadedMsg struct {
	dir      string
	hash     string
	sections []diff.FileSection
}

type indexChangedMsg struct {
	dir string
}

type committedMsg struct {
	dir     string
	summary string
}

type commitFailedMsg struct {
	dir string
	err error
}

type pushedMsg struct {
	dir      string
	upstream string
}

type pushFailedMsg struct {
	dir string
	err error
}

type gitFailedMsg struct {
	err error
}

type repositoryRequest struct {
	repository gitrepo.Repository
}

func (r repositoryRequest) loadStatus() tea.Msg {
	status, err := r.repository.Status(context.Background())
	if err != nil {
		return gitFailedMsg{err: err}
	}
	return statusLoadedMsg{dir: r.repository.Dir, status: status}
}

func (r repositoryRequest) stageAll() tea.Msg {
	if err := r.repository.StageAll(context.Background()); err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{dir: r.repository.Dir}
}

func (r repositoryRequest) unstageAll() tea.Msg {
	if err := r.repository.UnstageAll(context.Background()); err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{dir: r.repository.Dir}
}

type logRequest struct {
	repository gitrepo.Repository
	limit      int
}

func (r logRequest) load() tea.Msg {
	commits, err := r.repository.Log(context.Background(), r.limit)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	return logLoadedMsg{dir: r.repository.Dir, limit: r.limit, commits: commits, loadedAt: time.Now()}
}

type pushRequest struct {
	repository gitrepo.Repository
	upstream   string
}

func (r pushRequest) push() tea.Msg {
	if err := r.repository.Push(context.Background()); err != nil {
		return pushFailedMsg{dir: r.repository.Dir, err: err}
	}
	return pushedMsg{dir: r.repository.Dir, upstream: r.upstream}
}

type fileRequest struct {
	repository gitrepo.Repository
	file       gitrepo.FileStatus
}

func (r fileRequest) loadDiff() tea.Msg {
	text, err := r.repository.FileDiff(context.Background(), r.file)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	fileDiff, err := diff.ParseUnified(text)
	if err != nil {
		return gitFailedMsg{err: fmt.Errorf("parse diff of %s: %w", r.file.Path, err)}
	}
	return diffLoadedMsg{dir: r.repository.Dir, path: r.file.Path, fileDiff: fileDiff}
}

func (r fileRequest) stage() tea.Msg {
	if err := r.repository.Stage(context.Background(), r.file); err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{dir: r.repository.Dir}
}

func (r fileRequest) unstage() tea.Msg {
	if err := r.repository.Unstage(context.Background(), r.file); err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{dir: r.repository.Dir}
}

type commitDiffRequest struct {
	repository gitrepo.Repository
	hash       string
}

func (r commitDiffRequest) load() tea.Msg {
	text, err := r.repository.CommitDiff(context.Background(), r.hash)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	sections, err := diff.ParseFiles(text)
	if err != nil {
		return gitFailedMsg{err: fmt.Errorf("parse diff of commit %s: %w", r.hash, err)}
	}
	return commitDiffLoadedMsg{dir: r.repository.Dir, hash: r.hash, sections: sections}
}

type commitRequest struct {
	repository gitrepo.Repository
	message    string
}

func (r commitRequest) commit() tea.Msg {
	summary, err := r.repository.Commit(context.Background(), r.message)
	if err != nil {
		return commitFailedMsg{dir: r.repository.Dir, err: err}
	}
	return committedMsg{dir: r.repository.Dir, summary: summary}
}
