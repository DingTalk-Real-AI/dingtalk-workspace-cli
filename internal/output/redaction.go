// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package output

import (
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
)

const redactedValue = "[REDACTED]"

var (
	urlCredentialsPattern = regexp.MustCompile(`(?i)\b(https?://)[^/@\s]+@`)
	urlQueryValuePattern  = regexp.MustCompile(`(?i)[?&][^=&#\s]+=[^&#\s"'<>]+`)
	headerSecretPattern   = regexp.MustCompile(`(?im)\b(authorization|cookie|set-cookie)(\s*:\s*)[^\r\n]+`)
	jsonSecretPattern     = regexp.MustCompile(`(?i)("(?:authorization|token|access_token|refresh_token|client_secret|cookie|set-cookie|password)"\s*:\s*)("(?:\\.|[^"\\])*"|[^,}\s]+)`)
	assignedSecretPattern = regexp.MustCompile(`(?i)\b(authorization|token|access_token|refresh_token|client_secret|cookie|set-cookie|password)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&]+)`)
)

// redactEnvelope is the single pre-render boundary for framework-owned
// diagnostics. Business data remains authoritative: commands such as explicit
// credential reads intentionally return secrets and declare those fields in
// ToolSpec.result.sensitive_paths. Error, operation, and notice channels are
// never intentional secret-delivery surfaces and are always redacted.
func redactEnvelope(source *Envelope) *Envelope {
	if source == nil {
		return nil
	}
	out := cloneEnvelope(*source)
	seen := make(map[redactVisit]struct{})
	redactReflectValue(reflect.ValueOf(&out.Meta), "meta", seen)
	redactReflectValue(reflect.ValueOf(&out.Error), "error", seen)
	redactReflectValue(reflect.ValueOf(&out.Notice), "_notice", seen)
	return &out
}

type redactVisit struct {
	typ reflect.Type
	ptr uintptr
}

func redactReflectValue(value reflect.Value, jsonKey string, seen map[redactVisit]struct{}) {
	if !value.IsValid() {
		return
	}
	if isSensitiveOutputKey(jsonKey) && setRedactedValue(value) {
		return
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return
		}
		copy := reflect.New(value.Elem().Type()).Elem()
		copy.Set(value.Elem())
		redactReflectValue(copy, jsonKey, seen)
		value.Set(copy)
	case reflect.Pointer:
		if value.IsNil() {
			return
		}
		visit := redactVisit{typ: value.Type(), ptr: value.Pointer()}
		if _, ok := seen[visit]; ok {
			return
		}
		seen[visit] = struct{}{}
		redactReflectValue(value.Elem(), jsonKey, seen)
	case reflect.Map:
		if value.IsNil() {
			return
		}
		visit := redactVisit{typ: value.Type(), ptr: value.Pointer()}
		if _, ok := seen[visit]; ok {
			return
		}
		seen[visit] = struct{}{}
		iter := value.MapRange()
		for iter.Next() {
			key, item := iter.Key(), iter.Value()
			copy := reflect.New(item.Type()).Elem()
			copy.Set(item)
			name := ""
			if key.Kind() == reflect.String {
				name = key.String()
			}
			redactReflectValue(copy, name, seen)
			value.SetMapIndex(key, copy)
		}
	case reflect.Slice:
		if value.IsNil() {
			return
		}
		visit := redactVisit{typ: value.Type(), ptr: value.Pointer()}
		if _, ok := seen[visit]; ok {
			return
		}
		seen[visit] = struct{}{}
		for i := 0; i < value.Len(); i++ {
			redactReflectValue(value.Index(i), "", seen)
		}
	case reflect.Array:
		for i := 0; i < value.Len(); i++ {
			redactReflectValue(value.Index(i), "", seen)
		}
	case reflect.Struct:
		typ := value.Type()
		for i := 0; i < value.NumField(); i++ {
			fieldInfo := typ.Field(i)
			if fieldInfo.PkgPath != "" || fieldInfo.Tag.Get("json") == "-" {
				continue
			}
			name := strings.Split(fieldInfo.Tag.Get("json"), ",")[0]
			if name == "" {
				name = fieldInfo.Name
			}
			redactReflectValue(value.Field(i), name, seen)
		}
	case reflect.String:
		if value.CanSet() {
			value.SetString(redactRecognizableSecrets(value.String()))
		}
	}
}

func setRedactedValue(value reflect.Value) bool {
	if !value.CanSet() {
		return false
	}
	switch value.Kind() {
	case reflect.String:
		value.SetString(redactedValue)
		return true
	case reflect.Interface:
		replacement := reflect.ValueOf(redactedValue)
		if replacement.Type().AssignableTo(value.Type()) || replacement.Type().Implements(value.Type()) {
			value.Set(replacement)
			return true
		}
	case reflect.Pointer:
		replacement := reflect.New(value.Type().Elem())
		if setRedactedValue(replacement.Elem()) {
			value.Set(replacement)
			return true
		}
	}
	return false
}

// isSensitiveOutputKey reports whether a diagnostics-channel key holds a
// credential. It reuses logging.IsSensitiveKey semantics (case-insensitive;
// exact snake/kebab names such as api_key / client-secret, plus any key
// containing secret/token/credential/password, which also covers camelCase
// variants like clientSecret) so error details, RPC data, and notices follow
// the same sensitive-key boundary as the logging pipeline. The header-only
// "set-cookie" variant is not in the logging list and stays covered here.
func isSensitiveOutputKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "set-cookie":
		// Header-only variant the logging list does not cover.
		return true
	case "next_token":
		// meta.pagination.next_token is the resumable-pagination handle an
		// Agent must read (§3 two-state semantics): it is an opaque cursor,
		// not a credential, and stays visible across every channel.
		return false
	}
	return logging.IsSensitiveKey(normalized)
}

func redactRecognizableSecrets(text string) string {
	text = urlCredentialsPattern.ReplaceAllString(text, `${1}`+redactedValue+`@`)
	text = urlQueryValuePattern.ReplaceAllStringFunc(text, func(parameter string) string {
		equals := strings.IndexByte(parameter, '=')
		key, err := url.QueryUnescape(parameter[1:equals])
		if err != nil || !isSensitiveQueryKey(key) {
			return parameter
		}
		return parameter[:equals+1] + redactedValue
	})
	text = headerSecretPattern.ReplaceAllString(text, `${1}${2}`+redactedValue)
	text = jsonSecretPattern.ReplaceAllString(text, `${1}"`+redactedValue+`"`)
	return assignedSecretPattern.ReplaceAllString(text, `${1}${2}`+redactedValue)
}

func isSensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return isSensitiveOutputKey(normalized) || strings.Contains(normalized, "signature")
}
