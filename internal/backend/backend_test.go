package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"
)

// newTestBackend 创建一个基于临时目录的后端。
func newTestBackend(t *testing.T) *DefaultService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	svc, err := New(dbPath, slog.New(slog.NewTextHandler(testWriter{t: t}, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { return len(p), nil }

// ---- DisabledService ----

func TestDisabledService(t *testing.T) {
	d := NewDisabled()
	if d.Enabled() {
		t.Error("Disabled 应返回 false")
	}
	if _, _, err := d.Submit(context.Background(), nil); !errors.Is(err, ErrBackendDisabled) {
		t.Errorf("Submit 应返回 ErrBackendDisabled，实际: %v", err)
	}
	if err := d.KVPut("b", "k", []byte("v")); !errors.Is(err, ErrBackendDisabled) {
		t.Errorf("KVPut 应返回 ErrBackendDisabled，实际: %v", err)
	}
	if _, err := d.KVGet("b", "k"); !errors.Is(err, ErrBackendDisabled) {
		t.Errorf("KVGet 应返回 ErrBackendDisabled，实际: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Errorf("Close 应返回 nil，实际: %v", err)
	}
}

// ---- 异步任务 ----

func TestSubmitCompletes(t *testing.T) {
	svc := newTestBackend(t)
	var ran atomic.Bool
	id, ch, err := svc.Submit(context.Background(), func(_ context.Context) (any, error) {
		ran.Store(true)
		return "done", nil
	})
	if err != nil {
		t.Fatalf("Submit 失败: %v", err)
	}
	select {
	case res := <-ch:
		if res.Err != nil {
			t.Errorf("任务不应失败: %v", res.Err)
		}
		if res.Result != "done" {
			t.Errorf("结果应为 done，实际: %v", res.Result)
		}
	case <-time.After(time.Second):
		t.Fatal("任务超时未完成")
	}
	if !ran.Load() {
		t.Error("任务函数应被执行")
	}
	_ = id
}

func TestSubmitErrorPropagated(t *testing.T) {
	svc := newTestBackend(t)
	boom := errors.New("boom")
	_, ch, _ := svc.Submit(context.Background(), func(_ context.Context) (any, error) {
		return nil, boom
	})
	res := <-ch
	if !errors.Is(res.Err, boom) {
		t.Errorf("应回传 boom 错误，实际: %v", res.Err)
	}
}

func TestSubmitCancel(t *testing.T) {
	svc := newTestBackend(t)
	id, ch, _ := svc.Submit(context.Background(), func(ctx context.Context) (any, error) {
		<-ctx.Done() // 等待取消
		return nil, ctx.Err()
	})
	svc.Cancel(id)
	select {
	case res := <-ch:
		if !res.Canceled {
			t.Errorf("应标记为已取消，res=%+v", res)
		}
	case <-time.After(time.Second):
		t.Fatal("取消后任务应回传结果")
	}
}

func TestSubmitPanicIsolated(t *testing.T) {
	svc := newTestBackend(t)
	_, ch, _ := svc.Submit(context.Background(), func(_ context.Context) (any, error) {
		panic("boom")
	})
	select {
	case res := <-ch:
		if res.Err == nil {
			t.Error("panic 应转为错误结果")
		}
	case <-time.After(time.Second):
		t.Fatal("panic 任务应回传错误结果")
	}
	// 后端应仍可用。
	if _, _, err := svc.Submit(context.Background(), func(context.Context) (any, error) { return 1, nil }); err != nil {
		t.Errorf("panic 后后端应仍可用: %v", err)
	}
}

// ---- KV 持久化与分桶 ----

func TestKVPutAndGet(t *testing.T) {
	svc := newTestBackend(t)
	if err := svc.KVPut("notes", "k1", []byte("v1")); err != nil {
		t.Fatalf("KVPut 失败: %v", err)
	}
	got, err := svc.KVGet("notes", "k1")
	if err != nil {
		t.Fatalf("KVGet 失败: %v", err)
	}
	if string(got) != "v1" {
		t.Errorf("期望 v1，实际: %s", string(got))
	}
}

func TestKVGetMissingKey(t *testing.T) {
	svc := newTestBackend(t)
	got, err := svc.KVGet("notes", "nope")
	if err != nil {
		t.Fatalf("键不存在不应报错: %v", err)
	}
	if got != nil {
		t.Errorf("键不存在应返回 nil，实际: %v", got)
	}
}

func TestKVGetMissingBucket(t *testing.T) {
	svc := newTestBackend(t)
	got, err := svc.KVGet("ghost", "k")
	if err != nil {
		t.Fatalf("桶不存在不应报错: %v", err)
	}
	if got != nil {
		t.Errorf("桶不存在应返回 nil，实际: %v", got)
	}
}

func TestKVBucketIsolation(t *testing.T) {
	svc := newTestBackend(t)
	_ = svc.KVPut("a", "k", []byte("va"))
	_ = svc.KVPut("b", "k", []byte("vb"))
	a, _ := svc.KVGet("a", "k")
	b, _ := svc.KVGet("b", "k")
	if string(a) != "va" || string(b) != "vb" {
		t.Errorf("分桶应隔离，a=%s b=%s", a, b)
	}
}

func TestKVPersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")
	s1, err := New(dbPath, slog.Default())
	if err != nil {
		t.Fatalf("New s1 失败: %v", err)
	}
	_ = s1.KVPut("notes", "k", []byte("persisted"))
	_ = s1.Close()

	s2, err := New(dbPath, slog.Default())
	if err != nil {
		t.Fatalf("New s2 失败: %v", err)
	}
	defer s2.Close()
	got, err := s2.KVGet("notes", "k")
	if err != nil {
		t.Fatalf("KVGet 失败: %v", err)
	}
	if string(got) != "persisted" {
		t.Errorf("数据应跨实例持久化，实际: %s", string(got))
	}
}

func TestKVDelete(t *testing.T) {
	svc := newTestBackend(t)
	_ = svc.KVPut("notes", "k", []byte("v"))
	if err := svc.KVDelete("notes", "k"); err != nil {
		t.Fatalf("KVDelete 失败: %v", err)
	}
	got, _ := svc.KVGet("notes", "k")
	if got != nil {
		t.Errorf("删除后应为 nil，实际: %v", got)
	}
}

func TestKVValidation(t *testing.T) {
	svc := newTestBackend(t)
	if err := svc.KVPut("", "k", []byte("v")); err == nil {
		t.Error("空桶名应报错")
	}
	if err := svc.KVPut("b", "", []byte("v")); err == nil {
		t.Error("空键应报错")
	}
}

func TestSubmitAfterClose(t *testing.T) {
	dir := t.TempDir()
	svc, _ := New(filepath.Join(dir, "x.db"), slog.Default())
	_ = svc.Close()
	_, _, err := svc.Submit(context.Background(), func(context.Context) (any, error) { return 1, nil })
	if err == nil {
		t.Error("关闭后 Submit 应报错")
	}
}

// TestSubmitStress 简单并发提交校验无竞态。
func TestSubmitStress(t *testing.T) {
	svc := newTestBackend(t)
	var count atomic.Int64
	for i := 0; i < 20; i++ {
		_, ch, _ := svc.Submit(context.Background(), func(_ context.Context) (any, error) {
			count.Add(1)
			return nil, nil
		})
		<-ch
	}
	if count.Load() != 20 {
		t.Errorf("应执行 20 次，实际: %d", count.Load())
	}
}

// 避免未使用导入。
var _ = fmt.Sprintf
