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

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

const manualAgentSelectionLiveBatchSize = 12

type manualAgentSelectionLiveCandidate struct {
	CanonicalPath string   `json:"canonical_path"`
	AgentSummary  string   `json:"agent_summary"`
	UseWhen       []string `json:"use_when"`
	AvoidWhen     []string `json:"avoid_when"`
}

type manualAgentSelectionLiveInput struct {
	Cases      []manualAgentSelectionLiveCase      `json:"cases"`
	Candidates []manualAgentSelectionLiveCandidate `json:"candidates"`
}

// manualAgentSelectionLiveCase is deliberately answer-free. Expected and
// forbidden canonicals stay only in the local assertion fixture and are never
// sent to the model being evaluated.
type manualAgentSelectionLiveCase struct {
	ID       string `json:"id"`
	Scenario string `json:"scenario"`
}

type manualAgentSelectionLiveResult struct {
	ID            string `json:"id"`
	CanonicalPath string `json:"canonical_path"`
}

type manualAgentSelectionLiveResponse struct {
	Results []manualAgentSelectionLiveResult `json:"results"`
}

// TestManualAgentSelectionArkLive is intentionally opt-in. Deterministic CI
// validates all fixture and Cobra facts without network access; this test asks
// a real model to interpret the reviewed natural-language scenarios. Set
// DWS_AGENT_SELECTION_FULL=1 to evaluate every positive and negative case.
func TestManualAgentSelectionArkLive(t *testing.T) {
	if os.Getenv("DWS_AGENT_SELECTION_LIVE") != "1" {
		t.Skip("set DWS_AGENT_SELECTION_LIVE=1 and ARK_API_KEY/ARK_BASE_URL/ARK_MODEL to run live Agent command-selection evaluation")
	}
	apiKey := strings.TrimSpace(os.Getenv("ARK_API_KEY"))
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ARK_BASE_URL")), "/")
	model := strings.TrimSpace(os.Getenv("ARK_MODEL"))
	for name, value := range map[string]string{
		"ARK_API_KEY":  apiKey,
		"ARK_BASE_URL": baseURL,
		"ARK_MODEL":    model,
	} {
		if value == "" {
			t.Fatalf("%s is required when DWS_AGENT_SELECTION_LIVE=1", name)
		}
	}
	if err := validateManualAgentSelectionLiveBaseURL(baseURL, os.Getenv("DWS_AGENT_SELECTION_ALLOWED_BASE_URLS")); err != nil {
		t.Fatal(err)
	}

	fixture, hints := manualAgentSelectionLiveFixture(t)
	cases := selectManualAgentSelectionLiveCases(t, fixture.Cases)
	for _, batch := range batchManualAgentSelectionLiveCases(cases, manualAgentSelectionLiveBatchSize) {
		productID := batch[0].ProductID
		t.Run(productID+"/"+sanitizeManualAgentSelectionLiveTestID(batch[0].ID), func(t *testing.T) {
			input := buildManualAgentSelectionLiveInput(batch, hints)
			results := callManualAgentSelectionLiveModel(t, baseURL, apiKey, model, input)
			assertManualAgentSelectionLiveResults(t, batch, results)
		})
	}
}

func buildManualAgentSelectionLiveInput(batch []cli.ManualAgentSelectionCase, hints cli.ManualAgentHintSet) manualAgentSelectionLiveInput {
	input := manualAgentSelectionLiveInput{Cases: make([]manualAgentSelectionLiveCase, 0, len(batch))}
	if len(batch) == 0 {
		return input
	}
	for _, selectionCase := range batch {
		input.Cases = append(input.Cases, manualAgentSelectionLiveCase{
			ID:       selectionCase.ID,
			Scenario: selectionCase.Scenario,
		})
	}
	input.Candidates = make([]manualAgentSelectionLiveCandidate, 0, len(batch[0].CandidateCanonicals))
	for _, canonical := range batch[0].CandidateCanonicals {
		hint := hints.Tools[canonical]
		input.Candidates = append(input.Candidates, manualAgentSelectionLiveCandidate{
			CanonicalPath: canonical,
			AgentSummary:  hint.AgentSummary,
			UseWhen:       hint.UseWhen,
			AvoidWhen:     hint.AvoidWhen,
		})
	}
	return input
}

