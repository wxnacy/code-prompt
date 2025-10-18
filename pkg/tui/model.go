package tui

import tea "github.com/charmbracelet/bubbletea"

type Model interface {
	Restore(old Model)
	Init() tea.Cmd
	View() string
	Update(tea.Msg) (tea.Model, tea.Cmd)
	GetAction() string
	GetActionPayload() any
}
