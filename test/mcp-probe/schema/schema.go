// Package schema parses the canonical schema emitted by `dws schema --json`
// and provides helpers for building schema-driven MCP probe invocations.
package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Catalog struct {
	Products []Product `json:"products"`
}

type Product struct {
	ID          string `json:"id"`
	Command     string `json:"command,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	ToolCount   int    `json:"tool_count,omitempty"`
	Tools       []Tool `json:"tools,omitempty"`
}

type ProductSchema struct {
	Path    string  `json:"path"`
	Product Product `json:"product"`
	Tools   []Tool  `json:"tools"`
	Count   int     `json:"count,omitempty"`
}

type FixtureCatalog struct {
	Products []FixtureProduct `json:"products"`
}

type FixtureProduct struct {
	ID          string             `json:"id"`
	DisplayName string             `json:"display_name,omitempty"`
	Description string             `json:"description,omitempty"`
	ServerKey   string             `json:"server_key"`
	Endpoint    string             `json:"endpoint,omitempty"`
	CLI         *FixtureProductCLI `json:"cli,omitempty"`
	Tools       []FixtureTool      `json:"tools"`
}

type FixtureProductCLI struct {
	Command string `json:"command,omitempty"`
}

type FixtureTool struct {
	RPCName         string              `json:"rpc_name"`
	CLIName         string              `json:"cli_name,omitempty"`
	Title           string              `json:"title,omitempty"`
	Description     string              `json:"description,omitempty"`
	InputSchema     map[string]any      `json:"input_schema,omitempty"`
	Sensitive       bool                `json:"sensitive"`
	Group           string              `json:"group,omitempty"`
	FlagHints       map[string]FlagHint `json:"flag_hints,omitempty"`
	SourceServerKey string              `json:"source_server_key"`
	CanonicalPath   string              `json:"canonical_path"`
}

type FlagHint struct {
	Shorthand     string         `json:"shorthand,omitempty"`
	Alias         string         `json:"alias,omitempty"`
	Transform     string         `json:"transform,omitempty"`
	TransformArgs map[string]any `json:"transform_args,omitempty"`
	EnvDefault    string         `json:"env_default,omitempty"`
	Default       any            `json:"default,omitempty"`
	Hidden        bool           `json:"hidden,omitempty"`
	Required      bool           `json:"required,omitempty"`
}

type Tool struct {
	RPCName       string              `json:"rpc_name"`
	CLIName       string              `json:"cli_name,omitempty"`
	Title         string              `json:"title,omitempty"`
	Description   string              `json:"description,omitempty"`
	Sensitive     bool                `json:"sensitive,omitempty"`
	CanonicalPath string              `json:"canonical_path,omitempty"`
	CLIPath       []string            `json:"cli_path,omitempty"`
	Required      []string            `json:"required,omitempty"`
	Flags         []Flag              `json:"flags,omitempty"`
	FlagHints     map[string]FlagHint `json:"flag_hints,omitempty"`
	InputSchema   map[string]any      `json:"input_schema,omitempty"`
}

type ToolSchema struct {
	Path        string              `json:"path"`
	Product     Product             `json:"product"`
	Tool        Tool                `json:"tool"`
	CLIPath     []string            `json:"cli_path"`
	Required    []string            `json:"required"`
	Flags       []Flag              `json:"flags"`
	FlagHints   map[string]FlagHint `json:"flag_hints,omitempty"`
	InputSchema map[string]any      `json:"input_schema"`
}

type Flag struct {
	PropertyName string `json:"property_name"`
	FlagName     string `json:"flag_name"`
	Alias        string `json:"alias,omitempty"`
	Kind         string `json:"kind"`
	Description  string `json:"description,omitempty"`
}

func ParseCatalog(jsonBytes []byte) (Catalog, error) {
	var catalog Catalog
	if err := json.Unmarshal(jsonBytes, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("parse catalog JSON: %w", err)
	}
	return catalog, nil
}

func ParseProduct(jsonBytes []byte) (ProductSchema, error) {
	var product ProductSchema
	if err := json.Unmarshal(jsonBytes, &product); err != nil {
		return ProductSchema{}, fmt.Errorf("parse product schema JSON: %w", err)
	}
	return product, nil
}

func ParseTool(jsonBytes []byte) (ToolSchema, error) {
	var tool ToolSchema
	if err := json.Unmarshal(jsonBytes, &tool); err != nil {
		return ToolSchema{}, fmt.Errorf("parse tool schema JSON: %w", err)
	}
	if len(tool.CLIPath) == 0 {
		tool.CLIPath = append(tool.CLIPath, tool.Tool.CLIPath...)
	}
	if len(tool.Flags) == 0 {
		tool.Flags = append(tool.Flags, tool.Tool.Flags...)
	}
	if len(tool.FlagHints) == 0 {
		tool.FlagHints = cloneFlagHints(tool.Tool.FlagHints)
	}
	if len(tool.Required) == 0 {
		tool.Required = append(tool.Required, tool.Tool.Required...)
	}
	if len(tool.InputSchema) == 0 {
		tool.InputSchema = tool.Tool.InputSchema
	}
	return tool, nil
}

func (c *Catalog) FindProduct(productID string) (*Product, error) {
	for i := range c.Products {
		if c.Products[i].ID == productID {
			return &c.Products[i], nil
		}
	}
	return nil, fmt.Errorf("product %q not found in catalog", productID)
}

func (p *Product) FindTool(name string) (*Tool, error) {
	for i := range p.Tools {
		if p.Tools[i].CanonicalPath == name || p.Tools[i].RPCName == name || p.Tools[i].CLIName == name {
			return &p.Tools[i], nil
		}
	}
	return nil, fmt.Errorf("tool %q not found in product", name)
}

func (c *Catalog) WithProxyEndpoint(proxyURL string) Catalog {
	products := make([]Product, len(c.Products))
	copy(products, c.Products)
	for i := range products {
		products[i].Endpoint = proxyURL
	}
	return Catalog{Products: products}
}

func (c FixtureCatalog) WithProxyEndpoint(proxyURL string) FixtureCatalog {
	products := make([]FixtureProduct, len(c.Products))
	copy(products, c.Products)
	for i := range products {
		products[i].Endpoint = proxyURL
	}
	return FixtureCatalog{Products: products}
}

func (t ToolSchema) GenerateArguments() (map[string]any, error) {
	if len(t.Flags) == 0 {
		return map[string]any{}, nil
	}

	args := make(map[string]any, len(t.Flags))
	for _, flag := range t.Flags {
		property := strings.TrimSpace(flag.PropertyName)
		if property == "" {
			continue
		}
		propertySchema := schemaForProperty(t.InputSchema, property)
		if len(propertySchema) == 0 {
			continue
		}
		value, err := syntheticCLIValue(property, propertySchema, t.FlagHints[property])
		if err != nil {
			return nil, fmt.Errorf("generate value for %s: %w", property, err)
		}
		args[property] = value
	}
	return args, nil
}

func NormalizeArguments(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	normalized, ok := normalizeArgumentValue(args).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return normalized
}

func (t ToolSchema) NormalizeArguments(args map[string]any) (map[string]any, error) {
	if len(args) == 0 {
		return map[string]any{}, nil
	}

	transformed := make(map[string]any, len(args))
	for key, value := range args {
		transformed[key] = value
	}
	for property, hint := range t.FlagHints {
		transform := strings.TrimSpace(hint.Transform)
		if transform == "" {
			continue
		}
		propertySchema := schemaForProperty(t.InputSchema, property)
		value, exists := transformed[property]
		if !exists {
			if transform == "enum_map" && hint.TransformArgs != nil {
				if defaultValue, ok := hint.TransformArgs["_default"]; ok {
					transformed[property] = defaultValue
				}
			}
			continue
		}
		nextValue, err := applyTransform(value, transform, hint.TransformArgs, propertySchema)
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", property, err)
		}
		transformed[property] = nextValue
	}
	return NormalizeArguments(transformed), nil
}

func schemaForProperty(schema map[string]any, property string) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	if direct, ok := properties[property].(map[string]any); ok {
		return direct
	}

	current := schema
	for _, part := range strings.Split(property, ".") {
		properties, ok = current["properties"].(map[string]any)
		if !ok {
			return nil
		}
		next, ok := properties[part].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func BuildFlagArgs(tool ToolSchema, args map[string]any, useAliases bool) ([]string, error) {
	out := append([]string{}, tool.CLIPath...)
	for _, flag := range tool.Flags {
		value, ok := args[flag.PropertyName]
		if !ok {
			continue
		}
		flagName := flag.FlagName
		if useAliases {
			if alias := effectiveAlias(flag, tool.Flags); alias != "" {
				flagName = alias
			}
		}
		encoded, inline, err := encodeFlagValue(flag.Kind, value, tool.FlagHints[flag.PropertyName])
		if err != nil {
			return nil, fmt.Errorf("encode --%s: %w", flagName, err)
		}
		if inline {
			out = append(out, "--"+flagName+"="+encoded)
			continue
		}
		out = append(out, "--"+flagName, encoded)
	}
	return out, nil
}

func HasUsableAliases(flags []Flag) bool {
	for _, flag := range flags {
		if effectiveAlias(flag, flags) != "" {
			return true
		}
	}
	return false
}

func HasFlattenedNestedFlags(flags []Flag) bool {
	for _, flag := range flags {
		if strings.Contains(flag.PropertyName, ".") {
			return true
		}
	}
	return false
}

func BuildPublicJSONArgs(tool ToolSchema, args map[string]any, payloadFlag string, useAliases bool) ([]string, error) {
	payload := make(map[string]any, len(tool.Flags))
	for _, flag := range tool.Flags {
		value, ok := args[flag.PropertyName]
		if !ok {
			continue
		}
		key := flag.FlagName
		if useAliases {
			if alias := effectiveAlias(flag, tool.Flags); alias != "" {
				key = alias
			}
		}
		payload[key] = value
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", payloadFlag, err)
	}
	return append(append([]string{}, tool.CLIPath...), payloadFlag, string(data)), nil
}

func BuildJSONArgs(tool ToolSchema, args map[string]any, payloadFlag string) ([]string, error) {
	data, err := json.Marshal(NormalizeArguments(args))
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", payloadFlag, err)
	}
	return append(append([]string{}, tool.CLIPath...), payloadFlag, string(data)), nil
}

func BuildFixtureCatalog(catalog Catalog, tools []ToolSchema) FixtureCatalog {
	productsByID := make(map[string]*FixtureProduct, len(catalog.Products))
	order := make([]string, 0, len(catalog.Products))

	for _, product := range catalog.Products {
		serverKey := product.ID + "-probe"
		fixtureProduct := FixtureProduct{
			ID:          product.ID,
			DisplayName: product.DisplayName,
			Description: product.DisplayName,
			ServerKey:   serverKey,
			Endpoint:    product.Endpoint,
			Tools:       []FixtureTool{},
		}
		if strings.TrimSpace(product.Command) != "" {
			fixtureProduct.CLI = &FixtureProductCLI{Command: product.Command}
		}
		if strings.TrimSpace(product.Command) == "" && product.ID != "" {
			fixtureProduct.CLI = &FixtureProductCLI{Command: product.ID}
		}
		copyProduct := fixtureProduct
		productsByID[product.ID] = &copyProduct
		order = append(order, product.ID)
	}

	for _, tool := range tools {
		productID := strings.TrimSpace(tool.Product.ID)
		product := productsByID[productID]
		if product == nil {
			serverKey := productID + "-probe"
			command := tool.Product.Command
			if command == "" && len(tool.CLIPath) > 0 {
				command = tool.CLIPath[0]
			}
			product = &FixtureProduct{
				ID:          productID,
				DisplayName: tool.Product.DisplayName,
				Description: tool.Product.DisplayName,
				ServerKey:   serverKey,
				Endpoint:    tool.Product.Endpoint,
				Tools:       []FixtureTool{},
			}
			if strings.TrimSpace(command) != "" {
				product.CLI = &FixtureProductCLI{Command: command}
			}
			productsByID[productID] = product
			order = append(order, productID)
		}
		flagHints := cloneFlagHints(tool.FlagHints)
		if len(flagHints) == 0 {
			flagHints = make(map[string]FlagHint)
			for _, flag := range tool.Flags {
				alias := effectiveAlias(flag, tool.Flags)
				if alias == "" {
					continue
				}
				flagHints[flag.PropertyName] = FlagHint{Alias: alias}
			}
		}
		fixtureTool := FixtureTool{
			RPCName:         tool.Tool.RPCName,
			CLIName:         tool.Tool.CLIName,
			Title:           tool.Tool.CLIName,
			Description:     tool.Tool.Description,
			InputSchema:     projectFixtureInputSchema(tool),
			Sensitive:       tool.Tool.Sensitive,
			Group:           cliGroupFromPath(tool.CLIPath),
			FlagHints:       flagHints,
			SourceServerKey: product.ServerKey,
			CanonicalPath:   tool.Tool.CanonicalPath,
		}
		if strings.TrimSpace(tool.Tool.CLIName) == "" {
			fixtureTool.CLIName = lastPathToken(tool.CLIPath)
		}
		if strings.TrimSpace(tool.Tool.CanonicalPath) == "" {
			fixtureTool.CanonicalPath = tool.Path
		}
		product.Tools = append(product.Tools, fixtureTool)
	}

	fixtureProducts := make([]FixtureProduct, 0, len(order))
	for _, id := range order {
		if product := productsByID[id]; product != nil {
			fixtureProducts = append(fixtureProducts, *product)
		}
	}
	return FixtureCatalog{Products: fixtureProducts}
}

func projectFixtureInputSchema(tool ToolSchema) map[string]any {
	if len(tool.InputSchema) == 0 || len(tool.Flags) == 0 {
		return tool.InputSchema
	}

	projected := cloneSchemaEnvelope(tool.InputSchema)
	if len(projected) == 0 {
		projected = map[string]any{}
	}
	if _, ok := projected["type"]; !ok {
		projected["type"] = "object"
	}
	projected["properties"] = map[string]any{}

	required := make(map[string]struct{}, len(tool.Required))
	for _, property := range tool.Required {
		property = strings.TrimSpace(property)
		if property != "" {
			required[property] = struct{}{}
		}
	}

	for _, flag := range tool.Flags {
		property := strings.TrimSpace(flag.PropertyName)
		if property == "" {
			continue
		}
		propertySchema := schemaForProperty(tool.InputSchema, property)
		if len(propertySchema) == 0 {
			continue
		}
		setProjectedSchemaProperty(projected, tool.InputSchema, property, propertySchema, requiredProperty(required, property))
	}

	return projected
}

func cloneSchemaEnvelope(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		if key == "properties" || key == "required" {
			continue
		}
		out[key] = cloneSchemaValue(value)
	}
	return out
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = cloneSchemaValue(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for idx, child := range typed {
			out[idx] = cloneSchemaValue(child)
		}
		return out
	default:
		return typed
	}
}

func setProjectedSchemaProperty(root map[string]any, source map[string]any, path string, propertySchema map[string]any, required bool) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 {
		return
	}

	current := root
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return
		}
		properties, _ := current["properties"].(map[string]any)
		if properties == nil {
			properties = make(map[string]any)
			current["properties"] = properties
		}

		if required && index == len(parts)-1 {
			addRequiredSchemaField(current, part)
		}

		if index == len(parts)-1 {
			properties[part] = cloneSchemaValue(propertySchema)
			return
		}

		next, _ := properties[part].(map[string]any)
		if len(next) == 0 {
			next = cloneSchemaEnvelope(schemaForProperty(source, strings.Join(parts[:index+1], ".")))
			if len(next) == 0 {
				next = map[string]any{"type": "object"}
			}
			if _, ok := next["type"]; !ok {
				next["type"] = "object"
			}
			next["properties"] = make(map[string]any)
			properties[part] = next
		}
		current = next
	}
}

func addRequiredSchemaField(schema map[string]any, property string) {
	property = strings.TrimSpace(property)
	if property == "" {
		return
	}
	required, _ := schema["required"].([]any)
	for _, existing := range required {
		if text, ok := existing.(string); ok && strings.TrimSpace(text) == property {
			return
		}
	}
	schema["required"] = append(required, property)
}

func requiredProperty(required map[string]struct{}, property string) bool {
	if len(required) == 0 {
		return false
	}
	_, ok := required[strings.TrimSpace(property)]
	return ok
}

func ToKebabCase(name string) string {
	name = strings.ReplaceAll(name, "_", "-")
	var result strings.Builder
	for index, char := range name {
		if char >= 'A' && char <= 'Z' && index > 0 {
			result.WriteByte('-')
		}
		if char >= 'A' && char <= 'Z' {
			result.WriteRune(char + 32)
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func syntheticValue(name string, schema map[string]any) (any, error) {
	if enumValues, ok := schema["enum"].([]any); ok && len(enumValues) > 0 {
		return enumValues[0], nil
	}

	switch schema["type"] {
	case "string":
		return "probe-" + name, nil
	case "integer":
		return int64(6 + syntheticOrdinal(name)), nil
	case "number":
		return float64(6+syntheticOrdinal(name)) + 0.5, nil
	case "boolean":
		return syntheticOrdinal(name)%2 == 1, nil
	case "array":
		items, _ := schema["items"].(map[string]any)
		if len(items) == 0 {
			return []any{}, nil
		}
		first, err := syntheticValue(name+"-1", items)
		if err != nil {
			return nil, err
		}
		second, err := syntheticValue(name+"-2", items)
		if err != nil {
			return nil, err
		}
		return []any{first, second}, nil
	case "object":
		properties, _ := schema["properties"].(map[string]any)
		if len(properties) == 0 {
			return map[string]any{}, nil
		}
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			propertySchema, ok := properties[key].(map[string]any)
			if !ok {
				continue
			}
			value, err := syntheticValue(key, propertySchema)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
		return out, nil
	default:
		return "probe-" + name, nil
	}
}

func syntheticOrdinal(name string) int {
	idx := strings.LastIndex(name, "-")
	if idx < 0 || idx == len(name)-1 {
		return 1
	}
	value, err := strconv.Atoi(name[idx+1:])
	if err != nil || value <= 0 {
		return 1
	}
	return value
}

func syntheticCLIValue(name string, schema map[string]any, hint FlagHint) (any, error) {
	switch strings.TrimSpace(hint.Transform) {
	case "iso8601_to_millis":
		return syntheticISO8601(name), nil
	case "enum_map":
		if key, ok := firstEnumMapKey(hint.TransformArgs); ok {
			return key, nil
		}
		if defaultValue, ok := hint.TransformArgs["_default"]; ok {
			return fmt.Sprint(defaultValue), nil
		}
	case "json_parse":
		value, err := syntheticValue(name, schema)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	}
	return syntheticValue(name, schema)
}

func syntheticISO8601(name string) string {
	base := time.Date(2026, time.March, 27, 8, 9, 10, 0, time.UTC)
	lower := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(lower, "end") || strings.HasPrefix(lower, "to") || strings.Contains(lower, "until") {
		return base.Add(time.Hour).Format(time.RFC3339)
	}
	return base.Format(time.RFC3339)
}

func firstEnumMapKey(args map[string]any) (string, bool) {
	keys := make([]string, 0, len(args))
	for key := range args {
		if key == "_default" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "", false
	}
	return keys[0], true
}

func normalizeArgumentValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = normalizeArgumentValue(child)
		}
		nestDottedKeys(out)
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = normalizeArgumentValue(child)
		}
		return out
	default:
		return value
	}
}

func nestDottedKeys(values map[string]any) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0)
	for key := range values {
		if strings.Contains(key, ".") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		delete(values, key)

		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 {
			values[key] = value
			continue
		}
		parent, child := parts[0], parts[1]
		nested, ok := values[parent].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			values[parent] = nested
		}
		nested[child] = value
		nestDottedKeys(nested)
	}
}

func effectiveAlias(flag Flag, flags []Flag) string {
	alias := strings.TrimSpace(flag.Alias)
	if alias == "" || alias == strings.TrimSpace(flag.FlagName) {
		return ""
	}
	for _, candidate := range flags {
		if candidate.PropertyName == flag.PropertyName {
			continue
		}
		if strings.TrimSpace(candidate.FlagName) == alias {
			return ""
		}
		if strings.TrimSpace(candidate.Alias) == alias {
			return ""
		}
	}
	return alias
}

func encodeFlagValue(kind string, value any, hint FlagHint) (string, bool, error) {
	switch kind {
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return "", false, fmt.Errorf("want bool, got %T", value)
		}
		return strconv.FormatBool(boolean), true, nil
	case "string":
		return fmt.Sprintf("%v", value), false, nil
	case "integer":
		return fmt.Sprintf("%v", value), false, nil
	case "number":
		return fmt.Sprintf("%v", value), false, nil
	case "string_array", "integer_array", "number_array", "boolean_array":
		items, ok := value.([]any)
		if !ok {
			return "", false, fmt.Errorf("want []any, got %T", value)
		}
		if strings.TrimSpace(hint.Transform) == "csv_to_array" && shouldEncodeJSONArray(items) {
			data, err := json.Marshal(items)
			if err != nil {
				return "", false, err
			}
			return string(data), false, nil
		}
		parts := make([]string, 0, len(items))
		for _, item := range items {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, ","), false, nil
	case "json":
		if raw, ok := value.(string); ok {
			return raw, false, nil
		}
		data, err := json.Marshal(value)
		if err != nil {
			return "", false, err
		}
		return string(data), false, nil
	default:
		return "", false, fmt.Errorf("unsupported flag kind %q", kind)
	}
}

func shouldEncodeJSONArray(items []any) bool {
	for _, item := range items {
		switch item.(type) {
		case string:
			continue
		default:
			return true
		}
	}
	return false
}

func applyTransform(value any, transform string, args map[string]any, schema map[string]any) (any, error) {
	switch strings.TrimSpace(transform) {
	case "":
		return value, nil
	case "iso8601_to_millis":
		return transformISO8601ToMillis(value, schema)
	case "csv_to_array":
		return transformCSVToArray(value, schema)
	case "json_parse":
		return transformJSONParse(value, schema)
	case "enum_map":
		return transformEnumMap(value, args)
	default:
		return value, nil
	}
}

func transformISO8601ToMillis(value any, schema map[string]any) (any, error) {
	if schemaPrefersString(schema) {
		return value, nil
	}

	s, ok := canonicalString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	if millis, err := strconv.ParseInt(s, 10, 64); err == nil && millis > 1_000_000_000_000 {
		return millis, nil
	}

	layouts := []struct {
		layout   string
		location *time.Location
	}{
		{layout: time.RFC3339},
		{layout: "2006-01-02T15:04:05"},
		{layout: "2006-01-02 15:04:05"},
		{layout: "2006-01-02", location: time.UTC},
	}
	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.location != nil {
			parsed, err = time.ParseInLocation(candidate.layout, s, candidate.location)
		} else {
			parsed, err = time.Parse(candidate.layout, s)
		}
		if err == nil {
			return parsed.UnixMilli(), nil
		}
	}
	return nil, fmt.Errorf("iso8601_to_millis: cannot parse %q as ISO-8601", s)
}

func transformCSVToArray(value any, schema map[string]any) (any, error) {
	values, handled, err := normalizeCSVInput(value)
	if err != nil {
		return nil, err
	}
	if !handled {
		return value, nil
	}
	return coerceArrayToSchema(values, schema), nil
}

func normalizeCSVInput(value any) ([]any, bool, error) {
	switch typed := value.(type) {
	case string:
		values, err := splitCSVString(typed)
		return values, true, err
	case []string:
		raw := make([]any, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
		return normalizeCSVSequence(raw)
	case []any:
		return normalizeCSVSequence(typed)
	default:
		return nil, false, nil
	}
}

func normalizeCSVSequence(values []any) ([]any, bool, error) {
	if joined, ok := joinCanonicalSequence(values); ok {
		trimmed := strings.TrimSpace(joined)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			items, err := splitCSVString(joined)
			if err != nil {
				return nil, true, err
			}
			return items, true, nil
		}
	}

	out := make([]any, 0, len(values))
	for _, value := range values {
		text, ok := canonicalString(value)
		if !ok {
			out = append(out, value)
			continue
		}
		items, err := splitCSVString(text)
		if err != nil {
			return nil, true, err
		}
		out = append(out, items...)
	}
	return out, true, nil
}

func joinCanonicalSequence(values []any) (string, bool) {
	if len(values) < 2 {
		return "", false
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := canonicalString(value)
		if !ok {
			return "", false
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ","), true
}

func splitCSVString(raw string) ([]any, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return []any{}, nil
	}
	if strings.HasPrefix(s, "[") {
		var values []any
		if err := json.Unmarshal([]byte(s), &values); err == nil {
			return values, nil
		}
	}
	parts := strings.Split(s, ",")
	values := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values, nil
}

func coerceArrayToSchema(values []any, schema map[string]any) []any {
	if len(values) == 0 || len(schema) == 0 {
		return values
	}
	itemSchema, ok := schema["items"].(map[string]any)
	if !ok {
		return values
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = coerceScalarToSchema(value, itemSchema)
	}
	return out
}

func coerceScalarToSchema(value any, schema map[string]any) any {
	types := schemaTypes(schema)
	if len(types) == 0 || matchesAnyType(value, types) {
		return value
	}

	s, ok := canonicalString(value)
	if !ok {
		return value
	}
	s = strings.TrimSpace(s)

	switch {
	case hasType(types, "integer"):
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			return parsed
		}
	case hasType(types, "number"):
		if parsed, err := strconv.ParseFloat(s, 64); err == nil {
			return parsed
		}
	case hasType(types, "boolean"):
		if parsed, err := strconv.ParseBool(s); err == nil {
			return parsed
		}
	}
	return value
}

func transformJSONParse(value any, schema map[string]any) (any, error) {
	if schemaPrefersString(schema) {
		return value, nil
	}

	s, ok := canonicalString(value)
	if !ok {
		return value, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return value, nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return nil, fmt.Errorf("json_parse: invalid JSON: %v", err)
	}
	return parsed, nil
}

func transformEnumMap(value any, args map[string]any) (any, error) {
	s, ok := canonicalString(value)
	if !ok {
		s = fmt.Sprint(value)
	}
	s = strings.TrimSpace(s)
	if mapped, exists := args[s]; exists {
		return mapped, nil
	}
	if defaultValue, exists := args["_default"]; exists {
		return defaultValue, nil
	}
	return value, nil
}

func canonicalString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case fmt.Stringer:
		return typed.String(), true
	default:
		return "", false
	}
}

func schemaPrefersString(schema map[string]any) bool {
	types := schemaTypes(schema)
	return hasType(types, "string") && !hasType(types, "number") && !hasType(types, "integer")
}

func schemaTypes(schema map[string]any) []string {
	switch typed := schema["type"].(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			if strings.TrimSpace(entry) != "" {
				out = append(out, entry)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			text, ok := entry.(string)
			if ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func hasType(types []string, target string) bool {
	for _, item := range types {
		if item == target {
			return true
		}
	}
	return false
}

func matchesAnyType(value any, types []string) bool {
	for _, expected := range types {
		if matchesType(value, expected) {
			return true
		}
	}
	return false
}

func matchesType(value any, expected string) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := numberValue(value)
		return ok
	case "integer":
		n, ok := numberValue(value)
		if !ok {
			return false
		}
		return float64(int64(n)) == n
	case "null":
		return value == nil
	default:
		return true
	}
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func cliGroupFromPath(path []string) string {
	if len(path) <= 2 {
		return ""
	}
	return strings.Join(path[1:len(path)-1], ".")
}

func lastPathToken(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

func cloneFlagHints(value map[string]FlagHint) map[string]FlagHint {
	if len(value) == 0 {
		return nil
	}
	out := make(map[string]FlagHint, len(value))
	for key, hint := range value {
		copied := hint
		if len(hint.TransformArgs) > 0 {
			copied.TransformArgs = make(map[string]any, len(hint.TransformArgs))
			for childKey, childValue := range hint.TransformArgs {
				copied.TransformArgs[childKey] = childValue
			}
		}
		out[key] = copied
	}
	return out
}
