//go:build darwin || (linux && !openharmony)

package runtimecontext

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/ebitengine/purego"
)

func TestCrossPlatformCoverageNativeConfigEnvironment(t *testing.T) {
	const key = "SKILLTRACE_CONFIG"
	var getenv func(string) unsafe.Pointer
	var setenv func(string, string, int32) int32
	var unsetenv func(string) int32
	purego.RegisterLibFunc(&getenv, purego.RTLD_DEFAULT, "getenv")
	purego.RegisterLibFunc(&setenv, purego.RTLD_DEFAULT, "setenv")
	purego.RegisterLibFunc(&unsetenv, purego.RTLD_DEFAULT, "unsetenv")
	read := func() (string, bool) {
		pointer := getenv(key)
		if pointer == nil {
			return "", false
		}
		for n := 0; n <= 1<<20; n++ {
			if *(*byte)(unsafe.Add(pointer, n)) == 0 {
				return string(unsafe.Slice((*byte)(pointer), n)), true
			}
		}
		t.Fatal("unbounded native environment")
		return "", false
	}
	original, existed := read()
	t.Cleanup(func() {
		if existed {
			setenv(key, original, 1)
		} else {
			unsetenv(key)
		}
	})
	for _, tc := range []struct {
		name, value string
		exists      bool
	}{
		{"absent", "", false}, {"empty", "", true}, {"present", "previous/config", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.exists {
				if setenv(key, tc.value, 1) != 0 {
					t.Fatal("setenv failed")
				}
			} else if unsetenv(key) != 0 {
				t.Fatal("unsetenv failed")
			}
			restore, err := setNativeAgentConfig("runtime-test-config")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(restore)
			if got, present := read(); !present || got != "runtime-test-config" {
				t.Fatalf("configured value = %q, present=%v", got, present)
			}
			restore()
			if got, present := read(); present != tc.exists || got != tc.value {
				t.Fatalf("restored value = %q, present=%v", got, present)
			}
		})
	}
	// 检查读取有界且失败时不覆盖原值。
	oversized := strings.Repeat("x", (1<<20)+1)
	if setenv(key, oversized, 1) != 0 {
		t.Fatal("setenv failed")
	}
	if restore, err := setNativeAgentConfig("replacement"); !errors.Is(err, errNativeBinding) || restore != nil {
		t.Fatal("oversized previous value accepted")
	}
	if p := getenv(key); p == nil || string(unsafe.Slice((*byte)(p), len(oversized))) != oversized {
		t.Fatal("failed configuration replaced original value")
	}
}

func TestCrossPlatformCoverageNativeConfigFailureBoundaries(t *testing.T) {
	for _, missing := range []string{"getenv", "setenv", "unsetenv"} {
		t.Run(missing, func(t *testing.T) {
			lookup := lookupNativeSymbol
			testseam.Swap(t, &lookupNativeSymbol, func(handle uintptr, name string) (uintptr, error) {
				if name == missing {
					return 0, errors.New("missing native symbol")
				}
				return lookup(handle, name)
			})
			if restore, err := setNativeAgentConfig("config"); !errors.Is(err, errNativeSymbol) || restore != nil {
				t.Fatal("missing native symbol accepted")
			}
		})
	}
	t.Run("setenv failure", func(t *testing.T) {
		lookup := lookupNativeSymbol
		failSet := purego.NewCallback(func(_ purego.CDecl, key, value unsafe.Pointer, overwrite int32) uintptr { return 1 })
		testseam.Swap(t, &lookupNativeSymbol, func(handle uintptr, name string) (uintptr, error) {
			if name == "setenv" {
				return failSet, nil
			}
			return lookup(handle, name)
		})
		if restore, err := setNativeAgentConfig("config"); !errors.Is(err, errNativeBinding) || restore != nil {
			t.Fatal("setenv failure ignored")
		}
		runtime.KeepAlive(failSet)
	})
}
