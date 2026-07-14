//go:build windows

package auth

import "testing"

func TestCIPipelineProbeWindowsFailure(t *testing.T) {
	t.Fatal("intentional CI pipeline probe: Windows gate must fail")
}
