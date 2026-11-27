// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build windows

package runtimecontext

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func collectProcessAncestry(maxDepth int) ([]ancestorProcess, bool) {
	pid := os.Getppid()
	if pid <= 0 || maxDepth <= 0 {
		return nil, false
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	defer windows.CloseHandle(snapshot)

	type procInfo struct {
		ppid int
		comm string
	}
	procs := make(map[int]procInfo)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		procs[int(entry.ProcessID)] = procInfo{
			ppid: int(entry.ParentProcessID),
			comm: windows.UTF16ToString(entry.ExeFile[:]),
		}
	}

	chain := make([]ancestorProcess, 0, maxDepth)
	seen := make(map[int]struct{}, maxDepth)
	for len(chain) < maxDepth && pid > 0 {
		if _, dup := seen[pid]; dup {
			break
		}
		seen[pid] = struct{}{}
		info, exists := procs[pid]
		if !exists {
			break
		}
		var startTime int64
		if handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)); err == nil {
			var creation, exit, kernel, user windows.Filetime
			if windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user) == nil {
				startTime = creation.Nanoseconds()
			}
			_ = windows.CloseHandle(handle)
		}
		chain = append(chain, ancestorProcess{
			PID:       pid,
			PPID:      info.ppid,
			StartTime: startTime,
			Comm:      info.comm,
		})
		if info.ppid <= 0 || info.ppid == pid {
			break
		}
		pid = info.ppid
	}
	return chain, len(chain) > 0
}
