package engine

import (
	"context"
	"sync"
)

// ContextManager manages task cancellation contexts globally.
type ContextManager struct {
	cancels sync.Map
}

func NewContextManager() *ContextManager {
	return &ContextManager{}
}

// Register creates a context with cancellation for the taskID, returning the child context and a cancel func.
// The caller must ensure the cancel func is called to free resources.
func (m *ContextManager) Register(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	childCtx, cancel := context.WithCancel(parent)
	
	m.cancels.Store(taskID, cancel)
	
	wrappedCancel := func() {
		cancel()
		m.cancels.Delete(taskID)
	}
	
	return childCtx, wrappedCancel
}

// Cancel immediately cancels the context for the given taskID.
func (m *ContextManager) Cancel(taskID string) bool {
	if v, ok := m.cancels.Load(taskID); ok {
		v.(context.CancelFunc)()
		return true
	}
	return false
}

// CancelStage cancels the current stage but doesn't end the entire task.
func (m *ContextManager) CancelStage(taskID string) bool {
	// P1 placeholder: just cancel the whole task for now.
	return m.Cancel(taskID)
}

// Exists checks if a taskID is currently registered.
func (m *ContextManager) Exists(taskID string) bool {
	_, ok := m.cancels.Load(taskID)
	return ok
}
