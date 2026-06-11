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

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

// auditFilePrefix is the base name of the dated audit files
// (`<prefix>-YYYY-MM-DD.jsonl`).
const auditFilePrefix = "audit"

// setupAuditSink builds the active audit sink. When auditing is disabled
// (DWS_AUDIT_ENABLED unset) it returns audit.NopSink so emit is always safe.
// The local file lives next to the diagnostic log but is a SEPARATE file —
// audit and debug logs must not be conflated. The forwarder (if configured)
// targets the organization's own endpoint, never a vendor default.
//
// The local file is date-rotated (audit-YYYY-MM-DD.jsonl) so it never grows
// unbounded; files older than DWS_AUDIT_MAX_AGE_DAYS are pruned. The writer
// opens lazily on first write, so a read-only home simply yields a write error
// per event while a configured forwarder still works.
func setupAuditSink() audit.Sink {
	if !audit.Enabled() {
		return audit.NopSink{}
	}
	logDir := filepath.Join(defaultConfigDir(), "logs")
	w := audit.NewDateRotatingWriter(logDir, auditFilePrefix, audit.MaxAgeDays(), config.FilePerm, config.DirPerm)
	return audit.BuildSink(w)
}

// deviceOnce memoizes the device fingerprint: it is process-stable and the
// darwin/linux/windows collectors shell out, so we must not pay that cost on
// every command.
var (
	deviceOnce sync.Once
	deviceInfo audit.Device
)

func collectDeviceCached() audit.Device {
	deviceOnce.Do(func() {
		deviceInfo = audit.CollectDevice(audit.DeviceFingerprintEnabled())
	})
	return deviceInfo
}

// emitAudit assembles and emits one audit event for a finished invocation. It
// is called from executeInvocation's defer, where every dimension is known.
// Cheap to skip: audit.Enabled() is a single env read, so when auditing is off
// none of the heavier work (token load, device collect) runs.
func (r *runtimeRunner) emitAudit(ctx context.Context, execID, endpoint string,
	inv executor.Invocation, ok bool, errClass string, start time.Time) {

	if r == nil || r.auditSink == nil || !audit.Enabled() {
		return
	}

	ev := audit.New(time.Now(), execID)

	// Actor + Org from the persisted token — TRUSTWORTHY: the token is
	// validated by the gateway, so corp_id / user_id can't be spoofed by the
	// caller. (user_id is only present when the login flow captured it.)
	if td, err := authpkg.LoadTokenData(defaultConfigDir()); err == nil && td != nil {
		ev.Actor = audit.Actor{UserID: td.UserID, Name: td.UserName}
		ev.Org = audit.Org{CorpID: td.CorpID, Name: td.CorpName}
	}

	// Client: dws-managed install identity + compiled-in version. TRUSTWORTHY
	// (not caller-asserted per call). Load (not EnsureExists) so auditing never
	// creates identity state as a side effect.
	ev.Client.CLIVersion = version
	if id := authpkg.Load(defaultConfigDir()); id != nil {
		ev.Client.AgentID = id.AgentID
		ev.Client.Source = id.Source
	}
	// Channel (DWS_CHANNEL): which integration/agent is driving dws. SEMI-trusted
	// — the gateway validates channel membership against allowedChannels (an
	// unregistered channel is rejected), so it isn't an arbitrary label; but it
	// is not yet cryptographically bound, so a registered channel could still
	// impersonate another. Recorded so audit can group by "which agent called".
	ev.Client.Channel = strings.TrimSpace(os.Getenv(envDWSChannel))
	// TODO(audit): host_agent (DINGTALK_AGENT) / agent_code
	// (DINGTALK_DWS_AGENTCODE) are plain caller-supplied env labels — FULLY
	// FORGEABLE, so they are intentionally NOT recorded here. Add them (and
	// upgrade channel to fully-trusted) only once the gateway returns a SIGNED
	// agent identity bound to the token. See docs/audit.md "TODO".

	ev.Device = collectDeviceCached()

	// Natural-language intent only exists at the agent layer; mark provenance.
	if nl := audit.NLIntent(); nl != "" {
		ev.Intent.NLInput = nl
	}

	ev.Module = inv.CanonicalProduct
	ev.Command = inv.CanonicalProduct
	ev.Subcommand = inv.Tool
	ev.SubcommandDesc, ev.Target.Sensitivity = r.lookupToolMeta(ctx, inv)

	ev.Target = mergeTarget(ev.Target, buildTarget(inv.Params))
	ev.Flow = inferFlow(inv, endpoint)

	if ok {
		ev.Outcome = "ok"
	} else {
		ev.Outcome = "error"
		ev.ErrClass = errClass
		ev.ExitCode = 1
	}

	_ = r.auditSink.Emit(ev)
}

