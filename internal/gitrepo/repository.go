package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CLBRITTON2/perusal/internal/diff"
)

type Repository struct {
	Dir string // the working tree's top level
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

// git log spells fieldSeparator %x1f and for-each-ref spells it %1f.
const (
	fieldSeparator    = "\x1f"
	logFieldSeparator = "%x1f"
	refFieldSeparator = "%1f"
)

// --no-ext-diff keeps a diff driver the repository configures from running.
var diffFlags = []string{"--no-color", "--no-ext-diff"}

var ErrCannotAmend = errors.New("cannot amend")

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func Open(ctx context.Context, dir string) (Repository, error) {
	output, err := Repository{Dir: dir}.git(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, fmt.Errorf("open repository %s: %w", dir, err)
	}
	return Repository{Dir: strings.TrimSpace(string(output))}, nil
}

func (r Repository) Status(ctx context.Context) (Status, error) {
	output, err := r.git(ctx, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(output)
}

// commitGraph needs --topo-order. Fails on an unborn branch.
func (r Repository) Log(ctx context.Context, limit int) ([]Commit, error) {
	format := strings.Join([]string{"%H", "%h", "%P", "%D", "%an", "%at", "%B"}, logFieldSeparator)
	output, err := r.git(ctx, "log", "-z", "--topo-order", "--max-count="+strconv.Itoa(limit), "--format="+format)
	if err != nil {
		return nil, err
	}
	return parseLog(output)
}

// The message is the last field, so a separator inside it stays part of it.
func parseLog(output []byte) ([]Commit, error) {
	commits := []Commit{}
	for _, record := range bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		fields := strings.SplitN(string(record), fieldSeparator, 7)
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
func (r Repository) CommitDiff(ctx context.Context, hash string, contextLines int) (string, error) {
	if !commitHash.MatchString(hash) {
		return "", fmt.Errorf("show commit %q in %s: not a full commit hash", hash, r.Dir)
	}
	output, err := r.git(ctx, slices.Concat([]string{"show", "--format="}, diffFlags, []string{"--find-renames",
		"--unified=" + strconv.Itoa(contextLines), "--diff-merges=first-parent", hash, "--"})...)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// An untracked file's diff always shows the whole file.
func (r Repository) FileDiff(ctx context.Context, file FileStatus, contextLines int) (string, error) {
	if file.IsUntracked() {
		return r.untrackedFileDiff(ctx, file.Path)
	}
	args := slices.Concat([]string{"diff", "HEAD"}, diffFlags, []string{"--find-renames",
		"--unified=" + strconv.Itoa(contextLines), "--"}, file.pathspec())
	output, err := r.git(ctx, args...)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (r Repository) StagedLineCounts(ctx context.Context) (diff.LineCounts, error) {
	args := slices.Concat([]string{"diff", "--cached", "--numstat"}, diffFlags, []string{"--find-renames"})
	output, err := r.git(ctx, args...)
	if err != nil {
		return diff.LineCounts{}, err
	}
	return parseNumstat(output)
}

// A binary file's counts are "-", which adds nothing.
func parseNumstat(output []byte) (diff.LineCounts, error) {
	counts := diff.LineCounts{}
	for line := range strings.Lines(string(output)) {
		fields := strings.SplitN(strings.TrimSuffix(line, "\n"), "\t", 3)
		if len(fields) != 3 {
			return diff.LineCounts{}, fmt.Errorf("parse git diff --numstat line %q: want added, removed and a path", line)
		}
		if fields[0] == "-" && fields[1] == "-" {
			continue
		}
		added, err := strconv.Atoi(fields[0])
		if err != nil {
			return diff.LineCounts{}, fmt.Errorf("parse git diff --numstat line %q: %w", line, err)
		}
		removed, err := strconv.Atoi(fields[1])
		if err != nil {
			return diff.LineCounts{}, fmt.Errorf("parse git diff --numstat line %q: %w", line, err)
		}
		counts.Added += added
		counts.Removed += removed
	}
	return counts, nil
}

// DefaultBranch is origin's default branch as of the last fetch, such as origin/main. It is empty when origin/HEAD is
// unset, as in a repository that was pushed to origin instead of cloned from it.
func (r Repository) DefaultBranch(ctx context.Context) (string, error) {
	output, err := r.git(ctx, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	switch {
	case err == nil:
		return strings.TrimSpace(string(output)), nil
	case isExitCodeOne(err):
		return "", nil
	}
	return "", err
}

func (r Repository) CommitsBehind(ctx context.Context, ref string) (int, error) {
	output, err := r.git(ctx, "rev-list", "--count", "HEAD.."+ref)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("parse the count of commits on %s missing from HEAD in %s: %w", ref, r.Dir, err)
	}
	return count, nil
}

// MergedInto is true when merging revision into ref would change nothing, as after a merge or a squash merge. It
// compares trees, since ancestry misses a squash and patch-ids miss one whose context lines changed on ref first.
func (r Repository) MergedInto(ctx context.Context, revision string, ref string) (bool, error) {
	args := []string{"merge-tree", "--write-tree", "--no-messages", "--allow-unrelated-histories", ref, revision}
	mergedTree, err := r.git(ctx, args...)
	switch {
	case isExitCodeOne(err):
		return false, nil
	case err != nil:
		return false, err
	}
	refTree, err := r.git(ctx, "rev-parse", "--verify", ref+"^{tree}")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(mergedTree)) == strings.TrimSpace(string(refTree)), nil
}

// git diff --no-index exits 1 when the files differ, the normal result for an untracked file.
func (r Repository) untrackedFileDiff(ctx context.Context, path string) (string, error) {
	args := slices.Concat([]string{"diff", "--no-index"}, diffFlags, []string{"--", "/dev/null", path})
	output, err := r.git(ctx, args...)
	if err != nil && !isExitCodeOne(err) {
		return "", err
	}
	return string(output), nil
}

func (r Repository) Stage(ctx context.Context, file FileStatus) error {
	return r.run(ctx, "add", "--all", "--", file.Path)
}

func (r Repository) StageAll(ctx context.Context) error {
	return r.run(ctx, "add", "--all")
}

func (r Repository) UnstageAll(ctx context.Context) error {
	return r.run(ctx, "restore", "--staged", "--", ".")
}

func (r Repository) Unstage(ctx context.Context, file FileStatus) error {
	return r.run(ctx, append([]string{"restore", "--staged", "--"}, file.pathspec()...)...)
}

// A renamed file gets its original path back.
func (r Repository) Discard(ctx context.Context, file FileStatus) error {
	if file.IsUntracked() {
		return r.run(ctx, "clean", "--force", "--", file.Path)
	}
	return r.run(ctx, append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, file.pathspec()...)...)
}

// DiscardAll leaves the work tree as HEAD has it, except for ignored files and nested repositories.
func (r Repository) DiscardAll(ctx context.Context) error {
	if err := r.run(ctx, "restore", "--source=HEAD", "--staged", "--worktree", "--", "."); err != nil {
		return err
	}
	return r.run(ctx, "clean", "--force", "-d", "--", ".")
}

func (r Repository) Pull(ctx context.Context) error {
	return r.run(ctx, "pull", "--ff-only", "--no-rebase")
}

// Switch creates branch from the remote branch of that name when it exists only there.
func (r Repository) Switch(ctx context.Context, branch string) error {
	return r.run(ctx, "switch", branch)
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
	return r.run(ctx, "push")
}

// git refuses unless the remote branch is still where it was last fetched and that tip is in the local reflog,
// so a force push replaces only commits this branch already rewrote.
func (r Repository) ForcePush(ctx context.Context) error {
	return r.run(ctx, "push", "--force-with-lease", "--force-if-includes")
}

// Amend folds the staged changes into hash, which must be HEAD or an ancestor with no merge above it. Below HEAD it
// commits a fixup and autosquashes it in, and a rebase that stops is aborted and the fixup undone, so the staged
// changes stay staged.
func (r Repository) Amend(ctx context.Context, hash string) error {
	if !commitHash.MatchString(hash) {
		return fmt.Errorf("%w %q in %s: not a full commit hash", ErrCannotAmend, hash, r.Dir)
	}
	nothingStaged, err := r.gitSucceeds(ctx, "diff", "--cached", "--quiet")
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
		return r.run(ctx, "commit", "--amend", "--no-edit")
	}
	if err := r.checkRewritable(ctx, hash); err != nil {
		return err
	}
	if err := r.run(ctx, "commit", "--fixup="+hash); err != nil {
		return err
	}
	fixup, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return err
	}
	// ":" is the editor git skips, so the todo list runs as autosquash wrote it.
	rebaseErr := r.run(ctx, "-c", "sequence.editor=:", "rebase", "--interactive", "--autosquash",
		"--autostash", hash+"^")
	if rebaseErr != nil {
		return r.undoFixup(ctx, fixup, rebaseErr)
	}
	return nil
}

// A rebase would drop a merge above hash and has no parent to start from below a root.
func (r Repository) checkRewritable(ctx context.Context, hash string) error {
	isAncestor, err := r.gitSucceeds(ctx, "merge-base", "--is-ancestor", hash, "HEAD")
	if err != nil {
		return err
	}
	if !isAncestor {
		return fmt.Errorf("%w %s in %s: not in the history of HEAD", ErrCannotAmend, hash, r.Dir)
	}
	parentsOutput, err := r.git(ctx, "log", "--max-count=1", "--format=%P", hash)
	if err != nil {
		return err
	}
	if len(strings.Fields(string(parentsOutput))) != 1 {
		return fmt.Errorf("%w %s in %s: it is a merge or a root commit", ErrCannotAmend, hash, r.Dir)
	}
	mergesOutput, err := r.git(ctx, "rev-list", "--min-parents=2", hash+"..HEAD")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(mergesOutput)) != 0 {
		return fmt.Errorf("%w %s in %s: a merge sits between it and HEAD", ErrCannotAmend, hash, r.Dir)
	}
	return nil
}

