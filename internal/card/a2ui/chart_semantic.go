// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package a2ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type chartNode struct {
	component map[string]any
	pointer   string
}

type chartSurface struct {
	model map[string]any
	nodes map[string]chartNode
}

func chartTokens(path string) ([]string, bool) {
	if path == "/" {
		return []string{}, true
	}
	if !strings.HasPrefix(path, "/") {
		return nil, false
	}
	parts := strings.Split(path[1:], "/")
	for i, part := range parts {
		for j := 0; j < len(part); j++ {
			if part[j] == '~' && (j+1 >= len(part) || part[j+1] != '0' && part[j+1] != '1') {
				return nil, false
			}
			if part[j] == '~' {
				j++
			}
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, true
}

func chartArrayIndex(token string) (int, bool) {
	if token == "" || len(token) > 1 && token[0] == '0' {
		return 0, false
	}
	for i := 0; i < len(token); i++ {
		if token[i] < '0' || token[i] > '9' {
			return 0, false
		}
	}
	index, err := strconv.Atoi(token)
	return index, err == nil
}

func chartGet(model map[string]any, path string) (any, bool) {
	parts, ok := chartTokens(path)
	if !ok {
		return nil, false
	}
	var current any = model
	for _, part := range parts {
		switch value := current.(type) {
		case map[string]any:
			current, ok = value[part]
		case []any:
			index, valid := chartArrayIndex(part)
			if !valid || index >= len(value) {
				return nil, false
			}
			current = value[index]
			ok = true
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func chartSetValue(current any, parts []string, value any) (any, bool) {
	if len(parts) == 0 {
		return value, true
	}
	key := parts[0]
	switch container := current.(type) {
	case map[string]any:
		if len(parts) == 1 {
			if value == nil {
				delete(container, key)
			} else {
				container[key] = value
			}
			return container, true
		}
		next, exists := container[key]
		if !exists {
			next = map[string]any{}
		}
		updated, ok := chartSetValue(next, parts[1:], value)
		if !ok {
			return current, false
		}
		container[key] = updated
		return container, true
	case []any:
		leaf := len(parts) == 1
		if leaf && value != nil && key == "-" {
			return append(container, value), true
		}
		index, valid := chartArrayIndex(key)
		if !valid {
			return current, false
		}
		if leaf && value != nil && index == len(container) {
			return append(container, value), true
		}
		if index >= len(container) {
			return current, leaf && value == nil
		}
		if len(parts) == 1 {
			if value == nil {
				container = append(container[:index], container[index+1:]...)
			} else {
				container[index] = value
			}
			return container, true
		}
		updated, ok := chartSetValue(container[index], parts[1:], value)
		if !ok {
			return current, false
		}
		container[index] = updated
		return container, true
	default:
		return current, false
	}
}

// Bound the number of intermediate containers allocated during Chart replay.
const maxChartUpdatePathDepth = 2048

func chartSet(model map[string]any, path string, value any) (map[string]any, bool) {
	if strings.Count(path, "/") > maxChartUpdatePathDepth {
		return model, false
	}
	parts, ok := chartTokens(path)
	if !ok {
		return model, false
	}
	if len(parts) == 0 {
		root, ok := value.(map[string]any)
		if !ok {
			return model, false
		}
		return root, true
	}
	if model == nil {
		model = map[string]any{}
	}
	updated, ok := chartSetValue(model, parts, value)
	if !ok {
		return model, false
	}
	root, ok := updated.(map[string]any)
	return root, ok
}

func chartValue(value any) (float64, bool) {
	var number float64
	switch n := value.(type) {
	case json.Number:
		var err error
		number, err = n.Float64()
		if err != nil {
			return 0, false
		}
	case float64:
		number = n
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0) && math.Abs(number) <= 9007199254740991
}

func chartNumbers(values []any) bool {
	if len(values) == 0 || len(values) > 2000 {
		return false
	}
	for _, value := range values {
		if _, ok := chartValue(value); !ok {
			return false
		}
	}
	return true
}

func chartAxis(option map[string]any, key, kind string) []any {
	axis := object(option[key])
	if str(axis["type"]) != kind {
		return nil
	}
	if kind == "category" {
		values := array(axis["data"])
		for _, value := range values {
			if _, ok := value.(string); !ok {
				return nil
			}
		}
		return values
	}
	return []any{}
}

func chartGraphError(nodes map[string]bool, links []any) (string, string) {
	indegree, degree := map[string]int{}, map[string]int{}
	adjacency := map[string][]string{}
	minWeight, maxWeight := math.Inf(1), 0.0
	for _, link := range links {
		edge := object(link)
		source, target := str(edge["source"]), str(edge["target"])
		n, ok := chartValue(edge["value"])
		if !nodes[source] || !nodes[target] || source == target || !ok || n <= 0 {
			return "chart.endpoint_invalid", "Sankey link has an unknown endpoint or invalid value"
		}
		adjacency[source] = append(adjacency[source], target)
		indegree[target]++
		degree[source]++
		degree[target]++
		minWeight = math.Min(minWeight, n)
		maxWeight = math.Max(maxWeight, n)
	}
	if minWeight/maxWeight < 1e-12 {
		return "chart.range_invalid", "Sankey link weights span more than 12 orders of magnitude"
	}
	queue := []string{}
	for name := range nodes {
		if degree[name] == 0 {
			return "chart.endpoint_invalid", "Sankey has an isolated node"
		}
		if indegree[name] == 0 {
			queue = append(queue, name)
		}
	}
	for i := 0; i < len(queue); i++ {
		for _, target := range adjacency[queue[i]] {
			indegree[target]--
			if indegree[target] == 0 {
				queue = append(queue, target)
			}
		}
	}
	if len(queue) != len(nodes) {
		return "chart.endpoint_invalid", "Sankey contains a directed cycle"
	}
	return "", ""
}

func chartKnownType(kind string) bool {
	switch kind {
	case "bar", "line", "pie", "funnel", "scatter", "effectScatter", "heatmap", "candlestick", "radar", "gauge", "sankey":
		return true
	}
	return false
}

func chartHasUnknownType(option map[string]any) bool {
	for _, raw := range array(option["series"]) {
		if !chartKnownType(str(object(raw)["type"])) {
			return true
		}
	}
	return false
}

func chartOptionError(option map[string]any) (string, string) {
	if code, message := chartLocalError(option); code != "" {
		return code, message
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(option)
	if encoded.Len()-1 > 256*1024 {
		return "chart.option_size", "Chart option exceeds 256 KiB"
	}
	series := array(option["series"])
	if len(series) < 1 || len(series) > 32 {
		return "chart.series_count", "Chart requires 1–32 series"
	}
	unknown := chartHasUnknownType(option)
	types := map[string]bool{}
	total := 0
	for _, raw := range series {
		item := object(raw)
		kind := str(item["type"])
		if len(kind) == 0 || len(kind) > 32 || strings.IndexFunc(kind, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 {
			return "chart.series_type_unknown", "Invalid Chart series type name"
		}
		types[kind] = true
		points := array(item["data"])
		if len(points) < 1 || len(points) > 2000 {
			return "chart.series_data_invalid", "Every series requires 1–2000 points"
		}
		total += len(points)
		if unknown {
			total += len(array(item["links"]))
		}
		if total > 2000 {
			return "chart.series_data_invalid", "Chart exceeds 2000 points"
		}
		if unknown {
			continue // A registered template owns the shape; never certify it here.
		}
		switch kind {
		case "bar", "line":
			xCategory := chartAxis(option, "xAxis", "category")
			yCategory := chartAxis(option, "yAxis", "category")
			var axis []any
			switch {
			case xCategory != nil && yCategory == nil && chartAxis(option, "yAxis", "value") != nil:
				axis = xCategory
			case yCategory != nil && xCategory == nil && chartAxis(option, "xAxis", "value") != nil:
				axis = yCategory
			default:
				return "chart.axis_missing", "Bar and line need one category axis and one value axis"
			}
			if len(axis) == 0 || len(axis) > 80 {
				return "chart.axis_missing", "Category axis requires 1–80 labels"
			}
			if len(axis) != len(points) {
				return "chart.series_length", "Series length must match category axis"
			}
			if !chartNumbers(points) {
				return "chart.value_invalid", "Bar and line values must be finite numbers"
			}
		case "scatter", "effectScatter":
			x, y := object(option["xAxis"]), object(option["yAxis"])
			if x == nil || y == nil {
				return "chart.axis_missing", "Scatter needs single xAxis and yAxis objects"
			}
			xt, yt := "category", "value"
			if v, ok := x["type"]; ok {
				xt = str(v)
			}
			if v, ok := y["type"]; ok {
				yt = str(v)
			}
			if kind == "effectScatter" {
				for dimension, axis := range []map[string]any{x, y} {
					if (dimension == 0 && xt != "value") || (dimension == 1 && yt != "value") {
						continue
					}
					lo, hasLo := axis["min"]
					hi, hasHi := axis["max"]
					min, minOK := chartValue(lo)
					max, maxOK := chartValue(hi)
					if hasLo && !minOK || hasHi && !maxOK || hasLo && hasHi && min >= max {
						return "chart.range_invalid", "Invalid effectScatter axis range"
					}
					for _, point := range points {
						value := point
						if tuple, ok := point.([]any); ok {
							if len(tuple) <= dimension {
								return "chart.value_invalid", "Invalid scatter values"
							}
							value = tuple[dimension]
						}
						n, ok := chartValue(value)
						if !ok || hasLo && n < min || hasHi && n > max {
							return "chart.range_invalid", "EffectScatter axis excludes data"
						}
					}
				}
			}
			if xt == "value" && yt == "value" {
				for _, point := range points {
					tuple := array(point)
					if len(tuple) < 2 || !chartNumbers(tuple) {
						return "chart.value_invalid", "Scatter points require at least two numeric coordinates"
					}
				}
			} else {
				var axis map[string]any
				if xt == "category" && yt == "value" {
					axis = x
				} else if yt == "category" && xt == "value" {
					axis = y
				}
				labels := array(axis["data"])
				if len(labels) < 1 || len(labels) > 80 || len(labels) != len(points) {
					return "chart.axis_missing", "Scatter category axis must match its values"
				}
				for _, label := range labels {
					if _, ok := label.(string); !ok {
						return "chart.axis_missing", "Scatter category labels must be strings"
					}
				}
				if !chartNumbers(points) {
					return "chart.value_invalid", "Scatter category values must be finite numbers"
				}
			}
		case "pie", "funnel":
			min, max := 0.0, 100.0
			_, hasMax := item["max"]
			if kind == "funnel" {
				if v, exists := item["min"]; exists {
					var ok bool
					min, ok = chartValue(v)
					if !ok {
						return "chart.range_invalid", "Invalid funnel range"
					}
				}
				if v, exists := item["max"]; exists {
					var ok bool
					max, ok = chartValue(v)
					if !ok {
						return "chart.range_invalid", "Invalid funnel range"
					}
				}
				if min < 0 || hasMax && (max <= min || max <= 0) {
					return "chart.range_invalid", "Invalid funnel range"
				}
			}
			limit := 24
			if kind == "funnel" {
				limit = 12
			}
			if len(points) > limit {
				return "chart.series_data_invalid", "Pie and funnel item limit exceeded"
			}
			positive := false
			names := map[string]bool{}
			last := math.Inf(1)
			for _, point := range points {
				value := object(point)
				n, ok := chartValue(value["value"])
				name := str(value["name"])
				if !ok || n < 0 || name == "" || names[name] {
					return "chart.value_invalid", "Named values require a name and nonnegative finite number"
				}
				if kind == "funnel" && n > last {
					return "chart.series_order", "Funnel stages must be in descending order"
				}
				if kind == "funnel" && (n < min || hasMax && n > max) {
					return "chart.range_invalid", "Funnel value is outside min/max"
				}
				names[name] = true
				last = n
				positive = positive || n > 0
			}
			if !positive {
				return "chart.value_invalid", "At least one named value must be positive"
			}
		case "heatmap":
			x, y := chartAxis(option, "xAxis", "category"), chartAxis(option, "yAxis", "category")
			if x == nil || y == nil {
				return "chart.axis_missing", "Heatmap needs two category axes"
			}
			if len(x) == 0 || len(y) == 0 || len(x) > 80 || len(y) > 80 {
				return "chart.axis_missing", "Heatmap axes require 1–80 labels"
			}
			for _, point := range points {
				triple := array(point)
				if len(triple) != 3 {
					return "chart.value_invalid", "Heatmap points must have x, y, value"
				}
				a, aok := chartValue(triple[0])
				b, bok := chartValue(triple[1])
				_, cok := chartValue(triple[2])
				if !aok || !bok || !cok || a < 0 || b < 0 || a != math.Trunc(a) || b != math.Trunc(b) || a >= float64(len(x)) || b >= float64(len(y)) {
					return "chart.range_invalid", "Heatmap coordinate is outside the axes"
				}
			}
		case "candlestick":
			x := chartAxis(option, "xAxis", "category")
			y := object(option["yAxis"])
			yType, hasType := y["type"]
			if x == nil || y == nil || hasType && str(yType) != "value" {
				return "chart.axis_missing", "Candlestick needs a category xAxis and a value yAxis"
			}
			if len(x) == 0 || len(x) > 80 {
				return "chart.axis_missing", "Candlestick axis requires 1–80 labels"
			}
			if len(x) != len(points) {
				return "chart.series_length", "Candle count must match categories"
			}
			for _, point := range points {
				values := array(point)
				if len(values) != 4 || !chartNumbers(values) {
					return "chart.value_invalid", "Candle requires [open,close,low,high]"
				}
				o, _ := chartValue(values[0])
				c, _ := chartValue(values[1])
				lo, _ := chartValue(values[2])
				hi, _ := chartValue(values[3])
				if lo > math.Min(o, c) || hi < math.Max(o, c) {
					return "chart.range_invalid", "Candle high/low is inconsistent"
				}
			}
		case "radar":
			indicators := array(object(option["radar"])["indicator"])
			if len(indicators) < 3 || len(indicators) > 12 {
				return "chart.axis_missing", "Radar needs at least three indicators"
			}
			for _, group := range points {
				values := array(object(group)["value"])
				total += len(values)
				if total > 2000 {
					return "chart.series_data_invalid", "Chart exceeds 2000 points"
				}
				if len(values) != len(indicators) || !chartNumbers(values) {
					return "chart.series_length", "Radar values must match indicators"
				}
				for i, value := range values {
					if _, ok := object(indicators[i])["name"].(string); !ok {
						return "chart.value_invalid", "Radar indicator requires a name"
					}
					n, _ := chartValue(value)
					max, ok := chartValue(object(indicators[i])["max"])
					if !ok || max <= 0 || n < 0 || n > max {
						return "chart.range_invalid", "Radar value exceeds indicator bounds"
					}
				}
			}
		case "gauge":
			min, max, ok1, ok2 := 0.0, 100.0, true, true
			if value, exists := item["min"]; exists {
				min, ok1 = chartValue(value)
			}
			if value, exists := item["max"]; exists {
				max, ok2 = chartValue(value)
			}
			if !ok1 || !ok2 || min != 0 || max <= 0 {
				return "chart.range_invalid", "Gauge requires min=0 and positive max"
			}
			if len(points) > 5 {
				return "chart.series_count", "Gauge supports at most five measures"
			}
			if value, exists := item["splitNumber"]; exists {
				n, ok := chartValue(value)
				if !ok || n < 1 || n > 100 || math.Trunc(n) != n {
					return "chart.range_invalid", "Invalid gauge splitNumber"
				}
			}
			for _, point := range points {
				value := object(point)
				n, ok := chartValue(value["value"])
				if !ok || n < min || n > max {
					return "chart.range_invalid", "Gauge value is outside min/max"
				}
				if name, exists := value["name"]; exists {
					if _, ok := name.(string); !ok {
						return "chart.value_invalid", "Gauge name must be a string"
					}
				}
			}
		case "sankey":
			if orient, exists := item["orient"]; exists && str(orient) != "horizontal" && str(orient) != "vertical" {
				return "chart.range_invalid", "Invalid sankey orientation"
			}
			if len(points) > 64 {
				return "chart.series_data_invalid", "Sankey supports at most 64 nodes"
			}
			nodes := map[string]bool{}
			for _, point := range points {
				name := str(object(point)["name"])
				if name == "" || nodes[name] {
					return "chart.value_invalid", "Sankey nodes need unique names"
				}
				nodes[name] = true
			}
			links := array(item["links"])
			if len(links) == 0 || len(links) > 256 {
				return "chart.series_data_invalid", "Sankey requires 1–256 links"
			}
			if code, message := chartGraphError(nodes, links); code != "" {
				return code, message
			}
		}
	}
	if unknown {
		return "", ""
	}
	if len(types) > 1 && !(len(types) == 2 && types["bar"] && types["line"]) {
		return "chart.series_type_unknown", "Only bar+line mixed series are supported"
	}
	if len(types) == 1 && len(series) != 1 {
		for kind := range types {
			switch kind {
			case "pie", "funnel", "heatmap", "candlestick", "radar", "gauge", "sankey":
				return "chart.series_count", fmt.Sprintf("%s charts require exactly one series", kind)
			}
		}
	}
	return "", ""
}

func pureChartOption(option map[string]any) bool {
	if _, ok := option["series"]; !ok {
		return false
	}
	_, legacyType := option["type"]
	_, legacyData := option["data"]
	return !legacyType && !legacyData
}

func (p *Protocol) chartInstanceContexts(nodes map[string]chartNode) map[string]uint8 {
	type placement struct {
		id       string
		template bool
	}
	contexts := map[string]uint8{}
	pending := []placement{{"root", false}}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		node, exists := nodes[current.id]
		bit := uint8(1)
		if current.template {
			bit = 2
		}
		if !exists || contexts[current.id]&bit != 0 {
			continue
		}
		contexts[current.id] |= bit
		for _, raw := range p.assets.ChildRefs[str(node.component["component"])] {
			ref := object(raw)
			referenceSlots(node.component, strings.Split(str(ref["path"]), "."), "", func(value any, _ string) {
				if ref["kind"] == "id" {
					if child, ok := value.(string); ok {
						pending = append(pending, placement{child, current.template})
					}
				} else if ref["kind"] == "list" {
					switch v := value.(type) {
					case []any:
						for _, child := range v {
							if id, ok := child.(string); ok {
								pending = append(pending, placement{id, current.template})
							}
						}
					case map[string]any:
						if id, ok := v["componentId"].(string); ok {
							pending = append(pending, placement{id, true})
						}
					}
				}
			})
		}
	}
	return contexts
}

func (p *Protocol) checkChartBindings(docs []map[string]any, strict bool) ([]Diagnostic, map[string]any) {
	// Resolve final component types before replaying data. Chart validation must
	// not impose its update semantics on unrelated or replaced components.
	finalNodes := map[string]map[string]string{}
	for _, message := range docs {
		for _, op := range []string{"createSurface", "updateComponents"} {
			body, exists := message[op]
			if !exists {
				continue
			}
			value := object(body)
			sid := str(value["surfaceId"])
			if op == "createSurface" {
				finalNodes[sid] = map[string]string{}
			}
			if nodes, ok := finalNodes[sid]; ok {
				for _, raw := range array(value["components"]) {
					component := object(raw)
					nodes[str(component["id"])] = str(component["component"])
				}
			}
		}
	}
	chartSurfaces := map[string]bool{}
	for sid, nodes := range finalNodes {
		for _, kind := range nodes {
			if kind == "Chart" {
				chartSurfaces[sid] = true
			}
		}
	}
	states := map[string]*chartSurface{}
	diagnostics := []Diagnostic{}
	for i, message := range docs {
		for _, op := range []string{"createSurface", "updateDataModel", "updateComponents"} {
			body, exists := message[op]
			if !exists {
				continue
			}
			value := object(body)
			sid := str(value["surfaceId"])
			if !chartSurfaces[sid] {
				continue
			}
			if op == "createSurface" {
				states[sid] = &chartSurface{model: object(value["dataModel"]), nodes: map[string]chartNode{}}
			}
			state := states[sid]
			if state == nil {
				continue
			}
			if op == "updateDataModel" {
				var updated bool
				state.model, updated = chartSet(state.model, str(value["path"]), value["value"])
				if !updated {
					diagnostics = append(diagnostics, Diagnostic{"chart.data_model_update_unsupported", "error", fmt.Sprintf("/%d/updateDataModel/path", i), "Chart validation cannot apply this Data Model update path to the current container", ""})
				}
			}
			if op == "createSurface" || op == "updateComponents" {
				// A2UI component definitions are a flat message-level list; parent/child
				// relationships reference IDs, so every Chart definition appears here.
				for j, raw := range array(value["components"]) {
					component := object(raw)
					state.nodes[str(component["id"])] = chartNode{component, fmt.Sprintf("/%d/%s/components/%d", i, op, j)}
				}
			}
		}
	}
	checked, unverified := 0, 0
	surfaceIDs := make([]string, 0, len(states))
	for id := range states {
		surfaceIDs = append(surfaceIDs, id)
	}
	sort.Strings(surfaceIDs)
	for _, id := range surfaceIDs {
		state := states[id]
		contexts := p.chartInstanceContexts(state.nodes)
		nodeIDs := make([]string, 0, len(state.nodes))
		for nodeID := range state.nodes {
			nodeIDs = append(nodeIDs, nodeID)
		}
		sort.Strings(nodeIDs)
		for _, nodeID := range nodeIDs {
			node := state.nodes[nodeID]
			if str(node.component["component"]) != "Chart" {
				continue
			}
			if contexts[nodeID] != 1 {
				unverified++
				continue
			}
			ptr := node.pointer + "/data"
			data := node.component["data"]
			if binding, ok := data.(map[string]any); ok && len(binding) == 1 {
				if path, bound := binding["path"].(string); bound {
					if !strings.HasPrefix(path, "/") {
						unverified++
						continue
					}
					ptr += "/path"
					var found bool
					data, found = chartGet(state.model, path)
					if !found {
						diagnostics = append(diagnostics, Diagnostic{"chart.binding_missing", "error", ptr, "Chart binding has no value in the final Surface Data Model", ""})
						continue
					}
				}
			}
			option, ok := data.(map[string]any)
			if !ok {
				diagnostics = append(diagnostics, Diagnostic{"chart.binding_type", "error", ptr, "Chart data must resolve to an object", ""})
				continue
			}
			if !pureChartOption(option) {
				_, hasType := option["type"]
				_, hasData := option["data"]
				if hasType || hasData {
					if p.chartLegacySchema == nil {
						diagnostics = append(diagnostics, Diagnostic{"chart.legacy_structure_unavailable", "error", ptr, "The bundled protocol does not identify the legacy Chart schema", "Use a protocol bundle whose legacy Chart branch requires type and data"})
						continue
					}
					if err := p.chartLegacySchema.Validate(option); err != nil {
						legacyDiagnostics := p.diagnostics(err.(*jsonschema.ValidationError), option, ptr)
						if !strict {
							for i := range legacyDiagnostics {
								legacyDiagnostics[i].Severity = "warning"
								legacyDiagnostics[i].Hint = "Chart data remains unverified while preserving existing template content"
							}
							unverified++
						}
						diagnostics = append(diagnostics, legacyDiagnostics...)
						continue
					}
				}
				unverified++
				continue
			} // Legacy Chart branch remains Schema-only.
			checked++
			if !p.hasPureChartVariant {
				unverified++
				diagnostics = append(diagnostics, Diagnostic{"chart.option_structure_unavailable", "warning", ptr, "The bundled protocol lacks the pure-series Chart marker; offline Chart checks still run", "Use a protocol bundle with the x-aicard-variant pure-series marker"})
			}
			if code, msg := chartOptionError(option); code != "" {
				diagnostics = append(diagnostics, Diagnostic{code, "error", ptr, msg, ""})
			} else if chartHasUnknownType(option) {
				diagnostics = append(diagnostics, Diagnostic{"chart.series_type_unverified", "warning", ptr,
					"Chart contains an unknown series type and needs a client-registered template",
					"Check the series type spelling or verify the registered template on the client"})
				checked--
				unverified++
			}
		}
	}
	if !strict {
		downgraded := false
		for i := range diagnostics {
			if diagnostics[i].Severity == "error" && strings.HasPrefix(diagnostics[i].Code, "chart.") {
				diagnostics[i].Severity = "warning"
				diagnostics[i].Hint = "Explicit lint and chart build remain strict; preview and template save preserve existing content"
				downgraded = true
			}
		}
		if downgraded {
			unverified++
		}
	}
	valid := true
	for _, d := range diagnostics {
		if d.Severity == "error" {
			valid = false
		}
	}
	status := "not_applicable"
	if !valid {
		status = "invalid"
	} else if unverified > 0 {
		status = "unverified"
	} else if checked > 0 {
		status = "valid"
	}
	return diagnostics, map[string]any{"valid": valid, "checked": checked, "unverified": unverified, "status": status}
}
