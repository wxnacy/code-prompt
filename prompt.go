package prompt

import (
	"os"
	"strings"
	"sync"
	"time"

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

// HistoryItem 记录单条历史信息，遵循 zsh_history 的时间戳与耗时方案。
type HistoryItem struct {
	Timestamp       int64  // 执行命令的时间戳（秒）
	DurationSeconds int64  // 命令执行耗时（秒）
	Command         string // 执行的命令内容
}

var logger = log.GetLogger()

func Empty() tea.Msg {
	return EmptyMsg{}
}

func NewPrompt(opts ...Option) *Prompt {
	m := &Prompt{
		width:           100,
		prompt:          ">>> ",
		KeyMap:          DefaultPromptKeyMap(),
		outFunc:         func(input string) string { return input },
		historys:        make([]*History, 0),
		historyItems:    make([]HistoryItem, 0),
		historyFilePath: "",
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
	historys        []*History
	historyItems    []HistoryItem
	historyIndex    int // 历史记录索引，等于 len(historyItems) 表示当前输入
	historyFilePath string
	historyMu       sync.Mutex

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
			// 向上翻找记录
			if m.completion == nil {
				if err := m.refreshHistoryItemsFromFile(); err != nil {
					logger.Warnf("同步历史失败: %v", err)
				}
				var (
					historyValue string
					hasHistory   bool
					idx          int
				)
				m.historyMu.Lock()
				total := len(m.historyItems)
				if total == 0 {
					m.historyIndex = 0
				} else {
					if m.historyIndex > total {
						m.historyIndex = total
					}
					if m.historyIndex > 0 {
						m.historyIndex--
					} else {
						m.historyIndex = 0
					}
					historyValue = m.historyItems[m.historyIndex].Command
					hasHistory = true
				}
				idx = m.historyIndex
				m.historyMu.Unlock()
				if hasHistory {
					logger.Debugf("HistoryIndex: %d value: %s", idx, historyValue)
					m.SetValue(historyValue)
					m.SetCursor(len(historyValue))
				} else {
					logger.Debugf("HistoryIndex: %d", idx)
				}
			}
		case key.Matches(msg, m.KeyMap.NextHistory):
			// 向下翻找记录
			if m.completion == nil {
				if err := m.refreshHistoryItemsFromFile(); err != nil {
					logger.Warnf("同步历史失败: %v", err)
				}
				var (
					historyValue string
					hasHistory   bool
					resetEmpty   bool
					idx          int
				)
				m.historyMu.Lock()
				total := len(m.historyItems)
				switch {
				case total == 0:
					m.historyIndex = 0
				case m.historyIndex < total-1:
					m.historyIndex++
					historyValue = m.historyItems[m.historyIndex].Command
					hasHistory = true
				case total > 0:
					m.historyIndex = total
					resetEmpty = true
				default:
					m.historyIndex = 0
				}
				idx = m.historyIndex
				m.historyMu.Unlock()
				if hasHistory {
					logger.Debugf("HistoryIndex: %d value: %s", idx, historyValue)
					m.SetValue(historyValue)
					m.SetCursor(len(historyValue))
				} else if resetEmpty {
					logger.Debugf("HistoryIndex: %d value: %s", idx, "")
					m.SetValue("")
					m.SetCursor(0)
				} else {
					logger.Debugf("HistoryIndex: %d", idx)
				}
			}
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
				execStart := time.Now()
				out := value
				if m.outFunc != nil {
					out = m.outFunc(value)
				}
				duration := time.Since(execStart)
				m.AppendHistory(value, out, execStart, duration)
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

// History begin ================

func (m *Prompt) HistoryFile(p string) {
	WithHistoryFile(p)(m)
}

// AppendHistory 将执行结果写入历史记录，并同步内存与文件内容。
func (m *Prompt) AppendHistory(command string, outText string, startedAt time.Time, duration time.Duration) {
	out := NewOut(outText)
	if outText == "" {
		out = nil
	}
	h := NewHistory(m.input, out)
	m.historys = append(m.historys, h)

	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		// 仅记录包含有效字符的历史项
		return
	}

	item := HistoryItem{
		Timestamp:       startedAt.Unix(),
		DurationSeconds: int64(duration / time.Second),
		Command:         command,
	}

	m.AppendHistoryItem(item)
}

// AppendHistoryItem 将历史项写入内存，并同步到历史文件中。
func (m *Prompt) AppendHistoryItem(item HistoryItem) {
	if item.DurationSeconds < 0 {
		item.DurationSeconds = 0
	}
	m.historyMu.Lock()
	m.historyItems = append(m.historyItems, item)
	m.historyIndex = len(m.historyItems)
	m.historyMu.Unlock()

	if err := m.appendHistoryToFile(item); err != nil {
		logger.Warnf("写入历史文件失败: %v", err)
	}
}

// appendHistoryToFile 将历史记录安全追加到文件中。
func (m *Prompt) appendHistoryToFile(item HistoryItem) error {
	if m.historyFilePath == "" {
		return nil
	}
	path := m.historyFilePath
	if err := ensureHistoryFile(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := lockFile(file); err != nil {
		return err
	}
	defer func() {
		_ = unlockFile(file)
	}()

	if _, err := file.WriteString(formatHistoryItem(item)); err != nil {
		return err
	}
	return file.Sync()
}

// refreshHistoryItemsFromFile 刷新内存中的历史记录。
func (m *Prompt) refreshHistoryItemsFromFile() error {
	if m.historyFilePath == "" {
		return nil
	}
	items, err := readHistoryFile(m.historyFilePath)
	if err != nil {
		return err
	}
	m.historyMu.Lock()
	if len(items) >= len(m.historyItems) {
		m.historyItems = items
		if m.historyIndex > len(m.historyItems) {
			m.historyIndex = len(m.historyItems)
		}
	}
	m.historyMu.Unlock()
	return nil
}

// History end   ================

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

// WithHistoryFile 设置历史文件路径，允许覆盖默认地址。
func WithHistoryFile(path string) Option {
	return func(p *Prompt) {
		resolved, err := resolveHistoryFilePath(path)
		if err != nil {
			logger.Warnf("设置历史文件失败: %v", err)
			return
		}
		p.historyFilePath = resolved
		if err := p.refreshHistoryItemsFromFile(); err != nil {
			logger.Warnf("加载历史文件失败: %v", err)
		}
		p.historyMu.Lock()
		p.historyIndex = len(p.historyItems)
		p.historyMu.Unlock()
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
