package cardmentions

import (
	"fmt"
	"sort"
	"strings"
)

type mentionBody struct {
	message          int
	pointer, content string
}
type mentionBinding struct{ surface, path string }

// mentionBodies resolves statically available Markdown data paths. Bindings
// that become available only after creation stay a runtime concern.
func mentionBodies(messages []string) ([]mentionBody, bool) {
	bodies := []mentionBody{}
	bindings := []mentionBinding{}
	data := map[string]map[string]mentionBody{}
	for i, raw := range messages {
		value, err := decode([]byte(raw))
		if err != nil {
			continue
		}
		document := object(value)
		for _, op := range []string{"createSurface", "updateDataModel", "updateComponents"} {
			node := object(document[op])
			sid := str(node["surfaceId"])
			if data[sid] == nil {
				data[sid] = map[string]mentionBody{}
			}
			source := mentionBody{message: i, pointer: fmt.Sprintf("/%s", op)}
			if op == "createSurface" {
				collectMentionData(data[sid], "", node["dataModel"], source.pointer+"/dataModel", i)
			}
			if op == "updateDataModel" {
				path := str(node["path"])
				if path == "/" {
					path = ""
				}
				for key := range data[sid] {
					if key == path || strings.HasPrefix(key, path+"/") {
						delete(data[sid], key)
					}
				}
				collectMentionData(data[sid], path, node["value"], source.pointer+"/value", i)
			}
			walkMentionJSON(node, source.pointer, func(n map[string]any, ptr string) {
				if str(n["component"]) != "Markdown" {
					return
				}
				if literal, ok := n["content"].(string); ok {
					bodies = append(bodies, mentionBody{i, ptr + "/content", literal})
				} else if path := str(object(n["content"])["path"]); validMentionPath(path) {
					bindings = append(bindings, mentionBinding{sid, path})
				}
			})
		}
	}
	for _, binding := range bindings {
		if body, ok := data[binding.surface][binding.path]; ok {
			bodies = append(bodies, body)
		}
	}
	return bodies, len(bindings) > 0
}

func collectMentionData(data map[string]mentionBody, path string, value any, ptr string, message int) {
	switch node := value.(type) {
	case string:
		data[path] = mentionBody{message, ptr, node}
	case map[string]any:
		for key, child := range node {
			collectMentionData(data, path+pointer([]string{key}), child, ptr+pointer([]string{key}), message)
		}
	case []any:
		for i, child := range node {
			suffix := fmt.Sprintf("/%d", i)
			collectMentionData(data, path+suffix, child, ptr+suffix, message)
		}
	}
}

func walkMentionJSON(value any, ptr string, visit func(map[string]any, string)) {
	switch node := value.(type) {
	case map[string]any:
		visit(node, ptr)
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			walkMentionJSON(node[key], ptr+pointer([]string{key}), visit)
		}
	case []any:
		for i, child := range node {
			walkMentionJSON(child, fmt.Sprintf("%s/%d", ptr, i), visit)
		}
	}
}

func validMentionPath(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	for i := 0; i < len(path); i++ {
		if path[i] != '~' {
			continue
		}
		if i+1 == len(path) || path[i+1] != '0' && path[i+1] != '1' {
			return false
		}
		i++
	}
	return true
}
