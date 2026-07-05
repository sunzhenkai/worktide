// Package builtin 集中登记 WorkTide 的内置工具。
//
// 新增内置工具的步骤：
//  1. 在本目录或子包中实现 tools.Tool 接口；
//  2. 在 RegisterAll 中调用 registry.Register 登记它；
//  3. （可选）将其加入 config.Default().Tools.Enabled 默认列表。
package builtin

import (
	"github.com/sunzhenkai/worktide/internal/tools"
)

// RegisterAll 将所有内置工具登记到给定注册中心。
// 重复 ID 会返回错误，调用方可据此决定是否致命。
func RegisterAll(registry *tools.Registry) error {
	for _, t := range []tools.Tool{
		NewWelcome(),
		NewSysInfo(),
	} {
		if err := registry.Register(t); err != nil {
			return err
		}
	}
	return nil
}
