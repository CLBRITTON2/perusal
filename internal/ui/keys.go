package ui

import (
	"charm.land/bubbles/v2/key"
)

// A binding with no help text is the second half of a pair the first one documents, such as j/k.
var (
	downKey         = key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/k", "select"))
	upKey           = key.NewBinding(key.WithKeys("k", "up"))
	stageKey        = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "stage"))
	stageAllKey     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "stage all"))
	commitKey       = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "commit"))
	pushKey         = key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "push"))
	confirmPushKey  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "push"))
	confirmForceKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "force push"))
	amendKey        = key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "amend"))
	confirmAmendKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "amend"))
	switchViewKey   = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "changes/log"))
	scrollDownKey   = key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d/^u", "scroll"))
	scrollUpKey     = key.NewBinding(key.WithKeys("ctrl+u", "pgup"))
	fullscreenKey   = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "fullscreen"))
	quitKey         = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	forceQuitKey    = key.NewBinding(key.WithKeys("ctrl+c"))
	nextFieldKey    = key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "next field"))
	submitKey       = key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "commit"))
	enterKey        = key.NewBinding(key.WithKeys("enter"))
	cancelKey       = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	expandKey       = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand"))
	searchKey       = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	keepSearchKey   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep"))
	clearSearchKey  = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear"))
)

var (
	changesHelp   = []key.Binding{downKey, stageKey, stageAllKey, commitKey, pushKey, switchViewKey, scrollDownKey, fullscreenKey, quitKey}
	logHelp       = []key.Binding{downKey, expandKey, searchKey, amendKey, commitKey, pushKey, switchViewKey, scrollDownKey, fullscreenKey, quitKey}
	searchHelp    = []key.Binding{keepSearchKey, clearSearchKey}
	commitHelp    = []key.Binding{nextFieldKey, submitKey, cancelKey}
	pushHelp      = []key.Binding{confirmPushKey, cancelKey}
	forcePushHelp = []key.Binding{confirmForceKey, cancelKey}
	amendHelp     = []key.Binding{confirmAmendKey, cancelKey}
)
