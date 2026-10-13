//go:build darwin || linux

package maintenance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var ErrInUse = errors.New("cmdlib is open; background maintenance skipped")

type UsageLock struct{ f *os.File }

func acquire(path string, exclusive bool) (*UsageLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".in-use.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	mode := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		mode = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), mode); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrInUse
		}
		return nil, err
	}
	return &UsageLock{f: f}, nil
}

func AcquireUsageLock(path string) (*UsageLock, error) {
	lock, err := acquire(path, false)
	if !errors.Is(err, ErrInUse) {
		return lock, err
	}
	marker := path + ".foreground-request"
	token := []byte(fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()))
	if err := os.WriteFile(marker, token, 0600); err != nil {
		return nil, err
	}
	defer func() {
		if b, e := os.ReadFile(marker); e == nil && string(b) == string(token) {
			_ = os.Remove(marker)
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lock, err = acquire(path, false)
		if !errors.Is(err, ErrInUse) {
			return lock, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("Hintergrundprüfung beendet sich noch; cmdlib bitte gleich erneut öffnen")
}
func acquireIdleLock(path string) (*UsageLock, error) { return acquire(path, true) }

func (l *UsageLock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	// Never unlink the lock inode: every process must coordinate on the same file.
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
