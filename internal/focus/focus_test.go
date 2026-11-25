//go:build windows

package focus

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	tempDir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("TMP", tempDir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

const (
	mappedAgentDir = `C:\Users\claude\dev`
	mappedOwnerDir = `C:\Users\Chris\dev`
)

func TestMapPath(t *testing.T) {
	t.Run("mapPathMapsARepository", mapPathMapsARepository)
	t.Run("mapPathMapsANestedDirectory", mapPathMapsANestedDirectory)
	t.Run("mapPathMapsTheDevDirectoryItself", mapPathMapsTheDevDirectoryItself)
	t.Run("mapPathIgnoresCase", mapPathIgnoresCase)
	t.Run("mapPathRefusesASiblingWithTheSamePrefix", mapPathRefusesASiblingWithTheSamePrefix)
	t.Run("mapPathRefusesDotDotOutOfTheAgentDirectory", mapPathRefusesDotDotOutOfTheAgentDirectory)
	t.Run("mapPathRefusesARelativePath", mapPathRefusesARelativePath)
	t.Run("mapPathRefusesADriveRelativePath", mapPathRefusesADriveRelativePath)
	t.Run("mapPathRefusesAVerbatimPrefix", mapPathRefusesAVerbatimPrefix)
}

func mapPathMapsARepository(t *testing.T) {
	got, err := mapPath(`C:\Users\claude\dev\perusal`, mappedAgentDir, mappedOwnerDir)
	require.NoError(t, err)
	require.Equal(t, `C:\Users\Chris\dev\perusal`, got)
}

func mapPathMapsANestedDirectory(t *testing.T) {
	got, err := mapPath(`C:\Users\claude\dev\openziti\ziti\cmd`, mappedAgentDir, mappedOwnerDir)
	require.NoError(t, err)
	require.Equal(t, `C:\Users\Chris\dev\openziti\ziti\cmd`, got)
}

func mapPathMapsTheDevDirectoryItself(t *testing.T) {
	got, err := mapPath(`C:\Users\claude\dev\`, mappedAgentDir, mappedOwnerDir)
	require.NoError(t, err)
	require.Equal(t, `C:\Users\Chris\dev`, got)
}

func mapPathIgnoresCase(t *testing.T) {
	got, err := mapPath(`c:\users\CLAUDE\Dev\perusal`, mappedAgentDir, mappedOwnerDir)
	require.NoError(t, err)
	require.Equal(t, `C:\Users\Chris\dev\perusal`, got)
}

func mapPathRefusesASiblingWithTheSamePrefix(t *testing.T) {
	got, err := mapPath(`C:\Users\claude\devtools\x`, mappedAgentDir, mappedOwnerDir)
	require.ErrorIs(t, err, ErrOutsideAgentDir)
	require.Empty(t, got)
}

func mapPathRefusesDotDotOutOfTheAgentDirectory(t *testing.T) {
	got, err := mapPath(`C:\Users\claude\dev\..\..\Chris\.ssh`, mappedAgentDir, mappedOwnerDir)
	require.ErrorIs(t, err, ErrOutsideAgentDir)
	require.Empty(t, got)
}

func mapPathRefusesARelativePath(t *testing.T) {
	got, err := mapPath(`dev\perusal`, mappedAgentDir, mappedOwnerDir)
	require.ErrorIs(t, err, ErrOutsideAgentDir)
	require.Empty(t, got)
}

func mapPathRefusesADriveRelativePath(t *testing.T) {
	got, err := mapPath(`C:dev\perusal`, mappedAgentDir, mappedOwnerDir)
	require.ErrorIs(t, err, ErrOutsideAgentDir)
	require.Empty(t, got)
}

func mapPathRefusesAVerbatimPrefix(t *testing.T) {
	got, err := mapPath(`\\?\C:\Users\claude\dev\perusal`, mappedAgentDir, mappedOwnerDir)
	require.ErrorIs(t, err, ErrOutsideAgentDir)
	require.Empty(t, got)
}

func TestAgentCopyDirs(t *testing.T) {
	t.Run("agentCopyDirsMapsARepository", agentCopyDirsMapsARepository)
	t.Run("agentCopyDirsMapsANestedRepository", agentCopyDirsMapsANestedRepository)
	t.Run("agentCopyDirsMapsTheOwnerDirectoryItself", agentCopyDirsMapsTheOwnerDirectoryItself)
	t.Run("agentCopyDirsIgnoresCase", agentCopyDirsIgnoresCase)
	t.Run("agentCopyDirsRefusesARepositoryOutsideTheOwnerDirectory",
		agentCopyDirsRefusesARepositoryOutsideTheOwnerDirectory)
}

func agentCopyDirsMapsARepository(t *testing.T) {
	rules := Rules{Config: Config{AgentDir: mappedAgentDir, OwnerDir: mappedOwnerDir}}
	got, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: `C:\Users\Chris\dev\perusal`}})
	require.NoError(t, err)
	require.Equal(t, []string{`C:\Users\claude\dev\perusal`}, got)
}

func agentCopyDirsMapsANestedRepository(t *testing.T) {
	rules := Rules{Config: Config{AgentDir: mappedAgentDir, OwnerDir: mappedOwnerDir}}
	got, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: `C:\Users\Chris\dev\openziti\ziti`}})
	require.NoError(t, err)
	require.Equal(t, []string{`C:\Users\claude\dev\openziti\ziti`}, got)
}

func agentCopyDirsMapsTheOwnerDirectoryItself(t *testing.T) {
	rules := Rules{Config: Config{AgentDir: mappedAgentDir, OwnerDir: mappedOwnerDir}}
	got, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: `C:\Users\Chris\dev`}})
	require.NoError(t, err)
	require.Equal(t, []string{`C:\Users\claude\dev`}, got)
}

func agentCopyDirsIgnoresCase(t *testing.T) {
	rules := Rules{Config: Config{AgentDir: mappedAgentDir, OwnerDir: mappedOwnerDir}}
	got, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: `c:\users\chris\DEV\perusal`}})
	require.NoError(t, err)
	require.Equal(t, []string{`C:\Users\claude\dev\perusal`}, got)
}

func agentCopyDirsRefusesARepositoryOutsideTheOwnerDirectory(t *testing.T) {
	rules := Rules{Config: Config{AgentDir: mappedAgentDir, OwnerDir: mappedOwnerDir}}
	_, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: `C:\Users\Chris\devtools\x`}})
	require.Error(t, err)
}

func TestExistingOnAgentHostDropsAWorkTreeRemovedOnTheAgentHost(t *testing.T) {
	agentDir := t.TempDir()
	removedDir := filepath.Join(agentDir, "removed")
	kept := MissingWorkTree{AgentDir: agentDir, RelativePath: "worktrees/repo/kept"}
	removed := MissingWorkTree{AgentDir: removedDir, RelativePath: "worktrees/repo/removed"}

	existing, err := existingOnAgentHost(context.Background(), runLocalScript, []MissingWorkTree{kept, removed})

	require.NoError(t, err)
	require.Equal(t, []MissingWorkTree{kept}, existing)
}

func TestParseFocusFile(t *testing.T) {
	t.Run("readsOnePathPerLineWithAnyLineBreak", readsOnePathPerLineWithAnyLineBreak)
	t.Run("refusesAnEmptyLine", refusesAnEmptyLine)
}

func readsOnePathPerLineWithAnyLineBreak(t *testing.T) {
	got, err := parseFocusFile("C:\\Users\\claude\\dev\\a\r\nC:\\Users\\claude\\dev\\b\nC:\\Users\\claude\\dev\\c\r\n")

	want := []string{`C:\Users\claude\dev\a`, `C:\Users\claude\dev\b`, `C:\Users\claude\dev\c`}
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func refusesAnEmptyLine(t *testing.T) {
	_, err := parseFocusFile("C:\\Users\\claude\\dev\\a\n\nC:\\Users\\claude\\dev\\b")

	require.ErrorIs(t, err, ErrMalformedFocusFile)
}

func TestFindWorkTree(t *testing.T) {
	t.Run("findsTheNearestGitAbove", findsTheNearestGitAbove)
	t.Run("stopsAtTheOwnerDirectory", stopsAtTheOwnerDirectory)
}

func findsTheNearestGitAbove(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "org", "repo")
	mustMkdir(t, filepath.Join(repository, ".git"))
	tool := filepath.Join(repository, "cmd", "tool")
	mustMkdir(t, tool)

	got, err := findWorkTree(tool, devDir)

	require.NoError(t, err)
	require.Equal(t, repository, got)
}

func stopsAtTheOwnerDirectory(t *testing.T) {
	outer := t.TempDir()
	mustMkdir(t, filepath.Join(outer, ".git"))
	devDir := filepath.Join(outer, "dev")
	notes := filepath.Join(devDir, "notes")
	mustMkdir(t, notes)

	_, err := findWorkTree(notes, devDir)

	require.ErrorIs(t, err, ErrNotWorkTree)
}

func TestResolve(t *testing.T) {
	t.Run("opensTheCheckedWorkTree", opensTheCheckedWorkTree)
	t.Run("doesNotOpenTheRepositoryAboveAnUnusableGitDir", doesNotOpenTheRepositoryAboveAnUnusableGitDir)
	t.Run("keepsEachRepositoryOnceAndReportsTheRefusedPaths", keepsEachRepositoryOnceAndReportsTheRefusedPaths)
}

const agentDevDir = `C:\agent\dev`

func opensTheCheckedWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "repo")
	gitInit(t, repository)
	gitDir := filepath.Join(repository, ".git")
	denyGuests(t, gitDir, deniedWriteMask)
	focusFile := focusFileHolding(t, agentDevDir+`\repo`)
	rules := guestRules(t, devDir, focusFile)

	got, err := resolve(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 1)
	opened := got.Repositories[0]
	requireSameDirectory(t, opened.Dir, repository)
	sameGitDir := strings.EqualFold(opened.GitDir, gitDir)
	require.True(t, sameGitDir, "got git directory %q, want %q", opened.GitDir, gitDir)
}

func keepsEachRepositoryOnceAndReportsTheRefusedPaths(t *testing.T) {
	devDir := t.TempDir()
	first := filepath.Join(devDir, "first")
	second := filepath.Join(devDir, "second")
	for _, repository := range []string{first, second} {
		gitInit(t, repository)
		denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	}
	mustMkdir(t, filepath.Join(first, "cmd"))
	mustMkdir(t, filepath.Join(devDir, "notes"))
	lines := []string{agentDevDir + `\second`, agentDevDir + `\notes`, agentDevDir + `\first\cmd`, agentDevDir + `\first`}
	focusFile := focusFileHolding(t, strings.Join(lines, "\n"))
	rules := guestRules(t, devDir, focusFile)

	got, err := resolve(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 2)
	require.Len(t, got.Refusals, 1)
	require.ErrorIs(t, got.Refusals[0], ErrNotWorkTree)
	requireSameDirectory(t, got.Repositories[0].Dir, second)
	requireSameDirectory(t, got.Repositories[1].Dir, first)
}

func doesNotOpenTheRepositoryAboveAnUnusableGitDir(t *testing.T) {
	outer := t.TempDir()
	gitInit(t, outer)
	devDir := filepath.Join(outer, "dev")
	emptyGitDir := filepath.Join(devDir, "repo", ".git")
	mustMkdir(t, emptyGitDir)
	denyGuests(t, emptyGitDir, deniedWriteMask)
	focusFile := focusFileHolding(t, agentDevDir+`\repo`)
	rules := guestRules(t, devDir, focusFile)

	_, err := resolve(context.Background(), rules)

	require.Error(t, err)
}

func TestMissingWorkTree(t *testing.T) {
	t.Run("resolveReportsAnAgentHostsWorkTreeTheOwnerLacksOnce", resolveReportsAnAgentHostsWorkTreeTheOwnerLacksOnce)
	t.Run("resolveRefusesTheSamePathWithoutAnAgentHost", resolveRefusesTheSamePathWithoutAnAgentHost)
	t.Run("resolveAgainFindsTheSameWhileTheWorkTreeIsMissing", resolveAgainFindsTheSameWhileTheWorkTreeIsMissing)
}

func resolveAgainFindsTheSameWhileTheWorkTreeIsMissing(t *testing.T) {
	devDir := t.TempDir()
	lines := claudeXFocusLines(t, devDir)
	focusFile := focusFileHolding(t, strings.Join(lines, "\n"))
	rules := guestRules(t, devDir, focusFile)
	rules.AgentHost = "agent-vm"

	first, firstErr := resolve(context.Background(), rules)
	second, secondErr := resolve(context.Background(), rules)

	lacksClaudeX := lacksOwnerCopy(first, firstErr)
	require.True(t, lacksClaudeX)
	firstResult := Result{Focus: first, Err: firstErr}
	secondResult := Result{Focus: second, Err: secondErr}
	sameTwice := isSameResult(firstResult, secondResult)
	require.True(t, sameTwice)
	emptyResult := Result{Focus: Focus{}, Err: firstErr}
	sameAsEmpty := isSameResult(firstResult, emptyResult)
	require.False(t, sameAsEmpty)
}

func resolveReportsAnAgentHostsWorkTreeTheOwnerLacksOnce(t *testing.T) {
	devDir := t.TempDir()
	lines := claudeXFocusLines(t, devDir)
	focusFile := focusFileHolding(t, strings.Join(lines, "\n"))
	rules := guestRules(t, devDir, focusFile)
	rules.AgentHost = "agent-vm"

	got, err := resolve(context.Background(), rules)

	require.NoError(t, err)
	require.Empty(t, got.Repositories)
	require.Len(t, got.MissingWorkTrees, 1)
	require.Len(t, got.Refusals, 1)
	require.ErrorIs(t, got.Refusals[0], ErrNotWorkTree)
	missing := got.MissingWorkTrees[0]
	clone := filepath.Join(devDir, "org", "repo")
	requireSameDirectory(t, missing.Clone.Dir, clone)
	missing.Clone = gitrepo.Repository{}
	want := MissingWorkTree{
		AgentDir:     agentDevDir + `\worktrees\org\repo\claude-x`,
		Dir:          filepath.Join(devDir, "worktrees", "org", "repo", "claude-x"),
		RelativePath: "worktrees/org/repo/claude-x",
	}
	require.Equal(t, want, missing)
}

func resolveRefusesTheSamePathWithoutAnAgentHost(t *testing.T) {
	devDir := t.TempDir()
	lines := claudeXFocusLines(t, devDir)
	focusFile := focusFileHolding(t, strings.Join(lines, "\n"))
	rules := guestRules(t, devDir, focusFile)

	got, err := resolve(context.Background(), rules)

	require.ErrorIs(t, err, ErrNotWorkTree)
	require.Empty(t, got.MissingWorkTrees)
}

// claudeXFocusLines makes org\repo a protected clone and returns focus lines naming its worktree claude-x, which does
// not exist, from a folder inside it and from its root, then a worktrees path with no clone.
func claudeXFocusLines(t *testing.T, devDir string) []string {
	t.Helper()
	clone := filepath.Join(devDir, "org", "repo")
	gitInit(t, clone)
	denyGuests(t, filepath.Join(clone, ".git"), deniedWriteMask)
	return []string{
		agentDevDir + `\worktrees\org\repo\claude-x\cmd`,
		agentDevDir + `\worktrees\org\repo\claude-x`,
		agentDevDir + `\worktrees\nothing\here`,
	}
}

func TestFindRepositoriesRefusesAnUnprotectedOneAndSkipsANestedOne(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "org", "repo")
	gitInit(t, repository)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	vendored := filepath.Join(repository, "vendored")
	gitInit(t, vendored)
	denyGuests(t, filepath.Join(vendored, ".git"), deniedWriteMask)
	gitInit(t, filepath.Join(devDir, "plain"))
	mustMkdir(t, filepath.Join(devDir, "notes"))
	rules := guestRules(t, devDir, "")

	got, err := FindRepositories(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 1)
	require.Equal(t, "org/repo", got.Repositories[0].RelativePath)
	require.Len(t, got.Refusals, 1)
	refusal := got.Refusals[0]
	require.Equal(t, "plain", refusal.RelativePath)
	summary := refusal.Summary()
	require.Equal(t, "not write-denied to the agent", summary)
}

func TestFindRepositoriesListsAnUnprotectedOneForAnAgentHost(t *testing.T) {
	devDir := t.TempDir()
	repositoryWithWorkTree(t, devDir)
	rules := agentHostRules(t, agentDevDir, devDir)

	got, err := FindRepositories(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 2)
	require.Empty(t, got.Refusals)
}

func TestFindRepositoriesListsALinkedWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository, _ := repositoryWithWorkTree(t, devDir)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	rules := guestRules(t, devDir, "")

	got, err := FindRepositories(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 2)
	require.Equal(t, "repo-side", got.Repositories[1].RelativePath)
	require.Empty(t, got.Refusals)
}

func TestLinkedWorkTree(t *testing.T) {
	t.Run("opensThroughItsCheckedRepository", opensThroughItsCheckedRepository)
	t.Run("isRefusedWhenItsRepositoryIsUnprotected", isRefusedWhenItsRepositoryIsUnprotected)
	t.Run("isRefusedWhenItsGitFileNamesAnotherWorkTree", isRefusedWhenItsGitFileNamesAnotherWorkTree)
	t.Run("opensThroughAJunctionAsItsRealPath", opensThroughAJunctionAsItsRealPath)
}

func opensThroughItsCheckedRepository(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	focusFile := focusFileHolding(t, agentDevDir+`\repo-side\cmd`)
	rules := guestRules(t, devDir, focusFile)

	got, err := resolve(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 1)
	opened := got.Repositories[0]
	requireSameDirectory(t, opened.Dir, side)
	sideStatus, err := opened.Status(context.Background())
	require.NoError(t, err)
	require.Equal(t, "side", sideStatus.Branch)
}

func isRefusedWhenItsRepositoryIsUnprotected(t *testing.T) {
	devDir := t.TempDir()
	repositoryWithWorkTree(t, devDir)
	focusFile := focusFileHolding(t, agentDevDir+`\repo-side`)
	rules := guestRules(t, devDir, focusFile)

	_, err := resolve(context.Background(), rules)

	require.ErrorIs(t, err, ErrGitDirUnprotected)
}

func isRefusedWhenItsGitFileNamesAnotherWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	mustGit(t, "-C", repository, "worktree", "add", "-b", "other", filepath.Join(devDir, "repo-other"))
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	writeGitFile(t, side, filepath.Join(repository, ".git", "worktrees", "repo-other"))
	focusFile := focusFileHolding(t, agentDevDir+`\repo-side`)
	rules := guestRules(t, devDir, focusFile)

	_, err := resolve(context.Background(), rules)

	require.ErrorIs(t, err, ErrNotLinkedWorkTree)
}

func opensThroughAJunctionAsItsRealPath(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	gitDir := filepath.Join(repository, ".git")
	denyGuests(t, gitDir, deniedWriteMask)
	junction := filepath.Join(t.TempDir(), "link")
	makeJunction(t, junction, gitDir)
	writeGitFile(t, side, filepath.Join(junction, "worktrees", "repo-side"))
	focusFile := focusFileHolding(t, agentDevDir+`\repo-side`)
	rules := guestRules(t, devDir, focusFile)

	got, err := resolve(context.Background(), rules)

	require.NoError(t, err)
	require.Len(t, got.Repositories, 1)
	want := filepath.Join(gitDir, "worktrees", "repo-side")
	openedGitDir := got.Repositories[0].GitDir
	sameGitDir := strings.EqualFold(openedGitDir, want)
	require.True(t, sameGitDir, "got git directory %q, want %q", openedGitDir, want)
}

func TestACheckedRepositoryCannotBeMovedAside(t *testing.T) {
	t.Run("afterItsMainWorkTreeOpens", afterItsMainWorkTreeOpens)
	t.Run("afterItsLinkedWorkTreeOpens", afterItsLinkedWorkTreeOpens)
}

func afterItsMainWorkTreeOpens(t *testing.T) {
	repository := repositoryResolvedFrom(t, agentDevDir+`\repo`)

	err := os.Rename(repository, repository+"-moved")

	require.ErrorIs(t, err, fs.ErrPermission)
}

func afterItsLinkedWorkTreeOpens(t *testing.T) {
	repository := repositoryResolvedFrom(t, agentDevDir+`\repo-side`)

	err := os.Rename(repository, repository+"-moved")

	require.ErrorIs(t, err, fs.ErrPermission)
}

func repositoryResolvedFrom(t *testing.T, focusedAgentPath string) string {
	t.Helper()
	devDir := t.TempDir()
	repository, _ := repositoryWithWorkTree(t, devDir)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	focusFile := focusFileHolding(t, focusedAgentPath)
	rules := guestRules(t, devDir, focusFile)
	_, err := resolve(context.Background(), rules)
	require.NoError(t, err)
	return repository
}

func TestARefusedRepositoryCanStillBeMoved(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "repo")
	gitInit(t, repository)
	focusFile := focusFileHolding(t, agentDevDir+`\repo`)
	rules := guestRules(t, devDir, focusFile)
	_, err := resolve(context.Background(), rules)
	require.ErrorIs(t, err, ErrGitDirUnprotected)

	err = os.Rename(repository, repository+"-moved")

	require.NoError(t, err)
}

// repositoryWithWorkTree makes devDir/repo with one commit and its linked work tree devDir/repo-side on branch side.
func repositoryWithWorkTree(t *testing.T, devDir string) (string, string) {
	t.Helper()
	repository := filepath.Join(devDir, "repo")
	gitInit(t, repository)
	mustGit(t, "-C", repository, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com",
		"-c", "commit.gpgSign=false", "commit", "--allow-empty", "--message", "first")
	side := filepath.Join(devDir, "repo-side")
	mustGit(t, "-C", repository, "worktree", "add", "-b", "side", side)
	mustMkdir(t, filepath.Join(side, "cmd"))
	return repository, side
}

func requireSameDirectory(t *testing.T, dir string, want string) {
	t.Helper()
	same, err := isSameDirectory(dir, want)
	require.NoError(t, err)
	require.True(t, same, "got %q, want %q", dir, want)
}

func writeGitFile(t *testing.T, workTree string, adminDir string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(workTree, ".git"), []byte("gitdir: "+adminDir+"\n"), 0o600)
	require.NoError(t, err)
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, "init", dir)
}

func mustGit(t *testing.T, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %q\n%s", args, output)
	return string(output)
}

func guestRules(t *testing.T, devDir string, focusFile string) Rules {
	t.Helper()
	config := Config{FocusFile: focusFile, AgentDir: agentDevDir, OwnerDir: devDir}
	agentSID := parsedGuestsSID(t)
	rules := Rules{Config: config, AgentSID: agentSID, held: &heldGitDirs{handles: map[string]windows.Handle{}}}
	t.Cleanup(func() {
		err := rules.held.release()
		require.NoError(t, err)
	})
	return rules
}

func focusFileHolding(t *testing.T, agentPath string) string {
	t.Helper()
	focusFile := filepath.Join(t.TempDir(), ".herdr-focus")
	err := os.WriteFile(focusFile, []byte(agentPath), 0o600)
	require.NoError(t, err)
	return focusFile
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)
}
