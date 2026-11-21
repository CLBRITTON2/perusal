package ui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
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
	m := selectRow(t, branchesOf(t, gitrepo.Status{Branch: "fix", Files: twoFiles()}, originMain()), 4)

	_, opening := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, reloading := m.Update(opening())
	m = asModel(t, next)

	if reloading == nil || m.mode != modeBrowsing || m.repository.Dir != "C:/dev/repo/.claude/worktrees/side" {
		t.Fatalf("got reload %v, mode %d, repository %q, want side's work tree under review", reloading, m.mode,
			m.repository.Dir)
	}
}

func TestMissingWorkTreeOffer(t *testing.T) {
	t.Run("cCreatesTheOfferedWorkTreeAndClosesThePopupWithItsBase", cCreatesTheOfferedWorkTreeAndClosesThePopupWithItsBase)
	t.Run("enterLeavesTheOfferOpenWithoutCreating", enterLeavesTheOfferOpenWithoutCreating)
	t.Run("escClosesTheOfferAndItIsNotMadeAgainThisRun", escClosesTheOfferAndItIsNotMadeAgainThisRun)
}

func cCreatesTheOfferedWorkTreeAndClosesThePopupWithItsBase(t *testing.T) {
	created := []string{}
	m := offeringMissingWorkTree(t, func(_ context.Context, missing focus.MissingWorkTree) (string, error) {
		created = append(created, missing.Dir)
		return "0123456789abcdef0123456789abcdef01234567", nil
	})
	popup := ansi.Strip(m.renderConfirmPopup())

	next, creating := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m = update(t, asModel(t, next), creating())

	if !strings.Contains(popup, "create worktree worktrees/repo/claude-x") {
		t.Fatalf("got popup:\n%s\nwant it to name worktrees/repo/claude-x", popup)
	}
	if len(created) != 1 || created[0] != "C:/dev/worktrees/repo/claude-x" || m.popup.kind != noPopup ||
		m.footer.text != "created worktree worktrees/repo/claude-x at 0123456" {
		t.Fatalf("got created %q, popup %d, footer %q, want one creation and the popup closed", created,
			m.popup.kind, m.footer.text)
	}
}

func enterLeavesTheOfferOpenWithoutCreating(t *testing.T) {
	m := offeringMissingWorkTree(t, refuseToCreate)

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)

	if cmd != nil || m.popup.kind != confirmPopup || m.popup.running {
		t.Fatalf("got command %v, popup %d, running %v, want the offer still open and idle", cmd, m.popup.kind,
			m.popup.running)
	}
}

func escClosesTheOfferAndItIsNotMadeAgainThisRun(t *testing.T) {
	m := offeringMissingWorkTree(t, refuseToCreate)

	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = update(t, m, focus.Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{},
		MissingWorkTrees: []focus.MissingWorkTree{missingClaudeX()}})
	m = update(t, m, pollTickMsg{})

	if m.popup.kind != noPopup {
		t.Fatalf("got popup %d, want the declined worktree not offered again", m.popup.kind)
	}
}

func offeringMissingWorkTree(t *testing.T, createWorkTree WorkTreeCreator) Model {
	t.Helper()
	m := update(t, New(findNothing, openAnything, createWorkTree, AgentHost{}), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, focus.Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{},
		MissingWorkTrees: []focus.MissingWorkTree{missingClaudeX()}})
	m = update(t, m, pollTickMsg{})
	if m.popup.kind != confirmPopup {
		t.Fatalf("got popup %d, want the offer open", m.popup.kind)
	}
	return m
}

func missingClaudeX() focus.MissingWorkTree {
	return focus.MissingWorkTree{
		AgentDir:     "C:/agent/dev/worktrees/repo/claude-x",
		Dir:          "C:/dev/worktrees/repo/claude-x",
		RelativePath: "worktrees/repo/claude-x",
		Clone:        gitrepo.Repository{Dir: "C:/dev/repo"},
	}
}

