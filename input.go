package prompt

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

func NewInput() *Input {
	// input := huh.NewInput().
	// Prompt(">>> ")
	// input.Focus()
	input := textinput.New()
	input.Focus()
	input.Prompt = ">>> "
	// input.SetSuggestions([]string{
	// // "import",
	// "fmt.Println()",
	// })
	m := &Input{
		Model:  input,
		Style:  BaseFocusStyle,
		KeyMap: DefaultCompletionKeyMap(),
	}
	return m
}

type Input struct {
	Model textinput.Model
	Style lipgloss.Style

	KeyMap CompletionKeyMap
}

func (m Input) Init() tea.Cmd {
	return textinput.Blink
}

func (m Input) View() string {
	return m.Model.View()
}

func (m *Input) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	// var model tea.Model
	switch msg := msg.(type) {
	// 键位操作
	case tea.KeyMsg:
		switch {
		// 退出程序
		// case key.Matches(msg, m.KeyMap.NextCompletion):
		// cursor := m.Model.Cursor()
		// nextCursor := cursor + 1
		// if len(m.Model.Rows())-1 == cursor {
		// nextCursor = 0
		// }
		// m.Model.SetCursor(nextCursor)
		// case key.Matches(msg, m.KeyMap.PrevCompletion):
		// cursor := m.Model.Cursor()
		// nextCursor := cursor - 1
		// if nextCursor < 0 {
		// nextCursor = len(m.Model.Rows()) - 1
		// }
		// m.Model.SetCursor(nextCursor)
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

func (m Input) GetAction() string {
	return ""
}

func (m Input) GetActionPayload() any {
	return nil
}

func (m *Input) Restore(old tui.Model) {
}
