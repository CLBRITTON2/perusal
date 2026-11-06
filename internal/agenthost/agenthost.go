// Package agenthost runs commands on the machine the agent works in, over ssh.
package agenthost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// shellLoop runs each line read from stdin as a script in a child scope and writes its shellResult as one line of
// JSON. Strings the script outputs are its output, any other record (an error, native stderr, a warning) its stderr.
// It holds no double quote, since sshd hands it to the login shell's -c inside one.
var shellLoop = strings.Join([]string{
	"$perusalUtf8 = New-Object Text.UTF8Encoding $false",
	"$perusalIn = New-Object IO.StreamReader -ArgumentList ([Console]::OpenStandardInput()), $perusalUtf8",
	"$perusalOut = New-Object IO.StreamWriter -ArgumentList ([Console]::OpenStandardOutput()), $perusalUtf8",
	"$perusalOut.AutoFlush = $true",
	"while ($null -ne ($perusalScript = $perusalIn.ReadLine())) { " +
		"$perusalState = @{ failure = '' }; " +
		"$perusalRecords = @(& { try { & ([scriptblock]::Create($perusalScript)) } " +
		"catch { $perusalState.failure = $_.Exception.Message } } *>&1); " +
		"$perusalOutput = -join @($perusalRecords | Where-Object { $_ -is [string] } | " +
		"ForEach-Object { $_ + [Environment]::NewLine }); " +
		"$perusalStderr = @($perusalRecords | Where-Object { $_ -isnot [string] } | " +
		"ForEach-Object { $_.ToString() }) -join [Environment]::NewLine; " +
		"$perusalOut.WriteLine((ConvertTo-Json -Compress -InputObject @{ output = $perusalOutput; " +
		"stderr = $perusalStderr; failure = $perusalState.failure })) }",
}, "; ")

// The JSON line shellLoop writes for each script. Failure is the message of the error that stopped it.
type shellResult struct {
	Output  string `json:"output"`
	Stderr  string `json:"stderr"`
	Failure string `json:"failure"`
}

// Session runs scripts one at a time in a single PowerShell (5.1 or 7) on a host, so a script costs no ssh logon and no
// shell start. It starts the shell on the first Run and again after any Run that leaves the shell in an unknown state.
type Session struct {
	host       string
	startShell func() *exec.Cmd
	lock       sync.Mutex
	shell      *shellProcess
}

// NewSession runs scripts in host's ssh login shell, which must be PowerShell. Close it when done.
func NewSession(host string) *Session {
	return newSession(host, func() *exec.Cmd {
		// BatchMode makes ssh fail instead of asking for a passphrase or a host key over the TUI. The connection stays
		// open as long as the session, so no forwarding from ~/.ssh/config may hand the agent's host a way back.
		return exec.Command("ssh", "-o", "BatchMode=yes", "-a", "-x", "-o", "ClearAllForwardings=yes", "-T", host,
			shellLoop)
	})
}

func newSession(host string, startShell func() *exec.Cmd) *Session {
	return &Session{host: host, startShell: startShell, shell: nil}
}

// Run runs script, which must be one line and print only strings, and returns what it printed. A script that throws
// returns its output and an error with the message and its stderr. The error leaves script out, so callers name the
// step.
func (session *Session) Run(ctx context.Context, script string) ([]byte, error) {
	if strings.ContainsAny(script, "\r\n") {
		return nil, fmt.Errorf("ssh %s: a script must be one line", session.host)
	}
	session.lock.Lock()
	defer session.lock.Unlock()
	if session.shell == nil {
		started, err := startShellProcess(session.startShell())
		if err != nil {
			return nil, fmt.Errorf("ssh %s: %w", session.host, err)
		}
		session.shell = started
	}
	result, err := session.shell.run(ctx, script)
	if err != nil {
		stopErr := session.shell.stop()
		runErr := fmt.Errorf("ssh %s: %w (exit: %v): %s", session.host, err, session.shell.exit,
			session.shell.stderr.String())
		session.shell = nil
		return nil, errors.Join(runErr, stopErr)
	}
	if result.Failure != "" {
		return []byte(result.Output), fmt.Errorf("ssh %s: %s: %s", session.host, result.Failure,
			strings.TrimSpace(result.Stderr))
	}
	return []byte(result.Output), nil
}

