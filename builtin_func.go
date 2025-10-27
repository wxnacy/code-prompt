package prompt

import (
	"fmt"
	"strings"
)

type (
	BuiltinCommandFunc     func(p *Prompt, command string) string
	BuiltinCommandFuncItem struct {
		Command string             // 命令
		Desc    string             // 描述
		Func    BuiltinCommandFunc // 方法主题
	}
)

// 内置命令方法集合，改为切片结构，便于控制顺序与扩展字段
var builtinCommandFuncItems = []BuiltinCommandFuncItem{
	{
		Command: "/history", // 展示历史命令
		Desc:    "show command history",
		Func: func(p *Prompt, command string) string {
			outs := make([]string, 0)
			for i, history := range p.historyItems {
				outs = append(outs, fmt.Sprintf("%d %s", i, history.Command))
			}
			return strings.Join(outs, "\n")
		},
	},
}

// 获取内置方法的补全对象列表
func GetBuiltinCommandCompletions() []CompletionItem {
	items := make([]CompletionItem, 0)
	for _, v := range builtinCommandFuncItems {
		items = append(items, CompletionItem{
			Text: v.Command,
			Desc: v.Desc,
		})
	}
	return items
}

// AppendBuiltinCommandFunc 添加或覆盖内置命令方法
func AppendBuiltinCommandFunc(command, desc string, f BuiltinCommandFunc) {
	// 优先覆盖已存在的命令
	for i := range builtinCommandFuncItems {
		if builtinCommandFuncItems[i].Command == command {
			builtinCommandFuncItems[i].Func = f
			return
		}
	}
	// 不存在则追加
	builtinCommandFuncItems = append(builtinCommandFuncItems, BuiltinCommandFuncItem{
		Command: command,
		Desc:    desc,
		Func:    f,
	})
}

// IsMatchBuiltinCommandFunc 是否匹配内置命令方法
func IsMatchBuiltinCommandFunc(command string) (BuiltinCommandFunc, bool) {
	for i := range builtinCommandFuncItems {
		if builtinCommandFuncItems[i].Command == command {
			return builtinCommandFuncItems[i].Func, true
		}
	}
	return nil, false
}
