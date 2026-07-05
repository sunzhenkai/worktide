package tools

// Styler 是工具获取主题样式后用于渲染的抽象。
// UI 层提供基于 lipgloss 的实现；工具仅依赖此稳定接口，不直接依赖 lipgloss 类型。
type Styler interface {
	// Render 把字符串按当前样式渲染后返回（含颜色/边框/填充等）。
	Render(s string) string
}

// Renderer 是工具渲染时获取主题资源的入口。
// 由 UI 层实现并传入 Tool.View。工具通过它取得与当前主题一致的 Styler。
type Renderer interface {
	// StyleFor 按语义键（如 "title"、"muted"、"accent"）取得样式。
	StyleFor(name string) Styler
	// ThemeName 返回当前主题名（如 "default"、"dark"），便于工具做适配。
	ThemeName() string
}

// NoopRenderer 是一个不应用任何样式的 Renderer，主要用于测试。
// 它对任意 StyleFor 都返回一个原样输出的 Styler。
type NoopRenderer struct{}

// noopStyler 原样返回字符串。
type noopStyler struct{}

func (noopStyler) Render(s string) string { return s }

// StyleFor 返回 noop 样式。
func (NoopRenderer) StyleFor(_ string) Styler { return noopStyler{} }

// ThemeName 返回默认主题名。
func (NoopRenderer) ThemeName() string { return "noop" }
