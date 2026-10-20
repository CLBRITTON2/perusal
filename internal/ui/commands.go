package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// RepositoryFocused switches the review to Repository, unless it is already on screen.
type RepositoryFocused struct {
	Repository gitrepo.Repository
}

// FocusRejected clears the review and shows Reason, so nothing of the previous repository stays on screen.
type FocusRejected struct {
	Reason string
}

// The messages below carry dir, the repository they came from, so a result that lands after a switch is dropped.

type filesLoadedMsg struct {
	dir    string
	branch string
	files  []gitrepo.FileStatus
}

type diffLoadedMsg struct {
	dir      string
	path     string
	fileDiff diff.FileDiff
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

type gitFailedMsg struct {
	err error
}

type filesRequest struct {
	repository gitrepo.Repository
}

func (r filesRequest) load() tea.Msg {
	ctx := context.Background()
	branch, err := r.repository.CurrentBranch(ctx)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	files, err := r.repository.ChangedFiles(ctx)
	if err != nil {
		return gitFailedMsg{err: err}
	}
	return filesLoadedMsg{dir: r.repository.Dir, branch: branch, files: files}
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
