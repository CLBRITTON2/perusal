// Package focus maps the agent's focus file to the repositories the owner can safely run git in.
package focus

import (
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
type Focus struct {
	Repositories []gitrepo.Repository
	Refusals     []error
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
