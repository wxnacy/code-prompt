package prompt

import (
	"testing"
	"unicode/utf8"

	"github.com/wxnacy/code-prompt/pkg/lsp"
)

// 辅助断言：断言值与光标
func assertValueCursor(t *testing.T, p *Prompt, wantVal string, wantCursor int) {
	t.Helper()
	if got := p.Value(); got != wantVal {
		t.Fatalf("value mismatch: got %q, want %q", got, wantVal)
	}
	if got := p.Cursor(); got != wantCursor {
		t.Fatalf("cursor mismatch: got %d, want %d", got, wantCursor)
	}
}

// Test: 输入前缀为 '/'，选择补全以 '/' 开头时，避免生成重复的 '//'
func TestDefaultCompletionLSPSelectFunc_SlashDedup(t *testing.T) {
	p := NewPrompt()
	p.SetValue("/")
	p.SetCursor(utf8.RuneCountInString("/"))

	selected := CompletionItem{Text: "/history"}
	DefaultCompletionLSPSelectFunc(p, p.Value(), p.Cursor(), selected)

	assertValueCursor(t, p, "/history", len("/history"))
}

// Test: 可调用项首次选择时应自动追加括号并将光标放入括号内
func TestDefaultCompletionLSPSelectFunc_Callable_NewParens(t *testing.T) {
	p := NewPrompt()
	p.SetValue("fmt.Pr")
	p.SetCursor(len("fmt.Pr"))

	selected := CompletionItem{Text: "Println", Ext: lsp.CompletionItem{Kind: 3}}
	DefaultCompletionLSPSelectFunc(p, p.Value(), p.Cursor(), selected)

	want := "fmt.Println()"
	// 光标位于括号内
	wantCursor := len("fmt.Println(")
	assertValueCursor(t, p, want, wantCursor)
}

// Test: 可调用项在右侧已存在括号时，重复选择应将光标置于现有括号内
func TestDefaultCompletionLSPSelectFunc_Callable_ExistingParens(t *testing.T) {
	p := NewPrompt()
	p.SetValue("fmt.Println()")
	// 光标设在函数名末尾（左括号之前）
	p.SetCursor(len("fmt.Println"))

	selected := CompletionItem{Text: "Println", Ext: lsp.CompletionItem{Kind: 3}}
	DefaultCompletionLSPSelectFunc(p, p.Value(), p.Cursor(), selected)

	want := "fmt.Println()"
	// 光标应移动到现有括号内
	wantCursor := len("fmt.Println(")
	assertValueCursor(t, p, want, wantCursor)
}

// Test: 非可调用项且右侧有残留括号，应移除括号并将光标置于末尾
func TestDefaultCompletionLSPSelectFunc_NonCallable_RemoveParens(t *testing.T) {
	p := NewPrompt()
	p.SetValue("value()")
	p.SetCursor(len("value"))

	selected := CompletionItem{Text: "value"} // 非 callable
	DefaultCompletionLSPSelectFunc(p, p.Value(), p.Cursor(), selected)

	want := "value"
	wantCursor := len(want)
	assertValueCursor(t, p, want, wantCursor)
}
