// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package launcher implements the small public DWS entry point. The full CLI
// is a separately signed executable in the same immutable version directory.
package launcher

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

const (
	EnvLauncherPath = "DWS_INTERNAL_LAUNCHER_PATH"
	EnvCoreDigest   = "DWS_INTERNAL_CORE_SHA256"
	EnvCoreVersion  = "DWS_INTERNAL_CORE_VERSION"

	ExitLauncherFailure = 125
)

// Options is immutable release identity injected into the launcher build.
type Options struct {
	SchemaIdentity   *schemareader.Identity
	Version          string
	Commit           string
	Edition          string
	CoreSHA256       string
	CoreSize         int64
	CoreRelativePath string
}

// ErrorKind classifies launcher failures independently of core exit status.
type ErrorKind string

const (
	ErrorConfiguration ErrorKind = "configuration"
	ErrorArtifact      ErrorKind = "artifact"
	ErrorDelegate      ErrorKind = "delegate"
)

// Error is a launcher-owned failure. Launcher failures map to exit code 125.
type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s %s: %v", e.Kind, e.Op, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// ExitError represents a non-zero exit status returned by the core process.
// It is only produced on platforms where delegation requires spawning.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("core exited with status %d", e.Code) }

// Main executes the launcher against the current process environment.
func Main(options Options) int {
	err := run(options, systemDependencies())
	if err == nil {
		return 0
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	fmt.Fprintf(os.Stderr, "dws launcher: %v\n", err)
	return ExitLauncherFailure
}

type dependencies struct {
	openSchemaCache func(string) (*schemacache.Cache, error)
	args            []string
	environ         []string
	stdin           io.Reader
	stdout          io.Writer
	stderr          io.Writer
	executable      func() (string, error)
	evalSymlinks    func(string) (string, error)
	lstat           func(string) (os.FileInfo, error)
	stat            func(string) (os.FileInfo, error)
	open            func(string) (coreFile, error)
	getwd           func() (string, error)
	delegate        func(string, []string, []string, string, io.Reader, io.Writer, io.Writer) (int, error)
}

type coreFile interface {
	io.Reader
	Stat() (os.FileInfo, error)
	Close() error
}

func systemDependencies() dependencies {
	return dependencies{
		openSchemaCache: func(edition string) (*schemacache.Cache, error) { return schemacache.Open(edition) },
		args:            os.Args,
		environ:         os.Environ(),
		stdin:           os.Stdin,
		stdout:          os.Stdout,
		stderr:          os.Stderr,
		executable:      os.Executable,
		evalSymlinks:    filepath.EvalSymlinks,
		lstat:           os.Lstat,
		stat:            os.Stat,
		open:            func(path string) (coreFile, error) { return os.Open(path) },
		getwd:           os.Getwd,
		delegate:        platformDelegate,
	}
}

func run(options Options, deps dependencies) error {
	if err := validateIdentity(options); err != nil {
		return &Error{Kind: ErrorConfiguration, Op: "validate release identity", Err: err}
	}
	// Keep the shipped identity/clitrack behavior until an equivalent thin
	// telemetry path is available. An explicit opt-out needs neither profile
	// reads nor reporting and can safely use the filesystem-free fast path.
	if len(deps.args) == 2 && deps.args[1] == "--version" && telemetryOptedOut(deps.environ) {
		if _, err := fmt.Fprintf(deps.stdout, "dws version %s\n", options.Version); err != nil {
			return &Error{Kind: ErrorDelegate, Op: "write version", Err: err}
		}
		return nil
	}
	if handled, err := trySchema(options, deps); handled {
		if err != nil {
			return &Error{Kind: ErrorDelegate, Op: "write Schema", Err: err}
		}
		return nil
	}
	if options.CoreSize <= 0 {
		return &Error{Kind: ErrorConfiguration, Op: "validate core size", Err: errors.New("core size must be positive")}
	}

	executable, err := deps.executable()
	if err != nil {
		return &Error{Kind: ErrorArtifact, Op: "locate launcher", Err: err}
	}
	canonical, err := deps.evalSymlinks(executable)
	if err != nil {
		return &Error{Kind: ErrorArtifact, Op: "resolve launcher", Err: err}
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return &Error{Kind: ErrorArtifact, Op: "canonicalize launcher", Err: err}
	}
	launcherInfo, err := deps.stat(canonical)
	if err != nil || !launcherInfo.Mode().IsRegular() {
		if err == nil {
			err = errors.New("launcher is not a regular file")
		}
		return &Error{Kind: ErrorArtifact, Op: "stat launcher", Err: err}
	}

	relative := options.CoreRelativePath
	if relative == "" {
		relative = filepath.Join("..", "libexec", coreExecutableName())
	}
	if filepath.IsAbs(relative) {
		return &Error{Kind: ErrorConfiguration, Op: "resolve core", Err: errors.New("core path must be relative")}
	}
	corePath := filepath.Clean(filepath.Join(filepath.Dir(canonical), relative))
	coreLstat, err := deps.lstat(corePath)
	if err != nil || !coreLstat.Mode().IsRegular() || coreLstat.Mode()&os.ModeSymlink != 0 {
		if err == nil {
			err = errors.New("core is not a regular non-symlink file")
		}
		return &Error{Kind: ErrorArtifact, Op: "stat core", Err: err}
	}
	core, err := deps.open(corePath)
	if err != nil {
		return &Error{Kind: ErrorArtifact, Op: "open core", Err: err}
	}
	defer core.Close()
	coreInfo, err := core.Stat()
	if err != nil || !coreInfo.Mode().IsRegular() || !os.SameFile(coreLstat, coreInfo) {
		if err == nil {
			err = errors.New("opened core does not match the non-symlink path")
		}
		return &Error{Kind: ErrorArtifact, Op: "resolve core", Err: err}
	}
	if coreInfo.Size() != options.CoreSize {
		return &Error{Kind: ErrorArtifact, Op: "validate core size", Err: fmt.Errorf("got %d, want %d", coreInfo.Size(), options.CoreSize)}
	}
	if os.SameFile(launcherInfo, coreInfo) {
		return &Error{Kind: ErrorArtifact, Op: "resolve core", Err: errors.New("core aliases launcher")}
	}
	if err := validateCoreExecutable(coreInfo); err != nil {
		return &Error{Kind: ErrorArtifact, Op: "validate core", Err: err}
	}
	expectedDigest, err := hex.DecodeString(options.CoreSHA256)
	if err != nil {
		return &Error{Kind: ErrorConfiguration, Op: "decode core digest", Err: err}
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, core); err != nil {
		return &Error{Kind: ErrorArtifact, Op: "hash core", Err: err}
	}
	afterHash, err := core.Stat()
	if err != nil {
		return &Error{Kind: ErrorArtifact, Op: "restat core after hashing", Err: err}
	}
	if !sameCoreState(coreInfo, afterHash) {
		return &Error{Kind: ErrorArtifact, Op: "validate core stability", Err: errors.New("core identity, size, mode, or modification time changed while hashing")}
	}
	if subtle.ConstantTimeCompare(hasher.Sum(nil), expectedDigest) != 1 {
		return &Error{Kind: ErrorArtifact, Op: "validate core SHA-256", Err: errors.New("core SHA-256 mismatch")}
	}
	cwd, err := deps.getwd()
	if err != nil {
		return &Error{Kind: ErrorDelegate, Op: "read working directory", Err: err}
	}
	environment := withInternalEnvironment(deps.environ, canonical, options.CoreSHA256, options.Version, environmentKeysCaseInsensitive())
	if err := validateCorePath(corePath, afterHash, deps); err != nil {
		return &Error{Kind: ErrorArtifact, Op: "revalidate core path", Err: err}
	}
	code, err := deps.delegate(corePath, deps.args, environment, cwd, deps.stdin, deps.stdout, deps.stderr)
	if err != nil {
		return &Error{Kind: ErrorDelegate, Op: "execute core", Err: err}
	}
	if code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

func sameCoreState(before, after os.FileInfo) bool {
	return os.SameFile(before, after) &&
		before.Size() == after.Size() &&
		before.Mode() == after.Mode() &&
		before.ModTime() == after.ModTime()
}

func telemetryOptedOut(environment []string) bool {
	for _, entry := range environment {
		key, value, present := strings.Cut(entry, "=")
		if !present {
			continue
		}
		if key == "DO_NOT_TRACK" || (environmentKeysCaseInsensitive() && strings.EqualFold(key, "DO_NOT_TRACK")) {
			return strings.TrimSpace(value) != ""
		}
	}
	return false
}

func validateCorePath(path string, opened os.FileInfo, deps dependencies) error {
	pathInfo, err := deps.lstat(path)
	if err != nil {
		return err
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("core path is not a regular non-symlink file")
	}
	resolvedInfo, err := deps.stat(path)
	if err != nil {
		return err
	}
	if !sameCoreState(opened, pathInfo) || !sameCoreState(opened, resolvedInfo) {
		return errors.New("core path changed after hashing")
	}
	// Delegation APIs reopen an executable by path. An active same-UID process
	// replacing that path after this check is outside the launcher's threat model.
	return nil
}

func validateIdentity(options Options) error {
	for name, value := range map[string]string{"version": options.Version, "commit": options.Commit, "edition": options.Edition} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("%s is empty or not canonical", name)
		}
	}
	if len(options.CoreSHA256) != 64 {
		return errors.New("core SHA-256 must contain 64 lowercase hexadecimal characters")
	}
	for _, character := range options.CoreSHA256 {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return errors.New("core SHA-256 must contain 64 lowercase hexadecimal characters")
		}
	}
	return nil
}

func withInternalEnvironment(environment []string, launcherPath, digest, version string, caseInsensitive bool) []string {
	result := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		match := func(expected string) bool { return key == expected }
		if caseInsensitive {
			match = func(expected string) bool { return strings.EqualFold(key, expected) }
		}
		if match(EnvLauncherPath) || match(EnvCoreDigest) || match(EnvCoreVersion) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, EnvLauncherPath+"="+launcherPath, EnvCoreDigest+"="+digest, EnvCoreVersion+"="+version)
}
