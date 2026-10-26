package gitrepo

import (
	"context"
	"fmt"
	"slices"
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
		// %(upstream:track,nobracket) is empty, "gone", or "ahead N", "behind N" or both joined by ", ".
		track := fields[3]
		switch track {
		case "":
		case "gone":
			branch.UpstreamGone = true
		default:
			for part := range strings.SplitSeq(track, ", ") {
				direction, countText, _ := strings.Cut(part, " ")
				count, err := strconv.Atoi(countText)
				if err != nil {
					return nil, fmt.Errorf("parse git for-each-ref line %q: parse upstream track %q: %w", line, track, err)
				}
				switch direction {
				case "ahead":
					branch.Ahead = count
				case "behind":
					branch.Behind = count
				default:
					return nil, fmt.Errorf("parse git for-each-ref line %q: parse upstream track %q: unknown direction %q",
						line, track, direction)
				}
			}
		}
		branches = append(branches, branch)
	}
	return branches, nil
}

// Ref is the short name, such as origin/fix/x, and Name the part after the remote, such as fix/x.
type RemoteBranch struct {
	Ref  string
	Name string
}

// RemoteBranches skips each remote's HEAD.
func (r Repository) RemoteBranches(ctx context.Context) ([]RemoteBranch, error) {
	remotesOutput, err := r.git(ctx, "remote")
	if err != nil {
		return nil, err
	}
	remotes := strings.Fields(string(remotesOutput))
	// Longest first, since a remote name may hold a slash: team/fork/main belongs to team/fork rather than team.
	slices.SortFunc(remotes, func(first string, second string) int { return len(second) - len(first) })
	format := strings.Join([]string{"%(symref)", "%(refname:lstrip=2)"}, refFieldSeparator)
	output, err := r.git(ctx, "for-each-ref", "--format="+format, "refs/remotes")
	if err != nil {
		return nil, err
	}
	branches := []RemoteBranch{}
	for line := range strings.Lines(string(output)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), fieldSeparator)
		if len(fields) != 2 {
			return nil, fmt.Errorf("parse git for-each-ref line %q: want 2 fields, got %d", line, len(fields))
		}
		if fields[0] != "" {
			continue
		}
		ref := fields[1]
		index := slices.IndexFunc(remotes, func(remote string) bool { return strings.HasPrefix(ref, remote+"/") })
		if index < 0 {
			return nil, fmt.Errorf("name the remote of refs/remotes/%s in %s: no remote in %q matches it, "+
				"git update-ref -d refs/remotes/%s deletes it", ref, r.Dir, remotes, ref)
		}
		branches = append(branches, RemoteBranch{Ref: ref, Name: strings.TrimPrefix(ref, remotes[index]+"/")})
	}
	return branches, nil
}

// SwitchTracking creates the local branch for ref and switches to it, where a plain switch is ambiguous when two
// remotes have a branch of that name.
func (r Repository) SwitchTracking(ctx context.Context, ref string) error {
	return r.run(ctx, "switch", "--track", ref)
}

// DeleteBranch deletes branch whether or not it is merged, since plain --delete refuses a squash merged branch.
func (r Repository) DeleteBranch(ctx context.Context, branch string) error {
	return r.run(ctx, "branch", "--delete", "--force", branch)
}