func manualAgentSelectionLiveFixture(t testing.TB) (cli.ManualAgentSelectionFixture, cli.ManualAgentHintSet) {
	t.Helper()
	root := NewRootCommand()
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry() error = %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry() error = %v", err)
	}
	hints, err := cli.LoadAgentHintsFromSelectionForValidation(os.DirFS("../cli/schema_hints/selection"))
	if err != nil {
		t.Fatalf("LoadAgentHintsFromSelectionForValidation() error = %v", err)
	}
	fixture, _, err := cli.BuildManualAgentSelectionEvalFixture(bound, hints)
	if err != nil {
		t.Fatalf("BuildManualAgentSelectionEvalFixture() error = %v", err)
	}
	return fixture, hints
}

func selectManualAgentSelectionLiveCases(t testing.TB, cases []cli.ManualAgentSelectionCase) []cli.ManualAgentSelectionCase {
	t.Helper()
	if raw := strings.TrimSpace(os.Getenv("DWS_AGENT_SELECTION_CASES")); raw != "" {
		selected := map[string]bool{}
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				selected[id] = true
			}
		}
		result := make([]cli.ManualAgentSelectionCase, 0, len(selected))
		for _, selectionCase := range cases {
			if selected[selectionCase.ID] {
				result = append(result, selectionCase)
				delete(selected, selectionCase.ID)
			}
		}
		if len(selected) != 0 {
			missing := make([]string, 0, len(selected))
			for id := range selected {
				missing = append(missing, id)
			}
			sort.Strings(missing)
			t.Fatalf("DWS_AGENT_SELECTION_CASES contains unknown case IDs: %s", strings.Join(missing, ", "))
		}
		return result
	}
	if os.Getenv("DWS_AGENT_SELECTION_FULL") == "1" {
		return append([]cli.ManualAgentSelectionCase(nil), cases...)
	}

	// Smoke mode exercises one positive and one negative scenario per product.
	seenPositive := map[string]bool{}
	seenNegative := map[string]bool{}
	result := make([]cli.ManualAgentSelectionCase, 0)
	for _, selectionCase := range cases {
		if selectionCase.ExpectedCanonical != "" && !seenPositive[selectionCase.ProductID] {
			seenPositive[selectionCase.ProductID] = true
			result = append(result, selectionCase)
		}
		if selectionCase.ForbiddenCanonical != "" && !seenNegative[selectionCase.ProductID] {
			seenNegative[selectionCase.ProductID] = true
			result = append(result, selectionCase)
		}
	}
	return result
}

func batchManualAgentSelectionLiveCases(cases []cli.ManualAgentSelectionCase, batchSize int) [][]cli.ManualAgentSelectionCase {
	if batchSize <= 0 {
		batchSize = 1
	}
	grouped := map[string][]cli.ManualAgentSelectionCase{}
	products := make([]string, 0)
	for _, selectionCase := range cases {
		if _, ok := grouped[selectionCase.ProductID]; !ok {
			products = append(products, selectionCase.ProductID)
		}
		grouped[selectionCase.ProductID] = append(grouped[selectionCase.ProductID], selectionCase)
	}
	sort.Strings(products)
	result := make([][]cli.ManualAgentSelectionCase, 0)
	for _, productID := range products {
		productCases := grouped[productID]
		for start := 0; start < len(productCases); start += batchSize {
			end := start + batchSize
			if end > len(productCases) {
				end = len(productCases)
			}
			result = append(result, append([]cli.ManualAgentSelectionCase(nil), productCases[start:end]...))
		}
	}
	return result
}

