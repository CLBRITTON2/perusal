package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/ui"
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

// run reviews and commits the changes of one working tree until the user quits.
func run(args []string) error {
	flags := flag.NewFlagSet("perusal", flag.ContinueOnError)
	repositoryPath := flags.String("repo", "", "any directory inside the git working tree to review (required)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("perusal takes no arguments, got %q", flags.Args())
	}
	if *repositoryPath == "" {
		return errors.New("-repo is required")
	}
	repository, err := gitrepo.Open(context.Background(), *repositoryPath)
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(ui.New(repository)).Run(); err != nil {
		return fmt.Errorf("run perusal on %s: %w", repository.Dir, err)
	}
	return nil
}
