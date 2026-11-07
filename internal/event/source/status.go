// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package source

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"

func transportStatus(s Snapshot) transport.StatusSource {
	result := transport.StatusSource{State: string(s.State), Source: string(s.StateSource), Observed: true, ReconnectCount: s.ReconnectCount}
	if !s.LastEventAt.IsZero() {
		result.LastEventAtMS = s.LastEventAt.UnixMilli()
	}
	if !s.LastReconnectAt.IsZero() {
		result.LastReconnectMS = s.LastReconnectAt.UnixMilli()
	}
	return result
}

func (s *PersonalSource) SourceStatus() transport.StatusSource { return transportStatus(s.State()) }
func (s *DingtalkSource) SourceStatus() transport.StatusSource { return transportStatus(s.State()) }
