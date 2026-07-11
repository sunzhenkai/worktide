package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunzhenkai/worktide/internal/service"
)

// captureStdout 临时把 stdout 重定向到管道并返回读取器。
func captureStdout(t *testing.T) (*os.File, *bytes.Buffer, func()) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		io.Copy(buf, r)
		close(done)
	}()
	cleanup := func() {
		_ = w.Close()
		<-done
		_ = r.Close()
		os.Stdout = orig
	}
	return w, buf, cleanup
}

// TestRootCmdHelp 校验 --help 输出。
func TestRootCmdHelp(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("--help 失败: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "worktide") && !strings.Contains(out, "WorkTide") {
		t.Errorf("--help 应包含 worktide / WorkTide: %s", out)
	}
}

// TestSvcCmdHelp 校验 svc --help 列出所有 9 个子命令。
func TestSvcCmdHelp(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd()
	root.AddCommand(newSvcCmd())
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"svc", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("svc --help 失败: %v", err)
	}
	out := buf.String()
	for _, sub := range []string{"run", "list", "status", "logs", "kill", "restart", "rm", "clean", "dir"} {
		if !strings.Contains(out, sub) {
			t.Errorf("svc --help 应包含 %q", sub)
		}
	}
}

// TestRootUnknownSubcommand 校验根级别未知子命令报错。
func TestRootUnknownSubcommand(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCmd()
	root.AddCommand(newSvcCmd())
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"unknown-cmd"})
	err := root.Execute()
	if err == nil {
		t.Fatal("未知子命令应返回错误")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("错误信息应包含 unknown command: %v", err)
	}
}

// TestParseRunArgs 验证 -- 分隔解析。
func TestParseRunArgs(t *testing.T) {
	cases := []struct {
		args      []string
		name      string
		wantCmd   string
		wantShell bool
	}{
		{[]string{}, "", "", false},
		{[]string{"api"}, "api", "", false},
		{[]string{"--", "make dev"}, "", "make dev", true},
		{[]string{"api", "--", "make dev"}, "api", "make dev", true},
		{[]string{"api", "--", "pnpm", "dev"}, "api", "pnpm dev", false},
		{[]string{"--", "pnpm", "dev"}, "", "pnpm dev", false},
	}
	for _, tc := range cases {
		name, cmd, err := parseRunArgs(tc.args)
		if err != nil && tc.wantCmd != "" {
			t.Errorf("parseRunArgs(%v) 失败: %v", tc.args, err)
			continue
		}
		if name != tc.name {
			t.Errorf("parseRunArgs(%v) name = %q, want %q", tc.args, name, tc.name)
		}
		if tc.wantCmd != "" {
			got := cmd.String()
			if got != tc.wantCmd {
				t.Errorf("parseRunArgs(%v) cmd = %q, want %q", tc.args, got, tc.wantCmd)
			}
			if cmd.Shell != tc.wantShell {
				t.Errorf("parseRunArgs(%v) shell = %v, want %v", tc.args, cmd.Shell, tc.wantShell)
			}
		}
	}
}

// TestParseRunArgsMissingCommand 验证 -- 后无命令报错。
func TestParseRunArgsMissingCommand(t *testing.T) {
	_, _, err := parseRunArgs([]string{"api", "--"})
	if err == nil {
		t.Error("-- 后无命令应报错")
	}
}

// TestSvcRunFlow 端到端：起一个 sleep 服务并验证 services.yaml 被写入。
func TestSvcRunFlow(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("HOME", base)

	cfgDir := filepath.Join(base, "Library", "Application Support", "worktide")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("backend:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, out, cleanup := captureStdout(t)
	defer cleanup()

	root := newRootCmd()
	root.AddCommand(newSvcCmd())
	root.SetArgs([]string{"svc", "run", "testsleep", "--", "sleep", "3"})
	if err := root.Execute(); err != nil {
		t.Fatalf("svc run 失败: %v / 输出: %s", err, out.String())
	}
	if !strings.Contains(out.String(), "started service") {
		t.Errorf("输出应含 started service: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "services.yaml")); err != nil {
		t.Errorf("services.yaml 未写入: %v", err)
	}
	// 清理：kill。
	root.SetArgs([]string{"svc", "kill", "testsleep"})
	if err := root.Execute(); err != nil {
		t.Logf("svc kill 退出（可能正常）: %v", err)
	}
}

// TestServicesYAMLRoundTrip 验证 services.yaml 的读写。
func TestServicesYAMLRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.yaml")
	def := service.ServiceDef{Cwd: "/x", Command: service.ServiceCommand{Shell: true, Cmd: []string{"make dev"}}}
	if err := service.UpsertServiceDecl(path, "api", def); err != nil {
		t.Fatalf("UpsertServiceDecl 失败: %v", err)
	}
	decls, err := service.LoadServicesDecls(path)
	if err != nil {
		t.Fatalf("LoadServicesDecls 失败: %v", err)
	}
	if decls["api"].Cwd != "/x" {
		t.Errorf("services.yaml 读回错误: %+v", decls)
	}
	if err := service.RemoveServiceDecl(path, "api"); err != nil {
		t.Errorf("RemoveServiceDecl 失败: %v", err)
	}
	decls2, _ := service.LoadServicesDecls(path)
	if _, ok := decls2["api"]; ok {
		t.Error("RemoveServiceDecl 后应消失")
	}
}
