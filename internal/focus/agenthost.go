//go:build windows

package focus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// FollowAgentHost waits through session, up to waitTimeout at a time, for a change to the focus file on rules.AgentHost
// or to the agent's copy of a paired work tree, then reads the focus file and sends a Result when what it reads
// changed. It takes the agent's new commits into each paired work tree whose agent copy changed. A paired work tree is
// one under rules.OwnerDir whose agent copy exists at the same path under rules.AgentDir, found again when the focus
// changes and every 15 seconds. When the failed takes and refused paired work trees change, it sends their errors
// joined on the second channel, and an empty string once every take succeeds again. While a focused path has no owner
// copy, it resolves the focus file every poll, so a worktree the owner adds, by hand or with CreateWorkTree, is picked
// up with no change to the focus file. Every channel closes when ctx ends.
func FollowAgentHost(ctx context.Context, rules Rules, session *agenthost.Session, waitTimeout time.Duration,
	readOwnerWorkTree OwnerWorkTreeReader) (<-chan Result, <-chan string, <-chan string) {
	results := make(chan Result)
	takeFailures := make(chan string)
	ownerStateFailures := make(chan string)
	go pollAgentHost(ctx, rules, session, waitTimeout, readOwnerWorkTree, results, takeFailures, ownerStateFailures)
	return results, takeFailures, ownerStateFailures
}

// lacksOwnerCopy reports whether a focused path resolved to no owner work tree, which the owner can fix without the
// focus file changing.
func lacksOwnerCopy(resolved Focus, resolveErr error) bool {
	return len(resolved.MissingWorkTrees) > 0 || errors.Is(resolveErr, ErrNotWorkTree) ||
		slices.ContainsFunc(resolved.Refusals, func(refusal error) bool { return errors.Is(refusal, ErrNotWorkTree) })
}

// isSameResult reports whether two resolves found the same, so a repeated resolve sends nothing that would reset the
// footer.
func isSameResult(previous Result, next Result) bool {
	return slices.Equal(previous.Focus.Repositories, next.Focus.Repositories) &&
		slices.Equal(previous.Focus.MissingWorkTrees, next.Focus.MissingWorkTrees) &&
		slices.EqualFunc(previous.Focus.Refusals, next.Focus.Refusals, isSameError) &&
		isSameError(previous.Err, next.Err)
}

func isSameError(previous error, next error) bool {
	if previous == nil || next == nil {
		return previous == next
	}
	return previous.Error() == next.Error()
}

// CreateWorkTree adds the owner's copy of missing at missing.Dir, on the branch the agent's copy has checked out, and
// returns the commit that branch starts at.
func CreateWorkTree(ctx context.Context, rules Rules, session *agenthost.Session, missing MissingWorkTree) (string,
	error) {
	heads, err := gitrepo.ReadAgentHeads(ctx, session.Run, []string{missing.AgentDir})
	if err != nil {
		return "", err
	}
	return missing.Clone.AddWorkTreeFromAgent(ctx, rules.AgentHost+":"+missing.AgentDir, heads[0], missing.Dir)
}

// ReadAgentWorkTree reads the agent's copy of the owner's work tree at ownerWorkTree.
func ReadAgentWorkTree(ctx context.Context, rules Rules, session *agenthost.Session,
	ownerWorkTree string) (gitrepo.AgentWorkTree, error) {
	agentDirs, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: ownerWorkTree}})
	if err != nil {
		return gitrepo.AgentWorkTree{}, err
	}
	return gitrepo.ReadAgentWorkTree(ctx, session.Run, agentDirs[0])
}

// RemoveAgentWorkTree removes agentWorkTree from the agent's copy of the owner's clone at ownerClone.
func RemoveAgentWorkTree(ctx context.Context, rules Rules, session *agenthost.Session, ownerClone string,
	agentWorkTree gitrepo.AgentWorkTree) error {
	agentDirs, err := agentCopyDirs(rules, []gitrepo.Repository{{Dir: ownerClone}})
	if err != nil {
		return err
	}
	return gitrepo.RemoveAgentWorkTree(ctx, session.Run, agentDirs[0], agentWorkTree)
}

// existingOnAgentHost keeps the missing worktrees whose agent copy is still on the agent host, since the focus file
// keeps listing a worktree after it was removed there, and offering to copy it would fail. It runs nothing when
// missingWorkTrees is empty.
func existingOnAgentHost(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error),
	missingWorkTrees []MissingWorkTree) ([]MissingWorkTree, error) {
	if len(missingWorkTrees) == 0 {
		return []MissingWorkTree{}, nil
	}
	agentDirs := make([]string, len(missingWorkTrees))
	for index, missing := range missingWorkTrees {
		agentDirs[index] = missing.AgentDir
	}
	existingDirs, err := existingAgentDirs(ctx, run, agentDirs)
	if err != nil {
		return []MissingWorkTree{}, err
	}
	existing := []MissingWorkTree{}
	for _, missing := range missingWorkTrees {
		if slices.Contains(existingDirs, missing.AgentDir) {
			existing = append(existing, missing)
		}
	}
	return existing, nil
}

