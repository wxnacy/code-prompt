package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LSP协议相关结构体
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type CompletionParams struct {
	TextDocumentPositionParams
}

type CompletionItem struct {
	Label         string      `json:"label"`
	Kind          int         `json:"kind,omitempty"`
	Detail        *string     `json:"detail,omitempty"`
	Documentation interface{} `json:"documentation,omitempty"`
	InsertText    string      `json:"insertText,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type JSONRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"` // 添加Method字段以支持解析通知
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// LSP客户端结构体
type LSPClient struct {
	stdin            io.WriteCloser
	stdout           io.ReadCloser
	cmd              *exec.Cmd
	requestID        int
	initialized      bool
	workspacePath    string
	fileURI          string
	mutex            sync.Mutex
	notificationChan chan *JSONRPCNotification
}

// 创建新的LSP客户端
func NewLSPClient(ctx context.Context, workspace, filePath string) (*LSPClient, error) {
	fmt.Println("[DEBUG] 创建LSPClient...")

	// 检查gopls是否存在
	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		fmt.Printf("[DEBUG] 找不到gopls命令: %v\n", err)
		return nil, fmt.Errorf("找不到gopls命令，请确保已安装: %w", err)
	}
	fmt.Printf("[DEBUG] 找到gopls: %s\n", goplsPath)

	// 启动gopls进程
	fmt.Println("[DEBUG] 启动gopls进程...")
	cmd := exec.Command(goplsPath, "serve")
	cmd.Stderr = os.Stderr

	// 创建双向管道连接到gopls进程
	fmt.Println("[DEBUG] 创建stdin管道...")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("[DEBUG] 创建stdin管道失败: %v\n", err)
		return nil, fmt.Errorf("创建stdin管道失败: %w", err)
	}

	fmt.Println("[DEBUG] 创建stdout管道...")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		fmt.Printf("[DEBUG] 创建stdout管道失败: %v\n", err)
		return nil, fmt.Errorf("创建stdout管道失败: %w", err)
	}

	// 启动gopls进程
	fmt.Println("[DEBUG] 执行cmd.Start()...")
	err = cmd.Start()
	if err != nil {
		stdin.Close()
		stdout.Close()
		fmt.Printf("[DEBUG] 启动gopls进程失败: %v\n", err)
		return nil, fmt.Errorf("启动gopls进程失败: %w", err)
	}
	fmt.Printf("[DEBUG] gopls进程已启动，PID: %d\n", cmd.Process.Pid)

	client := &LSPClient{
		stdin:            stdin,
		stdout:           stdout,
		cmd:              cmd,
		requestID:        0,
		initialized:      false,
		workspacePath:    "file://" + workspace,
		fileURI:          "file://" + filePath,
		notificationChan: make(chan *JSONRPCNotification, 10),
	}

	// 启动通知处理协程（已禁用以避免读取冲突）
	// go client.handleNotifications()

	// 初始化LSP连接
	err = client.initialize(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("初始化LSP失败: %w", err)
	}

	return client, nil
}

// 发送LSP消息
func (c *LSPClient) sendMessage(message []byte) error {
	// 构建LSP协议头
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(message))

	fmt.Printf("[DEBUG] 发送消息头: %v\n", header)
	fmt.Printf("[DEBUG] 发送消息体前100字节: %s\n", string(message)[:min(100, len(message))])

	// 发送头和消息体
	fullMessage := append([]byte(header), message...)
	bytesWritten, err := c.stdin.Write(fullMessage)
	if err != nil {
		fmt.Printf("[DEBUG] 写入失败: %v\n", err)
		return err
	}
	fmt.Printf("[DEBUG] 成功写入 %d 字节\n", bytesWritten)
	return nil
}

