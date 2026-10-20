package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/ui"
)

// Published by windows-dev's herdr/focus-file plugin.
const focusFileName = ".herdr-focus"

func main() {
	err := run(os.Args[1:])
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return
	default:
		fmt.Fprintf(os.Stderr, "perusal: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "follow" {
		return runFollow(args[1:])
	}
	return runReview(args)
}

func runReview(args []string) error {
	flags := flag.NewFlagSet("perusal", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage:\n"+
			"  perusal [-repo <dir>]           review the working tree of the current directory, or of <dir>\n"+
			"  perusal follow <agent account>  review whichever working tree the agent's focused herdr pane is in\n\n")
		flags.PrintDefaults()
	}
	repositoryPath := flags.String("repo", "", "any directory inside the git working tree to review")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q, see perusal -h", flags.Args())
	}
	if *repositoryPath != "" {
		return review(*repositoryPath)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get the current directory: %w", err)
	}
	return review(workingDir)
}

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

func review(repositoryPath string) error {
	repository, err := gitrepo.Open(context.Background(), repositoryPath)
	if err != nil {
		return err
	}
	program := tea.NewProgram(ui.New())
	go program.Send(ui.RepositoryFocused{Repository: repository})
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run perusal on %s: %w", repository.Dir, err)
	}
	return nil
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
