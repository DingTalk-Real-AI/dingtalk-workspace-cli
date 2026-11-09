// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// 企业运动健康（钉钉运动）是显式维护的公开命令，不依赖生成式产品注册表。
// MCP 端点登记在 pkg/edition/default.go 的 openSupplementServers（server ID=sport）。
func init() {
	RegisterPublicNamed("sport", func() Handler {
		return wukongHandler{name: "sport", buildFn: newSportCommand}
	})
}
