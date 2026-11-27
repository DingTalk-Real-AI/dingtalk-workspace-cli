// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build darwin

package runtimecontext

import (
	"os"

	"golang.org/x/sys/unix"
)

func collectProcessAncestry(maxDepth int) ([]ancestorProcess, bool) {
	return walkProcessAncestry(os.Getppid(), maxDepth, 2, darwinLookupProcess)
}

func darwinLookupProcess(pid int) (ancestorProcess, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp == nil {
		return ancestorProcess{}, false
	}
	return ancestorProcess{
		PID:       pid,
		PPID:      int(kp.Eproc.Ppid),
		StartTime: kp.Proc.P_starttime.Sec*1_000_000_000 + int64(kp.Proc.P_starttime.Usec)*1_000,
		Comm:      unix.ByteSliceToString(kp.Proc.P_comm[:]),
	}, true
}
