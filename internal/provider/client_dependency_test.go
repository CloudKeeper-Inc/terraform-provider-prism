package provider

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// ========== isDependencyNotFoundError tests ==========

func TestIsDependencyNotFoundError_Nil(t *testing.T) {
	if isDependencyNotFoundError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsDependencyNotFoundError_404(t *testing.T) {
	err := fmt.Errorf("API error (404): resource not found")
	if !isDependencyNotFoundError(err) {
		t.Error("expected true for error containing '404'")
	}
}

func TestIsDependencyNotFoundError_NotFound(t *testing.T) {
	err := fmt.Errorf("the resource was Not Found in the system")
	if !isDependencyNotFoundError(err) {
		t.Error("expected true for error containing 'not found' (case-insensitive)")
	}
}

func TestIsDependencyNotFoundError_OtherError(t *testing.T) {
	err := fmt.Errorf("API error (500): internal server error")
	if isDependencyNotFoundError(err) {
		t.Error("expected false for non-404/non-not-found error")
	}
}

func TestIsDependencyNotFoundError_403(t *testing.T) {
	err := fmt.Errorf("API error (403): forbidden")
	if isDependencyNotFoundError(err) {
		t.Error("expected false for 403 error")
	}
}

// ========== waitForDependency tests ==========

func TestWaitForDependency_ImmediateSuccess(t *testing.T) {
	ctx := context.Background()
	start := time.Now()

	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		return nil
	})

	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("immediate success should return quickly, took %v", elapsed)
	}
}

func TestWaitForDependency_SuccessAfterPolling(t *testing.T) {
	ctx := context.Background()
	var calls int64

	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		n := atomic.AddInt64(&calls, 1)
		if n < 3 {
			return fmt.Errorf("API error (404): not found")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected nil error after polling, got: %v", err)
	}
	finalCalls := atomic.LoadInt64(&calls)
	if finalCalls != 3 {
		t.Errorf("expected 3 calls, got %d", finalCalls)
	}
}

func TestWaitForDependency_NonRetryableError(t *testing.T) {
	ctx := context.Background()

	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		return fmt.Errorf("API error (500): internal server error")
	})

	if err == nil {
		t.Fatal("expected error for non-retryable error")
	}
	expected := `error checking test_resource "test-id"`
	if got := err.Error(); !containsSubstring(got, expected) {
		t.Errorf("expected error to contain %q, got: %s", expected, got)
	}
}

func TestWaitForDependency_NonRetryableErrorDuringPolling(t *testing.T) {
	ctx := context.Background()
	var calls int64

	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		n := atomic.AddInt64(&calls, 1)
		if n == 1 {
			return fmt.Errorf("API error (404): not found")
		}
		return fmt.Errorf("API error (500): server broke")
	})

	if err == nil {
		t.Fatal("expected error for non-retryable error during polling")
	}
	finalCalls := atomic.LoadInt64(&calls)
	if finalCalls != 2 {
		t.Errorf("expected 2 calls (first 404, then 500), got %d", finalCalls)
	}
}

func TestWaitForDependency_Timeout(t *testing.T) {
	// Use a short-lived context to avoid waiting 60s
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		return fmt.Errorf("API error (404): not found")
	})

	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Should get context cancelled (5s) before the 60s timeout
	if elapsed > 10*time.Second {
		t.Errorf("should have stopped at context deadline, took %v", elapsed)
	}
}

func TestWaitForDependency_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		return fmt.Errorf("API error (404): not found")
	})

	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
	expected := "context cancelled"
	if got := err.Error(); !containsSubstring(got, expected) {
		t.Errorf("expected error to contain %q, got: %s", expected, got)
	}
}

func TestWaitForDependency_FullTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full timeout test in short mode")
	}

	// This test verifies the 60s internal timeout fires.
	// We use a long context but the function's own 60s deadline should trigger.
	ctx := context.Background()

	start := time.Now()
	err := waitForDependency(ctx, "test_resource", "test-id", func() error {
		return fmt.Errorf("API error (404): not found")
	})

	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	expected := "timed out after"
	if got := err.Error(); !containsSubstring(got, expected) {
		t.Errorf("expected error to contain %q, got: %s", expected, got)
	}
	// Should be approximately 60s (allow some slack)
	if elapsed < 55*time.Second || elapsed > 70*time.Second {
		t.Errorf("expected ~60s timeout, got %v", elapsed)
	}
}

// helper
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