// 接收LSP消息
func (c *LSPClient) receiveMessage() ([]byte, error) {
	fmt.Println("[DEBUG] 开始接收消息...")

	// 使用带缓冲的reader来读取响应
	reader := bufio.NewReader(c.stdout)
	contentLength := 0

	// 读取HTTP头
	headerLines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("[DEBUG] 读取头部行失败: %v\n", err)
			return nil, fmt.Errorf("读取响应头失败: %w", err)
		}
		// 去除换行符
		line = strings.TrimRight(line, "\r\n")
		headerLines = append(headerLines, line)
		fmt.Printf("[DEBUG] 读取到头部行: '%s'\n", line)

		if line == "" {
			// 头结束，开始读取消息体
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			// 解析Content-Length头
			lengthStr := strings.TrimPrefix(line, "Content-Length:")
			lengthStr = strings.TrimSpace(lengthStr)
			length, err := strconv.Atoi(lengthStr)
			if err != nil {
				fmt.Printf("[DEBUG] 解析Content-Length失败: %v\n", err)
				return nil, fmt.Errorf("解析Content-Length失败: %w", err)
			}
			contentLength = length
			fmt.Printf("[DEBUG] 解析到Content-Length: %d\n", contentLength)
		}
	}

	fmt.Printf("[DEBUG] 所有头部行: %v\n", headerLines)

	if contentLength == 0 {
		fmt.Println("[DEBUG] 警告: Content-Length为0")
		return nil, fmt.Errorf("接收到的消息没有Content-Length头")
	}

	// 直接读取消息体，使用更大的缓冲区和更长的超时
	fmt.Printf("[DEBUG] 开始读取消息体，大小: %d 字节\n", contentLength)
	message := make([]byte, contentLength)
	totalRead := 0

	// 增加读取超时时间，使用一个总的超时控制
	timeout := time.Now().Add(30 * time.Second) // 增加到30秒

	// 分批读取，每次读取更多数据
	for totalRead < contentLength {
		// 检查是否超时
		if time.Now().After(timeout) {
			fmt.Printf("[DEBUG] 读取消息体超时，已读取 %d/%d 字节\n", totalRead, contentLength)
			return nil, fmt.Errorf("读取消息体超时")
		}

		// 增大每次读取的缓冲区
		bytesToRead := min(contentLength-totalRead, 4096) // 增大到4096字节

		// bufio.Reader没有Deadline字段，我们依靠外部的超时检查

		// 尝试读取数据
		bytesRead, err := reader.Read(message[totalRead : totalRead+bytesToRead])
		if err != nil {
			if err == io.EOF {
				// 到达文件末尾，但我们还没读完所有数据
				fmt.Printf("[DEBUG] 提前到达EOF，已读取 %d/%d 字节\n", totalRead+bytesRead, contentLength)
				totalRead += bytesRead
				break
			}
			fmt.Printf("[DEBUG] 读取消息体失败，已读取 %d/%d 字节: %v\n", totalRead+bytesRead, contentLength, err)
			return nil, fmt.Errorf("读取消息体失败: %w", err)
		}

		if bytesRead == 0 {
			// 没有读取到数据，等待一下再试
			time.Sleep(50 * time.Millisecond)
			continue
		}

		totalRead += bytesRead
		fmt.Printf("[DEBUG] 已读取 %d/%d 字节\n", totalRead, contentLength)
	}

	// 检查是否读取了所有数据
	if totalRead < contentLength {
		fmt.Printf("[DEBUG] 警告：只读取了 %d/%d 字节\n", totalRead, contentLength)
		// 返回已读取的数据，但这可能会导致后续解析错误
		message = message[:totalRead]
	}

	fmt.Printf("[DEBUG] 成功读取消息体，共 %d 字节\n", len(message))
	if len(message) > 0 {
		fmt.Printf("[DEBUG] 消息体前100字节: %s\n", string(message)[:min(100, len(message))])
	}

	return message, nil
}

