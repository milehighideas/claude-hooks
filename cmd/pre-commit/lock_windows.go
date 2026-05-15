//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx    = kernel32.NewProc("LockFileEx")
	unlockFileEx  = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock = 0x00000002
	lockfileFailImmediately = 0x00000001
)

// acquireLock tries to get an exclusive file lock keyed to the current repo.
// Returns the lock file on success, or an error if another instance holds the lock.
func acquireLock() (*os.File, error) {
	lockPath := getLockPath()

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Non-blocking exclusive lock via LockFileEx
	handle := syscall.Handle(f.Fd())
	ol := new(syscall.Overlapped)
	r1, _, err := lockFileEx.Call(
		uintptr(handle),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1, 0,
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
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
	handle := syscall.Handle(f.Fd())
	ol := new(syscall.Overlapped)
	_, _, _ = unlockFileEx.Call(
		uintptr(handle),
		0,
		1, 0,
		uintptr(unsafe.Pointer(ol)),
	)
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
}

// acquireGlobalLockBlocking grabs a system-wide pre-commit lock at
// %TEMP%\pre-commit-global.lock, waiting for any previous holder to release.
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

	handle := syscall.Handle(f.Fd())

	// Try non-blocking first to skip the wait message in the common no-contention case.
	ol := new(syscall.Overlapped)
	r1, _, _ := lockFileEx.Call(
		uintptr(handle),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1, 0,
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		held, _ := os.ReadFile(lockPath)
		holder := strings.TrimSpace(string(held))
		if holder == "" {
			holder = "another pre-commit run"
		}
		fmt.Fprintf(os.Stderr, "⏳ Waiting on global pre-commit lock (held by %s)...\n", holder)
		// Blocking wait — no FailImmediately flag.
		ol2 := new(syscall.Overlapped)
		r2, _, errno := lockFileEx.Call(
			uintptr(handle),
			uintptr(lockfileExclusiveLock),
			0,
			1, 0,
			uintptr(unsafe.Pointer(ol2)),
		)
		if r2 == 0 {
			_ = f.Close()
			return nil, fmt.Errorf("global lock blocking wait failed: %w", errno)
		}
	}

	// We hold the lock — record our identity for the next waiter.
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	fmt.Fprintf(f, "%s pid=%d", repoID, os.Getpid())

	return f, nil
}
