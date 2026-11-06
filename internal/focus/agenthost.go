//go:build windows

package focus

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// FollowAgentHost waits through session, up to waitTimeout at a time, for a change to the focus file on rules.AgentHost
// or to the agent's copy of a focused repository, then reads the focus file and sends a Result when what it reads
// changed. When the agent's side or the owner's HEAD of a repository changed, it moves the agent's copy of every
// focused repository onto the owner's HEAD and mirrors it into the owner's. When the failed mirrors change, it sends
// their errors joined on the second channel, and an empty string once every mirror succeeds again. Both channels close
// when ctx ends.
func FollowAgentHost(ctx context.Context, rules Rules, session *agenthost.Session,
	waitTimeout time.Duration) (<-chan Result, <-chan string) {
	results := make(chan Result)
	mirrorFailures := make(chan string)
	go pollAgentHost(ctx, rules, session, waitTimeout, results, mirrorFailures)
	return results, mirrorFailures
}

func pollAgentHost(ctx context.Context, rules Rules, session *agenthost.Session, waitTimeout time.Duration,
	results chan<- Result, mirrorFailures chan<- string) {
	defer close(results)
	defer close(mirrorFailures)
	readScript := "[IO.File]::ReadAllText(" + agenthost.QuotedString(rules.FocusFile) + ")"
	lastContent := ""
	lastReadFailure := ""
	focused := Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{}}
	lastMirrorFailure := ""
	lastOwnerHeads := map[string]string{}
	for {
		changed, waitErr := session.WaitForChange(ctx, agentDirs(focused), rules.FocusFile, waitTimeout)
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
			if readErr.Error() != lastReadFailure && !sendResult(ctx, results, Result{Err: readErr}) {
				return
			}
			lastReadFailure = readErr.Error()
		} else if string(output) != lastContent {
			lastContent = string(output)
			lastReadFailure = ""
			changed = true
			var resolveErr error
			focused, resolveErr = resolveContent(ctx, rules, lastContent)
			if !sendResult(ctx, results, Result{Focus: focused, Err: resolveErr}) {
				return
			}
		}
		// The owner's commits, pulls and branch switches happen on this side, where no watcher sees them.
		ownerHeads := map[string]string{}
		for _, repository := range focused.Repositories {
			ownerHead, err := repository.HeadAndBranch(ctx)
			if err != nil {
				// The sync below runs git on the same HEAD and reports the error.
				changed = true
			}
			ownerHeads[repository.Dir] = ownerHead
		}
		failures := []string{}
		if waitErr != nil {
			failures = append(failures, waitErr.Error())
		}
		// A failed mirror is retried every poll, since the owner may fix it without the agent's side changing.
		if !changed && waitErr == nil && lastMirrorFailure == "" && maps.Equal(ownerHeads, lastOwnerHeads) {
			continue
		}
		lastOwnerHeads = ownerHeads
		snapshots, err := gitrepo.SnapshotAgentCopies(ctx, session.Run, agentDirs(focused))
		if err != nil {
			failures = append(failures, err.Error())
		} else {
			mirrorErrs := make([]error, len(focused.Repositories))
			var group sync.WaitGroup
			for index, repository := range focused.Repositories {
				group.Go(func() {
					agent := gitrepo.AgentCopy{Dir: repository.Dir, Remote: rules.AgentHost + ":" + repository.Dir,
						Run: session.Run}
					snapshotCommit, err := repository.SyncAgentCopy(ctx, agent, snapshots[index])
					if err != nil {
						mirrorErrs[index] = err
						return
					}
					mirrorErrs[index] = repository.MirrorAgentWorkTree(ctx, agent.Remote, snapshotCommit)
				})
			}
			group.Wait()
			for _, mirrorErr := range mirrorErrs {
				if mirrorErr != nil {
					failures = append(failures, mirrorErr.Error())
				}
			}
		}
		mirrorFailure := strings.Join(failures, ", ")
		if mirrorFailure != lastMirrorFailure {
			select {
			case <-ctx.Done():
				return
			case mirrorFailures <- mirrorFailure:
			}
			lastMirrorFailure = mirrorFailure
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

// agentDirs are the agent's copies of focused's repositories, which sit at the owner's paths on the agent host.
func agentDirs(focused Focus) []string {
	dirs := make([]string, len(focused.Repositories))
	for index, repository := range focused.Repositories {
		dirs[index] = repository.Dir
	}
	return dirs
}
