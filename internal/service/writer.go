package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// servicesYAMLVersion 是 services.yaml 的当前 schema 版本。
const servicesYAMLVersion = 1

// servicesDoc 是 services.yaml 的顶层结构。
type servicesDoc struct {
	Version  int                      `yaml:"version"`
	Services map[string]ServiceDef    `yaml:"services,omitempty"`
}

// UpsertServiceDecl 把声明写入 services.yaml（原子替换：临时文件 + Rename）。
// 不存在的 services 文件会被创建；同名声明被覆盖。
func UpsertServiceDecl(path string, name string, def ServiceDef) error {
	doc, err := loadServicesDoc(path)
	if err != nil {
		return err
	}
	if doc.Services == nil {
		doc.Services = map[string]ServiceDef{}
	}
	doc.Services[name] = def
	return writeServicesDoc(path, doc)
}

// RemoveServiceDecl 从 services.yaml 移除指定 name 的声明。
// 不存在时返回 nil（幂等）。
func RemoveServiceDecl(path string, name string) error {
	doc, err := loadServicesDoc(path)
	if err != nil {
		return err
	}
	if doc.Services == nil {
		return nil
	}
	if _, ok := doc.Services[name]; !ok {
		return nil
	}
	delete(doc.Services, name)
	return writeServicesDoc(path, doc)
}

// LoadServicesDecls 读取 services.yaml 的全部声明。
// 文件不存在时返回 (nil, nil)。
func LoadServicesDecls(path string) (map[string]ServiceDef, error) {
	doc, err := loadServicesDoc(path)
	if err != nil {
		return nil, err
	}
	return doc.Services, nil
}

// loadServicesDoc 读取并解析 services.yaml。
func loadServicesDoc(path string) (*servicesDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &servicesDoc{Version: servicesYAMLVersion}, nil
		}
		return nil, fmt.Errorf("读取 services.yaml 失败: %w", err)
	}
	doc := &servicesDoc{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("解析 services.yaml 失败: %w", err)
	}
	if doc.Version == 0 {
		doc.Version = servicesYAMLVersion
	}
	return doc, nil
}

// writeServicesDoc 原子写入 services.yaml。
func writeServicesDoc(path string, doc *servicesDoc) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	tmp := path + ".tmp"
	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("序列化 services.yaml 失败: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename 失败: %w", err)
	}
	slog.Debug("services.yaml 已更新", "path", path)
	return nil
}
