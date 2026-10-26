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

type FocusRejection struct {
	Reason string
}

type pollTickMsg struct{}

func nextPoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// The messages below carry dir, the repository they came from, so a result that lands after a switch is dropped.

type statusMsg struct {
	dir               string
	status            gitrepo.Status
	stagedLineCounts  diff.LineCounts
	branchCount       int
	remoteBranchCount int
	workTrees         []gitrepo.WorkTree
}

type diffMsg struct {
	dir          string
	path         string
	contextLines int
	fileDiff     diff.FileDiff
}

// logMsg carries the limit it was read with, so a reload still in flight when the limit grows is dropped.
type logMsg struct {
	dir      string
	limit    int
	commits  []gitrepo.Commit
	loadedAt time.Time
}

type commitDiffMsg struct {
	dir          string
	hash         string
	contextLines int
	sections     []diff.FileSection
}

// uncommittedMsg carries the focused repositories it checked instead of a dir.
type uncommittedMsg struct {
	focused     []gitrepo.Repository
	uncommitted map[string]bool
}

type indexChangeMsg struct {
	dir string
}

type commitMsg struct {
	dir     string
	summary string
}

type popupSuccessMsg struct {
	dir    string
	notice string
}

type popupFailureMsg struct {
	dir string
	err error
}

type gitFailureMsg struct {
	err error
}

type repositoryRequest struct {
	repository gitrepo.Repository
}

func (r repositoryRequest) loadStatus() tea.Msg {
	status, err := r.repository.Status(context.Background())
	if err != nil {
		return gitFailureMsg{err: err}
	}
	stagedLineCounts, err := r.repository.StagedLineCounts(context.Background())
	if err != nil {
		return gitFailureMsg{err: err}
	}
	branches, err := r.repository.Branches(context.Background())
	if err != nil {
		return gitFailureMsg{err: err}
	}
	remoteBranches, err := r.repository.RemoteBranches(context.Background())
	if err != nil {
		return gitFailureMsg{err: err}
	}
	workTrees, err := r.repository.WorkTrees(context.Background())
	if err != nil {
		return gitFailureMsg{err: err}
	}
	return statusMsg{dir: r.repository.Dir, status: status, stagedLineCounts: stagedLineCounts,
		branchCount: len(branches), remoteBranchCount: len(remoteOnlyBranches(branches, remoteBranches)),
		workTrees: workTrees}
}

func (r repositoryRequest) stageAll() tea.Msg {
	return indexChangeResult(r.repository.Dir, r.repository.StageAll(context.Background()))
}

func (r repositoryRequest) unstageAll() tea.Msg {
	return indexChangeResult(r.repository.Dir, r.repository.UnstageAll(context.Background()))
}

func (r repositoryRequest) discardAll() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.DiscardAll(context.Background()), "discarded all changes")
}

func indexChangeResult(dir string, err error) tea.Msg {
	if err != nil {
		return gitFailureMsg{err: err}
	}
	return indexChangeMsg{dir: dir}
}

func popupResult(dir string, err error, notice string) tea.Msg {
	if err != nil {
		return popupFailureMsg{dir: dir, err: err}
	}
	return popupSuccessMsg{dir: dir, notice: notice}
}

type focusedRequest struct {
	repositories []gitrepo.Repository
}

func (r focusedRequest) loadUncommitted() tea.Msg {
	uncommitted := map[string]bool{}
	for _, repository := range r.repositories {
		status, err := repository.Status(context.Background())
		if err != nil {
			return gitFailureMsg{err: err}
		}
		uncommitted[repository.Dir] = len(status.Files) > 0
	}
	return uncommittedMsg{focused: r.repositories, uncommitted: uncommitted}
}

type logRequest struct {
	repository gitrepo.Repository
	limit      int
}

func (r logRequest) load() tea.Msg {
	commits, err := r.repository.Log(context.Background(), r.limit)
	if err != nil {
		return gitFailureMsg{err: err}
	}
	return logMsg{dir: r.repository.Dir, limit: r.limit, commits: commits, loadedAt: time.Now()}
}

type pushRequest struct {
	repository gitrepo.Repository
	branch     string
}

func (r pushRequest) push() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.Push(context.Background(), r.branch), "pushed to origin/"+r.branch)
}

func (r pushRequest) forcePush() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.ForcePush(context.Background(), r.branch),
		"pushed to origin/"+r.branch)
}

type publishRequest struct {
	repository gitrepo.Repository
	branch     string
}

func (r publishRequest) publish() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.Publish(context.Background(), r.branch),
		"published "+r.branch+" to origin")
}

type amendRequest struct {
	repository gitrepo.Repository
	commit     gitrepo.Commit
}

func (r amendRequest) amend() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.Amend(context.Background(), r.commit.Hash),
		"amended "+r.commit.ShortHash+" "+r.commit.Summary)
}

type rewordRequest struct {
	repository gitrepo.Repository
	commit     gitrepo.Commit
	message    string
}

func (r rewordRequest) reword() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.Reword(context.Background(), r.commit.Hash, r.message),
		"reworded "+r.commit.ShortHash)
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
		return gitFailureMsg{err: err}
	}
	fileDiff, err := diff.ParseFile(text)
	if err != nil {
		return gitFailureMsg{err: fmt.Errorf("parse diff of %s: %w", r.file.Path, err)}
	}
	highlighted, err := highlight.FileDiff(r.file.Path, fileDiff)
	if err != nil {
		return gitFailureMsg{err: err}
	}
	return diffMsg{dir: r.repository.Dir, path: r.file.Path, contextLines: r.contextLines, fileDiff: highlighted}
}

func (r fileRequest) stage() tea.Msg {
	return indexChangeResult(r.repository.Dir, r.repository.Stage(context.Background(), r.file))
}

func (r fileRequest) unstage() tea.Msg {
	return indexChangeResult(r.repository.Dir, r.repository.Unstage(context.Background(), r.file))
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
		return gitFailureMsg{err: err}
	}
	sections, err := diff.ParseFiles(text)
	if err != nil {
		return gitFailureMsg{err: fmt.Errorf("parse diff of commit %s: %w", r.hash, err)}
	}
	highlighted := []diff.FileSection{}
	for _, section := range sections {
		fileDiff, err := highlight.FileDiff(section.Path, section.FileDiff)
		if err != nil {
			return gitFailureMsg{err: fmt.Errorf("commit %s: %w", r.hash, err)}
		}
		highlighted = append(highlighted, diff.FileSection{Path: section.Path, FileDiff: fileDiff})
	}
	return commitDiffMsg{dir: r.repository.Dir, hash: r.hash, contextLines: r.contextLines, sections: highlighted}
}

type commitRequest struct {
	repository gitrepo.Repository
	message    string
}

func (r commitRequest) commit() tea.Msg {
	summary, err := r.repository.Commit(context.Background(), r.message)
	if err != nil {
		return popupFailureMsg{dir: r.repository.Dir, err: err}
	}
	return commitMsg{dir: r.repository.Dir, summary: summary}
}
