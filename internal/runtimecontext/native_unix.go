//go:build darwin || (linux && !openharmony)

package runtimecontext

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

const initSymbol = "k9Xm2pQv"

type nativeInitializer func(purego.CDecl, int32, uintptr) int32
type nativeCallbackFunc func(purego.CDecl, unsafe.Pointer, int32) uintptr

var (
	openNativeLibrary = func(path string) (uintptr, error) {
		return sharedNativeLibrary(path, func(path string) (uintptr, error) {
			return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
		})
	}
	lookupNativeSymbol   = purego.Dlsym
	makeNativeCallback   = purego.NewCallback
	bindNativeInitialize = func(symbol uintptr) nativeInitializer {
		var initialize nativeInitializer
		purego.RegisterFunc(&initialize, symbol)
		return initialize
	}
)

func startNative(path string, callback func([]byte, int32, error)) (session nativeSession, accepted int32, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", errNativeBinding, recovered)
		}
	}()
	handle, err := openNativeLibrary(path)
	if err != nil {
		return nativeSession{}, 0, fmt.Errorf("%w: %v", errNativeLoad, err)
	}
	session.handle = handle
	symbol, err := lookupNativeSymbol(handle, initSymbol)
	if err != nil {
		return session, 0, fmt.Errorf("%w: %v", errNativeSymbol, err)
	}
	callbackPointer := makeNativeCallback(nativeCallbackFunc(func(_ purego.CDecl, token unsafe.Pointer, code int32) uintptr {
		raw, copyErr := copyCString(token)
		callback(raw, code, copyErr)
		return 0
	}))
	session.callback = callbackPointer
	initialize := bindNativeInitialize(symbol)
	accepted = initialize(purego.CDecl{}, initializationEnvironment, callbackPointer)
	return session, accepted, nil
}

func bindNativeAgent(getSymbol, freeSymbol uintptr) (func(int32, *int32) unsafe.Pointer, func(unsafe.Pointer)) {
	var get func(purego.CDecl, int32, *int32) unsafe.Pointer
	var free func(purego.CDecl, unsafe.Pointer)
	purego.RegisterFunc(&get, getSymbol)
	purego.RegisterFunc(&free, freeSymbol)
	return func(full int32, code *int32) unsafe.Pointer { return get(purego.CDecl{}, full, code) },
		func(pointer unsafe.Pointer) { free(purego.CDecl{}, pointer) }
}

// Go's environment table does not update libc getenv in a CGO-disabled binary.
// Save and restore libc's own value around the synchronous collection call.
func setNativeAgentConfig(path string) (func(), error) {
	var getenv func(purego.CDecl, string) unsafe.Pointer
	var setenv func(purego.CDecl, string, string, int32) int32
	var unsetenv func(purego.CDecl, string) int32
	for _, binding := range []struct {
		name string
		fn   any
	}{
		{"getenv", &getenv}, {"setenv", &setenv}, {"unsetenv", &unsetenv},
	} {
		symbol, err := purego.Dlsym(purego.RTLD_DEFAULT, binding.name)
		if err != nil {
			return nil, errNativeSymbol
		}
		purego.RegisterFunc(binding.fn, symbol)
	}
	const key = "SKILLTRACE_CONFIG"
	previous := getenv(purego.CDecl{}, key)
	var old string
	if previous != nil {
		// Environment values are NUL terminated. Copy before setenv invalidates it.
		for i := 0; ; i++ {
			c := *(*byte)(unsafe.Add(previous, i))
			if c == 0 {
				old = string(unsafe.Slice((*byte)(previous), i))
				break
			}
			if i >= 1<<20 {
				return nil, errNativeBinding
			}
		}
	}
	if setenv(purego.CDecl{}, key, path, 1) != 0 {
		return nil, errNativeBinding
	}
	return func() {
		if previous == nil {
			unsetenv(purego.CDecl{}, key)
		} else {
			setenv(purego.CDecl{}, key, old, 1)
		}
	}, nil
}
