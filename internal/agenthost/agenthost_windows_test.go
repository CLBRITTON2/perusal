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
	ctx := context.Background()
	waitForChange := func(write func()) []string {
		t.Helper()
		time.AfterFunc(300*time.Millisecond, write)
		changedDirs, err := session.WaitForChangedDirs(ctx, dirs, focusFile, 3*time.Second)
		require.NoError(t, err)
		return changedDirs
	}
	writeTo := func(path string) func() {
		return func() {
			err := os.WriteFile(path, []byte("x"), 0o644)
			assert.NoError(t, err)
		}
	}
	writeObject := writeTo(filepath.Join(objects, "object"))
	writeOtherWorkTreeFile := writeTo(filepath.Join(otherWorkTree, "a.txt"))
	writeWorkTreeFile := writeTo(filepath.Join(workTree, "a.txt"))

	firstChanged := waitForChange(func() {})
	objectChanged := waitForChange(writeObject)
	otherWorkTreeChanged := waitForChange(writeOtherWorkTreeFile)
	workTreeChanged := waitForChange(writeWorkTreeFile)

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

func scriptOutput(t *testing.T, session *Session, script string) string {
	t.Helper()
	ctx := context.Background()
	output, err := session.Run(ctx, script)
	require.NoError(t, err)
	return strings.TrimSpace(string(output))
}
