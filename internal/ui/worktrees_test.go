package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

func TestWorkTreeList(t *testing.T) {
	t.Run("namesEachWorkTreeWithItsBranchAndChanges", namesEachWorkTreeWithItsBranchAndChanges)
	t.Run("enterReviewsTheSelectedWorkTree", enterReviewsTheSelectedWorkTree)
	t.Run("enterOnARefusedWorkTreeShowsWhy", enterOnARefusedWorkTreeShowsWhy)
	t.Run("enterOnTheWorkTreeUnderReviewClosesTheList", enterOnTheWorkTreeUnderReviewClosesTheList)
}

func namesEachWorkTreeWithItsBranchAndChanges(t *testing.T) {
	m := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	next, loading := m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = update(t, asModel(t, next), workTreesMsg{dir: "C:/dev/repo", rows: fourWorkTrees()})

	screen := ansi.Strip(m.renderScreen())

	afterMark := strings.TrimLeft(screen[strings.Index(screen, "*")+1:], " ")
	if strings.Count(screen, "*") != 1 || !strings.HasPrefix(afterMark, folderGlyph+" repo ") {
		t.Fatalf("got screen:\n%s\nwant only repo marked as under review", screen)
	}
	for _, want := range []string{
		branchGlyph + " main", "clean",
		folderGlyph + " repo-side", branchGlyph + " fix", "2 changed",
		folderGlyph + " repo/.claude/worktrees/agent", branchGlyph + " detached HEAD",
		folderGlyph + " D:/elsewhere/repo-out", "not write-denied to the agent",
	} {
		if loading == nil || !strings.Contains(screen, want) {
			t.Fatalf("got load %v, screen:\n%s\nwant %q", loading, screen, want)
		}
	}
}

func enterReviewsTheSelectedWorkTree(t *testing.T) {
	m := selectRow(t, workTreesOf(t), 1)

	next, reloading := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	if reloading == nil || m.mode != modeBrowsing || m.repository.Dir != "C:/dev/repo-side" {
		t.Fatalf("got reload %v, mode %d, repository %q, want repo-side under review", reloading, m.mode,
			m.repository.Dir)
	}
}

func enterOnARefusedWorkTreeShowsWhy(t *testing.T) {
	m := selectRow(t, workTreesOf(t), 3)

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	if cmd != nil || m.mode != modeWorkTrees || !m.footer.isError ||
		m.footer.text != "D:/elsewhere/repo-out is refused: "+focus.ErrGitDirUnprotected.Error() {
		t.Fatalf("got command %v, mode %d, footer %q, want the refusal with the list still open", cmd, m.mode,
			m.footer.text)
	}
}

func enterOnTheWorkTreeUnderReviewClosesTheList(t *testing.T) {
	m := workTreesOf(t)

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	if cmd != nil || m.mode != modeBrowsing || m.repository.Dir != "C:/dev/repo" {
		t.Fatalf("got command %v, mode %d, repository %q, want the list closed on repo", cmd, m.mode,
			m.repository.Dir)
	}
}

func TestEnterOnABranchInAnotherWorkTreeReviewsThatWorkTree(t *testing.T) {
	m := selectRow(t, branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, "origin/main"), 4)

	_, opening := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, reloading := m.Update(opening())
	m = asModel(t, next)

	if reloading == nil || m.mode != modeBrowsing || m.repository.Dir != "C:/dev/repo/.claude/worktrees/side" {
		t.Fatalf("got reload %v, mode %d, repository %q, want side's work tree under review", reloading, m.mode,
			m.repository.Dir)
	}
}

func workTreesOf(t *testing.T) Model {
	t.Helper()
	m := reviewingRepository(t, gitrepo.Status{Branch: "main"})
	m = update(t, m, tea.KeyPressMsg{Code: 'w', Text: "w"})
	return update(t, m, workTreesMsg{dir: "C:/dev/repo", rows: fourWorkTrees()})
}

func fourWorkTrees() []workTreeRow {
	return []workTreeRow{
		{
			workTree:     gitrepo.WorkTree{Path: "C:/dev/repo", Branch: "main"},
			repository:   gitrepo.Repository{Dir: "C:/dev/repo"},
			changedCount: 0,
		},
		{
			workTree:     gitrepo.WorkTree{Path: "C:/dev/repo-side", Branch: "fix"},
			repository:   gitrepo.Repository{Dir: "C:/dev/repo-side"},
			changedCount: 2,
		},
		{
			workTree:   gitrepo.WorkTree{Path: "C:/dev/repo/.claude/worktrees/agent", Branch: ""},
			repository: gitrepo.Repository{Dir: "C:/dev/repo/.claude/worktrees/agent"},
		},
		{
			workTree: gitrepo.WorkTree{Path: "D:/elsewhere/repo-out", Branch: "out"},
			refusal:  focus.ErrGitDirUnprotected,
		},
	}
}

func openAnything(_ context.Context, dir string) (gitrepo.Repository, error) {
	return gitrepo.Repository{Dir: dir}, nil
}
