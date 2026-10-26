package gitrepo

import (
	"context"
	"slices"
	"strings"
)

// Path is slash separated, as git prints it. Branch is empty on a detached HEAD.
type WorkTree struct {
	Path   string
	Branch string
}

// WorkTrees lists the main work tree first, then the linked ones whose folder still exists.
func (r Repository) WorkTrees(ctx context.Context) ([]WorkTree, error) {
	output, err := r.git(ctx, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return parseWorkTrees(output), nil
}

// Each work tree is a run of NUL terminated attributes, starting with "worktree <path>" and ended by an empty one.
func parseWorkTrees(output []byte) []WorkTree {
	workTrees := []WorkTree{}
	for entry := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00\x00"), "\x00\x00") {
		attributes := strings.Split(entry, "\x00")
		path, isWorkTree := strings.CutPrefix(attributes[0], "worktree ")
		// git marks a work tree prunable when its folder is gone.
		isPrunable := slices.ContainsFunc(attributes, func(attribute string) bool {
			return attribute == "prunable" || strings.HasPrefix(attribute, "prunable ")
		})
		if !isWorkTree || isPrunable {
			continue
		}
		branchIndex := slices.IndexFunc(attributes, func(attribute string) bool {
			return strings.HasPrefix(attribute, "branch refs/heads/")
		})
		var branch string
		if branchIndex >= 0 {
			branch = strings.TrimPrefix(attributes[branchIndex], "branch refs/heads/")
		} else {
			branch = ""
		}
		workTrees = append(workTrees, WorkTree{Path: path, Branch: branch})
	}
	return workTrees
}
