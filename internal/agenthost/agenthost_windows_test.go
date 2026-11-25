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
			t.Run("waitForChangeReportsAWriteToTheFileOrUnderADirButNotToGitObjects", func(t *testing.T) {
				waitForChangeReportsAWriteToTheFileOrUnderADirButNotToGitObjects(t, shellName)
			})
		})
	}
}

func waitForChangeReportsAWriteToTheFileOrUnderADirButNotToGitObjects(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	focusFile := filepath.Join(t.TempDir(), "focus")
	workTree := t.TempDir()
	objects := filepath.Join(workTree, ".git", "objects")
	object := filepath.Join(objects, "object")
	workTreeFile := filepath.Join(workTree, "a.txt")
	err := os.MkdirAll(objects, 0o755)
	require.NoError(t, err)
	ctx := context.Background()
	waitForChange := func(write func()) bool {
		t.Helper()
		time.AfterFunc(300*time.Millisecond, write)
		changed, err := session.WaitForChange(ctx, []string{workTree}, focusFile, 3*time.Second)
		require.NoError(t, err)
		return changed
	}
	writeTo := func(path string) func() {
		return func() {
			err := os.WriteFile(path, []byte("x"), 0o644)
			assert.NoError(t, err)
		}
	}
	writeObject := writeTo(object)
	writeFocusFile := writeTo(focusFile)
	writeWorkTreeFile := writeTo(workTreeFile)

	firstChanged := waitForChange(func() {})
	objectChanged := waitForChange(writeObject)
	focusFileChanged := waitForChange(writeFocusFile)
	workTreeFileChanged := waitForChange(writeWorkTreeFile)

	require.True(t, firstChanged)
	require.False(t, objectChanged)
	require.True(t, focusFileChanged)
	require.True(t, workTreeFileChanged)
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
