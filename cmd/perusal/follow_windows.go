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

// Published by windows-dev's herdr/focus-file plugin.
const focusFileName = ".herdr-focus"

const followUsage = "  perusal follow <agent account>  review whichever working tree the agent's focused herdr pane is in\n"

// Relies on the dev directory layout windows-dev's setup-agent-account.ps1 creates.
func runFollow(args []string) error {
	flags := flag.NewFlagSet("perusal follow", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage: perusal follow <agent account>\n\n"+
			"Reviews whichever working tree the agent's focused herdr pane is in, read from <agent profile>\\dev\\"+
			focusFileName+".\n")
	}
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("perusal follow takes one agent account, as in perusal follow claude, got %q", flags.Args())
	}
	agentAccount := flags.Arg(0)
	agentSID, err := focus.LookupAccountSID(agentAccount)
	if err != nil {
		return err
	}
	agentProfileDir, err := focus.ProfileDirectory(agentSID)
	if err != nil {
		return err
	}
	ownerHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find this account's home directory: %w", err)
	}
	agentDevDir := filepath.Join(agentProfileDir, "dev")
	rules := focus.Rules{AgentDevDir: agentDevDir, OwnerDevDir: filepath.Join(ownerHome, "dev"), AgentSID: agentSID}
	return follow(rules, filepath.Join(agentDevDir, focusFileName))
}

func follow(rules focus.Rules, focusFile string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results, err := focus.Follow(ctx, rules, focusFile)
	if err != nil {
		return err
	}
	program := tea.NewProgram(ui.New())
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
			program.Send(ui.RepositoryFocused{Repository: result.Repository})
		}
	}
	program.Send(ui.FocusRejected{Reason: "stopped watching focus file " + focusFile})
}