func existingAgentDirs(ctx context.Context, run func(ctx context.Context, script string) ([]byte, error),
	agentDirs []string) ([]string, error) {
	if len(agentDirs) == 0 {
		return []string{}, nil
	}
	quotedDirs := make([]string, len(agentDirs))
	for index, dir := range agentDirs {
		quotedDirs[index] = agenthost.QuotedString(dir)
	}
	script := "ConvertTo-Json -Compress -InputObject @(foreach ($dir in @(" + strings.Join(quotedDirs, ", ") +
		")) { Test-Path -LiteralPath $dir -PathType Container })"
	output, err := run(ctx, script)
	if err != nil {
		return []string{}, fmt.Errorf("check which of the agent host's folders %s exist: %w",
			strings.Join(quotedDirs, ", "), err)
	}
	var exists []bool
	if err := json.Unmarshal(output, &exists); err != nil || len(exists) != len(agentDirs) {
		return []string{}, fmt.Errorf("check which of the agent host's folders %s exist: want %d booleans, "+
			"got %q: %v", strings.Join(quotedDirs, ", "), len(agentDirs), output, err)
	}
	existing := []string{}
	for index, dir := range agentDirs {
		if exists[index] {
			existing = append(existing, dir)
		}
	}
	return existing, nil
}

func pollAgentHost(ctx context.Context, rules Rules, session *agenthost.Session, waitTimeout time.Duration,
	readOwnerWorkTree OwnerWorkTreeReader, results chan<- Result, takeFailures chan<- string,
	ownerStateFailures chan<- string) {
	defer close(results)
	defer close(takeFailures)
	defer close(ownerStateFailures)
	readScript := "[IO.File]::ReadAllText(" + agenthost.QuotedString(rules.FocusFile) + ")"
	lastContent := ""
	lastReadFailure := ""
	lastResult := Result{}
	resolvesEachPoll := false
	focused := Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{}}
	scan := emptyOwnerScan()
	var scanErr error
	scannedAt := time.Time{}
	paired := scan.paired()
	// The owner may fix a failed take without the agent's side changing, so each is retried every poll.
	failedTakes := map[string]string{}
	lastTakeFailure := ""
	lastOwnerStateFailure := ""
	cachedOwnerWorkTrees := map[string]stampedOwnerWorkTree{}
	for {
		changedDirs, waitErr := session.WaitForChangedDirs(ctx, paired.agentDirs, rules.FocusFile, waitTimeout)
		if ctx.Err() != nil {
			return
		}
		focusChanged := false
		output, err := session.Run(ctx, readScript)
		var readErr error
		switch {
		case err != nil:
			readErr = fmt.Errorf("read focus file %s: %w", rules.FocusFile, err)
		case len(output) > maximumPointerFileBytes:
			readErr = fmt.Errorf("%w: %s on %s is over %d bytes", ErrMalformedFocusFile, rules.FocusFile, rules.AgentHost,
				maximumPointerFileBytes)
		}
		if readErr != nil {
			lastContent = ""
			focused = Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{}}
			if readErr.Error() != lastReadFailure && !sendResult(ctx, results, Result{Err: readErr}) {
				return
			}
			lastReadFailure = readErr.Error()
		} else if string(output) != lastContent || resolvesEachPoll {
			contentChanged := string(output) != lastContent
			lastContent = string(output)
			lastReadFailure = ""
			resolvedPaths, resolveErr := resolveContent(ctx, rules, lastContent)
			offeredWorkTrees, offeredErr := existingOnAgentHost(ctx, session.Run, resolvedPaths.MissingWorkTrees)
			focused = Focus{Repositories: resolvedPaths.Repositories, Refusals: resolvedPaths.Refusals,
				MissingWorkTrees: offeredWorkTrees}
			resolvesEachPoll = lacksOwnerCopy(focused, resolveErr)
			resolvedResult := Result{Focus: focused, Err: errors.Join(resolveErr, offeredErr)}
			if contentChanged || !isSameResult(lastResult, resolvedResult) {
				focusChanged = true
				if !sendResult(ctx, results, resolvedResult) {
					return
				}
			}
			lastResult = resolvedResult
		}
		scanDue := focusChanged || time.Since(scannedAt) >= 15*time.Second
		if scanDue {
			scannedAt = time.Now()
			nextScan, err := scanOwnerDir(ctx, rules, session.Run, scan)
			scanErr = err
			// A failed scan keeps the last paired work trees.
			if err == nil {
				scan = nextScan
				paired = scan.paired()
				maps.DeleteFunc(failedTakes, func(agentDir string, failure string) bool {
					return !slices.Contains(paired.agentDirs, agentDir)
				})
			}
		}
		takenRepositories := []gitrepo.Repository{}
		takenAgentDirs := []string{}
		for index, agentDir := range paired.agentDirs {
			_, failed := failedTakes[agentDir]
			if waitErr != nil || failed || slices.Contains(changedDirs, agentDir) {
				takenRepositories = append(takenRepositories, paired.repositories[index])
				takenAgentDirs = append(takenAgentDirs, agentDir)
			}
		}
		takeErrs, movedWorkTree := takeAgentCommits(ctx, rules, session, takenRepositories, takenAgentDirs)
		for index, agentDir := range takenAgentDirs {
			if takeErrs[index] == nil {
				delete(failedTakes, agentDir)
			} else {
				failedTakes[agentDir] = takeErrs[index].Error()
			}
		}
		failures := slices.Clone(paired.refusals)
		if waitErr != nil {
			failures = append(failures, waitErr.Error())
		}
		if scanErr != nil {
			failures = append(failures, scanErr.Error())
		}
		for _, agentDir := range paired.agentDirs {
			failure, failed := failedTakes[agentDir]
			// A failed read of the agent's HEADs fails every take with the same error, shown once.
			if failed && !slices.Contains(failures, failure) {
				failures = append(failures, failure)
			}
		}
		takeFailure := strings.Join(failures, ", ")
		if takeFailure != lastTakeFailure {
			select {
			case <-ctx.Done():
				return
			case takeFailures <- takeFailure:
			}
			lastTakeFailure = takeFailure
		}
		if rules.OwnerStateFile != "" && (movedWorkTree || scanDue) {
			ownerStateErr := scanErr
			if scanErr == nil {
				var state OwnerState
				state, cachedOwnerWorkTrees = readOwnerState(ctx, readOwnerWorkTree, scan, focused.Repositories,
					lastTakeFailure, cachedOwnerWorkTrees)
				ownerStateErr = writeOwnerState(ctx, session.Run, rules.OwnerStateFile, state)
			}
			ownerStateFailure := ""
			if ownerStateErr != nil {
				ownerStateFailure = "owner state not written: " + ownerStateErr.Error()
			}
			if ownerStateFailure != lastOwnerStateFailure {
				select {
				case <-ctx.Done():
					return
				case ownerStateFailures <- ownerStateFailure:
				}
				lastOwnerStateFailure = ownerStateFailure
			}
		}
		if waitErr != nil {
			// The failed wait returned at once.
			select {
			case <-ctx.Done():
				return
			case <-time.After(waitTimeout):
			}
		}
	}
}

