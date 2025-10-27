package prompt

import (
	"fmt"
	"strings"
)

type BuiltinCommandFunc func(p *Prompt, command string) string

// 内置命令方法集合
var builtinCommandFuncMap = map[string]BuiltinCommandFunc{
	// 展示历史命令
	"/history": func(p *Prompt, command string) string {
		outs := make([]string, 0)
		for i, history := range p.historyItems {
			outs = append(outs, fmt.Sprintf("%d %s", i, history.Command))
		}
		return strings.Join(outs, "\n")
	},
}

// 添加内置命令方法
func AppendBuiltinCommandFunc(command string, f BuiltinCommandFunc) {
	builtinCommandFuncMap[command] = f
}

// 是否匹配内置命令方法
func IsMatchBuiltinCommandFunc(command string) (BuiltinCommandFunc, bool) {
	if f, exists := builtinCommandFuncMap[command]; exists {
		return f, exists
	} else {
		return nil, false
	}
}