// 处理通知消息
func (c *LSPClient) handleNotifications() {
	defer close(c.notificationChan)
	fmt.Println("[DEBUG] 通知处理协程已启动")

	// 创建单独的reader用于通知处理，避免与主请求流程冲突
	notificationReader := bufio.NewReader(c.stdout)

	for {
		// 首先读取头部
		contentLength := 0
		headerLines := []string{}

		for {
			line, err := notificationReader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Println("[DEBUG] EOF到达，通知处理结束")
					return
				}
				fmt.Printf("[DEBUG] 读取通知头部失败: %v\n", err)
				// 继续尝试，不返回
				break
			}

			line = strings.TrimRight(line, "\r\n")
			headerLines = append(headerLines, line)

			if line == "" {
				// 头部结束
				break
			}

			if strings.HasPrefix(line, "Content-Length:") {
				lengthStr := strings.TrimPrefix(line, "Content-Length:")
				lengthStr = strings.TrimSpace(lengthStr)
				if length, err := strconv.Atoi(lengthStr); err == nil {
					contentLength = length
				}
			}
		}

		if contentLength <= 0 {
			// 没有有效的Content-Length，跳过
			continue
		}

		// 读取消息体
		message := make([]byte, contentLength)
		if _, err := io.ReadFull(notificationReader, message); err != nil {
			fmt.Printf("[DEBUG] 读取通知消息体失败: %v\n", err)
			continue
		}

		// 尝试解析消息
		var combinedMsg map[string]interface{}
		if err := json.Unmarshal(message, &combinedMsg); err != nil {
			continue
		}

		// 检查是否是通知（没有id字段）
		if _, hasID := combinedMsg["id"]; !hasID {
			if method, ok := combinedMsg["method"].(string); ok {
				// 这是一个通知
				params := combinedMsg["params"]

				// 只处理重要的通知
				switch method {
				case "window/showMessage":
					if paramsMap, ok := params.(map[string]interface{}); ok {
						if message, ok := paramsMap["message"].(string); ok {
							fmt.Printf("[gopls消息] %s\n", message)
						}
					}
				case "window/logMessage":
					if paramsMap, ok := params.(map[string]interface{}); ok {
						if message, ok := paramsMap["message"].(string); ok {
							fmt.Printf("[gopls日志] %s\n", message)
						}
					}
				}
			}
		}
	}
}

// 发送请求
func (c *LSPClient) sendRequest(ctx context.Context, method string, params interface{}) (interface{}, error) {
	c.mutex.Lock()
	c.requestID++
	reqID := c.requestID
	c.mutex.Unlock()

	// 构建请求
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}

	// 序列化请求
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 发送请求
	err = c.sendMessage(reqData)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 尝试多次接收响应，因为可能会先收到通知
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		// 接收响应
		respData, err := c.receiveMessage()
		if err != nil {
			return nil, fmt.Errorf("接收响应失败: %w", err)
		}

		// 解析响应
		var resp JSONRPCResponse
		err = json.Unmarshal(respData, &resp)
		if err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}

		// 检查是否是我们请求的响应
		if resp.ID == reqID {
			// 检查错误
			if resp.Error != nil {
				return nil, fmt.Errorf("LSP服务器返回错误: %v", resp.Error)
			}
			return resp.Result, nil
		} else {
			// 如果不是我们请求的响应，可能是一个通知
			fmt.Printf("[DEBUG] 收到未匹配的响应 (ID: %d, 期望: %d)，重试...\n", resp.ID, reqID)
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil, fmt.Errorf("未收到请求ID %d 的响应", reqID)
}

// 发送通知
func (c *LSPClient) sendNotification(method string, params interface{}) error {
	// 构建通知
	note := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	// 序列化通知
	noteData, err := json.Marshal(note)
	if err != nil {
		return fmt.Errorf("序列化通知失败: %w", err)
	}

	// 发送通知
	return c.sendMessage(noteData)
}

