// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageMinutesExportPackEmptyArtifactsE2E(t *testing.T) {
	for _, tc := range []struct {
		name, summary, keywords, transcript, artifacts string
		paragraphCount                                 int
		wantFailure                                    bool
	}{
		{"summary empty", "", `["test"]`, `[{"paragraphId":"p1","text":"测试。"}]`, "", 1, false},
		{"keywords empty", "summary", `[]`, `[{"paragraphId":"p1"}]`, "", 1, false},
		{"transcript empty", "summary", `["test"]`, `[]`, "", 0, true},
		{"all empty", "", `[]`, `[]`, "", 0, true},
		{"nonempty", "summary", `["test"]`, `[{"paragraphId":"p1"}]`, "", 1, false},
		{"summary only", "", `[]`, `[]`, "summary", 0, false},
		{"transcript only", "", `[]`, `[]`, "transcript", 0, true},
		{"whitespace preserved", " \n\t", `[]`, `[]`, "summary", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			summaryJSON, err := json.Marshal(tc.summary)
			if err != nil {
				t.Fatal(err)
			}
			caller := &minutesE2ECaller{responses: map[string][]string{
				"minutes/get_minutes_basic_info":    {`{"success":true,"result":{"taskUuid":"u1","title":"test"}}`},
				"minutes/get_minutes_ai_summary":    {`{"success":true,"result":{"fullSummary":` + string(summaryJSON) + `}}`},
				"minutes/get_minutes_keywords":      {`{"success":true,"result":{"keywords":` + tc.keywords + `}}`},
				"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":` + tc.transcript + `}}`},
				"minutes/list_minutes_todos":        {`{"success":true,"result":{"actions":[]}}`},
			}}
			args := []string{"minutes", "+export-pack", "--id", "u1", "--output", "pack", "--format", "json"}
			if tc.artifacts != "" {
				args = append(args, "--artifacts", tc.artifacts)
			}
			payload, _, err := runMinutesAlignmentCLI(t, caller, args...)
			if tc.wantFailure {
				if err == nil || payload["complete"] != false || payload["published"] != false {
					t.Fatalf("empty transcript accepted: payload=%#v err=%v", payload, err)
				}
				failures, ok := payload["failures"].([]any)
				if !ok || len(failures) != 1 || failures[0].(map[string]any)["artifact"] != "transcript" {
					t.Fatalf("failures=%#v", payload["failures"])
				}
				entries, readErr := os.ReadDir(".")
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("failed export left files: %v err=%v", entries, readErr)
				}
				return
			}
			if err != nil || payload["complete"] != true || payload["published"] != true {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			manifestRaw, err := os.ReadFile(filepath.Join("pack", "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct {
				Complete bool `json:"complete"`
				Files    map[string]struct {
					File     string `json:"file"`
					Complete bool   `json:"complete"`
					Size     int64  `json:"sizeBytes"`
				} `json:"files"`
			}
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatal(err)
			}
			wantFiles := 5
			if tc.artifacts != "" {
				wantFiles = 1
			}
			if !manifest.Complete || len(manifest.Files) != wantFiles {
				t.Fatalf("manifest=%s", manifestRaw)
			}
			for name, file := range manifest.Files {
				body, err := os.ReadFile(filepath.Join("pack", file.File))
				if err != nil {
					t.Fatal(err)
				}
				if !file.Complete || int64(len(body)) != file.Size {
					t.Fatalf("file=%#v", file)
				}
				switch name {
				case "summary":
					if string(body) != tc.summary {
						t.Fatalf("summary=%q", body)
					}
				case "transcript":
					var data map[string]any
					if err := json.Unmarshal(body, &data); err != nil {
						t.Fatal(err)
					}
					paragraphs, ok := data["paragraphList"].([]any)
					if !ok || len(paragraphs) != tc.paragraphCount || data["complete"] != true {
						t.Fatalf("transcript=%s", body)
					}
				case "keywords":
					var data map[string]json.RawMessage
					if err := json.Unmarshal(body, &data); err != nil {
						t.Fatal(err)
					}
					var words []string
					if err := json.Unmarshal(data["keywords"], &words); err != nil {
						t.Fatal(err)
					}
					got, err := json.Marshal(words)
					if err != nil || string(got) != tc.keywords {
						t.Fatalf("keywords=%s err=%v", got, err)
					}
				}
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesExportPackInvalidArtifactsRemainUnpublishedE2E(t *testing.T) {
	for _, tc := range []struct {
		name, artifact, tool string
		responses            []string
		failAt               int
		pageLimit            string
	}{
		{"summary unknown object", "summary", "get_minutes_ai_summary", []string{`{"success":true,"result":{"unknown":"value"}}`}, 0, "10"},
		{"summary missing result", "summary", "get_minutes_ai_summary", []string{`{"success":true}`}, 0, "10"},
		{"summary null result", "summary", "get_minutes_ai_summary", []string{`{"success":true,"result":null}`}, 0, "10"},
		{"summary array result", "summary", "get_minutes_ai_summary", []string{`{"success":true,"result":[]}`}, 0, "10"},
		{"summary empty without success", "summary", "get_minutes_ai_summary", []string{`{"result":{}}`}, 0, "10"},
		{"summary empty false", "summary", "get_minutes_ai_summary", []string{`{"success":false,"result":{}}`}, 0, "10"},
		{"summary empty string false", "summary", "get_minutes_ai_summary", []string{`{"success":"false","result":{}}`}, 0, "10"},
		{"summary empty invalid success", "summary", "get_minutes_ai_summary", []string{`{"success":{},"result":{}}`}, 0, "10"},
		{"summary empty conflicting error", "summary", "get_minutes_ai_summary", []string{`{"success":"true","errorCode":"FORBIDDEN","result":{}}`}, 0, "10"},
		{"summary empty conflicting ding error", "summary", "get_minutes_ai_summary", []string{`{"success":true,"dingOpenErrcode":403,"result":{}}`}, 0, "10"},
		{"summary null", "summary", "get_minutes_ai_summary", []string{`{"success":true,"result":{"fullSummary":null}}`}, 0, "10"},
		{"summary wrong type", "summary", "get_minutes_ai_summary", []string{`{"success":true,"result":{"fullSummary":[]}}`}, 0, "10"},
		{"summary business failure", "summary", "get_minutes_ai_summary", []string{`{"success":false,"errorMsg":"denied","result":{"fullSummary":""}}`}, 0, "10"},
		{"summary RPC failure", "summary", "get_minutes_ai_summary", nil, 1, "10"},
		{"transcript missing", "transcript", "get_minutes_transcription", []string{`{"success":true,"result":{}}`}, 0, "10"},
		{"transcript null", "transcript", "get_minutes_transcription", []string{`{"success":true,"result":{"paragraphList":null}}`}, 0, "10"},
		{"transcript wrong type", "transcript", "get_minutes_transcription", []string{`{"success":true,"result":{"paragraphList":""}}`}, 0, "10"},
		{"empty page missing cursor", "transcript", "get_minutes_transcription", []string{`{"success":true,"result":{"paragraphList":[],"hasNext":true}}`}, 0, "10"},
		{"empty page limit", "transcript", "get_minutes_transcription", []string{`{"success":true,"result":{"paragraphList":[],"hasNext":true,"nextToken":"n2"}}`}, 0, "1"},
		{"later page failure", "transcript", "get_minutes_transcription", []string{`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`}, 2, "10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			key := "minutes/" + tc.tool
			caller := &minutesE2ECaller{responses: map[string][]string{
				"minutes/get_minutes_basic_info": {`{"success":true,"result":{"taskUuid":"u1"}}`},
				key:                              tc.responses,
			}, failAt: map[string]int{key: tc.failAt}}
			payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", "basic,"+tc.artifact, "--page-limit", tc.pageLimit)
			if err == nil || payload["published"] != false || payload["complete"] != false {
				t.Fatalf("payload=%#v err=%v", payload, err)
			}
			failures, ok := payload["failures"].([]any)
			if !ok || len(failures) != 1 || failures[0].(map[string]any)["artifact"] != tc.artifact {
				t.Fatalf("failures=%#v", payload["failures"])
			}
			entries, err := os.ReadDir(".")
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed export left files: %v err=%v", entries, err)
			}
		})
	}
}

func TestCrossPlatformCoverageMinutesExportPackSuccessfulEmptySummaryObjectE2E(t *testing.T) {
	for _, success := range []string{`true`, `"true"`} {
		for _, artifacts := range []string{"basic,summary,keywords,transcript,todos", "summary"} {
			t.Run(success+"/"+artifacts, func(t *testing.T) {
				t.Chdir(t.TempDir())
				caller := &minutesE2ECaller{responses: map[string][]string{
					"minutes/get_minutes_basic_info":    {`{"success":true,"result":{"taskUuid":"u1"}}`},
					"minutes/get_minutes_ai_summary":    {`{"success":` + success + `,"result":{},"errorMsg":"ok"}`},
					"minutes/get_minutes_keywords":      {`{"success":true,"result":{"keywords":[]}}`},
					"minutes/get_minutes_transcription": {`{"success":true,"result":{"paragraphList":[{"paragraphId":"p1","text":"测试。"}]}}`},
					"minutes/list_minutes_todos":        {`{"success":true,"result":{"actions":[]}}`},
				}}
				payload, _, err := runMinutesAlignmentCLI(t, caller, "minutes", "+export-pack", "--id", "u1", "--output", "pack", "--artifacts", artifacts)
				if err != nil || payload["complete"] != true || payload["published"] != true {
					t.Fatalf("payload=%#v err=%v", payload, err)
				}
				body, err := os.ReadFile("pack/summary.md")
				if err != nil || len(body) != 0 {
					t.Fatalf("summary=%q err=%v", body, err)
				}
				raw, err := os.ReadFile("pack/manifest.json")
				if err != nil {
					t.Fatal(err)
				}
				var manifest struct {
					Complete bool `json:"complete"`
					Files    map[string]struct {
						File     string `json:"file"`
						Complete bool   `json:"complete"`
						Size     int64  `json:"sizeBytes"`
					} `json:"files"`
				}
				if err := json.Unmarshal(raw, &manifest); err != nil {
					t.Fatal(err)
				}
				wantFiles := 5
				if artifacts == "summary" {
					wantFiles = 1
				}
				if !manifest.Complete || len(manifest.Files) != wantFiles || manifest.Files["summary"].Size != 0 {
					t.Fatalf("manifest=%s", raw)
				}
				for _, file := range manifest.Files {
					info, err := os.Stat(filepath.Join("pack", file.File))
					if err != nil || !file.Complete || info.Size() != file.Size {
						t.Fatalf("file=%#v err=%v", file, err)
					}
				}
			})
		}
	}
}
