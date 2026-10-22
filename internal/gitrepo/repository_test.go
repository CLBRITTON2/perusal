package gitrepo

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

func TestRunGit(t *testing.T) {
	t.Run("failureCarriesWhatGitPrintedOnStdout", failureCarriesWhatGitPrintedOnStdout)
}

func TestPush(t *testing.T) {
	t.Run("sendsTheBranchToItsUpstream", sendsTheBranchToItsUpstream)
}

func TestStatus(t *testing.T) {
	t.Run("leavesTheIndexUnwritten", leavesTheIndexUnwritten)
}

func leavesTheIndexUnwritten(t *testing.T) {
	clone := t.TempDir()
	file := filepath.Join(clone, "a.txt")
	if err := os.WriteFile(file, []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runSteps(t, clone, [][]string{
		{"init", "--initial-branch=main", clone},
		{"-C", clone, "add", "a.txt"},
		{"-C", clone, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com", "-c", "commit.gpgsign=false",
			"commit", "--no-verify", "--message", "Start"},
	})
	hourAgo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(file, hourAgo, hourAgo); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(clone, ".git", "index")
	before, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}

	_, statusErr := Repository{Dir: clone}.Status(context.Background())

	after, err := os.Stat(index)
	if statusErr != nil || err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("got status error %v, index modified %v then %v (%v)", statusErr, before.ModTime(), after.ModTime(), err)
	}
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

	err := Repository{Dir: clone}.Push(context.Background())

	pushed, revParseErr := Repository{Dir: remote}.git(context.Background(), "rev-parse", "main")
	head, headErr := Repository{Dir: clone}.git(context.Background(), "rev-parse", "HEAD")
	if err != nil || revParseErr != nil || headErr != nil || string(pushed) != string(head) {
		t.Fatalf("got push error %v, remote main %q (%v), want local HEAD %q (%v)", err, pushed, revParseErr, head, headErr)
	}
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

	got, err := Repository{Dir: clone}.StagedLineCounts(context.Background())

	if err != nil || got != (diff.LineCounts{Added: 2, Removed: 1}) {
		t.Fatalf("got %+v, %v, want +2 -1 from a.txt and staged.txt", got, err)
	}
}

