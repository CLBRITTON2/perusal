package ui

import (
	"context"
	"errors"
	"maps"
	"path"
	"strings"
	"sync"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// RepositoryOpener opens the work tree at dir, applying the same checks as the repositories perusal was given.
type RepositoryOpener func(ctx context.Context, dir string) (gitrepo.Repository, error)

// WorkTreeCreator adds the owner's copy of an agent's worktree and returns the commit its branch starts at.
type WorkTreeCreator func(ctx context.Context, missing focus.MissingWorkTree) (string, error)

// AgentHost reads and removes the agent's copy of an owner work tree on the machine the agent works in. Its zero value
// is no agent host, where the agent works in the owner's own work trees.
type AgentHost struct {
	Name           string
	ReadWorkTree   func(ctx context.Context, ownerWorkTree string) (gitrepo.AgentWorkTree, error)
	RemoveWorkTree func(ctx context.Context, ownerClone string, agentWorkTree gitrepo.AgentWorkTree) error
}

type workTreeCreateRequest struct {
	createWorkTree WorkTreeCreator
	missing        focus.MissingWorkTree
}

func (r workTreeCreateRequest) create() tea.Msg {
	base, err := r.createWorkTree(context.Background(), r.missing)
	if err != nil {
		return popupResult(r.missing.Dir, err, "")
	}
	return popupResult(r.missing.Dir, nil, "created worktree "+r.missing.RelativePath+" at "+base[:7])
}

// withWorkTreeOffer asks about the first missing worktree not asked about yet, once per run, so an agent naming
// worktrees cannot keep the popup up. It waits while another popup or the search is open.
func (m Model) withWorkTreeOffer() Model {
	if m.createWorkTree == nil || m.popup.kind != noPopup || m.mode == modeSearching {
		return m
	}
	for _, missing := range m.missingWorkTrees {
		if m.askedWorkTrees[missing.AgentDir] {
			continue
		}
		m.askedWorkTrees = maps.Clone(m.askedWorkTrees)
		m.askedWorkTrees[missing.AgentDir] = true
		return m.withConfirmation(missing.Dir, confirmation{
			lines: []string{
				titleStyle.Render("create worktree " + sanitizeText(missing.RelativePath)),
				textStyle.Render("the agent's VM has this worktree and this machine doesn't"),
				textStyle.Render("the new branch starts where the agent's branch split off origin's default branch"),
			},
			confirmKey:  confirmCreateKey,
			run:         workTreeCreateRequest{createWorkTree: m.createWorkTree, missing: missing}.create,
			runningText: "creating...",
		})
	}
	return m
}

// refusal is the error the opener gave, and repository and changedCount are set only when it is nil.
type workTreeRow struct {
	workTree     gitrepo.WorkTree
	repository   gitrepo.Repository
	changedCount int
	refusal      error
}

type workTreesMsg struct {
	dir  string
	rows []workTreeRow
}

type workTreesFailureMsg struct {
	dir string
	err error
}

type workTreeOpenedMsg struct {
	repository gitrepo.Repository
}

// agentTaken is read only when the agent's copy is on the work tree's branch.
type workTreeDeleteCheckMsg struct {
	request           workTreeDeleteRequest
	uncommitted       bool
	defaultBranch     gitrepo.RemoteBranch
	mergedIntoDefault bool
	agentTaken        bool
}

// repository is the main clone or another of its work trees, whose git deletes row, and ownerClone the main work
// tree's path. screen and reviewedDir are the screen mode and the repository under review when D was pressed, so a
// check that arrives after either changed is dropped.
type workTreeDeleteRequest struct {
	repository    gitrepo.Repository
	row           workTreeRow
	folder        string
	ownerClone    string
	agentHost     AgentHost
	agentWorkTree gitrepo.AgentWorkTree
	screen        screenMode
	reviewedDir   string
}

// workTreeDeletedMsg is a delete's success, which also reloads the overview, since a row of it is gone.
type workTreeDeletedMsg struct {
	success popupSuccessMsg
}

func (r workTreeDeleteRequest) check() tea.Msg {
	ctx := context.Background()
	branchRef := "refs/heads/" + r.row.workTree.Branch
	workTreeStatus, err := r.row.repository.Status(ctx)
	if err != nil {
		return gitFailureMsg{err: err}
	}
	defaultBranch, err := r.repository.DefaultBranch(ctx)
	if err != nil {
		return gitFailureMsg{err: err}
	}
	mergedIntoDefault := false
	if defaultBranch.Ref != "" {
		mergedIntoDefault, err = r.repository.MergedInto(ctx, branchRef, defaultBranch.Ref)
		if err != nil {
			return gitFailureMsg{err: err}
		}
	}
	if r.agentHost.Name != "" {
		r.agentWorkTree, err = r.agentHost.ReadWorkTree(ctx, r.row.workTree.Path)
		if err != nil {
			return gitFailureMsg{err: err}
		}
	}
	agentTaken := false
	if r.agentWorkTree.Exists && r.agentWorkTree.Head.Branch == branchRef {
		agentTaken, err = r.repository.HasTaken(ctx, r.agentWorkTree.Head)
		if err != nil {
			return gitFailureMsg{err: err}
		}
	}
	return workTreeDeleteCheckMsg{request: r, uncommitted: len(workTreeStatus.Files) > 0, defaultBranch: defaultBranch,
		mergedIntoDefault: mergedIntoDefault, agentTaken: agentTaken}
}

// Refuses what would lose work: uncommitted changes on either machine, or agent commits perusal has not taken.
func (m Model) withWorkTreeDeleteChecked(msg workTreeDeleteCheckMsg) Model {
	request := msg.request
	folder := request.folder
	branch := request.row.workTree.Branch
	agentHostName := request.agentHost.Name
	agentWorkTree := request.agentWorkTree
	switch {
	case msg.uncommitted:
		return m.withFooterError(folder + " has uncommitted changes, so it cannot be deleted")
	case msg.defaultBranch.Ref == "":
		return m.withFooterError("origin's default branch is unknown, so perusal cannot rule out that " + branch + " is it")
	case branch == msg.defaultBranch.Name:
		return m.withFooterError(branch + " is the default branch, which perusal never deletes")
	}
	if agentWorkTree.Exists {
		switch {
		case agentWorkTree.Uncommitted:
			return m.withFooterError(folder + " has uncommitted changes in " + agentHostName + ", so it cannot be deleted")
		case agentWorkTree.Head.Branch != "refs/heads/"+branch:
			return m.withFooterError(folder + " is on " + headName(strings.TrimPrefix(agentWorkTree.Head.Branch, "refs/heads/")) +
				" in " + agentHostName + ", so it cannot be deleted")
		case !msg.agentTaken:
			return m.withFooterError(agentHostName + " has commits perusal has not taken, so " + folder + " cannot be deleted")
		}
	}
	var mergedLine string
	if msg.mergedIntoDefault {
		mergedLine = textStyle.Render(sanitizeText(branch) + " is in " + sanitizeText(msg.defaultBranch.Ref))
	} else {
		mergedLine = warningStyle.Render(sanitizeText(msg.defaultBranch.Ref) + " lacks some of " + sanitizeText(branch))
	}
	lines := []string{titleStyle.Render("delete " + sanitizeText(folder)), mergedLine}
	if agentWorkTree.Exists {
		lines = append(lines, textStyle.Render("also deletes the copy in "+sanitizeText(agentHostName)))
	}
	return m.withConfirmation(request.repository.Dir, confirmation{
		lines:       lines,
		confirmKey:  confirmDeleteKey,
		run:         request.delete,
		runningText: "deleting...",
	})
}

// The agent's copy goes first, so a failure there leaves the owner's for a retry.
func (r workTreeDeleteRequest) delete() tea.Msg {
	ctx := context.Background()
	if r.agentWorkTree.Exists {
		if err := r.agentHost.RemoveWorkTree(ctx, r.ownerClone, r.agentWorkTree); err != nil {
			return popupResult(r.repository.Dir, err, "")
		}
	}
	if err := r.repository.RemoveWorkTree(ctx, r.row.workTree); err != nil {
		return popupResult(r.repository.Dir, err, "")
	}
	return workTreeDeletedMsg{success: popupSuccessMsg{dir: r.repository.Dir, notice: "deleted " + r.folder}}
}

func (m Model) openDeleteWorkTreePopup() (tea.Model, tea.Cmd) {
	row, hasRows := m.workTreeList.selectedRow()
	if !hasRows {
		return m, nil
	}
	folder := m.workTreeFolder(row)
	switch {
	case row.workTree.Path == m.workTreeList.rows[0].workTree.Path:
		return m.withFooterError(folder + " is the main work tree, which perusal never deletes"), nil
	case m.isUnderReview(row):
		return m.withFooterError(folder + " is under review, so it cannot be deleted"), nil
	case row.refusal != nil:
		return m.withFooterError(folder + " is refused: " + row.refusal.Error()), nil
	case row.workTree.Branch == "":
		return m.withFooterError(folder + " has a detached HEAD, so it cannot be deleted"), nil
	}
	return m, workTreeDeleteRequest{repository: m.repository, row: row, folder: folder,
		ownerClone: m.workTreeList.rows[0].workTree.Path, agentHost: m.agentHost,
		agentWorkTree: gitrepo.AgentWorkTree{}, screen: modeWorkTrees, reviewedDir: m.repository.Dir}.check
}

// A work tree the opener refuses gets a row saying why, while a git failure fails the whole list.
func (m Model) openWorkTrees() (tea.Model, tea.Cmd) {
	if m.repository.Dir == "" {
		return m, nil
	}
	m.mode = modeWorkTrees
	m.workTreeList = selectableList[workTreeRow]{rows: []workTreeRow{}}
	return m, m.reloadWorkTrees()
}

func (m Model) reloadWorkTrees() tea.Cmd {
	if m.mode != modeWorkTrees {
		return nil
	}
	repository := m.repository
	openRepository := m.openRepository
	return func() tea.Msg {
		ctx := context.Background()
		workTrees, err := repository.WorkTrees(ctx)
		if err != nil {
			return workTreesFailureMsg{dir: repository.Dir, err: err}
		}
		rows := make([]workTreeRow, len(workTrees))
		errs := make([]error, len(workTrees))
		var group sync.WaitGroup
		for index, workTree := range workTrees {
			group.Go(func() {
				opened, err := openRepository(ctx, workTree.Path)
				if err != nil {
					rows[index] = workTreeRow{workTree: workTree, refusal: err}
					return
				}
				status, err := opened.Status(ctx)
				if err != nil {
					errs[index] = err
					return
				}
				rows[index] = workTreeRow{workTree: workTree, repository: opened, changedCount: len(status.Files)}
			})
		}
		group.Wait()
		if err := errors.Join(errs...); err != nil {
			return workTreesFailureMsg{dir: repository.Dir, err: err}
		}
		return workTreesMsg{dir: repository.Dir, rows: rows}
	}
}

func (m Model) updateWorkTrees(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footer = footer{}
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, closeWorkTreesKey):
		m.mode = modeBrowsing
		return m, nil
	case key.Matches(msg, downKey):
		m.workTreeList = m.workTreeList.withSelected(m.workTreeList.selected + 1).scrolled(m.paneInnerHeight())
		return m, nil
	case key.Matches(msg, upKey):
		m.workTreeList = m.workTreeList.withSelected(m.workTreeList.selected - 1).scrolled(m.paneInnerHeight())
		return m, nil
	case key.Matches(msg, openRepositoryKey):
		row, hasRows := m.workTreeList.selectedRow()
		switch {
		case !hasRows:
			return m, nil
		case row.refusal != nil:
			return m.withFooterError(row.workTree.Path + " is refused: " + row.refusal.Error()), nil
		case m.isUnderReview(row):
			m.mode = modeBrowsing
			return m, nil
		}
		return m.withFocusedRepository(row.repository)
	case key.Matches(msg, deleteWorkTreeKey):
		return m.openDeleteWorkTreePopup()
	}
	return m, nil
}

func (m Model) isUnderReview(row workTreeRow) bool {
	return row.workTree.Path == m.repository.Dir
}

// Names a work tree below the folder holding the main one, so the main one and its siblings read as folder names.
func (m Model) workTreeFolder(row workTreeRow) string {
	mainParent := path.Dir(m.workTreeList.rows[0].workTree.Path) + "/"
	relativePath, isBelowMain := strings.CutPrefix(row.workTree.Path, mainParent)
	if isBelowMain {
		return relativePath
	}
	return row.workTree.Path
}

func (m Model) workTreeCells(row workTreeRow) []string {
	name := folderGlyph + " " + sanitizeText(m.workTreeFolder(row))
	if row.refusal != nil {
		return []string{"", mutedStyle.Render(name), "",
			mutedStyle.Render(sanitizeText(focus.FoundRefusal{Err: row.refusal}.Summary()))}
	}
	return []string{
		renderCurrentMark(m.isUnderReview(row)),
		repositoryStyle.Render(name),
		branchStyle.Render(branchGlyph + " " + headName(row.workTree.Branch)),
		renderChangedCount(row.changedCount),
	}
}
