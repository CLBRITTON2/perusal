package ui

import (
	"charm.land/bubbles/v2/key"
)

// Bindings with no help text are the second half of a pair whose help the first one shows, such as j/k.
var (
	downKey        = key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/k", "select"))
	upKey          = key.NewBinding(key.WithKeys("k", "up"))
	stageKey       = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "stage"))
	commitKey      = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "commit"))
	switchViewKey  = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "changes/log"))
	scrollDownKey  = key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("^d/^u", "scroll"))
	scrollUpKey    = key.NewBinding(key.WithKeys("ctrl+u", "pgup"))
	refreshKey     = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	quitKey        = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	forceQuitKey   = key.NewBinding(key.WithKeys("ctrl+c"))
	nextFieldKey   = key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "next field"))
	submitKey      = key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "commit"))
	enterKey       = key.NewBinding(key.WithKeys("enter"))
	cancelKey      = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	expandKey      = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand"))
	searchKey      = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	keepSearchKey  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "keep"))
	clearSearchKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear"))
)

var (
	changesHelp = []key.Binding{downKey, stageKey, commitKey, switchViewKey, scrollDownKey, refreshKey, quitKey}
	logHelp     = []key.Binding{downKey, expandKey, searchKey, commitKey, switchViewKey, scrollDownKey, refreshKey,
		quitKey}
	searchHelp = []key.Binding{keepSearchKey, clearSearchKey}
	commitHelp = []key.Binding{nextFieldKey, submitKey, cancelKey}
)
