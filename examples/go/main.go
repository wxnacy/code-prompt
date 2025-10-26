package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirupsen/logrus"
	prompt "github.com/wxnacy/code-prompt"
	"github.com/wxnacy/code-prompt/pkg/log"
	"github.com/wxnacy/code-prompt/pkg/lsp"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

var (
	logger      = log.GetLogger()
	fileVersion = 0
)

func main() {
	log.SetOutputFile("prompt.log")
	log.SetLogLevel(logrus.DebugLevel)

	workspace, _ := os.Getwd()
	logger.Infof("workspace %s", workspace)
	codeDir := filepath.Join(workspace, ".prompt")
	os.MkdirAll(codeDir, 0o755)
	codePath := filepath.Join(codeDir, "main.go")

	// 构建文件URI和工作区URI
	fileURI := "file://" + codePath
	// workspaceURI := "file://" + workspace // This was unused, keeping it commented

	// 使用可取消上下文防止长时间运行后被统一超时取消
	logger.Debugf("创建可取消的上下文")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Infof("正在启动gopls并建立连接...")
	// 创建LSP客户端
	client, err := lsp.NewLSPClient(ctx, workspace, codePath)
	if err != nil {
		logger.Errorf("创建LSP客户端失败: %v", err)
		fmt.Println("1. 请确保gopls已安装: go install golang.org/x/tools/gopls@latest")
		fmt.Println("2. 请确保go版本 >= 1.16")
		fmt.Println("3. 检查PATH环境变量是否包含gopls")
		return
	}
	defer client.Close()

	// Notify server that we have a file open
	fileVersion++
	err = client.DidOpen(ctx, fileURI, "go", fileVersion, "")
	if err != nil {
		logger.Errorf("Initial DidOpen failed: %v", err)
	}

	fmt.Println("正在等待gopls加载项目包，请稍候...")
	if err := client.WaitForReady(ctx); err != nil {
		logger.Errorf("gopls未能成功加载: %v", err)
		return
	}
	fmt.Println("gopls已就绪，您可以开始输入了！")

	p := prompt.NewPrompt(
		prompt.WithOutFunc(insertCodeAndRun),
		prompt.WithCompletionFunc(func(input string, cursor int) []prompt.CompletionItem {
			return completionFunc(input, cursor, client, ctx)
		}),
		prompt.WithCompletionSelectFunc(completionSelectFunc),
	)
	err = tui.NewTerminal(p).Run()
	if err != nil {
		logger.Errorf("go prompt err %v", err)
	}
}

func completionSelectFunc(p *prompt.Prompt, input string, cursor int, selected prompt.CompletionItem) {
	// text before cursor
	textBeforeCursor := input[:cursor]

	// find last word separator
	wordSeparators := " .()[]{}<>"
	startOfWord := strings.LastIndexAny(textBeforeCursor, wordSeparators)
	if startOfWord == -1 {
		startOfWord = 0 // beginning of the string
	} else {
		startOfWord++ // after the separator
	}

	// text after cursor
	textAfterCursor := input[cursor:]

	newInput := input[:startOfWord] + selected.Text + textAfterCursor
	newCursor := startOfWord + len(selected.Text)

	p.SetValue(newInput)
	p.SetCursor(newCursor)
}

// 补全方法
// 功能需求:
// - 根据 input_suffix 和 cursor 光标结合确认补全的索引
// - 需要判断光标前面的字符是否适合补全，比如括号结尾和空等不适合补全的字符则不进行补全
func completionFunc(input string, cursor int, client *lsp.LSPClient, ctx context.Context) []prompt.CompletionItem {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(input) {
		cursor = len(input)
	}

	inputBefore := input[:cursor]
	if len(inputBefore) == 0 {
		return nil
	}

	prevChar, _ := utf8.DecodeLastRuneInString(inputBefore)
	if prevChar == utf8.RuneError {
		return nil
	}

	if strings.ContainsRune(" \t\n(){}[]", prevChar) {
		return nil
	}

	inputAfter := input[cursor:]

	fileVersion++
	// 根据输入，使用 client 获取补全结果，代码临时存放在 client.fileURI 中
	tpl := `package main

func main() {
	// 在这里我们使用fmt包，触发补全
	%s
}`
	input_suffix := "// :INPUT"
	code := fmt.Sprintf(tpl, inputBefore+input_suffix+inputAfter)

	// 从 file URI 中获取文件路径
	filePath := strings.ReplaceAll(client.GetFileURI(), "file://", "")
	logger.Infof("filePath %s", filePath)

	err := os.WriteFile(filePath, []byte(code), 0o644)
	if err != nil {
		logger.Errorf("写入临时文件失败: %v", err)
		return nil
	}

	// 为单次补全请求设置独立的超时，避免复用过期上下文
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = client.DidOpen(callCtx, client.GetFileURI(), "go", fileVersion, code)
	if err != nil {
		logger.Errorf("textDocument/didOpen failed: %v", err)
	}

	// 计算光标位置
	suffixPos := strings.Index(code, input_suffix)
	if suffixPos == -1 {
		logger.Errorf("Could not find input_suffix in code")
		return nil
	}

	// Get the code content before the suffix
	codeBeforeSuffix := code[:suffixPos]

	// Count lines and character offset
	linesBeforeSuffix := strings.Split(codeBeforeSuffix, "\n")
	row := len(linesBeforeSuffix) - 1
	col := len(linesBeforeSuffix[len(linesBeforeSuffix)-1])

	// 获取补全
	completions, err := client.GetCompletions(callCtx, row, col)
	if err != nil {
		logger.Errorf("获取代码补全失败: %v", err)
		return nil
	}

	if completions == nil {
		return nil
	}

	// 转换补全项
	var items []prompt.CompletionItem
	for _, comp := range completions.Items {
		var desc string
		if comp.Detail != nil {
			desc = *comp.Detail
		}
		items = append(items, prompt.CompletionItem{
			Text: comp.Label,
			Desc: desc,
		})
	}

	return items
}

