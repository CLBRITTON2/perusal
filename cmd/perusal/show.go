package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/diff"
	"github.com/CLBRITTON2/perusal/internal/gitrepo"
	"github.com/CLBRITTON2/perusal/internal/highlight"
	"github.com/CLBRITTON2/perusal/internal/ui"
)

const showUsage = "  perusal show -old <file> -new <file> -label <path> -note <file>\n" +
	"                                  show the diff of two files under a note, as herdr's diff popup does\n"

func runShow(args []string) error {
	flags := flag.NewFlagSet("perusal show", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), "usage:\n"+showUsage+"\n")
		flags.PrintDefaults()
	}
	oldPath := flags.String("old", "", "the file as it is")
	newPath := flags.String("new", "", "the file as proposed")
	label := flags.String("label", "", "the path the diff is titled with, which also picks the syntax highlighting")
	notePath := flags.String("note", "", "a text file shown above the diff")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q, see perusal show -h", flags.Args())
	}
	if *oldPath == "" || *newPath == "" || *label == "" || *notePath == "" {
		return errors.New("perusal show needs -old, -new, -label and -note, see perusal show -h")
	}
	note, err := os.ReadFile(*notePath)
	if err != nil {
		return fmt.Errorf("read the note: %w", err)
	}
	text, err := gitrepo.FilesDiff(context.Background(), *oldPath, *newPath)
	if err != nil {
		return err
	}
	fileDiff, err := diff.ParseFile(text)
	if err != nil {
		return fmt.Errorf("parse diff of %s and %s: %w", *oldPath, *newPath, err)
	}
	highlighted, err := highlight.FileDiff(*label, fileDiff)
	if err != nil {
		return err
	}
	if _, err := tea.NewProgram(ui.NewShow(*label, string(note), highlighted)).Run(); err != nil {
		return fmt.Errorf("show %s: %w", *label, err)
	}
	return nil
}
