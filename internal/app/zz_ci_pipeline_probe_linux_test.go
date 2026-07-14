//go:build linux

package app

import "testing"

func TestCIPipelineProbeLinuxFailure(t *testing.T) {
	t.Fatal("intentional CI pipeline probe: Linux Test and Coverage must fail")
}
