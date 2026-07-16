//go:build !windows

package process

import (
	"errors"
	"syscall"
	"testing"
)

func TestCrossPlatformCoverageAliveUnixErrorClassification(t *testing.T) {
	previous := killProcess
	t.Cleanup(func() { killProcess = previous })
	for _, tc := range []struct {
		err  error
		want bool
	}{{nil, true}, {syscall.EPERM, true}, {syscall.ESRCH, false}, {errors.New("ambiguous"), true}} {
		killProcess = func(int, syscall.Signal) error { return tc.err }
		if got := Alive(42); got != tc.want {
			t.Errorf("Alive with %v = %v, want %v", tc.err, got, tc.want)
		}
	}
}
