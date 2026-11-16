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
				textStyle.Render("the agent added it, and this machine has no copy"),
				textStyle.Render("its branch starts where the agent's left origin's default branch"),
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

// A work tree the opener refuses gets a row saying why, while a git failure fails the whole list.
func (m Model) openWorkTrees() (tea.Model, tea.Cmd) {
	if m.repository.Dir == "" {
		return m, nil
	}
	m.mode = modeWorkTrees
	m.workTreeList = selectableList[workTreeRow]{rows: []workTreeRow{}}
	repository := m.repository
	openRepository := m.openRepository
	return m, func() tea.Msg {
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
	}
	return m, nil
}

func (m Model) isUnderReview(row workTreeRow) bool {
	return row.workTree.Path == m.repository.Dir
}

// Names a work tree below the folder holding the main one, so the main one and its siblings read as folder names.
func (m Model) workTreeCells(row workTreeRow) []string {
	mainParent := path.Dir(m.workTreeList.rows[0].workTree.Path) + "/"
	relativePath, isBelowMain := strings.CutPrefix(row.workTree.Path, mainParent)
	var name string
	if isBelowMain {
		name = folderGlyph + " " + sanitizeText(relativePath)
	} else {
		name = folderGlyph + " " + sanitizeText(row.workTree.Path)
	}
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
