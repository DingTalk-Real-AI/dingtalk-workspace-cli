// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// 白板是显式编排的公开命令，不依赖 Wukong 的生成式产品注册表。
func init() {
	RegisterPublic(func() Handler {
		return wukongHandler{name: "whiteboard", buildFn: newWhiteboardCommand}
	})
}
