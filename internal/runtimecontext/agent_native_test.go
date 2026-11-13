//go:build darwin || windows || (linux && !openharmony)

package runtimecontext

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageNativeAgentReleasesEveryResult(t *testing.T) {
	for _, scenario := range []string{"ready", "oversized", "null", "symbol", "free symbol", "configuration", "load", "panic"} {
		t.Run(scenario, func(t *testing.T) {
			restored, freed := 0, 0
			testseam.Swap(t, &configureAgentEnvironment, func(config string) (func(), error) {
				if config != "config/skt.bin" {
					t.Fatal("wrong configuration")
				}
				if scenario == "configuration" {
					return nil, errNativeBinding
				}
				return func() { restored++ }, nil
			})
			testseam.Swap(t, &openNativeLibrary, func(string) (uintptr, error) {
				if scenario == "load" {
					return 0, errors.New("load")
				}
				return 11, nil
			})
			testseam.Swap(t, &lookupNativeSymbol, func(handle uintptr, name string) (uintptr, error) {
				if handle != 11 || (name != agentInfoSymbol && name != freeAgentInfoSymbol) {
					t.Fatal("wrong symbol")
				}
				if scenario == "symbol" || (scenario == "free symbol" && name == freeAgentInfoSymbol) {
					return 0, errors.New("symbol")
				}
				return 12, nil
			})
			buffer := make([]byte, MaxAgentJSONBytes+1)
			copy(buffer, `{"agent":7,"match_tier":"high"}`)
			if scenario == "oversized" {
				for i := range buffer {
					buffer[i] = 'x'
				}
			}
			testseam.Swap(t, &bindAgentFunctions, func(uintptr, uintptr) (func(int32, *int32) unsafe.Pointer, func(unsafe.Pointer)) {
				return func(full int32, code *int32) unsafe.Pointer {
						if full != 0 {
							t.Fatal("full ancestry requested")
						}
						*code = 2002
						if scenario == "null" {
							return nil
						}
						if scenario == "panic" {
							panic("binding")
						}
						return unsafe.Pointer(&buffer[0])
					}, func(pointer unsafe.Pointer) {
						if pointer != unsafe.Pointer(&buffer[0]) {
							t.Fatal("free pointer mismatch")
						}
						freed++
					}
			})
			raw, code, err := collectNativeAgent("library", "config/skt.bin")
			expectedFree := 0
			if scenario == "ready" || scenario == "oversized" {
				expectedFree = 1
			}
			expectedRestore := 1
			if scenario == "configuration" {
				expectedRestore = 0
			}
			if freed != expectedFree || restored != expectedRestore {
				t.Fatalf("free=%d restore=%d", freed, restored)
			}
			if scenario == "ready" && (len(raw) == 0 || code != 2002 || err != nil) {
				t.Fatal("valid object lost")
			}
			if scenario == "oversized" && !errors.Is(err, errAgentTooLarge) {
				t.Fatal("oversize not classified")
			}
			if scenario == "configuration" && !errors.Is(err, errNativeBinding) {
				t.Fatal("configuration failure ignored")
			}
			if scenario == "free symbol" && !errors.Is(err, errNativeSymbol) {
				t.Fatal("missing release symbol ignored")
			}
			if scenario == "null" && (raw != nil || code != 2002) {
				t.Fatal("NULL result lost error code")
			}
		})
	}
}

func TestCrossPlatformCoverageNativeEmptyResult(t *testing.T) {
	raw, err := copyAgentCString(nil)
	if raw != nil || err != nil {
		t.Fatalf("nil result = %v, %v", raw, err)
	}
}

func TestCrossPlatformCoverageNativeAgentBinding(t *testing.T) {
	buffer := []byte("runtime-result\x00")
	freed := 0
	getSymbol, freeSymbol := agentCallbacksForTest(func(full int32, code *int32) uintptr {
		if full != 0 {
			t.Errorf("full = %d", full)
		}
		*code = 2002
		return uintptr(unsafe.Pointer(&buffer[0]))
	}, func(pointer unsafe.Pointer) uintptr {
		if pointer != unsafe.Pointer(&buffer[0]) {
			t.Error("release pointer changed")
		}
		freed++
		return 0
	})
	get, free := bindNativeAgent(getSymbol, freeSymbol)
	var code int32
	pointer := get(0, &code)
	raw, err := copyAgentCString(pointer)
	free(pointer)
	if err != nil || string(raw) != "runtime-result" || code != 2002 || freed != 1 {
		t.Fatalf("native round trip: result=%q code=%d releases=%d error=%v", raw, code, freed, err)
	}
	runtime.KeepAlive(buffer)
}