// lookupToolMeta pulls the static subcommand description and sensitivity from
// the catalog. Best-effort: the catalog is already loaded/cached for the
// command tree, so this is a cheap in-memory scan; any failure yields empties.
func (r *runtimeRunner) lookupToolMeta(ctx context.Context, inv executor.Invocation) (string, audit.Sensitivity) {
	if r.loader == nil {
		return "", audit.SensitivityUnknown
	}
	cat, err := r.loader.Load(ctx)
	if err != nil {
		return "", audit.SensitivityUnknown
	}
	for _, p := range cat.Products {
		if p.ID != inv.CanonicalProduct {
			continue
		}
		for _, t := range p.Tools {
			if t.RPCName == inv.Tool || t.CLIName == inv.Tool || t.CanonicalPath == inv.CanonicalPath {
				desc := t.Description
				if desc == "" {
					desc = t.Title
				}
				sens := audit.SensitivityUnknown
				if t.Sensitive {
					sens = audit.SensitivityConfidential
				}
				return desc, sens
			}
		}
	}
	return "", audit.SensitivityUnknown
}

// localPathKeys are param names that indicate data is exported to local disk.
var localPathKeys = map[string]bool{
	"output": true, "out": true, "path": true, "file": true, "filepath": true,
	"dir": true, "directory": true, "save_path": true, "local_path": true,
	"dest": true, "destination": true, "output_path": true, "target_path": true,
}

// peerIDKeySubstrings mark params carrying an intra-tenant object/person id.
var peerIDKeySubstrings = []string{
	"groupid", "openid", "userid", "unionid", "docid", "conversationid",
	"chatid", "cid", "fileid", "spaceid", "dentryid", "nodeid", "minuteid",
}

// readVerbs mark a tool as read-only (no data movement).
var readVerbs = []string{"list", "get", "query", "search", "detail", "info", "fetch", "view", "read", "describe"}

// nameKeySubstrings mark params holding a human-readable object name.
var nameKeySubstrings = []string{"name", "title", "subject"}

// buildTarget extracts a best-effort object identity from the call params.
func buildTarget(params map[string]any) audit.Target {
	var t audit.Target
	for k, v := range params {
		sv, ok := v.(string)
		if !ok || sv == "" {
			continue
		}
		lk := strings.ToLower(k)
		if t.Name == "" && containsAny(lk, nameKeySubstrings) {
			t.Name = sv
		}
		if t.ID == "" && (lk == "id" || strings.HasSuffix(lk, "id")) {
			t.ID = sv
		}
	}
	return t
}

// mergeTarget overlays b onto a without clobbering a's already-set fields
// (a carries Sensitivity from the catalog; b carries id/name from params).
func mergeTarget(a, b audit.Target) audit.Target {
	if a.ID == "" {
		a.ID = b.ID
	}
	if a.Name == "" {
		a.Name = b.Name
	}
	if a.Type == "" {
		a.Type = b.Type
	}
	return a
}

// inferFlow classifies the data-movement footprint of the command.
func inferFlow(inv executor.Invocation, endpoint string) audit.Flow {
	f := audit.Flow{API: inv.Tool, Endpoint: endpoint}

	// Local export wins: an explicit local path means data left the tenant to disk.
	for k, v := range inv.Params {
		if localPathKeys[strings.ToLower(k)] {
			if sv, ok := v.(string); ok && sv != "" {
				f.Direction = audit.DirectionLocalExport
				f.LocalPath = sv
				return f
			}
		}
	}

	verb := lastPathSegment(inv.CanonicalPath)
	if verb == "" {
		verb = strings.ToLower(inv.Tool)
	}
	if containsAny(strings.ToLower(verb), readVerbs) {
		f.Direction = audit.DirectionRead
		return f
	}

	// Otherwise data moves between objects inside the tenant; collect peer ids.
	for k, v := range inv.Params {
		lk := strings.ToLower(k)
		if containsAny(lk, peerIDKeySubstrings) {
			if sv, ok := v.(string); ok && sv != "" {
				f.PeerIDs = append(f.PeerIDs, sv)
			}
		}
	}
	f.Direction = audit.DirectionIntraTenant
	return f
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func lastPathSegment(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == ' ' || r == '.' || r == '/' })
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
