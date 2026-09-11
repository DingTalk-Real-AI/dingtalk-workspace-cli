package aem

import (
	"slices"
	"sort"
	"testing"
)

func TestBuildSendConfigPublishesCLIIdentityAndVersion(t *testing.T) {
	got := buildSendConfig(Config{
		"pid":      "pid-1",
		"app_name": "dws",
		"version":  "v1.2.3",
		"uid":      "user-1",
	})

	for key, want := range map[string]string{
		"version":     "v1.2.3",
		"app_version": "v1.2.3",
		"uid":         "user-1",
	} {
		if got[key] != want {
			t.Fatalf("send config %s = %q, want %q", key, got[key], want)
		}
	}
}

func TestBuildSendConfigCanDisableAutomaticDimensions(t *testing.T) {
	got := buildSendConfig(Config{
		"pid":                       "pid-1",
		"app_name":                  "dws",
		"env":                       "prod",
		"version":                   "v1.2.3",
		"platform":                  "cli",
		"uid":                       "user-1",
		"username":                  "Alice",
		configDisableAutoDimensions: true,
	})

	gotKeys := make([]string, 0, len(got))
	for key := range got {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{"app_name", "app_version", "env", "pid", "platform", "uid", "username", "version"}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Fatalf("privacy send config keys = %v, want %v", gotKeys, wantKeys)
	}
	for _, key := range []string{"device_id", "ext", "os", "os_version", "pv_id", "sdk_version", "sid", "timezone_offset"} {
		if _, ok := got[key]; ok {
			t.Fatalf("privacy send config contains automatic dimension %q: %#v", key, got)
		}
	}
}
