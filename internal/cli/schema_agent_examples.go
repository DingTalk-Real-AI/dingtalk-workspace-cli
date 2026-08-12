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
	"fmt"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AgentExampleMode controls only how an already contract-validated example is
// exercised. Contract validation is the default. dry_run is used only when the
// final ToolSpec publishes an explicit reviewed capability; contract_only
// remains a precise reviewed exception for such a capability whose runtime
// preconditions cannot be exercised safely and deterministically in the
// isolated test process.
type AgentExampleMode = contract.ExampleDispositionMode

const (
	AgentExampleModeContract     = contract.ExampleDispositionModeContract
	AgentExampleModeDryRun       = contract.ExampleDispositionModeDryRun
	AgentExampleModeContractOnly = contract.ExampleDispositionModeContractOnly
)

// AgentExampleReasonCode is a closed taxonomy for reviewed contract-only
// exceptions to an explicit dry-run capability.
type AgentExampleReasonCode = contract.ExampleDispositionReasonCode

const (
	AgentExampleReasonLocalState        = contract.ExampleDispositionReasonLocalState
	AgentExampleReasonStatefulPreflight = contract.ExampleDispositionReasonStatefulPreflight
)

// AgentExampleDisposition narrows one exact example with an explicit
// typed dry-run capability to contract-only. Index is a pointer so a missing
// field cannot silently select example zero.
//
// Dispositions are authored on the owning ContractFinal Selection.
type AgentExampleDisposition = contract.ExampleDisposition

// AgentExampleExecution is one resolved example and its effective test mode.
type AgentExampleExecution struct {
	CanonicalPath string
	Index         int
	Example       string
	Mode          AgentExampleMode
	DryRun        *contract.DryRunSpec
	ReasonCode    AgentExampleReasonCode
	Reason        string
	Source        AgentExampleDispositionSource
}

// AgentExampleDispositionSource distinguishes the normal typed-contract
// classification from narrow reviewed exceptions.
type AgentExampleDispositionSource string

const (
	AgentExampleDispositionDefault  AgentExampleDispositionSource = "default"
	AgentExampleDispositionReviewed AgentExampleDispositionSource = ProvenanceReviewedManual
)

// AgentExampleExecutionPlan is a stable, typed report used by the
// exhaustive real-Cobra dry-run test.
type AgentExampleExecutionPlan struct {
	Examples             []AgentExampleExecution
	Total                int
	Contract             int
	DryRun               int
	ContractOnly         int
	ReviewedContractOnly int
	ContractOnlyByReason map[AgentExampleReasonCode]int
}

// ValidateAgentExampleDelivery is the final generation gate. It validates every
// bound tool's ContractFinal Selection.Examples against the assembled typed
// SchemaRegistry.
func ValidateAgentExampleDelivery(bound BoundCommandRegistry, registry SchemaRegistry) (AgentExampleExecutionPlan, error) {
	return BuildAgentExampleExecutionPlan(bound, registry)
}

// agentExampleSelectionFn is the selection source for example planning.
// Tests may override it to inject ExampleDispositions without changing
// ContractFinal production wiring.
var agentExampleSelectionFn = contractFinalToolSelection

// BuildAgentExampleExecutionPlan validates every ContractFinal example against
// its real BoundCommand/Cobra contract. Runtime dry-run execution is opt-in and
// comes only from the final typed ToolSpec.
func BuildAgentExampleExecutionPlan(bound BoundCommandRegistry, registry SchemaRegistry) (AgentExampleExecutionPlan, error) {
	tools := make(map[string]ToolSpec, len(bound.Commands))
	for _, product := range registry.Products {
		for _, tool := range product.Tools {
			canonical := strings.TrimSpace(tool.Identity.CanonicalPath)
			if canonical == "" {
				return AgentExampleExecutionPlan{}, fmt.Errorf("typed SchemaRegistry contains a tool with empty canonical path")
			}
			if _, duplicate := tools[canonical]; duplicate {
				return AgentExampleExecutionPlan{}, fmt.Errorf("typed SchemaRegistry contains duplicate tool %q", canonical)
			}
			tools[canonical] = tool
		}
	}
	return buildAgentExampleExecutionPlan(bound, tools)
}

