//go:build !windows

package prompt

import (
	"os"
	"syscall"
)

// lockFile 在非 Windows 平台获取文件锁，确保历史文件写入互斥。
func lockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

// unlockFile 释放文件锁。
func unlockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
