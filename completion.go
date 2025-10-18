package prompt

import (
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

type CompletionItem struct {
	Text string
	Desc string
}

func NewCompletion(items []CompletionItem) *Completion {
	textWidth := utf8.RuneCountInString("内容")
	descWidth := utf8.RuneCountInString("描述")

	for _, item := range items {
		if w := utf8.RuneCountInString(item.Text); w > textWidth {
			textWidth = w
		}
		if w := utf8.RuneCountInString(item.Desc); w > descWidth {
			descWidth = w
		}
	}

	columns := []table.Column{
		{Title: "内容", Width: textWidth + 1},
		{Title: "描述", Width: descWidth + 1},
	}
	rows := make([]table.Row, 0)
	for _, item := range items {
		rows = append(rows, table.Row{
			item.Text,
			item.Desc,
		})
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(len(items)+1),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	m := &Completion{
		items:  items,
		Model:  t,
		Style:  BaseFocusStyle,
		KeyMap: DefaultCompletionKeyMap(),
	}
	return m
}

type Completion struct {
	items []CompletionItem

	Model  table.Model
	Style  lipgloss.Style
	KeyMap CompletionKeyMap
}

func (m Completion) Init() tea.Cmd {
	return textinput.Blink
}

func (m Completion) View() string {
	return m.Style.Render(m.Model.View())
}

func (m *Completion) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	// 键位操作
	case tea.KeyMsg:
		switch {
		// 退出程序
		case key.Matches(msg, m.KeyMap.NextCompletion):
			cursor := m.Model.Cursor()
			nextCursor := cursor + 1
			if len(m.Model.Rows())-1 == cursor {
				nextCursor = 0
			}
			m.Model.SetCursor(nextCursor)
		case key.Matches(msg, m.KeyMap.PrevCompletion):
			cursor := m.Model.Cursor()
			nextCursor := cursor - 1
			if nextCursor < 0 {
				nextCursor = len(m.Model.Rows()) - 1
			}
			m.Model.SetCursor(nextCursor)
			// default:
			// // 其他按键：更新输入框，并根据输入实时过滤补全建议
			// m.Model, cmd = m.Model.Update(msg)
		}

		// 输入变化时，重新生成补全建议（可添加防抖优化）
		return m, cmd
	}

	// 处理列表和输入框的其他消息
	return m, cmd
}

func (m Completion) GetSelected() CompletionItem {
	return m.items[m.Model.Cursor()]
}

func (m Completion) GetAction() string {
	return ""
}

func (m Completion) GetActionPayload() any {
	return nil
}

func (m *Completion) Restore(old tui.Model) {
}
