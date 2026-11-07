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

package errors

import (
	stderrors "errors"
	"strings"
)

// DoctorCommand 是机器错误信封中的稳定体检入口。
// actions 字段承载可直接执行的补救命令，因此这里保持纯命令形式，不混入说明文字。
const DoctorCommand = "dws doctor --json"

// DoctorHumanCommand 保留 doctor 面向真人的默认可读输出。
const DoctorHumanCommand = "dws doctor"

// RecoveryActions 返回错误最终应该交付的恢复动作。
//
// 命令构造侧仍可声明更具体的动作；本函数只在 doctor 能诊断的认证或网络故障上
// 补齐稳定入口。权限、参数、确认门禁和上游业务错误不会被机械地
// 引导到 doctor，避免用本地体检掩盖明确的业务修复路径。
// 路由只读取 wire-stable 的 Category/Reason，不根据 Message/Hint 文案分支。
func RecoveryActions(err error) []string {
	var typed *Error
	if !stderrors.As(err, &typed) || typed == nil {
		return nil
	}

	actions := append([]string(nil), typed.Actions...)
	if shouldSuggestDoctor(typed) && !containsDoctorAction(actions) {
		actions = append(actions, DoctorCommand)
	}
	return actions
}

// HumanRecoveryActions 与 RecoveryActions 使用同一适用性策略，但把规范的
// Agent 体检命令投影为 doctor 默认的人类可读模式。
func HumanRecoveryActions(err error) []string {
	actions := RecoveryActions(err)
	for i, action := range actions {
		if strings.TrimSpace(action) == DoctorCommand {
			actions[i] = DoctorHumanCommand
		}
	}
	return actions
}

func shouldSuggestDoctor(err *Error) bool {
	reason := strings.ToLower(strings.TrimSpace(err.Reason))
	if permissionOrConfirmationFailure(reason) {
		return false
	}

	switch err.Category {
	case CategoryAuth:
		return true
	case CategoryAPI, CategoryDiscovery:
		switch reason {
		case "request_failed",
			"request_timeout",
			"http_client_timeout",
			"tls_timeout",
			"connection_refused",
			"dns_resolution_failed",
			"io_timeout",
			"backend_dependency_unavailable":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func permissionOrConfirmationFailure(reason string) bool {
	if reason == "confirmation_required" || reason == "http_403" || reason == "pat_auth_timeout" ||
		strings.Contains(reason, "forbidden") || strings.Contains(reason, "permission") || strings.Contains(reason, "scope") {
		return true
	}
	return false
}

func containsDoctorAction(actions []string) bool {
	for _, action := range actions {
		if strings.Contains(strings.ToLower(action), "dws doctor") {
			return true
		}
	}
	return false
}