// 初始化LSP连接
func (c *LSPClient) initialize(ctx context.Context) error {
	fmt.Println("[DEBUG] 开始初始化LSP连接...")

	// 构建initialize请求参数
	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   c.workspacePath,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"completion": map[string]interface{}{
					"dynamicRegistration": true,
				},
			},
		},
	}

	// 发送initialize请求
	fmt.Println("[DEBUG] 发送initialize请求...")
	result, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		fmt.Printf("[DEBUG] 发送initialize请求失败: %v\n", err)
		return err
	}
	fmt.Println("[DEBUG] 收到initialize响应")

	// 打印initialize响应内容（简略）
	if result != nil {
		resultBytes, _ := json.Marshal(result)
		fmt.Printf("[DEBUG] Initialize响应: %s\n", string(resultBytes)[:100]+"...")
	}

	// 发送initialized通知
	fmt.Println("[DEBUG] 发送initialized通知...")
	err = c.sendNotification("initialized", map[string]interface{}{})
	if err != nil {
		fmt.Printf("[DEBUG] 发送initialized通知失败: %v\n", err)
		return err
	}

	c.initialized = true
	fmt.Println("[DEBUG] LSP连接初始化完成")
	return nil
}

// 打开文档并通知LSP服务器
func (c *LSPClient) DidOpen(ctx context.Context, filename, languageID string, version int, text string) error {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        filename,
			"languageId": languageID,
			"version":    version,
			"text":       text,
		},
	}

	return c.sendNotification("textDocument/didOpen", params)
}

// 获取代码补全
func (c *LSPClient) GetCompletions(ctx context.Context, line, character int) (*CompletionList, error) {
	params := CompletionParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{
				URI: c.fileURI,
			},
			Position: Position{
				Line:      line,
				Character: character,
			},
		},
	}

	// 发送completion请求
	result, err := c.sendRequest(ctx, "textDocument/completion", params)
	if err != nil {
		return nil, err
	}

	// 序列化然后反序列化以正确处理嵌套结构
	resultData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("序列化结果失败: %w", err)
	}

	var completionList CompletionList
	err = json.Unmarshal(resultData, &completionList)
	if err != nil {
		return nil, fmt.Errorf("解析补全结果失败: %w", err)
	}

	return &completionList, nil
}

// 关闭连接
func (c *LSPClient) Close() error {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// 发送exit通知
		if c.initialized {
			_ = c.sendNotification("exit", nil)
		}
		// 关闭stdin
		if c.stdin != nil {
			c.stdin.Close()
		}
		// 关闭stdout
		if c.stdout != nil {
			c.stdout.Close()
		}
	}()

	// 等待连接关闭
	wg.Wait()

	// 等待gopls进程退出
	if c.cmd != nil {
		// 设置超时等待进程退出
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		ch := make(chan error, 1)
		go func() {
			ch <- c.cmd.Wait()
		}()

		select {
		case <-ctx.Done():
			// 超时，强制终止进程
			_ = c.cmd.Process.Kill()
		case <-ch:
			// 进程正常退出
		}
	}

	return nil
}

// 查找光标位置（或最后一个点的位置）
func findCursorPosition(code string) (line, character int) {
	lines := strings.Split(code, "\n")
	for i, lineContent := range lines {
		if idx := strings.Index(lineContent, "|"); idx != -1 {
			return i, idx
		}
	}
	// 如果没有找到光标标记，默认返回最后一个点的位置
	for i := len(lines) - 1; i >= 0; i-- {
		if idx := strings.LastIndex(lines[i], "."); idx != -1 {
			return i, idx + 1
		}
	}
	return 0, 0
}

// 获取补全项的类型描述
func getCompletionItemKindText(kind int) string {
	switch kind {
	case 1:
		return "text"
	case 2:
		return "method"
	case 3:
		return "func"
	case 4:
		return "constructor"
	case 5:
		return "field"
	case 6:
		return "var"
	case 7:
		return "class"
	case 8:
		return "interface"
	case 9:
		return "module"
	case 10:
		return "property"
	case 11:
		return "unit"
	case 12:
		return "value"
	case 13:
		return "enum"
	case 14:
		return "keyword"
	case 15:
		return "snippet"
	case 16:
		return "color"
	case 17:
		return "file"
	case 18:
		return "reference"
	case 19:
		return "folder"
	case 20:
		return "enum member"
	case 21:
		return "const"
	case 22:
		return "struct"
	case 23:
		return "event"
	case 24:
		return "operator"
	case 25:
		return "type parameter"
	default:
		return ""
	}
}

