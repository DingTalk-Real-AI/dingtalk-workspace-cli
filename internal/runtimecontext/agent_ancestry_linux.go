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
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			break
		}
		comm, ppid, startTicks, ok := parseLinuxProcStat(data)
		if !ok {
			break
		}
		chain = append(chain, ancestorProcess{
			PID:       pid,
			PPID:      ppid,
			StartTime: startTicks,
			Comm:      comm,
		})
		if ppid <= 1 || ppid == pid {
			break
		}
		pid = ppid
	}
	return chain, len(chain) > 0
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
