package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readHistoryFile(path string) ([]HistoryItem, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []HistoryItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	items := make([]HistoryItem, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		item, err := parseHistoryLine(line)
		if err != nil {
			logger.Warnf("解析历史行失败: %v", err)
			continue
		}
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func parseHistoryLine(line string) (HistoryItem, error) {
	if !strings.HasPrefix(line, ": ") {
		return HistoryItem{}, fmt.Errorf("历史记录格式不正确: %s", line)
	}
	content := line[2:]
	parts := strings.SplitN(content, ";", 2)
	if len(parts) != 2 {
		return HistoryItem{}, fmt.Errorf("历史记录缺少命令内容: %s", line)
	}
	meta := parts[0]
	commandPart := parts[1]
	metaParts := strings.SplitN(meta, ":", 2)
	if len(metaParts) != 2 {
		return HistoryItem{}, fmt.Errorf("历史记录元数据不完整: %s", line)
	}
	timestamp, err := strconv.ParseInt(strings.TrimSpace(metaParts[0]), 10, 64)
	if err != nil {
		return HistoryItem{}, fmt.Errorf("历史记录时间戳解析失败: %w", err)
	}
	duration, err := strconv.ParseInt(strings.TrimSpace(metaParts[1]), 10, 64)
	if err != nil {
		return HistoryItem{}, fmt.Errorf("历史记录耗时解析失败: %w", err)
	}
	return HistoryItem{
		Timestamp:       timestamp,
		DurationSeconds: duration,
		Command:         unescapeHistoryCommand(commandPart),
	}, nil
}

func formatHistoryItem(item HistoryItem) string {
	return fmt.Sprintf(": %d:%d;%s\n", item.Timestamp, item.DurationSeconds, escapeHistoryCommand(item.Command))
}

func escapeHistoryCommand(command string) string {
	var builder strings.Builder
	for _, r := range command {
		switch r {
		case '\\':
			builder.WriteString("\\\\")
		case '\n':
			builder.WriteString("\\n")
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func unescapeHistoryCommand(command string) string {
	var builder strings.Builder
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if ch != '\\' {
			builder.WriteByte(ch)
			continue
		}
		if i+1 >= len(command) {
			builder.WriteByte('\\')
			break
		}
		next := command[i+1]
		switch next {
		case 'n':
			builder.WriteByte('\n')
		case '\\':
			builder.WriteByte('\\')
		default:
			builder.WriteByte('\\')
			builder.WriteByte(next)
		}
		i++
	}
	return builder.String()
}

func ensureHistoryFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("历史文件禁止使用符号链接: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("历史文件路径不能是目录: %s", path)
		}
		if info.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(path, 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func resolveHistoryFilePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("历史文件禁止使用符号链接: %s", path)
		}
		if info.IsDir() {
			return "", fmt.Errorf("历史文件路径不能是目录: %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func defaultHistoryFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".code-prompt_history")
}
