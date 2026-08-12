// Command fetch_mcp_metadata pulls tools/list from ALL live MCP server endpoints
// and writes a local diagnostic dump. It is NOT a Schema delivery refresh:
// schema_mcp_metadata.json is retired; production Catalog assembles from
// Contract/ParamDecl/Interface + Cobra only.
//
// Usage:
//
//	dws auth login                     # ensure valid auth
//	make fetch-mcp-metadata             # writes artifacts/mcp_metadata_diagnostic.json
//
// The tool loads auth from the DWS keychain, iterates static server endpoints
// (internal/syncdata.StaticServers), calls tools/list on each, merges results,
// and writes the requested -output path (refuses the retired pin path).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/syncdata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// toolLister is the tools/list capability consumed by run; production code
// uses transport.Client, tests inject fakes.
type toolLister interface {
	ListTools(ctx context.Context, endpoint string) (transport.ToolsListResult, error)
}

// Injection points so run() is fully testable without network/keychain/exit.
var (
	osExit               = os.Exit
	getenv               = os.Getenv
	loadTokenData        = auth.LoadTokenDataKeychain
	staticServers        = syncdata.StaticServers
	registrySource       = collectedIdentityInterfaceRefs
	collectIdentitySpecs = cli.CollectIdentitySpecs
	listToolsTimeout     = 30 * time.Second
	gitHeadPath          = ".git/HEAD"
	newToolLister        = func(token string) toolLister {
		return transport.NewClient(&http.Client{Timeout: 60 * time.Second}).WithAuth(token, nil)
	}
)

