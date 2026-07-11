package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// processStartSep 是写入日志的分隔行：标识一次「启动会话」。
const processStartSep = "====== worktide service started at %s (pid will be assigned) ======\n"

// startCommand 启动一个服务进程并把 stdout/stderr 绑定到日志文件。
// 平台特定行为由 process_unix.go / process_windows.go 通过 setSysProcAttr 提供。
//
// 参数：
//   - cwd: 工作目录
//   - command: 已展开的 ServiceCommand
//   - logPath: 日志文件路径（不存在会自动创建；追加模式）
//   - env: 附加到子进程的环境变量（与 os.Environ() 合并）
//
// 返回：构造完成的 exec.Cmd（未启动），日志文件句柄（由调用方在进程退出后关闭）。
func startCommand(cwd string, command ServiceCommand, logPath string, env map[string]string) (*exec.Cmd, *os.File, error) {
	if len(command.Cmd) == 0 {
		return nil, nil, fmt.Errorf("命令为空")
	}
	if logPath == "" {
		return nil, nil, fmt.Errorf("日志路径为空")
	}

	// 1. 准备日志文件所在目录。
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	// 2. 以追加模式打开日志。
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("打开日志文件失败: %w", err)
	}
	// 3. 写分隔行（含时间戳）。
	sep := fmt.Sprintf(processStartSep, time.Now().Format(time.RFC3339))
	if _, err := f.WriteString(sep); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("写日志分隔行失败: %w", err)
	}

	// 4. 构造 exec.Cmd。
	var cmd *exec.Cmd
	if command.Shell {
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", command.Cmd[0])
		} else {
			cmd = exec.Command("sh", "-c", command.Cmd[0])
		}
	} else {
		cmd = exec.Command(command.Cmd[0], command.Cmd[1:]...)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdout = f
	cmd.Stderr = f

	// 5. 设置平台特定的 SysProcAttr（Setpgid 等）。
	setSysProcAttr(cmd)

	// 6. 合并环境变量。
	cmd.Env = mergeEnv(env)

	return cmd, f, nil
}

// mergeEnv 合并用户提供的 env 与 os.Environ()。
// 用户提供的同名键覆盖系统环境。
func mergeEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return os.Environ()
	}
	base := os.Environ()
	out := make([]string, 0, len(base)+len(extra))
	out = append(out, base...)
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}
