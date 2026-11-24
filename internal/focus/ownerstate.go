//go:build windows

package focus

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// ownerStateCache holds the last owner state read, each work tree by its path relative to the owner directory.
type ownerStateCache struct {
	repositories map[string]gitrepo.Repository
	workTrees    map[string]stampedOwnerWorkTree
}

func emptyOwnerStateCache() ownerStateCache {
	return ownerStateCache{repositories: map[string]gitrepo.Repository{}, workTrees: map[string]stampedOwnerWorkTree{}}
}

// stamps are the work tree's git state files as they were before workTree was read from them.
type stampedOwnerWorkTree struct {
	stamps   []gitrepo.FileStamp
	workTree OwnerWorkTree
}

// readOwnerState opens and reads again only the work trees whose git state files changed since cache was filled,
// since reading every one starts about ten git processes each, and returns the cache for the next read.
func readOwnerState(ctx context.Context, rules Rules, run func(ctx context.Context, script string) ([]byte, error),
	readOwnerWorkTree OwnerWorkTreeReader, focused []gitrepo.Repository, takeFailure string,
	cache ownerStateCache) (OwnerState, ownerStateCache, error) {
	found, err := findRepositories(ctx, rules, func(ctx context.Context, relativePath string, dir string) (
		gitrepo.Repository, error) {
		gitDir, err := checkedGitDir(rules, dir)
		if err != nil {
			return gitrepo.Repository{}, err
		}
		opened, isOpened := cache.repositories[relativePath]
		if isOpened && opened.GitDir == gitDir {
			return opened, nil
		}
		return gitrepo.Open(ctx, gitrepo.Repository{Dir: dir, GitDir: gitDir})
	})
	if err != nil {
		return OwnerState{}, ownerStateCache{}, err
	}
	repositoryAgentClones := make([]string, len(found.Repositories))
	repositoryAgentPaths := make([]string, len(found.Repositories))
	for index, repository := range found.Repositories {
		clone := repository.Repository.Dir
		if !strings.EqualFold(filepath.Clean(repository.Repository.GitDir), filepath.Join(clone, ".git")) {
			clone = filepath.Dir(filepath.Dir(filepath.Dir(repository.Repository.GitDir)))
		}
		if repositoryAgentClones[index], err = agentCopyDir(rules, clone); err != nil {
			return OwnerState{}, ownerStateCache{}, err
		}
		if repositoryAgentPaths[index], err = agentCopyDir(rules, repository.Repository.Dir); err != nil {
			return OwnerState{}, ownerStateCache{}, err
		}
	}
	refusalOwnerPaths := make([]string, len(found.Refusals))
	refusalAgentPaths := make([]string, len(found.Refusals))
	for index, refusal := range found.Refusals {
		refusalOwnerPaths[index] = filepath.Join(rules.OwnerDir, filepath.FromSlash(refusal.RelativePath))
		if refusalAgentPaths[index], err = agentCopyDir(rules, refusalOwnerPaths[index]); err != nil {
			return OwnerState{}, ownerStateCache{}, err
		}
	}
	candidateDirs := slices.Compact(slices.Sorted(slices.Values(slices.Concat(repositoryAgentClones, refusalAgentPaths))))
	existingDirs, err := existingAgentDirs(ctx, run, candidateDirs)
	if err != nil {
		return OwnerState{}, ownerStateCache{}, err
	}
	sharedIndexes := []int{}
	for index := range found.Repositories {
		if slices.Contains(existingDirs, repositoryAgentClones[index]) {
			sharedIndexes = append(sharedIndexes, index)
		}
	}
	readAt := time.Now()
	workTrees := make([]OwnerWorkTree, len(sharedIndexes))
	reads := make([]stampedOwnerWorkTree, len(sharedIndexes))
	succeeded := make([]bool, len(sharedIndexes))
	var group sync.WaitGroup
	for position, index := range sharedIndexes {
		repository := found.Repositories[index]
		group.Go(func() {
			reads[position], succeeded[position] = readStampedOwnerWorkTree(ctx, readOwnerWorkTree, repository,
				cache.workTrees[repository.RelativePath])
			workTrees[position] = withOwnerPaths(filepath.Clean(repository.Repository.Dir), repositoryAgentPaths[index],
				reads[position].workTree)
		})
	}
	group.Wait()
	nextCache := emptyOwnerStateCache()
	for _, repository := range found.Repositories {
		nextCache.repositories[repository.RelativePath] = repository.Repository
	}
	for position, index := range sharedIndexes {
		if succeeded[position] {
			nextCache.workTrees[found.Repositories[index].RelativePath] = reads[position]
		}
	}
	for index, refusal := range found.Refusals {
		if !slices.Contains(existingDirs, refusalAgentPaths[index]) {
			continue
		}
		workTrees = append(workTrees, withOwnerPaths(refusalOwnerPaths[index], refusalAgentPaths[index],
			refusedOwnerWorkTree(refusal.Err)))
	}
	slices.SortFunc(workTrees, func(a OwnerWorkTree, b OwnerWorkTree) int {
		return strings.Compare(a.OwnerPath, b.OwnerPath)
	})
	focusedPaths := []string{}
	for _, repository := range focused {
		focusedPaths = append(focusedPaths, filepath.Clean(repository.Dir))
	}
	return OwnerState{ReadAt: readAt.Format(time.RFC3339), TakeFailure: takeFailure, Focused: focusedPaths,
		WorkTrees: workTrees}, nextCache, nil
}

