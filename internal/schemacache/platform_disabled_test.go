//go:build !((darwin || linux || windows) && (amd64 || arm64))

package schemacache

import (
	"errors"
	"testing"
)

func TestCrossPlatformCoverageDisabledPlatformDoesNoCacheIO(t *testing.T) {
	counters := &Counters{}
	cache, err := Open("official", nil, WithCounters(counters), WithNoCreate())
	if cache != nil || !errors.Is(err, ErrDisabled) {
		t.Fatalf("Open = (%v, %v), want disabled", cache, err)
	}
	if got := counters.Snapshot(); got != (IOSnapshot{}) {
		t.Fatalf("disabled platform performed I/O: %+v", got)
	}
}
