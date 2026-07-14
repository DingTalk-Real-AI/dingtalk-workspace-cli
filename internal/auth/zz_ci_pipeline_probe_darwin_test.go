//go:build darwin

package auth

import "testing"

func TestCIPipelineProbeDarwinFailure(t *testing.T) {
	t.Fatal("intentional CI pipeline probe: macOS gate must fail")
}
