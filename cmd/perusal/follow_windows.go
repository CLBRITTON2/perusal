package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/ui"
)

const followUsage = "  perusal follow                  review the repositories the agent's focus file names\n"

func runFollow(args []string) error {
	configPath, err := followConfigPath()
	if err != nil {
		return err
	}
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

// The config names the account whose .git access is checked, so it must sit where the agent cannot write.
func followConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the config directory: %w", err)
	}
	return filepath.Join(configDir, "perusal", "config.json"), nil
}

func follow(rules focus.Rules) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results, err := focus.Follow(ctx, rules)
	if err != nil {
		return err
	}
	statePath, err := viewStatePath()
	if err != nil {
		return err
	}
	model, err := startingModel(func(ctx context.Context) (focus.Found, error) {
		return focus.FindRepositories(ctx, rules)
	}, statePath)
	if err != nil {
		return err
	}
	program := tea.NewProgram(model)
	go forwardResults(program, results, rules.FocusFile)
	if err := runKeepingViewState(program, statePath); err != nil {
		return fmt.Errorf("run perusal following %s: %w", rules.FocusFile, err)
	}
	return nil
}

func forwardResults(program *tea.Program, results <-chan focus.Result, focusFile string) {
	for result := range results {
		if result.Err != nil {
			program.Send(ui.FocusRejection{Reason: result.Err.Error()})
		} else {
			program.Send(result.Focus)
		}
	}
	program.Send(ui.FocusRejection{Reason: "stopped watching focus file " + focusFile})
}
