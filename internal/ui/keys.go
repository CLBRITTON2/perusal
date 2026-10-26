package ui

import (
	"slices"

	"charm.land/bubbles/v2/key"
)

// A binding with no help text is the second half of a pair the first one documents, such as j/k.
var (
	downKey           = key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/k", "select"))
	upKey             = key.NewBinding(key.WithKeys("k", "up"))
	stageKey          = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "stage"))
	stageAllKey       = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "stage all"))
	discardKey        = key.NewBinding(key.WithKeys("x"), key.WithHelp("x/X", "discard"))
	discardAllKey     = key.NewBinding(key.WithKeys("X"))
	confirmDiscardKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "discard"))
	commitKey         = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "commit"))
	pushKey           = key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "push"))
	confirmPushKey    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "push"))
	confirmForceKey   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "force push"))
	amendKey          = key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "amend"))
	confirmAmendKey   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "amend"))
	rewordKey         = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reword"))
	switchViewKey     = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "changes/log"))
	scrollDownKey     = key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d/^u", "scroll"))
	scrollUpKey       = key.NewBinding(key.WithKeys("ctrl+u", "pgup"))
	fullscreenKey     = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "fullscreen"))
	wholeFileKey      = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "whole file"))
	unifiedKey        = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unified"))
	nextChangeKey     = key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next change"))
	previousChangeKey = key.NewBinding(key.WithKeys("N"))
	overviewKey       = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "overview"))
	openRepositoryKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	pullKey           = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pull"))
	fetchKey          = key.NewBinding(key.WithKeys("f"), key.WithHelp("f/F", "fetch"))
	fetchAllKey       = key.NewBinding(key.WithKeys("F"))
	closeOverviewKey  = key.NewBinding(key.WithKeys("esc", "o"), key.WithHelp("esc", "close"))
	branchesKey       = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "branches"))
	switchBranchKey   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch"))
	deleteBranchKey   = key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete"))
	confirmDeleteKey  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "delete"))
	closeBranchesKey  = key.NewBinding(key.WithKeys("esc", "b"), key.WithHelp("esc", "close"))
	workTreesKey      = key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "work trees"))
	closeWorkTreesKey = key.NewBinding(key.WithKeys("esc", "w"), key.WithHelp("esc", "close"))
	quitKey           = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	forceQuitKey      = key.NewBinding(key.WithKeys("ctrl+c"))
	nextFieldKey      = key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "next field"))
	submitKey         = key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "commit"))
	submitRewordKey   = key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "reword"))
	enterKey          = key.NewBinding(key.WithKeys("enter"))
	cancelKey         = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	expandKey         = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand"))
	searchKey         = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	keepSearchKey     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep"))
	clearSearchKey    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear"))
	keyListKey        = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "all keys"))
	closeKeyListKey   = key.NewBinding(key.WithKeys("esc", "?"), key.WithHelp("esc", "close"))
)

var (
	changesKeys = []key.Binding{downKey, stageKey, stageAllKey, discardKey}
	logKeys     = []key.Binding{downKey, expandKey, searchKey, amendKey, rewordKey}
	diffKeys    = []key.Binding{scrollDownKey, nextChangeKey, fullscreenKey, wholeFileKey, unifiedKey}
	reviewKeys  = []key.Binding{commitKey, pushKey, switchViewKey, branchesKey, workTreesKey, overviewKey, quitKey}
	// Not every key fits one line, so the footer leaves the rest to the key list.
	reviewHelp    = []key.Binding{commitKey, pushKey, switchViewKey, keyListKey, quitKey}
	changesHelp   = slices.Concat(changesKeys, reviewHelp)
	logHelp       = slices.Concat(logKeys, reviewHelp)
	overviewHelp  = []key.Binding{downKey, openRepositoryKey, fetchKey, pullKey, pushKey, closeOverviewKey, quitKey}
	branchesHelp  = []key.Binding{downKey, switchBranchKey, deleteBranchKey, closeBranchesKey, quitKey}
	workTreesHelp = []key.Binding{downKey, openRepositoryKey, closeWorkTreesKey, quitKey}
	searchHelp    = []key.Binding{keepSearchKey, clearSearchKey}
	commitHelp    = []key.Binding{nextFieldKey, submitKey, cancelKey}
	rewordHelp    = []key.Binding{nextFieldKey, submitRewordKey, cancelKey}
	keyListHelp   = []key.Binding{closeKeyListKey}
)
