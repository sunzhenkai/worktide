// Package backend 提供 WorkTide 的本地后端能力：异步任务、缓存与持久化。
//
// 设计目标（见 spec local-backend）：
//   - 后端为「可选」组件，未启用时应用以「无后端」模式降级运行；
//   - 对外暴露稳定 Service 接口，与 TUI/工具解耦，未来可替换为进程间通信实现；
//   - 单任务 panic 与错误 MUST 被隔离，不得导致整个应用崩溃。
//
// 工具需要持久化/异步任务时 MUST 经本接口访问，SHALL NOT 直接读写文件或全局状态。
package backend

import (
	"context"
	"errors"
)

// TaskFunc 是后端执行的异步任务函数。
// ctx 在任务被取消时结束；返回值 result 与 err 将随完成事件回传给提交方。
type TaskFunc[T any] func(ctx context.Context) (result T, err error)

// TaskResult 是任务完成时回传的结果。
type TaskResult[T any] struct {
	// Result 是任务成功时的返回值（失败时为零值）。
	Result T
	// Err 是任务返回的错误（包含因取消或 panic 产生的合成错误）。
	Err error
	// Canceled 表示任务是否被取消。
	Canceled bool
}

// TaskID 标识一个已提交的任务，用于查询与取消。
type TaskID string

// ErrBackendDisabled 表示后端未启用，调用方应据此降级。
var ErrBackendDisabled = errors.New("后端未启用")

// ErrBucketNotFound 表示指定 KV 桶不存在。
var ErrBucketNotFound = errors.New("KV 桶不存在")

// Service 是后端对外暴露的统一能力接口。
//
// 实现方包括：
//   - *Service（默认实现，bbolt 持久化 + goroutine 任务队列）；
//   - DisabledService（无后端降级实现）。
//
// 工具与 UI 层仅依赖此接口，便于未来替换为进程间通信版本。
type Service interface {
	// Enabled 返回后端是否处于启用状态。
	Enabled() bool

	// Submit 提交一个异步任务，返回任务 ID 与结果接收通道。
	// 任务在独立 goroutine 执行，结果经 channel 回传，支持通过 ctx 取消。
	// 后端禁用时返回 ErrBackendDisabled。
	Submit(ctx context.Context, fn TaskFunc[any]) (TaskID, <-chan TaskResult[any], error)

	// Cancel 请求取消一个已提交未完成的任务。
	// 找不到对应任务时静默忽略。
	Cancel(id TaskID)

	// KVPut 向指定桶写入键值。桶不存在时自动创建。
	KVPut(bucket, key string, value []byte) error

	// KVGet 从指定桶读取键值。键不存在时返回 (nil, nil)。
	KVGet(bucket, key string) ([]byte, error)

	// KVDelete 从指定桶删除键。键不存在时静默忽略。
	KVDelete(bucket, key string) error

	// Close 释放后端资源（关闭数据库、等待在途任务等）。
	Close() error
}
