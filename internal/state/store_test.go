package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	t.Run("active lock blocks", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		lock, err := AcquireLock(paths)
		if err != nil {
			t.Fatalf("first lock: %v", err)
		}
		defer func() { _ = lock.Release() }()

		_, err = AcquireLock(paths)
		if err == nil {
			t.Fatal("expected second lock acquire to fail")
		}
		if !strings.Contains(err.Error(), "another bb process holds the lock") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stale lock with dead pid is recovered", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		if err := EnsureDir(paths.LocalStateRoot()); err != nil {
			t.Fatalf("ensure local state root: %v", err)
		}
		hostname, err := os.Hostname()
		if err != nil {
			t.Fatalf("hostname: %v", err)
		}
		lockBody := fmt.Sprintf(
			"pid=%d\nhostname=%s\ncreated_at=%s\n",
			99999999,
			hostname,
			time.Now().UTC().Format(time.RFC3339),
		)
		if err := os.WriteFile(paths.LockPath(), []byte(lockBody), 0o644); err != nil {
			t.Fatalf("write stale lock: %v", err)
		}

		lock, err := AcquireLock(paths)
		if err != nil {
			t.Fatalf("expected stale lock recovery, got: %v", err)
		}
		_ = lock.Release()
	})

	t.Run("old corrupt lock is recovered", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		if err := EnsureDir(paths.LocalStateRoot()); err != nil {
			t.Fatalf("ensure local state root: %v", err)
		}
		lockPath := paths.LockPath()
		if err := os.WriteFile(lockPath, []byte("held\n"), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
		old := time.Now().Add(-25 * time.Hour)
		if err := os.Chtimes(lockPath, old, old); err != nil {
			t.Fatalf("chtimes lock: %v", err)
		}

		lock, err := AcquireLock(paths)
		if err != nil {
			t.Fatalf("expected stale corrupt lock recovery, got: %v", err)
		}
		_ = lock.Release()
	})

	t.Run("recent corrupt lock still blocks", func(t *testing.T) {
		paths := NewPaths(t.TempDir())
		if err := EnsureDir(paths.LocalStateRoot()); err != nil {
			t.Fatalf("ensure local state root: %v", err)
		}
		lockPath := paths.LockPath()
		if err := os.WriteFile(lockPath, []byte("held\n"), 0o644); err != nil {
			t.Fatalf("write lock: %v", err)
		}
		now := time.Now()
		if err := os.Chtimes(lockPath, now, now); err != nil {
			t.Fatalf("chtimes lock: %v", err)
		}

		_, err := AcquireLock(paths)
		if err == nil {
			t.Fatal("expected recent corrupt lock to block")
		}
		if !strings.Contains(err.Error(), "another bb process holds the lock") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLockFilePayload(t *testing.T) {
	paths := NewPaths(t.TempDir())
	lock, err := AcquireLock(paths)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	raw, err := os.ReadFile(filepath.Clean(paths.LockPath()))
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "pid=") {
		t.Fatalf("expected pid in lock file, got: %q", text)
	}
	if !strings.Contains(text, "hostname=") {
		t.Fatalf("expected hostname in lock file, got: %q", text)
	}
	if !strings.Contains(text, "created_at=") {
		t.Fatalf("expected created_at in lock file, got: %q", text)
	}
}
