package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Dir is the working tree's top level.
type Repository struct {
	Dir string
}

type Commit struct {
	Hash      string
	ShortHash string
	// First parent first, which commitGraph relies on.
	Parents []string
	// Refs are the branch and tag names git log decorates the commit with, such as "HEAD -> master" or "tag: v1".
	Refs       []string
	Author     string
	AuthoredAt time.Time
	Summary    string
	Message    string
}

const logFieldSeparator = "\x1f"

var ErrCannotAmend = errors.New("cannot amend")

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func Open(ctx context.Context, dir string) (Repository, error) {
	output, err := runGit(ctx, dir, []string{"rev-parse", "--show-toplevel"})
	if err != nil {
		return Repository{}, fmt.Errorf("open repository %s: %w", dir, err)
	}
	return Repository{Dir: strings.TrimSpace(string(output))}, nil
}

func (r Repository) Status(ctx context.Context) (Status, error) {
	output, err := runGit(ctx, r.Dir, []string{"status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all"})
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(output)
}

// commitGraph needs --topo-order. Fails on an unborn branch.
func (r Repository) Log(ctx context.Context, maximumCount int) ([]Commit, error) {
	format := strings.Join([]string{"%H", "%h", "%P", "%D", "%an", "%at", "%B"}, "%x1f")
	output, err := runGit(ctx, r.Dir, []string{"log", "-z", "--topo-order", "--max-count=" + strconv.Itoa(maximumCount),
		"--format=" + format})
	if err != nil {
		return nil, err
	}
	return ParseLog(output)
}

// The message is the last field, so a separator inside it stays part of it.
func ParseLog(output []byte) ([]Commit, error) {
	commits := []Commit{}
	for _, record := range bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		fields := strings.SplitN(string(record), logFieldSeparator, 7)
		if len(fields) != 7 || !commitHash.MatchString(fields[0]) {
			return nil, fmt.Errorf("parse git log record %q: want a full hash and 6 more fields", record)
		}
		parents := strings.Fields(fields[2])
		for _, parent := range parents {
			if !commitHash.MatchString(parent) {
				return nil, fmt.Errorf("parse git log record %q: parent %q is not a full commit hash", record, parent)
			}
		}
		refs := []string{}
		if fields[3] != "" {
			refs = strings.Split(fields[3], ", ")
		}
		authoredUnix, err := strconv.ParseInt(fields[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse git log record %q: author time: %w", record, err)
		}
		message := strings.TrimSpace(fields[6])
		summary, _, _ := strings.Cut(message, "\n")
		commits = append(commits, Commit{
			Hash:       fields[0],
			ShortHash:  fields[1],
			Parents:    parents,
			Refs:       refs,
			Author:     fields[4],
			AuthoredAt: time.Unix(authoredUnix, 0),
			Summary:    summary,
			Message:    message,
		})
	}
	return commits, nil
}

// A merge is diffed against its first parent.
func (r Repository) CommitDiff(ctx context.Context, hash string) (string, error) {
	if !commitHash.MatchString(hash) {
		return "", fmt.Errorf("show commit %q in %s: not a full commit hash", hash, r.Dir)
	}
	output, err := runGit(ctx, r.Dir, []string{"show", "--format=", "--no-color", "--no-ext-diff", "--find-renames",
		"--diff-merges=first-parent", hash, "--"})
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (r Repository) FileDiff(ctx context.Context, file FileStatus) (string, error) {
	if file.IsUntracked() {
		return r.untrackedFileDiff(ctx, file.Path)
	}
	args := []string{"diff", "HEAD", "--no-color", "--no-ext-diff", "--find-renames", "--", file.Path}
	if file.OriginalPath != "" {
		args = append(args, file.OriginalPath)
	}
	output, err := runGit(ctx, r.Dir, args)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// git diff --no-index exits 1 when the files differ, the normal result for an untracked file.
func (r Repository) untrackedFileDiff(ctx context.Context, path string) (string, error) {
	args := []string{"diff", "--no-index", "--no-color", "--no-ext-diff", "--", "/dev/null", path}
	output, err := runGit(ctx, r.Dir, args)
	var exitErr *exec.ExitError
	if err != nil && !(errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		return "", err
	}
	return string(output), nil
}

func (r Repository) Stage(ctx context.Context, file FileStatus) error {
	_, err := runGit(ctx, r.Dir, []string{"add", "--all", "--", file.Path})
	return err
}

func (r Repository) StageAll(ctx context.Context) error {
	_, err := runGit(ctx, r.Dir, []string{"add", "--all"})
	return err
}

func (r Repository) UnstageAll(ctx context.Context) error {
	_, err := runGit(ctx, r.Dir, []string{"restore", "--staged", "--", "."})
	return err
}

func (r Repository) Unstage(ctx context.Context, file FileStatus) error {
	args := []string{"restore", "--staged", "--", file.Path}
	if file.OriginalPath != "" {
		args = append(args, file.OriginalPath)
	}
	_, err := runGit(ctx, r.Dir, args)
	return err
}

// The message goes on stdin to keep it out of the error text.
func (r Repository) Commit(ctx context.Context, message string) (string, error) {
	output, err := runGitWithInput(ctx, r.Dir, []string{"commit", "--file", "-"}, message)
	if err != nil {
		return "", err
	}
	summary, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return summary, nil
}

func (r Repository) Push(ctx context.Context) error {
	_, err := runGit(ctx, r.Dir, []string{"push"})
	return err
}

// git refuses unless the remote branch is still where it was last fetched and that tip is in the local reflog,
// so a force push replaces only commits this branch already rewrote.
func (r Repository) ForcePush(ctx context.Context) error {
	_, err := runGit(ctx, r.Dir, []string{"push", "--force-with-lease", "--force-if-includes"})
	return err
}

// Amend folds the staged changes into hash, which must be HEAD or an ancestor with no merge above it. Below HEAD it
// commits a fixup and autosquashes it in, and a rebase that stops is aborted and the fixup undone, so the staged
// changes stay staged.
func (r Repository) Amend(ctx context.Context, hash string) error {
	if !commitHash.MatchString(hash) {
		return fmt.Errorf("%w %q in %s: not a full commit hash", ErrCannotAmend, hash, r.Dir)
	}
	nothingStaged, err := r.gitSucceeds(ctx, []string{"diff", "--cached", "--quiet"})
	if err != nil {
		return err
	}
	if nothingStaged {
		return fmt.Errorf("%w %s in %s: nothing is staged", ErrCannotAmend, hash, r.Dir)
	}
	head, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return err
	}
	if hash == head {
		_, err := runGit(ctx, r.Dir, []string{"commit", "--amend", "--no-edit"})
		return err
	}
	if err := r.checkRewritable(ctx, hash); err != nil {
		return err
	}
	if _, err := runGit(ctx, r.Dir, []string{"commit", "--fixup=" + hash}); err != nil {
		return err
	}
	fixup, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return err
	}
	// ":" is the editor git skips, so the todo list runs as autosquash wrote it.
	_, rebaseErr := runGit(ctx, r.Dir, []string{"-c", "sequence.editor=:", "rebase", "--interactive", "--autosquash",
		"--autostash", hash + "^"})
	if rebaseErr != nil {
		return r.undoFixup(ctx, fixup, rebaseErr)
	}
	return nil
}

