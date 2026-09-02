// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import (
	"errors"
	"unsafe"
)

func copyCString(pointer unsafe.Pointer) ([]byte, error) {
	if pointer == nil {
		return nil, errors.New("nil runtime value")
	}
	// The complete JSON Header is limited to maxHeaderBytes, so a usable token
	// must terminate before this window ends. Do not probe one extra byte: the
	// native callback does not provide an allocation length that would make an
	// maxHeaderBytes+1 read safe.
	window := unsafe.Slice((*byte)(pointer), maxHeaderBytes)
	for index, value := range window {
		if value == 0 {
			result := make([]byte, index)
			copy(result, window[:index])
			return result, nil
		}
	}
	return nil, errors.New("unterminated runtime value")
}
