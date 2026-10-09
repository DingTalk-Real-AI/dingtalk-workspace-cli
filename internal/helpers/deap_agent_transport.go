// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"slices"
	"strings"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

var employeeDSHTransportStatus = observeEmployeeDSHTransport
var employeeEventBuses = busctl.EnumerateBuses
var employeeEventStatus = busctl.QueryStatus

// 宿主的 ready 标记只证明本地 IPC；独立核对同一员工的实时上游状态。
func observeEmployeeDSHTransport(b digitalEmployeeBinding) transport.StatusSource {
	unknown := transport.StatusSource{State: "unknown", Source: "unknown"}
	corp, user, ok := strings.Cut(b.DWSProfile, ":")
	if !ok || corp == "" || user == "" {
		return unknown
	}
	entries, err := employeeEventBuses(deapConnectConfigDir(), "")
	if err != nil {
		return unknown
	}
	var matches []busctl.BusEntry
	for _, entry := range entries {
		if entry.State != busctl.BusStateRunning || entry.Meta == nil || entry.SourceKind != dwsevent.SourceKindPersonalStream {
			continue
		}
		identity := personal.Identity{CorpID: corp, UserID: user, ClientID: entry.Meta.ClientID, SourceID: entry.Meta.SourceID}
		if dwsevent.IdentityHash(identity.Key()) == entry.IdentityHash {
			matches = append(matches, entry)
		}
	}
	// 多个来源不能互相代替，保守返回未知，避免借用其他 consumer 的健康状态。
	if len(matches) != 1 {
		return unknown
	}
	entry := matches[0]
	s, err := employeeEventStatus(entry.IPCEndpoint())
	if err != nil || s.Bus.PID != entry.HolderPID || s.Bus.IdentityHash != entry.IdentityHash || s.Bus.SourceID != entry.Meta.SourceID {
		return unknown
	}
	for _, consumer := range s.Consumers {
		if slices.Contains(consumer.EventTypes, personal.EventAllSingleChat) {
			return s.SourceState
		}
	}
	return unknown
}
