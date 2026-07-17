package engine

import (
	"context"
	"testing"
)

func TestContextManager(t *testing.T) {
	mgr := NewContextManager()
	
	t.Run("register and exists", func(t *testing.T) {
		ctx, cancel := mgr.Register(context.Background(), "task-1")
		
		if !mgr.Exists("task-1") {
			t.Errorf("expected task-1 to exist")
		}
		
		cancel()
		
		if mgr.Exists("task-1") {
			t.Errorf("expected task-1 to not exist after cancel")
		}
		
		if ctx.Err() == nil {
			t.Errorf("expected context to have error after cancel")
		}
	})
	
	t.Run("cancel task", func(t *testing.T) {
		ctx, _ := mgr.Register(context.Background(), "task-2")
		
		if !mgr.Exists("task-2") {
			t.Errorf("expected task-2 to exist")
		}
		
		success := mgr.Cancel("task-2")
		if !success {
			t.Errorf("expected cancel to succeed")
		}
		
		if ctx.Err() == nil {
			t.Errorf("expected context to be cancelled")
		}
		
		// The original cancel func will remove it from the map when caller defers it,
		// but since we aren't calling the wrapped cancel here, it technically stays in the map.
		// That's fine because Cancel sets context error. In reality the stage finishes and defers cancel.
	})
	
	t.Run("cancel non-existent", func(t *testing.T) {
		success := mgr.Cancel("task-missing")
		if success {
			t.Errorf("expected cancel to fail for missing task")
		}
	})
}
