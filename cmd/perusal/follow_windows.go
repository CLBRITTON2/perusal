package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/ui"
)

// Each poll costs an ssh login per read and two per focused repository, about 2 s each on the VM.
const agentHostPollInterval = 2 * time.Second

const followUsage = "  perusal follow                  review the repositories the agent's focus file names\n"

func runFollow(args []string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("find the config directory: %w", err)
	}
	// The config names the account whose .git access is checked, so it must sit where the agent cannot write.
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

func follow(rules focus.Rules) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var results <-chan focus.Result
	var mirrorFailures <-chan error
	if rules.AgentHost == "" {
		watched, err := focus.Follow(ctx, rules)
		if err != nil {
			return err
		}
		results = watched
	} else {
		results, mirrorFailures = focus.FollowAgentHost(ctx, rules, agentHostPollInterval)
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
func forwardResults(program *tea.Program, results <-chan focus.Result, mirrorFailures <-chan error, focusFile string) {
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
			program.Send(ui.MirrorFailure{Reason: failure.Error()})
		}
	}
}
