package prompt

import (
	"github.com/charmbracelet/bubbles/key"
)

var defaultCompletionKeyMap = DefaultCompletionKeyMap()

func DefaultCompletionKeyMap() CompletionKeyMap {
	return CompletionKeyMap{
		NextCompletion: key.NewBinding(
			key.WithKeys("tab", "down", "ctrl+n"),
			key.WithHelp("tab/↓/ctrl+n", "next completion"),
		),
		PrevCompletion: key.NewBinding(
			key.WithKeys("shift+tab", "up", "ctrl+p"),
			key.WithHelp("shift+tab/↑/ctrl+p", "prev completion"),
		),
	}
}

type CompletionKeyMap struct {
	// FullHelp
	NextCompletion key.Binding // ShortHelp
	PrevCompletion key.Binding // ShortHelp
}

func (km CompletionKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{km.NextCompletion, km.PrevCompletion}
}

func (km CompletionKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.NextCompletion, km.PrevCompletion},
	}
}

func (km CompletionKeyMap) ListenKeys() []key.Binding {
	return []key.Binding{
		km.NextCompletion,
		km.PrevCompletion,
	}
}
