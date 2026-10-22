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

const followUsage = "  perusal follow                  review whichever working tree the agent's focused herdr pane is in\n"

func runFollow(args []string) error {
	configPath, err := followConfigPath()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("perusal follow", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage: perusal follow\n\n"+
			"Reviews whichever working tree the agent's focused herdr pane is in, as set up in "+configPath+".\n")
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
	program := tea.NewProgram(ui.New(func(ctx context.Context) (focus.Found, error) {
		return focus.FindRepositories(ctx, rules)
	}))
	go forwardResults(program, results, rules.FocusFile)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run perusal following %s: %w", rules.FocusFile, err)
	}
	return nil
}

func forwardResults(program *tea.Program, results <-chan focus.Result, focusFile string) {
	for result := range results {
		if result.Err != nil {
			program.Send(ui.FocusRejected{Reason: result.Err.Error()})
		} else {
			program.Send(ui.RepositoriesFocused{Repositories: result.Focus.Repositories, Refusals: result.Focus.Refusals})
		}
	}
	program.Send(ui.FocusRejected{Reason: "stopped watching focus file " + focusFile})
}
