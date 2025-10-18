package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func NewTerminal(m Model) *Terminal {
	return &Terminal{
		model: m,
	}
}

type Terminal struct {
	program *tea.Program
	model   Model
}

func (t *Terminal) Send(second int, send func() any) {
	now := time.Now().Second()
	if now%second == 0 {
		// logger.Infof("监听时间并执行发送消息任务 %d", now)
		m := send()
		if m != nil {
			t.program.Send(m)
		}
	}
}

func (t *Terminal) Run() error {
	for {
		p := tea.NewProgram(t.model, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			// If Run returns an error, we should probably exit.
			return err
		}

		// After Run() returns, check if we need to perform an action outside the TUI
		switch t.model.GetAction() {
		case "restore":
			continue
		default:
			// No special action, so exit the loop and the program
			return nil
		}
	}
}
