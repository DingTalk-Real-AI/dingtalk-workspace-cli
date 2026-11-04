// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/publishedmcp"
	"github.com/spf13/cobra"
)

const aicardTemplateMaxBytes = 768 << 10
const aicardTemplateDigestAlgorithm = "sha256:json-sorted-number-preserving-v1"
const aicardTemplateVersionWarning = "The service may return latest content for an absent version and does not report the actual version. Verify an approved content digest before strict use."

// Test seams for file-system and encoding failures that cannot be produced
// deterministically on every native platform.
var (
	aicardTemplateOpen      = os.Open
	aicardTemplateReadAll   = io.ReadAll
	aicardTemplateMarshal   = jsonutil.Marshal
	aicardTemplateAbs       = filepath.Abs
	aicardTemplateStatPath  = os.Stat
	aicardTemplateStatFile  = (*os.File).Stat
	aicardTemplateWriteFile = (*os.File).Write
)

func aicardTemplateService(cmd *cobra.Command) string {
	return corecmd.EffectiveValue(cmd, aicardTemplateServiceFlag())
}

func aicardTemplateVersion(value string) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0
}

func readAicardTemplateFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, apperrors.NewValidation("Cannot inspect template file", apperrors.WithCause(err))
	}
	if !info.Mode().IsRegular() || info.Size() > aicardTemplateMaxBytes {
		return nil, apperrors.NewValidation("Template input must be a regular file of at most 768 KiB")
	}
	file, err := aicardTemplateOpen(path)
	if err != nil {
		return nil, apperrors.NewValidation("Cannot open template file", apperrors.WithCause(err))
	}
	defer file.Close()
	data, err := aicardTemplateReadAll(io.LimitReader(file, aicardTemplateMaxBytes+1))
	if err != nil {
		return nil, apperrors.NewValidation("Cannot read template file", apperrors.WithCause(err))
	}
	if len(data) > aicardTemplateMaxBytes {
		return nil, apperrors.NewValidation("Template input must be at most 768 KiB, including a leading UTF-8 BOM")
	}
	return bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}), nil
}

// aicardTemplateWire prepares a validated template for the MCP string argument.
// The transport rejects carriage returns and format characters such as the
// zero-width joiner inside emoji sequences, although both are legal JSON. Compact
// output drops insignificant whitespace, including CR from CRLF files, and the
// remaining format and control code points can only occur inside strings, where
// a \u escape keeps the same value. Key order, number lexemes and string values
// are preserved, so the content digest does not change.
func aicardTemplateWire(data []byte) string {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var compact bytes.Buffer
	// validateAicardTemplate has already accepted data as exactly one JSON value.
	_ = json.Compact(&compact, data)
	var wire strings.Builder
	for _, r := range compact.String() {
		if !aicardTemplateEscapedRune(r) {
			wire.WriteRune(r)
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&wire, `\u%04x`, unit)
		}
	}
	return wire.String()
}

func aicardTemplateEscapedRune(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r)
}

func validateAicardTemplate(data []byte) (string, error) {
	if len(data) > aicardTemplateMaxBytes {
		return "", apperrors.NewValidation("Template must contain UTF-8 JSON text of at most 768 KiB, including a leading BOM")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if len(data) == 0 || !utf8.Valid(data) {
		return "", apperrors.NewValidation("Template must contain UTF-8 JSON text of at most 768 KiB")
	}
	if !json.Valid(data) {
		return "", apperrors.NewValidation("Template must contain exactly one JSON value")
	}
	if !aicardTemplateUnicodeValid(data) {
		return "", apperrors.NewValidation("Template JSON strings and keys must not contain unpaired UTF-16 surrogates", apperrors.WithReason("invalid_template_unicode"))
	}
	if err := jsonutil.RejectDuplicateObjectKeys(data); err != nil {
		return "", apperrors.NewValidation("Template must be one JSON value without duplicate object keys", apperrors.WithCause(err))
	}
	var messages []any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&messages); err != nil || len(messages) == 0 {
		return "", apperrors.NewValidation("Template must be a nonempty A2UI message array")
	}
	p, err := aicardLoadProtocol()
	if err != nil {
		return "", apperrors.NewInternal("Cannot load the embedded A2UI protocol", apperrors.WithCause(err))
	}
	report, err := aicardLint(p, data, false, true)
	if err != nil {
		return "", apperrors.NewInternal("Cannot validate the template", apperrors.WithCause(err))
	}
	if !report.Valid {
		return "", apperrors.NewValidation("Template violates the A2UI protocol", apperrors.WithDetails(map[string]any{"diagnostics": report.Diagnostics}))
	}
	preflight, err := aicardPreflight(p, report.A2UIMessages, "new-card")
	if err != nil {
		return "", apperrors.NewValidation("Template initialization is invalid", apperrors.WithCause(err))
	}
	if preflight["valid"] != true {
		return "", apperrors.NewValidation("Template must fully initialize a new card", apperrors.WithDetails(preflight))
	}
	// This is a versioned content-normalization algorithm, not RFC 8785.
	// Sort object keys, retain array order and exact JSON number lexemes, and
	// normalize string escaping without rounding business integers through floats.
	canonical, err := aicardTemplateMarshal(messages)
	if err != nil {
		return "", apperrors.NewInternal("Cannot normalize template content", apperrors.WithCause(err))
	}
	digest := sha256.Sum256(canonical)
	return aicardTemplateDigestAlgorithm + ":" + hex.EncodeToString(digest[:]), nil
}

