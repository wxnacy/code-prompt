package prompt

import (
	"github.com/charmbracelet/bubbles/key"
)

func DefaultPromptKeyMap() PromptKeyMap {
	return PromptKeyMap{
		Exit: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "退出"),
		),
		// Enter: key.NewBinding(
		// key.WithKeys("enter"),
		// key.WithHelp("enter", "退出"),
		// ),
		NextCompletion: defaultCompletionKeyMap.NextCompletion,
		PrevCompletion: defaultCompletionKeyMap.PrevCompletion,
	}
}

type PromptKeyMap struct {
	// FullHelp
	NextCompletion key.Binding // ShortHelp
	PrevCompletion key.Binding // ShortHelp

	// FullHelp
	// Enter key.Binding
	Exit key.Binding
}

func (km PromptKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{km.NextCompletion, km.PrevCompletion}
}

func (km PromptKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.NextCompletion, km.PrevCompletion},
		{km.Exit},
	}
}

func (km PromptKeyMap) GetAllKeys() []key.Binding {
	return []key.Binding{
		km.NextCompletion,
		km.PrevCompletion,
		km.Exit,
	}
}
