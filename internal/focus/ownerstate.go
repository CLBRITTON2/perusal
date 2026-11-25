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

// ownerScan is every work tree under the owner directory with the paths of its agent copy and of its clone's agent
// copy, each slice in the order of found's, and which of those copies exist on the agent host.
type ownerScan struct {
	found             Found
	agentClones       []string
	agentPaths        []string
	refusalOwnerPaths []string
	refusalAgentPaths []string
	existingDirs      []string
}

func emptyOwnerScan() ownerScan {
	return ownerScan{found: Found{Repositories: []FoundRepository{}, Refusals: []FoundRefusal{}}, agentClones: []string{},
		agentPaths: []string{}, refusalOwnerPaths: []string{}, refusalAgentPaths: []string{}, existingDirs: []string{}}
}

// pairedWorkTrees are the owner's work trees whose agent copy exists at the same path under the agent directory, each
// repository at the index of its agent copy in agentDirs. refusals are the errors of those that failed a check.
type pairedWorkTrees struct {
	repositories []gitrepo.Repository
	agentDirs    []string
	refusals     []string
}

// stamps are the work tree's git state files as they were before workTree was read from them.
type stampedOwnerWorkTree struct {
	stamps   []gitrepo.FileStamp
	workTree OwnerWorkTree
}

// scanOwnerDir checks every work tree under the owner directory as resolve checks a focused one, opening again only
// those whose checked git dir differs from the one last opened, since each open starts git. It asks the agent host
// which copies exist in one script.
func scanOwnerDir(ctx context.Context, rules Rules, run func(ctx context.Context, script string) ([]byte, error),
	last ownerScan) (ownerScan, error) {
	opened := map[string]gitrepo.Repository{}
	for _, repository := range last.found.Repositories {
		opened[repository.RelativePath] = repository.Repository
	}
	found, err := findRepositories(ctx, rules, func(ctx context.Context, relativePath string, dir string) (
		gitrepo.Repository, error) {
		gitDir, err := checkedGitDir(rules, dir)
		if err != nil {
			return gitrepo.Repository{}, err
		}
		repository, isOpened := opened[relativePath]
		if isOpened && repository.GitDir == gitDir {
			return repository, nil
		}
		return gitrepo.Open(ctx, gitrepo.Repository{Dir: dir, GitDir: gitDir})
	})
	if err != nil {
		return ownerScan{}, err
	}
	scan := ownerScan{found: found, agentClones: make([]string, len(found.Repositories)),
		agentPaths: make([]string, len(found.Repositories)), refusalOwnerPaths: make([]string, len(found.Refusals)),
		refusalAgentPaths: make([]string, len(found.Refusals)), existingDirs: []string{}}
	for index, repository := range found.Repositories {
		clone := repository.Repository.Dir
		if !strings.EqualFold(filepath.Clean(repository.Repository.GitDir), filepath.Join(clone, ".git")) {
			clone = filepath.Dir(filepath.Dir(filepath.Dir(repository.Repository.GitDir)))
		}
		if scan.agentClones[index], err = agentCopyDir(rules, clone); err != nil {
			return ownerScan{}, err
		}
		if scan.agentPaths[index], err = agentCopyDir(rules, repository.Repository.Dir); err != nil {
			return ownerScan{}, err
		}
	}
	for index, refusal := range found.Refusals {
		scan.refusalOwnerPaths[index] = filepath.Join(rules.OwnerDir, filepath.FromSlash(refusal.RelativePath))
		if scan.refusalAgentPaths[index], err = agentCopyDir(rules, scan.refusalOwnerPaths[index]); err != nil {
			return ownerScan{}, err
		}
	}
	candidateDirs := slices.Compact(slices.Sorted(slices.Values(slices.Concat(scan.agentClones, scan.agentPaths,
		scan.refusalAgentPaths))))
	if scan.existingDirs, err = existingAgentDirs(ctx, run, candidateDirs); err != nil {
		return ownerScan{}, err
	}
	return scan, nil
}

func (scan ownerScan) paired() pairedWorkTrees {
	paired := pairedWorkTrees{repositories: []gitrepo.Repository{}, agentDirs: []string{}, refusals: []string{}}
	for index, repository := range scan.found.Repositories {
		if slices.Contains(scan.existingDirs, scan.agentPaths[index]) {
			paired.repositories = append(paired.repositories, repository.Repository)
			paired.agentDirs = append(paired.agentDirs, scan.agentPaths[index])
		}
	}
	for index, refusal := range scan.found.Refusals {
		if slices.Contains(scan.existingDirs, scan.refusalAgentPaths[index]) {
			paired.refusals = append(paired.refusals, refusal.Err.Error())
		}
	}
	return paired
}

// readOwnerState reads again only the work trees whose git state files changed since cache was filled, since reading
// every one starts about ten git processes each, and returns the cache for the next read. It lists the work trees whose
// clone the agent host has.
func readOwnerState(ctx context.Context, readOwnerWorkTree OwnerWorkTreeReader, scan ownerScan,
	focused []gitrepo.Repository, takeFailure string,
	cache map[string]stampedOwnerWorkTree) (OwnerState, map[string]stampedOwnerWorkTree) {
	found := scan.found
	sharedIndexes := []int{}
	for index := range found.Repositories {
		if slices.Contains(scan.existingDirs, scan.agentClones[index]) {
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
				cache[repository.RelativePath])
			workTrees[position] = withOwnerPaths(filepath.Clean(repository.Repository.Dir), scan.agentPaths[index],
				reads[position].workTree)
		})
	}
	group.Wait()
	nextCache := map[string]stampedOwnerWorkTree{}
	for position, index := range sharedIndexes {
		if succeeded[position] {
			nextCache[found.Repositories[index].RelativePath] = reads[position]
		}
	}
	for index, refusal := range found.Refusals {
		if !slices.Contains(scan.existingDirs, scan.refusalAgentPaths[index]) {
			continue
		}
		workTrees = append(workTrees, withOwnerPaths(scan.refusalOwnerPaths[index], scan.refusalAgentPaths[index],
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
		WorkTrees: workTrees}, nextCache
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
