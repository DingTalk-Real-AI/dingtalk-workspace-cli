// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemafastpath"

// capability is the executable allowlist from RFC §1.2. Any request that does
// not match one exact entry is delegated without trying another fast path.
type capability uint8

const (
	capabilityDelegate capability = iota
	capabilityVersion
	capabilityRootHelp
	capabilitySchema
)

func classifyCapability(args, environment []string) capability {
	// The explicit version opt-out is intentionally filesystem-free and has no
	// configuration-sensitive behavior. Preserve it even in an ambient build
	// environment that contains unrelated DWS_* variables.
	if len(args) == 2 && args[1] == "--version" && telemetryOptedOut(environment) {
		return capabilityVersion
	}
	if !schemafastpath.PlainEnvironment(environment) {
		return capabilityDelegate
	}
	if len(args) == 2 {
		switch args[1] {
		case "--version":
			return capabilityVersion
		case "--help":
			return capabilityRootHelp
		}
	}
	if telemetryOptedOut(environment) && schemafastpath.SupportsRequest(args, environment) {
		return capabilitySchema
	}
	return capabilityDelegate
}
