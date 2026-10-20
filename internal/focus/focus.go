//go:build windows

// Package focus maps the agent's focused herdr pane to a repository the owner can safely run git in.
package focus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const maximumFocusFileBytes = 4096

var (
	ErrMalformedFocusFile = errors.New("the focus file must hold one absolute path")
	ErrOutsideAgentDir    = errors.New("the path is outside the agent directory")
	ErrNotWorkTree        = errors.New("no .git between the path and the owner directory")
	ErrGitDirUnprotected  = errors.New("the .git directory is not write-denied to the agent account")
	ErrWorkTreeMismatch   = errors.New("git resolved a different work tree than the one checked")
)

// AgentDir and OwnerDir are one directory, as each account reaches it.
type Rules struct {
	AgentDir string
	OwnerDir string
	AgentSID string
}

type Result struct {
	Repository gitrepo.Repository
	Err        error
}

func Follow(ctx context.Context, rules Rules, focusFile string) (<-chan Result, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create a watcher for focus file %s: %w", focusFile, err)
	}
	// The publisher replaces the file instead of writing into it, which a watch on the file itself does not survive.
	if err := watcher.Add(filepath.Dir(focusFile)); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch the directory of focus file %s: %w", focusFile, err)
	}
	results := make(chan Result)
	go watchFocusFile(ctx, watcher, rules, focusFile, results)
	return results, nil
}

func watchFocusFile(ctx context.Context, watcher *fsnotify.Watcher, rules Rules, focusFile string, results chan<- Result) {
	defer close(results)
	defer watcher.Close()
	if !sendResult(ctx, results, resolveResult(ctx, rules, focusFile)) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-watcher.Events:
			if !open {
				return
			}
			if !isWriteTo(event, focusFile) {
				continue
			}
			if !sendResult(ctx, results, resolveResult(ctx, rules, focusFile)) {
				return
			}
		case err, open := <-watcher.Errors:
			if !open {
				return
			}
			if !sendResult(ctx, results, Result{Err: fmt.Errorf("watch focus file %s: %w", focusFile, err)}) {
				return
			}
		}
	}
}

func isWriteTo(event fsnotify.Event, focusFile string) bool {
	return event.Op&(fsnotify.Create|fsnotify.Write) != 0 && strings.EqualFold(filepath.Clean(event.Name), filepath.Clean(focusFile))
}

func sendResult(ctx context.Context, results chan<- Result, result Result) bool {
	select {
	case <-ctx.Done():
		return false
	case results <- result:
		return true
	}
}

func resolveResult(ctx context.Context, rules Rules, focusFile string) Result {
	repository, err := Resolve(ctx, rules, focusFile)
	return Result{Repository: repository, Err: err}
}

// The owner's git runs the config and hooks of a .git, so one the agent could write is refused.
func Resolve(ctx context.Context, rules Rules, focusFile string) (gitrepo.Repository, error) {
	content, err := readFocusFile(focusFile)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	agentPath, err := ParseFocusFile(content)
	if err != nil {
		return gitrepo.Repository{}, fmt.Errorf("parse focus file %s: %w", focusFile, err)
	}
	ownerPath, err := MapPath(agentPath, rules.AgentDir, rules.OwnerDir)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	workTree, err := FindWorkTree(ownerPath, rules.OwnerDir)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	if err := VerifyGitDir(filepath.Join(workTree, ".git"), rules.AgentSID); err != nil {
		return gitrepo.Repository{}, err
	}
	repository, err := gitrepo.Open(ctx, workTree)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	// git skips a .git it cannot use and keeps looking further up, past the directory that was checked.
	same, err := isSameDirectory(repository.Dir, workTree)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	if !same {
		return gitrepo.Repository{}, fmt.Errorf("%w: %s resolved to %s instead of %s", ErrWorkTreeMismatch, ownerPath, repository.Dir, workTree)
	}
	return repository, nil
}

// isSameDirectory compares file identity, since git reports a path through a junction by its target.
func isSameDirectory(first string, second string) (bool, error) {
	firstInfo, err := os.Stat(first)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", first, err)
	}
	secondInfo, err := os.Stat(second)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", second, err)
	}
	return os.SameFile(firstInfo, secondInfo), nil
}

func readFocusFile(focusFile string) (string, error) {
	file, err := openSharingDelete(focusFile)
	if err != nil {
		return "", fmt.Errorf("open focus file %s: %w", focusFile, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maximumFocusFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("read focus file %s: %w", focusFile, err)
	}
	if len(content) > maximumFocusFileBytes {
		return "", fmt.Errorf("%w: focus file %s is over %d bytes", ErrMalformedFocusFile, focusFile, maximumFocusFileBytes)
	}
	return string(content), nil
}

// A trailing line break is allowed so a hand-written file works.
func ParseFocusFile(content string) (string, error) {
	path := strings.TrimRight(content, "\r\n")
	if path == "" || strings.ContainsAny(path, "\r\n") {
		return "", fmt.Errorf("%w, got %q", ErrMalformedFocusFile, content)
	}
	return path, nil
}

func MapPath(agentPath string, agentDir string, ownerDir string) (string, error) {
	cleaned := filepath.Clean(agentPath)
	root := filepath.Clean(agentDir)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%w: %s is not absolute", ErrOutsideAgentDir, agentPath)
	}
	if strings.EqualFold(cleaned, root) {
		return filepath.Clean(ownerDir), nil
	}
	prefix := root + string(filepath.Separator)
	if len(cleaned) <= len(prefix) || !strings.EqualFold(cleaned[:len(prefix)], prefix) {
		return "", fmt.Errorf("%w: %s is not under %s", ErrOutsideAgentDir, agentPath, agentDir)
	}
	return filepath.Join(ownerDir, cleaned[len(prefix):]), nil
}

func FindWorkTree(dir string, ownerDir string) (string, error) {
	root := filepath.Clean(ownerDir)
	current := filepath.Clean(dir)
	for {
		_, err := os.Lstat(filepath.Join(current, ".git"))
		if err == nil {
			return current, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("look for .git in %s: %w", current, err)
		}
		parent := filepath.Dir(current)
		if strings.EqualFold(current, root) || parent == current {
			return "", fmt.Errorf("%w: %s", ErrNotWorkTree, dir)
		}
		current = parent
	}
}
