package prompt

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

type BaseModel struct{}

func (m BaseModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m BaseModel) View() string {
	return ""
}

func (m *BaseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	return m, cmd
}

func (m BaseModel) GetAction() string {
	return ""
}

func (m BaseModel) GetActionPayload() any {
	return nil
}

func (m *BaseModel) Restore(old tui.Model) {
}
