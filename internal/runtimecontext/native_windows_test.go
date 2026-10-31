//go:build windows

package runtimecontext

import (
	"github.com/ebitengine/purego"
	"os"
	"testing"
	"unsafe"
)

func nativeInitializerForTest(call func(int32, uintptr) int32) nativeInitializer {
	return func(environment int32, callback uintptr) int32 {
		return call(environment, callback)
	}
}

func invokeNativeCallbackForTest(callback any, pointer unsafe.Pointer, code int32) uintptr {
	return callback.(nativeCallbackFunc)(pointer, code)
}

func agentCallbacksForTest(get func(int32, *int32) uintptr, free func(unsafe.Pointer) uintptr) (uintptr, uintptr) {
	return purego.NewCallback(get), purego.NewCallback(free)
}

func TestCrossPlatformCoverageNativeConfigEnvironment(t *testing.T) {
	const key = "SKILLTRACE_CONFIG"
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[exists], func(t *testing.T) {
			t.Setenv(key, "previous")
			if !exists {
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			restore, err := setNativeAgentConfig("runtime-test-config")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(restore)
			if got := os.Getenv(key); got != "runtime-test-config" {
				t.Fatalf("configured value = %q", got)
			}
			restore()
			got, present := os.LookupEnv(key)
			if present != exists || (exists && got != "previous") {
				t.Fatalf("restored value = %q, present=%v", got, present)
			}
		})
	}
	t.Setenv(key, "unchanged")
	if restore, err := setNativeAgentConfig("invalid\x00path"); err == nil || restore != nil {
		t.Fatal("invalid native environment accepted")
	}
	if os.Getenv(key) != "unchanged" {
		t.Fatal("failed configuration changed environment")
	}
}
