//go:build !((darwin || linux) && (amd64 || arm64))

package schemacache

import (
	"errors"
	"testing"
)

func TestCrossPlatformCoverageDisabledPlatformDoesNoCacheIO(t *testing.T) {
	counters := &Counters{}
	cache, err := Open("official", WithCounters(counters))
	if cache != nil || !errors.Is(err, ErrDisabled) {
		t.Fatalf("Open = (%v, %v), want disabled", cache, err)
	}
	if got := counters.Snapshot(); got != (IOSnapshot{}) {
		t.Fatalf("disabled platform performed I/O: %+v", got)
	}
}