// WaitForChange waits up to timeout for a change to file or to anything under one of dirs, all Windows paths on the
// host. The first wait on a new set of paths, as in a new shell, reports a change, since nothing watched before it.
// Changes inside a .git's objects, logs and refs\perusal do not count, since a snapshot writes them. A watcher whose
// buffer overflowed counts as a change. A dir missing on the host is not watched.
func (session *Session) WaitForChange(ctx context.Context, dirs []string, file string,
	timeout time.Duration) (bool, error) {
	quotedDirs := make([]string, len(dirs))
	for index, dir := range dirs {
		quotedDirs[index] = QuotedString(dir)
	}
	separator := strings.LastIndexAny(file, `\/`)
	if separator < 0 {
		return false, fmt.Errorf("ssh %s: watch %s: want a full path", session.host, file)
	}
	// Events queue in the shell between waits, so a change made while a snapshot ran still counts.
	script := strings.Join([]string{
		"$watchKey = " + QuotedString(file+"|"+strings.Join(dirs, "|")),
		"$changed = $global:perusalWatchKey -ne $watchKey",
		"if ($changed) { " +
			"Get-EventSubscriber | Where-Object SourceIdentifier -like 'perusal-*' | Unregister-Event; " +
			"Get-Event | Remove-Event; " +
			"@($global:perusalWatchers) | Where-Object { $null -ne $_ } | ForEach-Object { $_.Dispose() }; " +
			"$fileWatcher = New-Object IO.FileSystemWatcher -ArgumentList " + QuotedString(file[:separator]) + ", " +
			QuotedString(file[separator+1:]) + "; " +
			"$dirWatchers = @(foreach ($dir in @(" + strings.Join(quotedDirs, ", ") + ")) { " +
			"if (Test-Path -LiteralPath $dir -PathType Container) { New-Object IO.FileSystemWatcher -ArgumentList $dir " +
			"-Property @{ IncludeSubdirectories = $true; InternalBufferSize = 65536 } } }); " +
			"$global:perusalWatchers = @($fileWatcher) + $dirWatchers; " +
			"foreach ($watcher in $global:perusalWatchers) { " +
			"foreach ($eventName in 'Changed', 'Created', 'Deleted', 'Renamed', 'Error') { " +
			"$null = Register-ObjectEvent -InputObject $watcher -EventName $eventName " +
			"-SourceIdentifier ('perusal-' + [guid]::NewGuid()) }; $watcher.EnableRaisingEvents = $true }; " +
			"$global:perusalWatchKey = $watchKey }",
		fmt.Sprintf("$deadline = (Get-Date).AddMilliseconds(%d)", timeout.Milliseconds()),
		"while (-not $changed) { " +
			"$seconds = [int][Math]::Ceiling(($deadline - (Get-Date)).TotalSeconds); if ($seconds -le 0) { break }; " +
			"if ($null -eq (Wait-Event -Timeout $seconds)) { break }; " +
			// Lets the rest of a burst, such as a save or a checkout, land before the snapshot.
			"Start-Sleep -Milliseconds 200; " +
			"foreach ($fileEvent in @(Get-Event)) { Remove-Event -EventIdentifier $fileEvent.EventIdentifier; " +
			"$sourceArgs = $fileEvent.SourceEventArgs; " +
			`if ($sourceArgs -is [IO.ErrorEventArgs] -or ` +
			`$sourceArgs.FullPath -notmatch '\\\.git\\(objects|logs|refs\\perusal)(\\|$)') { $changed = $true } } }`,
		"if ($changed) { 'changed' } else { 'unchanged' }",
	}, "; ")
	output, err := session.Run(ctx, script)
	if err != nil {
		return false, fmt.Errorf("watch %s and %d work trees: %w", file, len(dirs), err)
	}
	switch strings.TrimSpace(string(output)) {
	case "changed":
		return true, nil
	case "unchanged":
		return false, nil
	}
	return false, fmt.Errorf("ssh %s: watch %s: want changed or unchanged, got %q", session.host, file, output)
}

