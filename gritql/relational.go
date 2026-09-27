package gritql

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// RelationKey projects one named structural capture. DescendantKind optionally
// selects its first descendant of that grammar kind (including the root).
// Keys compare normalized structure, not whitespace or textual coincidence.
type RelationKey struct {
	Binding        string
	DescendantKind string
}

// RelationSpec joins two GritQL result sets across complete source files.
// Scope is "directory" or "repository". A partition program can further
// restrict matches to one source-declared namespace (for example Go packages).
// Mode "unmatched_left" reports left findings without a matching right finding,
// including in the same file. The default mode reports pairs in different files.
// UniqueLeft suppresses ambiguous declaration matches in the default mode.
type RelationSpec struct {
	Left, Right, Partition *Program
	LeftKey, RightKey      RelationKey
	PartitionKey           RelationKey
	Scope, Mode            string
	LeftInclude            []string // optional left-side output scope; right still scans every eligible source
	UniqueLeft             bool
}

// RelationHit contains the reported finding on Right for pairs, or on Left
// for unmatched_left. The opposite side is absent in unmatched_left results.
type RelationHit struct {
	Left, Right  Finding
	Key, KeyText string
}

type relationFact struct {
	finding   Finding
	key       string
	keyText   string
	partition string
	group     string
}

// Programs lists the structural collectors to evaluate on each file. When
// both sides use the same program it appears once; ordering is stable.
func (spec RelationSpec) Programs() []ProgramScan {
	programs := []ProgramScan{{Program: spec.Left, PatternID: "relation-left"}}
	if spec.Right != spec.Left {
		programs = append(programs, ProgramScan{Program: spec.Right, PatternID: "relation-right"})
	}
	if spec.Partition != nil {
		programs = append(programs, ProgramScan{Program: spec.Partition, PatternID: "relation-partition"})
	}
	return programs
}

// ScanFilesRelation performs the shared serial batch scan and joins its
// complete result. Callers using bounded file-level concurrency can instead
// pass ordered per-file Programs results to JoinRows.
func ScanFilesRelation(ctx context.Context, filesystem fs.FS, candidates []ScanCandidate, spec RelationSpec, options ScanOptions) ([]RelationHit, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("incomplete relational scan: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	results := ScanFilesPrograms(ctx, filesystem, spec.Programs(), candidates, options).Programs()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("incomplete relational scan: %w", err)
	}
	return spec.JoinRows([][]ProgramScanResult{results})
}

// JoinRows joins per-file or batch structural results. It rejects diagnosed,
// truncated, or incomplete rows rather than producing false clean relations.
func (spec RelationSpec) JoinRows(rows [][]ProgramScanResult) ([]RelationHit, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	programs := spec.Programs()
	findings := make([][]Finding, len(programs))
	for _, row := range rows {
		if len(row) != len(programs) {
			return nil, fmt.Errorf("incomplete relational scan: missing program results")
		}
		for index, program := range row {
			if truncations := program.Result.Truncations(); len(truncations) != 0 {
				return nil, fmt.Errorf("%s: incomplete relational scan: %v", program.PatternID, truncations)
			}
			for _, diagnostic := range program.Result.Diagnostics() {
				return nil, fmt.Errorf("%s: incomplete relational scan: %s: %s", program.PatternID, diagnostic.Code(), diagnostic.Message())
			}
			findings[index] = append(findings[index], program.Result.Findings()...)
		}
	}
	partitions := make(map[string]string)
	if spec.Partition != nil {
		for _, finding := range findings[len(findings)-1] {
			value, _, err := relationKey(finding, spec.PartitionKey)
			if err != nil {
				return nil, err
			}
			if prior, exists := partitions[finding.Path()]; exists && prior != value {
				return nil, fmt.Errorf("ambiguous partition in %s", finding.Path())
			}
			partitions[finding.Path()] = value
		}
	}
	leftFindings := findings[0]
	if len(spec.LeftInclude) != 0 {
		leftFindings = make([]Finding, 0, len(findings[0]))
		for _, finding := range findings[0] {
			if MatchesGlobs(finding.Path(), spec.LeftInclude, nil) {
				leftFindings = append(leftFindings, finding)
			}
		}
	}
	left, err := relationFacts(leftFindings, spec.LeftKey, spec.Scope, partitions, spec.Partition != nil)
	if err != nil {
		return nil, err
	}
	rightIndex := 0
	if spec.Right != spec.Left {
		rightIndex = 1
	}
	right, err := relationFacts(findings[rightIndex], spec.RightKey, spec.Scope, partitions, spec.Partition != nil)
	if err != nil {
		return nil, err
	}
	if spec.Mode == "unmatched_left" {
		return unmatchedRelationFacts(left, right), nil
	}
	return joinRelationFacts(left, right, spec.UniqueLeft), nil
}

// Validate checks that relation keys are bound by their source queries.
func (spec RelationSpec) Validate() error { return validateRelationSpec(spec) }