func refuseToCreate(context.Context, focus.MissingWorkTree) (string, error) {
	return "", errors.New("refuseToCreate creates nothing")
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

func TestDeleteWorkTree(t *testing.T) {
	t.Run("anUnmergedWorkTreeWarnsAndNamesTheAgentsCopy", anUnmergedWorkTreeWarnsAndNamesTheAgentsCopy)
	t.Run("aMergedWorkTreeWithNoAgentCopySaysItIsInTheDefaultBranch",
		aMergedWorkTreeWithNoAgentCopySaysItIsInTheDefaultBranch)
	t.Run("aFailedAgentRemoveStopsTheDeleteAndShowsGitsOutput", aFailedAgentRemoveStopsTheDeleteAndShowsGitsOutput)
	t.Run("aDeleteClosesThePopupAndReloadsTheList", aDeleteClosesThePopupAndReloadsTheList)
	for name, test := range map[string]struct {
		moves       int
		underReview string
		want        string
	}{
		"refusesTheMainWorkTree": {
			moves: 0, underReview: "C:/dev/repo", want: "repo is the main work tree, which perusal never deletes",
		},
		"refusesTheWorkTreeUnderReview": {
			moves: 1, underReview: "C:/dev/repo-side", want: "repo-side is under review, so it cannot be deleted",
		},
		"refusesADetachedHead": {
			moves: 2, underReview: "C:/dev/repo",
			want: "repo/.claude/worktrees/agent has a detached HEAD, so it cannot be deleted",
		},
		"refusesARefusedWorkTree": {
			moves: 3, underReview: "C:/dev/repo",
			want: "D:/elsewhere/repo-out is refused: " + focus.ErrGitDirUnprotected.Error(),
		},
	} {
		t.Run(name+"WithoutRunningGit", func(t *testing.T) {
			m := selectRow(t, workTreesOf(t), test.moves)
			m.repository.Dir = test.underReview

			next, cmd := m.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
			m = asModel(t, next)

			if cmd != nil || m.popup.kind != noPopup || !m.footer.isError || m.footer.text != test.want {
				t.Fatalf("got command %v, popup %d, footer %q, want the refusal %q", cmd, m.popup.kind, m.footer.text,
					test.want)
			}
		})
	}
	for name, test := range map[string]struct {
		check func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg
		want  string
	}{
		"refusesUncommittedChanges": {
			check: func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg {
				check.uncommitted = true
				return check
			},
			want: "repo-side has uncommitted changes, so it cannot be deleted",
		},
		"refusesWhenTheDefaultBranchIsUnknown": {
			check: func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg {
				check.defaultBranch = gitrepo.RemoteBranch{}
				return check
			},
			want: "origin's default branch is unknown, so perusal cannot rule out that fix is it",
		},
		"refusesTheDefaultBranch": {
			check: func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg {
				check.defaultBranch = gitrepo.RemoteBranch{Ref: "origin/fix", Name: "fix"}
				return check
			},
			want: "fix is the default branch, which perusal never deletes",
		},
		"refusesUncommittedChangesInTheAgentsCopy": {
			check: func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg {
				check.request.agentWorkTree.Uncommitted = true
				return check
			},
			want: "repo-side has uncommitted changes in claude-vm, so it cannot be deleted",
		},
		"refusesTheAgentsCopyOnAnotherBranch": {
			check: func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg {
				check.request.agentWorkTree.Head.Branch = "refs/heads/other"
				return check
			},
			want: "repo-side is on other in claude-vm, so it cannot be deleted",
		},
		"refusesAgentCommitsPerusalHasNotTaken": {
			check: func(check workTreeDeleteCheckMsg) workTreeDeleteCheckMsg {
				check.agentTaken = false
				return check
			},
			want: "claude-vm has commits perusal has not taken, so repo-side cannot be deleted",
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := update(t, selectRow(t, workTreesOf(t), 1), test.check(repoSideDeleteCheck(removeNothing)))

			if m.popup.kind != noPopup || !m.footer.isError || m.footer.text != test.want {
				t.Fatalf("got popup %d, footer %q, want the refusal %q", m.popup.kind, m.footer.text, test.want)
			}
		})
	}
	for name, stale := range map[string]func(m Model) Model{
		"dropsACheckForThePreviousRepository": func(m Model) Model {
			m.repository.Dir = "C:/dev/other"
			return m
		},
		"dropsACheckThatArrivesAfterTheWorkTreesScreenClosed": func(m Model) Model {
			m.mode = modeBrowsing
			return m
		},
		"dropsACheckThatArrivesWhileAPopupIsOpen": func(m Model) Model {
			m.popup.kind = confirmPopup
			return m
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := update(t, stale(selectRow(t, workTreesOf(t), 1)), repoSideDeleteCheck(removeNothing))

			if slices.Contains(m.popup.confirmation.lines, titleStyle.Render("delete repo-side")) || m.footer.text != "" {
				t.Fatalf("got lines %q, footer %q, want the check dropped", m.popup.confirmation.lines, m.footer.text)
			}
		})
	}
}

