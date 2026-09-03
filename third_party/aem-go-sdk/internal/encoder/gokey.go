package encoder

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func EncodeURIComponent(s string) string {
	result := url.QueryEscape(s)
	result = strings.ReplaceAll(result, "+", "%20")
	result = strings.ReplaceAll(result, "%21", "!")
	result = strings.ReplaceAll(result, "%27", "'")
	result = strings.ReplaceAll(result, "%28", "(")
	result = strings.ReplaceAll(result, "%29", ")")
	result = strings.ReplaceAll(result, "%2A", "*")
	return result
}

func ItemToString(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return "", false
		}
		return val, true
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val)), true
		}
		return fmt.Sprintf("%g", val), true
	case int:
		return fmt.Sprintf("%d", val), true
	case int64:
		return fmt.Sprintf("%d", val), true
	case bool:
		if val {
			return "true", true
		}
		return "false", true
	case json.Number:
		return val.String(), true
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(val)
		if err != nil {
			return "", false
		}
		return string(b), true
	default:
		return "", false
	}
}

func ObjToQS(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(m))
	for _, k := range keys {
		v := m[k]
		if v == "" {
			continue
		}
		parts = append(parts, k+"="+EncodeURIComponent(v))
	}
	return strings.Join(parts, "&")
}

func ToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string, len(data))
	for k, v := range data {
		if s, ok := ItemToString(v); ok {
			result[k] = s
		}
	}
	return result
}

func ProcessData(logs []map[string]string, config map[string]string) string {
	configQS := ObjToQS(config)

	logParts := make([]string, 0, len(logs))
	for _, log := range logs {
		logParts = append(logParts, ObjToQS(log))
	}
	logsJoined := strings.Join(logParts, "|")

	return configQS + "&msg=" + EncodeURIComponent(logsJoined)
}
