package ui

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

type stashPopMsg struct {
	dir string
	ref string
}

func (r repositoryRequest) stash() tea.Msg {
	return indexChangeResult(r.repository.Dir, r.repository.Stash(context.Background()))
}

type stashRequest struct {
	repository gitrepo.Repository
	stash      gitrepo.Stash
}

func (r stashRequest) pop() tea.Msg {
	if err := r.repository.PopStash(context.Background(), r.stash); err != nil {
		return gitFailureMsg{err: err}
	}
	return stashPopMsg{dir: r.repository.Dir, ref: r.stash.Ref}
}

func (r stashRequest) drop() tea.Msg {
	return popupResult(r.repository.Dir, r.repository.DropStash(context.Background(), r.stash), "dropped "+r.stash.Ref)
}

type stashDiffRequest struct {
	repository   gitrepo.Repository
	stash        gitrepo.Stash
	contextLines int
}

func (r stashDiffRequest) load() tea.Msg {
	text, err := r.repository.StashDiff(context.Background(), r.stash, r.contextLines)
	return commitDiffResult(r.repository.Dir, r.stash.Hash, r.contextLines, text, err)
}

// A stash that is gone keeps the selection at its index, so the one below it moves up into place.
func (m Model) withStashes(stashes []gitrepo.Stash, loadedAt time.Time) Model {
	var hash string
	if len(m.stashes) > 0 {
		hash = m.stashes[m.selectedStash].Hash
	}
	index := slices.IndexFunc(stashes, func(stash gitrepo.Stash) bool { return stash.Hash == hash })
	if index >= 0 {
		m.selectedStash = index
	} else {
		m.selectedStash = min(m.selectedStash, max(len(stashes)-1, 0))
		m.selectedStashFile = commitRow
	}
	m.stashes = stashes
	m.stashesLoadedAt = loadedAt
	return m
}

// Every stash whose diff has loaded lists its files under it, since enter pops rather than expands.
func (m Model) visibleStashRows() []treeRow {
	rows := []treeRow{}
	for index, stash := range m.stashes {
		rows = append(rows, treeRow{commit: index, file: commitRow})
		for file := range m.commitDiffs[stash.Hash].sections {
			rows = append(rows, treeRow{commit: index, file: file})
		}
	}
	return rows
}

func (m Model) selectedStashRow(rows []treeRow) int {
	return slices.Index(rows, treeRow{commit: m.selectedStash, file: m.selectedStashFile})
}

func (m Model) selectStashRow(index int) (Model, tea.Cmd) {
	rows := m.visibleStashRows()
	if len(rows) == 0 {
		return m, nil
	}
	row := rows[min(max(index, 0), len(rows)-1)]
	if row.commit != m.selectedStash || row.file != m.selectedStashFile {
		m.diffView.SetYOffset(0)
	}
	m.selectedStash = row.commit
	m.selectedStashFile = row.file
	m = m.withListScrolled().withDiffContent()
	return m, m.loadSelectedStashDiff()
}

func (m Model) loadSelectedStashDiff() tea.Cmd {
	if m.view != viewStashes || len(m.stashes) == 0 {
		return nil
	}
	stash := m.stashes[m.selectedStash]
	loadedDiff, found := m.commitDiffs[stash.Hash]
	if found && loadedDiff.contextLines == m.contextLines() {
		return nil
	}
	return stashDiffRequest{repository: m.repository, stash: stash, contextLines: m.contextLines()}.load
}

func (m Model) openDropStashPopup() Model {
	stash := m.stashes[m.selectedStash]
	return m.withConfirmation(m.repository.Dir, confirmation{
		lines: []string{
			titleStyle.Render("drop " + sanitizeText(stash.Ref)),
			textStyle.Render(sanitizeText(stash.Message)),
			warningStyle.Render("this cannot be undone"),
		},
		confirmKey:  confirmDropKey,
		run:         stashRequest{repository: m.repository, stash: stash}.drop,
		runningText: "dropping...",
	})
}

func (m Model) renderStashRows(innerWidth int) []string {
	if len(m.stashes) == 0 && m.repository.Dir != "" {
		return []string{mutedStyle.Render("no stashes")}
	}
	rows := m.visibleStashRows()
	window := listWindow{offset: m.stashesOffset, selected: m.selectedStashRow(rows), height: m.listInnerHeight()}
	width := innerWidth - 1
	return window.render(len(rows), innerWidth, func(index int) string {
		row := rows[index]
		stash := m.stashes[row.commit]
		if row.file != commitRow {
			files := m.commitDiffs[stash.Hash].sections
			return renderTreeFileRow(files[row.file], row.file == len(files)-1, width)
		}
		age := shortAge(m.stashesLoadedAt.Sub(stash.AuthoredAt))
		var details string
		if stash.Branch != "" {
			details = sanitizeText(stash.Branch) + " · " + age
		} else {
			details = age
		}
		summaryWidth := max(width-ansi.StringWidth(details)-1, 0)
		return fitToWidth(fitToWidth(textStyle.Render(sanitizeText(stash.Summary)), summaryWidth)+" "+
			mutedStyle.Render(details), width)
	})
}
