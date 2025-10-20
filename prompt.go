package prompt

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type (
	CompletionFunc func(input string) []CompletionItem
	OutFunc        func(input string) string

	EmptyMsg struct{}
)

func Empty() tea.Msg {
	return EmptyMsg{}
}

func NewPrompt(opts ...Option) *Prompt {
	m := &Prompt{
		width:    100,
		prompt:   ">>> ",
		KeyMap:   DefaultPromptKeyMap(),
		outFunc:  func(input string) string { return input },
		historys: make([]*History, 0),
	}
	for _, opt := range opts {
		opt(m)
	}
	m.input = m.NewInput()
	return m
}

type Prompt struct {
	BaseModel
	width  int
	height int
	prompt string

	// history
	historys []*History

	// completion
	completionItems []CompletionItem
	completionFunc  CompletionFunc
	completion      *Completion

	// input
	input *Input

	// out
	outFunc OutFunc

	KeyMap PromptKeyMap
}

func (m Prompt) Init() tea.Cmd {
	return textinput.Blink
}

func (m Prompt) View() string {
	views := make([]string, 0)
	if m.historys != nil && len(m.historys) > 0 {
		for _, history := range m.historys {
			views = append(views, history.View())
		}
	}
	views = append(views, m.input.View())
	views = append(views, m.GetCompletionView())
	view := lipgloss.JoinVertical(
		lipgloss.Top,
		views...,
	)
	return view
}

// 其他按键：更新输入框，并根据输入实时过滤补全建议
func (m *Prompt) UpdateInput(msg tea.Msg) tea.Cmd {
	input, cmd := m.input.Update(msg)
	m.input = input.(*Input)
	// value := m.input.Model.Value()
	// if value != m.inputText {
	// m.inputText = value
	// }
	return cmd
}

func (m *Prompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	// 键位操作
	case tea.KeyMsg:

		// 全局键位
		switch {
		// 退出程序
		case key.Matches(msg, m.KeyMap.Exit):
			return m, tea.Quit
		case key.Matches(msg, m.KeyMap.Enter):
			value := m.input.Model.Value()
			out := value
			if m.outFunc != nil {
				out = m.outFunc(value)
			}
			m.AppendHistory(out)
			m.input = m.NewInput()
			m.completion = nil
			return m, Empty
		}
		// 组件键位监听 begin
		isCompletionKey := m.completion != nil && key.Matches(msg, m.completion.KeyMap.ListenKeys()...)

		if isCompletionKey {
			completion, cmd := m.completion.Update(msg)
			cmds = append(cmds, cmd)
			m.completion = completion.(*Completion)

			if key.Matches(msg, m.completion.KeyMap.NextCompletion, m.completion.KeyMap.PrevCompletion) {
				selected := m.completion.GetSelected()
				m.input.Model.SetValue(selected.Text)
				m.input.Model.SetCursor(len(selected.Text))
			}

		} else {
			cmd = m.UpdateInput(msg)
			cmds = append(cmds, cmd)

			value := m.input.Model.Value()
			newCompletionItems := m.filterCompletionItems(value)
			if newCompletionItems != nil && len(newCompletionItems) > 0 {
				m.completion = NewCompletion(newCompletionItems)
			} else {
				m.completion = nil
			}
		}
		// 组件键位监听 end
		return m, tea.Batch(cmds...)
	}

	// 处理列表和输入框的其他消息
	return m, cmd
}

func (m *Prompt) Width(w int) {
	m.width = w
}

func (m *Prompt) AppendHistory(outText string) {
	out := NewOut(outText)
	if outText == "" {
		out = nil
	}
	h := NewHistory(m.input, out)
	m.historys = append(m.historys, h)
}

func (m *Prompt) CompletionFunc(f CompletionFunc) {
	m.completionFunc = f
}

func (m Prompt) GetCompletionView() string {
	if m.completion != nil {
		return m.completion.View()
	}
	return ""
}

func (m Prompt) filterCompletionItems(value string) []CompletionItem {
	if m.completionFunc != nil {
		return m.completionFunc(value)
	}
	newCompletionItems := make([]CompletionItem, 0)
	for _, item := range m.completionItems {
		if strings.HasPrefix(item.Text, value) {
			newCompletionItems = append(newCompletionItems, item)
		}
	}
	return newCompletionItems
}

// Input begin ==================

func (m Prompt) NewInput() *Input {
	input := NewInput()
	input.Model.Prompt = m.prompt
	return input
}

// Input end   ==================

type Option func(*Prompt)

func WithPrompt(s string) Option {
	return func(p *Prompt) {
		p.prompt = s
		// p.input.Model.Prompt = s
	}
}

func WithWidth(w int) Option {
	return func(p *Prompt) {
		p.width = w
	}
}

func WithCompletions(items []CompletionItem) Option {
	return func(p *Prompt) {
		p.completionItems = items
	}
}

func WithOutFunc(f OutFunc) Option {
	return func(p *Prompt) {
		p.outFunc = f
	}
}
