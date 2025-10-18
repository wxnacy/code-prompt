package prompt

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

func NewPrompt(opts ...Option) *Prompt {
	// input := textinput.New()
	// input.Focus()
	// input.Prompt = ">>> "
	// input.SetSuggestions([]string{
	// "import",
	// "fmt.Println()",
	// })

	// input2 := huh.NewInput().Suggestions([]string{
	// "import",
	// "fmt.Println()",
	// }).Prompt(">>> ")
	// input2.Focus()

	m := &Prompt{
		width:  100,
		input:  NewInput(),
		KeyMap: DefaultPromptKeyMap(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

type Prompt struct {
	width  int
	height int
	prompt string

	completionItems []CompletionItem

	// input      textinput.Model
	// input2     *huh.Input
	input      *Input
	completion *Completion

	KeyMap PromptKeyMap
}

func (m Prompt) Init() tea.Cmd {
	return textinput.Blink
}

func (m Prompt) View() string {
	inputView := m.input.View()

	completionView := m.GetCompletionView()
	view := lipgloss.JoinVertical(
		lipgloss.Top,
		inputView,
		completionView,
	)
	return view
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
		}
		// 组件键位监听 begin
		// 其他按键：更新输入框，并根据输入实时过滤补全建议
		input, cmd := m.input.Update(msg)
		cmds = append(cmds, cmd)
		m.input = input.(*Input)

		// value := m.input.Model.Value()

		// 补全键位监听
		if key.Matches(msg, m.completion.KeyMap.ListenKeys()...) {
			completion, cmd := m.completion.Update(msg)
			cmds = append(cmds, cmd)
			m.completion = completion.(*Completion)
			// 检测到补全选中变动，为 input 赋值
			if key.Matches(msg, m.completion.KeyMap.NextCompletion, m.completion.KeyMap.PrevCompletion) {
				selected := m.completion.GetSelected()
				m.input.Model.SetValue(selected.Text)
				m.input.Model.SetCursor(len(selected.Text))
			}
		}

		// 组件键位监听 end

		// 输入变化时，重新生成补全建议（可添加防抖优化）
		return m, tea.Batch(cmds...)
	}

	// 处理列表和输入框的其他消息
	return m, cmd
}

func (m Prompt) FilterCompletionItems() []CompletionItem {
	return nil
}

func (m Prompt) GetAction() string {
	return ""
}

func (m Prompt) GetActionPayload() any {
	return nil
}

func (m *Prompt) Restore(old tui.Model) {
}

func (m *Prompt) Width(w int) {
	m.width = w
}

func (m Prompt) GetCompletionView() string {
	if m.completion != nil {
		return m.completion.View()
	}
	return ""
}

type Option func(*Prompt)

func WithPrompt(s string) Option {
	return func(p *Prompt) {
		p.input.Model.Prompt = s
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
		if len(items) > 0 {
			p.completion = NewCompletion(items)
		}
	}
}
