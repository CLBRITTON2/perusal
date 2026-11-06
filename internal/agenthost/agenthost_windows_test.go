package agenthost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			t.Run("waitForChangeReportsAWriteToTheFileOrUnderADirButNotASnapshot", func(t *testing.T) {
				waitForChangeReportsAWriteToTheFileOrUnderADirButNotASnapshot(t, shellName)
			})
		})
	}
}

func waitForChangeReportsAWriteToTheFileOrUnderADirButNotASnapshot(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	focusFile := filepath.Join(t.TempDir(), "focus")
	workTree := t.TempDir()
	snapshotRefs := filepath.Join(workTree, ".git", "refs", "perusal")
	if err := os.MkdirAll(snapshotRefs, 0o755); err != nil {
		t.Fatal(err)
	}
	waitForChange := func(write func()) bool {
		t.Helper()
		time.AfterFunc(300*time.Millisecond, write)
		changed, err := session.WaitForChange(context.Background(), []string{workTree}, focusFile, 3*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return changed
	}
	writeTo := func(path string) func() {
		return func() {
			if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
				t.Error(err)
			}
		}
	}

	first := waitForChange(func() {})
	snapshot := waitForChange(writeTo(filepath.Join(snapshotRefs, "snapshot")))
	focus := waitForChange(writeTo(focusFile))
	edit := waitForChange(writeTo(filepath.Join(workTree, "a.txt")))

	if !first || snapshot || !focus || !edit {
		t.Fatalf("got first wait %t, snapshot %t, focus file %t, edit %t, want only the snapshot unchanged", first,
			snapshot, focus, edit)
	}
}

func runsEveryScriptInOneShell(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")

	output := scriptOutput(t, session, "'grüße'; [string]$PID; Write-Warning 'kept out of the output'")

	if output != "grüße\r\n"+firstPID {
		t.Fatalf("got %q, want the text and the first script's process %s", output, firstPID)
	}
}

func returnsAThrownErrorWithItsStderrAndKeepsTheShell(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")

	output, err := session.Run(context.Background(),
		"'partial'; $null = git -C "+QuotedString(t.TempDir())+" rev-parse HEAD; throw 'stopped here'")

	if err == nil || !strings.Contains(err.Error(), "stopped here") || !strings.Contains(err.Error(), "fatal:") ||
		string(output) != "partial\r\n" {
		t.Fatalf("got %q and error %v, want the output before the throw and an error with its message and git's stderr",
			output, err)
	}
	if pid := scriptOutput(t, session, "[string]$PID"); pid != firstPID {
		t.Fatalf("got process %s after the error, want the same shell %s", pid, firstPID)
	}
}

func startsANewShellAfterTheShellExits(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")

	_, err := session.Run(context.Background(), "[Environment]::Exit(3)")

	if err == nil || !strings.Contains(err.Error(), "the shell exited") {
		t.Fatalf("got error %v, want one saying the shell exited", err)
	}
	if pid := scriptOutput(t, session, "[string]$PID"); pid == firstPID {
		t.Fatalf("got process %s after the exit, want a new shell", pid)
	}
}

func startsANewShellAfterAScriptTimesOut(t *testing.T, shellName string) {
	session := localSession(t, shellName)
	firstPID := scriptOutput(t, session, "[string]$PID")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := session.Run(ctx, "Start-Sleep -Seconds 30")

	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("got error %v, want the deadline", err)
	}
	if pid := scriptOutput(t, session, "[string]$PID"); pid == firstPID {
		t.Fatalf("got process %s after the timeout, want a new shell", pid)
	}
}

func refusesAResultOverOneMebibyte(t *testing.T, shellName string) {
	session := localSession(t, shellName)

	_, err := session.Run(context.Background(), "'x' * 2MB")

	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("got error %v, want the result refused as too long", err)
	}
	scriptOutput(t, session, "'still runs'")
}

// A local shell stands in for the ssh login shell, which runs shellLoop the same way.
func localSession(t *testing.T, shellName string) *Session {
	t.Helper()
	session := newSession("local", func() *exec.Cmd {
		return exec.Command(shellName, "-NoProfile", "-NonInteractive", "-Command", shellLoop)
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	return session
}

func scriptOutput(t *testing.T, session *Session, script string) string {
	t.Helper()
	output, err := session.Run(context.Background(), script)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(output))
}
