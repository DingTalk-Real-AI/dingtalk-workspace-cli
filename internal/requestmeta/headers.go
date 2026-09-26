// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package requestmeta owns DWS request metadata header names.
package requestmeta

import "strings"

const DingTalkExtHeader = "x-dingtalk-ext"

// 委托输入由宿主提供；统一 UID 只能由网关解析后注入业务请求。
const (
	DelegatorUserIDHeader         = "delegator-user-id"
	DelegatorCorpIDHeader         = "delegator-corp-id"
	DelegatorOpenDingtalkIDHeader = "delegator-open-dingtalk-id"
	DelegatorUIDHeader            = "delegator-uid"
)

// IsDelegatorHeader 同时保留输入和网关输出字段，防止其他 Header 来源冒充身份。
func IsDelegatorHeader(name string) bool {
	switch strings.ToLower(name) {
	case DelegatorUserIDHeader, DelegatorCorpIDHeader, DelegatorOpenDingtalkIDHeader, DelegatorUIDHeader:
		return true
	default:
		return false
	}
}

func RemoveDelegatorHeaders(headers map[string]string) {
	for key := range headers {
		if IsDelegatorHeader(key) {
			delete(headers, key)
		}
	}
}
