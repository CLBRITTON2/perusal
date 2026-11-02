package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ErrStashListChanged = errors.New("stash list changed")

// A refname cannot hold a colon, so the branch ends at the first one.
var stashSubjectPrefix = regexp.MustCompile(`^(?:WIP on ([^:]*): [0-9a-f]+ |On ([^:]*): )`)

// Ref is the reflog name, such as stash@{0}, which every newer stash shifts, so PopStash and DropStash check it still
// names Hash. Summary drops the "WIP on <branch>: <short hash>" or "On <branch>:" prefix git gives Message.
type Stash struct {
	Commit
	Ref    string
	Branch string
}

func (r Repository) Stashes(ctx context.Context) ([]Stash, error) {
	output, err := r.git(ctx, "stash", "list", "-z", "--format=%gd%x1f%H%x1f%h%x1f%an%x1f%at%x1f%gs")
	if err != nil {
		return nil, err
	}
	stashes := []Stash{}
	for _, record := range bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		fields := strings.SplitN(string(record), fieldSeparator, 6)
		if len(fields) != 6 || !commitHash.MatchString(fields[1]) {
			return nil, fmt.Errorf("parse git stash list record %q: want a ref, a full hash and 4 more fields", record)
		}
		authoredUnix, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse git stash list record %q: author time: %w", record, err)
		}
		subject := fields[5]
		stash := Stash{
			Commit: Commit{Hash: fields[1], ShortHash: fields[2], Author: fields[3],
				AuthoredAt: time.Unix(authoredUnix, 0), Message: subject},
			Ref: fields[0],
		}
		if prefix := stashSubjectPrefix.FindStringSubmatch(subject); prefix != nil {
			stash.Branch = prefix[1] + prefix[2]
			stash.Summary = subject[len(prefix[0]):]
		} else {
			stash.Summary = subject
		}
		stashes = append(stashes, stash)
	}
	return stashes, nil
}

// StashDiff shows the tracked changes stash holds against the commit it was made on, then its untracked files.
func (r Repository) StashDiff(ctx context.Context, stash Stash, contextLines int) (string, error) {
	if !commitHash.MatchString(stash.Hash) {
		return "", fmt.Errorf("show stash %q in %s: not a full commit hash", stash.Hash, r.Dir)
	}
	output, err := r.git(ctx, slices.Concat([]string{"stash", "show", "--patch", "--include-untracked"}, diffFlags,
		[]string{"--find-renames", "--unified=" + strconv.Itoa(contextLines), stash.Hash})...)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// Stash stashes every change, untracked files included, and leaves the work tree as HEAD has it.
func (r Repository) Stash(ctx context.Context) error {
	return r.run(ctx, "stash", "push", "--include-untracked")
}

// PopStash restores stash with its staged changes staged again and drops it. git keeps a stash that conflicts.
func (r Repository) PopStash(ctx context.Context, stash Stash) error {
	if err := r.checkStashRef(ctx, stash); err != nil {
		return err
	}
	return r.run(ctx, "stash", "pop", "--index", stash.Ref)
}

func (r Repository) DropStash(ctx context.Context, stash Stash) error {
	if err := r.checkStashRef(ctx, stash); err != nil {
		return err
	}
	return r.run(ctx, "stash", "drop", stash.Ref)
}

func (r Repository) checkStashRef(ctx context.Context, stash Stash) error {
	hash, err := r.revParse(ctx, stash.Ref)
	if err != nil {
		return err
	}
	if hash != stash.Hash {
		return fmt.Errorf("%w in %s: %s is now %s rather than %s, try again once the list reloads",
			ErrStashListChanged, r.Dir, stash.Ref, hash, stash.Hash)
	}
	return nil
}
