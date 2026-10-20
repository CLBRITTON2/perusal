package focus

import (
	"errors"
	"os"
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
		{"refusesASiblingWithTheSamePrefix", `C:\Users\claude\devtools\x`, "", ErrOutsideAgentDevDir},
		{"refusesDotDotOutOfTheDevDirectory", `C:\Users\claude\dev\..\..\Chris\.ssh`, "", ErrOutsideAgentDevDir},
		{"refusesARelativePath", `dev\perusal`, "", ErrOutsideAgentDevDir},
		{"refusesADriveRelativePath", `C:dev\perusal`, "", ErrOutsideAgentDevDir},
		{"refusesAVerbatimPrefix", `\\?\C:\Users\claude\dev\perusal`, "", ErrOutsideAgentDevDir},
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
	t.Run("refusesAnEmptyFile", refusesAnEmptyFile)
	t.Run("refusesTwoLines", refusesTwoLines)
}

func allowsATrailingLineBreak(t *testing.T) {
	got, err := ParseFocusFile("C:\\Users\\claude\\dev\\perusal\r\n")

	if err != nil || got != `C:\Users\claude\dev\perusal` {
		t.Fatalf("got %q, %v", got, err)
	}
}

func refusesAnEmptyFile(t *testing.T) {
	_, err := ParseFocusFile("\n")

	if !errors.Is(err, ErrMalformedFocusFile) {
		t.Fatalf("got %v, want %v", err, ErrMalformedFocusFile)
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
	t.Run("stopsAtTheOwnerDevDirectory", stopsAtTheOwnerDevDirectory)
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

func stopsAtTheOwnerDevDirectory(t *testing.T) {
	outer := t.TempDir()
	mustMkdir(t, filepath.Join(outer, ".git"))
	devDir := filepath.Join(outer, "dev")
	mustMkdir(t, filepath.Join(devDir, "notes"))

	_, err := FindWorkTree(filepath.Join(devDir, "notes"), devDir)

	if !errors.Is(err, ErrNotWorkTree) {
		t.Fatalf("got %v, want %v", err, ErrNotWorkTree)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}
