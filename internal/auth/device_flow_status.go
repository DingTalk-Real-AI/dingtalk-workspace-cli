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

package auth

// Device flow authorization status constants.
// Shared across device_flow.go and pat_auth_retry.go to avoid maintaining
// string literals in multiple places.
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"
)

// ParseDeviceFlowStatus normalizes a raw status string from the device flow
// poll response into a canonical status constant.  When the server returns an
// empty status with success=false, it falls back to StatusExpired (server
// error / flow not found).
func ParseDeviceFlowStatus(rawStatus string, success bool) string {
	switch rawStatus {
	case StatusApproved, StatusRejected, StatusExpired, StatusPending, StatusCancelled:
		return rawStatus
	default:
		if rawStatus == "" && !success {
			return StatusExpired
		}
		return rawStatus
	}
}
