//go:build windows

package service

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setSysProcAttr 在 Windows 上无 Setpgid 概念，留空。
func setSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Windows 没有 Setpgid；保留 CreationFlags 占位（未来可启用 Job Object）。
	cmd.SysProcAttr.CreationFlags = 0
}

// isProcessAlive 通过 OpenProcess 检查 PID 是否存在。
func isProcessAlive(pid, pgid int) bool {
	if pid <= 0 {
		return false
	}
	// 简化实现：调用 OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)，失败即视为不在。
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = syscall.CloseHandle(h)
	return true
}

// signalProcess 在 Windows 上回退到 PID 信号（无 PGID 概念）。
func signalProcess(pid, pgid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("无效的 pid: %d", pid)
	}
	// 简化：将 sig 映射为 Windows TerminateProcess（仅支持 SIGKILL 等价）。
	if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
		h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			return err
		}
		defer syscall.CloseHandle(h)
		return syscall.TerminateProcess(h, 1)
	}
	return fmt.Errorf("Windows 仅支持 SIGKILL/SIGTERM 终止")
}

// pgidFromCmd 在 Windows 上没有 PGID 概念，返回 0。
func pgidFromCmd(cmd *exec.Cmd) (int, bool) {
	return 0, false
}
