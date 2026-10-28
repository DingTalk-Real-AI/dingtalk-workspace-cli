//go:build darwin || windows || (linux && !openharmony)

package runtimecontext

import (
	"errors"
	"testing"
	"unsafe"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageNativeAgentReleasesEveryResult(t *testing.T) {
	for _, scenario := range []string{"ready", "oversized", "null", "symbol", "load", "panic"} {
		t.Run(scenario, func(t *testing.T) {
			restored, freed := 0, 0
			testseam.Swap(t, &configureAgentEnvironment, func(config string) (func(), error) {
				if config != "config/skt.bin" {
					t.Fatal("wrong configuration")
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
				if scenario == "symbol" {
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
			if freed != expectedFree || restored != 1 {
				t.Fatalf("free=%d restore=%d", freed, restored)
			}
			if scenario == "ready" && (len(raw) == 0 || code != 2002 || err != nil) {
				t.Fatal("valid object lost")
			}
			if scenario == "oversized" && !errors.Is(err, errAgentTooLarge) {
				t.Fatal("oversize not classified")
			}
			if scenario == "null" && (raw != nil || code != 2002) {
				t.Fatal("NULL result lost error code")
			}
		})
	}
}
