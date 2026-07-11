package service

import (
	"time"

	"github.com/sunzhenkai/worktide/internal/config"
)

// ServiceDef 是服务的声明定义（cwd/command/env）。
// 直接复用 config.ServiceDef；service 包仅消费声明，不参与配置加载。
type ServiceDef = config.ServiceDef

// ServiceCommand 与 config.ServiceCommand 等价。
type ServiceCommand = config.ServiceCommand

// Status 表示服务的运行态。
type Status string

const (
	// StatusRunning 表示进程当前在运行。
	StatusRunning Status = "running"
	// StatusExited 表示进程曾经运行但当前不在运行。
	StatusExited Status = "exited"
	// StatusStale 表示其他异常情况（启动失败、orphan 等）。
	StatusStale Status = "stale"
)

// Record 是服务运行态记录，持久化到 bbolt bucket "services"。
type Record struct {
	// Name 是服务名（bbolt key）。
	Name string `json:"name"`
	// PID 是进程 ID。
	PID int `json:"pid"`
	// PGID 是进程组 ID（Unix）。
	PGID int `json:"pgid"`
	// Cwd 是工作目录。
	Cwd string `json:"cwd"`
	// Command 是启动命令（已展开）。
	Command ServiceCommand `json:"command"`
	// LogPath 是日志文件路径。
	LogPath string `json:"log_path"`
	// StartedAt 是启动时间。
	StartedAt time.Time `json:"started_at"`
	// LastStatus 是最近一次评估的状态。
	LastStatus Status `json:"last_status"`
	// ExitStatus 是最近一次退出码（未退出时为 nil）。
	ExitStatus *int `json:"exit_status,omitempty"`
	// Source 标记声明来源（"config" 或 "services"）。
	Source string `json:"source"`
}

// Entry 是「声明 + 运行态」的合并视图，列表/状态查询返回该类型。
type Entry struct {
	// Name 是服务名。
	Name string
	// Decl 是声明定义（可为空：服务未在 config/services 中声明）。
	Decl *ServiceDef
	// Record 是当前运行态（可为空：服务未启动）。
	Record *Record
	// Status 是当前状态。
	Status Status
}
