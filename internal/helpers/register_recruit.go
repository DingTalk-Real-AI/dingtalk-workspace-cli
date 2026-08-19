// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// 招聘是开源库显式维护的公开命令，不依赖生成式产品注册表。
func init() {
	RegisterPublic(func() Handler {
		return wukongHandler{name: "recruit", buildFn: newRecruitCommand}
	})
}
