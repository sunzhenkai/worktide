package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// maxIncludeDepth 是 include 嵌套深度的上限（包含 config.yaml 自身为第 1 层）。
const maxIncludeDepth = 8

// includeCycleMsg 是循环 include 检测的错误前缀。
const includeCycleMsg = "include cycle detected"

// includeDepthMsg 是 include 嵌套深度超限的错误前缀。
const includeDepthMsg = "include depth exceeded"

// envVarRE 匹配 ${VAR} 与 ${VAR:-default} 形式的引用。
// 捕获组：0=完整 1=变量名 2=可选修饰符+默认值整体 3=修饰符(:-/:+:?) 4=默认值
var envVarRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)((:[-+?]?)([^}]*))?\}`)

// nodeKind 描述一个 YAML 节点在 deep merge 上下文中的类型分类。
type nodeKind int

const (
	kindScalar nodeKind = iota
	kindList
	kindMap
	kindUnknown
)

func kindOf(v any) nodeKind {
	switch v.(type) {
	case map[string]any:
		return kindMap
	case []any:
		return kindList
	default:
		return kindScalar
	}
}

func kindName(k nodeKind) string {
	switch k {
	case kindScalar:
		return "scalar"
	case kindList:
		return "list"
	case kindMap:
		return "map"
	default:
		return "unknown"
	}
}

// expandPath 把 include 路径中的 ~ 与 ${ENV} 引用展开为最终路径。
func expandPath(s string) (string, error) {
	if s == "" {
		return "", errors.New("路径不能为空")
	}

	if s == "~" || strings.HasPrefix(s, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("解析用户主目录失败: %w", err)
		}
		if s == "~" {
			s = home
		} else {
			s = filepath.Join(home, s[2:])
		}
	}

	var unresolved string
	expanded := envVarRE.ReplaceAllStringFunc(s, func(match string) string {
		sub := envVarRE.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		name := sub[1]
		mod := sub[3]
		def := sub[4]

		val, ok := os.LookupEnv(name)
		if !ok || val == "" {
			if mod == ":-" {
				return def
			}
			if unresolved == "" {
				unresolved = name
			}
			return match
		}
		return val + def
	})

	if unresolved != "" {
		return "", fmt.Errorf("environment variable %s is not set", unresolved)
	}

	if strings.Contains(expanded, ":+") || strings.Contains(expanded, ":?") {
		return "", fmt.Errorf("不支持的变量修饰符: %s", expanded)
	}

	return expanded, nil
}

// deepMerge 把 src 中的键逐项合并进 dst（in-place）。
func deepMerge(dst, src map[string]any) error {
	for k, sv := range src {
		dv, exists := dst[k]
		if !exists {
			dst[k] = sv
			continue
		}
		dk, sk := kindOf(dv), kindOf(sv)
		if dk != sk {
			return fmt.Errorf("type mismatch at %q: %s vs %s", k, kindName(dk), kindName(sk))
		}
		if dk == kindMap {
			merged, mErr := mergeMaps(dv.(map[string]any), sv.(map[string]any))
			if mErr != nil {
				return mErr
			}
			dst[k] = merged
		} else {
			dst[k] = sv
		}
	}
	return nil
}

// mergeMaps 合并两个 map 并返回合并结果。
func mergeMaps(dst, src map[string]any) (map[string]any, error) {
	if err := deepMerge(dst, src); err != nil {
		return nil, err
	}
	return dst, nil
}

// mergeIncludes 解析 include 列表并合并到一个 map 中。
//
// 循环检测：visited 跟踪「当前调用栈」中的文件路径，递归共享；
// 兄弟去重：seen 跟踪「同一调用层」中的文件路径，已访问则跳过。
func mergeIncludes(rootPath string, paths []string, visited map[string]bool, depth int) (map[string]any, error) {
	if depth > maxIncludeDepth {
		return nil, fmt.Errorf("%s: > %d", includeDepthMsg, maxIncludeDepth)
	}
	out := map[string]any{}
	seen := map[string]bool{}
	for _, p := range paths {
		expanded, err := expandPath(p)
		if err != nil {
			return nil, fmt.Errorf("展开 include 路径失败 %q: %w", p, err)
		}
		var abs string
		if filepath.IsAbs(expanded) {
			abs = expanded
		} else {
			abs = filepath.Join(rootPath, expanded)
		}
		abs, err = filepath.Abs(abs)
		if err != nil {
			return nil, fmt.Errorf("解析绝对路径失败 %q: %w", expanded, err)
		}
		if seen[abs] {
			// 同一调用层已处理过该文件，跳过（不去重写入）。
			continue
		}
		if visited[abs] {
			return nil, fmt.Errorf("%s: %s", includeCycleMsg, abs)
		}
		seen[abs] = true
		visited[abs] = true
		defer delete(visited, abs)

		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("include %q file not found", abs)
			}
			return nil, fmt.Errorf("include %q stat 失败: %w", abs, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("include %q is a directory", abs)
		}
		f, err := os.Open(abs)
		if err != nil {
			return nil, fmt.Errorf("include %q 不可读: %w", abs, err)
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("include %q 读取失败: %w", abs, err)
		}
		raw := map[string]any{}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("include %q 解析失败: %w", abs, err)
		}

		if sub, ok := raw["include"].([]any); ok {
			subPaths := toStringSlice(sub)
			if len(subPaths) > 0 {
				subBase := filepath.Dir(abs)
				subOut, err := mergeIncludes(subBase, subPaths, visited, depth+1)
				if err != nil {
					return nil, err
				}
				if err := deepMerge(raw, subOut); err != nil {
					return nil, fmt.Errorf("include %q 嵌套合并失败: %w", abs, err)
				}
			}
			delete(raw, "include")
		}

		if err := deepMerge(out, raw); err != nil {
			return nil, fmt.Errorf("include %q 合并失败: %w", abs, err)
		}
	}
	return out, nil
}

// toStringSlice 把 []any 转成 []string（非字符串元素被丢弃）。
func toStringSlice(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
