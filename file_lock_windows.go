//go:build windows

package prompt

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx    = kernel32.NewProc("LockFileEx")
	procUnlockFileEx  = kernel32.NewProc("UnlockFileEx")
	lockFileRangeLow  = uintptr(^uint32(0))
	lockFileRangeHigh = uintptr(^uint32(0))
	lockFileExclusive = uintptr(0x00000002)
)

// lockFile 在 Windows 平台获取文件锁，确保历史文件写入互斥。
func lockFile(file *os.File) error {
	handle := file.Fd()
	var overlapped syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		handle,
		lockFileExclusive,
		0,
		lockFileRangeLow,
		lockFileRangeHigh,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		if err != nil && err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

// unlockFile 释放文件锁。
func unlockFile(file *os.File) error {
	handle := file.Fd()
	var overlapped syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(
		handle,
		0,
		lockFileRangeLow,
		lockFileRangeHigh,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		if err != nil && err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
