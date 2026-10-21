package ui

import (
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
	switchViewKey     = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "changes/log"))
	scrollDownKey     = key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d/^u", "scroll"))
	scrollUpKey       = key.NewBinding(key.WithKeys("ctrl+u", "pgup"))
	fullscreenKey     = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "fullscreen"))
	overviewKey       = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "overview"))
	openRepositoryKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	pullKey           = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pull"))
	closeOverviewKey  = key.NewBinding(key.WithKeys("esc", "o"), key.WithHelp("esc", "close"))
	branchesKey       = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "branches"))
	switchBranchKey   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch"))
	deleteBranchKey   = key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete"))
	confirmDeleteKey  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "delete"))
	closeBranchesKey  = key.NewBinding(key.WithKeys("esc", "b"), key.WithHelp("esc", "close"))
	quitKey           = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	forceQuitKey      = key.NewBinding(key.WithKeys("ctrl+c"))
	nextFieldKey      = key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "next field"))
	submitKey         = key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "commit"))
	enterKey          = key.NewBinding(key.WithKeys("enter"))
	cancelKey         = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	expandKey         = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand"))
	searchKey         = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	keepSearchKey     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep"))
	clearSearchKey    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear"))
)

var (
	changesHelp      = []key.Binding{downKey, stageKey, stageAllKey, discardKey, commitKey, pushKey, switchViewKey, scrollDownKey, fullscreenKey, branchesKey, overviewKey, quitKey}
	logHelp          = []key.Binding{downKey, expandKey, searchKey, amendKey, commitKey, pushKey, switchViewKey, scrollDownKey, fullscreenKey, branchesKey, overviewKey, quitKey}
	overviewHelp     = []key.Binding{downKey, openRepositoryKey, pullKey, closeOverviewKey, quitKey}
	branchesHelp     = []key.Binding{downKey, switchBranchKey, deleteBranchKey, closeBranchesKey, quitKey}
	searchHelp       = []key.Binding{keepSearchKey, clearSearchKey}
	commitHelp       = []key.Binding{nextFieldKey, submitKey, cancelKey}
	pushHelp         = []key.Binding{confirmPushKey, cancelKey}
	forcePushHelp    = []key.Binding{confirmForceKey, cancelKey}
	amendHelp        = []key.Binding{confirmAmendKey, cancelKey}
	discardHelp      = []key.Binding{confirmDiscardKey, cancelKey}
	deleteBranchHelp = []key.Binding{confirmDeleteKey, cancelKey}
)
