package prompt

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wxnacy/code-prompt/pkg/lsp"
)

// 选择补全方法
// 功能点概述:
// - 依据光标位置回退并替换完整标识符，避免重复叠加已有括号
// - selected.Ext 为 lsp.CompletionItem 时按 Kind 决定是否补全 "()" 并把光标放在括号内
// - 非可调用项若存在残留括号则移除，保持输入整洁
// - 内置命令补全「/ 开头」的特殊处理：当输入前缀以 '/' 结尾且补全文本以 '/' 开头时，去重边界，避免生成 "//history"
func DefaultCompletionLSPSelectFunc(p *Prompt, input string, cursor int, selected CompletionItem) {
	if len(input) == 0 {
		p.SetValue(selected.Text)
		p.SetCursor(len(selected.Text))
		return
	}

	adjustCursor := cursor
	if adjustCursor > 0 {
		prev, size := utf8.DecodeLastRuneInString(input[:adjustCursor])
		if prev == '(' {
			// 如果光标位于括号内，将光标回退到函数名末尾，以便替换函数名
			adjustCursor -= size
		}
	}

	wordStart := adjustCursor
	for wordStart > 0 {
		r, size := utf8.DecodeLastRuneInString(input[:wordStart])
		if !isIdentRune(r) {
			break
		}
		wordStart -= size
	}

	wordEnd := adjustCursor
	for wordEnd < len(input) {
		r, size := utf8.DecodeRuneInString(input[wordEnd:])
		if !isIdentRune(r) {
			break
		}
		wordEnd += size
	}

	prefix := input[:wordStart]
	wordSuffix := input[wordEnd:]
	hasParens := strings.HasPrefix(wordSuffix, "()")

	replacement := selected.Text
	isCallable := isCallableCompletionKind(getCompletionKind(selected.Ext))

	if isCallable {
		if !hasParens {
			replacement += "()"
		}
	} else if hasParens {
		wordSuffix = wordSuffix[2:]
	}

	// 去重边界分隔符：避免前缀以 '/' 结尾且 replacement 以 '/' 开头导致重复，如 "//history"
	if len(prefix) > 0 && len(replacement) > 0 {
		pr, _ := utf8.DecodeLastRuneInString(prefix)
		rr, sz := utf8.DecodeRuneInString(replacement)
		if pr == rr && pr == '/' {
			replacement = replacement[sz:]
		}
	}

	// 基于最终 replacement 计算光标位置
	newCursor := wordStart + len(replacement)
	if isCallable {
		if strings.HasSuffix(replacement, "()") {
			// 我们在 replacement 中追加了括号，将光标放在括号内
			newCursor = wordStart + len(replacement) - 1
		} else if hasParens {
			// 右侧已存在括号，重复选择时应将光标置于现有括号内
			newCursor = wordStart + len(replacement) + 1
		}
	}

	newInput := prefix + replacement + wordSuffix

	p.SetValue(newInput)
	p.SetCursor(newCursor)
}

func getCompletionKind(ext interface{}) int {
	switch v := ext.(type) {
	case lsp.CompletionItem:
		return v.Kind
	case *lsp.CompletionItem:
		return v.Kind
	default:
		return 0
	}
}

func isCallableCompletionKind(kind int) bool {
	switch kind {
	case 2, 3, 4:
		return true
	default:
		return false
	}
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