// FixtureAgentSelectionSet is a transitional Agent selection fixture type
// retained only so live-selection call sites can project candidate tables.
// Production paths must keep it empty; example planning reads ContractFinal.
type FixtureAgentSelectionSet struct {
	Revisions map[string]FixtureAgentSelectionRevision `json:"revisions,omitempty"`
	Products  map[string]FixtureAgentProductSelection  `json:"products,omitempty"`
	Tools     map[string]AgentToolSelection            `json:"tools,omitempty"`
}

// FixtureAgentSelectionRevision is retained for transitional fixtures only.
type FixtureAgentSelectionRevision struct {
	GeneratedBy   string `json:"generated_by"`
	Model         string `json:"model,omitempty"`
	PromptVersion string `json:"prompt_version,omitempty"`
	Reason        string `json:"reason"`
}

// FixtureAgentProductSelection is retained for transitional fixtures only.
type FixtureAgentProductSelection struct {
	AgentSummary string   `json:"agent_summary"`
	UseWhen      []string `json:"use_when"`
	AvoidWhen    []string `json:"avoid_when"`
	Reviewed     bool     `json:"reviewed"`
	Revision     string   `json:"revision"`
	Reason       string   `json:"reason"`
	Evidence     []string `json:"evidence"`
}

// AgentToolSelection is the tool-level selection projection used by example
// planning and live-selection fixtures. Production authority remains
// ContractFinal Selection on the owning leaf.
type AgentToolSelection struct {
	AgentSummary        string                    `json:"agent_summary"`
	UseWhen             []string                  `json:"use_when"`
	AvoidWhen           []string                  `json:"avoid_when"`
	Examples            []string                  `json:"examples"`
	ExampleDispositions []AgentExampleDisposition `json:"example_dispositions,omitempty"`
	Reviewed            bool                      `json:"reviewed"`
	Revision            string                    `json:"revision"`
	Reason              string                    `json:"reason"`
	Evidence            []string                  `json:"evidence"`
}

