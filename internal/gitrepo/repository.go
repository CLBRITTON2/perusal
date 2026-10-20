package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Repository runs git in one working tree. Dir is the tree's top level.
type Repository struct {
	Dir string
}

type Commit struct {
	Hash       string
	ShortHash  string
	Author     string
	AuthoredAt time.Time
	Summary    string
	// Message is the full commit message, summary included.
	Message string
}

const logFieldSeparator = "\x1f"

var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Open resolves dir, which may be any directory inside a working tree, to its repository.
func Open(ctx context.Context, dir string) (Repository, error) {
	output, err := runGit(ctx, dir, []string{"rev-parse", "--show-toplevel"})
	if err != nil {
		return Repository{}, fmt.Errorf("open repository %s: %w", dir, err)
	}
	return Repository{Dir: strings.TrimSpace(string(output))}, nil
}

// Status returns the branch and every staged, unstaged and untracked file, with untracked directories expanded to
// their files.
func (r Repository) Status(ctx context.Context) (Status, error) {
	output, err := runGit(ctx, r.Dir, []string{"status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all"})
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(output)
}

// Log returns up to maximumCount commits reachable from HEAD, newest first. HEAD must have a commit.
func (r Repository) Log(ctx context.Context, maximumCount int) ([]Commit, error) {
	format := strings.Join([]string{"%H", "%h", "%an", "%at", "%B"}, "%x1f")
	output, err := runGit(ctx, r.Dir, []string{"log", "-z", "--max-count=" + strconv.Itoa(maximumCount),
		"--format=" + format})
	if err != nil {
		return nil, err
	}
	return ParseLog(output)
}

// ParseLog reads git log -z output whose format is hash, short hash, author, author Unix time and message, separated
// by U+001F. The message comes last, so a separator inside it stays part of it.
func ParseLog(output []byte) ([]Commit, error) {
	commits := []Commit{}
	for _, record := range bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		fields := strings.SplitN(string(record), logFieldSeparator, 5)
		if len(fields) != 5 || !commitHash.MatchString(fields[0]) {
			return nil, fmt.Errorf("parse git log record %q: want a full hash and 4 more fields", record)
		}
		authoredUnix, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse git log record %q: author time: %w", record, err)
		}
		message := strings.TrimSpace(fields[4])
		summary, _, _ := strings.Cut(message, "\n")
		commits = append(commits, Commit{
			Hash:       fields[0],
			ShortHash:  fields[1],
			Author:     fields[2],
			AuthoredAt: time.Unix(authoredUnix, 0),
			Summary:    summary,
			Message:    message,
		})
	}
	return commits, nil
}

// CommitDiff returns the unified diff of every file the commit changed. A merge is diffed against its first parent.
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

// StageAll adds every change in the working tree to the index, untracked files and deletions included.
func (r Repository) StageAll(ctx context.Context) error {
	_, err := runGit(ctx, r.Dir, []string{"add", "--all"})
	return err
}

// UnstageAll resets the whole index to HEAD and leaves the working tree alone.
func (r Repository) UnstageAll(ctx context.Context) error {
	_, err := runGit(ctx, r.Dir, []string{"restore", "--staged", "--", "."})
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
