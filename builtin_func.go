package prompt

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type (
	BuiltinCommandFunc     func(p *Prompt, command string) (string, tea.Cmd)
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
		Desc:    "展示历史命令",
		Func: func(p *Prompt, command string) (string, tea.Cmd) {
			outs := make([]string, 0)
			// 计算右对齐的宽度：使用最大索引的位数（len(historyItems)-1）作为宽度
			// 例如有 120 条记录，则最大索引为 119，位数为 3，最终以 3 宽度右对齐
			width := 1
			if n := len(p.historyItems); n > 0 {
				width = len(strconv.Itoa(n - 1))
			}
			for i, history := range p.historyItems {
				// 使用动态宽度占位符 %*d 实现右对齐输出索引
				// 例如：  1 cmd、 23 cmd、123 cmd
				outs = append(outs, fmt.Sprintf("%*d %s", width, i, history.Command))
			}
			return strings.Join(outs, "\n"), Empty
		},
	},
	{
		Command: "/exit", // 退出程序
		Desc:    "退出程序",
		Func: func(p *Prompt, command string) (string, tea.Cmd) {
			return "", tea.Quit
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
