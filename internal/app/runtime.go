package app

import "runtime"

// GoVersion 返回编译期 Go 版本字符串。
func GoVersion() string {
	return runtime.Version()
}