func anUnmergedWorkTreeWarnsAndNamesTheAgentsCopy(t *testing.T) {
	m := update(t, selectRow(t, workTreesOf(t), 1), repoSideDeleteCheck(removeNothing))

	want := []string{
		titleStyle.Render("delete repo-side"),
		warningStyle.Render("origin/main lacks some of fix"),
		textStyle.Render("also deletes the copy in claude-vm"),
	}
	if m.popup.kind != confirmPopup || !slices.Equal(m.popup.confirmation.lines, want) {
		t.Fatalf("got popup %d, lines %q, want %q", m.popup.kind, m.popup.confirmation.lines, want)
	}
}

func aMergedWorkTreeWithNoAgentCopySaysItIsInTheDefaultBranch(t *testing.T) {
	check := repoSideDeleteCheck(removeNothing)
	check.request.agentHost = AgentHost{}
	check.request.agentWorkTree = gitrepo.AgentWorkTree{}
	check.mergedIntoDefault = true

	m := update(t, selectRow(t, workTreesOf(t), 1), check)

	want := []string{titleStyle.Render("delete repo-side"), textStyle.Render("fix is in origin/main")}
	if m.popup.kind != confirmPopup || !slices.Equal(m.popup.confirmation.lines, want) {
		t.Fatalf("got popup %d, lines %q, want %q", m.popup.kind, m.popup.confirmation.lines, want)
	}
}

func aFailedAgentRemoveStopsTheDeleteAndShowsGitsOutput(t *testing.T) {
	ownerClones := []string{}
	m := update(t, selectRow(t, workTreesOf(t), 1), repoSideDeleteCheck(
		func(_ context.Context, ownerClone string, _ gitrepo.AgentWorkTree) error {
			ownerClones = append(ownerClones, ownerClone)
			return errors.New("fatal: cannot remove a locked working tree")
		}))

	next, deleting := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, next)
	popup := ansi.Strip(m.renderConfirmPopup())
	m = update(t, m, deleting())

	if !strings.Contains(popup, "deleting...") {
		t.Fatalf("got popup:\n%s\nwant it to say deleting...", popup)
	}
	if !slices.Equal(ownerClones, []string{"C:/dev/repo"}) || m.popup.kind != confirmPopup ||
		m.popup.err != "fatal: cannot remove a locked working tree" {
		t.Fatalf("got owner clones %q, popup %d, error %q, want git's output in the popup", ownerClones, m.popup.kind,
			m.popup.err)
	}
}

func aDeleteClosesThePopupAndReloadsTheList(t *testing.T) {
	m := update(t, selectRow(t, workTreesOf(t), 1), repoSideDeleteCheck(removeNothing))
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	next, reloading := m.Update(workTreeDeletedMsg{
		success: popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted repo-side"},
	})
	m = asModel(t, next)
	reloads := []string{}
	for _, cmd := range reloading().(tea.BatchMsg) {
		if failure, isWorkTreeList := cmd().(workTreesFailureMsg); isWorkTreeList {
			reloads = append(reloads, failure.dir)
		}
	}

	if m.popup.kind != noPopup || m.mode != modeWorkTrees || m.footer.text != "deleted repo-side" ||
		!slices.Equal(reloads, []string{"C:/dev/repo"}) {
		t.Fatalf("got popup %d, mode %d, footer %q, work tree list reloads %q, want the list reloaded",
			m.popup.kind, m.mode, m.footer.text, reloads)
	}
}

