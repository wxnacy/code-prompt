package prompt

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func NewOut(text string) *Out {
	vp := viewport.New(100, 1)
	vp.SetContent(text)
	m := &Out{
		Model:  vp,
		Style:  BaseFocusStyle,
		KeyMap: DefaultCompletionKeyMap(),
	}
	return m
}

type Out struct {
	BaseModel
	Model viewport.Model
	Style lipgloss.Style

	KeyMap CompletionKeyMap
}

func (m Out) Init() tea.Cmd {
	return textinput.Blink
}

func (m Out) View() string {
	return m.Model.View()
}

func (m *Out) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	// var model tea.Model
	switch msg := msg.(type) {
	// 键位操作
	case tea.KeyMsg:
		switch {
		default:
			// 其他按键：更新输入框，并根据输入实时过滤补全建议
			m.Model, cmd = m.Model.Update(msg)
			// m.Model = model
		}

		// 输入变化时，重新生成补全建议（可添加防抖优化）
		return m, cmd
	}

	// 处理列表和输入框的其他消息
	return m, cmd
}
