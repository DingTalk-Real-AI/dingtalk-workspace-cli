package safechat

import (
	"time"

	"github.com/google/uuid"
)

// Logger defines the logging interface for the SDK.
// Implementations should be safe for concurrent use.
type Logger interface {
	// Debug logs a debug-level message
	Debug(msg string, args ...interface{})
	// Info logs an info-level message
	Info(msg string, args ...interface{})
	// Error logs an error-level message
	Error(msg string, args ...interface{})
}

// Config holds configuration parameters for the SafeChat client.
type Config struct {
	// DataPath is the directory path for storing keys and related data (required).
	// The directory must exist and be writable.
	DataPath string

	// UserID is an optional user identifier. If empty, a random UUID is
	// generated automatically. The C library stores this value but does not
	// use it for key operations, so any non-empty string works.
	UserID string

	// Code is a DingTalk 免登 authCode used for key server authentication
	// when AuthCodeHook is nil. Prefer AuthCodeHook: the code is one-shot
	// and should be minted only when goProxy actually needs a key.
	//
	// Required (via Code or AuthCodeHook) when:
	//   - First-time key fetch (empty keystore)
	//   - Server-side key version rotation
	//
	// If keys are already cached AND the key version has not changed,
	// neither field is used (no network request is made).
	Code string

	// AuthCodeHook is called from goProxy immediately before the key
	// request. corpID and domain come from the C library callback.
	// domain is the vendor SSO host and must not be forwarded as an
	// authorize redirectURI; it is only for local host checks.
	// If set, it replaces Code for that request and the returned value
	// is not stored on the client.
	AuthCodeHook func(corpID, domain string) (string, error)

	// KeyServer is an optional override for the key server URL.
	// If empty, the URL provided by the C library's goProxy callback will be used.
	KeyServer string

	// MaxRetry is the maximum number of retry attempts when key is not yet available.
	// Default: 5 (private server redirect consumes 2 retries, so 5 provides sufficient margin)
	MaxRetry int

	// HTTPTimeout is the timeout for HTTP key requests.
	// Default: 10s
	HTTPTimeout time.Duration

	// Logger is an optional logger instance.
	// If nil, no logging will be performed.
	Logger Logger
}

// defaultConfig returns a Config with sensible defaults applied.
func defaultConfig(cfg Config) Config {
	if cfg.MaxRetry <= 0 {
		cfg.MaxRetry = 5
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}
	if cfg.UserID == "" {
		cfg.UserID = uuid.New().String()
	}
	return cfg
}

// validate checks that required config fields are set.
func (cfg *Config) validate() error {
	if cfg.DataPath == "" {
		return ErrConfigDataPathEmpty
	}
	return nil
}
