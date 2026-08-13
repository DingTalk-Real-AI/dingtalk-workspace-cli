// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// ToolSearchV1Request is the stable stdin transport used by Agent hosts.
type ToolSearchV1Request struct {
	Version          string   `json:"version"`
	Query            string   `json:"query"`
	Subqueries       []string `json:"subqueries,omitempty"`
	Limit            int      `json:"limit,omitempty"`
	CandidateLimit   int      `json:"candidate_limit,omitempty"`
	ProductIDs       []string `json:"product_ids,omitempty"`
	Effects          []string `json:"effects,omitempty"`
	ExcludeCanonical []string `json:"exclude_canonical,omitempty"`
}

var schemaSearchNewEngine = NewDeliveryToolSearchEngine

func newSchemaSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "按自然语言搜索可用命令 Schema",
		Long: `按自然语言或精确身份检索 ToolReference。

输出固定为完整、紧凑的 tool-search.v1 JSON envelope。为保证版本、Catalog
绑定和响应字节预算可验证，本命令不支持 --fields、--jq 或非 JSON --format。

DWS 对 Agent 是一个元工具：已知子命令直接执行，未知子命令才使用 search。
选中候选后，必须使用 response catalog 的 source/surface hash Inspect canonical_path，
再按 primary_cli_path 执行；Search 结果本身不授权、不执行。`,
		Example: `  dws schema search --query "查询群消息已读状态"
  dws schema chat.query_msg_read_status --compact --format json \
    --expected-source-hash "<search.catalog.source_hash>" \
    --expected-surface-hash "<search.catalog.surface_hash>"
  printf '%s' '{"version":"tool-search.v1","query":"发群文件并确认已读","subqueries":["给群里发送文件消息","查询群消息已读状态"]}' |
    dws schema search --request-json -`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateSchemaSearchOutputFlags(cmd); err != nil {
				return err
			}
			request, err := schemaSearchRequest(cmd)
			if err != nil {
				return err
			}
			engine, err := schemaSearchNewEngine()
			if err != nil {
				return err
			}
			internal := ToolSearchRequest{
				Query:                 request.Query,
				Limit:                 request.Limit,
				CandidateLimit:        request.CandidateLimit,
				ProductIDs:            request.ProductIDs,
				Effects:               request.Effects,
				ExcludeCanonicalPaths: request.ExcludeCanonical,
			}
			var response ToolSearchResponse
			if len(request.Subqueries) > 0 {
				response, err = engine.SearchSubqueries(cmd.Context(), request.Subqueries, internal)
			} else {
				response, err = engine.Search(cmd.Context(), internal)
			}
			if err != nil {
				return err
			}
			// The response budget is defined over this compact wire encoding. Do
			// not run it through pretty/table formatting or post-filtering because
			// Agent hosts require one complete versioned envelope.
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(response); err != nil {
				return fmt.Errorf("encode tool-search.v1 response: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().String("query", "", "自然语言意图或 canonical/CLI path")
	cmd.Flags().Int("limit", defaultToolSearchLimit, "返回引用数量")
	cmd.Flags().Int("candidate-limit", defaultToolSearchCandidateLimit, "每路召回候选数量")
	cmd.Flags().StringSlice("product", nil, "限制产品 ID，可重复或逗号分隔")
	cmd.Flags().StringSlice("effect", nil, "限制 effect，可重复或逗号分隔")
	cmd.Flags().StringSlice("exclude", nil, "排除 canonical path，可重复或逗号分隔")
	cmd.Flags().String("request-json", "", `从 stdin 读取 JSON；值必须为 -；必含 version="tool-search.v1"，动作分解字段为 subqueries`)
	return cmd
}

func validateSchemaSearchOutputFlags(cmd *cobra.Command) error {
	for _, name := range []string{"fields", "jq"} {
		if flag := cmd.Flag(name); flag != nil && flag.Changed {
			return apperrors.NewValidation(
				"schema search returns one complete tool-search.v1 envelope and does not support --"+name,
				apperrors.WithReason("unsupported_output_filter"),
			)
		}
	}
	if flag := cmd.Flag("format"); flag != nil && flag.Changed && strings.TrimSpace(flag.Value.String()) != "json" {
		return apperrors.NewValidation(
			"schema search only supports --format json",
			apperrors.WithReason("unsupported_output_format"),
		)
	}
	return nil
}

func schemaSearchRequest(cmd *cobra.Command) (ToolSearchV1Request, error) {
	requestJSON, _ := cmd.Flags().GetString("request-json")
	requestJSON = strings.TrimSpace(requestJSON)
	if requestJSON != "" {
		if requestJSON != "-" {
			return ToolSearchV1Request{}, apperrors.NewValidation("--request-json only accepts - (stdin)", apperrors.WithReason("invalid_transport"))
		}
		for _, name := range []string{"query", "limit", "candidate-limit", "product", "effect", "exclude"} {
			if cmd.Flags().Changed(name) {
				return ToolSearchV1Request{}, apperrors.NewValidation("--request-json cannot be combined with --"+name, apperrors.WithReason("ambiguous_input"))
			}
		}
		request, err := decodeToolSearchV1Request(cmd.InOrStdin())
		if err != nil {
			return ToolSearchV1Request{}, err
		}
		return validateToolSearchV1Request(request)
	}
	query, _ := cmd.Flags().GetString("query")
	limit, _ := cmd.Flags().GetInt("limit")
	candidateLimit, _ := cmd.Flags().GetInt("candidate-limit")
	products, _ := cmd.Flags().GetStringSlice("product")
	effects, _ := cmd.Flags().GetStringSlice("effect")
	excluded, _ := cmd.Flags().GetStringSlice("exclude")
	return validateToolSearchV1Request(ToolSearchV1Request{
		Version:          "tool-search.v1",
		Query:            query,
		Limit:            limit,
		CandidateLimit:   candidateLimit,
		ProductIDs:       products,
		Effects:          effects,
		ExcludeCanonical: excluded,
	})
}

func decodeToolSearchV1Request(reader io.Reader) (ToolSearchV1Request, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maxToolSearchRequestBytes+1))
	if err != nil {
		return ToolSearchV1Request{}, apperrors.NewValidation("read tool search request: "+err.Error(), apperrors.WithReason("read_request_failed"))
	}
	if len(payload) > maxToolSearchRequestBytes {
		return ToolSearchV1Request{}, apperrors.NewValidation(fmt.Sprintf("tool search request exceeds %d bytes", maxToolSearchRequestBytes), apperrors.WithReason("request_too_large"))
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request ToolSearchV1Request
	if err := decoder.Decode(&request); err != nil {
		return ToolSearchV1Request{}, apperrors.NewValidation("decode tool-search.v1 request: "+err.Error(), apperrors.WithReason("invalid_request_json"))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ToolSearchV1Request{}, apperrors.NewValidation("tool search request must contain exactly one JSON object", apperrors.WithReason("invalid_request_json"))
		}
		return ToolSearchV1Request{}, apperrors.NewValidation("decode trailing tool search input: "+err.Error(), apperrors.WithReason("invalid_request_json"))
	}
	return request, nil
}

func validateToolSearchV1Request(request ToolSearchV1Request) (ToolSearchV1Request, error) {
	if request.Version != "tool-search.v1" {
		return ToolSearchV1Request{}, apperrors.NewValidation(`tool search request must include "version":"tool-search.v1"; action decomposition uses the "subqueries" array`, apperrors.WithReason("unsupported_version"))
	}
	return request, nil
}