// Close stops the shell, if one is running.
func (session *Session) Close() error {
	session.lock.Lock()
	defer session.lock.Unlock()
	if session.shell == nil {
		return nil
	}
	err := session.shell.stop()
	session.shell = nil
	if err != nil {
		return fmt.Errorf("ssh %s: %w", session.host, err)
	}
	return nil
}

type shellProcess struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	lines   <-chan shellLine
	stderr  *stderrTail
	// Closed by stop, so the reader stops waiting to hand over a line.
	stopped chan struct{}
	// How the process ended, set by stop.
	exit *exec.ExitError
}

// shellLine is one line of the shell's stdout, or the error that ended reading it.
type shellLine struct {
	text []byte
	err  error
}

func startShellProcess(command *exec.Cmd) (*shellProcess, error) {
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open the shell's stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open the shell's stdout: %w", err)
	}
	stderr := &stderrTail{lock: sync.Mutex{}, text: []byte{}}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command.Path, err)
	}
	lines := make(chan shellLine)
	stopped := make(chan struct{})
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		// The agent's host controls what comes back, so one result may not grow without bound.
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := shellLine{text: append([]byte{}, scanner.Bytes()...), err: nil}
			select {
			case lines <- line:
			case <-stopped:
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case lines <- shellLine{text: nil, err: err}:
			case <-stopped:
			}
		}
	}()
	return &shellProcess{command: command, stdin: stdin, lines: lines, stderr: stderr, stopped: stopped, exit: nil},
		nil
}

func (shell *shellProcess) run(ctx context.Context, script string) (shellResult, error) {
	// A hung host would otherwise stall the caller for good.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := io.WriteString(shell.stdin, script+"\n"); err != nil {
		return shellResult{}, fmt.Errorf("send a script to the shell: %w", err)
	}
	select {
	case <-ctx.Done():
		return shellResult{}, fmt.Errorf("wait for the shell: %w", ctx.Err())
	case line, open := <-shell.lines:
		switch {
		case !open:
			return shellResult{}, errors.New("the shell exited")
		case line.err != nil:
			return shellResult{}, fmt.Errorf("read the shell's result: %w", line.err)
		}
		var result shellResult
		if err := json.Unmarshal(line.text, &result); err != nil {
			return shellResult{}, fmt.Errorf("read the shell's result %q: %w", line.text, err)
		}
		return result, nil
	}
}

// stop kills the shell, waits for it and keeps its exit status in exit. An exit status is what the kill does, so only a
// failure to kill or wait is an error.
func (shell *shellProcess) stop() error {
	close(shell.stopped)
	if err := shell.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("kill the shell: %w", err)
	}
	err := shell.command.Wait()
	if !errors.As(err, &shell.exit) && err != nil {
		return fmt.Errorf("wait for the shell: %w", err)
	}
	return nil
}

// stderrTail keeps the last 4 KiB written to it, since the agent's host controls how much it writes.
type stderrTail struct {
	lock sync.Mutex
	text []byte
}

func (tail *stderrTail) Write(data []byte) (int, error) {
	tail.lock.Lock()
	defer tail.lock.Unlock()
	tail.text = append(tail.text, data...)
	if excess := len(tail.text) - 4096; excess > 0 {
		tail.text = tail.text[excess:]
	}
	return len(data), nil
}

func (tail *stderrTail) String() string {
	tail.lock.Lock()
	defer tail.lock.Unlock()
	return strings.TrimSpace(string(tail.text))
}

// QuotedString quotes text as a PowerShell single-quoted string.
func QuotedString(text string) string {
	return "'" + strings.ReplaceAll(text, "'", "''") + "'"
}
