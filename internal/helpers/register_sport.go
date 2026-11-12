// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// sport 沿用公开产品的手写注册路径，不修改生成式 register_products.go。
// 根帮助可见性由 newSportCommand 声明，MCP 端点由 edition supplement servers 提供。
func init() {
	RegisterPublicNamed("sport", func() Handler {
		return wukongHandler{name: "sport", buildFn: newSportCommand}
	})
}
