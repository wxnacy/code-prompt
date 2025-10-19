package prompt

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func NewHistory(input *Input, out *Out) *History {
	m := &History{
		Input:  input,
		Out:    out,
		Style:  BaseFocusStyle,
		KeyMap: DefaultCompletionKeyMap(),
	}
	return m
}

type History struct {
	BaseModel
	Input *Input
	Out   *Out
	Style lipgloss.Style

	KeyMap CompletionKeyMap
}

func (m History) Init() tea.Cmd {
	return textinput.Blink
}

func (m History) View() string {
	views := make([]string, 0)
	m.Input.Model.Blur()
	views = append(views, m.Input.View())
	if m.Out != nil {
		views = append(views, m.Out.View())
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		views...,
	)
}

func (m *History) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	// var model tea.Model
	switch msg := msg.(type) {
	// 键位操作
	case tea.KeyMsg:
		switch {
		default:
			// 其他按键：更新输入框，并根据输入实时过滤补全建议
			input, cmd := m.Input.Update(msg)
			cmds = append(cmds, cmd)
			m.Input = input.(*Input)
			// m.Model = model
		}

		// 输入变化时，重新生成补全建议（可添加防抖优化）
		return m, cmd
	}

	// 处理列表和输入框的其他消息
	return m, tea.Batch(cmds...)
}
