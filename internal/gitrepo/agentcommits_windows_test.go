package gitrepo

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTakeAgentCommits(t *testing.T) {
	t.Run("showsTheAgentsCommitAsUnstagedChanges", showsTheAgentsCommitAsUnstagedChanges)
	t.Run("takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits", takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits)
	t.Run("keepsAFileTheOwnerEditedAndRefusesTheCommit", keepsAFileTheOwnerEditedAndRefusesTheCommit)
	t.Run("skipsTheFetchWhenTheAgentsHeadIsTaken", skipsTheFetchWhenTheAgentsHeadIsTaken)
}

func TestReadAgentHeads(t *testing.T) {
	_, agent := ownerAndAgentCopies(t)
	notAClone := t.TempDir()

	heads, err := ReadAgentHeads(context.Background(), localAgentCopy(agent).Run, []string{notAClone, agent})

	if err != nil {
		t.Fatal(err)
	}
	if heads[0].Err == nil || !strings.Contains(heads[0].Err.Error(), notAClone) {
		t.Fatalf("got error %v for %s, want one naming it", heads[0].Err, notAClone)
	}
	if heads[1] != (AgentHead{Commit: revParse(t, agent, "HEAD"), Branch: "refs/heads/main"}) {
		t.Fatalf("got %+v for %s, want its HEAD on main", heads[1], agent)
	}
}

func showsTheAgentsCommitAsUnstagedChanges(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	ownerHead := revParse(t, owner, "HEAD")
	runSteps(t, agent, [][]string{{"switch", "--create", "claude/x"}})
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	writeFile(t, filepath.Join(agent, "new.txt"), "new\n")
	removeFile(t, filepath.Join(agent, "b.txt"))
	commitAll(t, agent, "Agent")

	changed, err := takeAgentCommits(t, owner, agent)

	if err != nil || !changed {
		t.Fatalf("got changed %v, error %v, want the commit taken", changed, err)
	}
	assertChangedFiles(t, owner, []FileStatus{
		{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'},
		{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'D'},
		{Path: "new.txt", IndexStatus: '?', WorktreeStatus: '?'},
	})
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "agent\n")
	if head := revParse(t, owner, "HEAD"); head != ownerHead {
		t.Fatalf("got the owner's HEAD at %s, want it left at %s", head, ownerHead)
	}
}

func takesOnlyWhatTheNextCommitChangesAfterTheOwnerCommits(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "First")
	if _, err := takeAgentCommits(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	commitAll(t, owner, "Owner takes the first")
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	commitAll(t, agent, "Second")

	_, err := takeAgentCommits(t, owner, agent)

	if err != nil {
		t.Fatal(err)
	}
	assertChangedFiles(t, owner, []FileStatus{{Path: "b.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func keepsAFileTheOwnerEditedAndRefusesTheCommit(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "First")
	if _, err := takeAgentCommits(t, owner, agent); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(owner, "a.txt"), "owner\n")
	writeFile(t, filepath.Join(agent, "a.txt"), "agent again\n")
	writeFile(t, filepath.Join(agent, "b.txt"), "agent\n")
	commitAll(t, agent, "Second")

	changed, err := takeAgentCommits(t, owner, agent)

	if err == nil || !strings.Contains(err.Error(), "a.txt") || changed {
		t.Fatalf("got changed %v, error %v, want the commit refused over a.txt", changed, err)
	}
	assertFileHolds(t, filepath.Join(owner, "a.txt"), "owner\n")
	assertFileHolds(t, filepath.Join(owner, "b.txt"), "b\n")
}

func skipsTheFetchWhenTheAgentsHeadIsTaken(t *testing.T) {
	owner, agent := ownerAndAgentCopies(t)
	writeFile(t, filepath.Join(agent, "a.txt"), "agent\n")
	commitAll(t, agent, "Agent")
	if _, err := takeAgentCommits(t, owner, agent); err != nil {
		t.Fatal(err)
	}

	changed, err := Repository{Dir: owner}.TakeAgentCommits(context.Background(),
		filepath.Join(t.TempDir(), "missing"), readAgentHead(t, agent))

	if err != nil || changed {
		t.Fatalf("got changed %v, error %v, want nothing to do", changed, err)
	}
	assertChangedFiles(t, owner, []FileStatus{{Path: "a.txt", IndexStatus: ' ', WorktreeStatus: 'M'}})
}

func takeAgentCommits(t *testing.T, owner string, agent string) (bool, error) {
	t.Helper()
	return Repository{Dir: owner}.TakeAgentCommits(context.Background(), agent, readAgentHead(t, agent))
}

func readAgentHead(t *testing.T, agent string) AgentHead {
	t.Helper()
	heads, err := ReadAgentHeads(context.Background(), localAgentCopy(agent).Run, []string{agent})
	if err != nil {
		t.Fatal(err)
	}
	return heads[0]
}

func commitAll(t *testing.T, dir string, message string) {
	t.Helper()
	runSteps(t, dir, [][]string{{"add", "--all"}, {"commit", "--message", message}})
}