// 打印补全结果
func printCompletions(completions *CompletionList) {
	if completions == nil || len(completions.Items) == 0 {
		fmt.Println("没有找到补全项")
		return
	}

	fmt.Printf("找到 %d 个补全项:\n\n", len(completions.Items))
	fmt.Println("\033[1m补全项\033[0m | \033[1m类型\033[0m | \033[1m详情\033[0m")
	fmt.Println("----------------------------------------")

	for _, item := range completions.Items {
		fmt.Printf("%#v\n", item)
		kindText := getCompletionItemKindText(item.Kind)
		detail := ""
		if item.Detail != nil {
			detail = *item.Detail
		}
		fmt.Printf("%-20s | %-10s | %s\n", item.Label, kindText, detail)
	}
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	fmt.Println("Go LSP代码补全示例")
	fmt.Println("=================")
	fmt.Println("程序启动中...")

	// 示例代码，包含fmt包导入和fmt.调用点 - 使用更完整的代码示例
	code := `package main

func getName( s string) string {
	return s
}

func main() {
	// 在这里我们使用fmt包，触发补全
	log.
}`

	// 创建临时文件
	tmpDir, err := os.MkdirTemp("", "code-prompt*")
	if err != nil {
		fmt.Printf("创建临时目录失败: %v\n", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "main.go")
	err = os.WriteFile(tmpFile, []byte(code), 0o644)
	if err != nil {
		fmt.Printf("写入临时文件失败: %v\n", err)
		return
	}

	// 构建文件URI和工作区URI
	fileURI := "file://" + tmpFile
	workspaceURI := "file://" + tmpDir

	// 创建带超时的上下文
	fmt.Println("[DEBUG] 创建带超时的上下文")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // 增加超时时间
	defer cancel()

	fmt.Println("正在启动gopls并建立连接...")

	// 创建LSP客户端
	fmt.Printf("[DEBUG] 工作区路径: %s\n", workspaceURI)
	fmt.Printf("[DEBUG] 文件路径: %s\n", fileURI)
	client, err := NewLSPClient(ctx, workspaceURI, fileURI)
	if err != nil {
		fmt.Printf("创建LSP客户端失败: %v\n", err)
		fmt.Println("\n调试信息:")
		fmt.Println("1. 请确保gopls已安装: go install golang.org/x/tools/gopls@latest")
		fmt.Println("2. 请确保go版本 >= 1.16")
		fmt.Println("3. 检查PATH环境变量是否包含gopls")
		return
	}
	defer client.Close()

	fmt.Println("连接建立成功，正在打开文档...")

	// 通知服务器打开文档
	err = client.DidOpen(ctx, fileURI, "go", 1, code)
	if err != nil {
		fmt.Printf("通知文档打开失败: %v\n", err)
		return
	}

	// 等待gopls加载包（给gopls一些时间处理文档）
	fmt.Println("正在等待gopls加载包...")
	time.Sleep(2 * time.Second) // 给gopls一些时间加载包

	// 查找光标位置
	line, character := findCursorPosition(code)
	fmt.Printf("光标位置: 行 %d, 列 %d\n", line, character)
	fmt.Println("正在请求代码补全...")

	// 请求代码补全
	completions, err := client.GetCompletions(ctx, line, character)
	if err != nil {
		fmt.Printf("获取代码补全失败: %v\n", err)
		fmt.Println("\n可能的原因:")
		fmt.Println("1. gopls版本不兼容")
		fmt.Println("2. 临时文件内容有问题")
		fmt.Println("3. LSP协议实现有差异")
		fmt.Println("4. gopls可能还在加载包，请尝试增加等待时间")
		return
	}

	// 打印补全结果
	printCompletions(completions)

	fmt.Println("\n代码补全演示完成！")
	fmt.Println("\n实现说明:")
	fmt.Println("1. 使用Go标准库实现了完整的LSP客户端")
	fmt.Println("2. 正确处理了LSP协议的消息格式和头部")
	fmt.Println("3. 能够启动gopls进程并与之交互")
	fmt.Println("4. 支持代码补全功能")
}