// It reports false for a failed read, which the caller leaves out of the cache so the next call reads it again.
func readStampedOwnerWorkTree(ctx context.Context, readOwnerWorkTree OwnerWorkTreeReader, repository FoundRepository,
	last stampedOwnerWorkTree) (stampedOwnerWorkTree, bool) {
	stamps, err := repository.Repository.StateFileStamps()
	if err != nil {
		return stampedOwnerWorkTree{stamps: []gitrepo.FileStamp{}, workTree: refusedOwnerWorkTree(err)}, false
	}
	if slices.Equal(stamps, last.stamps) {
		return last, true
	}
	workTree, err := readOwnerWorkTree(ctx, repository)
	if err == nil && workTree.Branch != "" {
		workTree.TakenCommit, err = repository.Repository.TakenCommit(ctx, workTree.Branch)
	}
	if err != nil {
		return stampedOwnerWorkTree{stamps: []gitrepo.FileStamp{}, workTree: refusedOwnerWorkTree(err)}, false
	}
	return stampedOwnerWorkTree{stamps: stamps, workTree: workTree}, true
}

func agentCopyDir(rules Rules, ownerDir string) (string, error) {
	agentDirs, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: ownerDir}})
	if err != nil {
		return "", err
	}
	return agentDirs[0], nil
}

func refusedOwnerWorkTree(reason error) OwnerWorkTree {
	return OwnerWorkTree{ChangedFiles: []string{}, Refusal: reason.Error()}
}

func withOwnerPaths(ownerPath string, agentPath string, workTree OwnerWorkTree) OwnerWorkTree {
	workTree.OwnerPath = ownerPath
	workTree.AgentPath = agentPath
	return workTree
}

// writeOwnerState moves a temporary file into place, so the agent never reads half a state.
func writeOwnerState(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error), file string,
	state OwnerState) error {
	var content bytes.Buffer
	encoder := json.NewEncoder(&content)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode the owner state for %s: %w", file, err)
	}
	// Base64 keeps file names, which the agent chooses, out of the script's syntax.
	temporaryFile := agenthost.QuotedString(file + ".tmp")
	script := "[IO.File]::WriteAllBytes(" + temporaryFile + ", [Convert]::FromBase64String('" +
		base64.StdEncoding.EncodeToString(content.Bytes()) + "')); Move-Item -LiteralPath " + temporaryFile +
		" -Destination " + agenthost.QuotedString(file) + " -Force -ErrorAction Stop"
	if _, err := run(ctx, script); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}
