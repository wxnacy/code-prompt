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
	newCursor := wordStart + len(replacement)
	isCallable := isCallableCompletionKind(getCompletionKind(selected.Ext))

	if isCallable {
		if !hasParens {
			replacement += "()"
		}
		newCursor = wordStart + len(selected.Text) + 1
	} else if hasParens {
		wordSuffix = wordSuffix[2:]
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
