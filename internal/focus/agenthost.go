//go:build windows

package focus

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// FollowAgentHost waits through session, up to waitTimeout at a time, for a change to the focus file on rules.AgentHost
// or to the agent's copy of a focused repository, then reads the focus file and sends a Result when what it reads
// changed. When the agent's side changed, it takes the agent's new commits in every focused repository into the owner's
// work tree. When the failed takes change, it sends their errors joined on the second channel, and an empty string once
// every take succeeds again. Both channels close when ctx ends.
func FollowAgentHost(ctx context.Context, rules Rules, session *agenthost.Session,
	waitTimeout time.Duration) (<-chan Result, <-chan string) {
	results := make(chan Result)
	takeFailures := make(chan string)
	go pollAgentHost(ctx, rules, session, waitTimeout, results, takeFailures)
	return results, takeFailures
}

func pollAgentHost(ctx context.Context, rules Rules, session *agenthost.Session, waitTimeout time.Duration,
	results chan<- Result, takeFailures chan<- string) {
	defer close(results)
	defer close(takeFailures)
	readScript := "[IO.File]::ReadAllText(" + agenthost.QuotedString(rules.FocusFile) + ")"
	lastContent := ""
	lastReadFailure := ""
	focused := Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{}}
	focusedAgentDirs := []string{}
	lastTakeFailure := ""
	for {
		changed, waitErr := session.WaitForChange(ctx, focusedAgentDirs, rules.FocusFile, waitTimeout)
		if ctx.Err() != nil {
			return
		}
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
			focusedAgentDirs = []string{}
			if readErr.Error() != lastReadFailure && !sendResult(ctx, results, Result{Err: readErr}) {
				return
			}
			lastReadFailure = readErr.Error()
		} else if string(output) != lastContent {
			lastContent = string(output)
			lastReadFailure = ""
			changed = true
			resolved, resolveErr := resolveContent(ctx, rules, lastContent)
			resolvedAgentDirs, agentDirsErr := agentCopyDirs(rules, resolved.Repositories)
			if agentDirsErr != nil {
				focused = Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{}}
				focusedAgentDirs = []string{}
			} else {
				focused = resolved
				focusedAgentDirs = resolvedAgentDirs
			}
			if !sendResult(ctx, results, Result{Focus: focused, Err: errors.Join(resolveErr, agentDirsErr)}) {
				return
			}
		}
		// A failed take is retried every poll, since the owner may fix it without the agent's side changing.
		if !changed && waitErr == nil && lastTakeFailure == "" {
			continue
		}
		failures := []string{}
		if waitErr != nil {
			failures = append(failures, waitErr.Error())
		}
		failures = append(failures, takeAgentCommits(ctx, rules, session, focused.Repositories, focusedAgentDirs)...)
		takeFailure := strings.Join(failures, ", ")
		if takeFailure != lastTakeFailure {
			select {
			case <-ctx.Done():
				return
			case takeFailures <- takeFailure:
			}
			lastTakeFailure = takeFailure
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

// takeAgentCommits brings the agent's new commits in each focused repository into the owner's, leaving the agent's
// copies as they are, and returns what failed.
func takeAgentCommits(ctx context.Context, rules Rules, session *agenthost.Session, repositories []gitrepo.Repository,
	agentDirs []string) []string {
	heads, err := gitrepo.ReadAgentHeads(ctx, session.Run, agentDirs)
	if err != nil {
		return []string{err.Error()}
	}
	takeErrs := make([]error, len(repositories))
	var group sync.WaitGroup
	for index, repository := range repositories {
		group.Go(func() {
			_, takeErrs[index] = repository.TakeAgentCommits(ctx, rules.AgentHost+":"+agentDirs[index], heads[index])
		})
	}
	group.Wait()
	failures := []string{}
	for _, takeErr := range takeErrs {
		if takeErr != nil {
			failures = append(failures, takeErr.Error())
		}
	}
	return failures
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
