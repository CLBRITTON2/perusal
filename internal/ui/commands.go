package ui

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/highlight"
)

const (
	logPageSize  = 500
	pollInterval = 2 * time.Second
	// git parses --max-count into a 32-bit int.
	wholeHistory = math.MaxInt32
	// git's own default.
	hunkContext = 3
	// git parses --unified into a 32-bit int.
	wholeFileContext = math.MaxInt32
)

// Refusals are the focused paths that failed a check, shown so a refused repository is never silently skipped.
type RepositoriesFocused struct {
	Repositories []gitrepo.Repository
	Refusals     []error
}

type FocusRejected struct {
	Reason string
}

type pollTickMsg struct{}

func nextPoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// The messages below carry dir, the repository they came from, so a result that lands after a switch is dropped.

type statusLoadedMsg struct {
	dir              string
	status           gitrepo.Status
	stagedLineCounts diff.LineCounts
}

type diffLoadedMsg struct {
	dir          string
	path         string
	contextLines int
	fileDiff     diff.FileDiff
}

// logLoadedMsg carries the limit it was read with, so a reload still in flight when the limit grows is dropped.
type logLoadedMsg struct {
	dir      string
	limit    int
	commits  []gitrepo.Commit
	loadedAt time.Time
}

type commitDiffLoadedMsg struct {
	dir          string
	hash         string
	contextLines int
	sections     []diff.FileSection
}

// uncommittedLoadedMsg carries the focused repositories it checked instead of a dir.
type uncommittedLoadedMsg struct {
	focused     []gitrepo.Repository
	uncommitted map[string]bool
}

type indexChangedMsg struct {
	dir string
}

type committedMsg struct {
	dir     string
	summary string
}

type popupSucceededMsg struct {
	dir    string
	notice string
}

type popupFailedMsg struct {
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
	stagedLineCounts, err := r.repository.StagedLineCounts(context.Background())
	if err != nil {
		return gitFailedMsg{err: err}
	}
	return statusLoadedMsg{dir: r.repository.Dir, status: status, stagedLineCounts: stagedLineCounts}
}

func (r repositoryRequest) stageAll() tea.Msg {
	return indexChanged(r.repository.Dir, r.repository.StageAll(context.Background()))
}

func (r repositoryRequest) unstageAll() tea.Msg {
	return indexChanged(r.repository.Dir, r.repository.UnstageAll(context.Background()))
}

func (r repositoryRequest) discardAll() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.DiscardAll(context.Background()), "discarded all changes")
}

func indexChanged(dir string, err error) tea.Msg {
	if err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{dir: dir}
}

func popupResult(dir string, err error, notice string) tea.Msg {
	if err != nil {
		return popupFailedMsg{dir: dir, err: err}
	}
	return popupSucceededMsg{dir: dir, notice: notice}
}

type focusedRequest struct {
	repositories []gitrepo.Repository
}

func (r focusedRequest) loadUncommitted() tea.Msg {
	uncommitted := map[string]bool{}
	for _, repository := range r.repositories {
		status, err := repository.Status(context.Background())
		if err != nil {
			return gitFailedMsg{err: err}
		}
		uncommitted[repository.Dir] = len(status.Files) > 0
	}
	return uncommittedLoadedMsg{focused: r.repositories, uncommitted: uncommitted}
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
	return popupResult(r.repository.Dir, r.repository.Push(context.Background()), "pushed to "+r.upstream)
}

func (r pushRequest) forcePush() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.ForcePush(context.Background()), "pushed to "+r.upstream)
}

type amendRequest struct {
	repository gitrepo.Repository
	commit     gitrepo.Commit
}

func (r amendRequest) amend() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.Amend(context.Background(), r.commit.Hash),
		"amended "+r.commit.ShortHash+" "+r.commit.Summary)
}

type fileRequest struct {
	repository gitrepo.Repository
	file       gitrepo.FileStatus
}

type fileDiffRequest struct {
	repository   gitrepo.Repository
	file         gitrepo.FileStatus
	contextLines int
}

func (r fileDiffRequest) load() tea.Msg {
	text, err := r.repository.FileDiff(context.Background(), r.file, r.contextLines)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	fileDiff, err := diff.ParseUnified(text)
	if err != nil {
		return gitFailedMsg{err: fmt.Errorf("parse diff of %s: %w", r.file.Path, err)}
	}
	highlighted, err := highlight.FileDiff(r.file.Path, fileDiff)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	return diffLoadedMsg{dir: r.repository.Dir, path: r.file.Path, contextLines: r.contextLines, fileDiff: highlighted}
}

func (r fileRequest) stage() tea.Msg {
	return indexChanged(r.repository.Dir, r.repository.Stage(context.Background(), r.file))
}

func (r fileRequest) unstage() tea.Msg {
	return indexChanged(r.repository.Dir, r.repository.Unstage(context.Background(), r.file))
}

func (r fileRequest) discard() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.Discard(context.Background(), r.file), "discarded "+r.file.Path)
}

type commitDiffRequest struct {
	repository   gitrepo.Repository
	hash         string
	contextLines int
}

func (r commitDiffRequest) load() tea.Msg {
	text, err := r.repository.CommitDiff(context.Background(), r.hash, r.contextLines)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	sections, err := diff.ParseFiles(text)
	if err != nil {
		return gitFailedMsg{err: fmt.Errorf("parse diff of commit %s: %w", r.hash, err)}
	}
	highlighted := []diff.FileSection{}
	for _, section := range sections {
		fileDiff, err := highlight.FileDiff(section.Path, section.FileDiff)
		if err != nil {
			return gitFailedMsg{err: fmt.Errorf("commit %s: %w", r.hash, err)}
		}
		highlighted = append(highlighted, diff.FileSection{Path: section.Path, FileDiff: fileDiff})
	}
	return commitDiffLoadedMsg{dir: r.repository.Dir, hash: r.hash, contextLines: r.contextLines, sections: highlighted}
}

type commitRequest struct {
	repository gitrepo.Repository
	message    string
}

func (r commitRequest) commit() tea.Msg {
	summary, err := r.repository.Commit(context.Background(), r.message)
	if err != nil {
		return popupFailedMsg{dir: r.repository.Dir, err: err}
	}
	return committedMsg{dir: r.repository.Dir, summary: summary}
}
