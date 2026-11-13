// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

// voip 沿用公开产品的手写注册路径（同 sport），不修改生成式
// register_products.go。根命令 Hidden（misc 长尾形态）；MCP 端点经
// internal/syncdata/endpoints.go 的 conference server 注册。
func init() {
	RegisterPublicNamed("voip", func() Handler {
		return wukongHandler{name: "voip", buildFn: newVoipCommand}
	})
}
