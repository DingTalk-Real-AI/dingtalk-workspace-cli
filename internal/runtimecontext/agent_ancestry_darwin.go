// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build darwin

package runtimecontext

import (
	"os"

	"golang.org/x/sys/unix"
)

func collectProcessAncestry(maxDepth int) ([]ancestorProcess, bool) {
	pid := os.Getppid()
	if pid <= 1 || maxDepth <= 0 {
		return nil, false
	}
	chain := make([]ancestorProcess, 0, maxDepth)
	seen := make(map[int]struct{}, maxDepth)
	for len(chain) < maxDepth && pid > 1 {
		if _, dup := seen[pid]; dup {
			break
		}
		seen[pid] = struct{}{}
		kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
		if err != nil || kp == nil {
			break
		}
		ppid := int(kp.Eproc.Ppid)
		comm := unix.ByteSliceToString(kp.Proc.P_comm[:])
		startNS := kp.Proc.P_starttime.Sec*1_000_000_000 + int64(kp.Proc.P_starttime.Usec)*1_000
		chain = append(chain, ancestorProcess{
			PID:       pid,
			PPID:      ppid,
			StartTime: startNS,
			Comm:      comm,
		})
		if ppid <= 1 || ppid == pid {
			break
		}
		pid = ppid
	}
	return chain, len(chain) > 0
}
