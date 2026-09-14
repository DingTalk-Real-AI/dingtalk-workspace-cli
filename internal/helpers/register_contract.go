// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

// 智能合同是显式维护的隐藏 vendor 扩展命令，不依赖生成式产品注册表。
func init() {
	RegisterPublic(func() Handler {
		return wukongHandler{name: "contract", buildFn: newContractCommand}
	})
}
