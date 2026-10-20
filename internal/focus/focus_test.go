//go:build windows

package focus

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	t.Run("allowsATrailingLineBreak", allowsATrailingLineBreak)
	t.Run("refusesTwoLines", refusesTwoLines)
}

func allowsATrailingLineBreak(t *testing.T) {
	got, err := ParseFocusFile("C:\\Users\\claude\\dev\\perusal\r\n")

	if err != nil || got != `C:\Users\claude\dev\perusal` {
		t.Fatalf("got %q, %v", got, err)
	}
}

func refusesTwoLines(t *testing.T) {
	_, err := ParseFocusFile("C:\\Users\\claude\\dev\\a\nC:\\Users\\claude\\dev\\b")

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
}

const agentDevDir = `C:\agent\dev`

func opensTheCheckedWorkTree(t *testing.T) {
	devDir := t.TempDir()
	repository := filepath.Join(devDir, "repo")
	gitInit(t, repository)
	denyGuests(t, filepath.Join(repository, ".git"), deniedWriteMask)
	rules := Rules{AgentDir: agentDevDir, OwnerDir: devDir, AgentSID: guestsSID}

	got, err := Resolve(context.Background(), rules, focusFileHolding(t, agentDevDir+`\repo`))

	if err != nil {
		t.Fatal(err)
	}
	same, err := isSameDirectory(got.Dir, repository)
	if err != nil || !same {
		t.Fatalf("got %q, %v, want %q", got.Dir, err, repository)
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