func aicardTemplateInvoke(cmd *cobra.Command, service, tool string, args map[string]any) (map[string]any, publishedmcp.ValidatedInvocationResult, error) {
	var empty publishedmcp.ValidatedInvocationResult
	caller, ok := GetCaller().(AicardTemplateCaller)
	if !ok {
		return nil, empty, apperrors.NewInternal("The DWS published-template caller is unavailable; install a compatible AICard module")
	}
	invocation, err := caller.InvokeAicardTemplate(cmd.Context(), service, tool, args)
	if err != nil {
		var classified *apperrors.Error
		if errors.As(err, &classified) {
			return nil, empty, err
		}
		return nil, empty, aicardTemplateUnconfirmed(tool, err)
	}
	if invocation.Result.IsError {
		return nil, empty, aicardTemplateServiceError(service, tool, invocation, nil)
	}
	payload := invocation.Result.StructuredContent
	if payload == nil && len(invocation.Result.Blocks) == 0 {
		payload = invocation.Result.Content
	}
	if payload == nil {
		for _, block := range invocation.Result.Blocks {
			if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
				continue
			}
			if payload != nil {
				return nil, empty, aicardTemplateUnconfirmed(tool, fmt.Errorf("ambiguous result content"))
			}
			if err := jsonutil.RejectDuplicateObjectKeys([]byte(block.Text)); err != nil {
				return nil, empty, aicardTemplateUnconfirmed(tool, err)
			}
			if err := json.Unmarshal([]byte(block.Text), &payload); err != nil {
				return nil, empty, aicardTemplateUnconfirmed(tool, err)
			}
		}
	}
	if success, explicit := payload["success"].(bool); !explicit || !success {
		if explicit {
			return nil, empty, aicardTemplateServiceError(service, tool, invocation, payload)
		}
		return nil, empty, aicardTemplateUnconfirmed(tool, fmt.Errorf("missing explicit success"), aicardTemplateServiceDetails(service, tool, invocation, payload))
	}
	return payload, invocation, nil
}

func aicardTemplateUnconfirmed(tool string, cause error, diagnostics ...map[string]any) error {
	details := map[string]any{"resultUnknown": tool != "get_aicard_template", "retryAutomatically": false}
	for _, extra := range diagnostics {
		for key, value := range extra {
			details[key] = value
		}
	}
	return apperrors.NewAPI("The template operation returned no confirmable result; investigate before retrying",
		apperrors.WithReason("aicard_template_result_unconfirmed"), apperrors.WithRetryable(false),
		apperrors.WithDetails(details), apperrors.WithCause(cause))
}

func aicardTemplateEvidence(result map[string]any, service string, invocation publishedmcp.ValidatedInvocationResult) {
	result["mcpId"] = service
	result["inputSchemaValidation"] = invocation.InputSchemaValidation
	result["inputSchemaDigest"] = invocation.InputSchemaDigest
}

func runAicardTemplateCreate(cmd *cobra.Command, _ []string) error {
	return runAicardTemplateWrite(cmd, "create_aicard_template", "")
}

func runAicardTemplateUpdate(cmd *cobra.Command, _ []string) error {
	id, _ := cmd.Flags().GetString("template-id")
	return runAicardTemplateWrite(cmd, "update_aicard_template", strings.TrimSpace(id))
}

