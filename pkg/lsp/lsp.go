package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wxnacy/code-prompt/pkg/log"
)

var logger = log.GetLogger()

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
	logger.Debugf("创建LSPClient...")

	// 检查gopls是否存在
	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		logger.Debugf("找不到gopls命令: %v", err)
		return nil, fmt.Errorf("找不到gopls命令，请确保已安装: %w", err)
	}
	logger.Debugf("找到gopls: %s", goplsPath)

	// 启动gopls进程
	logger.Debugf("启动gopls进程...")
	cmd := exec.Command(goplsPath, "serve")
	cmd.Stderr = os.Stderr

	// 创建双向管道连接到gopls进程
	logger.Debugf("创建stdin管道...")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.Debugf("创建stdin管道失败: %v", err)
		return nil, fmt.Errorf("创建stdin管道失败: %w", err)
	}

	logger.Debugf("创建stdout管道...")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		logger.Debugf("创建stdout管道失败: %v", err)
		return nil, fmt.Errorf("创建stdout管道失败: %w", err)
	}

	// 启动gopls进程
	logger.Debugf("执行cmd.Start()...")
	err = cmd.Start()
	if err != nil {
		stdin.Close()
		stdout.Close()
		logger.Debugf("启动gopls进程失败: %v", err)
		return nil, fmt.Errorf("启动gopls进程失败: %w", err)
	}
	logger.Debugf("gopls进程已启动，PID: %d", cmd.Process.Pid)

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

func (c *LSPClient) GetFileURI() string {
	return c.fileURI
}

// 发送LSP消息
func (c *LSPClient) sendMessage(message []byte) error {
	// 构建LSP协议头
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(message))

	logger.Debugf("发送消息头: %v", header)
	// logger.Debugf("发送消息体前100字节: %s", string(message)[:min(100, len(message))])
	logger.Debugf("发送消息体前100字节: %s", string(message))

	// 发送头和消息体
	fullMessage := append([]byte(header), message...)
	bytesWritten, err := c.stdin.Write(fullMessage)
	if err != nil {
		logger.Debugf("写入失败: %v", err)
		return err
	}
	logger.Debugf("成功写入 %d 字节", bytesWritten)
	return nil
}

// 接收LSP消息
func (c *LSPClient) receiveMessage() ([]byte, error) {
	logger.Debugf("开始接收消息...")

	// 使用带缓冲的reader来读取响应
	reader := bufio.NewReader(c.stdout)
	contentLength := 0

	// 读取HTTP头
	headerLines := []string{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			logger.Debugf("读取头部行失败: %v", err)
			return nil, fmt.Errorf("读取响应头失败: %w", err)
		}
		// 去除换行符
		line = strings.TrimRight(line, "\r\n")
		headerLines = append(headerLines, line)
		logger.Debugf("读取到头部行: '%s'", line)

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
				logger.Debugf("解析Content-Length失败: %v", err)
				return nil, fmt.Errorf("解析Content-Length失败: %w", err)
			}
			contentLength = length
			logger.Debugf("解析到Content-Length: %d", contentLength)
		}
	}

	logger.Debugf("所有头部行: %v", headerLines)

	if contentLength == 0 {
		logger.Debugf("警告: Content-Length为0")
		return nil, fmt.Errorf("接收到的消息没有Content-Length头")
	}

	// 直接读取消息体，使用更大的缓冲区和更长的超时
	logger.Debugf("开始读取消息体，大小: %d 字节", contentLength)
	message := make([]byte, contentLength)
	totalRead := 0

	// 增加读取超时时间，使用一个总的超时控制
	timeout := time.Now().Add(30 * time.Second) // 增加到30秒

	// 分批读取，每次读取更多数据
	for totalRead < contentLength {
		// 检查是否超时
		if time.Now().After(timeout) {
			logger.Debugf("读取消息体超时，已读取 %d/%d 字节", totalRead, contentLength)
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
				logger.Debugf("提前到达EOF，已读取 %d/%d 字节", totalRead+bytesRead, contentLength)
				totalRead += bytesRead
				break
			}
			logger.Debugf("读取消息体失败，已读取 %d/%d 字节: %v", totalRead+bytesRead, contentLength, err)
			return nil, fmt.Errorf("读取消息体失败: %w", err)
		}

		if bytesRead == 0 {
			// 没有读取到数据，等待一下再试
			time.Sleep(50 * time.Millisecond)
			continue
		}

		totalRead += bytesRead
		logger.Debugf("已读取 %d/%d 字节", totalRead, contentLength)
	}

	// 检查是否读取了所有数据
	if totalRead < contentLength {
		logger.Debugf("警告：只读取了 %d/%d 字节", totalRead, contentLength)
		// 返回已读取的数据，但这可能会导致后续解析错误
		message = message[:totalRead]
	}

	logger.Debugf("成功读取消息体，共 %d 字节", len(message))
	if len(message) > 0 {
		logger.Debugf("消息体前100字节: %s", string(message)[:min(100, len(message))])
	}

	return message, nil
}

// 处理通知消息
func (c *LSPClient) handleNotifications() {
	defer close(c.notificationChan)
	logger.Debugf("通知处理协程已启动")

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
					logger.Debugf("EOF到达，通知处理结束")
					return
				}
				logger.Debugf("读取通知头部失败: %v", err)
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
			logger.Debugf("读取通知消息体失败: %v", err)
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
							logger.Infof("[gopls消息] %s", message)
						}
					}
				case "window/logMessage":
					if paramsMap, ok := params.(map[string]interface{}); ok {
						if message, ok := paramsMap["message"].(string); ok {
							logger.Infof("[gopls日志] %s", message)
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
			logger.Debugf("收到未匹配的响应 (ID: %d, 期望: %d)，重试...", resp.ID, reqID)
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
	logger.Debugf("开始初始化LSP连接...")

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
	logger.Debugf("发送initialize请求...")
	result, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		logger.Debugf("发送initialize请求失败: %v", err)
		return err
	}
	logger.Debugf("收到initialize响应")

	// 打印initialize响应内容（简略）
	if result != nil {
		resultBytes, _ := json.Marshal(result)
		logger.Debugf("Initialize响应: %s", string(resultBytes)[:100]+"...")
	}

	// 发送initialized通知
	logger.Debugf("发送initialized通知...")
	err = c.sendNotification("initialized", map[string]interface{}{})
	if err != nil {
		logger.Debugf("发送initialized通知失败: %v", err)
		return err
	}

	c.initialized = true
	logger.Debugf("LSP连接初始化完成")
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
	logger.Debugf("===== 光标位置 行: %d 列: %d", line, character)
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
	logger.Debugf("CompletionList length %d", len(completionList.Items))

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

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
