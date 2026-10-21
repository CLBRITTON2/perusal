package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// Name is the work tree's path below the directory the Lister searched.
type ListedRepository struct {
	Name       string
	Repository gitrepo.Repository
}

// Refusals are the work trees that failed a check and the directories that could not be listed.
type Listing struct {
	Repositories []ListedRepository
	Refusals     []string
}

type Lister func(context.Context) (Listing, error)

// behindDefault counts the commits HEAD lacks from defaultBranch, and stays 0 when defaultBranch is the upstream, whose
// count the status already holds.
type overviewRow struct {
	listed        ListedRepository
	status        gitrepo.Status
	defaultBranch string
	behindDefault int
}

type overview struct {
	open     bool
	loaded   bool
	rows     []overviewRow
	selected int
	offset   int
}

// problems are the refusals and the repositories git failed on, which get no row.
type overviewLoadedMsg struct {
	rows     []overviewRow
	problems []string
}

type overviewFailedMsg struct {
	err error
}

type overviewRequest struct {
	list Lister
}

func (r overviewRequest) load() tea.Msg {
	listing, err := r.list(context.Background())
	if err != nil {
		return overviewFailedMsg{err: err}
	}
	rows := make([]overviewRow, len(listing.Repositories))
	failures := make([]error, len(listing.Repositories))
	var group sync.WaitGroup
	for index, listed := range listing.Repositories {
		group.Go(func() {
			rows[index], failures[index] = loadOverviewRow(listed)
		})
	}
	group.Wait()
	loaded := []overviewRow{}
	problems := slices.Clone(listing.Refusals)
	for index, row := range rows {
		if failures[index] != nil {
			problems = append(problems, failures[index].Error())
		} else {
			loaded = append(loaded, row)
		}
	}
	return overviewLoadedMsg{rows: loaded, problems: problems}
}

func loadOverviewRow(listed ListedRepository) (overviewRow, error) {
	ctx := context.Background()
	status, err := listed.Repository.Status(ctx)
	if err != nil {
		return overviewRow{}, err
	}
	defaultBranch, err := listed.Repository.DefaultBranch(ctx)
	if err != nil {
		return overviewRow{}, err
	}
	row := overviewRow{listed: listed, status: status, defaultBranch: defaultBranch}
	if defaultBranch == "" || defaultBranch == status.Upstream || !status.HasCommits {
		return row, nil
	}
	behind, err := listed.Repository.CommitsBehind(ctx, defaultBranch)
	if err != nil {
		return overviewRow{}, err
	}
	row.behindDefault = behind
	return row, nil
}

func (m Model) openOverview() (tea.Model, tea.Cmd) {
	m.overview = overview{open: true, rows: []overviewRow{}}
	return m, overviewRequest{list: m.list}.load
}

// Selects the repository under review, so enter right away keeps it.
func (m Model) withOverviewLoaded(msg overviewLoadedMsg) Model {
	m.overview.loaded = true
	m.overview.rows = msg.rows
	m.overview.selected = max(slices.IndexFunc(msg.rows, func(row overviewRow) bool {
		return row.listed.Repository == m.repository
	}), 0)
	m.overview.offset = 0
	m = m.withListScrolled()
	if len(msg.problems) == 0 {
		return m
	}
	return m.withFooterError("skipped " + strings.Join(msg.problems, ", "))
}

func (m Model) updateOverview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.footerText = ""
	switch {
	case key.Matches(msg, quitKey):
		return m, tea.Quit
	case key.Matches(msg, closeOverviewKey):
		m.overview.open = false
		return m, nil
	case key.Matches(msg, downKey):
		return m.selectOverviewRow(m.overview.selected + 1), nil
	case key.Matches(msg, upKey):
		return m.selectOverviewRow(m.overview.selected - 1), nil
	case key.Matches(msg, openRepositoryKey):
		return m.openOverviewRow()
	}
	return m, nil
}

func (m Model) selectOverviewRow(index int) Model {
	if len(m.overview.rows) == 0 {
		return m
	}
	m.overview.selected = min(max(index, 0), len(m.overview.rows)-1)
	return m.withListScrolled()
}

func (m Model) openOverviewRow() (tea.Model, tea.Cmd) {
	if len(m.overview.rows) == 0 {
		return m, nil
	}
	repository := m.overview.rows[m.overview.selected].listed.Repository
	m.overview.open = false
	if repository == m.repository {
		return m, nil
	}
	return m.withFocusedRepository(repository)
}

func (m Model) renderOverviewPane() string {
	innerWidth := m.width - 2
	return renderPane("repositories", "", m.overviewLabel(), m.overviewLines(innerWidth), innerWidth, m.paneInnerHeight())
}

func (m Model) overviewLabel() string {
	if len(m.overview.rows) == 0 {
		return ""
	}
	return strconv.Itoa(m.overview.selected+1) + " of " + strconv.Itoa(len(m.overview.rows))
}

func (m Model) overviewLines(innerWidth int) []string {
	switch {
	case !m.overview.loaded:
		return []string{mutedStyle.Render("loading...")}
	case len(m.overview.rows) == 0:
		return []string{mutedStyle.Render("no repositories found")}
	}
	cells := [][]string{}
	for _, row := range m.overview.rows {
		cells = append(cells, renderOverviewCells(row))
	}
	aligned := alignColumns(cells)
	lines := []string{}
	end := min(m.overview.offset+m.paneInnerHeight(), len(aligned))
	for index := m.overview.offset; index < end; index++ {
		lines = append(lines, listRow(aligned[index], index == m.overview.selected, innerWidth))
	}
	return lines
}

func renderOverviewCells(row overviewRow) []string {
	return []string{
		repositoryStyle.Render(folderGlyph + " " + sanitizeText(row.listed.Name)),
		branchStyle.Render(branchGlyph + " " + branchLabel(row.status)),
		renderChangedCount(len(row.status.Files)),
		renderAheadBehind(row.status),
		renderBehindDefault(row),
	}
}

func renderChangedCount(count int) string {
	if count == 0 {
		return mutedStyle.Render("clean")
	}
	return warningStyle.Render(strconv.Itoa(count) + " changed")
}

func renderBehindDefault(row overviewRow) string {
	if row.behindDefault == 0 {
		return ""
	}
	return behindStyle.Render("↓"+strconv.Itoa(row.behindDefault)) + mutedStyle.Render(" "+sanitizeText(row.defaultBranch))
}

func alignColumns(rows [][]string) []string {
	widths := []int{}
	for _, row := range rows {
		for column, cell := range row {
			if column == len(widths) {
				widths = append(widths, 0)
			}
			widths[column] = max(widths[column], ansi.StringWidth(cell))
		}
	}
	lines := []string{}
	for _, row := range rows {
		padded := []string{}
		for column, cell := range row {
			padded = append(padded, cell+strings.Repeat(" ", widths[column]-ansi.StringWidth(cell)))
		}
		lines = append(lines, strings.Join(padded, "  "))
	}
	return lines
}
