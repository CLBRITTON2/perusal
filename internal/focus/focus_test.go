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
			got, err := MapPath(testCase.agentPath, `C:\Users\claude\dev`, `C:\Users\Chris\dev`)

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
	got, err := ParseFocusFile("C:\\Users\\claude\\dev\\a\r\nC:\\Users\\claude\\dev\\b\nC:\\Users\\claude\\dev\\c\r\n")

	want := []string{`C:\Users\claude\dev\a`, `C:\Users\claude\dev\b`, `C:\Users\claude\dev\c`}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("got %q, %v, want %q", got, err, want)
	}
}

func refusesAnEmptyLine(t *testing.T) {
	_, err := ParseFocusFile("C:\\Users\\claude\\dev\\a\n\nC:\\Users\\claude\\dev\\b")

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

	got, err := FindWorkTree(filepath.Join(repository, "cmd", "tool"), devDir)

	if err != nil || got != repository {
		t.Fatalf("got %q, %v, want %q", got, err, repository)
	}
}

func stopsAtTheOwnerDirectory(t *testing.T) {
	outer := t.TempDir()
	mustMkdir(t, filepath.Join(outer, ".git"))
	devDir := filepath.Join(outer, "dev")
	mustMkdir(t, filepath.Join(devDir, "notes"))

	_, err := FindWorkTree(filepath.Join(devDir, "notes"), devDir)

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
	rules := Rules{AgentDir: agentDevDir, OwnerDir: devDir, AgentSID: guestsSID}

	got, err := Resolve(context.Background(), rules, focusFileHolding(t, agentDevDir+`\repo`))

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
	rules := Rules{AgentDir: agentDevDir, OwnerDir: devDir, AgentSID: guestsSID}
	lines := []string{agentDevDir + `\second`, agentDevDir + `\notes`, agentDevDir + `\first\cmd`, agentDevDir + `\first`}

	got, err := Resolve(context.Background(), rules, focusFileHolding(t, strings.Join(lines, "\n")))

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
	rules := Rules{AgentDir: agentDevDir, OwnerDir: devDir, AgentSID: guestsSID}

	_, err := Resolve(context.Background(), rules, focusFileHolding(t, agentDevDir+`\repo`))

	if !errors.Is(err, ErrWorkTreeMismatch) {
		t.Fatalf("got %v, want %v", err, ErrWorkTreeMismatch)
	}
}

func TestFindRepositories(t *testing.T) {
	devDir := t.TempDir()
	gitInit(t, filepath.Join(devDir, "org", "repo"))
	denyGuests(t, filepath.Join(devDir, "org", "repo", ".git"), deniedWriteMask)
	gitInit(t, filepath.Join(devDir, "org", "repo", "vendored"))
	denyGuests(t, filepath.Join(devDir, "org", "repo", "vendored", ".git"), deniedWriteMask)
	gitInit(t, filepath.Join(devDir, "plain"))
	mustMkdir(t, filepath.Join(devDir, "notes"))
	rules := Rules{AgentDir: agentDevDir, OwnerDir: devDir, AgentSID: guestsSID}

	got, err := FindRepositories(context.Background(), rules)

	if err != nil || len(got.Repositories) != 1 || got.Repositories[0].Name != "org/repo" {
		t.Fatalf("got %v, %v, want only org/repo", got, err)
	}
	if len(got.Refusals) != 1 || got.Refusals[0].Name != "plain" ||
		got.Refusals[0].Summary() != "not write-denied to the agent" {
		t.Fatalf("got refusals %v, want plain refused as %v", got.Refusals, ErrGitDirUnprotected)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	output, err := exec.Command("git", "init", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, output)
	}
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
