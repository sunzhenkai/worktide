package backend

import "context"

// DisabledService 是后端未启用时的降级实现。
// 所有方法返回「不可用」语义，绝不 panic，使应用能在无后端模式下继续运行。
type DisabledService struct{}

// NewDisabled 创建一个降级后端。
func NewDisabled() *DisabledService { return &DisabledService{} }

// Enabled 始终返回 false。
func (DisabledService) Enabled() bool { return false }

// Submit 返回 ErrBackendDisabled。
func (DisabledService) Submit(_ context.Context, _ TaskFunc[any]) (TaskID, <-chan TaskResult[any], error) {
	return "", nil, ErrBackendDisabled
}

// Cancel 静默忽略。
func (DisabledService) Cancel(_ TaskID) {}

// KVPut 返回 ErrBackendDisabled。
func (DisabledService) KVPut(_, _ string, _ []byte) error { return ErrBackendDisabled }

// KVGet 返回 ErrBackendDisabled。
func (DisabledService) KVGet(_, _ string) ([]byte, error) { return nil, ErrBackendDisabled }

// KVDelete 返回 ErrBackendDisabled。
func (DisabledService) KVDelete(_, _ string) error { return ErrBackendDisabled }

// Close 静默返回 nil。
func (DisabledService) Close() error { return nil }
