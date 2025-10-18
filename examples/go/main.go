package main

import (
	"fmt"

	prompt "github.com/wxnacy/code-prompt"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

func main() {
	p := prompt.NewPrompt(
		prompt.WithCompletions([]prompt.CompletionItem{
			{Text: "fmt.Printf", Desc: "func"},
			{Text: "fmt.Println", Desc: "func"},
			{Text: "fmt.Println", Desc: "func"},
		}),
	)
	err := tui.NewTerminal(p).Run()
	if err != nil {
		fmt.Printf("go prompt err %v", err)
	}
}