func callManualAgentSelectionLiveModel(t testing.TB, baseURL, apiKey, model string, input manualAgentSelectionLiveInput) []manualAgentSelectionLiveResult {
	t.Helper()
	body, err := marshalManualAgentSelectionLiveRequest(baseURL, model, input)
	if err != nil {
		t.Fatalf("marshal live selection request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build live selection request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 0 && (request.URL.Scheme != via[0].URL.Scheme || !strings.EqualFold(request.URL.Host, via[0].URL.Host)) {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("live selection model request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		t.Fatalf("live selection model status %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil {
		t.Fatalf("decode live selection response envelope: %v", err)
	}
	if len(envelope.Choices) == 0 || strings.TrimSpace(envelope.Choices[0].Message.Content) == "" {
		t.Fatal("live selection response has no model content")
	}
	var selection manualAgentSelectionLiveResponse
	decoder := json.NewDecoder(strings.NewReader(envelope.Choices[0].Message.Content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&selection); err != nil {
		t.Fatalf("decode live selection model JSON: %v; content=%s", err, envelope.Choices[0].Message.Content)
	}
	return selection.Results
}

func marshalManualAgentSelectionLiveRequest(baseURL, model string, input manualAgentSelectionLiveInput) ([]byte, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal live selection input: %w", err)
	}
	requestBody := map[string]any{
		"model":       model,
		"temperature": 0,
		"max_tokens":  4096,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You evaluate DWS Agent command selection. For each case, interpret the natural-language scenario and choose exactly one canonical_path from candidates, or the literal string none when no candidate is appropriate. Return only JSON as {\"results\":[{\"id\":\"case id\",\"canonical_path\":\"candidate or none\"}]}. Return every case ID exactly once. Do not execute commands.",
			},
			{"role": "user", "content": string(inputJSON)},
		},
	}
	// Ark plan endpoints do not consistently accept response_format. Other
	// OpenAI-compatible endpoints get the stricter JSON-object request.
	if !strings.HasSuffix(strings.TrimRight(baseURL, "/"), "/api/plan/v3") {
		requestBody["response_format"] = map[string]string{"type": "json_object"}
	}
	return json.Marshal(requestBody)
}

func assertManualAgentSelectionLiveResults(t testing.TB, cases []cli.ManualAgentSelectionCase, results []manualAgentSelectionLiveResult) {
	t.Helper()
	byID := make(map[string]manualAgentSelectionLiveResult, len(results))
	for _, result := range results {
		if _, exists := byID[result.ID]; exists {
			t.Fatalf("live selection returned duplicate case ID %q", result.ID)
		}
		byID[result.ID] = result
	}
	for _, selectionCase := range cases {
		result, ok := byID[selectionCase.ID]
		if !ok {
			t.Errorf("live selection omitted case %q", selectionCase.ID)
			continue
		}
		delete(byID, selectionCase.ID)
		selected := strings.TrimSpace(result.CanonicalPath)
		if selected == "" {
			t.Errorf("live selection returned empty canonical for %q", selectionCase.ID)
			continue
		}
		if selected != "none" && !containsManualAgentSelectionCanonical(selectionCase.CandidateCanonicals, selected) {
			t.Errorf("live selection returned non-candidate %q for %q", selected, selectionCase.ID)
			continue
		}
		if selectionCase.ExpectedCanonical != "" && selected != selectionCase.ExpectedCanonical {
			t.Errorf("live positive selection %q = %q, want %q; scenario=%q", selectionCase.ID, selected, selectionCase.ExpectedCanonical, selectionCase.Scenario)
		}
		if selectionCase.ForbiddenCanonical != "" && selected == selectionCase.ForbiddenCanonical {
			t.Errorf("live negative selection %q chose forbidden %q; scenario=%q", selectionCase.ID, selected, selectionCase.Scenario)
		}
	}
	if len(byID) != 0 {
		unexpected := make([]string, 0, len(byID))
		for id := range byID {
			unexpected = append(unexpected, id)
		}
		sort.Strings(unexpected)
		t.Errorf("live selection returned unexpected case IDs: %s", strings.Join(unexpected, ", "))
	}
}

func validateManualAgentSelectionLiveBaseURL(raw, extraAllowed string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return fmt.Errorf("ARK_BASE_URL must be an absolute HTTP(S) API base without query or fragment")
	}
	if parsed.Scheme == "http" {
		if !manualAgentSelectionLoopbackHost(parsed.Hostname()) {
			return fmt.Errorf("ARK_BASE_URL may use plaintext HTTP only for a loopback test endpoint")
		}
		return nil
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("ARK_BASE_URL must use HTTPS, except for a loopback HTTP test endpoint")
	}
	allowed := map[string]bool{
		"https://ark.ap-southeast.bytepluses.com/api/v3": true,
		"https://ark.cn-beijing.volces.com/api/plan/v3":  true,
	}
	for _, candidate := range strings.Split(extraAllowed, ",") {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate != "" {
			allowed[candidate] = true
		}
	}
	normalized := strings.TrimRight(raw, "/")
	if !allowed[normalized] {
		return fmt.Errorf("ARK_BASE_URL %q is not allowlisted; use a built-in Ark base or add the exact HTTPS base to DWS_AGENT_SELECTION_ALLOWED_BASE_URLS", raw)
	}
	return nil
}

func manualAgentSelectionLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func containsManualAgentSelectionCanonical(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sanitizeManualAgentSelectionLiveTestID(value string) string {
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, "/", "_")
	return value
}

func TestManualAgentSelectionLiveResultContract(t *testing.T) {
	cases := []cli.ManualAgentSelectionCase{
		{ID: "sample.search/use_when/0", Scenario: "find an item", ExpectedCanonical: "sample.search", CandidateCanonicals: []string{"sample.create", "sample.search"}},
		{ID: "sample.search/avoid_when/0", Scenario: "create an item", ForbiddenCanonical: "sample.search", CandidateCanonicals: []string{"sample.create", "sample.search"}},
	}
	t.Run("accepts exact positive and negative choices", func(t *testing.T) {
		assertManualAgentSelectionLiveResults(t, cases, []manualAgentSelectionLiveResult{
			{ID: cases[0].ID, CanonicalPath: "sample.search"},
			{ID: cases[1].ID, CanonicalPath: "sample.create"},
		})
	})
}

func TestManualAgentSelectionLiveInputDoesNotLeakAssertionsOrRepeatCandidates(t *testing.T) {
	batch := []cli.ManualAgentSelectionCase{
		{ID: "sample.search/use_when/0", Scenario: "find an item", ExpectedCanonical: "sample.search", CandidateCanonicals: []string{"sample.create", "sample.search"}},
		{ID: "sample.search/avoid_when/0", Scenario: "create an item", ForbiddenCanonical: "sample.search", CandidateCanonicals: []string{"sample.create", "sample.search"}},
	}
	hints := cli.ManualAgentHintSet{Tools: map[string]cli.ManualAgentToolHint{
		"sample.create": {AgentSummary: "Create an item", UseWhen: []string{"create"}, AvoidWhen: []string{"find"}},
		"sample.search": {AgentSummary: "Search items", UseWhen: []string{"find"}, AvoidWhen: []string{"create"}},
	}}
	input := buildManualAgentSelectionLiveInput(batch, hints)
	data, err := marshalManualAgentSelectionLiveRequest("https://ark.cn-beijing.volces.com/api/plan/v3", "fixed-model", input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbiddenKey := range []string{"expected_canonical", "forbidden_canonical", "candidate_canonicals"} {
		if strings.Contains(string(data), forbiddenKey) {
			t.Fatalf("live model payload leaks local assertion field %q: %s", forbiddenKey, data)
		}
	}
	if len(input.Candidates) != 2 || len(input.Cases) != 2 {
		t.Fatalf("live model input = %+v", input)
	}
	if count := strings.Count(string(data), `\"candidates\"`); count != 1 {
		t.Fatalf("live request contains candidate table %d times, want once: %s", count, data)
	}
}

func TestValidateManualAgentSelectionLiveBaseURL(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		extraAllowed string
		wantErr      string
	}{
		{name: "built-in Ark", baseURL: "https://ark.cn-beijing.volces.com/api/plan/v3"},
		{name: "loopback localhost", baseURL: "http://localhost:8080/v1"},
		{name: "loopback IPv4", baseURL: "http://127.0.0.1:8080/v1"},
		{name: "loopback IPv6", baseURL: "http://[::1]:8080/v1"},
		{name: "allowlisted HTTPS extension", baseURL: "https://models.example.test/v1", extraAllowed: "https://models.example.test/v1"},
		{name: "plaintext remote", baseURL: "http://models.example.test/v1", wantErr: "only for a loopback"},
		{name: "HTTPS not allowlisted", baseURL: "https://models.example.test/v1", wantErr: "not allowlisted"},
		{name: "allowlist path mismatch", baseURL: "https://models.example.test/v2", extraAllowed: "https://models.example.test/v1", wantErr: "not allowlisted"},
		{name: "URL credentials", baseURL: "https://token@ark.cn-beijing.volces.com/api/plan/v3", wantErr: "absolute HTTP(S)"},
		{name: "query", baseURL: "https://ark.cn-beijing.volces.com/api/plan/v3?q=1", wantErr: "absolute HTTP(S)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateManualAgentSelectionLiveBaseURL(test.baseURL, test.extraAllowed)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validate base URL: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}