func runAicardTemplateWrite(cmd *cobra.Command, tool, id string) error {
	service := aicardTemplateService(cmd)
	path, _ := cmd.Flags().GetString("file")
	data, err := readAicardTemplateFile(strings.TrimSpace(path))
	if err != nil {
		return err
	}
	digest, err := validateAicardTemplate(data)
	if err != nil {
		return err
	}
	wire := aicardTemplateWire(data)
	if len(wire) > aicardTemplateMaxBytes {
		return apperrors.NewValidation("The transport-safe template exceeds 768 KiB after JSON escaping")
	}
	result := map[string]any{"executed": false, "templateDigest": digest, "digestAlgorithm": aicardTemplateDigestAlgorithm}
	if id != "" {
		result["templateId"] = id
	}
	if commandDryRun(cmd) {
		return output.StoreResult(cmd.Context(), output.Success(result, output.WithDryRun()))
	}
	args := map[string]any{"file": wire}
	if id != "" {
		args["templateId"] = id
	}
	payload, invocation, err := aicardTemplateInvoke(cmd, service, tool, args)
	if err != nil {
		return err
	}
	returnedID, _ := payload["templateId"].(string)
	version, _ := payload["version"].(string)
	if strings.TrimSpace(returnedID) == "" || returnedID != strings.TrimSpace(returnedID) || !aicardTemplateVersion(version) || id != "" && returnedID != id {
		return aicardTemplateUnconfirmed(tool, fmt.Errorf("invalid template identity or version"))
	}
	result["executed"], result["templateId"], result["version"] = true, returnedID, version
	aicardTemplateEvidence(result, service, invocation)
	return output.StoreResult(cmd.Context(), output.Success(result))
}

func runAicardTemplateGet(cmd *cobra.Command, _ []string) error {
	service := aicardTemplateService(cmd)
	id, _ := cmd.Flags().GetString("template-id")
	id = strings.TrimSpace(id)
	version, _ := cmd.Flags().GetString("version")
	version = strings.TrimSpace(version)
	if !aicardTemplateVersion(version) {
		return apperrors.NewValidation("--version must be a positive decimal int64 string without leading zeros")
	}
	out, _ := cmd.Flags().GetString("out")
	path, err := aicardTemplateAbs(strings.TrimSpace(out))
	if err != nil {
		return apperrors.NewValidation("Cannot resolve --out", apperrors.WithCause(err))
	}
	commandOutput, err := aicardTemplateCommandOutput(cmd)
	if err != nil {
		return err
	}
	if aicardTemplateOutputConflict(path, commandOutput) {
		return apperrors.NewValidation(aicardTemplateOutputConflictMessage)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return apperrors.NewValidation("--out must be a new path; existing files and symlinks are never overwritten", apperrors.WithCause(err))
	}
	result := map[string]any{"executed": false, "templateId": id, "requestedVersion": version,
		"versionConfirmed": false, "filePath": path, "versionWarning": aicardTemplateVersionWarning}
	if commandDryRun(cmd) {
		return output.StoreResult(cmd.Context(), output.Success(result, output.WithDryRun()))
	}
	payload, invocation, err := aicardTemplateInvoke(cmd, service, "get_aicard_template", map[string]any{"templateId": id, "version": version})
	if err != nil {
		return err
	}
	file, ok := payload["file"].(string)
	if !ok {
		return aicardTemplateUnconfirmed("get_aicard_template", fmt.Errorf("missing file string"))
	}
	digest, err := validateAicardTemplate([]byte(file))
	if err != nil {
		return apperrors.NewAPI("The retrieved template is not a valid complete A2UI file", apperrors.WithCause(err), apperrors.WithRetryable(false))
	}
	file = strings.TrimPrefix(file, "\ufeff")
	if err := writeAicardTemplateFile(path, []byte(file), commandOutput); err != nil {
		if errors.Is(err, errAicardTemplateOutputConflict) {
			return apperrors.NewValidation(aicardTemplateOutputConflictMessage)
		}
		return apperrors.NewValidation("Cannot save template to --out; no existing file was overwritten", apperrors.WithCause(err))
	}
	result["executed"], result["templateDigest"], result["digestAlgorithm"] = true, digest, aicardTemplateDigestAlgorithm
	aicardTemplateEvidence(result, service, invocation)
	return output.StoreResult(cmd.Context(), output.Success(result))
}

func writeAicardTemplateFile(path string, data []byte, commandOutput string) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	// Compare the created file's identity too: case-insensitive filesystems can
	// alias distinct basenames even when both destinations were initially absent.
	if commandOutput != "" {
		created, statErr := aicardTemplateStatFile(file)
		if statErr != nil {
			return statErr
		}
		other, statErr := aicardTemplateStatPath(commandOutput)
		if statErr == nil && os.SameFile(created, other) {
			return errAicardTemplateOutputConflict
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	if _, err = aicardTemplateWriteFile(file, data); err != nil {
		return err
	}
	return file.Close()
}
