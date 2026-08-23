//go:build !windows

package process

import (
	"errors"
	"syscall"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAliveUnixErrorClassification(t *testing.T) {
	testseam.Protect(t, &killProcess)
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
