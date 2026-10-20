package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunGit(t *testing.T) {
	t.Run("failureCarriesWhatGitPrintedOnStdout", failureCarriesWhatGitPrintedOnStdout)
}

func TestPush(t *testing.T) {
	t.Run("sendsTheBranchToItsUpstream", sendsTheBranchToItsUpstream)
}

func sendsTheBranchToItsUpstream(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	work := t.TempDir()
	steps := [][]string{
		{"init", "--bare", "--initial-branch=main", remote},
		{"init", "--initial-branch=main", work},
		{"-C", work, "-c", "user.name=perusal", "-c", "user.email=perusal@example.com", "-c", "commit.gpgsign=false",
			"commit", "--allow-empty", "--no-verify", "--message", "Start"},
		{"-C", work, "remote", "add", "origin", remote},
		{"-C", work, "config", "branch.main.remote", "origin"},
		{"-C", work, "config", "branch.main.merge", "refs/heads/main"},
	}
	for _, step := range steps {
		if _, err := runGit(context.Background(), work, step); err != nil {
			t.Fatal(err)
		}
	}

	err := Repository{Dir: work}.Push(context.Background())

	pushed, revParseErr := runGit(context.Background(), remote, []string{"rev-parse", "main"})
	head, headErr := runGit(context.Background(), work, []string{"rev-parse", "HEAD"})
	if err != nil || revParseErr != nil || headErr != nil || string(pushed) != string(head) {
		t.Fatalf("got push error %v, remote main %q (%v), want local HEAD %q (%v)", err, pushed, revParseErr, head, headErr)
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

	commits, err := ParseLog(output)
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
	_, err := ParseLog([]byte("--output=x\x1fa\x1f\x1f\x1fAda\x1f1700000000\x1fFix\x00"))

	if err == nil {
		t.Fatal("got no error for a record whose hash is not a full commit hash")
	}
}

func rejectsAParentThatIsNotAFullHash(t *testing.T) {
	_, err := ParseLog([]byte(strings.Repeat("a", 40) + "\x1faaaaaaa\x1f--all\x1f\x1fAda\x1f1700000000\x1fFix\x00"))

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

	_, err := runGit(context.Background(), dir, []string{"diff", "--no-index", "--exit-code", "--", "old.txt", "new.txt"})

	if err == nil || !strings.Contains(err.Error(), "+new line") {
		t.Fatalf("got error %v, want one holding the diff git printed on stdout", err)
	}
}
