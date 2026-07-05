package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

// DefaultName 是默认实现的标识。
const DefaultName = "worktide-backend"

// DefaultService 是后端的默认实现：异步任务队列 + bbolt KV 持久化。
// 它实现了 Service 接口。
//
// 特性：
//   - 异步任务在独立 goroutine 执行，结果经 channel 回传，支持取消；
//   - 单任务 panic 被隔离并转为错误结果，不影响其他任务与数据库；
//   - KV 按命名分桶隔离，bbolt 单文件持久化，重启后数据仍在。
type DefaultService struct {
	db *bolt.DB

	// 任务管理。
	nextID    uint64
	mu        sync.Mutex
	tasks     map[TaskID]context.CancelFunc
	closed    atomic.Bool
	closeOnce sync.Once

	// 日志。
	log *slog.Logger
}

// New 在指定数据目录创建默认后端。
// dbPath 为 bbolt 数据库文件路径；通常位于数据目录下，如 <data>/worktide.db。
func New(dbPath string, log *slog.Logger) (*DefaultService, error) {
	if log == nil {
		log = slog.Default()
	}
	if dbPath == "" {
		return nil, errors.New("数据库路径不能为空")
	}
	// bbolt 使用 0600 默认权限与 10s 超时打开。
	db, err := bolt.Open(filepath.Clean(dbPath), 0o600, &bolt.Options{Timeout: 10 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	return &DefaultService{
		db:    db,
		tasks: make(map[TaskID]context.CancelFunc),
		log:   log,
	}, nil
}

// Enabled 始终返回 true。
func (s *DefaultService) Enabled() bool { return true }

// Submit 提交一个异步任务。
//
//   - 在独立 goroutine 执行 fn；
//   - 返回一个缓冲为 1 的结果通道，确保即使无人接收也不阻塞；
//   - 任务 panic 被恢复并转为错误结果回传。
func (s *DefaultService) Submit(parent context.Context, fn TaskFunc[any]) (TaskID, <-chan TaskResult[any], error) {
	if s.closed.Load() {
		return "", nil, errors.New("后端已关闭")
	}
	id := TaskID(fmt.Sprintf("task-%d", atomic.AddUint64(&s.nextID, 1)))

	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.tasks[id] = cancel
	s.mu.Unlock()

	ch := make(chan TaskResult[any], 1)

	go func(taskID TaskID) {
		defer func() {
			// 清理任务登记。
			s.mu.Lock()
			delete(s.tasks, taskID)
			s.mu.Unlock()
			cancel()
			close(ch)
		}()
		result := runSafely(ctx, fn, s.log, taskID)
		// channel 缓冲为 1，这里不会阻塞；若无人接收也安全。
		ch <- result
	}(id)

	return id, ch, nil
}

// runSafely 执行任务并隔离 panic。
func runSafely(ctx context.Context, fn TaskFunc[any], log *slog.Logger, id TaskID) (res TaskResult[any]) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("任务 panic，已隔离", "task", id, "panic", r)
			res = TaskResult[any]{
				Err: fmt.Errorf("任务 panic: %v", r),
			}
		}
	}()
	// 在 ctx 取消前执行；fn 内部应感知 ctx。
	out, err := fn(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return TaskResult[any]{Err: err, Canceled: true}
		}
		return TaskResult[any]{Err: err}
	}
	return TaskResult[any]{Result: out}
}

// Cancel 请求取消指定任务。找不到时静默忽略。
func (s *DefaultService) Cancel(id TaskID) {
	s.mu.Lock()
	cancel, ok := s.tasks[id]
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// KVPut 向指定桶写入键值，桶不存在时自动创建。
func (s *DefaultService) KVPut(bucket, key string, value []byte) error {
	if err := validateKV(bucket, key); err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return fmt.Errorf("创建桶 %q 失败: %w", bucket, err)
		}
		return b.Put([]byte(key), cloneBytes(value))
	})
}

// KVGet 从指定桶读取键值。键不存在时返回 (nil, nil)。
func (s *DefaultService) KVGet(bucket, key string) ([]byte, error) {
	if err := validateKV(bucket, key); err != nil {
		return nil, err
	}
	var out []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			out = nil
			return nil
		}
		v := b.Get([]byte(key))
		if v != nil {
			out = cloneBytes(v) // bbolt 的值在事务结束后失效，必须拷贝。
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取 %s/%s 失败: %w", bucket, key, err)
	}
	return out, nil
}

// KVDelete 删除指定键。键或桶不存在时静默忽略。
func (s *DefaultService) KVDelete(bucket, key string) error {
	if err := validateKV(bucket, key); err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
	})
}

// Close 关闭数据库与所有在途任务。可多次调用。
func (s *DefaultService) Close() error {
	var dbErr error
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		// 取消所有在途任务。
		s.mu.Lock()
		for id, cancel := range s.tasks {
			cancel()
			delete(s.tasks, id)
		}
		s.mu.Unlock()
		dbErr = s.db.Close()
	})
	return dbErr
}

// validateKV 校验桶名与键非空。
func validateKV(bucket, key string) error {
	if bucket == "" {
		return errors.New("桶名不能为空")
	}
	if key == "" {
		return errors.New("键不能为空")
	}
	return nil
}

// cloneBytes 复制一份字节切片，避免 bbolt 内存复用导致的隐患。
func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
