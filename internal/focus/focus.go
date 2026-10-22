//go:build windows

package focus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sys/windows"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

const maximumFocusFileBytes = 4096

type Rules struct {
	Config
	AgentSID *windows.SID
}

func NewRules(config Config) (Rules, error) {
	sid, _, accountType, err := windows.LookupSID("", config.AgentAccount)
	if err != nil {
		return Rules{}, fmt.Errorf("look up account %s: %w", config.AgentAccount, err)
	}
	if accountType != windows.SidTypeUser {
		return Rules{}, fmt.Errorf("account %s is not a user account (SID type %d)", config.AgentAccount, accountType)
	}
	return Rules{Config: config, AgentSID: sid}, nil
}

type Result struct {
	Focus Focus
	Err   error
}

func Follow(ctx context.Context, rules Rules) (<-chan Result, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create a watcher for focus file %s: %w", rules.FocusFile, err)
	}
	// The publisher replaces the file instead of writing into it, which a watch on the file itself does not survive.
	if err := watcher.Add(filepath.Dir(rules.FocusFile)); err != nil {
		watchErr := fmt.Errorf("watch the directory of focus file %s: %w", rules.FocusFile, err)
		return nil, errors.Join(watchErr, watcher.Close())
	}
	results := make(chan Result)
	go watchFocusFile(ctx, watcher, rules, results)
	return results, nil
}

func watchFocusFile(ctx context.Context, watcher *fsnotify.Watcher, rules Rules, results chan<- Result) {
	defer close(results)
	defer func() {
		if err := watcher.Close(); err != nil {
			sendResult(ctx, results, Result{Err: fmt.Errorf("stop watching focus file %s: %w", rules.FocusFile, err)})
		}
	}()
	if !sendResult(ctx, results, resolveResult(ctx, rules)) {
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
			if !isFocusFileUpdate(event, rules.FocusFile) {
				continue
			}
			if !sendResult(ctx, results, resolveResult(ctx, rules)) {
				return
			}
		case err, open := <-watcher.Errors:
			if !open {
				return
			}
			if !sendResult(ctx, results, Result{Err: fmt.Errorf("watch focus file %s: %w", rules.FocusFile, err)}) {
				return
			}
		}
	}
}

func isFocusFileUpdate(event fsnotify.Event, focusFile string) bool {
	return event.Op&(fsnotify.Create|fsnotify.Write) != 0 && isSamePath(event.Name, focusFile)
}

func sendResult(ctx context.Context, results chan<- Result, result Result) bool {
	select {
	case <-ctx.Done():
		return false
	case results <- result:
		return true
	}
}

func resolveResult(ctx context.Context, rules Rules) Result {
	focus, err := resolve(ctx, rules)
	return Result{Focus: focus, Err: err}
}

// resolve fails only when the focus file is unreadable or none of its paths passes the checks.
func resolve(ctx context.Context, rules Rules) (Focus, error) {
	content, err := readFocusFile(rules.FocusFile)
	if err != nil {
		return Focus{}, err
	}
	agentPaths, err := parseFocusFile(content)
	if err != nil {
		return Focus{}, fmt.Errorf("parse focus file %s: %w", rules.FocusFile, err)
	}
	repositories := []gitrepo.Repository{}
	refusals := []error{}
	for _, agentPath := range agentPaths {
		repository, err := resolvePath(ctx, rules, agentPath)
		switch {
		case err != nil:
			refusals = append(refusals, err)
		case !slices.Contains(repositories, repository):
			repositories = append(repositories, repository)
		}
	}
	if len(repositories) == 0 {
		return Focus{}, errors.Join(refusals...)
	}
	return Focus{Repositories: repositories, Refusals: refusals}, nil
}

func resolvePath(ctx context.Context, rules Rules, agentPath string) (gitrepo.Repository, error) {
	ownerPath, err := mapPath(agentPath, rules.AgentDir, rules.OwnerDir)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	workTree, err := findWorkTree(ownerPath, rules.OwnerDir)
	if err != nil {
		return gitrepo.Repository{}, err
	}
	return openWorkTree(ctx, rules.AgentSID, workTree)
}

