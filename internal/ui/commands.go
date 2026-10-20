package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

type filesLoadedMsg struct {
	branch string
	files  []gitrepo.FileStatus
}

type diffLoadedMsg struct {
	path     string
	fileDiff diff.FileDiff
}

type indexChangedMsg struct{}

type committedMsg struct {
	summary string
}

type commitFailedMsg struct {
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
	return filesLoadedMsg{branch: branch, files: files}
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
	return diffLoadedMsg{path: r.file.Path, fileDiff: fileDiff}
}

func (r fileRequest) stage() tea.Msg {
	if err := r.repository.Stage(context.Background(), r.file); err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{}
}

func (r fileRequest) unstage() tea.Msg {
	if err := r.repository.Unstage(context.Background(), r.file); err != nil {
		return gitFailedMsg{err: err}
	}
	return indexChangedMsg{}
}

type commitRequest struct {
	repository gitrepo.Repository
	message    string
}

func (r commitRequest) commit() tea.Msg {
	summary, err := r.repository.Commit(context.Background(), r.message)
	if err != nil {
		return commitFailedMsg{err: err}
	}
	return committedMsg{summary: summary}
}