// takeAgentCommits brings the agent's new commits in each of repositories into the owner's, leaving the agent's copies
// at agentDirs as they are, and returns each one's error, and whether any work tree moved.
func takeAgentCommits(ctx context.Context, rules Rules, session *agenthost.Session, repositories []gitrepo.Repository,
	agentDirs []string) ([]error, bool) {
	takeErrs := make([]error, len(repositories))
	heads, err := gitrepo.ReadAgentHeads(ctx, session.Run, agentDirs)
	if err != nil {
		for index := range takeErrs {
			takeErrs[index] = err
		}
		return takeErrs, false
	}
	moved := make([]bool, len(repositories))
	var group sync.WaitGroup
	for index, repository := range repositories {
		group.Go(func() {
			moved[index], takeErrs[index] = repository.TakeAgentCommits(ctx, rules.AgentHost+":"+agentDirs[index],
				heads[index])
		})
	}
	group.Wait()
	return takeErrs, slices.Contains(moved, true)
}

// agentCopyDirs are the agent's copies of repositories on the agent host, each at the same path under rules.AgentDir as
// the repository under rules.OwnerDir.
func agentCopyDirs(rules Rules, repositories []gitrepo.Repository) ([]string, error) {
	dirs := make([]string, len(repositories))
	for index, repository := range repositories {
		relativePath, err := filepath.Rel(rules.OwnerDir, repository.Dir)
		if err != nil {
			return nil, fmt.Errorf("map repository %s from %s to %s: %w", repository.Dir, rules.OwnerDir,
				rules.AgentDir, err)
		}
		if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%s is not under %s", repository.Dir, rules.OwnerDir)
		}
		dirs[index] = filepath.Join(rules.AgentDir, relativePath)
	}
	return dirs, nil
}