func buildAgentExampleExecutionPlan(bound BoundCommandRegistry, typedTools map[string]ToolSpec) (AgentExampleExecutionPlan, error) {
	plan := AgentExampleExecutionPlan{
		ContractOnlyByReason: map[AgentExampleReasonCode]int{},
	}
	canonicalPaths := make([]string, 0, len(bound.Commands))
	for _, command := range bound.Commands {
		canonical := strings.TrimSpace(command.CanonicalPath)
		if !contractfinal.HasRuntimeContractFinal(command.PrimaryCommand) {
			return AgentExampleExecutionPlan{}, fmt.Errorf("bound tool %q has no ContractFinal declaration; Schema examples require leaf Schema.Selection", canonical)
		}
		canonicalPaths = append(canonicalPaths, canonical)
	}
	sort.Strings(canonicalPaths)
	for _, canonical := range canonicalPaths {
		spec, ok := bound.ByCanonical[canonical]
		if !ok {
			return AgentExampleExecutionPlan{}, fmt.Errorf("example plan references unknown canonical tool %q", canonical)
		}
		selection := agentExampleSelectionFn(spec.PrimaryCommand)
		var typedTool ToolSpec
		if typedTools != nil {
			var found bool
			typedTool, found = typedTools[canonical]
			if !found {
				return AgentExampleExecutionPlan{}, fmt.Errorf("example tool %q is missing from final typed SchemaRegistry", canonical)
			}
		}
		if len(selection.Examples) == 0 {
			return AgentExampleExecutionPlan{}, fmt.Errorf("ContractFinal tool %s requires non-empty Selection.Examples", canonical)
		}
		if len(selection.Examples) > 2 {
			return AgentExampleExecutionPlan{}, fmt.Errorf("ContractFinal tool %s has %d examples; maximum is 2", canonical, len(selection.Examples))
		}
		if err := validateAgentExampleDispositions(canonical, selection.Examples, selection.ExampleDispositions); err != nil {
			return AgentExampleExecutionPlan{}, err
		}
		dispositions := make(map[int]AgentExampleDisposition, len(selection.ExampleDispositions))
		for _, disposition := range selection.ExampleDispositions {
			dispositions[*disposition.Index] = disposition
		}
		paths := []agentExamplePath{{Path: spec.PrimaryCLIPath, Argv: strings.Fields(spec.PrimaryCLIPath), Command: spec.PrimaryCommand}}
		for _, alias := range spec.AliasCommands {
			paths = append(paths, agentExamplePath{Path: alias.Path, Argv: strings.Fields(alias.Path), Command: alias.Command})
		}
		sort.Slice(paths, func(i, j int) bool {
			if len(paths[i].Argv) != len(paths[j].Argv) {
				return len(paths[i].Argv) > len(paths[j].Argv)
			}
			return paths[i].Path < paths[j].Path
		})
		for index, example := range selection.Examples {
			argv, err := tokenizeAgentExample(example)
			if err != nil {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example has invalid argv syntax: %w", canonical, err)
			}
			if len(argv) < 2 || argv[0] != "dws" {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example must start with dws: %q", canonical, example)
			}
			for _, argument := range argv[1:] {
				if argument == "--yes" || strings.HasPrefix(argument, "--yes=") {
					return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example must not bypass confirmation with --yes", canonical)
				}
				if argument == "--help" || strings.HasPrefix(argument, "--help=") || argument == "-h" || strings.HasPrefix(argument, "-h=") {
					return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example must demonstrate execution, not only --help", canonical)
				}
			}
			remainder, matched, ok := matchAgentExamplePath(argv, paths)
			if !ok {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example does not use its reviewed primary/alias path: %q", canonical, example)
			}
			if matched.Command == nil {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s reviewed path %q has no bound Cobra command", canonical, matched.Path)
			}
			constraints, err := strictCompatibilityConstraints(matched.Command, canonical)
			if err != nil {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example for %q has invalid executable constraints: %w", canonical, matched.Path, err)
			}
			if typedTools != nil {
				constraints.MutuallyExclusive = append(constraints.MutuallyExclusive, typedTool.Constraints.MutuallyExclusive...)
				constraints.RequireOneOf = append(constraints.RequireOneOf, typedTool.Constraints.RequireOneOf...)
				constraints.RequireTogether = append(constraints.RequireTogether, typedTool.Constraints.RequireTogether...)
				constraints = normalizeRuntimeSchemaConstraints(constraints)
			}
			positionals, err := strictCompatibilityPositionals(matched.Command)
			if err != nil {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example for %q has invalid executable positionals: %w", canonical, matched.Path, err)
			}
			if typedTools != nil {
				positionals = mergeAgentExamplePositionals(positionals, typedTool.Positionals)
			}
			if err := validateAgentExampleCobraContract(matched.Command, remainder, constraints, positionals); err != nil {
				return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example for %q: %w", canonical, matched.Path, err)
			}

			execution := AgentExampleExecution{
				CanonicalPath: canonical,
				Index:         index,
				Example:       example,
				Mode:          AgentExampleModeContract,
				Source:        AgentExampleDispositionDefault,
			}
			if typedTool.DryRun != nil {
				dryRun := *typedTool.DryRun
				execution.DryRun = &dryRun
				execution.Mode = AgentExampleModeDryRun
			}
			disposition, hasDisposition := dispositions[index]
			if hasDisposition {
				if typedTools != nil && execution.DryRun == nil {
					return AgentExampleExecutionPlan{}, fmt.Errorf("tool %s example disposition index %d narrows no explicit dry_run capability", canonical, index)
				}
				execution.Mode = disposition.Mode
				execution.ReasonCode = disposition.ReasonCode
				execution.Reason = strings.TrimSpace(disposition.Reason)
				execution.Source = AgentExampleDispositionReviewed
				plan.ContractOnly++
				plan.ReviewedContractOnly++
				plan.ContractOnlyByReason[disposition.ReasonCode]++
			} else if execution.DryRun != nil {
				plan.DryRun++
			} else {
				plan.Contract++
			}
			plan.Examples = append(plan.Examples, execution)
			plan.Total++
		}
	}
	for canonical := range typedTools {
		found := false
		for _, command := range bound.Commands {
			if strings.TrimSpace(command.CanonicalPath) == canonical {
				found = true
				break
			}
		}
		if !found {
			return AgentExampleExecutionPlan{}, fmt.Errorf("final typed SchemaRegistry tool %q is missing from BoundCommandRegistry", canonical)
		}
	}
	return plan, nil
}

