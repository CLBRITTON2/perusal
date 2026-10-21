package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Upstream, UpstreamGone, Ahead and Behind mean what they do in Status.
type Branch struct {
	Name         string
	Current      bool
	Upstream     string
	UpstreamGone bool
	Ahead        int
	Behind       int
}

var ErrBranchNotMerged = errors.New("branch is not merged")

// Branches lists the local branches by name.
func (r Repository) Branches(ctx context.Context) ([]Branch, error) {
	format := strings.Join([]string{"%(HEAD)", "%(refname:lstrip=2)", "%(upstream:short)", "%(upstream:track,nobracket)"},
		"%1f")
	output, err := runGit(ctx, r.Dir, []string{"for-each-ref", "--format=" + format, "refs/heads"})
	if err != nil {
		return nil, err
	}
	return parseBranches(output)
}

func parseBranches(output []byte) ([]Branch, error) {
	branches := []Branch{}
	for line := range strings.Lines(string(output)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), logFieldSeparator)
		if len(fields) != 4 {
			return nil, fmt.Errorf("parse git for-each-ref line %q: want 4 fields, got %d", line, len(fields))
		}
		branch := Branch{Name: fields[1], Current: fields[0] == "*", Upstream: fields[2]}
		withTrack, err := branchWithTrack(branch, fields[3])
		if err != nil {
			return nil, fmt.Errorf("parse git for-each-ref line %q: %w", line, err)
		}
		branches = append(branches, withTrack)
	}
	return branches, nil
}

// track is %(upstream:track,nobracket): empty, "gone", or "ahead N", "behind N" or both joined by ", ".
func branchWithTrack(branch Branch, track string) (Branch, error) {
	if track == "gone" {
		branch.UpstreamGone = true
		return branch, nil
	}
	if track == "" {
		return branch, nil
	}
	for part := range strings.SplitSeq(track, ", ") {
		direction, countText, _ := strings.Cut(part, " ")
		count, err := strconv.Atoi(countText)
		if err != nil {
			return Branch{}, fmt.Errorf("parse upstream track %q: %w", track, err)
		}
		switch direction {
		case "ahead":
			branch.Ahead = count
		case "behind":
			branch.Behind = count
		default:
			return Branch{}, fmt.Errorf("parse upstream track %q: unknown direction %q", track, direction)
		}
	}
	return branch, nil
}

// DeleteMergedBranch deletes branch only when MergedInto finds it merged into ref, so it deletes a squash merged branch
// that git branch --delete refuses.
func (r Repository) DeleteMergedBranch(ctx context.Context, branch string, ref string) error {
	merged, err := r.MergedInto(ctx, "refs/heads/"+branch, ref)
	if err != nil {
		return err
	}
	if !merged {
		return fmt.Errorf("delete %s in %s: %w into %s", branch, r.Dir, ErrBranchNotMerged, ref)
	}
	_, err = runGit(ctx, r.Dir, []string{"branch", "--delete", "--force", branch})
	return err
}
