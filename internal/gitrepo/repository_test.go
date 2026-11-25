package gitrepo

import (
	"context"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/stretchr/testify/require"
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

func TestRunGit(t *testing.T) {
	t.Run("failureCarriesWhatGitPrintedOnStdout", failureCarriesWhatGitPrintedOnStdout)
}

func TestPush(t *testing.T) {
	t.Run("sendsTheBranchToItsUpstream", sendsTheBranchToItsUpstream)
	t.Run("sendsTheBranchToItsOwnNameEvenWhenItTracksAnother", sendsTheBranchToItsOwnNameEvenWhenItTracksAnother)
}

func TestPublishSendsANewBranchToOriginAndTracksIt(t *testing.T) {
	_, clone := pushedClone(t)
	runSteps(t, clone, [][]string{
		{"-C", clone, "switch", "--create", "feature"},
		{"-C", clone, "commit", "--allow-empty", "--message", "One"},
		{"-C", clone, "commit", "--allow-empty", "--message", "Two"},
	})
	repository := Repository{Dir: clone}
	before, beforeErr := repository.CommitsOnNoRemote(context.Background())
	require.NoError(t, beforeErr)
	require.Equal(t, 2, before)

	err := repository.Publish(context.Background(), "feature")

	require.NoError(t, err)
	after, afterErr := repository.CommitsOnNoRemote(context.Background())
	require.NoError(t, afterErr)
	require.Equal(t, 0, after)
	workTreeStatus, statusErr := repository.Status(context.Background())
	require.NoError(t, statusErr)
	require.Equal(t, "origin/feature", workTreeStatus.Upstream)
	require.Equal(t, 0, workTreeStatus.Ahead)
}

func TestStatus(t *testing.T) {
	t.Run("leavesTheIndexUnwritten", leavesTheIndexUnwritten)
}

func leavesTheIndexUnwritten(t *testing.T) {
	clone := t.TempDir()
	file := filepath.Join(clone, "a.txt")
	writeFile(t, file, "a\n")
	runSteps(t, clone, [][]string{
		{"init", "--initial-branch=main", clone},
		{"-C", clone, "add", "a.txt"},
		{"-C", clone, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com", "-c", "commit.gpgsign=false",
			"commit", "--no-verify", "--message", "Start"},
	})
	hourAgo := time.Now().Add(-time.Hour)
	chtimesErr := os.Chtimes(file, hourAgo, hourAgo)
	require.NoError(t, chtimesErr)
	index := filepath.Join(clone, ".git", "index")
	before, beforeErr := os.Stat(index)
	require.NoError(t, beforeErr)

	_, err := Repository{Dir: clone}.Status(context.Background())

	require.NoError(t, err)
	after, afterErr := os.Stat(index)
	require.NoError(t, afterErr)
	modifiedBefore := before.ModTime()
	modifiedAfter := after.ModTime()
	require.Equal(t, modifiedBefore, modifiedAfter)
}

func sendsTheBranchToItsUpstream(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	clone := t.TempDir()
	runSteps(t, clone, [][]string{
		{"init", "--bare", "--initial-branch=main", remote},
		{"init", "--initial-branch=main", clone},
		{"-C", clone, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com", "-c", "commit.gpgsign=false",
			"commit", "--allow-empty", "--no-verify", "--message", "Start"},
		{"-C", clone, "remote", "add", "origin", remote},
		{"-C", clone, "config", "branch.main.remote", "origin"},
		{"-C", clone, "config", "branch.main.merge", "refs/heads/main"},
	})

	err := Repository{Dir: clone}.Push(context.Background(), "main")

	require.NoError(t, err)
	pushed := revParse(t, remote, "main")
	head := revParse(t, clone, "HEAD")
	require.Equal(t, head, pushed)
}

func TestStagedLineCounts(t *testing.T) {
	t.Run("countsOnlyTheStagedChanges", countsOnlyTheStagedChanges)
	t.Run("countsTheStagedLinesOnAnUnbornBranch", countsTheStagedLinesOnAnUnbornBranch)
}

func countsOnlyTheStagedChanges(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1\n2\n3"})
	writeFile(t, filepath.Join(clone, "b.txt"), "moved\nwhole\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "b.txt"}, {"-C", clone, "commit", "--message", "Add b"}})
	writeFile(t, filepath.Join(clone, "a.txt"), "1\ntwo\n3\n")
	writeFile(t, filepath.Join(clone, "staged.txt"), "s\n")
	writeFile(t, filepath.Join(clone, "binary.bin"), "\x00\x01\n\n")
	runSteps(t, clone, [][]string{
		{"-C", clone, "add", "a.txt", "staged.txt", "binary.bin"},
		{"-C", clone, "mv", "b.txt", "c.txt"},
	})
	writeFile(t, filepath.Join(clone, "a.txt"), "1\ntwo\n3\nunstaged\n")
	writeFile(t, filepath.Join(clone, "untracked.txt"), "u1\nu2\n")

	counts, err := Repository{Dir: clone}.StagedLineCounts(context.Background())

	require.NoError(t, err)
	want := diff.LineCounts{Added: 2, Removed: 1}
	require.Equal(t, want, counts)
}

func countsTheStagedLinesOnAnUnbornBranch(t *testing.T) {
	useTestIdentity(t)
	clone := t.TempDir()
	runSteps(t, clone, [][]string{{"init", "--initial-branch=main", clone}})
	writeFile(t, filepath.Join(clone, "staged.txt"), "a\nb\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "staged.txt"}})
	writeFile(t, filepath.Join(clone, "staged.txt"), "a\nb\nc\n")

	counts, err := Repository{Dir: clone}.StagedLineCounts(context.Background())

	require.NoError(t, err)
	want := diff.LineCounts{Added: 2, Removed: 0}
	require.Equal(t, want, counts)
}

func TestTheLargestContextGitParsesShowsTheWholeFile(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1\n2\n3\n4\n5\n6\n7\n8\n9", "1\n2\n3\n4\n5\n6\n7\n8\nnine"})
	writeFile(t, filepath.Join(clone, "a.txt"), "one\n2\n3\n4\n5\n6\n7\n8\nnine\n")
	repository := Repository{Dir: clone}
	file := FileStatus{Path: "a.txt", IndexStatus: '.', WorktreeStatus: 'M'}
	hash := revParse(t, clone, "HEAD")

	narrowFileDiff, narrowFileErr := repository.FileDiff(context.Background(), file, 1)
	narrowCommitDiff, narrowCommitErr := repository.CommitDiff(context.Background(), hash, 1)
	wholeFileDiff, wholeFileErr := repository.FileDiff(context.Background(), file, math.MaxInt32)
	wholeCommitDiff, wholeCommitErr := repository.CommitDiff(context.Background(), hash, math.MaxInt32)

	require.NoError(t, narrowFileErr)
	require.NoError(t, narrowCommitErr)
	require.NoError(t, wholeFileErr)
	require.NoError(t, wholeCommitErr)
	narrowFileHeaders := hunkHeaders(narrowFileDiff)
	narrowCommitHeaders := hunkHeaders(narrowCommitDiff)
	wholeFileHeaders := hunkHeaders(wholeFileDiff)
	wholeCommitHeaders := hunkHeaders(wholeCommitDiff)
	require.Equal(t, "@@ -1,2 +1,2 @@", narrowFileHeaders)
	require.Equal(t, "@@ -8,2 +8,2 @@", narrowCommitHeaders)
	require.Equal(t, "@@ -1,9 +1,9 @@", wholeFileHeaders)
	require.Equal(t, "@@ -1,9 +1,9 @@", wholeCommitHeaders)
}

func hunkHeaders(text string) string {
	headers := []string{}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			headers = append(headers, line)
		}
	}
	return strings.Join(headers, " ")
}

func TestHeadSubjectIsTheFirstLineOfHeadsMessage(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2"})

	subject, err := Repository{Dir: clone}.HeadSubject(context.Background())

	require.NoError(t, err)
	require.Equal(t, "Write 2", subject)
}

func TestDefaultBranch(t *testing.T) {
	t.Run("countsTheCommitsHeadLacksFromOriginsDefaultBranch", countsTheCommitsHeadLacksFromOriginsDefaultBranch)
	t.Run("isEmptyWithoutOriginHead", isEmptyWithoutOriginHead)
	t.Run("failsWhenOriginHeadPointsOutsideOrigin", failsWhenOriginHeadPointsOutsideOrigin)
}

func TestRebaseOntoDefault(t *testing.T) {
	t.Run("rebaseOntoDefaultMovesBranchOntoOriginsMain", rebaseOntoDefaultMovesBranchOntoOriginsMain)
	t.Run("rebaseOntoDefaultPrefersUpstreamOverOrigin", rebaseOntoDefaultPrefersUpstreamOverOrigin)
	t.Run("rebaseOntoDefaultAbortsAndKeepsHeadOnConflict", rebaseOntoDefaultAbortsAndKeepsHeadOnConflict)
}

func rebaseOntoDefaultMovesBranchOntoOriginsMain(t *testing.T) {
	remote, clone := featureBehindMain(t, "feature.txt")
	runSteps(t, clone, [][]string{{"remote", "set-head", "origin", "main"}})

	base, err := Repository{Dir: clone}.RebaseOntoDefault(context.Background())

	require.NoError(t, err)
	wantBase := RemoteBranch{Ref: "origin/main", Name: "main"}
	require.Equal(t, wantBase, base)
	remoteMain := revParse(t, remote, "main")
	headParent := revParse(t, clone, "HEAD^")
	require.Equal(t, remoteMain, headParent)
	headMessage := messageAt(t, clone, "HEAD")
	require.Equal(t, "Feature", headMessage)
}

func rebaseOntoDefaultPrefersUpstreamOverOrigin(t *testing.T) {
	remote, clone := featureBehindMain(t, "feature.txt")
	parent := t.TempDir()
	upstream := filepath.Join(parent, "upstream.git")
	upstreamClone := filepath.Join(parent, "upstream-work")
	runSteps(t, parent, [][]string{
		{"clone", "--bare", remote, upstream},
		{"clone", upstream, upstreamClone},
		{"-C", upstreamClone, "commit", "--allow-empty", "--message", "Upstream only"},
		{"-C", upstreamClone, "push"},
		{"-C", clone, "remote", "add", "upstream", upstream},
		{"-C", clone, "fetch", "upstream"},
		{"-C", clone, "remote", "set-head", "upstream", "main"},
	})

	base, err := Repository{Dir: clone}.RebaseOntoDefault(context.Background())

	require.NoError(t, err)
	wantBase := RemoteBranch{Ref: "upstream/main", Name: "main"}
	require.Equal(t, wantBase, base)
	upstreamMain := revParse(t, upstream, "main")
	headParent := revParse(t, clone, "HEAD^")
	require.Equal(t, upstreamMain, headParent)
}

func rebaseOntoDefaultAbortsAndKeepsHeadOnConflict(t *testing.T) {
	_, clone := featureBehindMain(t, "shared.txt")
	runSteps(t, clone, [][]string{{"remote", "set-head", "origin", "main"}})
	headBefore := revParse(t, clone, "HEAD")

	_, err := Repository{Dir: clone}.RebaseOntoDefault(context.Background())

	require.ErrorContains(t, err, "aborted the rebase onto origin/main")
	headAfter := revParse(t, clone, "HEAD")
	require.Equal(t, headBefore, headAfter)
	porcelainStatus := gitOutput(t, clone, "status", "--porcelain")
	require.Empty(t, porcelainStatus)
}

func failsWhenOriginHeadPointsOutsideOrigin(t *testing.T) {
	_, clone := pushedClone(t)
	runSteps(t, clone, [][]string{{"symbolic-ref", "refs/remotes/origin/HEAD", "refs/heads/main"}})

	_, err := Repository{Dir: clone}.DefaultBranch(context.Background())

	require.ErrorContains(t, err, "points to main, outside origin")
}

func countsTheCommitsHeadLacksFromOriginsDefaultBranch(t *testing.T) {
	remote, _ := pushedClone(t)
	parent := t.TempDir()
	clone := filepath.Join(parent, "work")
	otherClone := filepath.Join(parent, "other")
	runSteps(t, parent, [][]string{
		{"clone", remote, clone},
		{"clone", remote, otherClone},
		{"-C", otherClone, "commit", "--allow-empty", "--message", "One"},
		{"-C", otherClone, "commit", "--allow-empty", "--message", "Two"},
		{"-C", otherClone, "push"},
		{"-C", clone, "switch", "--create", "feature"},
		{"-C", clone, "fetch"},
	})
	repository := Repository{Dir: clone}

	branch, branchErr := repository.DefaultBranch(context.Background())
	behind, behindErr := repository.CommitsBehind(context.Background(), branch.Ref)

	require.NoError(t, branchErr)
	require.NoError(t, behindErr)
	wantBranch := RemoteBranch{Ref: "origin/main", Name: "main"}
	require.Equal(t, wantBranch, branch)
	require.Equal(t, 2, behind)
}

func isEmptyWithoutOriginHead(t *testing.T) {
	_, clone := pushedClone(t)

	branch, err := Repository{Dir: clone}.DefaultBranch(context.Background())

	require.NoError(t, err)
	require.Zero(t, branch)
}

func TestParseLog(t *testing.T) {
	t.Run("keepsASeparatorInsideTheMessage", keepsASeparatorInsideTheMessage)
	t.Run("rejectsARecordWithoutAFullHash", rejectsARecordWithoutAFullHash)
	t.Run("rejectsAParentThatIsNotAFullHash", rejectsAParentThatIsNotAFullHash)
}

func keepsASeparatorInsideTheMessage(t *testing.T) {
	hash := strings.Repeat("a", 40)
	parent := strings.Repeat("b", 40)
	output := []byte(hash + "\x1faaaaaaa\x1f" + parent + "\x1fHEAD -> master, tag: v1\x1fAda\x1f1700000000\x1fFix it\n\nbody \x1f text\n\x00")

	commits, err := parseLog(output)

	require.NoError(t, err)
	want := []Commit{{Hash: hash, ShortHash: "aaaaaaa", Parents: []string{parent}, Refs: []string{"HEAD -> master", "tag: v1"}, Author: "Ada", AuthoredAt: time.Unix(1700000000, 0),
		Summary: "Fix it", Message: "Fix it\n\nbody \x1f text"}}
	require.Equal(t, want, commits)
}

func rejectsARecordWithoutAFullHash(t *testing.T) {
	output := []byte("--output=x\x1fa\x1f\x1f\x1fAda\x1f1700000000\x1fFix\x00")

	_, err := parseLog(output)

	require.Error(t, err)
}

func rejectsAParentThatIsNotAFullHash(t *testing.T) {
	output := []byte(strings.Repeat("a", 40) + "\x1faaaaaaa\x1f--all\x1f\x1fAda\x1f1700000000\x1fFix\x00")

	_, err := parseLog(output)

	require.Error(t, err)
}

func failureCarriesWhatGitPrintedOnStdout(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old.txt"), "old line\n")
	writeFile(t, filepath.Join(dir, "new.txt"), "new line\n")

	_, err := Repository{Dir: dir}.git(context.Background(), "diff", "--no-index", "--exit-code", "--", "old.txt",
		"new.txt")

	require.ErrorContains(t, err, "+new line")
}

func runSteps(t *testing.T, dir string, steps [][]string) {
	t.Helper()
	repository := Repository{Dir: dir}
	for _, step := range steps {
		_, err := repository.git(context.Background(), step...)
		require.NoError(t, err)
	}
}

func TestDiscard(t *testing.T) {
	t.Run("discardsOnlyTheGivenFiles", discardsOnlyTheGivenFiles)
	t.Run("discardAllKeepsIgnoredFiles", discardAllKeepsIgnoredFiles)
}

func discardsOnlyTheGivenFiles(t *testing.T) {
	clone := changedWorkTree(t)
	repository := Repository{Dir: clone}
	workTreeStatus, statusErr := repository.Status(context.Background())
	require.NoError(t, statusErr)

	for _, file := range workTreeStatus.Files {
		if file.Path == "kept.txt" {
			continue
		}
		err := repository.Discard(context.Background(), file)
		require.NoError(t, err)
	}

	after, afterErr := repository.Status(context.Background())
	require.NoError(t, afterErr)
	require.Len(t, after.Files, 1)
	require.Equal(t, "kept.txt", after.Files[0].Path)
	aFile := filepath.Join(clone, "a.txt")
	assertFileHolds(t, aFile, "1\n")
}

func discardAllKeepsIgnoredFiles(t *testing.T) {
	clone := changedWorkTree(t)
	repository := Repository{Dir: clone}

	err := repository.DiscardAll(context.Background())

	require.NoError(t, err)
	after, statusErr := repository.Status(context.Background())
	require.NoError(t, statusErr)
	require.Empty(t, after.Files)
	ignoredLog := filepath.Join(clone, "build.log")
	require.FileExists(t, ignoredLog)
	_, untrackedDirErr := os.Stat(filepath.Join(clone, "dir"))
	require.ErrorIs(t, untrackedDirErr, fs.ErrNotExist)
}

func TestOnAnUnbornBranch(t *testing.T) {
	t.Run("diffsAFileAgainstNothing", diffsAFileAgainstNothing)
	t.Run("unstagesAFile", unstagesAFile)
	t.Run("unstagesEverything", unstagesEverything)
	t.Run("discardsAFile", discardsAFile)
	t.Run("discardsEverything", discardsEverything)
}

func diffsAFileAgainstNothing(t *testing.T) {
	repository, file := unbornWithStagedFile(t)

	text, err := repository.FileDiff(context.Background(), file, 3)

	require.NoError(t, err)
	headers := hunkHeaders(text)
	require.Equal(t, "@@ -0,0 +1,2 @@", headers)
}

func unstagesAFile(t *testing.T) {
	repository, file := unbornWithStagedFile(t)

	err := repository.Unstage(context.Background(), file)

	require.NoError(t, err)
	assertOnlyUntracked(t, repository, "staged.txt")
}

func unstagesEverything(t *testing.T) {
	repository, _ := unbornWithStagedFile(t)

	err := repository.UnstageAll(context.Background())

	require.NoError(t, err)
	assertOnlyUntracked(t, repository, "staged.txt")
}

func discardsAFile(t *testing.T) {
	repository, file := unbornWithStagedFile(t)
	writeFile(t, filepath.Join(repository.Dir, "kept.txt"), "kept\n")

	err := repository.Discard(context.Background(), file)

	require.NoError(t, err)
	assertOnlyUntracked(t, repository, "kept.txt")
}

func discardsEverything(t *testing.T) {
	repository, _ := unbornWithStagedFile(t)
	writeFile(t, filepath.Join(repository.Dir, "untracked.txt"), "u\n")

	err := repository.DiscardAll(context.Background())

	require.NoError(t, err)
	workTreeStatus, statusErr := repository.Status(context.Background())
	require.NoError(t, statusErr)
	require.Empty(t, workTreeStatus.Files)
}

// unbornWithStagedFile makes a repository with no commits whose staged.txt is staged and then changed again.
func unbornWithStagedFile(t *testing.T) (Repository, FileStatus) {
	t.Helper()
	clone := t.TempDir()
	runSteps(t, clone, [][]string{{"init", "--initial-branch=main", clone}})
	writeFile(t, filepath.Join(clone, "staged.txt"), "a\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "staged.txt"}})
	writeFile(t, filepath.Join(clone, "staged.txt"), "a\nb\n")
	return Repository{Dir: clone}, FileStatus{Path: "staged.txt", IndexStatus: 'A', WorktreeStatus: 'M'}
}

func assertOnlyUntracked(t *testing.T, repository Repository, path string) {
	t.Helper()
	workTreeStatus, err := repository.Status(context.Background())
	require.NoError(t, err)
	require.Len(t, workTreeStatus.Files, 1)
	require.Equal(t, path, workTreeStatus.Files[0].Path)
	untracked := workTreeStatus.Files[0].IsUntracked()
	require.True(t, untracked)
}

func changedWorkTree(t *testing.T) string {
	t.Helper()
	clone := repositoryWithHistory(t, []string{"1"})
	writeFile(t, filepath.Join(clone, "b.txt"), "b\n")
	writeFile(t, filepath.Join(clone, ".gitignore"), "*.log\n")
	runSteps(t, clone, [][]string{
		{"-C", clone, "add", "b.txt", ".gitignore"}, {"-C", clone, "commit", "--message", "Add b"}})
	writeFile(t, filepath.Join(clone, "a.txt"), "changed\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "a.txt"}, {"-C", clone, "mv", "b.txt", "c.txt"}})
	mkdirErr := os.Mkdir(filepath.Join(clone, "dir"), 0o700)
	require.NoError(t, mkdirErr)
	writeFile(t, filepath.Join(clone, "dir", "new.txt"), "new\n")
	writeFile(t, filepath.Join(clone, "kept.txt"), "kept\n")
	writeFile(t, filepath.Join(clone, "build.log"), "ignored\n")
	return clone
}

func TestPull(t *testing.T) {
	t.Run("fastForwardsToTheUpstream", fastForwardsToTheUpstream)
	t.Run("refusesADivergedBranch", refusesADivergedBranch)
}

func fastForwardsToTheUpstream(t *testing.T) {
	clone, otherClone := cloneBehindOther(t)

	err := Repository{Dir: clone}.Pull(context.Background())

	require.NoError(t, err)
	head := revParse(t, clone, "HEAD")
	pushed := revParse(t, otherClone, "HEAD")
	require.Equal(t, pushed, head)
}

func refusesADivergedBranch(t *testing.T) {
	clone, _ := cloneBehindOther(t)
	runSteps(t, clone, [][]string{{"-C", clone, "commit", "--allow-empty", "--message", "Local"}})
	before := revParse(t, clone, "HEAD")

	err := Repository{Dir: clone}.Pull(context.Background())

	require.Error(t, err)
	after := revParse(t, clone, "HEAD")
	require.Equal(t, before, after)
}

func TestLogAheadAndBehindListTheCommitsOnEachSide(t *testing.T) {
	clone, _ := cloneBehindOther(t)
	runSteps(t, clone, [][]string{
		{"-C", clone, "commit", "--allow-empty", "--message", "Local"},
		{"-C", clone, "fetch"},
	})
	repository := Repository{Dir: clone}

	ahead, aheadErr := repository.LogAhead(context.Background(), "origin/main", 10)
	behind, behindErr := repository.LogBehind(context.Background(), "origin/main", 10)

	require.NoError(t, aheadErr)
	require.NoError(t, behindErr)
	require.Len(t, ahead, 1)
	require.Equal(t, "Local", ahead[0].Summary)
	require.Len(t, behind, 1)
	require.Equal(t, "Pushed", behind[0].Summary)
}

func TestLogOnNoRemoteListsTheCommitsNoRemoteHolds(t *testing.T) {
	_, clone, _ := featureTrackingMain(t)

	commits, err := Repository{Dir: clone}.LogOnNoRemote(context.Background(), 10)

	require.NoError(t, err)
	require.Len(t, commits, 1)
	require.Equal(t, "Feature", commits[0].Summary)
}

func cloneBehindOther(t *testing.T) (string, string) {
	t.Helper()
	remote, clone := pushedClone(t)
	otherClone := filepath.Join(t.TempDir(), "other")
	runSteps(t, clone, [][]string{
		{"clone", remote, otherClone},
		{"-C", otherClone, "commit", "--allow-empty", "--message", "Pushed"},
		{"-C", otherClone, "push"},
	})
	return clone, otherClone
}

func TestFetchBringsTheRemoteCommitsAndPrunesItsDeletedBranches(t *testing.T) {
	remote, clone := pushedClone(t)
	otherClone := filepath.Join(t.TempDir(), "other")
	runSteps(t, clone, [][]string{
		{"-C", clone, "push", "origin", "main:gone"},
		{"clone", remote, otherClone},
		{"-C", otherClone, "commit", "--allow-empty", "--message", "Pushed"},
		{"-C", otherClone, "push"},
		{"-C", otherClone, "push", "origin", "--delete", "gone"},
	})
	repository := Repository{Dir: clone}

	err := repository.Fetch(context.Background())

	require.NoError(t, err)
	behind, behindErr := repository.CommitsBehind(context.Background(), "origin/main")
	require.NoError(t, behindErr)
	require.Equal(t, 1, behind)
	_, goneErr := repository.revParse(context.Background(), "origin/gone")
	require.Error(t, goneErr)
}

func TestABranchMergedAndDeletedOnTheRemote(t *testing.T) {
	t.Run("hasItsUpstreamGone", hasItsUpstreamGone)
	t.Run("isMergedIntoTheDefaultBranchUntilItChangesAFile", isMergedIntoTheDefaultBranchUntilItChangesAFile)
	t.Run("switchesToTheDefaultBranchCreatedFromOriginsSoPullFastForwardsIt",
		switchesToTheDefaultBranchCreatedFromOriginsSoPullFastForwardsIt)
}

func hasItsUpstreamGone(t *testing.T) {
	clone, _ := mergedAndDeletedOnTheRemote(t)

	workTreeStatus, err := Repository{Dir: clone}.Status(context.Background())

	require.NoError(t, err)
	require.Equal(t, "origin/feature", workTreeStatus.Upstream)
	require.True(t, workTreeStatus.UpstreamGone)
}

func isMergedIntoTheDefaultBranchUntilItChangesAFile(t *testing.T) {
	clone, _ := mergedAndDeletedOnTheRemote(t)
	repository := Repository{Dir: clone}

	merged, mergedErr := repository.MergedInto(context.Background(), "HEAD", "origin/main")
	writeFile(t, filepath.Join(clone, "local.txt"), "local\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "local.txt"}, {"-C", clone, "commit", "--message", "Local"}})
	mergedAfter, mergedAfterErr := repository.MergedInto(context.Background(), "HEAD", "origin/main")

	require.NoError(t, mergedErr)
	require.True(t, merged)
	require.NoError(t, mergedAfterErr)
	require.False(t, mergedAfter)
}

func switchesToTheDefaultBranchCreatedFromOriginsSoPullFastForwardsIt(t *testing.T) {
	clone, otherClone := mergedAndDeletedOnTheRemote(t)
	repository := Repository{Dir: clone}

	switchErr := repository.Switch(context.Background(), "main")
	pullErr := repository.Pull(context.Background())

	require.NoError(t, switchErr)
	require.NoError(t, pullErr)
	workTreeStatus, statusErr := repository.Status(context.Background())
	require.NoError(t, statusErr)
	require.Equal(t, "main", workTreeStatus.Branch)
	head := revParse(t, clone, "HEAD")
	otherHead := revParse(t, otherClone, "HEAD")
	require.Equal(t, otherHead, head)
}

func mergedAndDeletedOnTheRemote(t *testing.T) (string, string) {
	t.Helper()
	remote, clone := pushedClone(t)
	otherClone := filepath.Join(t.TempDir(), "other")
	runSteps(t, clone, [][]string{
		{"-C", clone, "switch", "--create", "feature"},
		{"-C", clone, "commit", "--allow-empty", "--message", "Feature"},
		{"-C", clone, "push", "--set-upstream", "origin", "feature"},
		{"-C", clone, "push", "origin", "feature:main"},
		{"-C", clone, "push", "origin", "--delete", "feature"},
		{"-C", clone, "branch", "--delete", "--force", "main"},
		{"clone", remote, otherClone},
		{"-C", otherClone, "commit", "--allow-empty", "--message", "After"},
		{"-C", otherClone, "push"},
	})
	return clone, otherClone
}

func TestCherryPickBranch(t *testing.T) {
	t.Run("cherryPickBranchPutsTheWorkTreesCommitsOnMain", cherryPickBranchPutsTheWorkTreesCommitsOnMain)
	t.Run("cherryPickBranchSkipsCommitsAlreadyPicked", cherryPickBranchSkipsCommitsAlreadyPicked)
	t.Run("cherryPickBranchAbortsAndKeepsMainOnConflict", cherryPickBranchAbortsAndKeepsMainOnConflict)
	t.Run("cherryPickBranchRefusesAnotherBranchCheckedOut", cherryPickBranchRefusesAnotherBranchCheckedOut)
}

func cherryPickBranchPutsTheWorkTreesCommitsOnMain(t *testing.T) {
	clone, workTree := cloneWithFeatureWorkTree(t)
	mainBefore := revParse(t, clone, "main")
	featureBefore := revParse(t, workTree, "HEAD")

	picked, err := Repository{Dir: clone}.CherryPickBranch(context.Background(), "feature")

	require.NoError(t, err)
	require.Equal(t, 2, picked)
	mainBase := revParse(t, clone, "main~2")
	require.Equal(t, mainBefore, mainBase)
	mainMessage := messageAt(t, clone, "main")
	require.Equal(t, "Feature two", mainMessage)
	featureAfter := revParse(t, workTree, "HEAD")
	require.Equal(t, featureBefore, featureAfter)
}

func cherryPickBranchSkipsCommitsAlreadyPicked(t *testing.T) {
	clone, workTree := cloneWithFeatureWorkTree(t)
	repository := Repository{Dir: clone}
	_, firstErr := repository.CherryPickBranch(context.Background(), "feature")
	require.NoError(t, firstErr)
	writeFile(t, filepath.Join(workTree, "feature.txt"), "three\n")
	runSteps(t, workTree, [][]string{{"commit", "--all", "--message", "Feature three"}})

	picked, err := repository.CherryPickBranch(context.Background(), "feature")
	_, againErr := repository.CherryPickBranch(context.Background(), "feature")

	require.NoError(t, err)
	require.Equal(t, 1, picked)
	mainMessage := messageAt(t, clone, "main")
	require.Equal(t, "Feature three", mainMessage)
	require.ErrorContains(t, againErr, "already has every commit")
}

func cherryPickBranchAbortsAndKeepsMainOnConflict(t *testing.T) {
	clone, _ := cloneWithFeatureWorkTree(t)
	writeFile(t, filepath.Join(clone, "feature.txt"), "main's own\n")
	runSteps(t, clone, [][]string{{"add", "feature.txt"}, {"commit", "--message", "Main's own"}})
	mainBefore := revParse(t, clone, "main")

	_, err := Repository{Dir: clone}.CherryPickBranch(context.Background(), "feature")

	require.ErrorContains(t, err, "aborted the cherry-pick of feature onto main")
	mainAfter := revParse(t, clone, "main")
	require.Equal(t, mainBefore, mainAfter)
	porcelainStatus := gitOutput(t, clone, "status", "--porcelain")
	require.Empty(t, porcelainStatus)
}

func cherryPickBranchRefusesAnotherBranchCheckedOut(t *testing.T) {
	clone, _ := cloneWithFeatureWorkTree(t)
	runSteps(t, clone, [][]string{{"switch", "--create", "other"}})

	_, err := Repository{Dir: clone}.CherryPickBranch(context.Background(), "feature")

	require.ErrorContains(t, err, `has "other" checked out, switch it to main first`)
}

// cloneWithFeatureWorkTree returns a clone on main, with origin/HEAD set, and its linked work tree on feature with two
// commits main lacks.
func cloneWithFeatureWorkTree(t *testing.T) (string, string) {
	t.Helper()
	_, clone := pushedClone(t)
	workTree := filepath.Join(t.TempDir(), "feature")
	runSteps(t, clone, [][]string{
		{"remote", "set-head", "origin", "main"},
		{"worktree", "add", "-b", "feature", workTree},
	})
	writeFile(t, filepath.Join(workTree, "feature.txt"), "one\n")
	runSteps(t, workTree, [][]string{{"add", "feature.txt"}, {"commit", "--message", "Feature one"}})
	writeFile(t, filepath.Join(workTree, "feature.txt"), "two\n")
	runSteps(t, workTree, [][]string{{"commit", "--all", "--message", "Feature two"}})
	return clone, workTree
}

func TestMergedIntoCountsASquashMergeWhoseContextChangedFirst(t *testing.T) {
	remote, clone := pushedClone(t)
	otherClone := filepath.Join(t.TempDir(), "other")
	lines := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	withLine := func(index int, text string) string {
		changed := slices.Clone(lines)
		changed[index] = text
		return strings.Join(changed, "\n") + "\n"
	}
	writeFile(t, filepath.Join(clone, "a.txt"), withLine(0, "1"))
	runSteps(t, clone, [][]string{
		{"-C", clone, "add", "a.txt"},
		{"-C", clone, "commit", "--message", "Add a"},
		{"-C", clone, "push"},
		{"-C", clone, "switch", "--create", "feature"},
	})
	writeFile(t, filepath.Join(clone, "a.txt"), withLine(8, "nine"))
	runSteps(t, clone, [][]string{{"-C", clone, "commit", "--all", "--message", "Nine"}})
	writeFile(t, filepath.Join(clone, "a.txt"), withLine(8, "NINE"))
	runSteps(t, clone, [][]string{
		{"-C", clone, "commit", "--all", "--message", "Louder"},
		{"-C", clone, "push", "--set-upstream", "origin", "feature"},
		{"clone", remote, otherClone},
	})
	writeFile(t, filepath.Join(otherClone, "a.txt"), withLine(6, "seven"))
	runSteps(t, otherClone, [][]string{
		{"-C", otherClone, "commit", "--all", "--message", "Context"},
		{"-C", otherClone, "merge", "--squash", "origin/feature"},
		{"-C", otherClone, "commit", "--message", "Squashed"},
		{"-C", otherClone, "push"},
		{"-C", clone, "fetch"},
	})

	merged, err := Repository{Dir: clone}.MergedInto(context.Background(), "HEAD", "origin/main")

	require.NoError(t, err)
	require.True(t, merged)
}

func TestForcePush(t *testing.T) {
	t.Run("replacesTheUpstreamCommitThisBranchRewrote", replacesTheUpstreamCommitThisBranchRewrote)
	t.Run("refusesWhenTheRemoteMovedSinceTheLastFetch", refusesWhenTheRemoteMovedSinceTheLastFetch)
}

func TestAmend(t *testing.T) {
	t.Run("foldsTheStagedChangesIntoHead", foldsTheStagedChangesIntoHead)
	t.Run("foldsTheStagedChangesIntoAnOlderCommitAndKeepsUnstagedOnes",
		foldsTheStagedChangesIntoAnOlderCommitAndKeepsUnstagedOnes)
	t.Run("undoesTheFixupWhenTheRebaseStops", undoesTheFixupWhenTheRebaseStops)
	t.Run("refusesACommitWithAMergeAboveIt", refusesACommitWithAMergeAboveIt)
}

func TestReword(t *testing.T) {
	t.Run("rewordsHeadAndLeavesTheStagedChangesStaged", rewordsHeadAndLeavesTheStagedChangesStaged)
	t.Run("rewordsAnOlderCommitAndKeepsTheTreesTheAuthorsAndTheIndex",
		rewordsAnOlderCommitAndKeepsTheTreesTheAuthorsAndTheIndex)
	t.Run("signsTheRewrittenCommitsWhenCommitGpgSignIsOn", signsTheRewrittenCommitsWhenCommitGpgSignIsOn)
}

func rewordsHeadAndLeavesTheStagedChangesStaged(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2"})
	parent := revParse(t, clone, "HEAD~1")
	before := identitiesAndTrees(t, clone)
	writeFile(t, filepath.Join(clone, "b.txt"), "staged\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "b.txt"}})
	head := revParse(t, clone, "HEAD")

	err := Repository{Dir: clone}.Reword(context.Background(), head, "Reworded\n\nWith a body")

	require.NoError(t, err)
	headMessage := messageAt(t, clone, "HEAD")
	require.Equal(t, "Reworded\n\nWith a body", headMessage)
	parentAfter := revParse(t, clone, "HEAD~1")
	require.Equal(t, parent, parentAfter)
	after := identitiesAndTrees(t, clone)
	require.Equal(t, before, after)
	staged := stagedNames(t, clone)
	require.Equal(t, "b.txt", staged)
}

func rewordsAnOlderCommitAndKeepsTheTreesTheAuthorsAndTheIndex(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2", "3"})
	before := identitiesAndTrees(t, clone)
	writeFile(t, filepath.Join(clone, "b.txt"), "staged\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "b.txt"}})
	writeFile(t, filepath.Join(clone, "a.txt"), "unstaged\n")
	target := revParse(t, clone, "HEAD~1")

	err := Repository{Dir: clone}.Reword(context.Background(), target, "Reworded")

	require.NoError(t, err)
	targetMessage := messageAt(t, clone, "HEAD~1")
	require.Equal(t, "Reworded", targetMessage)
	headMessage := messageAt(t, clone, "HEAD")
	require.Equal(t, "Write 3", headMessage)
	after := identitiesAndTrees(t, clone)
	require.Equal(t, before, after)
	staged := stagedNames(t, clone)
	require.Equal(t, "b.txt", staged)
	aFile := filepath.Join(clone, "a.txt")
	assertFileHolds(t, aFile, "unstaged\n")
}

func signsTheRewrittenCommitsWhenCommitGpgSignIsOn(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2", "3"})
	head := revParse(t, clone, "HEAD")
	target := revParse(t, clone, "HEAD~1")
	t.Setenv("GIT_CONFIG_VALUE_0", "true")
	t.Setenv("GIT_CONFIG_COUNT", "4")
	t.Setenv("GIT_CONFIG_KEY_3", "gpg.program")
	t.Setenv("GIT_CONFIG_VALUE_3", "perusal-missing-gpg")

	err := Repository{Dir: clone}.Reword(context.Background(), target, "Reworded")

	require.Error(t, err)
	headAfter := revParse(t, clone, "HEAD")
	require.Equal(t, head, headAfter)
}

func replacesTheUpstreamCommitThisBranchRewrote(t *testing.T) {
	remote, clone := pushedClone(t)
	runSteps(t, clone, [][]string{{"-C", clone, "commit", "--amend", "--allow-empty", "--message", "Rewritten"}})

	err := Repository{Dir: clone}.ForcePush(context.Background(), "main")

	require.NoError(t, err)
	remoteMain := revParse(t, remote, "main")
	head := revParse(t, clone, "HEAD")
	require.Equal(t, head, remoteMain)
}

func refusesWhenTheRemoteMovedSinceTheLastFetch(t *testing.T) {
	remote, clone := pushedClone(t)
	otherClone := filepath.Join(t.TempDir(), "other")
	runSteps(t, clone, [][]string{
		{"clone", remote, otherClone},
		{"-C", otherClone, "commit", "--allow-empty", "--message", "Someone else"},
		{"-C", otherClone, "push"},
		{"-C", clone, "commit", "--amend", "--allow-empty", "--message", "Rewritten"},
	})
	before := revParse(t, remote, "main")

	err := Repository{Dir: clone}.ForcePush(context.Background(), "main")

	require.Error(t, err)
	after := revParse(t, remote, "main")
	require.Equal(t, before, after)
}

func foldsTheStagedChangesIntoHead(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2"})
	parent := revParse(t, clone, "HEAD~1")
	writeFile(t, filepath.Join(clone, "a.txt"), "3\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "a.txt"}})
	head := revParse(t, clone, "HEAD")

	err := Repository{Dir: clone}.Amend(context.Background(), head)

	require.NoError(t, err)
	amended := blobAt(t, clone, "HEAD:a.txt")
	require.Equal(t, "3\n", amended)
	parentAfter := revParse(t, clone, "HEAD~1")
	require.Equal(t, parent, parentAfter)
}

func foldsTheStagedChangesIntoAnOlderCommitAndKeepsUnstagedOnes(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2", "3"})
	writeFile(t, filepath.Join(clone, "b.txt"), "staged\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "b.txt"}})
	writeFile(t, filepath.Join(clone, "a.txt"), "unstaged\n")
	target := revParse(t, clone, "HEAD~1")

	err := Repository{Dir: clone}.Amend(context.Background(), target)

	require.NoError(t, err)
	countOutput := gitOutput(t, clone, "rev-list", "--count", "HEAD")
	count := strings.TrimSpace(countOutput)
	require.Equal(t, "3", count)
	amended := blobAt(t, clone, "HEAD~1:b.txt")
	require.Equal(t, "staged\n", amended)
	aFile := filepath.Join(clone, "a.txt")
	assertFileHolds(t, aFile, "unstaged\n")
}

func undoesTheFixupWhenTheRebaseStops(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2", "3"})
	head := revParse(t, clone, "HEAD")
	writeFile(t, filepath.Join(clone, "a.txt"), "4\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "a.txt"}})
	target := revParse(t, clone, "HEAD~1")

	err := Repository{Dir: clone}.Amend(context.Background(), target)

	require.Error(t, err)
	headAfter := revParse(t, clone, "HEAD")
	require.Equal(t, head, headAfter)
	staged := stagedNames(t, clone)
	require.Equal(t, "a.txt", staged)
	_, statErr := os.Stat(filepath.Join(clone, ".git", "rebase-merge"))
	require.ErrorIs(t, statErr, fs.ErrNotExist)
}

func refusesACommitWithAMergeAboveIt(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1"})
	target := revParse(t, clone, "HEAD")
	runSteps(t, clone, [][]string{
		{"-C", clone, "branch", "side"},
		{"-C", clone, "commit", "--allow-empty", "--message", "Main"},
		{"-C", clone, "merge", "--no-ff", "--message", "Merge side", "side"},
	})
	head := revParse(t, clone, "HEAD")
	writeFile(t, filepath.Join(clone, "b.txt"), "staged\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "b.txt"}})

	err := Repository{Dir: clone}.Amend(context.Background(), target)

	require.ErrorIs(t, err, ErrCannotRewrite)
	headAfter := revParse(t, clone, "HEAD")
	require.Equal(t, head, headAfter)
}

func repositoryWithHistory(t *testing.T, contents []string) string {
	t.Helper()
	useTestIdentity(t)
	clone := t.TempDir()
	runSteps(t, clone, [][]string{{"init", "--initial-branch=main", clone}})
	for _, content := range contents {
		writeFile(t, filepath.Join(clone, "a.txt"), content+"\n")
		runSteps(t, clone, [][]string{
			{"-C", clone, "add", "a.txt"}, {"-C", clone, "commit", "--message", "Write " + content}})
	}
	return clone
}

// featureBehindMain is a clone on branch feature, whose one commit writes featurePath, while another clone has pushed
// a commit writing shared.txt to origin's main, so a featurePath of shared.txt conflicts.
func featureBehindMain(t *testing.T, featurePath string) (string, string) {
	t.Helper()
	remote, _ := pushedClone(t)
	parent := t.TempDir()
	clone := filepath.Join(parent, "work")
	otherClone := filepath.Join(parent, "other")
	runSteps(t, parent, [][]string{
		{"clone", remote, clone},
		{"clone", remote, otherClone},
	})
	writeFile(t, filepath.Join(otherClone, "shared.txt"), "main\n")
	writeFile(t, filepath.Join(clone, featurePath), "feature\n")
	runSteps(t, parent, [][]string{
		{"-C", otherClone, "add", "shared.txt"},
		{"-C", otherClone, "commit", "--message", "Main"},
		{"-C", otherClone, "push"},
		{"-C", clone, "switch", "--create", "feature"},
		{"-C", clone, "add", featurePath},
		{"-C", clone, "commit", "--message", "Feature"},
	})
	return remote, clone
}

func pushedClone(t *testing.T) (string, string) {
	t.Helper()
	useTestIdentity(t)
	parent := t.TempDir()
	remote := filepath.Join(parent, "remote.git")
	clone := filepath.Join(parent, "work")
	runSteps(t, parent, [][]string{
		{"init", "--bare", "--initial-branch=main", remote},
		{"clone", remote, clone},
		{"-C", clone, "commit", "--allow-empty", "--message", "Start"},
		{"-C", clone, "push", "--set-upstream", "origin", "main"},
	})
	return remote, clone
}

// Amend runs git commit itself, so the identity and config go in the environment rather than -c flags.
func useTestIdentity(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"GIT_AUTHOR_NAME": "perusal", "GIT_AUTHOR_EMAIL": "perusal@example.com",
		"GIT_COMMITTER_NAME": "perusal", "GIT_COMMITTER_EMAIL": "perusal@example.com",
		"GIT_CONFIG_COUNT": "3", "GIT_CONFIG_KEY_0": "commit.gpgsign", "GIT_CONFIG_VALUE_0": "false",
		"GIT_CONFIG_KEY_1": "core.hooksPath", "GIT_CONFIG_VALUE_1": t.TempDir(),
		"GIT_CONFIG_KEY_2": "core.autocrlf", "GIT_CONFIG_VALUE_2": "false",
	} {
		t.Setenv(name, value)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0o600)
	require.NoError(t, err)
}

func revParse(t *testing.T, dir string, revision string) string {
	t.Helper()
	hash, err := Repository{Dir: dir}.revParse(context.Background(), revision)
	require.NoError(t, err)
	return hash
}

func messageAt(t *testing.T, dir string, revision string) string {
	t.Helper()
	message := gitOutput(t, dir, "log", "--max-count=1", "--format=%B", revision)
	return strings.TrimSpace(message)
}

func stagedNames(t *testing.T, dir string) string {
	t.Helper()
	names := gitOutput(t, dir, "diff", "--cached", "--name-only")
	return strings.TrimSpace(names)
}

func identitiesAndTrees(t *testing.T, dir string) string {
	t.Helper()
	return gitOutput(t, dir, "log", "--format=%T %an %ae %ad", "--date=raw")
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := Repository{Dir: dir}.git(context.Background(), args...)
	require.NoError(t, err)
	return string(output)
}

func blobAt(t *testing.T, dir string, object string) string {
	t.Helper()
	content, err := Repository{Dir: dir}.git(context.Background(), "show", object)
	require.NoError(t, err)
	return string(content)
}

func assertFileHolds(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(content))
}

func sendsTheBranchToItsOwnNameEvenWhenItTracksAnother(t *testing.T) {
	remote, clone, main := featureTrackingMain(t)

	err := Repository{Dir: clone}.Push(context.Background(), "feature")

	require.NoError(t, err)
	assertFeaturePushedAndMainKept(t, remote, clone, main)
}

func TestPublishSendsABranchTrackingAnotherNameToItsOwn(t *testing.T) {
	remote, clone, main := featureTrackingMain(t)

	err := Repository{Dir: clone}.Publish(context.Background(), "feature")

	require.NoError(t, err)
	assertFeaturePushedAndMainKept(t, remote, clone, main)
}

// push.default=upstream maps a refspec with no destination onto the branch it tracks.
func featureTrackingMain(t *testing.T) (string, string, string) {
	t.Helper()
	remote, clone := pushedClone(t)
	runSteps(t, clone, [][]string{
		{"-C", clone, "switch", "--create", "feature", "--track", "origin/main"},
		{"-C", clone, "commit", "--allow-empty", "--message", "Feature"},
		{"-C", clone, "config", "push.default", "upstream"},
	})
	return remote, clone, revParse(t, remote, "main")
}

func assertFeaturePushedAndMainKept(t *testing.T, remote string, clone string, main string) {
	t.Helper()
	remoteMain := revParse(t, remote, "main")
	require.Equal(t, main, remoteMain)
	remoteFeature := revParse(t, remote, "feature")
	head := revParse(t, clone, "HEAD")
	require.Equal(t, head, remoteFeature)
}

func TestGitDirKeepsGitOffARewrittenDotGitFile(t *testing.T) {
	_, clone := pushedClone(t)
	side := filepath.Join(t.TempDir(), "side")
	runSteps(t, clone, [][]string{{"worktree", "add", "-b", "side", side}})
	writeFile(t, filepath.Join(side, ".git"), "gitdir: "+filepath.Join(t.TempDir(), "elsewhere")+"\n")
	gitDir := filepath.Join(clone, ".git", "worktrees", "side")
	repository, openErr := Open(context.Background(), Repository{Dir: side, GitDir: gitDir})
	require.NoError(t, openErr)

	workTreeStatus, err := repository.Status(context.Background())

	require.NoError(t, err)
	require.Equal(t, "side", workTreeStatus.Branch)
}

func TestFilesDiffParsesAsAModifiedFileForTwoFilesInDifferentDirsOutsideARepository(t *testing.T) {
	oldPath := filepath.Join(t.TempDir(), "current.cs")
	newPath := filepath.Join(t.TempDir(), "current.cs")
	writeFile(t, oldPath, "a\nb\nc\n")
	writeFile(t, newPath, "a\nB\nc\n")

	text, err := FilesDiff(context.Background(), oldPath, newPath)

	require.NoError(t, err)
	fileDiff, parseErr := diff.ParseFile(text)
	require.NoError(t, parseErr)
	require.Equal(t, diff.FileModified, fileDiff.Change)
	counts := diff.CountLines(fileDiff)
	want := diff.LineCounts{Added: 1, Removed: 1}
	require.Equal(t, want, counts)
}
