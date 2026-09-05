//go:build !((darwin && arm64) || (linux && amd64))

package schemacache

import (
	"errors"
	"testing"
)

func TestDisabledPlatformDoesNoCacheIO(t *testing.T) {
	counters := &Counters{}
	cache, err := Open("official", WithCounters(counters))
	if cache != nil || !errors.Is(err, ErrDisabled) {
		t.Fatalf("Open = (%v, %v), want disabled", cache, err)
	}
	if got := counters.Snapshot(); got != (IOSnapshot{}) {
		t.Fatalf("disabled platform performed I/O: %+v", got)
	}
}