// A rebase would drop a merge above hash and has no parent to start from below a root.
func (r Repository) checkRewritable(ctx context.Context, hash string) error {
	isAncestor, err := r.gitSucceeds(ctx, []string{"merge-base", "--is-ancestor", hash, "HEAD"})
	if err != nil {
		return err
	}
	if !isAncestor {
		return fmt.Errorf("%w %s in %s: not in the history of HEAD", ErrCannotAmend, hash, r.Dir)
	}
	parents, err := runGit(ctx, r.Dir, []string{"log", "--max-count=1", "--format=%P", hash})
	if err != nil {
		return err
	}
	if len(strings.Fields(string(parents))) != 1 {
		return fmt.Errorf("%w %s in %s: it is a merge or a root commit", ErrCannotAmend, hash, r.Dir)
	}
	merges, err := runGit(ctx, r.Dir, []string{"rev-list", "--min-parents=2", hash + "..HEAD"})
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(merges)) != 0 {
		return fmt.Errorf("%w %s in %s: a merge sits between it and HEAD", ErrCannotAmend, hash, r.Dir)
	}
	return nil
}

func (r Repository) undoFixup(ctx context.Context, fixup string, rebaseErr error) error {
	_, abortErr := runGit(ctx, r.Dir, []string{"rebase", "--abort"})
	head, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return errors.Join(rebaseErr, abortErr, err)
	}
	if head != fixup {
		return errors.Join(rebaseErr, abortErr, fmt.Errorf("HEAD in %s is %s after the rebase stopped, not the fixup "+
			"commit %s, so the fixup was left in place", r.Dir, head, fixup))
	}
	if _, err := runGit(ctx, r.Dir, []string{"reset", "--soft", fixup + "^"}); err != nil {
		return errors.Join(rebaseErr, abortErr, err)
	}
	return fmt.Errorf("undid the fixup commit, so the changes are staged again: %w", errors.Join(rebaseErr, abortErr))
}

func (r Repository) revParse(ctx context.Context, revision string) (string, error) {
	output, err := runGit(ctx, r.Dir, []string{"rev-parse", "--verify", revision + "^{commit}"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// For the git commands that answer with exit code 1 for no.
func (r Repository) gitSucceeds(ctx context.Context, args []string) (bool, error) {
	_, err := runGit(ctx, r.Dir, args)
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return false, nil
	}
	return false, err
}

// The error carries stdout too: git commit with nothing staged explains itself only there.
func runGit(ctx context.Context, dir string, args []string) ([]byte, error) {
	return runGitWithInput(ctx, dir, args, "")
}

func runGitWithInput(ctx context.Context, dir string, args []string, input string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	// A credential prompt would fight the TUI for the terminal, so git fails instead of asking.
	// Optional locks off: polling git status would otherwise take index.lock and fail the owner's own git add.
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	command.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return output, fmt.Errorf("run git %s in %s: %w: %s", strings.Join(args, " "), dir, err,
			strings.TrimSpace(stderr.String()+"\n"+string(output)))
	}
	return output, nil
}
