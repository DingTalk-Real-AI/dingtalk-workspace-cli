// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build windows

package runtimecontext

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProcEntry struct {
	ppid int
	comm string
}

var (
	windowsCreateProcessSnapshot = windows.CreateToolhelp32Snapshot
	windowsGetProcessTimes       = windows.GetProcessTimes
)

func collectProcessAncestry(maxDepth int) ([]ancestorProcess, bool) {
	procs, ok := windowsSnapshotProcesses()
	if !ok {
		return nil, false
	}
	return walkProcessAncestry(os.Getppid(), maxDepth, 1, func(pid int) (ancestorProcess, bool) {
		return windowsLookupProcess(procs, pid)
	})
}

func windowsLookupProcess(procs map[int]windowsProcEntry, pid int) (ancestorProcess, bool) {
	info, exists := procs[pid]
	if !exists {
		return ancestorProcess{}, false
	}
	return ancestorProcess{
		PID:       pid,
		PPID:      info.ppid,
		StartTime: windowsProcessStartTime(pid),
		Comm:      info.comm,
	}, true
}

func windowsSnapshotProcesses() (map[int]windowsProcEntry, bool) {
	snapshot, err := windowsCreateProcessSnapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	defer windows.CloseHandle(snapshot)

	procs := make(map[int]windowsProcEntry)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		procs[int(entry.ProcessID)] = windowsProcEntry{
			ppid: int(entry.ParentProcessID),
			comm: windows.UTF16ToString(entry.ExeFile[:]),
		}
	}
	return procs, true
}

func windowsProcessStartTime(pid int) int64 {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if windowsGetProcessTimes(handle, &creation, &exit, &kernel, &user) != nil {
		return 0
	}
	return creation.Nanoseconds()
}
