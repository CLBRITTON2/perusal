package gitrepo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// OtherWorkTree is the slash separated path of another work tree that has the branch checked out, empty when none.
type Branch struct {
	Name          string
	Current       bool
	OtherWorkTree string
	Tracking
}

func (r Repository) Branches(ctx context.Context) ([]Branch, error) {
	format := strings.Join([]string{"%(HEAD)", "%(refname:lstrip=2)", "%(upstream:short)", "%(upstream:track,nobracket)",
		"%(worktreepath)"}, refFieldSeparator)
	output, err := r.git(ctx, "for-each-ref", "--format="+format, "refs/heads")
	if err != nil {
		return nil, err
	}
	return parseBranches(output)
}

func parseBranches(output []byte) ([]Branch, error) {
	branches := []Branch{}
	for line := range strings.Lines(string(output)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), fieldSeparator)
		if len(fields) != 5 {
			return nil, fmt.Errorf("parse git for-each-ref line %q: want 5 fields, got %d", line, len(fields))
		}
		branch := Branch{Name: fields[1], Current: fields[0] == "*", Tracking: Tracking{Upstream: fields[2]}}
		// %(worktreepath) also names the current work tree for the current branch.
		if !branch.Current {
			branch.OtherWorkTree = fields[4]
		}
		withTrack, err := branchWithTrack(branch, fields[3])
		if err != nil {
			return nil, fmt.Errorf("parse git for-each-ref line %q: %w", line, err)
		}
		branches = append(branches, withTrack)
	}
	return branches, nil
}

// Ref is the short name, such as origin/fix/x, and Name the part after the remote, such as fix/x.
type RemoteBranch struct {
	Ref  string
	Name string
}

// RemoteBranches skips each remote's HEAD. Name drops only the first path segment, so a remote named with a slash
// gets a wrong Name.
func (r Repository) RemoteBranches(ctx context.Context) ([]RemoteBranch, error) {
	format := strings.Join([]string{"%(symref)", "%(refname:lstrip=2)", "%(refname:lstrip=3)"}, refFieldSeparator)
	output, err := r.git(ctx, "for-each-ref", "--format="+format, "refs/remotes")
	if err != nil {
		return nil, err
	}
	branches := []RemoteBranch{}
	for line := range strings.Lines(string(output)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), fieldSeparator)
		if len(fields) != 3 {
			return nil, fmt.Errorf("parse git for-each-ref line %q: want 3 fields, got %d", line, len(fields))
		}
		if fields[0] != "" {
			continue
		}
		branches = append(branches, RemoteBranch{Ref: fields[1], Name: fields[2]})
	}
	return branches, nil
}

// SwitchTracking creates the local branch for ref and switches to it, where a plain switch is ambiguous when two
// remotes have a branch of that name.
func (r Repository) SwitchTracking(ctx context.Context, ref string) error {
	return r.run(ctx, "switch", "--track", ref)
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

// DeleteBranch deletes branch whether or not it is merged, since plain --delete refuses a squash merged branch.
func (r Repository) DeleteBranch(ctx context.Context, branch string) error {
	return r.run(ctx, "branch", "--delete", "--force", branch)
}