func countsTheStagedLinesOnAnUnbornBranch(t *testing.T) {
	useTestIdentity(t)
	clone := t.TempDir()
	runSteps(t, clone, [][]string{{"init", "--initial-branch=main", clone}})
	writeFile(t, filepath.Join(clone, "staged.txt"), "a\nb\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "staged.txt"}})
	writeFile(t, filepath.Join(clone, "staged.txt"), "a\nb\nc\n")

	got, err := Repository{Dir: clone}.StagedLineCounts(context.Background())

	if err != nil || got != (diff.LineCounts{Added: 2, Removed: 0}) {
		t.Fatalf("got %+v, %v, want +2 -0", got, err)
	}
}

func TestTheLargestContextGitParsesShowsTheWholeFile(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1\n2\n3\n4\n5\n6\n7\n8\n9", "1\n2\n3\n4\n5\n6\n7\n8\nnine"})
	writeFile(t, filepath.Join(clone, "a.txt"), "one\n2\n3\n4\n5\n6\n7\n8\nnine\n")
	head, err := Repository{Dir: clone}.git(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	repository := Repository{Dir: clone}
	file := FileStatus{Path: "a.txt", IndexStatus: '.', WorktreeStatus: 'M'}
	hash := strings.TrimSpace(string(head))

	got := []string{}
	for _, contextLines := range []int{1, math.MaxInt32} {
		fileDiff, fileErr := repository.FileDiff(context.Background(), file, contextLines)
		commitDiff, commitErr := repository.CommitDiff(context.Background(), hash, contextLines)
		if fileErr != nil || commitErr != nil {
			t.Fatalf("got %v, %v", fileErr, commitErr)
		}
		got = append(got, hunkHeaders(fileDiff)+" "+hunkHeaders(commitDiff))
	}

	want := []string{"@@ -1,2 +1,2 @@ @@ -8,2 +8,2 @@", "@@ -1,9 +1,9 @@ @@ -1,9 +1,9 @@"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
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

func TestDefaultBranch(t *testing.T) {
	t.Run("countsTheCommitsHeadLacksFromOriginsDefaultBranch", countsTheCommitsHeadLacksFromOriginsDefaultBranch)
	t.Run("isEmptyWithoutOriginHead", isEmptyWithoutOriginHead)
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
	behind, behindErr := repository.CommitsBehind(context.Background(), branch)

	if branchErr != nil || behindErr != nil || branch != "origin/main" || behind != 2 {
		t.Fatalf("got %q (%v), %d behind (%v), want origin/main, 2 behind", branch, branchErr, behind, behindErr)
	}
}

func isEmptyWithoutOriginHead(t *testing.T) {
	_, clone := pushedClone(t)

	branch, err := Repository{Dir: clone}.DefaultBranch(context.Background())

	if err != nil || branch != "" {
		t.Fatalf("got %q, %v, want no default branch for a clone of an empty repository", branch, err)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	want := []Commit{{Hash: hash, ShortHash: "aaaaaaa", Parents: []string{parent}, Refs: []string{"HEAD -> master", "tag: v1"}, Author: "Ada", AuthoredAt: time.Unix(1700000000, 0),
		Summary: "Fix it", Message: "Fix it\n\nbody \x1f text"}}
	if !reflect.DeepEqual(commits, want) {
		t.Fatalf("commits:\n%+v\nwant:\n%+v", commits, want)
	}
}

func rejectsARecordWithoutAFullHash(t *testing.T) {
	_, err := parseLog([]byte("--output=x\x1fa\x1f\x1f\x1fAda\x1f1700000000\x1fFix\x00"))

	if err == nil {
		t.Fatal("got no error for a record whose hash is not a full commit hash")
	}
}

func rejectsAParentThatIsNotAFullHash(t *testing.T) {
	_, err := parseLog([]byte(strings.Repeat("a", 40) + "\x1faaaaaaa\x1f--all\x1f\x1fAda\x1f1700000000\x1fFix\x00"))

	if err == nil {
		t.Fatal("got no error for a parent that is not a full commit hash")
	}
}

func failureCarriesWhatGitPrintedOnStdout(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{"old.txt": "old line\n", "new.txt": "new line\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, err := Repository{Dir: dir}.git(context.Background(), "diff", "--no-index", "--exit-code", "--", "old.txt",
		"new.txt")

	if err == nil || !strings.Contains(err.Error(), "+new line") {
		t.Fatalf("got error %v, want one holding the diff git printed on stdout", err)
	}
}

func runSteps(t *testing.T, dir string, steps [][]string) {
	t.Helper()
	repository := Repository{Dir: dir}
	for _, step := range steps {
		if _, err := repository.git(context.Background(), step...); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscard(t *testing.T) {
	t.Run("discardsOnlyTheGivenFiles", discardsOnlyTheGivenFiles)
	t.Run("discardAllKeepsIgnoredFiles", discardAllKeepsIgnoredFiles)
}

func discardsOnlyTheGivenFiles(t *testing.T) {
	clone := changedWorkTree(t)
	repository := Repository{Dir: clone}
	status, err := repository.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range status.Files {
		if file.Path == "kept.txt" {
			continue
		}
		if err := repository.Discard(context.Background(), file); err != nil {
			t.Fatal(err)
		}
	}

	after, err := repository.Status(context.Background())
	content, readErr := os.ReadFile(filepath.Join(clone, "a.txt"))
	if err != nil || len(after.Files) != 1 || after.Files[0].Path != "kept.txt" {
		t.Fatalf("got %+v, %v, want only kept.txt left", after.Files, err)
	}
	if readErr != nil || string(content) != "1\n" {
		t.Fatalf("got a.txt %q, %v, want HEAD's 1", content, readErr)
	}
}

func discardAllKeepsIgnoredFiles(t *testing.T) {
	clone := changedWorkTree(t)
	repository := Repository{Dir: clone}

	err := repository.DiscardAll(context.Background())

	after, statusErr := repository.Status(context.Background())
	_, ignoredErr := os.Stat(filepath.Join(clone, "build.log"))
	_, untrackedDirErr := os.Stat(filepath.Join(clone, "dir"))
	if err != nil || statusErr != nil || len(after.Files) != 0 {
		t.Fatalf("got %+v, %v, %v, want a clean work tree", after.Files, err, statusErr)
	}
	if ignoredErr != nil || !errors.Is(untrackedDirErr, fs.ErrNotExist) {
		t.Fatalf("got ignored file %v, untracked dir %v, want the ignored file kept and the dir gone", ignoredErr,
			untrackedDirErr)
	}
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
	if err := os.Mkdir(filepath.Join(clone, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
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

	if err != nil || revParse(t, clone, "HEAD") != revParse(t, otherClone, "HEAD") {
		t.Fatalf("got %v, want HEAD at the pushed commit", err)
	}
}

func refusesADivergedBranch(t *testing.T) {
	clone, _ := cloneBehindOther(t)
	runSteps(t, clone, [][]string{{"-C", clone, "commit", "--allow-empty", "--message", "Local"}})
	before := revParse(t, clone, "HEAD")

	err := Repository{Dir: clone}.Pull(context.Background())

	if err == nil || revParse(t, clone, "HEAD") != before {
		t.Fatalf("got %v, want an error and HEAD unmoved", err)
	}
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

func TestABranchMergedAndDeletedOnTheRemote(t *testing.T) {
	t.Run("hasItsUpstreamGone", hasItsUpstreamGone)
	t.Run("isMergedIntoTheDefaultBranchUntilItChangesAFile", isMergedIntoTheDefaultBranchUntilItChangesAFile)
	t.Run("switchesToTheDefaultBranchCreatedFromOriginsSoPullFastForwardsIt",
		switchesToTheDefaultBranchCreatedFromOriginsSoPullFastForwardsIt)
}

func hasItsUpstreamGone(t *testing.T) {
	clone, _ := mergedAndDeletedOnTheRemote(t)

	status, err := Repository{Dir: clone}.Status(context.Background())

	if err != nil || status.Upstream != "origin/feature" || !status.UpstreamGone {
		t.Fatalf("got %+v, %v, want origin/feature gone", status, err)
	}
}

func isMergedIntoTheDefaultBranchUntilItChangesAFile(t *testing.T) {
	clone, _ := mergedAndDeletedOnTheRemote(t)
	repository := Repository{Dir: clone}

	merged, mergedErr := repository.MergedInto(context.Background(), "HEAD", "origin/main")
	writeFile(t, filepath.Join(clone, "local.txt"), "local\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "local.txt"}, {"-C", clone, "commit", "--message", "Local"}})
	mergedAfter, mergedAfterErr := repository.MergedInto(context.Background(), "HEAD", "origin/main")

	if mergedErr != nil || mergedAfterErr != nil || !merged || mergedAfter {
		t.Fatalf("got %t (%v) then %t (%v), want merged until the local commit", merged, mergedErr, mergedAfter,
			mergedAfterErr)
	}
}

func switchesToTheDefaultBranchCreatedFromOriginsSoPullFastForwardsIt(t *testing.T) {
	clone, otherClone := mergedAndDeletedOnTheRemote(t)
	repository := Repository{Dir: clone}

	switchErr := repository.Switch(context.Background(), "main")
	pullErr := repository.Pull(context.Background())

	status, err := repository.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if switchErr != nil || pullErr != nil || status.Branch != "main" ||
		revParse(t, clone, "HEAD") != revParse(t, otherClone, "HEAD") {
		t.Fatalf("got switch %v, pull %v, branch %q, want main at other's commit", switchErr, pullErr, status.Branch)
	}
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

	if err != nil || !merged {
		t.Fatalf("got %t, %v, want the squash merge counted", merged, err)
	}
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

func replacesTheUpstreamCommitThisBranchRewrote(t *testing.T) {
	remote, clone := pushedClone(t)
	runSteps(t, clone, [][]string{{"-C", clone, "commit", "--amend", "--allow-empty", "--message", "Rewritten"}})

	err := Repository{Dir: clone}.ForcePush(context.Background())

	if err != nil || revParse(t, remote, "main") != revParse(t, clone, "HEAD") {
		t.Fatalf("got push error %v, remote main %s, want local HEAD %s", err, revParse(t, remote, "main"),
			revParse(t, clone, "HEAD"))
	}
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

	err := Repository{Dir: clone}.ForcePush(context.Background())

	if err == nil || revParse(t, remote, "main") != before {
		t.Fatalf("got push error %v, remote main %s, want a refusal leaving it at %s", err, revParse(t, remote, "main"),
			before)
	}
}

func foldsTheStagedChangesIntoHead(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2"})
	parent := revParse(t, clone, "HEAD~1")
	writeFile(t, filepath.Join(clone, "a.txt"), "3\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "a.txt"}})

	err := Repository{Dir: clone}.Amend(context.Background(), revParse(t, clone, "HEAD"))

	if err != nil || blobAt(t, clone, "HEAD:a.txt") != "3\n" || revParse(t, clone, "HEAD~1") != parent {
		t.Fatalf("got error %v, a.txt at HEAD %q, want 3 on the same parent", err, blobAt(t, clone, "HEAD:a.txt"))
	}
}

func foldsTheStagedChangesIntoAnOlderCommitAndKeepsUnstagedOnes(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2", "3"})
	writeFile(t, filepath.Join(clone, "b.txt"), "staged\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "b.txt"}})
	writeFile(t, filepath.Join(clone, "a.txt"), "unstaged\n")

	err := Repository{Dir: clone}.Amend(context.Background(), revParse(t, clone, "HEAD~1"))

	count, countErr := Repository{Dir: clone}.git(context.Background(), "rev-list", "--count", "HEAD")
	unstaged, readErr := os.ReadFile(filepath.Join(clone, "a.txt"))
	if err != nil || countErr != nil || readErr != nil || strings.TrimSpace(string(count)) != "3" ||
		blobAt(t, clone, "HEAD~1:b.txt") != "staged\n" || string(unstaged) != "unstaged\n" {
		t.Fatalf("got error %v, %q commits (%v), a.txt %q (%v), want b.txt in the second of 3 commits, a.txt untouched",
			err, count, countErr, unstaged, readErr)
	}
}

func undoesTheFixupWhenTheRebaseStops(t *testing.T) {
	clone := repositoryWithHistory(t, []string{"1", "2", "3"})
	head := revParse(t, clone, "HEAD")
	writeFile(t, filepath.Join(clone, "a.txt"), "4\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "a.txt"}})

	err := Repository{Dir: clone}.Amend(context.Background(), revParse(t, clone, "HEAD~1"))

	staged, diffErr := Repository{Dir: clone}.git(context.Background(), "diff", "--cached", "--name-only")
	_, statErr := os.Stat(filepath.Join(clone, ".git", "rebase-merge"))
	if err == nil || revParse(t, clone, "HEAD") != head || diffErr != nil ||
		strings.TrimSpace(string(staged)) != "a.txt" ||
		!os.IsNotExist(statErr) {
		t.Fatalf("got error %v, HEAD %s, staged %q (%v), rebase state %v, want HEAD back at %s with a.txt staged",
			err, revParse(t, clone, "HEAD"), staged, diffErr, statErr, head)
	}
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

	if !errors.Is(err, ErrCannotAmend) || revParse(t, clone, "HEAD") != head {
		t.Fatalf("got error %v, want ErrCannotAmend with HEAD left at %s", err, head)
	}
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
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func revParse(t *testing.T, dir string, revision string) string {
	t.Helper()
	hash, err := Repository{Dir: dir}.revParse(context.Background(), revision)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func blobAt(t *testing.T, dir string, object string) string {
	t.Helper()
	content, err := Repository{Dir: dir}.git(context.Background(), "show", object)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
