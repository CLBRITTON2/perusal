package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/agenthost"
	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/ui"
)

// The longest a wait for a change on the VM lasts. Each wait ends with a focus read and a HEAD check of every focused
// repository, and only a change starts the snapshot, sync and mirror.
const agentHostWaitTimeout = 2 * time.Second

const followUsage = "  perusal follow                  review the repositories the agent's focus file names\n"

func runFollow(args []string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("find the config directory: %w", err)
	}
	// The config names the account whose .git access is checked and the host perusal runs scripts on, so it must sit
	// where the agent cannot write.
	configPath := filepath.Join(configDir, "perusal", "config.json")
	flags := flag.NewFlagSet("perusal follow", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage: perusal follow\n\n"+
			"Reviews the repositories the agent's focus file names, as set up in "+configPath+".\n")
	}
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("perusal follow takes no arguments, it reads %s, got %q", configPath, flags.Args())
	}
	config, err := focus.LoadConfig(configPath)
	if err != nil {
		return err
	}
	rules, err := focus.NewRules(config)
	if err != nil {
		return err
	}
	return follow(rules)
}

func follow(rules focus.Rules) (followErr error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var results <-chan focus.Result
	var mirrorFailures <-chan string
	if rules.AgentHost == "" {
		watched, err := focus.Follow(ctx, rules)
		if err != nil {
			return err
		}
		results = watched
	} else {
		session := agenthost.NewSession(rules.AgentHost)
		defer func() {
			// The poll's Run returns once ctx ends, so Close does not wait on it.
			cancel()
			followErr = errors.Join(followErr, session.Close())
		}()
		results, mirrorFailures = focus.FollowAgentHost(ctx, rules, session, agentHostWaitTimeout)
	}
	statePath, err := viewStatePath()
	if err != nil {
		return err
	}
	model, err := startingModel(func(ctx context.Context) (focus.Found, error) {
		return focus.FindRepositories(ctx, rules)
	}, func(ctx context.Context, dir string) (gitrepo.Repository, error) {
		return focus.OpenWorkTree(ctx, rules, dir)
	}, statePath)
	if err != nil {
		return err
	}
	program := tea.NewProgram(model)
	go forwardResults(program, results, mirrorFailures, rules.FocusFile)
	if err := runKeepingViewState(program, statePath); err != nil {
		return fmt.Errorf("run perusal following %s: %w", rules.FocusFile, err)
	}
	return nil
}

// forwardResults returns when results closes. mirrorFailures is nil when the agent works locally, which blocks its case.
func forwardResults(program *tea.Program, results <-chan focus.Result, mirrorFailures <-chan string, focusFile string) {
	for {
		select {
		case result, open := <-results:
			if !open {
				program.Send(ui.FocusRejection{Reason: "stopped watching focus file " + focusFile})
				return
			}
			if result.Err != nil {
				program.Send(ui.FocusRejection{Reason: result.Err.Error()})
			} else {
				program.Send(result.Focus)
			}
		case failure, open := <-mirrorFailures:
			if !open {
				mirrorFailures = nil
				continue
			}
			program.Send(ui.MirrorFailure{Reason: failure})
		}
	}
}
