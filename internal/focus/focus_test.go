//go:build windows

package focus

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMapPath(t *testing.T) {
	cases := []struct {
		name      string
		agentPath string
		want      string
		wantErr   error
	}{
		{"mapsARepository", `C:\Users\claude\dev\perusal`, `C:\Users\Chris\dev\perusal`, nil},
		{"mapsANestedDirectory", `C:\Users\claude\dev\openziti\ziti\cmd`, `C:\Users\Chris\dev\openziti\ziti\cmd`, nil},
		{"mapsTheDevDirectoryItself", `C:\Users\claude\dev\`, `C:\Users\Chris\dev`, nil},
		{"ignoresCase", `c:\users\CLAUDE\Dev\perusal`, `C:\Users\Chris\dev\perusal`, nil},
		{"refusesASiblingWithTheSamePrefix", `C:\Users\claude\devtools\x`, "", ErrOutsideAgentDir},
		{"refusesDotDotOutOfTheAgentDirectory", `C:\Users\claude\dev\..\..\Chris\.ssh`, "", ErrOutsideAgentDir},
		{"refusesARelativePath", `dev\perusal`, "", ErrOutsideAgentDir},
		{"refusesADriveRelativePath", `C:dev\perusal`, "", ErrOutsideAgentDir},
		{"refusesAVerbatimPrefix", `\\?\C:\Users\claude\dev\perusal`, "", ErrOutsideAgentDir},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := mapPath(testCase.agentPath, `C:\Users\claude\dev`, `C:\Users\Chris\dev`)

			if !errors.Is(err, testCase.wantErr) || got != testCase.want {
				t.Fatalf("got %q, %v, want %q, %v", got, err, testCase.want, testCase.wantErr)
			}
		})
	}
}

func TestParseFocusFile(t *testing.T) {
	t.Run("readsOnePathPerLineWithAnyLineBreak", readsOnePathPerLineWithAnyLineBreak)
	t.Run("refusesAnEmptyLine", refusesAnEmptyLine)
}

func readsOnePathPerLineWithAnyLineBreak(t *testing.T) {
	got, err := parseFocusFile("C:\\Users\\claude\\dev\\a\r\nC:\\Users\\claude\\dev\\b\nC:\\Users\\claude\\dev\\c\r\n")

	want := []string{`C:\Users\claude\dev\a`, `C:\Users\claude\dev\b`, `C:\Users\claude\dev\c`}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("got %q, %v, want %q", got, err, want)
	}
}

func refusesAnEmptyLine(t *testing.T) {
	_, err := parseFocusFile("C:\\Users\\claude\\dev\\a\n\nC:\\Users\\claude\\dev\\b")

	if !errors.Is(err, ErrMalformedFocusFile) {
		t.Fatalf("got %v, want %v", err, ErrMalformedFocusFile)
	}
}

func TestFindWorkTree(t *testing.T) {
	t.Run("findsTheNearestGitAbove", findsTheNearestGitAbove)
	t.Run("stopsAtTheOwnerDirectory", stopsAtTheOwnerDirectory)
}

func findsTheNearestGitAbove(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "org", "repo")
	mustMkdir(t, filepath.Join(repository, ".git"))
	mustMkdir(t, filepath.Join(repository, "cmd", "tool"))

	got, err := findWorkTree(filepath.Join(repository, "cmd", "tool"), devDir)

	if err != nil || got != repository {
		t.Fatalf("got %q, %v, want %q", got, err, repository)
	}
}

func stopsAtTheOwnerDirectory(t *testing.T) {
	outer := t.TempDir()
	mustMkdir(t, filepath.Join(outer, ".git"))
	devDir := filepath.Join(outer, "dev")
	mustMkdir(t, filepath.Join(devDir, "notes"))

	_, err := findWorkTree(filepath.Join(devDir, "notes"), devDir)

	if !errors.Is(err, ErrNotWorkTree) {
		t.Fatalf("got %v, want %v", err, ErrNotWorkTree)
	}
}

func TestResolve(t *testing.T) {
	t.Run("opensTheCheckedWorkTree", opensTheCheckedWorkTree)
	t.Run("refusesAWorkTreeGitSkipsPast", refusesAWorkTreeGitSkipsPast)
	t.Run("keepsEachRepositoryOnceAndReportsTheRefusedPaths", keepsEachRepositoryOnceAndReportsTheRefusedPaths)
}

const agentDevDir = `C:\agent\dev`

func opensTheCheckedWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "repo")
	gitInit(t, repository)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	rules := guestRules(t, devDir, focusFileHolding(t, agentDevDir+`\repo`))

	got, err := resolve(context.Background(), rules)

	if err != nil || len(got.Repositories) != 1 {
		t.Fatalf("got %v, %v, want one repository", got, err)
	}
	same, err := isSameDirectory(got.Repositories[0].Dir, repository)
	if err != nil || !same {
		t.Fatalf("got %q, %v, want %q", got.Repositories[0].Dir, err, repository)
	}
}

func keepsEachRepositoryOnceAndReportsTheRefusedPaths(t *testing.T) {
	devDir := t.TempDir()
	for _, name := range []string{"first", "second"} {
		gitInit(t, filepath.Join(devDir, name))
		denyGuests(t, filepath.Join(devDir, name, ".git"), deniedWriteMask)
	}
	mustMkdir(t, filepath.Join(devDir, "first", "cmd"))
	mustMkdir(t, filepath.Join(devDir, "notes"))
	lines := []string{agentDevDir + `\second`, agentDevDir + `\notes`, agentDevDir + `\first\cmd`, agentDevDir + `\first`}
	rules := guestRules(t, devDir, focusFileHolding(t, strings.Join(lines, "\n")))

	got, err := resolve(context.Background(), rules)

	if err != nil || len(got.Repositories) != 2 || len(got.Refusals) != 1 || !errors.Is(got.Refusals[0], ErrNotWorkTree) {
		t.Fatalf("got %v, %v, want second and first, and notes refused", got, err)
	}
	for index, name := range []string{"second", "first"} {
		same, err := isSameDirectory(got.Repositories[index].Dir, filepath.Join(devDir, name))
		if err != nil || !same {
			t.Fatalf("got repository %d %q, %v, want %s", index, got.Repositories[index].Dir, err, name)
		}
	}
}

func refusesAWorkTreeGitSkipsPast(t *testing.T) {
	outer := t.TempDir()
	gitInit(t, outer)
	devDir := filepath.Join(outer, "dev")
	emptyGitDir := filepath.Join(devDir, "repo", ".git")
	mustMkdir(t, emptyGitDir)
	denyGuests(t, emptyGitDir, deniedWriteMask)
	rules := guestRules(t, devDir, focusFileHolding(t, agentDevDir+`\repo`))

	_, err := resolve(context.Background(), rules)

	if !errors.Is(err, ErrWorkTreeMismatch) {
		t.Fatalf("got %v, want %v", err, ErrWorkTreeMismatch)
	}
}

func TestFindRepositoriesRefusesAnUnprotectedOneAndSkipsANestedOne(t *testing.T) {
	devDir := t.TempDir()
	gitInit(t, filepath.Join(devDir, "org", "repo"))
	denyGuests(t, filepath.Join(devDir, "org", "repo", ".git"), deniedWriteMask)
	gitInit(t, filepath.Join(devDir, "org", "repo", "vendored"))
	denyGuests(t, filepath.Join(devDir, "org", "repo", "vendored", ".git"), deniedWriteMask)
	gitInit(t, filepath.Join(devDir, "plain"))
	mustMkdir(t, filepath.Join(devDir, "notes"))
	rules := guestRules(t, devDir, "")

	got, err := FindRepositories(context.Background(), rules)

	if err != nil || len(got.Repositories) != 1 || got.Repositories[0].RelativePath != "org/repo" {
		t.Fatalf("got %v, %v, want only org/repo", got, err)
	}
	if len(got.Refusals) != 1 || got.Refusals[0].RelativePath != "plain" ||
		got.Refusals[0].Summary() != "not write-denied to the agent" {
		t.Fatalf("got refusals %v, want plain refused as %v", got.Refusals, ErrGitDirUnprotected)
	}
}

func TestFindRepositoriesListsALinkedWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository, _ := repositoryWithWorkTree(t, devDir)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)

	got, err := FindRepositories(context.Background(), guestRules(t, devDir, ""))

	if err != nil || len(got.Repositories) != 2 || got.Repositories[1].RelativePath != "repo-side" ||
		len(got.Refusals) != 0 {
		t.Fatalf("got %v, %v, want repo and repo-side", got, err)
	}
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

	got, err := resolve(context.Background(), guestRules(t, devDir, focusFileHolding(t, agentDevDir+`\repo-side\cmd`)))

	if err != nil || len(got.Repositories) != 1 {
		t.Fatalf("got %v, %v, want one repository", got, err)
	}
	same, err := isSameDirectory(got.Repositories[0].Dir, side)
	if err != nil || !same {
		t.Fatalf("got %q, %v, want %q", got.Repositories[0].Dir, err, side)
	}
	status, err := got.Repositories[0].Status(context.Background())
	if err != nil || status.Branch != "side" {
		t.Fatalf("got branch %q, %v, want side", status.Branch, err)
	}
}

func isRefusedWhenItsRepositoryIsUnprotected(t *testing.T) {
	devDir := t.TempDir()
	repositoryWithWorkTree(t, devDir)

	_, err := resolve(context.Background(), guestRules(t, devDir, focusFileHolding(t, agentDevDir+`\repo-side`)))

	if !errors.Is(err, ErrGitDirUnprotected) {
		t.Fatalf("got %v, want %v", err, ErrGitDirUnprotected)
	}
}

func isRefusedWhenItsGitFileNamesAnotherWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	mustGit(t, "-C", repository, "worktree", "add", "-b", "other", filepath.Join(devDir, "repo-other"))
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	writeGitFile(t, side, filepath.Join(repository, ".git", "worktrees", "repo-other"))

	_, err := resolve(context.Background(), guestRules(t, devDir, focusFileHolding(t, agentDevDir+`\repo-side`)))

	if !errors.Is(err, ErrNotLinkedWorkTree) {
		t.Fatalf("got %v, want %v", err, ErrNotLinkedWorkTree)
	}
}

func opensThroughAJunctionAsItsRealPath(t *testing.T) {
	devDir := t.TempDir()
	repository, side := repositoryWithWorkTree(t, devDir)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	junction := filepath.Join(t.TempDir(), "link")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, filepath.Join(repository, ".git")).
		CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, output)
	}
	writeGitFile(t, side, filepath.Join(junction, "worktrees", "repo-side"))

	got, err := resolve(context.Background(), guestRules(t, devDir, focusFileHolding(t, agentDevDir+`\repo-side`)))

	want := filepath.Join(repository, ".git", "worktrees", "repo-side")
	if err != nil || len(got.Repositories) != 1 || !strings.EqualFold(got.Repositories[0].GitDir, want) {
		t.Fatalf("got %v, %v, want git directory %s", got, err, want)
	}
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

func writeGitFile(t *testing.T, workTree string, adminDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workTree, ".git"), []byte("gitdir: "+adminDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, "init", dir)
}

func mustGit(t *testing.T, args ...string) {
	t.Helper()
	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func guestRules(t *testing.T, devDir string, focusFile string) Rules {
	t.Helper()
	config := Config{FocusFile: focusFile, AgentDir: agentDevDir, OwnerDir: devDir}
	return Rules{Config: config, AgentSID: parsedGuestsSID(t)}
}

func focusFileHolding(t *testing.T, agentPath string) string {
	t.Helper()
	focusFile := filepath.Join(t.TempDir(), ".herdr-focus")
	if err := os.WriteFile(focusFile, []byte(agentPath), 0o600); err != nil {
		t.Fatal(err)
	}
	return focusFile
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}
