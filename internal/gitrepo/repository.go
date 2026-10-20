package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Repository runs git in one working tree. Dir is the tree's top level.
type Repository struct {
	Dir string
}

// Open resolves dir, which may be any directory inside a working tree, to its repository.
func Open(ctx context.Context, dir string) (Repository, error) {
	output, err := runGit(ctx, dir, []string{"rev-parse", "--show-toplevel"})
	if err != nil {
		return Repository{}, fmt.Errorf("open repository %s: %w", dir, err)
	}
	return Repository{Dir: strings.TrimSpace(string(output))}, nil
}

// CurrentBranch returns the checked out branch, or "" when HEAD is detached.
func (r Repository) CurrentBranch(ctx context.Context) (string, error) {
	output, err := runGit(ctx, r.Dir, []string{"branch", "--show-current"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// ChangedFiles lists every staged, unstaged and untracked file, with untracked directories expanded to their files.
func (r Repository) ChangedFiles(ctx context.Context) ([]FileStatus, error) {
	output, err := runGit(ctx, r.Dir, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"})
	if err != nil {
		return nil, err
	}
	return ParseStatus(output)
}

// FileDiff returns the unified diff of file against HEAD, staged and unstaged changes together.
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

// untrackedFileDiff diffs path against an empty file. git diff --no-index exits 1 when the files differ, which for
// an untracked file is the normal result.
func (r Repository) untrackedFileDiff(ctx context.Context, path string) (string, error) {
	args := []string{"diff", "--no-index", "--no-color", "--no-ext-diff", "--", "/dev/null", path}
	output, err := runGit(ctx, r.Dir, args)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return string(output), nil
	}
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// Stage adds every change to file, including a deletion, to the index.
func (r Repository) Stage(ctx context.Context, file FileStatus) error {
	_, err := runGit(ctx, r.Dir, []string{"add", "--all", "--", file.Path})
	return err
}

// Unstage resets file in the index to HEAD and leaves the working tree alone. A rename unstages both paths.
func (r Repository) Unstage(ctx context.Context, file FileStatus) error {
	args := []string{"restore", "--staged", "--", file.Path}
	if file.OriginalPath != "" {
		args = append(args, file.OriginalPath)
	}
	_, err := runGit(ctx, r.Dir, args)
	return err
}

// Commit commits the index with message and returns git's summary line, such as "[master 1a2b3c4] Fix the thing".
// Signing and hooks follow the user's git config. The message goes on stdin to keep it out of the error text.
func (r Repository) Commit(ctx context.Context, message string) (string, error) {
	output, err := runGitWithInput(ctx, r.Dir, []string{"commit", "--file", "-"}, message)
	if err != nil {
		return "", err
	}
	summary, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return summary, nil
}

// runGit returns git's stdout. A failure carries the arguments, the directory and git's stderr and stdout, since
// some failures, such as git commit with nothing staged, explain themselves only on stdout.
func runGit(ctx context.Context, dir string, args []string) ([]byte, error) {
	return runGitWithInput(ctx, dir, args, "")
}

func runGitWithInput(ctx context.Context, dir string, args []string, input string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
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
