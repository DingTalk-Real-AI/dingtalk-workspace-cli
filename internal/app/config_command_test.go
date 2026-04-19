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
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
)

func seedTestConfig(t *testing.T) {
	t.Helper()
	configmeta.Reset()
	t.Cleanup(configmeta.Reset)

	configmeta.Register(configmeta.ConfigItem{
		Name: "DWS_CONFIG_DIR", Category: configmeta.CategoryCore,
		Description: "覆盖默认配置目录", DefaultValue: "~/.dws",
	})
	configmeta.Register(configmeta.ConfigItem{
		Name: "DWS_CLIENT_SECRET", Category: configmeta.CategoryAuth,
		Description: "OAuth AppSecret", Sensitive: true,
	})
	configmeta.Register(configmeta.ConfigItem{
		Name: "DWS_CATALOG_FIXTURE", Category: configmeta.CategoryDebug,
		Description: "目录 Fixture 路径", Hidden: true,
	})
}

func TestConfigListTable(t *testing.T) {
	seedTestConfig(t)

	cmd := newConfigListCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "DWS_CONFIG_DIR") {
		t.Error("expected DWS_CONFIG_DIR in output")
	}
	if !strings.Contains(out, "DWS_CLIENT_SECRET") {
		t.Error("expected DWS_CLIENT_SECRET in output")
	}
	// Hidden items should be excluded by default
	if strings.Contains(out, "DWS_CATALOG_FIXTURE") {
		t.Error("expected DWS_CATALOG_FIXTURE to be hidden")
	}
}

func TestConfigListShowHidden(t *testing.T) {
	seedTestConfig(t)

	cmd := newConfigListCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--show-hidden"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "DWS_CATALOG_FIXTURE") {
		t.Error("expected DWS_CATALOG_FIXTURE with --show-hidden")
	}
}

func TestConfigListCategory(t *testing.T) {
	seedTestConfig(t)

	cmd := newConfigListCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--category", "auth"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "DWS_CLIENT_SECRET") {
		t.Error("expected DWS_CLIENT_SECRET for auth category")
	}
	if strings.Contains(out, "DWS_CONFIG_DIR") {
		t.Error("DWS_CONFIG_DIR should not appear for auth category")
	}
}

func TestConfigListJSON(t *testing.T) {
	seedTestConfig(t)

	cmd := newConfigListCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--json", "--show-hidden"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["kind"] != "config_list" {
		t.Errorf("expected kind=config_list, got %v", result["kind"])
	}
	count, ok := result["count"].(float64)
	if !ok || count != 3 {
		t.Errorf("expected count=3, got %v", result["count"])
	}
}

func TestConfigListShowValues(t *testing.T) {
	seedTestConfig(t)

	t.Setenv("DWS_CONFIG_DIR", "/custom/dir")
	t.Setenv("DWS_CLIENT_SECRET", "supersecret123")

	cmd := newConfigListCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--show-values"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "/custom/dir") {
		t.Error("expected actual value for DWS_CONFIG_DIR")
	}
	if strings.Contains(out, "supersecret123") {
		t.Error("sensitive value should be masked")
	}
	if !strings.Contains(out, "当前值") {
		t.Error("expected '当前值' column header")
	}
}

func TestConfigListEmpty(t *testing.T) {
	configmeta.Reset()
	defer configmeta.Reset()

	cmd := newConfigListCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "没有找到") {
		t.Error("expected empty message")
	}
}