func (r Repository) undoFixup(ctx context.Context, fixup string, rebaseErr error) error {
	abortErr := r.run(ctx, "rebase", "--abort")
	head, err := r.revParse(ctx, "HEAD")
	if err != nil {
		return errors.Join(rebaseErr, abortErr, err)
	}
	if head != fixup {
		return errors.Join(rebaseErr, abortErr, fmt.Errorf("HEAD in %s is %s after the rebase stopped, not the fixup "+
			"commit %s, so the fixup was left in place", r.Dir, head, fixup))
	}
	if err := r.run(ctx, "reset", "--soft", fixup+"^"); err != nil {
		return errors.Join(rebaseErr, abortErr, err)
	}
	return fmt.Errorf("undid the fixup commit, so the changes are staged again: %w", errors.Join(rebaseErr, abortErr))
}

func (r Repository) revParse(ctx context.Context, revision string) (string, error) {
	output, err := r.git(ctx, "rev-parse", "--verify", revision+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// For the git commands that answer with exit code 1 for no.
func (r Repository) gitSucceeds(ctx context.Context, args ...string) (bool, error) {
	err := r.run(ctx, args...)
	switch {
	case err == nil:
		return true, nil
	case isExitCodeOne(err):
		return false, nil
	}
	return false, err
}

func isExitCodeOne(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

func (r Repository) git(ctx context.Context, args ...string) ([]byte, error) {
	return runGit(ctx, r.Dir, args)
}

func (r Repository) run(ctx context.Context, args ...string) error {
	_, err := r.git(ctx, args...)
	return err
}

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
		// Carries stdout too: git commit with nothing staged explains itself only there.
		return output, fmt.Errorf("run git %s in %s: %w: %s", strings.Join(args, " "), dir, err,
			strings.TrimSpace(stderr.String()+"\n"+string(output)))
	}
	return output, nil
}
