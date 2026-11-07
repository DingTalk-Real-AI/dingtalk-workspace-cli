package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows enforces file access through ACLs, not POSIX mode bits")
	}
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
		if err != context.Canceled {
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

func TestCrossPlatformCoverageAcquireDualLockFileContentionContext(t *testing.T) {
	for _, explicitCancel := range []bool{false, true} {
		name := "deadline"
		if explicitCancel {
			name = "cancel"
		}
		t.Run(name, func(t *testing.T) {
			configDir := t.TempDir()
			// 独立 fd 持有真实系统文件锁，绕开进程内锁以覆盖跨进程等待路径。
			held, err := acquireTokenLock(configDir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(held.release)

			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			wantErr := error(context.DeadlineExceeded)
			if explicitCancel {
				cancel()
				ctx, cancel = context.WithCancel(context.Background())
				timer := time.AfterFunc(150*time.Millisecond, cancel)
				defer timer.Stop()
				wantErr = context.Canceled
			}
			defer cancel()
			started := time.Now()
			got, err := AcquireDualLock(ctx, configDir)
			elapsed := time.Since(started)
			if got != nil {
				got.Release()
				t.Fatal("competing file lock unexpectedly acquired")
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
			if elapsed > time.Second {
				t.Fatalf("context budget exceeded: %v", elapsed)
			}
			t.Logf("real file lock wait returned after %v: %v", elapsed, err)
			if _, exists := processLocks.Load(processLockKey(configDir)); exists {
				t.Fatal("process lock leaked after file lock cancellation")
			}

			probe, err := os.OpenFile(filepath.Join(configDir, lockFileName), os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer probe.Close()
			if err := lockFile(probe); err == nil {
				unlockFile(probe)
				t.Fatal("cancellation released another holder's lock")
			}
			held.release()
			retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
			defer retryCancel()
			retry, err := AcquireDualLock(retryCtx, configDir)
			if err != nil {
				t.Fatalf("reacquire after cancellation: %v", err)
			}
			retry.Release()
		})
	}
}

func TestCrossPlatformCoverageAcquireDualLockNilContext(t *testing.T) {
	configDir := t.TempDir()
	for i := 0; i < 2; i++ {
		lock, err := AcquireDualLock(nil, configDir)
		if err != nil {
			t.Fatalf("nil context acquisition %d: %v", i, err)
		}
		lock.Release()
	}
	if _, exists := processLocks.Load(processLockKey(configDir)); exists {
		t.Fatal("process lock leaked with nil context")
	}
}

func TestCrossPlatformCoverageAcquireDualLockAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	configDir := t.TempDir()
	testseam.Swap(t, &fileLockMkdirAll, func(string, os.FileMode) error {
		t.Fatal("canceled lock acquisition must not create a directory")
		return nil
	})
	if lock, err := AcquireDualLock(ctx, configDir); lock != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquisition = %v, %v", lock, err)
	}
	if _, exists := processLocks.Load(processLockKey(configDir)); exists {
		t.Fatal("process lock leaked for canceled context")
	}
}

func TestCrossPlatformCoverageAcquireDualLockCanceledDuringFileAcquire(t *testing.T) {
	configDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var attemptedFile *os.File
	testseam.Swap(t, &fileLockTry, func(f *os.File) error {
		attemptedFile = f
		err := lockFile(f)
		cancel()
		return err
	})
	if lock, err := AcquireDualLock(ctx, configDir); lock != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled during acquisition = %v, %v", lock, err)
	}
	if attemptedFile == nil {
		t.Fatal("file lock attempt was not exercised")
	}
	// Windows 对已关闭句柄的 Stat 返回系统错误；重复 Close 在各平台统一返回 ErrClosed。
	if err := attemptedFile.Close(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("file descriptor not closed after cancellation: %v", err)
	}
	if _, exists := processLocks.Load(processLockKey(configDir)); exists {
		t.Fatal("process lock leaked after successful-but-canceled file acquisition")
	}
	probe, err := os.OpenFile(filepath.Join(configDir, lockFileName), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if err := lockFile(probe); err != nil {
		t.Fatalf("file lock leaked after cancellation: %v", err)
	}
	unlockFile(probe)
}

func TestCrossPlatformCoverageAcquireTokenLockCancellationAtBoundaries(t *testing.T) {
	for _, phase := range []string{"opened", "contended"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var opened *os.File
			openFile := fileLockOpenFile
			testseam.Swap(t, &fileLockOpenFile, func(path string, flag int, mode os.FileMode) (*os.File, error) {
				f, err := openFile(path, flag, mode)
				opened = f
				if phase == "opened" {
					cancel()
				}
				return f, err
			})
			if phase == "contended" {
				testseam.Swap(t, &fileLockTry, func(*os.File) error { cancel(); return errors.New("fixture contention") })
			}
			lock, err := acquireTokenLockContext(ctx, t.TempDir())
			if lock != nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("取消后仍取得锁: %v %v", lock, err)
			}
			if opened == nil || !errors.Is(opened.Close(), os.ErrClosed) {
				t.Fatal("取消后未关闭文件")
			}
		})
	}
}
