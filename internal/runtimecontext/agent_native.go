//go:build darwin || windows || (linux && !openharmony)

package runtimecontext

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

const (
	agentInfoSymbol     = "r7Qk3Wzp"
	freeAgentInfoSymbol = "m2Xt9Bvn"
)

var (
	nativeEnvironmentMu       sync.Mutex
	bindAgentFunctions        = bindNativeAgent
	configureAgentEnvironment = setNativeAgentConfig
)

func collectNativeAgent(path, config string) (raw []byte, code int32, err error) {
	defer func() {
		if recover() != nil {
			err = errNativeBinding
		}
	}()
	nativeEnvironmentMu.Lock()
	defer nativeEnvironmentMu.Unlock()
	restore, err := configureAgentEnvironment(config)
	if err != nil {
		return nil, 0, err
	}
	defer restore()
	handle, err := openNativeLibrary(path)
	if err != nil {
		return nil, 0, fmt.Errorf("%w", errNativeLoad)
	}
	getSymbol, err := lookupNativeSymbol(handle, agentInfoSymbol)
	if err != nil {
		return nil, 0, fmt.Errorf("%w", errNativeSymbol)
	}
	freeSymbol, err := lookupNativeSymbol(handle, freeAgentInfoSymbol)
	if err != nil {
		return nil, 0, fmt.Errorf("%w", errNativeSymbol)
	}
	get, free := bindAgentFunctions(getSymbol, freeSymbol)
	pointer := get(0, &code)
	runtime.KeepAlive(&code)
	if pointer == nil {
		return nil, code, nil
	}
	// Installed before copying/parsing: every non-NULL allocation is freed once,
	// including malformed/oversized results and results arriving after timeout.
	defer free(pointer)
	raw, err = copyAgentCString(pointer)
	return raw, code, err
}

func copyAgentCString(pointer unsafe.Pointer) ([]byte, error) {
	if pointer == nil {
		return nil, nil
	}
	for i := 0; i <= MaxAgentJSONBytes; i++ {
		if *(*byte)(unsafe.Add(pointer, i)) == 0 {
			raw := make([]byte, i)
			copy(raw, unsafe.Slice((*byte)(pointer), i))
			return raw, nil
		}
	}
	return nil, errAgentTooLarge
}
