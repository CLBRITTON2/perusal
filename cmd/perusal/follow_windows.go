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
	path, err := followConfigPath()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("perusal follow", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage: perusal follow\n\n"+
			"Reviews whichever working tree the agent's focused herdr pane is in, as set up in "+path+".\n")
	}
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("perusal follow takes no arguments, it reads %s, got %q", path, flags.Args())
	}
	config, err := focus.LoadConfig(path)
	if err != nil {
		return err
	}
	agentSID, err := focus.LookupAccountSID(config.AgentAccount)
	if err != nil {
		return err
	}
	rules := focus.Rules{AgentDir: config.AgentDir, OwnerDir: config.OwnerDir, AgentSID: agentSID}
	return follow(rules, config.FocusFile)
}

// The config names the account whose .git access is checked, so it must sit where the agent cannot write.
func followConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the config directory: %w", err)
	}
	return filepath.Join(configDir, "perusal", "config.json"), nil
}

func follow(rules focus.Rules, focusFile string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results, err := focus.Follow(ctx, rules, focusFile)
	if err != nil {
		return err
	}
	program := tea.NewProgram(ui.New(repositoryLister(rules)))
	go forwardResults(program, results, focusFile)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run perusal following %s: %w", focusFile, err)
	}
	return nil
}

func forwardResults(program *tea.Program, results <-chan focus.Result, focusFile string) {
	for result := range results {
		if result.Err != nil {
			program.Send(ui.FocusRejected{Reason: result.Err.Error()})
		} else {
			program.Send(ui.RepositoriesFocused{Repositories: result.Focus.Repositories, Refusals: refusalReasons(result.Focus.Refusals)})
		}
	}
	program.Send(ui.FocusRejected{Reason: "stopped watching focus file " + focusFile})
}

func repositoryLister(rules focus.Rules) ui.Lister {
	return func(ctx context.Context) (ui.Listing, error) {
		found, err := focus.FindRepositories(ctx, rules)
		if err != nil {
			return ui.Listing{}, err
		}
		listed := []ui.ListedRepository{}
		for _, repository := range found.Repositories {
			listed = append(listed, ui.ListedRepository{Name: repository.Name, Repository: repository.Repository})
		}
		return ui.Listing{Repositories: listed, Refusals: refusalReasons(found.Refusals)}, nil
	}
}

func refusalReasons(refusals []error) []string {
	reasons := []string{}
	for _, refusal := range refusals {
		reasons = append(reasons, refusal.Error())
	}
	return reasons
}
