//go:build unix

package service

import (
	"fmt"
	"os/exec"
	"syscall"
)

// setSysProcAttr 在 Unix 上启用 Setpgid，使子进程自成进程组。
func setSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// isProcessAlive 通过 kill -0 判断进程是否存活。
// PGID/PID 任一为 0 时退化为 PID 检查。
func isProcessAlive(pid, pgid int) bool {
	target := pid
	if pgid > 0 {
		target = -pgid // 进程组
	}
	if target == 0 {
		return false
	}
	if err := syscall.Kill(target, 0); err != nil {
		return false
	}
	return true
}

// signalProcess 向进程组或进程发送信号。
// pgid > 0 时向 -pgid 发信号，否则向 pid 发信号。
func signalProcess(pid, pgid int, sig syscall.Signal) error {
	target := pid
	if pgid > 0 {
		target = -pgid
	}
	if target == 0 {
		return fmt.Errorf("无效的 pid/pgid: pid=%d pgid=%d", pid, pgid)
	}
	return syscall.Kill(target, sig)
}

// pgidFromCmd 在 Unix 上返回进程 PGID。
// Setpgid=true 时，新进程的 PGID 等于其 PID。
func pgidFromCmd(cmd *exec.Cmd) (int, bool) {
	if cmd == nil || cmd.Process == nil {
		return 0, false
	}
	return cmd.Process.Pid, true
}
