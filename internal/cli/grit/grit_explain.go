package grit

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/gritql"
	"github.com/greppleai/grepple/internal/parser"
)

const gritExplainSchema = "grepple-grit-explain-v1"

type gritExplainArgs struct {
	QueryFile            string `arg:"-f,--query-file" placeholder:"PATH" help:"read the GritQL query from PATH (- for stdin)"`
	JSON                 bool   `arg:"--json" help:"emit complete machine-readable compile output"`
	MaxOutputBytes       int    `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human output (default 16384; 0 = unlimited; JSON is uncapped)"`
	MaxPatternBytes      int    `arg:"--max-pattern-bytes" placeholder:"N" help:"bound query source bytes"`
	MaxRegexBytes        int    `arg:"--max-regex-bytes" placeholder:"N" help:"bound one regex constraint"`
	MaxRegexInstructions int    `arg:"--max-regex-instructions" placeholder:"N" help:"bound compiled regex instructions"`
	MaxParseDepth        int    `arg:"--max-parse-depth" placeholder:"N" help:"bound query and structural recursion depth"`
	Query                string `arg:"positional" placeholder:"QUERY"`
}

func (gritExplainArgs) Description() string {
	return "Compile without scanning source and explain the target, grammar wrappers, features, and metavariable roles."
}

type gritExplainOutput struct {
	Schema          string                      `json:"schema"`
	OK              bool                        `json:"ok"`
	Compatibility   string                      `json:"compatibility,omitempty"`
	Language        string                      `json:"language,omitempty"`
	GrammarABI      uint32                      `json:"grammarAbi,omitempty"`
	Grammar         string                      `json:"grammarFingerprint,omitempty"`
	Features        []string                    `json:"features"`
	Interpretations []gritExplainInterpretation `json:"interpretations"`
	Variables       []gritExplainVariable       `json:"variables"`
	Diagnostics     []gritExplainDiagnostic     `json:"diagnostics"`
}

type gritExplainInterpretation struct {
	ExpressionKind string   `json:"expressionKind"`
	Context        string   `json:"context"`
	RootKind       string   `json:"rootKind,omitempty"`
	RootSlot       string   `json:"rootSlot,omitempty"`
	Roles          []string `json:"roles"`
}

type gritExplainVariable struct {
	ID           uint32   `json:"id"`
	Name         string   `json:"name"`
	FirstRange   string   `json:"firstRange"`
	Occurrences  int      `json:"occurrences"`
	BindingKinds []string `json:"bindingKinds"`
	Roles        []string `json:"roles"`
}

