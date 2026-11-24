package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/focus"
)

func TestReadOwnerWorkTree(t *testing.T) {
	t.Run("aSquashMergedBranchWithAGoneUpstreamNamesTheBranchItIsMergedInto",
		aSquashMergedBranchWithAGoneUpstreamNamesTheBranchItIsMergedInto)
	t.Run("aSquashMergedBranchWithALiveUpstreamNamesTheBranchItIsMergedInto",
		aSquashMergedBranchWithALiveUpstreamNamesTheBranchItIsMergedInto)
	t.Run("aFreshBranchAtOriginsMainIsNotMerged", aFreshBranchAtOriginsMainIsNotMerged)
	t.Run("aBranchWithAChangeOriginsMainLacksIsNotMerged", aBranchWithAChangeOriginsMainLacksIsNotMerged)
	t.Run("listsTheFirstFiftyChangedFilesAndCountsThemAll", listsTheFirstFiftyChangedFilesAndCountsThemAll)
}

func aSquashMergedBranchWithAGoneUpstreamNamesTheBranchItIsMergedInto(t *testing.T) {
	clone := squashMergedBranchWithAGoneUpstream(t)

	workTree, err := ReadOwnerWorkTree(context.Background(), foundAt("work", clone))

	if err != nil {
		t.Fatal(err)
	}
	want := focus.OwnerWorkTree{Branch: "fix", Head: gitIn(t, clone, "rev-parse", "HEAD"), HeadSubject: "Fix the parser",
		ChangedFiles: []string{}, BehindDefault: 1, MergedInto: "origin/main"}
	if !reflect.DeepEqual(workTree, want) {
		t.Fatalf("got\n%+v\nwant\n%+v", workTree, want)
	}
}

func aSquashMergedBranchWithALiveUpstreamNamesTheBranchItIsMergedInto(t *testing.T) {
	clone := squashMergedBranchWithALiveUpstream(t)

	workTree, err := ReadOwnerWorkTree(context.Background(), foundAt("work", clone))

	if err != nil || workTree.Upstream != "origin/fix" || workTree.MergedInto != "origin/main" {
		t.Fatalf("got %+v, %v, want upstream origin/fix merged into origin/main", workTree, err)
	}
}

func aFreshBranchAtOriginsMainIsNotMerged(t *testing.T) {
	clone := squashMergedBranchWithALiveUpstream(t)
	gitIn(t, clone, "switch", "--create", "fresh", "origin/main")

	workTree, err := ReadOwnerWorkTree(context.Background(), foundAt("work", clone))

	if err != nil || workTree.Branch != "fresh" || workTree.MergedInto != "" {
		t.Fatalf("got %+v, %v, want fresh with no mergedInto", workTree, err)
	}
}

func aBranchWithAChangeOriginsMainLacksIsNotMerged(t *testing.T) {
	clone := squashMergedBranchWithALiveUpstream(t)
	if err := os.WriteFile(filepath.Join(clone, "lexer.go"), []byte("package parser\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, clone, "add", "lexer.go")
	gitIn(t, clone, "commit", "--message", "Add the lexer")

	workTree, err := ReadOwnerWorkTree(context.Background(), foundAt("work", clone))

	if err != nil || workTree.MergedInto != "" {
		t.Fatalf("got %+v, %v, want no mergedInto", workTree, err)
	}
}

func listsTheFirstFiftyChangedFilesAndCountsThemAll(t *testing.T) {
	clone := squashMergedBranchWithAGoneUpstream(t)
	for index := range 51 {
		if err := os.WriteFile(filepath.Join(clone, fmt.Sprintf("new%02d.txt", index)), []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	workTree, err := ReadOwnerWorkTree(context.Background(), foundAt("work", clone))

	if err != nil || len(workTree.ChangedFiles) != 50 || workTree.ChangedFiles[49] != "new49.txt" ||
		workTree.ChangedFileCount != 51 || workTree.MergedInto != "origin/main" {
		t.Fatalf("got %+v, %v, want new00.txt to new49.txt of 51 files, still merged into origin/main", workTree, err)
	}
}

func squashMergedBranchWithAGoneUpstream(t *testing.T) string {
	t.Helper()
	clone := squashMergedBranchWithALiveUpstream(t)
	gitIn(t, clone, "push", "--delete", "origin", "fix")
	gitIn(t, clone, "fetch", "--prune")
	return clone
}

func squashMergedBranchWithALiveUpstream(t *testing.T) string {
	t.Helper()
	for name, value := range map[string]string{
		"GIT_AUTHOR_NAME": "perusal", "GIT_AUTHOR_EMAIL": "perusal@example.com",
		"GIT_COMMITTER_NAME": "perusal", "GIT_COMMITTER_EMAIL": "perusal@example.com",
		"GIT_CONFIG_COUNT": "2", "GIT_CONFIG_KEY_0": "commit.gpgsign", "GIT_CONFIG_VALUE_0": "false",
		"GIT_CONFIG_KEY_1": "core.hooksPath", "GIT_CONFIG_VALUE_1": t.TempDir(),
	} {
		t.Setenv(name, value)
	}
	parent := t.TempDir()
	remote := filepath.Join(parent, "remote.git")
	clone := filepath.Join(parent, "work")
	gitIn(t, parent, "init", "--bare", "--initial-branch=main", remote)
	gitIn(t, parent, "clone", remote, clone)
	gitIn(t, clone, "commit", "--allow-empty", "--message", "Start")
	gitIn(t, clone, "push", "origin", "main")
	gitIn(t, clone, "remote", "set-head", "origin", "main")
	gitIn(t, clone, "switch", "--create", "fix")
	commitParser(t, clone, "Fix the parser")
	gitIn(t, clone, "push", "--set-upstream", "origin", "fix")
	gitIn(t, clone, "switch", "main")
	commitParser(t, clone, "Fix the parser (#1)")
	gitIn(t, clone, "push", "origin", "main")
	gitIn(t, clone, "switch", "fix")
	return clone
}

func commitParser(t *testing.T, clone string, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(clone, "parser.go"), []byte("package parser\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, clone, "add", "parser.go")
	gitIn(t, clone, "commit", "--message", message)
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