type agentExamplePath struct {
	Path    string
	Argv    []string
	Command *cobra.Command
}

func matchAgentExamplePath(argv []string, paths []agentExamplePath) ([]string, agentExamplePath, bool) {
	if len(argv) == 0 || argv[0] != "dws" {
		return nil, agentExamplePath{}, false
	}
	for index := range paths {
		if paths[index].Argv == nil {
			paths[index].Argv = strings.Fields(strings.TrimSpace(paths[index].Path))
		}
		pathArgv := paths[index].Argv
		if len(pathArgv) == 0 || len(argv) < len(pathArgv)+1 {
			continue
		}
		matches := true
		for offset := range pathArgv {
			if argv[offset+1] != pathArgv[offset] {
				matches = false
				break
			}
		}
		if matches {
			return argv[len(pathArgv)+1:], paths[index], true
		}
	}
	return nil, agentExamplePath{}, false
}

func validateAgentExampleDispositions(canonical string, examples []string, dispositions []AgentExampleDisposition) error {
	seen := make(map[int]bool, len(dispositions))
	for _, disposition := range dispositions {
		if disposition.Index == nil {
			return fmt.Errorf("tool %s example disposition requires index", canonical)
		}
		index := *disposition.Index
		if index < 0 || index >= len(examples) {
			return fmt.Errorf("tool %s example disposition index %d is out of range for %d examples", canonical, index, len(examples))
		}
		if seen[index] {
			return fmt.Errorf("tool %s has duplicate example disposition index %d", canonical, index)
		}
		seen[index] = true
		if !disposition.Reviewed {
			return fmt.Errorf("tool %s example disposition index %d must be reviewed", canonical, index)
		}
		if disposition.Mode != AgentExampleModeContractOnly {
			return fmt.Errorf("tool %s example disposition index %d has invalid mode %q; only %q may narrow an explicit dry_run capability", canonical, index, disposition.Mode, AgentExampleModeContractOnly)
		}
		if !validAgentExampleReasonCode(disposition.ReasonCode) {
			return fmt.Errorf("tool %s example disposition index %d has invalid reason_code %q", canonical, index, disposition.ReasonCode)
		}
		if strings.TrimSpace(disposition.Reason) == "" {
			return fmt.Errorf("tool %s example disposition index %d requires a non-empty reason", canonical, index)
		}
	}
	return nil
}

func validAgentExampleReasonCode(code AgentExampleReasonCode) bool {
	switch code {
	case AgentExampleReasonLocalState, AgentExampleReasonStatefulPreflight:
		return true
	default:
		return false
	}
}

