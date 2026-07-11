package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// ---- expandPath ----

func TestExpandPathTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("UserHomeDir 失败: %v", err)
	}
	cases := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{"/abs/~x", "/abs/~x"},
	}
	for _, tc := range cases {
		got, err := expandPath(tc.in)
		if err != nil {
			t.Errorf("expandPath(%q) 失败: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("expandPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExpandPathEnv(t *testing.T) {
	t.Setenv("WT_TEST_VAR", "/tmp/wt-test")
	got, err := expandPath("${WT_TEST_VAR}/sub")
	if err != nil {
		t.Fatalf("expandPath 失败: %v", err)
	}
	if got != "/tmp/wt-test/sub" {
		t.Errorf("expandPath 结果 = %q, want /tmp/wt-test/sub", got)
	}
}

func TestExpandPathEnvUnsetError(t *testing.T) {
	os.Unsetenv("WT_NOT_SET_VAR")
	_, err := expandPath("${WT_NOT_SET_VAR}/x")
	if err == nil {
		t.Fatal("未设置变量应报错")
	}
	if !strings.Contains(err.Error(), "is not set") {
		t.Errorf("错误信息应包含 'is not set'，实际: %v", err)
	}
}

func TestExpandPathEnvDefault(t *testing.T) {
	os.Unsetenv("WT_NOT_SET_VAR")
	got, err := expandPath("${WT_NOT_SET_VAR:-/etc/wt}/x")
	if err != nil {
		t.Fatalf("expandPath 失败: %v", err)
	}
	if got != "/etc/wt/x" {
		t.Errorf("默认展开错误: %q", got)
	}
}

// ---- deepMerge ----

func TestDeepMergeMapKeys(t *testing.T) {
	dst := map[string]any{
		"a": map[string]any{"x": 1},
		"b": "value",
	}
	src := map[string]any{
		"a": map[string]any{"y": 2},
		"c": "new",
	}
	if err := deepMerge(dst, src); err != nil {
		t.Fatalf("deepMerge 失败: %v", err)
	}
	a, _ := dst["a"].(map[string]any)
	if a["x"] != 1 || a["y"] != 2 {
		t.Errorf("map 合并错误: a=%v", a)
	}
	if dst["b"] != "value" || dst["c"] != "new" {
		t.Errorf("标量合并错误: b=%v c=%v", dst["b"], dst["c"])
	}
}

func TestDeepMergeListReplace(t *testing.T) {
	dst := map[string]any{"xs": []any{1, 2, 3}}
	src := map[string]any{"xs": []any{4, 5}}
	if err := deepMerge(dst, src); err != nil {
		t.Fatalf("deepMerge 失败: %v", err)
	}
	got, _ := dst["xs"].([]any)
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Errorf("list 替换错误: %v", got)
	}
}

func TestDeepMergeTypeMismatch(t *testing.T) {
	dst := map[string]any{"x": "scalar"}
	src := map[string]any{"x": map[string]any{"y": 1}}
	err := deepMerge(dst, src)
	if err == nil {
		t.Fatal("类型冲突应报错")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("错误信息应包含 type mismatch，实际: %v", err)
	}
}

func TestDeepMergeScalarOverride(t *testing.T) {
	dst := map[string]any{"x": "old"}
	src := map[string]any{"x": "new"}
	if err := deepMerge(dst, src); err != nil {
		t.Fatalf("deepMerge 失败: %v", err)
	}
	if dst["x"] != "new" {
		t.Errorf("标量覆盖错误: %v", dst["x"])
	}
}

// ---- mergeIncludes ----

func TestMergeIncludesSingleFile(t *testing.T) {
	base := t.TempDir()
	inc := filepath.Join(base, "inc.yaml")
	if err := os.WriteFile(inc, []byte("services:\n  api: {cwd: /A, command: make}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := mergeIncludes(base, []string{inc}, map[string]bool{}, 1)
	if err != nil {
		t.Fatalf("mergeIncludes 失败: %v", err)
	}
	if _, ok := out["services"]; !ok {
		t.Errorf("include 文件内容缺失: %v", out)
	}
}

func TestMergeIncludesNotFound(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "missing.yaml")
	_, err := mergeIncludes(base, []string{missing}, map[string]bool{}, 1)
	if err == nil {
		t.Fatal("缺失文件应报错")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("错误信息应包含 file not found，实际: %v", err)
	}
}

func TestMergeIncludesIsDirectory(t *testing.T) {
	base := t.TempDir()
	_, err := mergeIncludes(base, []string{base}, map[string]bool{}, 1)
	if err == nil {
		t.Fatal("目录应报错")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("错误信息应包含 is a directory，实际: %v", err)
	}
}

func TestMergeIncludesCycle(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "a.yaml")
	b := filepath.Join(base, "b.yaml")
	if err := os.WriteFile(a, []byte("include: [b.yaml]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("include: [a.yaml]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	visited := map[string]bool{a: true, b: true}
	_, err := mergeIncludes(base, []string{a}, visited, 1)
	if err == nil {
		t.Fatal("循环 include 应报错")
	}
	if !strings.Contains(err.Error(), includeCycleMsg) {
		t.Errorf("错误信息应包含 cycle，实际: %v", err)
	}
}

func TestMergeIncludesDepthExceeded(t *testing.T) {
	base := t.TempDir()
	for i := 0; i < 10; i++ {
		name := string(rune('a'+i)) + ".yaml"
		p := filepath.Join(base, name)
		var content string
		if i < 9 {
			next := string(rune('a'+i+1)) + ".yaml"
			content = "include: [" + next + "]\n"
		} else {
			content = "x: 1\n"
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	visited := map[string]bool{}
	_, err := mergeIncludes(base, []string{"a.yaml"}, visited, 1)
	if err == nil {
		t.Fatal("深度超限应报错")
	}
	if !strings.Contains(err.Error(), includeDepthMsg) {
		t.Errorf("错误信息应包含 depth exceeded，实际: %v", err)
	}
}

func TestMergeIncludesSameFileMultipleTimes(t *testing.T) {
	base := t.TempDir()
	inc := filepath.Join(base, "inc.yaml")
	if err := os.WriteFile(inc, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := mergeIncludes(base, []string{inc, inc}, map[string]bool{}, 1)
	if err != nil {
		t.Fatalf("同文件多次引用应仅合并一次，实际: %v", err)
	}
	if out["x"] != 1 {
		t.Errorf("合并结果错误: %v", out)
	}
}

// ---- loadConfig 完整流程 ----

func TestLoadConfigIncludeOrder(t *testing.T) {
	base := t.TempDir()
	paths := Paths{
		ConfigDir: base,
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "data", "logs"),
	}
	inc := filepath.Join(base, "inc.yaml")
	if err := os.WriteFile(inc, []byte("services:\n  web: {cwd: /W, command: pnpm dev}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := []byte("include: [inc.yaml]\nservices:\n  api: {cwd: /A, command: make dev}\n")
	if err := os.WriteFile(paths.ConfigFilePath(), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	svcs := []byte("services:\n  api: {cwd: /B, command: pnpm dev}\n  redis: {cwd: /R, command: redis}\n")
	if err := os.WriteFile(paths.ServicesFile(), svcs, 0o600); err != nil {
		t.Fatal(err)
	}
	c, _, err := loadConfig(paths)
	if err != nil {
		t.Fatalf("loadConfig 失败: %v", err)
	}
	if c.Services["api"].Cwd != "/B" {
		t.Errorf("services.yaml 应覆盖 config.yaml 的 api.cwd，期望 /B，实际: %q", c.Services["api"].Cwd)
	}
	if c.Services["web"].Cwd != "/W" {
		t.Errorf("include 应保留 web 条目，实际: %q", c.Services["web"].Cwd)
	}
	if c.Services["redis"].Cwd != "/R" {
		t.Errorf("services.yaml 应新增 redis，实际: %q", c.Services["redis"].Cwd)
	}
}

func TestLoadConfigServicesYAMLIncludeIgnored(t *testing.T) {
	base := t.TempDir()
	paths := Paths{
		ConfigDir: base,
		DataDir:   filepath.Join(base, "data"),
		CacheDir:  filepath.Join(base, "cache"),
		LogDir:    filepath.Join(base, "data", "logs"),
	}
	// services.yaml 包含 include 字段（应被忽略并 WARN）。
	svcs := []byte("include: [evil.yaml]\nservices:\n  api: {cwd: /A, command: make}\n")
	if err := os.WriteFile(paths.ServicesFile(), svcs, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(paths); err != nil {
		t.Fatalf("loadConfig 失败: %v", err)
	}
}

// ---- ServiceCommand ----

func TestServiceCommandShellVsArgv(t *testing.T) {
	var c1 ServiceCommand
	if err := c1.UnmarshalYAML(yamlScalar("make dev")); err != nil {
		t.Fatalf("scalar 解码失败: %v", err)
	}
	if !c1.Shell || c1.String() != "make dev" {
		t.Errorf("shell 形态错误: %+v", c1)
	}

	var c2 ServiceCommand
	if err := c2.UnmarshalYAML(yamlSeq([]string{"pnpm", "dev"})); err != nil {
		t.Fatalf("seq 解码失败: %v", err)
	}
	if c2.Shell || c2.String() != "pnpm dev" {
		t.Errorf("argv 形态错误: %+v", c2)
	}
}

func TestServiceCommandEmptyArray(t *testing.T) {
	var c ServiceCommand
	if err := c.UnmarshalYAML(yamlSeq(nil)); err == nil {
		t.Error("空数组应报错")
	}
}

// 辅助：构造 yaml scalar/sequence 节点。
func yamlScalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
}

func yamlSeq(vs []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range vs {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: v})
	}
	return n
}