func TestDeleteWorkTreeInTheOverview(t *testing.T) {
	t.Run("aLinkedWorkTreeRunsTheCheck", aLinkedWorkTreeRunsTheCheck)
	t.Run("aCheckOpensThePopupOnTheMainClone", aCheckOpensThePopupOnTheMainClone)
	t.Run("aDeleteReloadsTheOverview", aDeleteReloadsTheOverview)
	for name, test := range map[string]struct {
		target overviewRow
		want   string
	}{
		"refusesAMainClone": {
			target: repositoryRow{
				FoundRepository: focus.FoundRepository{RelativePath: "other", Repository: gitrepo.Repository{
					Dir: "C:/dev/other", GitDir: filepath.FromSlash("C:/dev/other/.git"),
				}},
				status: gitrepo.Status{Branch: "main", HasCommits: true},
			},
			want: "other is the main work tree, which perusal never deletes",
		},
		"refusesTheWorkTreeUnderReview": {
			target: linkedWorkTreeRow("C:/dev/first", "fix"),
			want:   "worktrees/repo/fix is under review, so it cannot be deleted",
		},
		"refusesADetachedHead": {
			target: linkedWorkTreeRow("C:/dev/worktrees/repo/fix", ""),
			want:   "worktrees/repo/fix has a detached HEAD, so it cannot be deleted",
		},
		"refusesAWorkTreeWhoseMainCloneIsNotLoaded": {
			target: repositoryRow{
				FoundRepository: focus.FoundRepository{RelativePath: "worktrees/gone/fix", Repository: gitrepo.Repository{
					Dir: "C:/dev/worktrees/gone/fix", GitDir: filepath.FromSlash("C:/dev/gone/.git/worktrees/fix"),
				}},
				status: gitrepo.Status{Branch: "fix", HasCommits: true},
			},
			want: "worktrees/gone/fix's main clone is not loaded in the overview",
		},
		"refusesARefusedRow": {
			target: refusedRow{FoundRefusal: focus.FoundRefusal{RelativePath: "worktrees/repo/fix",
				Err: focus.ErrGitDirUnprotected}},
			want: "worktrees/repo/fix is refused: " + focus.ErrGitDirUnprotected.Error(),
		},
	} {
		t.Run(name+"WithoutRunningGit", func(t *testing.T) {
			m := overviewWithMainClone(t, test.target)

			next, cmd := m.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
			m = asModel(t, next)

			if cmd != nil || m.popup.kind != noPopup || !m.footer.isError || m.footer.text != test.want {
				t.Fatalf("got command %v, popup %d, footer %q, want the refusal %q", cmd, m.popup.kind, m.footer.text,
					test.want)
			}
		})
	}
}

// aLinkedWorkTreeRunsTheCheck runs the check's first git call, the work tree's status, which fails on the made up
// directory and so shows the check runs in the work tree.
func aLinkedWorkTreeRunsTheCheck(t *testing.T) {
	m := overviewWithMainClone(t, linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "fix"))

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	m = asModel(t, next)

	if cmd == nil || m.footer.text != "" {
		t.Fatalf("got command %v, footer %q, want the check to run", cmd, m.footer.text)
	}
	failure, isFailure := cmd().(gitFailureMsg)
	if !isFailure || !strings.Contains(failure.err.Error(), "C:/dev/worktrees/repo/fix") {
		t.Fatalf("got %#v, want the work tree's status to fail on its made up directory", failure)
	}
}

func aCheckOpensThePopupOnTheMainClone(t *testing.T) {
	m := update(t, overviewWithMainClone(t, linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "fix")), overviewFixCheck())

	want := []string{titleStyle.Render("delete worktrees/repo/fix"), warningStyle.Render("origin/main lacks some of fix")}
	if m.popup.kind != confirmPopup || m.popup.dir != "C:/dev/repo" || !slices.Equal(m.popup.confirmation.lines, want) {
		t.Fatalf("got popup %d for %q, lines %q, want %q for C:/dev/repo", m.popup.kind, m.popup.dir,
			m.popup.confirmation.lines, want)
	}
}

