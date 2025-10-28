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
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "确认"),
		),
		ClearCompletion: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "取消补全建议"),
		),
		NextCompletion: defaultCompletionKeyMap.NextCompletion,
		PrevCompletion: defaultCompletionKeyMap.PrevCompletion,
		NextHistory: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "上一条历史"),
		),
		PrevHistory: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "下一条历史"),
		),
		Clear: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "清屏"),
		),
		GiveUp: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "放弃当前命令"),
		),
	}
}

type PromptKeyMap struct {
	// FullHelp
	NextCompletion  key.Binding // ShortHelp ListenKeys
	PrevCompletion  key.Binding // ShortHelp ListenKeys
	ClearCompletion key.Binding // ShortHelp ListenKeys

	// FullHelp
	NextHistory key.Binding // ListenKeys
	PrevHistory key.Binding // ListenKeys

	// FullHelp
	Clear  key.Binding // ListenKeys
	GiveUp key.Binding // ListenKeys

	// FullHelp
	Enter key.Binding // ListenKeys
	Exit  key.Binding // ListenKeys
}

func (km PromptKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{km.NextCompletion, km.PrevCompletion, km.ClearCompletion}
}

// FullHelp 返回所有快捷键的帮助信息。
func (km PromptKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.NextCompletion, km.PrevCompletion, km.ClearCompletion},
		{km.NextHistory, km.PrevHistory},
		{km.Clear, km.GiveUp},
		{km.Exit, km.Enter},
	}
}

// ListenKeys 返回需要监听的快捷键绑定列表。
func (km PromptKeyMap) ListenKeys() []key.Binding {
	return []key.Binding{
		km.NextCompletion,
		km.PrevCompletion,
		km.ClearCompletion,
		km.NextHistory,
		km.PrevHistory,
		km.Clear,
		km.GiveUp,
		km.Enter,
		km.Exit,
	}
}
