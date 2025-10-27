package main

import (
	"fmt"

	prompt "github.com/wxnacy/code-prompt"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

func main() {
	p := prompt.NewPrompt(
		prompt.WithPrompt("> "),
		prompt.WithHistoryFile("~/.prompt_history"),
	)
	err := tui.NewTerminal(p).Run()
	if err != nil {
		fmt.Printf("go prompt err %v", err)
	}
}
