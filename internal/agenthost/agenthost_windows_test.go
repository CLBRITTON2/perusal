package agenthost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// git writes a linked work tree's paths in long form, so a short (8.3) TMP would make a clone's watched path differ
// from the one its linked work trees name.
func TestMain(m *testing.M) {
	tempDir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("TMP", tempDir); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestSession(t *testing.T) {
	for _, shellName := range []string{"powershell.exe", "pwsh.exe"} {
		t.Run(shellName, func(t *testing.T) {
			t.Run("runsEveryScriptInOneShell", func(t *testing.T) { runsEveryScriptInOneShell(t, shellName) })
			t.Run("returnsAThrownErrorWithItsStderrAndKeepsTheShell", func(t *testing.T) {
				returnsAThrownErrorWithItsStderrAndKeepsTheShell(t, shellName)
			})
			t.Run("startsANewShellAfterTheShellExits", func(t *testing.T) {
				startsANewShellAfterTheShellExits(t, shellName)
			})
			t.Run("startsANewShellAfterAScriptTimesOut", func(t *testing.T) {
				startsANewShellAfterAScriptTimesOut(t, shellName)
			})
			t.Run("refusesAResultOverOneMebibyte", func(t *testing.T) { refusesAResultOverOneMebibyte(t, shellName) })
			t.Run("waitForChangedDirsReturnsEveryDirOnANewWatchThenOnlyTheDirWrittenOutsideGitObjects",
				func(t *testing.T) {
					waitForChangedDirsReturnsEveryDirOnANewWatchThenOnlyTheDirWrittenOutsideGitObjects(t, shellName)
				})
			t.Run("waitForChangedDirsEndsOnAWriteToTheFileWithNoDirChanged", func(t *testing.T) {
				waitForChangedDirsEndsOnAWriteToTheFileWithNoDirChanged(t, shellName)
			})
			t.Run("waitForChangedDirsReturnsOnlyTheLinkedWorkTreeCommittedIn", func(t *testing.T) {
				waitForChangedDirsReturnsOnlyTheLinkedWorkTreeCommittedIn(t, shellName)
			})
			t.Run("waitForChangedDirsReturnsTheLinkedWorkTreeWhoseBranchMovedInItsClone", func(t *testing.T) {
				waitForChangedDirsReturnsTheLinkedWorkTreeWhoseBranchMovedInItsClone(t, shellName)
			})
			t.Run("waitForChangedDirsReturnsNoDirWhenABranchNoDirHasCheckedOutMoves", func(t *testing.T) {
				waitForChangedDirsReturnsNoDirWhenABranchNoDirHasCheckedOutMoves(t, shellName)
			})
		})
	}
}

func waitForChangedDirsReturnsEveryDirOnANewWatchThenOnlyTheDirWrittenOutsideGitObjects(t *testing.T,
	shellName string) {
	session := localSession(t, shellName)
	focusFile := filepath.Join(t.TempDir(), "focus")
	workTree := t.TempDir()
	otherWorkTree := t.TempDir()
	missingWorkTree := filepath.Join(t.TempDir(), "missing")
	dirs := []string{workTree, otherWorkTree, missingWorkTree}
	objects := filepath.Join(workTree, ".git", "objects")
	err := os.MkdirAll(objects, 0o755)
	require.NoError(t, err)
	writeTo := func(path string) func() {
		return func() {
			err := os.WriteFile(path, []byte("x"), 0o644)
			assert.NoError(t, err)
		}
	}
	writeObject := writeTo(filepath.Join(objects, "object"))
	writeOtherWorkTreeFile := writeTo(filepath.Join(otherWorkTree, "a.txt"))
	writeWorkTreeFile := writeTo(filepath.Join(workTree, "a.txt"))

	firstChanged := changedDirsAfter(t, session, dirs, focusFile, func() {})
	objectChanged := changedDirsAfter(t, session, dirs, focusFile, writeObject)
	otherWorkTreeChanged := changedDirsAfter(t, session, dirs, focusFile, writeOtherWorkTreeFile)
	workTreeChanged := changedDirsAfter(t, session, dirs, focusFile, writeWorkTreeFile)

	require.Equal(t, dirs, firstChanged)
	require.Empty(t, objectChanged)
	require.Equal(t, []string{otherWorkTree}, otherWorkTreeChanged)
	require.Equal(t, []string{workTree}, workTreeChanged)
}

func waitForChangedDirsEndsOnAWriteToTheFileWithNoDirChanged(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	focusFile := filepath.Join(t.TempDir(), "focus")
	dirs := []string{t.TempDir()}
	ctx := context.Background()
	_, err := session.WaitForChangedDirs(ctx, dirs, focusFile, 3*time.Second)
	require.NoError(t, err)
	time.AfterFunc(300*time.Millisecond, func() {
		err := os.WriteFile(focusFile, []byte("x"), 0o644)
		assert.NoError(t, err)
	})
	started := time.Now()

	changedDirs, err := session.WaitForChangedDirs(ctx, dirs, focusFile, 10*time.Second)

	waited := time.Since(started)
	require.NoError(t, err)
	require.Empty(t, changedDirs)
	require.Less(t, waited, 5*time.Second)
}

func waitForChangedDirsReturnsOnlyTheLinkedWorkTreeCommittedIn(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	clone, sideWorkTree := cloneWithSideWorkTree(t)
	dirs := []string{clone, sideWorkTree}
	focusFile := filepath.Join(t.TempDir(), "focus")
	changedDirsAfter(t, session, dirs, focusFile, func() {})

	changedDirs := changedDirsAfter(t, session, dirs, focusFile, func() {
		gitOutput(t, sideWorkTree, "commit", "--allow-empty", "--message", "side")
	})

	require.Equal(t, []string{sideWorkTree}, changedDirs)
}

func waitForChangedDirsReturnsTheLinkedWorkTreeWhoseBranchMovedInItsClone(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	clone, sideWorkTree := cloneWithSideWorkTree(t)
	dirs := []string{sideWorkTree}
	focusFile := filepath.Join(t.TempDir(), "focus")
	gitOutput(t, clone, "pack-refs", "--all")
	commit := gitOutput(t, clone, "commit-tree", "-m", "moved", "HEAD^{tree}")
	changedDirsAfter(t, session, dirs, focusFile, func() {})

	changedDirs := changedDirsAfter(t, session, dirs, focusFile, func() {
		gitOutput(t, clone, "update-ref", "refs/heads/side", commit)
	})

	require.Equal(t, []string{sideWorkTree}, changedDirs)
}

func waitForChangedDirsReturnsNoDirWhenABranchNoDirHasCheckedOutMoves(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	clone, sideWorkTree := cloneWithSideWorkTree(t)
	dirs := []string{clone, sideWorkTree}
	focusFile := filepath.Join(t.TempDir(), "focus")
	changedDirsAfter(t, session, dirs, focusFile, func() {})

	changedDirs := changedDirsAfter(t, session, dirs, focusFile, func() {
		gitOutput(t, clone, "branch", "unpaired")
		gitOutput(t, clone, "pack-refs", "--all")
	})

	require.Empty(t, changedDirs)
}

func runsEveryScriptInOneShell(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")

	output := scriptOutput(t, session, "'grüße'; [string]$PID; Write-Warning 'kept out of the output'")

	require.Equal(t, "grüße\r\n"+firstPID, output)
}

func returnsAThrownErrorWithItsStderrAndKeepsTheShell(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")
	quotedEmptyDir := QuotedString(t.TempDir())
	ctx := context.Background()

	output, err := session.Run(ctx, "'partial'; $null = git -C "+quotedEmptyDir+" rev-parse HEAD; throw 'stopped here'")

	require.ErrorContains(t, err, "stopped here")
	require.ErrorContains(t, err, "fatal:")
	require.Equal(t, "partial\r\n", string(output))
	pid := scriptOutput(t, session, "[string]$PID")
	require.Equal(t, firstPID, pid)
}

func startsANewShellAfterTheShellExits(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")
	ctx := context.Background()

	_, err := session.Run(ctx, "[Environment]::Exit(3)")

	require.ErrorContains(t, err, "the shell exited")
	pid := scriptOutput(t, session, "[string]$PID")
	require.NotEqual(t, firstPID, pid)
}

func startsANewShellAfterAScriptTimesOut(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := session.Run(ctx, "Start-Sleep -Seconds 30")

	require.ErrorContains(t, err, "deadline exceeded")
	pid := scriptOutput(t, session, "[string]$PID")
	require.NotEqual(t, firstPID, pid)
}

func refusesAResultOverOneMebibyte(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	ctx := context.Background()

	_, err := session.Run(ctx, "'x' * 2MB")

	require.ErrorContains(t, err, "too long")
	output := scriptOutput(t, session, "'still runs'")
	require.Equal(t, "still runs", output)
}

// A local shell stands in for the ssh login shell, which runs shellLoop the same way.
func localSession(t *testing.T, shellName string) *Session {
	t.Helper()
	session := newSession("local", func() *exec.Cmd {
		return exec.Command(shellName, "-NoProfile", "-NonInteractive", "-Command", shellLoop)
	})
	t.Cleanup(func() {
		err := session.Close()
		require.NoError(t, err)
	})
	return session
}

func changedDirsAfter(t *testing.T, session *Session, dirs []string, focusFile string, write func()) []string {
	t.Helper()
	time.AfterFunc(300*time.Millisecond, write)
	changedDirs, err := session.WaitForChangedDirs(context.Background(), dirs, focusFile, 3*time.Second)
	require.NoError(t, err)
	return changedDirs
}

func cloneWithSideWorkTree(t *testing.T) (string, string) {
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
	clone := filepath.Join(parent, "clone")
	sideWorkTree := filepath.Join(parent, "side")
	gitOutput(t, parent, "init", "--initial-branch", "main", clone)
	gitOutput(t, clone, "commit", "--allow-empty", "--message", "first")
	gitOutput(t, clone, "worktree", "add", "-b", "side", sideWorkTree)
	return clone, sideWorkTree
}

// Called inside time.AfterFunc, so it checks with assert.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	assert.NoError(t, err, string(output))
	return strings.TrimSpace(string(output))
}

func scriptOutput(t *testing.T, session *Session, script string) string {
	t.Helper()
	ctx := context.Background()
	output, err := session.Run(ctx, script)
	require.NoError(t, err)
	return strings.TrimSpace(string(output))
}