// processCode finds unused variables in the main function of the provided Go code
// and adds assignments to the blank identifier (_) to make the code compile.
func processCode(code string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "go-process-")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(tmpFile, []byte(code), 0o644); err != nil {
		return "", fmt.Errorf("writing temp file: %w", err)
	}

	// Initialize a temporary go module.
	modCmd := exec.Command("go", "mod", "init", "tmpmodule")
	modCmd.Dir = tmpDir
	var modErr bytes.Buffer
	modCmd.Stderr = &modErr
	if err := modCmd.Run(); err != nil {
		return "", fmt.Errorf("go mod init failed: %s", modErr.String())
	}

	// Run 'go build' and capture stderr. We expect it to fail if there are unused vars.
	cmd := exec.Command("go", "build")
	cmd.Dir = tmpDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Run()

	// Regex to find "declared and not used: var" errors.
	re := regexp.MustCompile(`(?m)^.*: declared and not used: (\w+)$`)
	matches := re.FindAllStringSubmatch(stderr.String(), -1)

	var unusedVars []string
	for _, match := range matches {
		if len(match) > 1 {
			unusedVars = append(unusedVars, match[1])
		}
	}

	if len(unusedVars) == 0 {
		// No unused variables found, or a different build error occurred.
		// For this task, we assume other errors are not present and return the original code.
		return code, nil
	}

	// Use AST to find the position of the main function's closing brace.
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", code, 0)
	if err != nil {
		return "", fmt.Errorf("parsing code: %w", err)
	}

	var mainFuncEnd token.Pos = -1
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFuncEnd = fn.Body.Rbrace
			return false // Stop searching
		}
		return true
	})

	if mainFuncEnd == -1 {
		return "", fmt.Errorf("main function not found")
	}

	// The position is a 1-based offset from the beginning of the file.
	offset := fset.File(mainFuncEnd).Offset(mainFuncEnd)

	var assignments strings.Builder
	for _, v := range unusedVars {
		assignments.WriteString(fmt.Sprintf("\n\t_ = %s", v))
	}

	// Insert the assignments before the closing brace.
	newCode := code[:offset] + assignments.String() + "\n" + code[offset:]

	// Format the resulting code for proper indentation.
	formatted, err := format.Source([]byte(newCode))
	if err != nil {
		// If formatting fails, return the unformatted version.
		return newCode, nil
	}

	return string(formatted), nil
}

func insertCodeAndRun(input string) string {
	curDir, _ := os.Getwd()
	// defer os.Chdir(curDir)
	tpl := `package main


func main() {
	// 在这里我们使用fmt包，触发补全
	%s
}`

	code := fmt.Sprintf(tpl, input)
	code, err := processCode(code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing code: %v\n", err)
	}

	codeDir := filepath.Join(curDir, ".prompt")
	os.MkdirAll(codeDir, 0o755)
	codePath := filepath.Join(codeDir, "main.go")

	err = os.WriteFile(codePath, []byte(code), 0o644)
	if err != nil {
		fmt.Printf("写入临时文件失败: %v\n", err)
		panic(err)
	}

	// os.Chdir(codeDir)
	if _, err := Command("goimports", "-w", codePath); err != nil {
		fmt.Printf("goimports failed: %v\n", err)
	}
	out, err := Command("go", "run", codePath)
	if err != nil {
		fmt.Printf("go run failed: %v\n", err)
		// 即使执行失败，也返回 out
		return out
	}

	// out = strings.Trim(out, "\n")
	// out = fmt.Sprintf("-%s-", out)
	return out + "\n"
}

func Command(name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	var out bytes.Buffer
	var outErr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &outErr
	err := c.Run()

	outStr := strings.TrimSpace(out.String())
	errStr := strings.TrimSpace(outErr.String())

	if err != nil {
		if errStr != "" {
			return outStr, errors.New(errStr)
		}
		return outStr, err
	}

	if errStr != "" {
		// 即使成功，但是 err 有输出，也认为是错误
		return outStr, errors.New(errStr)
	}

	return outStr, nil
}
