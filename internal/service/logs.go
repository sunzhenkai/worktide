package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// ReadTailLines 读取文件最后 n 行（n<=0 时返回空）。
func ReadTailLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("打开日志失败: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("读取日志失败: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	lines := splitLines(data)
	if n > len(lines) {
		n = len(lines)
	}
	return lines[len(lines)-n:], nil
}

// splitLines 按 \n 切分字节切片。
func splitLines(data []byte) []string {
	var out []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := data[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			out = append(out, string(line))
			start = i + 1
		}
	}
	if start < len(data) {
		tail := data[start:]
		if len(tail) > 0 && tail[len(tail)-1] == '\r' {
			tail = tail[:len(tail)-1]
		}
		out = append(out, string(tail))
	}
	return out
}

// ReadLinesFromOffset 从 offset 字节开始读取文件全部行。
func ReadLinesFromOffset(path string, offset int64) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("打开日志失败: %w", err)
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek 失败: %w", err)
		}
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var out []string
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描失败: %w", err)
	}
	return out, nil
}

// FollowLog 持续把 path 追加内容写入 w；ctx 取消时返回。
func FollowLog(ctx context.Context, path string, w io.Writer) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(200 * time.Millisecond):
				}
				continue
			}
			return fmt.Errorf("打开日志失败: %w", err)
		}
		// 已存在文件：跳到末尾并开始 follow。
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			f.Close()
			return err
		}
		reader := bufio.NewReader(f)
		buf := make([]byte, 4096)
		readDone := false
		for !readDone {
			n, rerr := reader.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					f.Close()
					return werr
				}
			}
			if rerr != nil {
				if rerr == io.EOF {
					select {
					case <-ctx.Done():
						f.Close()
						return ctx.Err()
					case <-time.After(150 * time.Millisecond):
					}
					// 检查文件是否被截断/重建（inode 变化）。
					if truncated, _ := fileChanged(path, f); truncated {
						f.Close()
						break
					}
					continue
				}
				f.Close()
				return rerr
			}
		}
		f.Close()
	}
}

// fileChanged 检查 path 与已打开 f 的 inode/大小是否一致。
func fileChanged(path string, f *os.File) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return true, err
	}
	finfo, err := f.Stat()
	if err != nil {
		return true, err
	}
	if !info.ModTime().Equal(finfo.ModTime()) || info.Size() < finfo.Size() {
		return true, nil
	}
	return false, nil
}

// OpenLog 用系统默认程序打开日志文件。
func OpenLog(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("日志文件不存在: %w", err)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
	return cmd.Start()
}