func tokenizeAgentExample(input string) ([]string, error) {
	var (
		argv         []string
		current      strings.Builder
		quote        byte
		tokenStarted bool
	)
	flush := func() {
		if tokenStarted {
			argv = append(argv, current.String())
			current.Reset()
			tokenStarted = false
		}
	}
	for index := 0; index < len(input); index++ {
		character := input[index]
		if quote != 0 {
			if character == quote {
				quote = 0
				continue
			}
			if quote == '"' && character == '\\' {
				if index+1 >= len(input) {
					return nil, fmt.Errorf("trailing escape in double-quoted value")
				}
				index++
				current.WriteByte(input[index])
				continue
			}
			if quote == '"' && (character == '`' || character == '$') {
				return nil, fmt.Errorf("shell expansion is not allowed")
			}
			current.WriteByte(character)
			continue
		}

		switch character {
		case ' ', '\t':
			flush()
		case '\r', '\n':
			return nil, fmt.Errorf("unquoted newline shell operator is not allowed")
		case '\'', '"':
			quote = character
			tokenStarted = true
		case '\\':
			if index+1 >= len(input) {
				return nil, fmt.Errorf("trailing escape")
			}
			index++
			current.WriteByte(input[index])
			tokenStarted = true
		case '<':
			placeholder, next, ok := agentExamplePlaceholderAt(input, index)
			if !ok {
				return nil, fmt.Errorf("shell redirection operator %q is not allowed", string(character))
			}
			current.WriteString(placeholder)
			tokenStarted = true
			index = next
		case '>', ';', '|', '&', '(', ')':
			return nil, fmt.Errorf("shell operator %q is not allowed", string(character))
		case '`', '$':
			return nil, fmt.Errorf("shell expansion is not allowed")
		case '#':
			return nil, fmt.Errorf("shell comments are not allowed")
		default:
			current.WriteByte(character)
			tokenStarted = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted value")
	}
	flush()
	for _, argument := range argv {
		if argument == "--" {
			return nil, fmt.Errorf("the -- argument terminator is not allowed")
		}
	}
	return argv, nil
}

// ParseAgentExampleArgv exposes the shell-free argv parser used by
// example validation and dry-run tests.
func ParseAgentExampleArgv(input string) ([]string, error) {
	return tokenizeAgentExample(input)
}

func agentExamplePlaceholderAt(input string, start int) (string, int, bool) {
	endOffset := strings.IndexByte(input[start+1:], '>')
	if endOffset < 1 {
		return "", start, false
	}
	end := start + 1 + endOffset
	body := input[start+1 : end]
	for index := 0; index < len(body); index++ {
		character := body[index]
		if strings.ContainsRune(" \t\r\n<>;&|`$()#'\"\\", rune(character)) {
			return "", start, false
		}
	}
	return input[start : end+1], end, true
}

func validateAgentExampleCobraContract(command *cobra.Command, arguments []string, constraints RuntimeSchemaConstraints, positionalSpecs []contract.RuntimeSchemaPositional) error {
	if command == nil {
		return fmt.Errorf("bound Cobra command is nil")
	}
	providedFacts := map[string]bool{}
	positionals := make([]string, 0)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return fmt.Errorf("the -- argument terminator is not allowed")
		}
		if strings.HasPrefix(argument, "--") {
			nameAndValue := strings.TrimPrefix(argument, "--")
			name, _, hasValue := strings.Cut(nameAndValue, "=")
			if name == "" {
				return fmt.Errorf("invalid empty long flag")
			}
			if name == "help" {
				return fmt.Errorf("must demonstrate execution, not only --help")
			}
			flag := runtimeCommandFlag(command, name)
			if flag == nil {
				return fmt.Errorf("uses unknown flag --%s", name)
			}
			providedFacts[flag.Name] = true
			if !hasValue && flag.NoOptDefVal == "" {
				if index+1 >= len(arguments) {
					return fmt.Errorf("flag --%s requires a value", name)
				}
				index++
			}
			continue
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			positionals = append(positionals, argument)
			continue
		}

		shorthandsAndValue := strings.TrimPrefix(argument, "-")
		shorthands, _, hasExplicitValue := strings.Cut(shorthandsAndValue, "=")
		for offset := 0; offset < len(shorthands); offset++ {
			shorthand := shorthands[offset : offset+1]
			if shorthand[0] >= 0x80 {
				return fmt.Errorf("uses invalid non-ASCII shorthand flag")
			}
			if shorthand == "h" {
				return fmt.Errorf("must demonstrate execution, not only -h")
			}
			flag := runtimeCommandFlagByShorthand(command, shorthand)
			if flag == nil {
				return fmt.Errorf("uses unknown shorthand flag -%s", shorthand)
			}
			providedFacts[flag.Name] = true
			if flag.NoOptDefVal == "" {
				if offset+1 < len(shorthands) || hasExplicitValue {
					break
				}
				if index+1 >= len(arguments) {
					return fmt.Errorf("shorthand flag -%s requires a value", shorthand)
				}
				index++
				break
			}
		}
	}
	missingRequired := make([]string, 0)
	visitAgentExampleCommandFlags(command, func(flag *pflag.Flag) {
		if flag != nil && len(flag.Annotations[cobra.BashCompOneRequiredFlag]) > 0 && !providedFacts[flag.Name] {
			missingRequired = append(missingRequired, "--"+flag.Name)
		}
	})
	if len(missingRequired) != 0 {
		sort.Strings(missingRequired)
		return fmt.Errorf("missing required flag(s): %s", strings.Join(missingRequired, ", "))
	}
	missingPositionals := make([]string, 0)
	for _, positional := range positionalSpecs {
		provided := positional.Index >= 0 && positional.Index < len(positionals)
		if positional.Variadic {
			provided = positional.Index >= 0 && len(positionals) > positional.Index
		}
		if provided && strings.TrimSpace(positional.Name) != "" {
			providedFacts[strings.TrimLeft(strings.TrimSpace(positional.Name), "-")] = true
		}
		if positional.Required && !provided {
			missingPositionals = append(missingPositionals, strings.TrimSpace(positional.Name))
		}
	}
	if len(missingPositionals) != 0 {
		sort.Strings(missingPositionals)
		return fmt.Errorf("missing required positional argument(s): %s", strings.Join(missingPositionals, ", "))
	}
	if err := validateAgentExampleConstraints(providedFacts, constraints); err != nil {
		return err
	}
	if command.Args != nil && len(positionalSpecs) == 0 {
		if err := command.Args(command, positionals); err != nil {
			return fmt.Errorf("invalid positional arguments: %w", err)
		}
	}
	return nil
}

