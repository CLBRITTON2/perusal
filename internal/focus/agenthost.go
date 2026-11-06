//go:build windows

package focus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
)

// FollowAgentHost reads the focus file on rules.AgentHost over ssh every interval, since no change notice crosses ssh,
// and sends a Result when what it reads changes. After each read it moves the agent's copy of every focused repository
// onto the owner's HEAD and mirrors it into the owner's, and sends a failure on the second channel when it differs from that repository's last
// one. Both channels close when ctx ends.
func FollowAgentHost(ctx context.Context, rules Rules, interval time.Duration) (<-chan Result, <-chan error) {
	results := make(chan Result)
	mirrorFailures := make(chan error)
	go pollAgentHost(ctx, rules, interval, results, mirrorFailures)
	return results, mirrorFailures
}

func pollAgentHost(ctx context.Context, rules Rules, interval time.Duration, results chan<- Result,
	mirrorFailures chan<- error) {
	defer close(results)
	defer close(mirrorFailures)
	// UTF-8 without a BOM, since Windows PowerShell writes the OEM code page by default.
	readScript := "[Console]::OutputEncoding = New-Object Text.UTF8Encoding $false; [IO.File]::ReadAllText(" +
		agenthost.QuotedString(rules.FocusFile) + ")"
	lastContent := ""
	lastReadFailure := ""
	focused := Focus{Repositories: []gitrepo.Repository{}, Refusals: []error{}}
	lastMirrorFailures := map[string]string{}
	for {
		output, err := agenthost.Run(ctx, rules.AgentHost, readScript)
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
			var resolveErr error
			focused, resolveErr = resolveContent(ctx, rules, lastContent)
			if !sendResult(ctx, results, Result{Focus: focused, Err: resolveErr}) {
				return
			}
		}
		mirrorErrs := make([]error, len(focused.Repositories))
		var group sync.WaitGroup
		for index, repository := range focused.Repositories {
			group.Go(func() {
				// The agent's copy sits at the owner's path on the agent host.
				agent := gitrepo.AgentCopy{Dir: repository.Dir, Remote: rules.AgentHost + ":" + repository.Dir,
					Run: func(ctx context.Context, script string) ([]byte, error) {
						return agenthost.Run(ctx, rules.AgentHost, script)
					}}
				if err := repository.SyncAgentCopy(ctx, agent); err != nil {
					mirrorErrs[index] = err
					return
				}
				mirrorErrs[index] = repository.MirrorAgentWorkTree(ctx, agent.Remote)
			})
		}
		group.Wait()
		for index, repository := range focused.Repositories {
			if mirrorErrs[index] == nil {
				delete(lastMirrorFailures, repository.Dir)
				continue
			}
			failure := mirrorErrs[index].Error()
			if failure != lastMirrorFailures[repository.Dir] {
				select {
				case <-ctx.Done():
					return
				case mirrorFailures <- mirrorErrs[index]:
				}
			}
			lastMirrorFailures[repository.Dir] = failure
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}