func validateRelationSpec(spec RelationSpec) error {
	if spec.Left == nil || spec.Right == nil || spec.Scope != "directory" && spec.Scope != "repository" {
		return fmt.Errorf("relation requires two programs and scope directory or repository")
	}
	if spec.Mode != "" && spec.Mode != "unmatched_left" {
		return fmt.Errorf("unsupported relation mode %q", spec.Mode)
	}
	if spec.Mode == "unmatched_left" && spec.UniqueLeft {
		return fmt.Errorf("unique_left is not supported in unmatched_left mode")
	}
	if len(spec.LeftInclude) > 32 || ValidateGlobs(spec.LeftInclude, nil) != nil {
		return fmt.Errorf("relation left_include requires at most 32 valid globs")
	}
	for _, glob := range spec.LeftInclude {
		if glob == "" || path.IsAbs(glob) || strings.ContainsRune(glob, 0) {
			return fmt.Errorf("relation left_include must be repository-relative")
		}
		for _, segment := range strings.Split(glob, "/") {
			if segment == ".." {
				return fmt.Errorf("relation left_include cannot escape the repository")
			}
		}
	}
	if spec.Partition == nil && (spec.PartitionKey.Binding != "" || spec.PartitionKey.DescendantKind != "") {
		return fmt.Errorf("relation partition key requires a partition query")
	}
	if spec.Left.Language() != spec.Right.Language() || spec.Partition != nil && spec.Partition.Language() != spec.Left.Language() {
		return fmt.Errorf("relation queries must use the same language")
	}
	for _, item := range []struct {
		program *Program
		key     RelationKey
	}{{spec.Left, spec.LeftKey}, {spec.Right, spec.RightKey}, {spec.Partition, spec.PartitionKey}} {
		if item.program == nil {
			continue
		}
		if item.key.DescendantKind != "" && !parser.GrammarNodeKind(item.program.Language(), item.key.DescendantKind) {
			return fmt.Errorf("relation descendant kind %q is not in the %s grammar", item.key.DescendantKind, item.program.Language())
		}
		valid := false
		for _, variable := range item.program.Variables() {
			if variable.Name == "$"+item.key.Binding {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("relation key %q is not captured by its query", item.key.Binding)
		}
	}
	return nil
}

func relationFacts(findings []Finding, key RelationKey, scope string, partitions map[string]string, partitioned bool) ([]relationFact, error) {
	facts := make([]relationFact, 0, len(findings))
	for _, finding := range findings {
		value, text, err := relationKey(finding, key)
		if err != nil {
			return nil, err
		}
		group := ""
		if scope == "directory" {
			group = path.Dir(finding.Path())
		}
		partition := ""
		if partitioned {
			var ok bool
			partition, ok = partitions[finding.Path()]
			if !ok {
				return nil, fmt.Errorf("missing source partition for %s", finding.Path())
			}
		}
		facts = append(facts, relationFact{finding: finding, key: value, keyText: text, group: group, partition: partition})
	}
	return facts, nil
}

func relationKey(finding Finding, key RelationKey) (string, string, error) {
	for _, binding := range finding.Bindings() {
		if binding.Name() != key.Binding {
			continue
		}
		node, ok := binding.Node()
		if !ok {
			return "", "", fmt.Errorf("%s: relation key %q must bind one node", finding.Path(), key.Binding)
		}
		if key.DescendantKind != "" {
			node, ok = firstDescendantOfKind(node, key.DescendantKind)
			if !ok {
				return "", "", fmt.Errorf("%s: relation key %q has no %s descendant", finding.Path(), key.Binding, key.DescendantKind)
			}
		}
		encoded, err := node.MarshalJSON()
		if err != nil {
			return "", "", err
		}
		return string(encoded), node.Lexeme(), nil
	}
	return "", "", fmt.Errorf("%s: relation key %q has no binding", finding.Path(), key.Binding)
}

func firstDescendantOfKind(root StructuralNode, kind string) (StructuralNode, bool) {
	stack := []StructuralNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node.Kind() == kind {
			return node, true
		}
		children := node.Children()
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return StructuralNode{}, false
}

func joinRelationFacts(left, right []relationFact, uniqueLeft bool) []RelationHit {
	index := make(map[string][]relationFact)
	for _, fact := range left {
		index[fact.group+"\x00"+fact.partition+"\x00"+fact.key] = append(index[fact.group+"\x00"+fact.partition+"\x00"+fact.key], fact)
	}
	var hits []RelationHit
	for _, fact := range right {
		matches := index[fact.group+"\x00"+fact.partition+"\x00"+fact.key]
		if uniqueLeft && len(matches) != 1 {
			continue
		}
		for _, match := range matches {
			if match.finding.Path() != fact.finding.Path() {
				hits = append(hits, RelationHit{Left: match.finding, Right: fact.finding, Key: fact.key, KeyText: fact.keyText})
				break
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i].Right, hits[j].Right
		if a.Path() != b.Path() {
			return a.Path() < b.Path()
		}
		if a.StartByte() != b.StartByte() {
			return a.StartByte() < b.StartByte()
		}
		return hits[i].Left.Path() < hits[j].Left.Path()
	})
	return hits
}

// unmatchedRelationFacts reports absence against the entire scanned source set,
// not just other files. Every right-side occurrence suppresses its key.
func unmatchedRelationFacts(left, right []relationFact) []RelationHit {
	seen := make(map[string]bool, len(right))
	for _, fact := range right {
		seen[fact.group+"\x00"+fact.partition+"\x00"+fact.key] = true
	}
	var hits []RelationHit
	for _, fact := range left {
		if !seen[fact.group+"\x00"+fact.partition+"\x00"+fact.key] {
			hits = append(hits, RelationHit{Left: fact.finding, Key: fact.key, KeyText: fact.keyText})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i].Left, hits[j].Left
		if a.Path() != b.Path() {
			return a.Path() < b.Path()
		}
		if a.StartByte() != b.StartByte() {
			return a.StartByte() < b.StartByte()
		}
		return hits[i].Key < hits[j].Key
	})
	return hits
}
