//go:build !darwin && !linux

package maintenance

import "errors"

var ErrInUse = errors.New("idle maintenance locking is unsupported on this platform")

type UsageLock struct{}

func AcquireUsageLock(string) (*UsageLock, error) { return &UsageLock{}, nil }
func acquireIdleLock(string) (*UsageLock, error)  { return nil, ErrInUse }
func (*UsageLock) Close() error                   { return nil }
