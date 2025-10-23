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

// LSP protocol structures
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
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("LSP Error (code %d): %s", e.Code, e.Message)
}

// LSPClient structure
type LSPClient struct {
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	cmd            *exec.Cmd
	requestID      int
	requestIDMutex sync.Mutex
	workspacePath  string
	fileURI        string

	pendingRequests map[int]chan *JSONRPCResponse
	pendingMutex    sync.RWMutex
	isReady         bool
	readyChan       chan struct{}
	readyMutex      sync.RWMutex
}

// NewLSPClient creates a new LSP client
func NewLSPClient(ctx context.Context, workspace, filePath string) (*LSPClient, error) {
	logger.Debugf("创建LSPClient...")

	goplsPath, err := exec.LookPath("gopls")
	if err != nil {
		return nil, fmt.Errorf("找不到gopls命令，请确保已安装: %w", err)
	}

	cmd := exec.Command(goplsPath, "serve")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("创建stdin管道失败: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("创建stdout管道失败: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("启动gopls进程失败: %w", err)
	}
	logger.Debugf("gopls进程已启动，PID: %d", cmd.Process.Pid)

	client := &LSPClient{
		stdin:           stdin,
		stdout:          stdout,
		cmd:             cmd,
		workspacePath:   "file://" + workspace,
		fileURI:         "file://" + filePath,
		pendingRequests: make(map[int]chan *JSONRPCResponse),
		readyChan:       make(chan struct{}),
	}

	go client.reader()

	if err := client.initialize(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("初始化LSP失败: %w", err)
	}

	return client, nil
}

// reader is the central message reader from the LSP server
func (c *LSPClient) reader() {
	reader := bufio.NewReader(c.stdout)
	for {
		b, err := receiveMessage(reader)
		if err != nil {
			if err == io.EOF {
				logger.Infof("LSP server stdout closed.")
				return
			}
			logger.Errorf("LSP receiveMessage error: %v", err)
			continue
		}

		var baseMessage struct {
			ID     *int    `json:"id"`
			Method *string `json:"method"`
		}
		if err := json.Unmarshal(b, &baseMessage); err != nil {
			logger.Errorf("Failed to unmarshal base LSP message: %v", err)
			continue
		}

		if baseMessage.ID != nil && baseMessage.Method == nil { // It's a response
			var resp JSONRPCResponse
			if err := json.Unmarshal(b, &resp); err != nil {
				logger.Errorf("Failed to unmarshal LSP response: %v", err)
				continue
			}

			c.pendingMutex.RLock()
			ch, ok := c.pendingRequests[*baseMessage.ID]
			c.pendingMutex.RUnlock()

			if ok {
				ch <- &resp
			} else {
				logger.Warnf("Received response for unknown request ID: %d", *baseMessage.ID)
			}
		} else if baseMessage.ID == nil && baseMessage.Method != nil { // It's a notification
			var notif JSONRPCNotification
			if err := json.Unmarshal(b, &notif); err != nil {
				logger.Errorf("Failed to unmarshal LSP notification: %v", err)
				continue
			}
			c.handleNotification(&notif)
		}
	}
}

func (c *LSPClient) handleNotification(n *JSONRPCNotification) {
	switch n.Method {
	case "window/showMessage":
		paramsBytes, err := json.Marshal(n.Params)
		if err != nil {
			logger.Errorf("Failed to marshal showMessage params: %v", err)
			return
		}
		var params struct {
			Type    int    `json:"type"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(paramsBytes, &params); err == nil {
			logger.Infof("[gopls message]: %s", params.Message)
			if strings.Contains(params.Message, "Finished loading packages") {
				logger.Infof("gopls is ready (detected via showMessage).")
				c.readyMutex.Lock()
				if !c.isReady {
					c.isReady = true
					close(c.readyChan)
				}
			c.readyMutex.Unlock()
			}
		}
	case "window/logMessage":
		logger.Infof("[gopls log]: %s", n.Params)
	default:
		// unhandled
	}
}

// WaitForReady blocks until the client has received a notification that gopls has finished loading packages.
func (c *LSPClient) WaitForReady(ctx context.Context) error {
	c.readyMutex.RLock()
	isReady := c.isReady
	c.readyMutex.RUnlock()
	if isReady {
		return nil
	}

	select {
	case <-c.readyChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *LSPClient) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.requestIDMutex.Lock()
	c.requestID++
	reqID := c.requestID
	c.requestIDMutex.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  method,
		Params:  params,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	respChan := make(chan *JSONRPCResponse, 1)
	c.pendingMutex.Lock()
	c.pendingRequests[reqID] = respChan
	c.pendingMutex.Unlock()

	defer func() {
		c.pendingMutex.Lock()
		delete(c.pendingRequests, reqID)
		c.pendingMutex.Unlock()
	}()

	if err := c.sendMessage(reqData); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *LSPClient) sendNotification(method string, params interface{}) error {
	note := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	noteData, err := json.Marshal(note)
	if err != nil {
		return fmt.Errorf("序列化通知失败: %w", err)
	}
	return c.sendMessage(noteData)
}

func (c *LSPClient) initialize(ctx context.Context) error {
	logger.Debugf("开始初始化LSP连接...")
	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   c.workspacePath,
		"capabilities":  map[string]interface{}{},
	}

	_, err := c.sendRequest(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("发送initialize请求失败: %w", err)
	}
	logger.Debugf("收到initialize响应")

	return c.sendNotification("initialized", map[string]interface{}{})
}

func (c *LSPClient) GetFileURI() string {
	return c.fileURI
}

func (c *LSPClient) sendMessage(message []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(message))
	logger.Debugf("发送消息: %s%s", header, string(message))
	fullMessage := append([]byte(header), message...)
	_, err := c.stdin.Write(fullMessage)
	return err
}

func receiveMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				length, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err == nil {
					contentLength = length
				}
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return nil, err
	}
	logger.Debugf("收到消息: %s", string(body))
	return body, nil
}

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

	result, err := c.sendRequest(ctx, "textDocument/completion", params)
	if err != nil {
		return nil, err
	}

	var completionList CompletionList
	if err := json.Unmarshal(result, &completionList); err != nil {
		return nil, fmt.Errorf("解析补全结果失败: %w", err)
	}
	logger.Debugf("CompletionList length %d", len(completionList.Items))

	return &completionList, nil
}

func (c *LSPClient) Close() error {
	if err := c.sendNotification("exit", nil); err != nil {
		logger.Warnf("Failed to send exit notification: %v", err)
	}

	if c.stdin != nil {
		c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		waitErr := make(chan error, 1)
		go func() {
			waitErr <- c.cmd.Wait()
		}()
		select {
		case err := <-waitErr:
			if err != nil {
				logger.Warnf("gopls process exited with error: %v. Forcing kill.", err)
				_ = c.cmd.Process.Kill()
			} else {
				logger.Infof("gopls process exited gracefully.")
			}
		case <-time.After(2 * time.Second):
			logger.Warnf("gopls process did not exit in 2 seconds. Forcing kill.")
			_ = c.cmd.Process.Kill()
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
