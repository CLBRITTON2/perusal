package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/focus"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/ui"
)

type followSettings struct {
	focusFile    string
	agentDevDir  string
	ownerDevDir  string
	agentAccount string
}

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

// run reviews and commits the changes of one working tree, or of whichever one the agent's focused herdr pane works
// in, until the user quits.
func run(args []string) error {
	flags := flag.NewFlagSet("perusal", flag.ContinueOnError)
	repositoryPath := flags.String("repo", "", "any directory inside the git working tree to review")
	focusFile := flags.String("follow", "", "file holding the directory of the agent's focused herdr pane, to review "+
		"whichever working tree that pane is in")
	agentDevDir := flags.String("agent-dev-dir", "", "with -follow: the dev directory as the agent account's paths in "+
		"the focus file name it")
	ownerDevDir := flags.String("owner-dev-dir", "", "with -follow: the same dev directory as this account reaches it")
	agentAccount := flags.String("agent-account", "", "with -follow: the agent account, whose write deny every "+
		"followed .git must carry")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("perusal takes no arguments, got %q", flags.Args())
	}
	followFlagSet := *agentDevDir != "" || *ownerDevDir != "" || *agentAccount != ""
	switch {
	case *repositoryPath != "" && *focusFile != "":
		return errors.New("pass -repo or -follow, not both")
	case *repositoryPath != "" && followFlagSet:
		return errors.New("-agent-dev-dir, -owner-dev-dir and -agent-account go with -follow, not -repo")
	case *repositoryPath != "":
		return review(*repositoryPath)
	case *focusFile != "" && (*agentDevDir == "" || *ownerDevDir == "" || *agentAccount == ""):
		return errors.New("-follow needs -agent-dev-dir, -owner-dev-dir and -agent-account")
	case *focusFile != "":
		return follow(followSettings{
			focusFile:    *focusFile,
			agentDevDir:  *agentDevDir,
			ownerDevDir:  *ownerDevDir,
			agentAccount: *agentAccount,
		})
	default:
		return errors.New("pass -repo <dir> or -follow <focus file>")
	}
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

func follow(settings followSettings) error {
	agentSID, err := focus.LookupAccountSID(settings.agentAccount)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rules := focus.Rules{AgentDevDir: settings.agentDevDir, OwnerDevDir: settings.ownerDevDir, AgentSID: agentSID}
	results, err := focus.Follow(ctx, rules, settings.focusFile)
	if err != nil {
		return err
	}
	program := tea.NewProgram(ui.New())
	go forwardResults(program, results, settings.focusFile)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run perusal following %s: %w", settings.focusFile, err)
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
