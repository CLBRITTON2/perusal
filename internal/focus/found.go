// Package focus maps the agent's focus file to the repositories the owner can safely run git in.
package focus

import (
	"context"
	"errors"
	"io/fs"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

var (
	ErrMalformedFocusFile = errors.New("the focus file must hold one absolute path per line")
	ErrOutsideAgentDir    = errors.New("the path is outside the agent directory")
	ErrNotWorkTree        = errors.New("no .git between the path and the owner directory")
	ErrGitDirUnprotected  = errors.New("the .git directory is not write-denied to the agent account")
	ErrNotLinkedWorkTree  = errors.New("the .git file does not name this work tree's folder in a repository's worktrees")
)

// Repositories are in focus file order, each once. Refusals hold an error for each path that failed a check.
// MissingWorkTrees are the agent's worktrees the owner has no copy of yet, each once.
type Focus struct {
	Repositories     []gitrepo.Repository
	Refusals         []error
	MissingWorkTrees []MissingWorkTree
}

// MissingWorkTree is a worktree the agent added under its worktrees folder, at AgentDir, whose owner copy at Dir does
// not exist yet. Clone is the owner's clone of its repository, checked as a focused one is. RelativePath has forward
// slashes.
type MissingWorkTree struct {
	AgentDir     string
	Dir          string
	RelativePath string
	Clone        gitrepo.Repository
}

// RelativePath has forward slashes.
type FoundRepository struct {
	RelativePath string
	Repository   gitrepo.Repository
}

type FoundRefusal struct {
	RelativePath string
	Err          error
}

// Summary names the check Err failed in a few words, short enough for a list row.
func (r FoundRefusal) Summary() string {
	switch {
	case errors.Is(r.Err, ErrGitDirUnprotected):
		return "not write-denied to the agent"
	case errors.Is(r.Err, ErrNotLinkedWorkTree):
		return ".git file names no work tree of it"
	case errors.Is(r.Err, fs.ErrPermission):
		return "access denied"
	}
	return "failed a check"
}

// Repositories are in path order. Refusals are the work trees that failed a check and the directories that could not
// be listed.
type Found struct {
	Repositories []FoundRepository
	Refusals     []FoundRefusal
}

type OwnerState struct {
	ReadAt      string          `json:"readAt"`
	TakeFailure string          `json:"takeFailure"`
	Focused     []string        `json:"focused"`
	WorkTrees   []OwnerWorkTree `json:"workTrees"`
}

type OwnerWorkTree struct {
	OwnerPath        string   `json:"ownerPath"`
	AgentPath        string   `json:"agentPath"`
	Branch           string   `json:"branch"`
	Head             string   `json:"head"`
	HeadSubject      string   `json:"headSubject"`
	ChangedFiles     []string `json:"changedFiles"`
	ChangedFileCount int      `json:"changedFileCount"`
	Upstream         string   `json:"upstream"`
	Ahead            int      `json:"ahead"`
	Behind           int      `json:"behind"`
	BehindDefault    int      `json:"behindDefault,omitempty"`
	MergedInto       string   `json:"mergedInto"`
	TakenCommit      string   `json:"takenCommit"`
	Refusal          string   `json:"refusal"`
}

type OwnerWorkTreeReader func(ctx context.Context, found FoundRepository) (OwnerWorkTree, error)