type gritExplainDiagnostic struct {
	Code     string `json:"code"`
	Class    string `json:"class"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Range    string `json:"range,omitempty"`
}

type gritVariableExplanation struct {
	occurrences  map[string]bool
	bindingKinds map[string]bool
	roles        map[string]bool
}

func runGritExplain(args []string, dependencies Dependencies) error {
	values := gritExplainArgs{MaxOutputBytes: DefaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple grit explain"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			output := dependencies.Stdout
			if output == nil {
				output = os.Stdout
			}
			argumentParser.WriteHelp(output)
			fmt.Fprintln(output, "No source files are read. Human diagnostics are bounded; JSON is complete under compile limits.")
			return nil
		}
		return err
	}
	return executeGritExplain(&values, dependencies)
}

func executeGritExplain(values *ExplainArgs, dependencies Dependencies) error {
	if err := validateGritExplainArgs(*values); err != nil {
		return err
	}
	query, err := loadGritQuery(gritArgs{Query: values.Query, QueryFile: values.QueryFile})
	if err != nil {
		return err
	}
	output := explainGritQuery(query, *values)
	if err := outputGritExplain(*values, output, dependencies); err != nil {
		return err
	}
	if !output.OK {
		dependencies.requestExit(1)
	}
	return nil
}

func validateGritExplainArgs(values gritExplainArgs) error {
	if values.Query == "" && values.QueryFile == "" {
		return fmt.Errorf("grit explain requires query text or --query-file")
	}
	if values.Query != "" && values.QueryFile != "" {
		return fmt.Errorf("grit explain query text and query file cannot be used together")
	}
	if values.MaxOutputBytes < 0 || values.MaxPatternBytes < 0 || values.MaxRegexBytes < 0 || values.MaxRegexInstructions < 0 || values.MaxParseDepth < 0 {
		return fmt.Errorf("grit explain limits must not be negative")
	}
	return nil
}

func outputGritExplain(values gritExplainArgs, output gritExplainOutput, dependencies Dependencies) error {
	if values.JSON {
		return stdoutWriter(dependencies).writeJSON(output)
	}
	if err := renderGritExplain(output, values.MaxOutputBytes, dependencies); err != nil && !errors.Is(err, errOutputTruncated) {
		return err
	}
	return nil
}

func explainGritQuery(query string, values gritExplainArgs) gritExplainOutput {
	output := gritExplainOutput{Schema: gritExplainSchema, Features: []string{}, Interpretations: []gritExplainInterpretation{}, Variables: []gritExplainVariable{}, Diagnostics: []gritExplainDiagnostic{}}
	program, err := gritql.Compile([]byte(query), gritql.CompileOptions{MaxPatternBytes: values.MaxPatternBytes, MaxRegexBytes: values.MaxRegexBytes, MaxRegexInstructions: values.MaxRegexInstructions, MaxDepth: values.MaxParseDepth})
	if err != nil {
		output.Diagnostics = []gritExplainDiagnostic{gritCompileDiagnostic(err)}
		return output
	}
	output.OK = true
	output.Compatibility = program.Compatibility()
	output.Language = program.Language()
	if capabilities, ok := parser.CapabilitiesForLanguage(output.Language); ok {
		output.GrammarABI = capabilities.GrammarABI
		output.Grammar = capabilities.GrammarFingerprint
	}
	output.Features = gritProgramFeatures(program.Features())
	explanations := initializeGritVariableExplanations(program.Variables())
	collectGritExpressionExplanations(program.Root(), &output.Interpretations, explanations)
	output.Variables = gritExplainVariables(program.Variables(), explanations)
	return output
}

func gritCompileDiagnostic(err error) gritExplainDiagnostic {
	diagnostic := gritExplainDiagnostic{Code: "COMPILE_FAILED", Class: "compile", Severity: "error", Message: err.Error()}
	var compileError *gritql.CompileError
	if errors.As(err, &compileError) {
		diagnostic.Code, diagnostic.Class, diagnostic.Message = compileError.Code, compileError.Class, compileError.Message
		if compileError.Range != nil {
			diagnostic.Range = gritRangeString(*compileError.Range)
		}
	}
	return diagnostic
}

func initializeGritVariableExplanations(variables []gritql.Variable) map[string]*gritVariableExplanation {
	result := make(map[string]*gritVariableExplanation, len(variables))
	for _, variable := range variables {
		result[variable.Name] = &gritVariableExplanation{occurrences: map[string]bool{}, bindingKinds: map[string]bool{}, roles: map[string]bool{}}
	}
	return result
}

func collectGritExpressionExplanations(expression gritql.Expression, interpretations *[]gritExplainInterpretation, variables map[string]*gritVariableExplanation) {
	kind := expression.Kind().String()
	for _, reference := range expression.Variables() {
		if !reference.Anonymous {
			addGritVariableOccurrence(variables, reference.Name, reference.Range)
			bindingKind := ""
			if expression.Kind() == gritql.KindAs {
				bindingKind = "node"
			}
			addGritVariableRole(variables, reference.Name, "expression:"+kind, bindingKind)
		}
	}
	for _, template := range expression.Templates() {
		interpretation := explainGritTemplate(kind, template, variables)
		*interpretations = append(*interpretations, interpretation)
	}
	for _, child := range expression.Children() {
		collectGritExpressionExplanations(child, interpretations, variables)
	}
	for _, constraint := range expression.Constraints() {
		lhs := constraint.LHS()
		if !lhs.Anonymous {
			addGritVariableOccurrence(variables, lhs.Name, lhs.Range)
			addGritVariableRole(variables, lhs.Name, "constraint:left", "")
		}
		collectGritExpressionExplanations(constraint.RHS(), interpretations, variables)
	}
}

func explainGritTemplate(expressionKind string, template gritql.Template, variables map[string]*gritVariableExplanation) gritExplainInterpretation {
	context := template.Context().String()
	interpretation := gritExplainInterpretation{ExpressionKind: expressionKind, Context: context, Roles: []string{}}
	if slot := template.RootSlot(); slot.Valid() {
		reference := slot.Variable()
		interpretation.RootSlot = "$" + gritVariableDisplayName(reference.Name)
		role := "wrapper:" + context + ":root"
		interpretation.Roles = append(interpretation.Roles, role)
		addGritVariableRole(variables, reference.Name, role, gritSlotBindingKind(slot.Cardinality()))
	}
	if root := template.Root(); root.Valid() {
		interpretation.RootKind = root.Kind()
		collectGritTemplateNode(root, context, variables, &interpretation.Roles)
	}
	interpretation.Roles = sortedUniqueStrings(interpretation.Roles)
	return interpretation
}

func collectGritTemplateNode(node gritql.TemplateNode, context string, variables map[string]*gritVariableExplanation, roles *[]string) {
	for _, child := range node.Children() {
		if child.IsSlot() {
			slot := child.Slot()
			reference := slot.Variable()
			field := child.Field()
			if field == "" {
				field = "unfielded"
			}
			role := "wrapper:" + context + ":field:" + field
			*roles = append(*roles, role)
			addGritVariableRole(variables, reference.Name, role, gritSlotBindingKind(slot.Cardinality()))
		} else if nested := child.Node(); nested.Valid() {
			collectGritTemplateNode(nested, context, variables, roles)
		}
	}
}

func addGritVariableRole(variables map[string]*gritVariableExplanation, name, role, bindingKind string) {
	explanation := variables[name]
	if explanation == nil {
		return
	}
	explanation.roles[role] = true
	if bindingKind != "" {
		explanation.bindingKinds[bindingKind] = true
	}
}

func addGritVariableOccurrence(variables map[string]*gritVariableExplanation, name string, sourceRange gritql.Range) {
	if explanation := variables[name]; explanation != nil {
		explanation.occurrences[gritRangeString(sourceRange)] = true
	}
}

func gritSlotBindingKind(cardinality gritql.SlotCardinality) string {
	if cardinality == gritql.SlotMany {
		return "list"
	}
	return "node"
}

func gritExplainVariables(variables []gritql.Variable, explanations map[string]*gritVariableExplanation) []gritExplainVariable {
	result := make([]gritExplainVariable, 0, len(variables))
	for _, variable := range variables {
		explanation := explanations[variable.Name]
		result = append(result, gritExplainVariable{ID: uint32(variable.ID), Name: gritVariableDisplayName(variable.Name), FirstRange: gritRangeString(variable.Range), Occurrences: len(explanation.occurrences), BindingKinds: sortedMapKeys(explanation.bindingKinds), Roles: sortedMapKeys(explanation.roles)})
	}
	return result
}

func gritVariableDisplayName(name string) string {
	return strings.TrimPrefix(name, "$")
}

func gritProgramFeatures(features gritql.FeatureSet) []string {
	known := []struct {
		flag gritql.FeatureSet
		name string
	}{{gritql.FeatureSnippet, "snippet"}, {gritql.FeatureRegex, "regex"}, {gritql.FeatureAnd, "and"}, {gritql.FeatureOr, "or"}, {gritql.FeatureNot, "not"}, {gritql.FeatureMaybe, "maybe"}, {gritql.FeatureContains, "contains"}, {gritql.FeatureWithin, "within"}, {gritql.FeatureWhere, "where"}, {gritql.FeatureVariables, "variables"}, {gritql.FeatureAs, "as"}}
	result := []string{}
	for _, feature := range known {
		if features.Has(feature.flag) {
			result = append(result, feature.name)
		}
	}
	return result
}

func sortedMapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedUniqueStrings(values []string) []string {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return sortedMapKeys(set)
}

func gritRangeString(value gritql.Range) string {
	return fmt.Sprintf("%d:%d-%d:%d", value.Start.Line, value.Start.Column, value.End.Line, value.End.Column)
}

func renderGritExplain(output gritExplainOutput, maxOutputBytes int, dependencies Dependencies) error {
	writer := stdoutWriter(dependencies)
	if maxOutputBytes > 0 {
		writer = newBoundedOutputWriter(outputDestination(dependencies), maxOutputBytes)
	}
	if !output.OK {
		for _, diagnostic := range output.Diagnostics {
			if err := writer.writeString(fmt.Sprintf("%s [%s] %s %s\n", diagnostic.Code, diagnostic.Class, diagnostic.Range, diagnostic.Message)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writer.writeString(fmt.Sprintf("grit explain %s language=%s compatibility=%s grammar-abi=%d grammar=%s\n", output.Schema, output.Language, output.Compatibility, output.GrammarABI, output.Grammar)); err != nil {
		return err
	}
	if err := writer.writeString("features: " + strings.Join(output.Features, ",") + "\n"); err != nil {
		return err
	}
	for _, interpretation := range output.Interpretations {
		if err := writer.writeString(fmt.Sprintf("wrapper %s context=%s root=%s slot=%s roles=%s\n", interpretation.ExpressionKind, interpretation.Context, interpretation.RootKind, interpretation.RootSlot, strings.Join(interpretation.Roles, ","))); err != nil {
			return err
		}
	}
	for _, variable := range output.Variables {
		if err := writer.writeString(fmt.Sprintf("variable $%s id=%d occurrences=%d bindings=%s first=%s roles=%s\n", variable.Name, variable.ID, variable.Occurrences, strings.Join(variable.BindingKinds, ","), variable.FirstRange, strings.Join(variable.Roles, ","))); err != nil {
			return err
		}
	}
	return nil
}
