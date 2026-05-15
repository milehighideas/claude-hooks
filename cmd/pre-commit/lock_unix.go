//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// acquireLock tries to get an exclusive file lock keyed to the current repo.
// Returns the lock file on success, or an error if another instance holds the lock.
func acquireLock() (*os.File, error) {
	lockPath := getLockPath()

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Non-blocking exclusive lock
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("lock already held: %w", err)
	}

	return f, nil
}

// releaseLock releases and removes the lock file.
func releaseLock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
}

// acquireGlobalLockBlocking grabs a system-wide pre-commit lock at
// /tmp/pre-commit-global.lock, waiting for any previous holder to release.
// Used to serialize pre-commit runs across multiple repos on the same machine
// so heavy test/typecheck loads don't starve each other on CPU. The current
// holder writes "<repoPath> pid=<n>" into the lock file so waiters can see
// who they're queued behind.
func acquireGlobalLockBlocking(repoID string) (*os.File, error) {
	lockPath := filepath.Join(os.TempDir(), "pre-commit-global.lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open global lock file: %w", err)
	}

	// Try non-blocking first to skip the wait message in the common no-contention case.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		held, _ := os.ReadFile(lockPath)
		holder := strings.TrimSpace(string(held))
		if holder == "" {
			holder = "another pre-commit run"
		}
		fmt.Fprintf(os.Stderr, "⏳ Waiting on global pre-commit lock (held by %s)...\n", holder)
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("global lock blocking wait failed: %w", err)
		}
	}

	// We hold the lock — record our identity for the next waiter.
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	_, _ = fmt.Fprintf(f, "%s pid=%d", repoID, os.Getpid())

	return f, nil
}
