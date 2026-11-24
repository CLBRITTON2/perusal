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

func readOwnerState(ctx context.Context, rules Rules, run func(ctx context.Context, script string) ([]byte, error),
	readOwnerWorkTree OwnerWorkTreeReader, focused []gitrepo.Repository, takeFailure string) (OwnerState, error) {
	found, err := FindRepositories(ctx, rules)
	if err != nil {
		return OwnerState{}, err
	}
	repositoryAgentClones := make([]string, len(found.Repositories))
	repositoryAgentPaths := make([]string, len(found.Repositories))
	for index, repository := range found.Repositories {
		clone := repository.Repository.Dir
		if !strings.EqualFold(filepath.Clean(repository.Repository.GitDir), filepath.Join(clone, ".git")) {
			clone = filepath.Dir(filepath.Dir(filepath.Dir(repository.Repository.GitDir)))
		}
		if repositoryAgentClones[index], err = agentCopyDir(rules, clone); err != nil {
			return OwnerState{}, err
		}
		if repositoryAgentPaths[index], err = agentCopyDir(rules, repository.Repository.Dir); err != nil {
			return OwnerState{}, err
		}
	}
	refusalOwnerPaths := make([]string, len(found.Refusals))
	refusalAgentPaths := make([]string, len(found.Refusals))
	for index, refusal := range found.Refusals {
		refusalOwnerPaths[index] = filepath.Join(rules.OwnerDir, filepath.FromSlash(refusal.RelativePath))
		if refusalAgentPaths[index], err = agentCopyDir(rules, refusalOwnerPaths[index]); err != nil {
			return OwnerState{}, err
		}
	}
	candidateDirs := slices.Compact(slices.Sorted(slices.Values(slices.Concat(repositoryAgentClones, refusalAgentPaths))))
	existingDirs, err := existingAgentDirs(ctx, run, candidateDirs)
	if err != nil {
		return OwnerState{}, err
	}
	sharedIndexes := []int{}
	for index := range found.Repositories {
		if slices.Contains(existingDirs, repositoryAgentClones[index]) {
			sharedIndexes = append(sharedIndexes, index)
		}
	}
	readAt := time.Now()
	workTrees := make([]OwnerWorkTree, len(sharedIndexes))
	var group sync.WaitGroup
	for position, index := range sharedIndexes {
		repository := found.Repositories[index]
		group.Go(func() {
			workTree, err := readOwnerWorkTree(ctx, repository)
			if err == nil && workTree.Branch != "" {
				workTree.TakenCommit, err = repository.Repository.TakenCommit(ctx, workTree.Branch)
			}
			if err != nil {
				workTree = refusedOwnerWorkTree(err)
			}
			workTrees[position] = withOwnerPaths(filepath.Clean(repository.Repository.Dir), repositoryAgentPaths[index],
				workTree)
		})
	}
	group.Wait()
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
		WorkTrees: workTrees}, nil
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
