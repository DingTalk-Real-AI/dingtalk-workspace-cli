package app

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

var (
	auditSinkOnce   sync.Once
	auditCloseOnce  sync.Once
	sharedAuditSink audit.Sink

	auditIDMu      sync.Mutex
	cachedActor    audit.Actor
	cachedAgentID  string
	cachedProfile  string
	identityLoaded bool

	// loadTokenForProfile is the profile-scoped token loader. It is a package
	// variable so profile-switch Actor attribution can be tested deterministically
	// without touching the OS keychain.
	loadTokenForProfile = auth.LoadTokenDataForProfile
)

// setupAuditSink builds the process-wide audit sink once and caches it so the
// runner and the shutdown hook share a single writer/forwarder instance.
func setupAuditSink() audit.Sink {
	auditSinkOnce.Do(func() {
		sink, err := audit.BuildSink(defaultConfigDir(), auditReport)
		if err != nil {
			auditReport("initialization failed, audit disabled for this session: %v", err)
			sharedAuditSink = audit.NopSink{}
			return
		}
		sharedAuditSink = sink
	})
	return sharedAuditSink
}

// CloseAuditSink flushes in-flight remote forwards and closes the audit writer.
// It is invoked from an unconditional defer in Execute so the drain happens for
// both successful and failed commands (Cobra skips PersistentPostRunE when RunE
// returns an error). The sync.Once makes repeated calls safe.
func CloseAuditSink() {
	auditCloseOnce.Do(func() {
		if sharedAuditSink == nil {
			return
		}
		if err := sharedAuditSink.Close(); err != nil {
			auditReport("close failed: %v", err)
		}
	})
}

// auditReport routes non-fatal audit-subsystem diagnostics to the structured
// file log (always, when available) and to stderr when DWS_AUDIT_DEBUG is set,
// so init/write/forward failures are observable instead of silently swallowed.
func auditReport(format string, args ...any) {
	msg := "audit: " + fmt.Sprintf(format, args...)
	if l := FileLoggerInstance(); l != nil {
		l.Warn(msg)
	}
	if audit.DebugEnabled() {
		fmt.Fprintln(os.Stderr, "[dws] "+msg)
	}
}

// auditIdentity resolves the Actor for the active runtime profile. The result
// is cached per-profile so a profile switch within a long-running process (e.g.
// serve mode) re-resolves rather than reusing a stale identity.
func auditIdentity() (audit.Actor, string) {
	profile := auth.RuntimeProfile()

	auditIDMu.Lock()
	defer auditIDMu.Unlock()
	if identityLoaded && profile == cachedProfile {
		return cachedActor, cachedAgentID
	}

	configDir := defaultConfigDir()
	var actor audit.Actor
	if td, err := loadTokenForProfile(configDir, profile); err == nil && td != nil {
		actor = audit.Actor{
			UserID:   td.UserID,
			Name:     td.UserName,
			CorpID:   td.CorpID,
			CorpName: td.CorpName,
		}
	} else if err != nil {
		auditReport("resolve actor for profile %q failed: %v", profile, err)
	}

	agentID := ""
	if id := auth.Load(configDir); id != nil {
		agentID = id.AgentID
	}

	cachedActor, cachedAgentID, cachedProfile, identityLoaded = actor, agentID, profile, true
	return actor, agentID
}

func emitAudit(sink audit.Sink, execID string, invokeStart time.Time, invocation executor.Invocation, endpoint string, retErr error, cliVersion string) {
	if sink == nil {
		return
	}
	if _, ok := sink.(audit.NopSink); ok {
		return
	}

	actor, agentID := auditIdentity()

	result := "success"
	var errCat, errReason string
	if retErr != nil {
		result = "error"
		errCat, errReason = classifyAuditError(retErr)
	}

	paramsSummary := logging.SanitizeArguments(invocation.Params, 1024)

	evt := &audit.Event{
		Timestamp:     invokeStart,
		ExecutionID:   execID,
		AgentID:       agentID,
		Actor:         actor,
		Product:       invocation.CanonicalProduct,
		Command:       invocation.Tool,
		Endpoint:      transport.RedactURL(endpoint),
		ParamsSummary: paramsSummary,
		Result:        result,
		ErrCategory:   errCat,
		ErrReason:     errReason,
		DurationMs:    time.Since(invokeStart).Milliseconds(),
		CLIVersion:    cliVersion,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
	}

	if err := sink.Emit(evt); err != nil {
		auditReport("emit event failed (exec %s): %v", execID, err)
	}
}

func classifyAuditError(err error) (category, reason string) {
	if err == nil {
		return "", ""
	}
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		return string(typed.Category), typed.Reason
	}
	return "unknown", err.Error()
}
