package gitrepo

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	stashErr := repository.Stash(context.Background())
	require.NoError(t, stashErr)
	writeFile(t, filepath.Join(repository.Dir, "a.txt"), "newer\n")
	runSteps(t, repository.Dir, [][]string{{"-C", repository.Dir, "stash", "push", "--message", "try: the overview"}})

	stashes, err := repository.Stashes(context.Background())

	require.NoError(t, err)
	require.Len(t, stashes, 2)
	require.Equal(t, "main", stashes[0].Branch)
	require.Equal(t, "try: the overview", stashes[0].Summary)
	require.Equal(t, "main", stashes[1].Branch)
	require.Equal(t, "Write 1", stashes[1].Summary)
}

func diffShowsTrackedAndUntrackedFiles(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	stashErr := repository.Stash(context.Background())
	require.NoError(t, stashErr)
	stashes, stashesErr := repository.Stashes(context.Background())
	require.NoError(t, stashesErr)

	text, err := repository.StashDiff(context.Background(), stashes[0], 3)

	require.NoError(t, err)
	require.Contains(t, text, "+unstaged")
	require.Contains(t, text, "+untracked")
}

func popRestoresStagedUnstagedAndUntrackedChanges(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	stashErr := repository.Stash(context.Background())
	require.NoError(t, stashErr)
	stashed, statusErr := repository.Status(context.Background())
	require.NoError(t, statusErr)
	require.Empty(t, stashed.Files)
	headLine := gitOutput(t, repository.Dir, "log", "--max-count=1", "--format=%h %s")
	head := strings.TrimSpace(headLine)
	stashes, stashesErr := repository.Stashes(context.Background())
	require.NoError(t, stashesErr)
	require.Len(t, stashes, 1)
	require.Equal(t, "stash@{0}", stashes[0].Ref)
	require.Equal(t, "WIP on main: "+head, stashes[0].Message)

	err := repository.PopStash(context.Background(), stashes[0])

	require.NoError(t, err)
	staged := blobAt(t, repository.Dir, ":a.txt")
	require.Equal(t, "staged\n", staged)
	unstagedFile := filepath.Join(repository.Dir, "a.txt")
	assertFileHolds(t, unstagedFile, "unstaged\n")
	untracked := filepath.Join(repository.Dir, "untracked.txt")
	require.FileExists(t, untracked)
	remaining, remainingErr := repository.Stashes(context.Background())
	require.NoError(t, remainingErr)
	require.Empty(t, remaining)
}

func popRefusesAStashANewerOneShifted(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	stashErr := repository.Stash(context.Background())
	require.NoError(t, stashErr)
	stashes, stashesErr := repository.Stashes(context.Background())
	require.NoError(t, stashesErr)
	writeFile(t, filepath.Join(repository.Dir, "a.txt"), "newer\n")
	newerStashErr := repository.Stash(context.Background())
	require.NoError(t, newerStashErr)

	err := repository.PopStash(context.Background(), stashes[0])

	require.ErrorIs(t, err, ErrStashListChanged)
	remaining, remainingErr := repository.Stashes(context.Background())
	require.NoError(t, remainingErr)
	require.Len(t, remaining, 2)
}

func dropDropsOnlyTheGivenStash(t *testing.T) {
	repository := partlyStagedWorkTree(t)
	stashErr := repository.Stash(context.Background())
	require.NoError(t, stashErr)
	writeFile(t, filepath.Join(repository.Dir, "a.txt"), "newer\n")
	newerStashErr := repository.Stash(context.Background())
	require.NoError(t, newerStashErr)
	stashes, stashesErr := repository.Stashes(context.Background())
	require.NoError(t, stashesErr)
	require.Len(t, stashes, 2)

	err := repository.DropStash(context.Background(), stashes[1])

	require.NoError(t, err)
	remaining, remainingErr := repository.Stashes(context.Background())
	require.NoError(t, remainingErr)
	require.Len(t, remaining, 1)
	require.Equal(t, stashes[0].Hash, remaining[0].Hash)
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
