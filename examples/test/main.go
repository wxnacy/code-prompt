package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 补全建议项
type completionItem struct {
	text        string // 补全文本
	description string // 描述（可选）
}

// 实现 list.Item 接口（用于在列表中展示）
func (i completionItem) Title() string       { return i.text }
func (i completionItem) Description() string { return i.description }
func (i completionItem) FilterValue() string { return i.text }

// 应用状态
type model struct {
	input           textinput.Model  // 输入框组件
	completionList  list.Model       // 补全建议列表
	showCompletions bool             // 是否显示补全列表
	completions     []completionItem // 所有补全建议
}

func main() {
	// 初始化输入框
	input := textinput.New()
	input.Placeholder = "输入命令..."
	input.Focus()
	input.Prompt = "> "

	// 初始化补全列表（样式通过 lipgloss 定制）
	items := []list.Item{}
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	delegate.Styles.NormalDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Background(lipgloss.Color("32")).Foreground(lipgloss.Color("white"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedTitle.Copy()
	completionList := list.New(items, delegate, 0, 0)
	completionList.Title = "补全建议"
	completionList.SetHeight(5) // 限制补全列表高度为 5 行

	// 启动应用
	m := model{
		input:          input,
		completionList: completionList,
	}
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		panic(err)
	}
}

// 初始化：返回空命令
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// 更新状态（处理输入和补全逻辑）
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// 退出程序
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// Tab 键或 Enter 键触发补全（或选择当前高亮的建议）
		if msg.Type == tea.KeyTab || msg.Type == tea.KeyEnter {
			if m.showCompletions && len(m.completionList.Items()) > 0 {
				// 应用选中的补全项
				selected := m.completionList.SelectedItem().(completionItem)
				m.input.SetValue(selected.text)
				m.input.SetCursor(len(selected.text))
				m.showCompletions = false // 隐藏补全列表
			} else {
				// 生成补全建议（这里是示例，实际可对接 gopls 等）
				m.generateCompletions()
				m.showCompletions = true
			}
			return m, nil
		}

		// 上下键导航补全列表
		if m.showCompletions {
			switch msg.Type {
			case tea.KeyUp, tea.KeyDown:
				var cmd tea.Cmd
				m.completionList, cmd = m.completionList.Update(msg)
				return m, cmd
			}
		}

		// 其他按键：更新输入框，并根据输入实时过滤补全建议
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)

		// 输入变化时，重新生成补全建议（可添加防抖优化）
		m.generateCompletions()
		m.showCompletions = len(m.completionList.Items()) > 0
		return m, cmd
	}

	// 处理列表和输入框的其他消息
	var cmd tea.Cmd
	m.completionList, cmd = m.completionList.Update(msg)
	return m, cmd
}

// 生成补全建议（示例：简单匹配，实际可对接 gopls 或自定义数据源）
func (m *model) generateCompletions() {
	// 模拟补全数据源（实际可替换为 gopls 返回的补全项）
	allCompletions := []completionItem{
		{text: "fmt.Println", description: "打印到标准输出"},
		{text: "fmt.Printf", description: "格式化打印"},
		{text: "os.Open", description: "打开文件"},
		{text: "strings.Contains", description: "检查字符串包含"},
	}

	// 根据当前输入过滤补全项
	input := m.input.Value()
	filtered := []completionItem{}
	for _, item := range allCompletions {
		if strings.HasPrefix(item.text, input) {
			filtered = append(filtered, item)
		}
	}

	// 更新补全列表
	listItems := make([]list.Item, len(filtered))
	for i, item := range filtered {
		listItems[i] = item
	}
	m.completionList.SetItems(listItems)
}

// 渲染界面
func (m model) View() string {
	// 输入框 + 补全列表（如果显示）
	var completionView string
	if m.showCompletions {
		completionView = "\n" + m.completionList.View()
	}
	return m.input.View() + completionView + "\n"
}
