package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type generatedCatalogSnapshot struct {
	Products []generatedCatalogProduct `json:"products"`
}

type generatedCatalogProduct struct {
	Tools []generatedCatalogTool `json:"tools"`
}

type generatedCatalogTool struct {
	CanonicalPath string `json:"canonical_path"`
}

type generatedSchemaExpectation struct {
	Path    string              `json:"path"`
	CLIPath []string            `json:"cli_path"`
	Flags   []generatedFlagSpec `json:"flags"`
}

type generatedFlagSpec struct {
	FlagName string `json:"flag_name"`
	Alias    string `json:"alias"`
}

func TestGeneratedSchemaDocsCoverCLICommandsAndPayloadFallback(t *testing.T) {
	expectations, err := loadGeneratedSchemaExpectations(generatedSchemaRoot())
	if err != nil {
		t.Fatalf("loadGeneratedSchemaExpectations() error = %v", err)
	}
	if len(expectations) == 0 {
		t.Fatal("loadGeneratedSchemaExpectations() returned no expectations")
	}

	catalog, err := (cli.FixtureLoader{Path: generatedCatalogPath()}).Load(context.Background())
	if err != nil {
		t.Fatalf("FixtureLoader.Load() error = %v", err)
	}

	root := &cobra.Command{Use: "dws"}
	if err := cli.AddCanonicalProducts(root, cli.StaticLoader{Catalog: catalog}, executor.EchoRunner{}); err != nil {
		t.Fatalf("AddCanonicalProducts() error = %v", err)
	}

	for _, expectation := range expectations {
		expectation := expectation
		t.Run(expectation.Path, func(t *testing.T) {
			flags, helpOutput, err := inspectToolCommand(root, expectation.CLIPath)
			if err != nil {
				t.Fatalf("inspectToolCommand(%v) error = %v", expectation.CLIPath, err)
			}

			for _, fallback := range []string{"json", "params"} {
				if _, ok := flags[fallback]; !ok {
					t.Fatalf("missing --%s fallback for %s\nhelp:\n%s", fallback, expectation.Path, helpOutput)
				}
			}

			for _, flag := range expectation.Flags {
				if _, ok := flags[flag.FlagName]; !ok {
					t.Fatalf("missing documented flag --%s for %s\nhelp:\n%s", flag.FlagName, expectation.Path, helpOutput)
				}
				if alias := strings.TrimSpace(flag.Alias); alias != "" && alias != flag.FlagName {
					if _, ok := flags[alias]; !ok {
						t.Fatalf("missing documented alias --%s for %s\nhelp:\n%s", alias, expectation.Path, helpOutput)
					}
				}
			}
		})
	}
}

func loadGeneratedSchemaExpectations(schemaRoot string) ([]generatedSchemaExpectation, error) {
	data, err := os.ReadFile(filepath.Join(schemaRoot, "catalog.json"))
	if err != nil {
		return nil, fmt.Errorf("read catalog snapshot: %w", err)
	}

	var snapshot generatedCatalogSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("decode catalog snapshot: %w", err)
	}

	expectations := make([]generatedSchemaExpectation, 0)
	for _, product := range snapshot.Products {
		for _, tool := range product.Tools {
			if strings.TrimSpace(tool.CanonicalPath) == "" {
				continue
			}

			docPath := filepath.Join(schemaRoot, tool.CanonicalPath+".json")
			docData, err := os.ReadFile(docPath)
			if err != nil {
				return nil, fmt.Errorf("read schema doc %s: %w", docPath, err)
			}

			var expectation generatedSchemaExpectation
			if err := json.Unmarshal(docData, &expectation); err != nil {
				return nil, fmt.Errorf("decode schema doc %s: %w", docPath, err)
			}
			if expectation.Path == "" {
				return nil, fmt.Errorf("schema doc %s missing path", docPath)
			}
			if len(expectation.CLIPath) == 0 {
				return nil, fmt.Errorf("schema doc %s missing cli_path", docPath)
			}

			expectations = append(expectations, expectation)
		}
	}

	return expectations, nil
}

func inspectToolCommand(root *cobra.Command, cliPath []string) (map[string]*pflag.Flag, string, error) {
	current := root
	for _, token := range cliPath {
		next := findCommandByName(current, token)
		if next == nil {
			return nil, "", fmt.Errorf("missing %q under %s", token, current.CommandPath())
		}
		current = next
	}

	var out bytes.Buffer
	current.SetOut(&out)
	current.SetErr(&out)
	if err := current.Help(); err != nil {
		return nil, "", err
	}

	flags := make(map[string]*pflag.Flag)
	current.Flags().VisitAll(func(flag *pflag.Flag) {
		flags[flag.Name] = flag
	})
	return flags, out.String(), nil
}

func findCommandByName(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func generatedSchemaRoot() string {
	return filepath.Join("..", "..", "skills", "generated", "docs", "schema")
}

func generatedCatalogPath() string {
	return filepath.Join(generatedSchemaRoot(), "catalog.json")
}
