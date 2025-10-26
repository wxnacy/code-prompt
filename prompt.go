package prompt

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wxnacy/code-prompt/pkg/log"
)

type (
	CompletionFunc       func(input string, cursor int) []CompletionItem
	CompletionSelectFunc func(p *Prompt, input string, cursor int, selected CompletionItem)
	OutFunc              func(input string) string

	EmptyMsg struct{}
)

var logger = log.GetLogger()

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
	WithCompletionFunc(m.DefaultCompletionFunc)(m)
	WithCompletionSelectFunc(DefaultCompletionSelectFunc)(m)
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
	historys     []*History
	historyIndex int // 历史记录索引

	// completion
	completionItems      []CompletionItem
	completionFunc       CompletionFunc
	completionSelectFunc CompletionSelectFunc
	completion           *Completion

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
			if m.Value() == "" {
				// 没有输入数据的时候才退出，否则执行向后删除功能
				return m, tea.Quit
			}
		case key.Matches(msg, m.KeyMap.ClearCompletion):
			m.completion = nil
			return m, Empty
		case key.Matches(msg, m.KeyMap.PrevHistory):
			if m.completion == nil && len(m.historys) > 0 {
				if m.historyIndex > len(m.historys) {
					m.historyIndex = len(m.historys)
				}
				if m.historyIndex > 0 {
					m.historyIndex--
				} else {
					m.historyIndex = 0
				}
				historyValue := m.historys[m.historyIndex].Input.Model.Value()
				logger.Debugf("HistoryIndex: %d value: %s", m.historyIndex, historyValue)
				m.SetValue(historyValue)
				m.SetCursor(len(historyValue))
			}
			logger.Debugf("HistoryIndex: %d", m.historyIndex)
		case key.Matches(msg, m.KeyMap.NextHistory):
			if m.completion == nil && len(m.historys) > 0 {
				if m.historyIndex < len(m.historys)-1 {
					m.historyIndex++
					historyValue := m.historys[m.historyIndex].Input.Model.Value()
					logger.Debugf("HistoryIndex: %d value: %s", m.historyIndex, historyValue)
					m.SetValue(historyValue)
					m.SetCursor(len(historyValue))
				} else {
					m.historyIndex = len(m.historys)
					m.SetValue("")
					m.SetCursor(0)
					logger.Debugf("HistoryIndex: %d value: %s", m.historyIndex, "")
				}
			}
			logger.Debugf("HistoryIndex: %d", m.historyIndex)
		case key.Matches(msg, m.KeyMap.Enter):
			value := m.Value()
			if m.completion != nil {
				// 如果有补全建议，使用选中的，或者开始的第一个
				selected := m.completion.GetSelected()
				// 触发选择补全的方法
				if m.completionSelectFunc != nil {
					m.completionSelectFunc(m, value, m.Cursor(), selected)
				}
				m.completion = nil
			} else {
				// 进行输出
				out := value
				if m.outFunc != nil {
					out = m.outFunc(value)
				}
				m.AppendHistory(out)
				m.input = m.NewInput()
			}
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
				// 触发选择补全的方法
				if m.completionSelectFunc != nil {
					m.completionSelectFunc(m, m.Value(), m.Cursor(), selected)
				}
			}

		} else {
			cmd = m.UpdateInput(msg)
			cmds = append(cmds, cmd)

			value := m.Value()
			// 使用补全方法获取自全列表
			if m.completionFunc != nil {
				newCompletionItems := m.completionFunc(value, m.Cursor())
				if newCompletionItems != nil && len(newCompletionItems) > 0 {
					m.completion = NewCompletion(newCompletionItems)
				} else {
					m.completion = nil
				}
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

// AppendHistory 追加新的历史记录并将索引指向当前输入。
func (m *Prompt) AppendHistory(outText string) {
	out := NewOut(outText)
	if outText == "" {
		out = nil
	}
	h := NewHistory(m.input, out)
	m.historys = append(m.historys, h)
	m.historyIndex = len(m.historys)
}

func (m *Prompt) OutFunc(f OutFunc) {
	WithOutFunc(f)(m)
}

func (m *Prompt) Completions(items []CompletionItem) {
	WithCompletions(items)(m)
}

func (m *Prompt) CompletionFunc(f CompletionFunc) {
	m.completionFunc = f
}

func (m *Prompt) CompletionSelectFunc(f CompletionSelectFunc) {
	WithCompletionSelectFunc(f)(m)
}

func (m *Prompt) DefaultCompletionFunc(input string, cursor int) []CompletionItem {
	newCompletionItems := make([]CompletionItem, 0)
	for _, item := range m.completionItems {
		if strings.HasPrefix(item.Text, input) {
			newCompletionItems = append(newCompletionItems, item)
		}
	}
	logger.Debugf("Completion items length %d", len(newCompletionItems))
	return newCompletionItems
}

func DefaultCompletionSelectFunc(p *Prompt, input string, cursor int, selected CompletionItem) {
	p.SetValue(selected.Text)
	p.SetCursor(len(selected.Text))
}

func (m Prompt) GetCompletionView() string {
	if m.completion != nil {
		return m.completion.View()
	}
	return ""
}

// Input begin ==================

func (m Prompt) NewInput() *Input {
	input := NewInput()
	input.Model.Prompt = m.prompt
	return input
}

func (m Prompt) Value() string {
	return m.input.Model.Value()
}

func (m Prompt) Cursor() int {
	return m.input.Model.Position()
}

func (m Prompt) SetValue(s string) {
	m.input.Model.SetValue(s)
}

func (m Prompt) SetCursor(pos int) {
	m.input.Model.SetCursor(pos)
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

func WithCompletionFunc(f CompletionFunc) Option {
	return func(p *Prompt) {
		p.completionFunc = f
	}
}

func WithCompletionSelectFunc(f CompletionSelectFunc) Option {
	return func(p *Prompt) {
		p.completionSelectFunc = f
	}
}
