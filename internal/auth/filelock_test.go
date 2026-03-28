package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireTokenLock_Basic(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	lock, err := acquireTokenLock(configDir)
	if err != nil {
		t.Fatalf("acquireTokenLock() error = %v", err)
	}

	// Lock file should exist while held.
	lockPath := filepath.Join(configDir, lockFileName)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should exist while held, stat error = %v", err)
	}

	lock.release()

	// Lock file may still exist on disk after release (flock does not remove
	// the file), but we should be able to re-acquire it.
	lock2, err := acquireTokenLock(configDir)
	if err != nil {
		t.Fatalf("re-acquire after release error = %v", err)
	}
	lock2.release()
}

func TestAcquireTokenLock_CreatesDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	configDir := filepath.Join(base, "a", "b", "c")

	lock, err := acquireTokenLock(configDir)
	if err != nil {
		t.Fatalf("acquireTokenLock() error = %v", err)
	}
	defer lock.release()

	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat(configDir) error = %v", err)
	}
	if !info.IsDir() {
		t.Fatal("configDir should be a directory")
	}
}

func TestAcquireTokenLock_DoubleRelease(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	lock, err := acquireTokenLock(configDir)
	if err != nil {
		t.Fatalf("acquireTokenLock() error = %v", err)
	}

	// First release should work fine.
	lock.release()

	// Second release should not panic.
	lock.release()
}

func TestAcquireTokenLock_Contention(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	// Goroutine 1 acquires the lock first.
	lock1, err := acquireTokenLock(configDir)
	if err != nil {
		t.Fatalf("acquireTokenLock() g1 error = %v", err)
	}

	acquired := make(chan struct{})
	var g2Err error
	var wg sync.WaitGroup
	wg.Add(1)

	// Goroutine 2 tries to acquire — should block until g1 releases.
	go func() {
		defer wg.Done()
		lock2, err := acquireTokenLock(configDir)
		if err != nil {
			g2Err = err
			close(acquired)
			return
		}
		close(acquired)
		lock2.release()
	}()

	// Give goroutine 2 a moment to start blocking.
	time.Sleep(100 * time.Millisecond)

	// Verify goroutine 2 has not acquired yet.
	select {
	case <-acquired:
		t.Fatal("goroutine 2 should not have acquired the lock while goroutine 1 holds it")
	default:
		// Expected: goroutine 2 is still waiting.
	}

	// Release lock1 so goroutine 2 can proceed.
	lock1.release()

	// Wait for goroutine 2 to finish.
	wg.Wait()

	if g2Err != nil {
		t.Fatalf("acquireTokenLock() g2 error = %v", g2Err)
	}
}

func TestAcquireTokenLock_LockFilePermissions(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	lock, err := acquireTokenLock(configDir)
	if err != nil {
		t.Fatalf("acquireTokenLock() error = %v", err)
	}
	defer lock.release()

	lockPath := filepath.Join(configDir, lockFileName)
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("Stat(lock file) error = %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Fatalf("lock file permissions = %o, want 0600", perm)
	}
}

// ─── Process-level lock tests ───────────────────────────────────────────

func TestAcquireProcessLock_Basic(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	ctx := context.Background()

	release, waited, err := acquireProcessLock(ctx, configDir)
	if err != nil {
		t.Fatalf("acquireProcessLock() error = %v", err)
	}
	if waited {
		t.Fatal("should not have waited on first acquisition")
	}

	release()

	// Should be able to re-acquire after release
	release2, waited2, err := acquireProcessLock(ctx, configDir)
	if err != nil {
		t.Fatalf("re-acquire after release error = %v", err)
	}
	if waited2 {
		t.Fatal("should not have waited on re-acquisition")
	}
	release2()
}

func TestAcquireProcessLock_Contention(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	ctx := context.Background()

	// Goroutine 1 acquires the lock first
	release1, _, err := acquireProcessLock(ctx, configDir)
	if err != nil {
		t.Fatalf("acquireProcessLock() g1 error = %v", err)
	}

	acquired := make(chan bool, 1)
	var g2Waited bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Goroutine 2 tries to acquire — should block until g1 releases
	go func() {
		defer wg.Done()
		release2, waited, err := acquireProcessLock(ctx, configDir)
		if err != nil {
			acquired <- false
			return
		}
		g2Waited = waited
		acquired <- true
		release2()
	}()

	// Give goroutine 2 a moment to start blocking
	time.Sleep(50 * time.Millisecond)

	// Verify goroutine 2 has not acquired yet
	select {
	case <-acquired:
		t.Fatal("goroutine 2 should not have acquired the lock while goroutine 1 holds it")
	default:
		// Expected: goroutine 2 is still waiting
	}

	// Release lock1 so goroutine 2 can proceed
	release1()

	// Wait for goroutine 2 to finish
	wg.Wait()

	if !g2Waited {
		t.Fatal("goroutine 2 should have reported that it waited")
	}
}

func TestAcquireProcessLock_ContextCancellation(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	// Goroutine 1 holds the lock
	release1, _, err := acquireProcessLock(ctx, configDir)
	if err != nil {
		t.Fatalf("acquireProcessLock() g1 error = %v", err)
	}
	defer release1()

	// Goroutine 2 tries to acquire with a cancellable context
	done := make(chan error, 1)
	go func() {
		_, _, err := acquireProcessLock(ctx, configDir)
		done <- err
	}()

	// Give goroutine 2 time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Goroutine 2 should return with context.Canceled
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("goroutine 2 did not return after context cancellation")
	}
}

// ─── Dual-layer lock tests ──────────────────────────────────────────────

func TestAcquireDualLock_Basic(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	ctx := context.Background()

	lock, err := AcquireDualLock(ctx, configDir)
	if err != nil {
		t.Fatalf("AcquireDualLock() error = %v", err)
	}
	if lock.Waited {
		t.Fatal("should not have waited on first acquisition")
	}

	lock.Release()

	// Should be able to re-acquire after release
	lock2, err := AcquireDualLock(ctx, configDir)
	if err != nil {
		t.Fatalf("re-acquire after release error = %v", err)
	}
	lock2.Release()
}

func TestAcquireDualLock_DoubleRelease(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	ctx := context.Background()

	lock, err := AcquireDualLock(ctx, configDir)
	if err != nil {
		t.Fatalf("AcquireDualLock() error = %v", err)
	}

	// First release should work fine
	lock.Release()

	// Second release should not panic
	lock.Release()
}

func TestAcquireDualLock_ConcurrentGoroutines(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	ctx := context.Background()

	const numGoroutines = 10
	var counter int64
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch multiple goroutines that all try to increment a counter
	// while holding the dual lock. If locking works correctly,
	// the final counter value should be numGoroutines.
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			lock, err := AcquireDualLock(ctx, configDir)
			if err != nil {
				t.Errorf("AcquireDualLock() error = %v", err)
				return
			}
			defer lock.Release()

			// Critical section: read-modify-write
			current := atomic.LoadInt64(&counter)
			time.Sleep(time.Millisecond) // Simulate some work
			atomic.StoreInt64(&counter, current+1)
		}()
	}

	wg.Wait()

	if counter != numGoroutines {
		t.Fatalf("counter = %d, want %d (race condition detected)", counter, numGoroutines)
	}
}