func aDeleteReloadsTheOverview(t *testing.T) {
	m := update(t, overviewWithMainClone(t, linkedWorkTreeRow("C:/dev/worktrees/repo/fix", "fix")), overviewFixCheck())
	m = update(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	next, reloading := m.Update(workTreeDeletedMsg{
		success: popupSuccessMsg{dir: "C:/dev/repo", notice: "deleted worktrees/repo/fix"},
	})
	m = asModel(t, next)
	overviewReloads := 0
	for _, cmd := range reloading().(tea.BatchMsg) {
		if cmd == nil {
			continue
		}
		if _, isOverview := cmd().(overviewMsg); isOverview {
			overviewReloads++
		}
	}

	if m.popup.kind != noPopup || m.mode != modeOverview || m.footer.text != "deleted worktrees/repo/fix" ||
		overviewReloads != 1 {
		t.Fatalf("got popup %d, mode %d, footer %q, %d overview reloads, want the overview reloaded once",
			m.popup.kind, m.mode, m.footer.text, overviewReloads)
	}
}

// overviewWithMainClone lists target first, then the main clone of C:/dev/repo, while C:/dev/first is under review.
func overviewWithMainClone(t *testing.T, target overviewRow) Model {
	t.Helper()
	m := focusOnSeveral(t, map[string]bool{"C:/dev/cwd": false, "C:/dev/first": true, "C:/dev/second": false})
	m = update(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	return update(t, m, overviewMsg{rows: []overviewRow{
		target,
		repositoryRow{
			FoundRepository: focus.FoundRepository{RelativePath: "repo", Repository: gitrepo.Repository{
				Dir: "C:/dev/repo", GitDir: filepath.FromSlash("C:/dev/repo/.git"),
			}},
			status:        gitrepo.Status{Branch: "main", HasCommits: true},
			defaultBranch: originMain(),
		},
	}})
}

// linkedWorkTreeRow is the work tree fix of C:/dev/repo at dir, on branch.
func linkedWorkTreeRow(dir string, branch string) repositoryRow {
	return repositoryRow{
		FoundRepository: focus.FoundRepository{RelativePath: "worktrees/repo/fix", Repository: gitrepo.Repository{
			Dir: dir, GitDir: filepath.FromSlash("C:/dev/repo/.git/worktrees/fix"),
		}},
		status: gitrepo.Status{Branch: branch, HasCommits: true},
	}
}

// overviewFixCheck passes every check of an overview delete of worktrees/repo/fix, which has no agent copy.
func overviewFixCheck() workTreeDeleteCheckMsg {
	return workTreeDeleteCheckMsg{
		request: workTreeDeleteRequest{
			repository: gitrepo.Repository{Dir: "C:/dev/repo"},
			row: workTreeRow{
				workTree:   gitrepo.WorkTree{Path: "C:/dev/worktrees/repo/fix", Branch: "fix"},
				repository: gitrepo.Repository{Dir: "C:/dev/worktrees/repo/fix"},
			},
			folder:        "worktrees/repo/fix",
			ownerClone:    "C:/dev/repo",
			agentHost:     AgentHost{},
			agentWorkTree: gitrepo.AgentWorkTree{},
			screen:        modeOverview,
			reviewedDir:   "C:/dev/first",
		},
		uncommitted:       false,
		defaultBranch:     originMain(),
		mergedIntoDefault: false,
		agentTaken:        false,
	}
}

// repoSideDeleteCheck passes every check of a delete of repo-side, which has an agent copy on claude-vm.
func repoSideDeleteCheck(removeAgentWorkTree func(ctx context.Context, ownerClone string,
	agentWorkTree gitrepo.AgentWorkTree) error) workTreeDeleteCheckMsg {
	return workTreeDeleteCheckMsg{
		request: workTreeDeleteRequest{
			repository: gitrepo.Repository{Dir: "C:/dev/repo"},
			row:        fourWorkTrees()[1],
			folder:     "repo-side",
			ownerClone: "C:/dev/repo",
			agentHost:  AgentHost{Name: "claude-vm", ReadWorkTree: nil, RemoveWorkTree: removeAgentWorkTree},
			agentWorkTree: gitrepo.AgentWorkTree{
				Dir:    "C:/agent/dev/repo-side",
				Exists: true,
				Head:   gitrepo.AgentHead{Branch: "refs/heads/fix", Commit: "0123456789abcdef0123456789abcdef01234567"},
			},
			screen:      modeWorkTrees,
			reviewedDir: "C:/dev/repo",
		},
		uncommitted:       false,
		defaultBranch:     originMain(),
		mergedIntoDefault: false,
		agentTaken:        true,
	}
}

func removeNothing(context.Context, string, gitrepo.AgentWorkTree) error {
	return errors.New("removeNothing removes nothing")
}
