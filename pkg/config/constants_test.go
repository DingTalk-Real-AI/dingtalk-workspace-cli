package config

import (
	"testing"
	"time"
)

func TestDirPerm(t *testing.T) {
	t.Parallel()
	if DirPerm != 0o700 {
		t.Fatalf("DirPerm = %o, want 700", DirPerm)
	}
}

func TestFilePerm(t *testing.T) {
	t.Parallel()
	if FilePerm != 0o600 {
		t.Fatalf("FilePerm = %o, want 600", FilePerm)
	}
}

func TestHTTPTimeout(t *testing.T) {
	t.Parallel()
	if HTTPTimeout <= 0 {
		t.Fatalf("HTTPTimeout = %v, want positive", HTTPTimeout)
	}
	if HTTPTimeout != 30*time.Second {
		t.Fatalf("HTTPTimeout = %v, want 30s", HTTPTimeout)
	}
}

func TestOAuthTimeout(t *testing.T) {
	t.Parallel()
	if OAuthTimeout <= 0 {
		t.Fatalf("OAuthTimeout = %v, want positive", OAuthTimeout)
	}
	if OAuthTimeout != 15*time.Second {
		t.Fatalf("OAuthTimeout = %v, want 15s", OAuthTimeout)
	}
}

func TestDiscoveryTimeout(t *testing.T) {
	t.Parallel()
	if DiscoveryTimeout <= 0 {
		t.Fatalf("DiscoveryTimeout = %v, want positive", DiscoveryTimeout)
	}
	if DiscoveryTimeout != 10*time.Second {
		t.Fatalf("DiscoveryTimeout = %v, want 10s", DiscoveryTimeout)
	}
}

func TestLockTimeout(t *testing.T) {
	t.Parallel()
	if LockTimeout <= 0 {
		t.Fatalf("LockTimeout = %v, want positive", LockTimeout)
	}
	if LockTimeout != 10*time.Second {
		t.Fatalf("LockTimeout = %v, want 10s", LockTimeout)
	}
}

func TestMaxResponseBodySize(t *testing.T) {
	t.Parallel()
	want := 10 * 1024 * 1024
	if MaxResponseBodySize != want {
		t.Fatalf("MaxResponseBodySize = %d, want %d (10MB)", MaxResponseBodySize, want)
	}
}

func TestDefaultPartition(t *testing.T) {
	t.Parallel()
	if DefaultPartition != "default/default" {
		t.Fatalf("DefaultPartition = %q, want %q", DefaultPartition, "default/default")
	}
}

func TestManualTokenExpiry(t *testing.T) {
	t.Parallel()
	if ManualTokenExpiry <= 0 {
		t.Fatalf("ManualTokenExpiry = %v, want positive", ManualTokenExpiry)
	}
	if ManualTokenExpiry != 24*time.Hour {
		t.Fatalf("ManualTokenExpiry = %v, want 24h", ManualTokenExpiry)
	}
}

func TestDeviceFlowTimeout(t *testing.T) {
	t.Parallel()
	if DeviceFlowTimeout <= 0 {
		t.Fatalf("DeviceFlowTimeout = %v, want positive", DeviceFlowTimeout)
	}
	if DeviceFlowTimeout != 16*time.Minute {
		t.Fatalf("DeviceFlowTimeout = %v, want 16m", DeviceFlowTimeout)
	}
}

func TestOAuthFlowTimeout(t *testing.T) {
	t.Parallel()
	if OAuthFlowTimeout <= 0 {
		t.Fatalf("OAuthFlowTimeout = %v, want positive", OAuthFlowTimeout)
	}
	if OAuthFlowTimeout != 6*time.Minute {
		t.Fatalf("OAuthFlowTimeout = %v, want 6m", OAuthFlowTimeout)
	}
}

func TestDefaultAccessTokenExpiry(t *testing.T) {
	t.Parallel()
	if DefaultAccessTokenExpiry != 7200 {
		t.Fatalf("DefaultAccessTokenExpiry = %d, want 7200", DefaultAccessTokenExpiry)
	}
}

func TestDefaultRefreshTokenLifetime(t *testing.T) {
	t.Parallel()
	if DefaultRefreshTokenLifetime <= 0 {
		t.Fatalf("DefaultRefreshTokenLifetime = %v, want positive", DefaultRefreshTokenLifetime)
	}
	if DefaultRefreshTokenLifetime != 30*24*time.Hour {
		t.Fatalf("DefaultRefreshTokenLifetime = %v, want 720h", DefaultRefreshTokenLifetime)
	}
}

func TestDefaultFetchServersLimit(t *testing.T) {
	t.Parallel()
	if DefaultFetchServersLimit != 200 {
		t.Fatalf("DefaultFetchServersLimit = %d, want 200", DefaultFetchServersLimit)
	}
}

func TestMaxUploadFileSize(t *testing.T) {
	t.Parallel()
	var want int64 = 100 * 1024 * 1024
	if MaxUploadFileSize != want {
		t.Fatalf("MaxUploadFileSize = %d, want %d (100MB)", MaxUploadFileSize, want)
	}
}
