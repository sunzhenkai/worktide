// Package tools 定义 WorkTide 的工具（Tool）抽象与注册中心。
//
// Tool 是 WorkTide 中可插拔模块的统一契约。每个工具自描述元信息，
// 通过 Registry 显式注册，由主壳按配置启用并调度生命周期。
//
// 设计约束（见 spec tool-registry）：
//   - 工具实现 SHALL NOT 直接依赖具体 TUI 框架（如 bubbletea）的类型；
//   - 工具需要持久化/异步任务时 MUST 经 backend.Service 接口访问；
//   - 本包定义的 Event / Renderer / Effect 为稳定抽象，由 UI 层适配具体框架。
package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// Meta 描述工具的自描述元信息。每个工具 MUST 返回唯一且稳定的元信息。
type Meta struct {
	// ID 是工具的全局唯一标识，使用 kebab-case（如 "welcome"）。
	// ID 在配置启用列表、日志、持久化分桶中被引用，一经发布不应变更。
	ID string
	// Name 是面向用户的显示名（简体中文）。
	Name string
	// Description 是工具的一句话简介。
	Description string
	// Icon 是工具在侧边导航的图标字符（emoji 或单字符）。
	Icon string
	// Version 是工具自身的语义版本，用于兼容性判断。
	Version string
}

// KeyType 标识按键的语义类别。
type KeyType int

const (
	// KeyRune 表示普通可打印字符，对应字段 Rune 有效。
	KeyRune KeyType = iota
	// KeyEsc 表示 Esc 键。
	KeyEsc
	// KeyEnter 表示回车键。
	KeyEnter
	// KeyBackspace 表示退格键。
	KeyBackspace
	// KeyTab 表示 Tab 键。
	KeyTab
	// KeyUp / KeyDown / KeyLeft / KeyRight 表示方向键。
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	// KeySpace 表示空格键（也可作为 KeyRune ' ' 出现，UI 层负责归一）。
	KeySpace
	// KeyResize 表示终端尺寸变化（bounds 已更新）。
	KeyResize
	// KeyUnknown 表示未识别的按键。
	KeyUnknown
)

// Key 表示一个抽象化的按键事件，屏蔽底层框架差异。
type Key struct {
	Type KeyType
	// Rune 在 Type == KeyRune 时有效。
	Rune rune
	// Alt 表示是否同时按下 Alt/Option。
	Alt bool
}

// Bounds 描述工具视图可用的渲染区域（字符宽高）。
type Bounds struct {
	Width  int
	Height int
}

// Effect 是工具处理按键后希望主壳执行的副作用。
type Effect int

const (
	// EffectNone 表示无副作用，但仍需按返回的渲染指令决定是否重绘。
	EffectNone Effect = iota
	// EffectRerender 表示请求主壳立即重绘当前工具视图。
	EffectRerender
	// EffectQuit 表示请求退出应用。
	EffectQuit
	// EffectSwitchFocus 表示请求在侧边导航与主内容区之间切换焦点。
	EffectSwitchFocus
	// EffectToggleHelp 表示请求切换帮助面板。
	EffectToggleHelp
	// EffectToggleSettings 表示请求切换设置面板。
	EffectToggleSettings
)

// Result 是 HandleKey 的返回，包含副作用 Effect 与可选的异步任务描述。
//
// 当前版本仅返回 Effect；未来可扩展携带 backend.Task 描述符。
type Result struct {
	Effect Effect
}

// Tool 是所有工具 MUST 实现的统一接口。
//
// 生命周期：Init -> (Activate <-> Deactivate)* -> Close。
// 同一时刻仅一个工具处于激活态；切换时主壳先 Deactivate 旧工具再 Activate 新工具。
type Tool interface {
	// Meta 返回工具的元信息。MUST 在注册前后保持一致。
	Meta() Meta

	// Init 执行一次性初始化（加载资源、连接后端等）。
	// 在工具被启用时调用一次。返回错误将阻止该工具启用并记录日志。
	Init(ctx context.Context) error

	// Activate 将工具置为激活态（获得焦点、开始接收事件）。
	// 可重复调用，与 Deactivate 配对。
	Activate(ctx context.Context) error

	// Deactivate 将工具置为非激活态，暂停其交互但保留状态。
	Deactivate() error

	// Close 释放工具持有的资源，在工具被卸载或应用退出时调用。
	Close() error

	// HandleKey 处理一个按键事件，返回希望主壳执行的副作用。
	HandleKey(key Key) Result

	// View 渲染当前工具视图为字符串。
	// Renderer 提供主题样式查询；bounds 给出可用区域。
	View(bounds Bounds, r Renderer) string
}

// ---- 错误定义 ----

var (
	// ErrDuplicateID 表示尝试以已存在的 ID 注册工具。
	ErrDuplicateID = errors.New("工具 ID 已存在")
	// ErrUnknownTool 表示引用了未注册的工具。
	ErrUnknownTool = errors.New("工具未注册")
)

var kebabCaseRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidateID 校验工具 ID 是否符合 kebab-case 规范。
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("工具 ID 不能为空")
	}
	if !kebabCaseRE.MatchString(id) {
		return fmt.Errorf("工具 ID 必须为 kebab-case（小写字母、数字、连字符）: %q", id)
	}
	return nil
}
