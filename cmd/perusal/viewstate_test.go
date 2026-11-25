package main

import (
	"path/filepath"
	"testing"

	"github.com/CLBRITTON2/perusal/internal/ui"
	"github.com/stretchr/testify/require"
)

func TestTheViewStateSavedOnQuitLoadsOnTheNextRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perusal", "state.json")
	firstRun, err := loadViewState(path)
	require.NoError(t, err)
	require.Zero(t, firstRun)
	saved := ui.ViewState{LogView: true, FullscreenDiff: false, WholeFile: true, Unified: true}

	err = saveViewState(path, saved)
	require.NoError(t, err)
	loaded, err := loadViewState(path)

	require.NoError(t, err)
	require.Equal(t, saved, loaded)
}
