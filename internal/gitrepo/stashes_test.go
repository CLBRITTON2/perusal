package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStash(t *testing.T) {
	t.Run("popRestoresStagedUnstagedAndUntrackedChanges", popRestoresStagedUnstagedAndUntrackedChanges)
	t.Run("popRefusesAStashANewerOneShifted", popRefusesAStashANewerOneShifted)
	t.Run("dropDropsOnlyTheGivenStash", dropDropsOnlyTheGivenStash)
	t.Run("listSplitsTheBranchFromTheSummary", listSplitsTheBranchFromTheSummary)
	t.Run("diffShowsTrackedAndUntrackedFiles", diffShowsTrackedAndUntrackedFiles)
}

func listSplitsTheBranchFromTheSummary(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository.Dir, "a.txt"), "newer\n")
	runSteps(t, repository.Dir, [][]string{{"-C", repository.Dir, "stash", "push", "--message", "try: the overview"}})

	stashes, err := repository.Stashes(context.Background())

	if err != nil || len(stashes) != 2 || stashes[0].Branch != "main" || stashes[0].Summary != "try: the overview" ||
		stashes[1].Branch != "main" || stashes[1].Summary != "Write 1" {
		t.Fatalf("got %+v, %v, want both on main, summarized without git's prefix", stashes, err)
	}
}

func diffShowsTrackedAndUntrackedFiles(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}
	stashes, err := repository.Stashes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	text, err := repository.StashDiff(context.Background(), stashes[0], 3)

	if err != nil || !strings.Contains(text, "+unstaged") || !strings.Contains(text, "+untracked") {
		t.Fatalf("got %v, diff:\n%s\nwant a.txt's work tree change and untracked.txt", err, text)
	}
}

func popRestoresStagedUnstagedAndUntrackedChanges(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}
	stashed, err := repository.Status(context.Background())
	if err != nil || len(stashed.Files) != 0 {
		t.Fatalf("got %+v, %v, want a clean work tree once stashed", stashed.Files, err)
	}
	head := strings.TrimSpace(gitOutput(t, repository.Dir, "log", "--max-count=1", "--format=%h %s"))
	stashes, err := repository.Stashes(context.Background())
	if err != nil || len(stashes) != 1 || stashes[0].Ref != "stash@{0}" || stashes[0].Message != "WIP on main: "+head {
		t.Fatalf("got %+v, %v, want one stash named for main's head", stashes, err)
	}

	err = repository.PopStash(context.Background(), stashes[0])

	content, readErr := os.ReadFile(filepath.Join(repository.Dir, "a.txt"))
	_, untrackedErr := os.Stat(filepath.Join(repository.Dir, "untracked.txt"))
	remaining, stashesErr := repository.Stashes(context.Background())
	if err != nil || blobAt(t, repository.Dir, ":a.txt") != "staged\n" || readErr != nil ||
		string(content) != "unstaged\n" {
		t.Fatalf("got %v, a.txt %q (%v), want staged and unstaged changes back", err, content, readErr)
	}
	if untrackedErr != nil || stashesErr != nil || len(remaining) != 0 {
		t.Fatalf("got untracked file %v, stashes %+v (%v), want it back and the stash dropped", untrackedErr, remaining,
			stashesErr)
	}
}

func popRefusesAStashANewerOneShifted(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}
	stashes, err := repository.Stashes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository.Dir, "a.txt"), "newer\n")
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}

	err = repository.PopStash(context.Background(), stashes[0])

	remaining, stashesErr := repository.Stashes(context.Background())
	if !errors.Is(err, ErrStashListChanged) || stashesErr != nil || len(remaining) != 2 {
		t.Fatalf("got %v, %+v (%v), want ErrStashListChanged with both stashes kept", err, remaining, stashesErr)
	}
}

func dropDropsOnlyTheGivenStash(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository.Dir, "a.txt"), "newer\n")
	if err := repository.Stash(context.Background()); err != nil {
		t.Fatal(err)
	}
	stashes, err := repository.Stashes(context.Background())
	if err != nil || len(stashes) != 2 {
		t.Fatalf("got %+v, %v, want two stashes", stashes, err)
	}

	err = repository.DropStash(context.Background(), stashes[1])

	remaining, stashesErr := repository.Stashes(context.Background())
	if err != nil || stashesErr != nil || len(remaining) != 1 || remaining[0].Hash != stashes[0].Hash {
		t.Fatalf("got %v, %+v (%v), want only the newer stash %s left", err, remaining, stashesErr, stashes[0].Hash)
	}
}

func partlyStagedWorkTree(t *testing.T) Repository {
	t.Helper()
	clone := repositoryWithHistory(t, []string{"1"})
	writeFile(t, filepath.Join(clone, "a.txt"), "staged\n")
	runSteps(t, clone, [][]string{{"-C", clone, "add", "a.txt"}})
	writeFile(t, filepath.Join(clone, "a.txt"), "unstaged\n")
	writeFile(t, filepath.Join(clone, "untracked.txt"), "untracked\n")
	return Repository{Dir: clone}
}