func main() {
	osExit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("fetch_mcp_metadata", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "artifacts/mcp_metadata_diagnostic.json", "diagnostic dump path (not a Schema pin)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if retiredPinnedMCPMetadataPath(*output) {
		fmt.Fprintln(stderr, "fetch_mcp_metadata: refusing to write retired Schema pin internal/cli/schema_mcp_metadata.json")
		return 2
	}

	token := resolveToken(stderr)
	if token == "" {
		fmt.Fprintln(stderr, "fetch_mcp_metadata: no auth token. Run 'dws auth login' first.")
		return 1
	}

	client := newToolLister(token)

	// Iterate ALL static server endpoints (26 servers covering all products).
	servers := staticServers()
	fmt.Fprintf(stderr, "fetch_mcp_metadata: querying %d server endpoints\n", len(servers))

	// Collect command identity to build tool_name → interface_ref mapping.
	registryMap := loadRegistryInterfaceRefs(stderr)
	fmt.Fprintf(stderr, "fetch_mcp_metadata: registry mapping: %d entries\n", len(registryMap))

	// Load a previous diagnostic dump (if any) to preserve hand-curated
	// cross-server interface_ref mappings that automated matching can't derive.
	prevData, prevErr := os.ReadFile(*output)
	prevTools := map[string]map[string]any{}
	if prevErr == nil {
		var prev struct {
			Tools map[string]map[string]any `json:"tools"`
		}
		if json.Unmarshal(prevData, &prev) == nil {
			prevTools = prev.Tools
		}
	}

	// Start from previous data (preserves cross-server refs), then overwrite
	// with fresh MCP data where available.
	allTools := make(map[string]map[string]any)
	for k, v := range prevTools {
		allTools[k] = v
	}

	// Reviewed cross-server interface_refs live only in the previous snapshot
	// (the registry stores canonical paths, not MCP identities). Build a
	// live-key → canonicals index so those tools get refreshed instead of
	// being skipped and frozen at the previous snapshot forever.
	crossRefs := buildCrossServerRefs(prevTools, registryMap)
	if len(crossRefs) > 0 {
		fmt.Fprintf(stderr, "fetch_mcp_metadata: cross-server ref index: %d live keys\n", len(crossRefs))
	}
	// Canonicals with a reviewed cross-server identity must only be fed by
	// that identity; a same-named tool on another server is a coincidence,
	// not a data source.
	crossOwned := map[string]bool{}
	for _, canonicals := range crossRefs {
		for _, canonical := range canonicals {
			crossOwned[canonical] = true
		}
	}
	totalRaw := 0
	failedServices := []string{}

	for _, srv := range servers {
		endpoint := strings.TrimSpace(srv.Endpoint)
		if endpoint == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), listToolsTimeout)
		result, err := client.ListTools(ctx, endpoint)
		cancel()
		if err != nil {
			fmt.Fprintf(stderr, "  [skip] %s: %v\n", srv.ID, err)
			failedServices = append(failedServices, srv.ID)
			continue
		}
		fmt.Fprintf(stderr, "  [ok]   %s: %d tools\n", srv.ID, len(result.Tools))
		totalRaw += len(result.Tools)
		for _, tool := range result.Tools {
			name := strings.TrimSpace(tool.Name)
			if name == "" {
				continue
			}
			// Direct match: CLI canonical equals server-prefixed tool name
			// (e.g., "doc.copy_document"). Cross-owned canonicals are skipped
			// here — their reviewed identity feeds them below.
			canonicalKey := srv.ID + "." + name
			if ref, hasRef := registryMap[canonicalKey]; hasRef && !crossOwned[canonicalKey] {
				mergeLiveMCPTool(allTools, canonicalKey, tool, ref)
			}
			// Cross-server match: registry canonicals whose reviewed
			// interface_ref points at this live tool (one live tool may feed
			// several canonicals, e.g. advperm_enable/disable → set_advanced_permission).
			for _, canonical := range crossRefs[canonicalKey] {
				mergeLiveMCPTool(allTools, canonical, tool, registryMap[canonical])
			}
		}
	}

	matched := 0
	for _, t := range allTools {
		if _, ok := t["interface_ref"]; ok {
			matched++
		}
	}
	fmt.Fprintf(stderr, "fetch_mcp_metadata: MCP matched=%d, with interface_ref=%d\n", len(allTools), matched)

	// Fill gaps: for registry canonicals not covered by MCP tools/list OR
	// previous data, add stub entries (interface_ref only).
	stubs := 0
	for canonicalKey, ref := range registryMap {
		if _, exists := allTools[canonicalKey]; exists {
			continue
		}
		allTools[canonicalKey] = map[string]any{
			"interface_ref": ref,
		}
		stubs++
	}
	if stubs > 0 {
		fmt.Fprintf(stderr, "fetch_mcp_metadata: added %d registry stubs (no MCP data, interface_ref only)\n", stubs)
	}

	// Compute coverage fields required by check-schema-catalog.sh. Failed
	// services must be reported honestly so policy can spot snapshot gaps.
	if len(failedServices) > 0 {
		fmt.Fprintf(stderr, "fetch_mcp_metadata: %d/%d services unreachable: %s\n",
			len(failedServices), len(servers), strings.Join(failedServices, ", "))
	}

	metadata := map[string]any{
		"version":  1,
		"source":   "mcp-tools-list+cli-registry",
		"coverage": buildCoverage(len(servers), failedServices, totalRaw, len(allTools), stubs),
		"tools":    allTools,
	}

	// source_revision: git commit hash (proves provenance).
	if rev, err := os.ReadFile(gitHeadPath); err == nil {
		metadata["source_revision"] = strings.TrimSpace(string(rev))
	}

	if err := writeMetadata(*output, metadata); err != nil {
		fmt.Fprintf(stderr, "fetch_mcp_metadata: %v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "fetch_mcp_metadata: wrote %d tools to %s\n", len(allTools), *output)
	return 0
}

// resolveToken returns the access token from DWS_ACCESS_TOKEN or, as a
// fallback, the DWS keychain.
func resolveToken(stderr io.Writer) string {
	token := strings.TrimSpace(getenv("DWS_ACCESS_TOKEN"))
	if token != "" {
		return token
	}
	td, err := loadTokenData()
	if err != nil || td == nil || td.AccessToken == "" {
		return ""
	}
	fmt.Fprintf(stderr, "fetch_mcp_metadata: loaded token from keychain (%d chars)\n", len(td.AccessToken))
	return td.AccessToken
}

// writeMetadata marshals the snapshot and writes it to the output path.
func writeMetadata(path string, metadata map[string]any) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s failed: %w", path, err)
	}
	return nil
}

// buildCoverage reports snapshot coverage honestly: snapshot_services only
// counts services whose tools/list succeeded, missing_services names the
// failures, and matched_tools excludes registry stubs (entries carrying no
// live MCP metadata) so a stub-heavy snapshot cannot claim full matching.
func buildCoverage(sourceServices int, failedServices []string, sourceTools, surfaceTools, stubs int) map[string]any {
	missing := failedServices
	if missing == nil {
		missing = []string{}
	}
	return map[string]any{
		"surface_scope":     "source_revision",
		"source_services":   sourceServices,
		"snapshot_services": sourceServices - len(missing),
		"missing_services":  missing,
		"source_tools":      sourceTools,
		"surface_tools":     surfaceTools,
		"matched_tools":     surfaceTools - stubs,
		"aliased_tools":     0,
		"unmatched_tools":   stubs,
	}
}