// FindRepositories lists every work tree under the owner directory, checked as resolve checks a focused one. It does
// not look inside a work tree, so a nested one is not listed.
func FindRepositories(ctx context.Context, rules Rules) (Found, error) {
	found := Found{Repositories: []FoundRepository{}, Refusals: []FoundRefusal{}}
	err := filepath.WalkDir(rules.OwnerDir, func(path string, entry fs.DirEntry, listErr error) error {
		switch {
		case entry == nil: // WalkDir passes a nil entry only when the root's Lstat fails
			return fmt.Errorf("list owner directory %s: %w", rules.OwnerDir, listErr)
		case listErr == nil && !entry.IsDir():
			return nil
		}
		name, err := nameBelow(rules.OwnerDir, path)
		if err != nil {
			return err
		}
		repository, isWorkTree, err := openIfWorkTree(ctx, rules.AgentSID, path, listErr)
		switch {
		case err != nil:
			found.Refusals = append(found.Refusals, FoundRefusal{RelativePath: name, Err: err})
			return filepath.SkipDir
		case !isWorkTree:
			return nil
		}
		found.Repositories = append(found.Repositories, FoundRepository{RelativePath: name, Repository: repository})
		return filepath.SkipDir
	})
	if err != nil {
		return Found{}, err
	}
	return found, nil
}

// listErr is the error WalkDir hit listing dir.
func openIfWorkTree(ctx context.Context, agentSID *windows.SID, dir string, listErr error) (
	gitrepo.Repository, bool, error,
) {
	if listErr != nil {
		return gitrepo.Repository{}, false, fmt.Errorf("list %s: %w", dir, listErr)
	}
	isWorkTree, err := hasDotGit(dir)
	if err != nil || !isWorkTree {
		return gitrepo.Repository{}, false, err
	}
	repository, err := openWorkTree(ctx, agentSID, dir)
	if err != nil {
		return gitrepo.Repository{}, false, err
	}
	return repository, true, nil
}

func nameBelow(ownerDir string, path string) (string, error) {
	name, err := filepath.Rel(ownerDir, path)
	if err != nil {
		return "", fmt.Errorf("name %s below %s: %w", path, ownerDir, err)
	}
	return filepath.ToSlash(name), nil
}

// The owner's git runs the config and hooks of a .git, so one the agent could write is refused.
func openWorkTree(ctx context.Context, agentSID *windows.SID, workTree string) (gitrepo.Repository, error) {
	if err := verifyGitDir(filepath.Join(workTree, ".git"), agentSID); err != nil {
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
		return gitrepo.Repository{}, fmt.Errorf("%w: git opened %s as %s", ErrWorkTreeMismatch, workTree, repository.Dir)
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

// isSamePath compares spelling, case-insensitively as Windows does.
func isSamePath(first string, second string) bool {
	return strings.EqualFold(filepath.Clean(first), filepath.Clean(second))
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
func parseFocusFile(content string) ([]string, error) {
	trimmed := strings.TrimRight(content, "\r\n")
	if trimmed == "" {
		return nil, fmt.Errorf("%w, got %q", ErrMalformedFocusFile, content)
	}
	paths := []string{}
	for line := range strings.SplitSeq(trimmed, "\n") {
		path := strings.TrimSuffix(line, "\r")
		if path == "" {
			return nil, fmt.Errorf("%w, got an empty line in %q", ErrMalformedFocusFile, content)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func mapPath(agentPath string, agentDir string, ownerDir string) (string, error) {
	cleanAgentPath := filepath.Clean(agentPath)
	agentRoot := filepath.Clean(agentDir)
	if !filepath.IsAbs(cleanAgentPath) {
		return "", fmt.Errorf("%w: %s is not absolute", ErrOutsideAgentDir, agentPath)
	}
	if isSamePath(cleanAgentPath, agentRoot) {
		return filepath.Clean(ownerDir), nil
	}
	prefix := agentRoot + string(filepath.Separator)
	if len(cleanAgentPath) <= len(prefix) || !strings.EqualFold(cleanAgentPath[:len(prefix)], prefix) {
		return "", fmt.Errorf("%w: %s is not under %s", ErrOutsideAgentDir, agentPath, agentDir)
	}
	return filepath.Join(ownerDir, cleanAgentPath[len(prefix):]), nil
}

func findWorkTree(dir string, ownerDir string) (string, error) {
	current := filepath.Clean(dir)
	for {
		isWorkTree, err := hasDotGit(current)
		if err != nil {
			return "", err
		}
		if isWorkTree {
			return current, nil
		}
		parent := filepath.Dir(current)
		if isSamePath(current, ownerDir) || parent == current {
			return "", fmt.Errorf("%w: %s", ErrNotWorkTree, dir)
		}
		current = parent
	}
}

func hasDotGit(dir string) (bool, error) {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("look for .git in %s: %w", dir, err)
}
