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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/skillgen"
)

func main() {
	var source string
	var fixture string
	var snapshot string
	var outputRoot string

	flag.StringVar(&source, "source", string(skillgen.CatalogSourceEnv), "Catalog source: fixture, env, or snapshot")
	flag.StringVar(&fixture, "fixture", "", "Path to the catalog fixture that seeds generated outputs")
	flag.StringVar(&snapshot, "snapshot", "", "Path to the catalog snapshot used by --source snapshot")
	flag.StringVar(&outputRoot, "output-root", ".", "Directory root where generated outputs are written")
	flag.Parse()

	catalogPath := fixture
	if strings.EqualFold(strings.TrimSpace(source), string(skillgen.CatalogSourceSnapshot)) {
		catalogPath = snapshot
	}

	catalog, err := skillgen.LoadCatalogWithSource(context.Background(), source, catalogPath)
	if err != nil {
		fail(err)
	}

	artifacts, err := skillgen.Generate(catalog)
	if err != nil {
		fail(err)
	}
	if err := skillgen.WriteArtifacts(outputRoot, artifacts); err != nil {
		fail(err)
	}
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate: %v\n", err)
	os.Exit(1)
}
