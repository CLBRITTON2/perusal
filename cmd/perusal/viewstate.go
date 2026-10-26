package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/CLBRITTON2/perusal/internal/ui"
)

func viewStatePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the config directory: %w", err)
	}
	return filepath.Join(configDir, "perusal", "state.json"), nil
}

// A missing file is a first run, so it gives the default view.
func loadViewState(path string) (ui.ViewState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ui.ViewState{}, nil
	}
	if err != nil {
		return ui.ViewState{}, fmt.Errorf("read the view state: %w", err)
	}
	var viewState ui.ViewState
	if err := json.Unmarshal(data, &viewState); err != nil {
		return ui.ViewState{}, fmt.Errorf("parse the view state %s, delete it to start over: %w", path, err)
	}
	return viewState, nil
}

func saveViewState(path string, viewState ui.ViewState) error {
	data, err := json.MarshalIndent(viewState, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the view state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create the view state directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write the view state: %w", err)
	}
	return nil
}

func startingModel(findRepositories ui.RepositoryFinder, openRepository ui.RepositoryOpener, statePath string) (
	ui.Model, error,
) {
	viewState, err := loadViewState(statePath)
	if err != nil {
		return ui.Model{}, err
	}
	return ui.New(findRepositories, openRepository).WithViewState(viewState), nil
}

func runKeepingViewState(program *tea.Program, statePath string) error {
	final, err := program.Run()
	if err != nil {
		return err
	}
	model, isModel := final.(ui.Model)
	if !isModel {
		return fmt.Errorf("perusal quit with a %T instead of its model, so the view state was not saved", final)
	}
	return saveViewState(statePath, model.ViewState())
}
