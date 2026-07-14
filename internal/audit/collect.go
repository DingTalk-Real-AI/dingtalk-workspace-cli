package audit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

const (
	EnvAudit         = "DWS_AUDIT"
	EnvAuditDir      = "DWS_AUDIT_DIR"
	EnvRetentionDays = "DWS_AUDIT_RETENTION_DAYS"
	EnvForwardURL    = "DWS_AUDIT_FORWARD_URL"
	EnvForwardToken  = "DWS_AUDIT_FORWARD_TOKEN"
	EnvForwardRedact = "DWS_AUDIT_FORWARD_REDACT"
	EnvAuditDebug    = "DWS_AUDIT_DEBUG"

	defaultRetentionDays = 90
	auditSubdir          = "audit"
)

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name:         EnvAudit,
		Category:     configmeta.CategoryAudit,
		Description:  "操作审计日志开关（默认启用，设 0/false/off 关闭）",
		DefaultValue: "启用",
		Example:      "0",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        EnvAuditDir,
		Category:    configmeta.CategoryAudit,
		Description: "审计日志目录（默认 <configDir>/audit）",
		Example:     "/var/log/dws-audit",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:         EnvRetentionDays,
		Category:     configmeta.CategoryAudit,
		Description:  "审计日志留存天数",
		DefaultValue: "90",
		Example:      "180",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        EnvForwardURL,
		Category:    configmeta.CategoryAudit,
		Description: "审计事件远端转发 URL（POST JSON）",
		Example:     "https://siem.example.com/audit",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        EnvForwardToken,
		Category:    configmeta.CategoryAudit,
		Description: "远端转发 Bearer Token",
		Sensitive:   true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:         EnvForwardRedact,
		Category:     configmeta.CategoryAudit,
		Description:  "远端转发脱敏级别：none / hashed / minimal",
		DefaultValue: "none",
		Example:      "hashed",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name:        EnvAuditDebug,
		Category:    configmeta.CategoryAudit,
		Description: "打印审计子系统初始化/写入/转发失败诊断到 stderr（设 1/true/on 开启）",
		Example:     "1",
	})
}

// DebugEnabled reports whether audit-subsystem diagnostics should be surfaced to
// stderr. Failures are always eligible for the structured log; this gates the
// noisier stderr channel.
func DebugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvAuditDebug))) {
	case "1", "true", "on", "yes", "y":
		return true
	}
	return false
}

func IsEnabled() bool {
	v := os.Getenv(EnvAudit)
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "0", "false", "off", "no", "n":
		return false
	}
	return true
}

// BuildSink constructs the audit sink for configDir. It returns an error when
// the audit subsystem is enabled but cannot initialize (e.g. the log directory
// is not writable) so the caller can surface the failure instead of silently
// degrading. report receives non-fatal forwarder diagnostics; it may be nil.
func BuildSink(configDir string, report func(format string, args ...any)) (Sink, error) {
	if !IsEnabled() {
		return NopSink{}, nil
	}

	dir := os.Getenv(EnvAuditDir)
	if dir == "" {
		dir = filepath.Join(configDir, auditSubdir)
	}

	retention := defaultRetentionDays
	if v := os.Getenv(EnvRetentionDays); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			retention = n
		}
	}

	writer, err := NewDateRotatingWriter(dir, retention)
	if err != nil {
		return NopSink{}, err
	}

	chain := NewChain(dir)

	var forwarder *HTTPForwarder
	if fwdURL := os.Getenv(EnvForwardURL); fwdURL != "" {
		token := os.Getenv(EnvForwardToken)
		redact := RedactLevel(strings.ToLower(os.Getenv(EnvForwardRedact)))
		if redact != RedactHashed && redact != RedactMinimal {
			redact = RedactNone
		}
		forwarder = NewHTTPForwarder(fwdURL, token, redact, report)
	}

	return NewFileSink(writer, chain, forwarder), nil
}