func mergeAgentExamplePositionals(groups ...[]contract.RuntimeSchemaPositional) []contract.RuntimeSchemaPositional {
	byIdentity := map[string]contract.RuntimeSchemaPositional{}
	for _, group := range groups {
		for _, positional := range group {
			key := fmt.Sprintf("%d\x00%s", positional.Index, strings.TrimSpace(positional.Name))
			if _, exists := byIdentity[key]; !exists {
				byIdentity[key] = positional
			}
		}
	}
	result := make([]contract.RuntimeSchemaPositional, 0, len(byIdentity))
	for _, positional := range byIdentity {
		result = append(result, positional)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Index != result[j].Index {
			return result[i].Index < result[j].Index
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func validateAgentExampleConstraints(provided map[string]bool, constraints RuntimeSchemaConstraints) error {
	for _, group := range constraints.RequireOneOf {
		if agentExampleProvidedFlagCount(provided, group) == 0 {
			return fmt.Errorf("missing require_one_of flags: %s", agentExampleFlagGroup(group))
		}
	}
	for _, group := range constraints.RequireTogether {
		providedCount := agentExampleProvidedFlagCount(provided, group)
		if providedCount != 0 && providedCount != len(group) {
			return fmt.Errorf("incomplete require_together flags: %s", agentExampleFlagGroup(group))
		}
	}
	for _, group := range constraints.MutuallyExclusive {
		if agentExampleProvidedFlagCount(provided, group) > 1 {
			return fmt.Errorf("mutually_exclusive flags used together: %s", agentExampleFlagGroup(group))
		}
	}
	return nil
}

func agentExampleProvidedFlagCount(provided map[string]bool, group []string) int {
	count := 0
	seen := map[string]bool{}
	for _, raw := range group {
		name := strings.TrimLeft(strings.TrimSpace(raw), "-")
		if name != "" && !seen[name] && provided[name] {
			seen[name] = true
			count++
		}
	}
	return count
}

func agentExampleFlagGroup(group []string) string {
	flags := make([]string, 0, len(group))
	for _, raw := range group {
		if name := strings.TrimLeft(strings.TrimSpace(raw), "-"); name != "" {
			flags = append(flags, "--"+name)
		}
	}
	sort.Strings(flags)
	return strings.Join(flags, ", ")
}

func visitAgentExampleCommandFlags(command *cobra.Command, visit func(*pflag.Flag)) {
	seen := map[string]bool{}
	visitSet := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(flag *pflag.Flag) {
			if seen[flag.Name] {
				return
			}
			seen[flag.Name] = true
			visit(flag)
		})
	}
	visitSet(command.Flags())
	for current := command; current != nil; current = current.Parent() {
		visitSet(current.PersistentFlags())
	}
}

func runtimeCommandFlagByShorthand(command *cobra.Command, shorthand string) *pflag.Flag {
	if command == nil || len(shorthand) != 1 {
		return nil
	}
	if flag := command.Flags().ShorthandLookup(shorthand); flag != nil {
		return flag
	}
	for current := command; current != nil; current = current.Parent() {
		if flag := current.PersistentFlags().ShorthandLookup(shorthand); flag != nil {
			return flag
		}
	}
	return nil
}