// mergeLiveMCPTool replaces stale live-derived fields while retaining an
// existing reviewed interface_ref. Some CLI canonicals intentionally route to
// a differently named product/RPC, so the previous cross-server mapping must
// survive even though title, description, and parameters are refreshed.
func mergeLiveMCPTool(allTools map[string]map[string]any, canonicalKey string, tool transport.ToolDescriptor, fallbackRef map[string]string) {
	interfaceRef := any(fallbackRef)
	if previous := allTools[canonicalKey]; previous != nil {
		if reviewedRef, ok := previous["interface_ref"]; ok && reviewedRef != nil {
			interfaceRef = reviewedRef
		}
	}

	entry := map[string]any{
		"title":         tool.Title,
		"description":   tool.Description,
		"interface_ref": interfaceRef,
	}
	if tool.InputSchema != nil {
		entry["parameters"] = extractParams(tool.InputSchema)
	}
	allTools[canonicalKey] = entry
}

// buildCrossServerRefs indexes reviewed cross-server mappings from the
// previous snapshot: for every registry canonical whose interface_ref names a
// different MCP identity (product_id.rpc_name != canonical), the live key is
// mapped back to that canonical. One live tool may serve several canonicals,
// so values are slices, sorted for deterministic merge order.
func buildCrossServerRefs(prevTools map[string]map[string]any, registryMap map[string]map[string]string) map[string][]string {
	index := map[string][]string{}
	for canonical, entry := range prevTools {
		if _, inRegistry := registryMap[canonical]; !inRegistry {
			continue
		}
		ref, ok := entry["interface_ref"].(map[string]any)
		if !ok {
			continue
		}
		productID, _ := ref["product_id"].(string)
		rpcName, _ := ref["rpc_name"].(string)
		if productID == "" || rpcName == "" {
			continue
		}
		liveKey := productID + "." + rpcName
		if liveKey == canonical {
			continue
		}
		index[liveKey] = append(index[liveKey], canonical)
	}
	for _, canonicals := range index {
		sort.Strings(canonicals)
	}
	return index
}

// collectedIdentityInterfaceRefs collects command identity from the live
// command tree — the replacement for the retired reviewed CommandRegistry —
// and derives the canonical_path → {product_id, rpc_name} mapping used for
// interface_ref injection.
func collectedIdentityInterfaceRefs() (map[string]map[string]string, error) {
	root := app.NewSchemaSourceRootCommand()
	specs, _, err := collectIdentitySpecs(root)
	if err != nil {
		return nil, fmt.Errorf("collect command identity: %w", err)
	}
	out := make(map[string]map[string]string, len(specs))
	for _, spec := range specs {
		cp := strings.TrimSpace(spec.CanonicalPath)
		if cp == "" || !strings.Contains(cp, ".") {
			continue
		}
		parts := strings.SplitN(cp, ".", 2)
		out[cp] = map[string]string{
			"product_id": parts[0],
			"rpc_name":   parts[1],
		}
	}
	return out, nil
}

// loadRegistryInterfaceRefs builds the canonical_path → interface_ref mapping
// from the collected command identity. 与旧实现同等告警：静默返回空映射会让
// 所有 live tool 被丢弃、产出 stub-only 快照且零提示（P1#1 的故障模式）。
func loadRegistryInterfaceRefs(stderr io.Writer) map[string]map[string]string {
	refs, err := registrySource()
	if err != nil {
		fmt.Fprintf(stderr, "fetch_mcp_metadata: warning: cannot collect command identity: %v\n", err)
		return map[string]map[string]string{}
	}
	return refs
}

func retiredPinnedMCPMetadataPath(path string) bool {
	cleaned := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	return cleaned == "internal/cli/schema_mcp_metadata.json" ||
		strings.HasSuffix(cleaned, "/internal/cli/schema_mcp_metadata.json")
}

// extractParams converts a JSON Schema inputSchema (from MCP tools/list) into
// the flat param-name → metadata map used by diagnostic dumps.
func extractParams(inputSchema map[string]any) map[string]map[string]any {
	if inputSchema == nil {
		return nil
	}
	properties, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	requiredSet := map[string]bool{}
	if req, ok := inputSchema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	params := make(map[string]map[string]any, len(properties))
	for name, raw := range properties {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		meta := map[string]any{}
		if t, ok := prop["type"].(string); ok {
			meta["type"] = t
		}
		if d, ok := prop["description"].(string); ok {
			meta["description"] = d
		}
		if d, ok := prop["default"].(string); ok {
			meta["default"] = d
		}
		if e, ok := prop["enum"].([]any); ok {
			enums := make([]string, 0, len(e))
			for _, v := range e {
				if s, ok := v.(string); ok {
					enums = append(enums, s)
				}
			}
			if len(enums) > 0 {
				meta["enum"] = enums
			}
		}
		meta["required"] = requiredSet[name]
		params[name] = meta
	}
	return params
}
