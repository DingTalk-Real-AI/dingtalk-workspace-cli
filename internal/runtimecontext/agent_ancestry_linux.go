// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build linux

package runtimecontext

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var linuxReadProcStat = os.ReadFile

func collectProcessAncestry(maxDepth int) ([]ancestorProcess, bool) {
	return walkProcessAncestry(os.Getppid(), maxDepth, 2, linuxLookupProcess)
}

func linuxLookupProcess(pid int) (ancestorProcess, bool) {
	data, err := linuxReadProcStat(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ancestorProcess{}, false
	}
	comm, ppid, startTicks, ok := parseLinuxProcStat(data)
	if !ok {
		return ancestorProcess{}, false
	}
	return ancestorProcess{
		PID:       pid,
		PPID:      ppid,
		StartTime: startTicks,
		Comm:      comm,
	}, true
}

func parseLinuxProcStat(data []byte) (comm string, ppid int, startTime int64, ok bool) {
	openIdx := bytes.IndexByte(data, '(')
	closeIdx := bytes.LastIndexByte(data, ')')
	if openIdx < 0 || closeIdx <= openIdx || closeIdx+2 >= len(data) {
		return "", 0, 0, false
	}
	comm = string(data[openIdx+1 : closeIdx])
	fields := strings.Fields(string(data[closeIdx+2:]))
	// fields[0]=state, fields[1]=ppid, ..., fields[19]=starttime (field 22 overall).
	if len(fields) < 20 {
		return "", 0, 0, false
	}
	parsedPPID, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, 0, false
	}
	parsedStart, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return "", 0, 0, false
	}
	return comm, parsedPPID, parsedStart, true
}
