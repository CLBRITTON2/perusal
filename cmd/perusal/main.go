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
)

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
	flags := flag.NewFlagSet("perusal", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage:\n"+
			"  perusal [-repo <dir>]           review the working tree of the current directory, or of <dir>\n"+
			followUsage+"\n")
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

func review(repositoryPath string) error {
	repository, err := gitrepo.Open(context.Background(), gitrepo.Repository{Dir: repositoryPath})
	if err != nil {
		return err
	}
	statePath, err := viewStatePath()
	if err != nil {
		return err
	}
	model, err := startingModel(func(context.Context) (focus.Found, error) {
		return focus.Found{}, errors.New("the overview lists the repositories under the config's ownerDir, " +
			"so it needs perusal follow")
	}, func(ctx context.Context, dir string) (gitrepo.Repository, error) {
		return gitrepo.Open(ctx, gitrepo.Repository{Dir: dir})
	}, statePath)
	if err != nil {
		return err
	}
	program := tea.NewProgram(model)
	go program.Send(focus.Focus{Repositories: []gitrepo.Repository{repository}, Refusals: []error{}})
	if err := runKeepingViewState(program, statePath); err != nil {
		return fmt.Errorf("run perusal on %s: %w", repository.Dir, err)
	}
	return nil
}
