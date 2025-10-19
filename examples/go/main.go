package main

import (
	"bytes"
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

	prompt "github.com/wxnacy/code-prompt"
	"github.com/wxnacy/code-prompt/pkg/tui"
)

func main() {
	// out := insertCodeAndRun("fmt.Println(\"code\")")
	// fmt.Printf("Output: %s\n", out)
	p := prompt.NewPrompt(
		prompt.WithCompletions([]prompt.CompletionItem{
			{Text: "fmt.Println", Desc: "func"},
			{Text: "fmt.Println(\"code\")", Desc: "func"},
			{Text: "fmt.Printf", Desc: "func"},
			{Text: "time.Now", Desc: "func"},
		}),
		prompt.WithOutFunc(func(input string) string {
			return insertCodeAndRun(input)
		}),
	)
	err := tui.NewTerminal(p).Run()
	if err != nil {
		fmt.Printf("go prompt err %v", err)
	}
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

func getName( s string) string {
	return s
}

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
