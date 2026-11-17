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

package cmdpolicy

import (
	"fmt"
	"sort"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/commandstore"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// IsPolicyDiagnostic reports whether cmd is the command that reports the
// policy itself. It reads only local state and must stay usable when sign-in
// is missing, expired, or blocked, so callers skip credential and edition
// hooks for it.
func IsPolicyDiagnostic(cmd *cobra.Command) bool {
	return CommandPath(cmd) == PolicyShowPath
}

// snapshots owns the resolved policy of each live command tree, keyed weakly
// by its root. Snapshot values hold no command pointers.
var snapshots commandstore.Map

// For returns the policy snapshot of the tree cmd belongs to, or nil when
// command restriction is disabled for that tree.
func For(cmd *cobra.Command) *Snapshot {
	if cmd == nil {
		return nil
	}
	value, ok := snapshots.Load(cmd.Root())
	if !ok {
		return nil
	}
	snapshot, _ := value.(*Snapshot)
	return snapshot
}

// Apply evaluates snapshot against the fully assembled tree under root,
// installs the denials, and binds the completed snapshot to root. It must run
// after every command is mounted and before corecmd.PrepareCommandTree. A nil
// snapshot (policy disabled) leaves the tree untouched.
//
// Denied commands are rewritten in place, never removed: every invocation
// still constructs the same complete tree.
func Apply(root *cobra.Command, snapshot *Snapshot) {
	if root == nil || snapshot == nil {
		return
	}
	index := buildIdentityIndex(root)
	snapshot.decisions = make(map[string]Decision, len(index.entries))
	snapshot.identities = make(map[string]string, len(index.entries))

	evaluations := make(map[string]evaluation, len(index.subjects))
	for primary, subj := range index.subjects {
		evaluations[primary] = evaluate(snapshot.state, snapshot.rules, *subj)
	}
	for canonical, primary := range index.canonical {
		snapshot.identities[canonical] = primary
	}

	for _, item := range index.entries {
		primary := index.owner[item.path]
		decision := evaluations[primary].decision
		if reason, conflicted := index.conflicts[item.path]; conflicted && snapshot.state == StateActive && !discoverySet[item.path] {
			decision = Decision{ReasonCode: ReasonIdentityConflict, Reason: "command identity cannot be attributed reliably: " + reason}
		}
		snapshot.decisions[item.path] = decision
		snapshot.identities[item.path] = item.path
		if !decision.Allowed {
			installDenial(item, decision, snapshot.state)
		}
	}
	// An alias path with no live node of its own still resolves to its
	// primary for lookups.
	for path, primary := range index.owner {
		if _, ok := snapshot.identities[path]; !ok {
			if _, live := snapshot.decisions[primary]; live {
				snapshot.identities[path] = primary
			}
		}
	}

	hideDeadBranches(root, snapshot)
	snapshot.channels = channelStatuses(snapshot)
	snapshot.hints = buildHints(snapshot, evaluations)
	snapshots.Store(root, snapshot)
}

func installDenial(item *entry, decision Decision, state State) {
	path := item.path
	deny := func(*cobra.Command, []string) error { return DenialError(path, decision, state) }
	switch item.kind {
	case kindHybrid:
		corecmd.DenyGroupBusiness(item.cmd, deny)
		hideBusinessFlags(item.cmd)
	case kindUndeclaredGroup:
		item.cmd.Run, item.cmd.RunE = nil, deny
		item.cmd.PreRun, item.cmd.PreRunE = nil, deny
		hideBusinessFlags(item.cmd)
	default:
		installLeafStub(item.cmd, deny)
	}
}

// installLeafStub turns a leaf into a hidden stub that only reports the
// denial. Flag parsing is disabled and arbitrary arguments are accepted so the
// denial is reported whatever the caller typed. The stub shadows the root's
// PersistentPreRunE: a denied command must not reach credential, edition, or
// output hooks.
func installLeafStub(cmd *cobra.Command, deny func(*cobra.Command, []string) error) {
	cmd.Hidden = true
	cmd.DisableFlagParsing = true
	cmd.Args = cobra.ArbitraryArgs
	cmd.ValidArgs = nil
	cmd.ValidArgsFunction = nil
	cmd.PersistentPreRun = nil
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error { return nil }
	cmd.PersistentPostRun = nil
	cmd.PersistentPostRunE = func(*cobra.Command, []string) error { return nil }
	cmd.PreRun, cmd.PreRunE = nil, nil
	cmd.PostRun, cmd.PostRunE = nil, nil
	cmd.Run, cmd.RunE = nil, deny
	// Shell completion resolves the target and enumerates its flags without
	// ever running RunE, and does so even when flag parsing is disabled.
	// Hidden flags are the one thing it skips.
	cmd.NonInheritedFlags().VisitAll(hideFlag)
}

// hideBusinessFlags hides the flags that belong to a denied group's own
// business operation from help and completion. Persistent flags stay visible:
// permitted children inherit them.
func hideBusinessFlags(cmd *cobra.Command) {
	cmd.LocalNonPersistentFlags().VisitAll(hideFlag)
}

func hideFlag(flag *pflag.Flag) {
	if flag.Name != "help" {
		flag.Hidden = true
	}
}

// hideDeadBranches hides every node that offers nothing executable anymore: a
// denied entry whose children (if any) are all dead, and a navigation group
// whose children are all dead. Hiding is presentation only; execution is
// refused by the installed denials, never by Hidden.
func hideDeadBranches(root *cobra.Command, snapshot *Snapshot) {
	var dead func(cmd *cobra.Command) bool
	dead = func(cmd *cobra.Command) bool {
		children := cmd.Commands()
		allChildrenDead := true
		for _, child := range children {
			if !dead(child) {
				allChildrenDead = false
			}
		}
		decision, evaluated := snapshot.decisions[CommandPath(cmd)]
		switch {
		case evaluated && decision.Allowed:
			return false
		case evaluated:
			// Denied entry: dead once nothing below it is alive.
		case len(children) == 0:
			// Not an executable entry and no children: nothing was denied
			// here, so leave its visibility alone.
			return false
		}
		if !allChildrenDead {
			return false
		}
		cmd.Hidden = true
		return true
	}
	for _, child := range root.Commands() {
		dead(child)
	}
}

func channelStatuses(snapshot *Snapshot) []ChannelStatus {
	statuses := make([]ChannelStatus, 0, len(channelEntries))
	for _, channel := range channelEntries {
		decision, ok := snapshot.decisions[channel.Path]
		if !ok {
			continue
		}
		status := ChannelStatus{Path: channel.Path, Allowed: decision.Allowed}
		if decision.Allowed {
			status.RuleName = decision.RuleName
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func buildHints(snapshot *Snapshot, evaluations map[string]evaluation) []Hint {
	var hints []Hint
	for _, rule := range snapshot.rules {
		if RuleAllowsAll(rule) {
			hints = append(hints, Hint{
				Kind:    HintRuleAllowsAll,
				Rule:    rule.Name,
				Message: fmt.Sprintf("rule %q has no Allow and no effect/risk ceiling: it allows every declared non-channel command", rule.Name),
			})
		}
	}
	primaries := make([]string, 0, len(evaluations))
	for primary := range evaluations {
		primaries = append(primaries, primary)
	}
	sort.Strings(primaries)
	for _, primary := range primaries {
		result := evaluations[primary]
		if !result.decision.Allowed {
			continue
		}
		for _, denier := range result.deniedBy {
			hints = append(hints, Hint{
				Kind:      HintDenyOverridden,
				Path:      primary,
				Rule:      denier,
				OtherRule: result.decision.RuleName,
				Message: fmt.Sprintf("%q is denied by rule %q but allowed by rule %q; a Deny only applies within its own rule",
					primary, denier, result.decision.RuleName),
			})
		}
	}
	return hints
}

// Gate is the earliest execution check, called from the root's
// PersistentPreRunE before any credential, edition, or output hook. Leaf stubs
// shadow that hook and refuse on their own; the gate covers hybrid groups and
// subtrees that chain the root hook.
func Gate(cmd *cobra.Command, args []string) error {
	snapshot := For(cmd)
	if snapshot == nil {
		return nil
	}
	path := CommandPath(cmd)
	decision, evaluated := snapshot.decisions[path]
	if !evaluated || decision.Allowed {
		return nil
	}
	if len(args) > 0 && corecmd.GroupRecoversPositionals(cmd) {
		// A leftover positional on a recovering group is a mistyped
		// subcommand, not the group's business operation.
		return nil
	}
	return DenialError(path, decision, snapshot.state)
}

// DiscoveryDenial returns the denial that hides path from discovery surfaces
// (help and Schema), or nil when path may be shown. A denied command reports
// its own verdict; a group in which every command is denied reports
// all_children_denied. A hybrid group with a permitted child stays visible
// even when its own operation is denied, so that child remains discoverable.
func DiscoveryDenial(snapshot *Snapshot, path string) error {
	if snapshot == nil || !snapshot.SubtreeDenied(path) {
		return nil
	}
	decision, evaluated := snapshot.Decision(path)
	if !evaluated {
		decision = Decision{
			ReasonCode: ReasonAllChildrenDenied,
			Reason:     "no command under this path is permitted",
		}
	}
	return DenialError(path, decision, snapshot.state)
}

// DenialError is the structured error every denied command returns. It is a
// classified validation error (exit code 3): the invocation is refused before
// any business work starts, so it is not retryable and changes nothing.
func DenialError(path string, decision Decision, state State) error {
	details := map[string]any{
		"path":         path,
		"reason_code":  decision.ReasonCode,
		"policy_state": string(state),
	}
	if decision.RuleName != "" {
		details["rule_name"] = decision.RuleName
	}
	message := fmt.Sprintf("%s is denied by the command restriction policy (%s): %s",
		DisplayPath(path), decision.ReasonCode, decision.Reason)
	hint := "this command is not permitted in this build; run `dws config policy show` to inspect the active rules, and do not retry it"
	// The distributor's guidance is additive: it never replaces the stable
	// message, reason code, or the instruction not to retry.
	if decision.DenyMessage != "" {
		details["deny_message"] = decision.DenyMessage
		hint += ". " + decision.DenyMessage
	}
	return apperrors.NewValidation(message,
		apperrors.WithReason("command_denied"),
		apperrors.WithHint(hint),
		apperrors.WithDetails(details),
		apperrors.WithRetryable(false),
	)
}
