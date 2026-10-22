package main

import (
	"path/filepath"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/ui"
)

func TestTheViewStateSavedOnQuitLoadsOnTheNextRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perusal", "state.json")
	firstRun, err := loadViewState(path)
	if err != nil || firstRun != (ui.ViewState{}) {
		t.Fatalf("got %+v, %v on the first run, want the default view", firstRun, err)
	}
	saved := ui.ViewState{LogView: true, FullscreenDiff: false, WholeFile: true, Unified: true}

	if err := saveViewState(path, saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadViewState(path)

	if err != nil || loaded != saved {
		t.Fatalf("got %+v, %v, want %+v", loaded, err, saved)
	}
}
