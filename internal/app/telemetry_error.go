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

package app

import "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clitelemetry"

const maxTelemetryErrorRunes = clitelemetry.MaxErrorRunes

func telemetryErrorSummary(err error) string            { return clitelemetry.ErrorSummary(err) }
func telemetryPanicMessages(value any) (string, string) { return clitelemetry.PanicMessages(value) }
func sanitizeTelemetryErrorText(message string) string {
	return clitelemetry.SanitizeErrorText(message)
}
func truncateTelemetryText(message string, maxRunes int) string {
	return clitelemetry.TruncateText(message, maxRunes)
}
