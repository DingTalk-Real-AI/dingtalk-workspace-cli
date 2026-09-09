package runtimecontext

import "testing"

func TestCrossPlatformCoverageHostAndLoginRedirectGuards(t *testing.T) {
	if hostAllowed("", []string{"example.com"}) {
		t.Fatal("empty candidate host must be rejected")
	}
	if loginRedirectAllowed(":", nil) {
		t.Fatal("colon-only redirect must be rejected")
	}
	if loginRedirectAllowed("https://", nil) {
		t.Fatal("https URL with empty host must be rejected")
	}
}
