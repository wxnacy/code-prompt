package main

import (
	"fmt"

	"github.com/sirupsen/logrus"
	prompt "github.com/wxnacy/code-prompt"
	"github.com/wxnacy/code-prompt/pkg/log"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

func main() {
	log.SetOutputFile("prompt.log")
	log.SetLogLevel(logrus.DebugLevel)
	p := prompt.NewPrompt(
		prompt.WithPrompt("> "),
		prompt.WithCompletions([]prompt.CompletionItem{
			{Text: "fmt.Printf", Desc: "func"},
			{Text: "fmt.Printf", Desc: "func"},
			{Text: "time.Now()", Desc: "func"},
		}),
	)
	err := tui.NewTerminal(p).Run()
	if err != nil {
		fmt.Printf("go prompt err %v", err)
	}
}
