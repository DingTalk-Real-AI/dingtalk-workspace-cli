package auth

import "testing"

func TestValidateBrowserURLAllowsHTTPS(t *testing.T) {
	if err := validateBrowserURL("https://example.com/verify?code=123"); err != nil {
		t.Fatalf("validateBrowserURL() error = %v, want nil", err)
	}
}

func TestValidateBrowserURLRejectsDisallowedSchemes(t *testing.T) {
	err := validateBrowserURL("file:///tmp/secret.txt")
	if err == nil {
		t.Fatal("validateBrowserURL() error = nil, want disallowed scheme error")
	}
}
