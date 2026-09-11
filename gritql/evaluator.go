package gritql

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/greppleai/grepple/parser"
)

const (
	defaultMaxCandidates = 250_000
	hardMaxCandidates    = 1_000_000
	defaultMaxSteps      = 10_000_000
	hardMaxSteps         = 100_000_000
	defaultMaxFindings   = 10_000
	hardMaxFindings      = 100_000
	defaultFileTime      = 2 * time.Second
	hardFileTime         = 10 * time.Second
)

// EvaluateOptions bounds evaluation. Zero values select the native compatibility
// defaults. MaxFindings and MaxBatchElapsed are batch normalization limits;
// they are deliberately not applied by evaluation of one file. MaxSteps accounts
// syntax visits, query operations, constraints, and matcher/backtracking work.
type EvaluateOptions struct {
	MaxDepth            int
	MaxCandidates       int
	MaxSteps            int
	MaxFindings         int
	MaxSourceBytes      int
	MaxMemoryBytes      int
	MaxElapsed          time.Duration
	MaxBatchElapsed     time.Duration
	DisableFileTimeout  bool
	DisableBatchTimeout bool
	Deadline            time.Time
}

// EvaluationOptions is an alias retained for callers that prefer the noun form.
type EvaluationOptions = EvaluateOptions

// EvaluationError is a stable, typed failure for one document evaluation.
// Evaluation is transactional: whenever it is returned, no matches are returned.
type EvaluationError struct {
	Code    string
	Class   string
	Message string
	cause   error
}

func (e *EvaluationError) Error() string { return e.Message }
func (e *EvaluationError) Unwrap() error { return e.cause }

// EvaluationMatch is one query record emitted by a top-level candidate.
type EvaluationMatch struct {
	rng      parser.Range
	bindings BindingSet
}

// Range returns the exact significant source range emitted for the match.
func (m EvaluationMatch) Range() parser.Range { return m.rng }

// Bindings returns the immutable binding set committed by the evaluation.
func (m EvaluationMatch) Bindings() BindingSet { return m.bindings }

type evaluationBudget struct {
	ctx      context.Context
	deadline time.Time
	steps    int
	maxSteps int
	err      *EvaluationError
}

func (b *evaluationBudget) take() bool {
	if b == nil {
		return true
	}
	if b.err != nil {
		return false
	}
	b.steps++
	if b.steps > b.maxSteps {
		b.err = evaluationFailure("LIMIT_AST_STEPS", "resource", "AST/match step budget exceeded", nil)
		return false
	}
	if err := b.ctx.Err(); err != nil {
		b.err = evaluationFailure("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", err)
		return false
	}
	if !b.deadline.IsZero() && !time.Now().Before(b.deadline) {
		b.err = evaluationFailure("LIMIT_TIME_FILE", "resource", "per-file evaluation deadline exceeded", nil)
		return false
	}
	return true
}

type evalCandidate struct {
	target    MatchTarget
	rng       parser.Range
	node      parser.ViewNode
	ancestors []parser.ViewNode
	sequence  bool
	parent    parser.ViewNode
	elements  []parser.ViewNode
}

type queryRecord struct {
	rng      parser.Range
	bindings BindingSet
}

// Evaluate evaluates a compiled Program against one already-parsed target document.
// Candidates and records are deterministic and no path or Finding model is used.
func Evaluate(ctx context.Context, program *Program, document *parser.Document, options EvaluateOptions) ([]EvaluationMatch, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateEvaluationInputs(program, document); err != nil {
		return nil, err
	}
	options = normalizeEvaluateOptions(options)
	budget := newEvaluationBudget(ctx, options)
	var matches []EvaluationMatch
	err := document.Read(func(view parser.DocumentView) error {
		root := view.Root()
		if err := inspectEvaluationSource(root, options.MaxDepth, budget); err != nil {
			return err
		}
		candidates, nodesByRange, err := collectEvaluationCandidates(root, options, budget)
		if err != nil {
			return err
		}
		matches, err = evaluateCandidates(program.root, candidates, nodesByRange, budget)
		return err
	})
	if err != nil {
		if errors.Is(err, parser.ErrDocumentClosed) {
			return nil, evaluationFailure("INTERNAL_ERROR", "internal", "source document closed during evaluation", err)
		}
		return nil, err
	}
	return matches, nil
}

func validateEvaluationInputs(program *Program, document *parser.Document) error {
	if program == nil || program.root == nil {
		return evaluationFailure("INTERNAL_ERROR", "internal", "invalid compiled program", nil)
	}
	if document == nil || document.Language() == "" || !document.Root().Valid() {
		return evaluationFailure("INTERNAL_ERROR", "internal", "invalid or closed source document", nil)
	}
	if program.Language() != document.Language() {
		return evaluationFailure("INTERNAL_ERROR", "internal", "program and document languages differ", nil)
	}
	return nil
}

func newEvaluationBudget(ctx context.Context, options EvaluateOptions) *evaluationBudget {
	deadline := evaluationDeadline(time.Now(), options.MaxElapsed, options.Deadline)
	return &evaluationBudget{ctx: ctx, deadline: deadline, maxSteps: options.MaxSteps}
}

func evaluationDeadline(started time.Time, maximum time.Duration, explicit time.Time) time.Time {
	deadline := explicit
	if maximum > 0 {
		candidate := started.Add(maximum)
		if deadline.IsZero() || candidate.Before(deadline) {
			deadline = candidate
		}
	}
	return deadline
}

func deadlineExceeded(deadline time.Time) bool {
	return !deadline.IsZero() && !time.Now().Before(deadline)
}

func inspectEvaluationSource(root parser.ViewNode, maxDepth int, budget *evaluationBudget) error {
	message, found, err := inspectSource(root, maxDepth, budget)
	if err != nil {
		return err
	}
	if found {
		return evaluationFailure("SOURCE_PARSE", "source", "source parse failure: "+message, nil)
	}
	return nil
}

func evaluateCandidates(root *expression, candidates []evalCandidate, nodes map[string]evalCandidate, budget *evaluationBudget) ([]EvaluationMatch, error) {
	var out []EvaluationMatch
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if !budget.take() {
			return nil, budget.err
		}
		records := evaluateExpression(root, candidate, BindingSet{}, nodes, budget)
		if budget.err != nil {
			return nil, budget.err
		}
		var err error
		out, err = appendEvaluationRecords(out, records, seen, budget)
		if err != nil {
			return nil, err
		}
	}
	if !budget.take() {
		return nil, budget.err
	}
	return out, nil
}

func appendEvaluationRecords(out []EvaluationMatch, records []queryRecord, seen map[string]struct{}, budget *evaluationBudget) ([]EvaluationMatch, error) {
	for _, record := range records {
		if !budget.take() {
			return nil, budget.err
		}
		match := EvaluationMatch{rng: record.rng, bindings: record.bindings}
		key := evaluationMatchKey(match)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, match)
	}
	return out, nil
}

// EvaluateDocument is an explicit-name alias for Evaluate.
func EvaluateDocument(ctx context.Context, program *Program, document *parser.Document, options EvaluateOptions) ([]EvaluationMatch, error) {
	return Evaluate(ctx, program, document, options)
}

func normalizeEvaluateOptions(o EvaluateOptions) EvaluateOptions {
	o.MaxDepth = boundedOption(o.MaxDepth, defaultMaxDepth, hardMaxDepth)
	o.MaxCandidates = boundedOption(o.MaxCandidates, defaultMaxCandidates, hardMaxCandidates)
	o.MaxSteps = boundedOption(o.MaxSteps, defaultMaxSteps, hardMaxSteps)
	o.MaxFindings = boundedOption(o.MaxFindings, defaultMaxFindings, hardMaxFindings)
	o.MaxSourceBytes = boundedOption(o.MaxSourceBytes, defaultMaxSourceBytes, hardMaxSourceBytes)
	o.MaxMemoryBytes = boundedOption(o.MaxMemoryBytes, defaultMaxMemoryBytes, hardMaxMemoryBytes)
	if o.DisableFileTimeout {
		o.MaxElapsed = 0
	} else if o.MaxElapsed <= 0 {
		o.MaxElapsed = defaultFileTime
	} else if o.MaxElapsed > hardFileTime {
		o.MaxElapsed = hardFileTime
	}
	if o.DisableBatchTimeout {
		o.MaxBatchElapsed = 0
	} else if o.MaxBatchElapsed <= 0 {
		o.MaxBatchElapsed = defaultBatchTime
	} else if o.MaxBatchElapsed > hardBatchTime {
		o.MaxBatchElapsed = hardBatchTime
	}
	return o
}

func evaluationFailure(code, class, message string, cause error) *EvaluationError {
	return &EvaluationError{Code: code, Class: class, Message: message, cause: cause}
}

type sourceInspectionItem struct {
	node  parser.ViewNode
	depth int
}

func inspectSource(root parser.ViewNode, maxDepth int, budget *evaluationBudget) (string, bool, error) {
	stack := []sourceInspectionItem{{node: root, depth: 1}}
	var earliest *parser.ParseDiagnostic
	for len(stack) > 0 {
		if !budget.take() {
			return "", false, budget.err
		}
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if err := validateInspectedNode(x, maxDepth); err != nil {
			return "", false, err
		}
		earliest = earlierParseDiagnostic(earliest, x.node)
		stack = appendInspectionChildren(stack, x)
	}
	if earliest != nil {
		return earliest.Message, true, nil
	}
	return "", false, nil
}

func validateInspectedNode(item sourceInspectionItem, maxDepth int) error {
	if item.depth > maxDepth {
		return evaluationFailure("LIMIT_PARSE_DEPTH", "resource", "source tree exceeds effective depth limit", nil)
	}
	if !item.node.Valid() {
		return evaluationFailure("INTERNAL_ERROR", "internal", "source document closed during evaluation", nil)
	}
	return nil
}

func earlierParseDiagnostic(earliest *parser.ParseDiagnostic, node parser.ViewNode) *parser.ParseDiagnostic {
	if !node.IsError() && !node.IsMissing() {
		return earliest
	}
	message := "syntax error"
	if node.IsMissing() {
		message = "missing " + node.Kind()
	}
	diagnostic := parser.ParseDiagnostic{Message: message, Kind: node.Kind(), Range: node.Range()}
	if earliest == nil || diagnostic.Range.StartByte < earliest.Range.StartByte ||
		diagnostic.Range.StartByte == earliest.Range.StartByte && diagnostic.Message < earliest.Message {
		return &diagnostic
	}
	return earliest
}

func appendInspectionChildren(stack []sourceInspectionItem, item sourceInspectionItem) []sourceInspectionItem {
	children := item.node.Children()
	for i := len(children) - 1; i >= 0; i-- {
		stack = append(stack, sourceInspectionItem{node: children[i], depth: item.depth + 1})
	}
	return stack
}

type significantRangeFrame struct {
	node        parser.ViewNode
	children    []parser.ViewNode
	next        int
	first, last parser.Range
	has         bool
}

func collectSignificantRanges(root parser.ViewNode, budget *evaluationBudget) (map[string]parser.Range, error) {
	stack := []significantRangeFrame{{node: root}}
	out := make(map[string]parser.Range)
	for len(stack) > 0 {
		if !budget.take() {
			return nil, budget.err
		}
		if descendSignificantRange(&stack) {
			continue
		}
		finishSignificantRange(&stack, out)
	}
	return out, nil
}

func descendSignificantRange(stack *[]significantRangeFrame) bool {
	current := &(*stack)[len(*stack)-1]
	if current.children == nil {
		current.children = current.node.Children()
	}
	if current.next >= len(current.children) {
		return false
	}
	child := current.children[current.next]
	current.next++
	if child.IsExtra() || child.Kind() == "comment" || !child.IsNamed() && child.Range().StartByte == child.Range().EndByte {
		return true
	}
	*stack = append(*stack, significantRangeFrame{node: child})
	return true
}

func finishSignificantRange(stack *[]significantRangeFrame, out map[string]parser.Range) {
	i := len(*stack) - 1
	current := &(*stack)[i]
	rng := current.node.Range()
	if current.has {
		rng.StartByte, rng.Start = current.first.StartByte, current.first.Start
		rng.EndByte, rng.End = current.last.EndByte, current.last.End
	}
	out[nodeRangeKey(current.node.Kind(), current.node.Range())] = rng
	*stack = (*stack)[:i]
	if len(*stack) == 0 {
		return
	}
	parent := &(*stack)[len(*stack)-1]
	if !parent.has {
		parent.first = rng
	}
	parent.last, parent.has = rng, true
}

type evaluationTraversalItem struct {
	node      parser.ViewNode
	depth     int
	ancestors []parser.ViewNode
}

func collectEvaluationCandidates(root parser.ViewNode, options EvaluateOptions, budget *evaluationBudget) ([]evalCandidate, map[string]evalCandidate, error) {
	significantRanges, err := collectSignificantRanges(root, budget)
	if err != nil {
		return nil, nil, err
	}
	stack := []evaluationTraversalItem{{node: root, depth: 1}}
	var candidates []evalCandidate
	nodesByRange := make(map[string]evalCandidate)
	for len(stack) > 0 {
		if !budget.take() {
			return nil, nil, budget.err
		}
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if err := validateEvaluationTraversalItem(x, options.MaxDepth); err != nil {
			return nil, nil, err
		}
		if evaluationCandidateNode(x.node) {
			candidates, err = appendNodeAndSequenceCandidates(candidates, nodesByRange, x, significantRanges, options.MaxCandidates, budget)
			if err != nil {
				return nil, nil, err
			}
		}
		stack = appendEvaluationChildren(stack, x)
	}
	return candidates, nodesByRange, nil
}

func validateEvaluationTraversalItem(item evaluationTraversalItem, maxDepth int) error {
	if item.depth > maxDepth {
		return evaluationFailure("LIMIT_PARSE_DEPTH", "resource", "source tree exceeds effective depth limit", nil)
	}
	if !item.node.Valid() {
		return evaluationFailure("INTERNAL_ERROR", "internal", "source document closed during evaluation", nil)
	}
	return nil
}

func evaluationCandidateNode(node parser.ViewNode) bool {
	return node.IsNamed() && !node.IsExtra() && node.Kind() != "comment"
}

func appendNodeAndSequenceCandidates(candidates []evalCandidate, nodes map[string]evalCandidate, item evaluationTraversalItem, significantRanges map[string]parser.Range, maxCandidates int, budget *evaluationBudget) ([]evalCandidate, error) {
	key := nodeRangeKey(item.node.Kind(), item.node.Range())
	candidate := evalCandidate{
		target: viewNodeTarget(item.node), rng: significantRanges[key], node: item.node,
		ancestors: append([]parser.ViewNode(nil), item.ancestors...),
	}
	candidates = append(candidates, candidate)
	nodes[key] = candidate
	if len(candidates) > maxCandidates {
		return nil, evaluationFailure("LIMIT_CANDIDATES", "resource", "traversal candidate limit exceeded", nil)
	}
	sequences := sequenceCandidates(item.node, item.ancestors, significantRanges, maxCandidates-len(candidates), budget)
	if budget.err != nil {
		return nil, budget.err
	}
	candidates = append(candidates, sequences...)
	if len(candidates) > maxCandidates {
		return nil, evaluationFailure("LIMIT_CANDIDATES", "resource", "traversal candidate limit exceeded", nil)
	}
	return candidates, nil
}

func appendEvaluationChildren(stack []evaluationTraversalItem, item evaluationTraversalItem) []evaluationTraversalItem {
	children := item.node.Children()
	ancestors := item.ancestors
	if evaluationCandidateNode(item.node) {
		ancestors = append([]parser.ViewNode{item.node}, ancestors...)
	}
	for i := len(children) - 1; i >= 0; i-- {
		stack = append(stack, evaluationTraversalItem{node: children[i], depth: item.depth + 1, ancestors: ancestors})
	}
	return stack
}

type sequencePosition struct {
	field   string
	indexes []int
}

func sequenceCandidates(parent parser.ViewNode, ancestors []parser.ViewNode, significantRanges map[string]parser.Range, remaining int, budget *evaluationBudget) []evalCandidate {
	children := parent.Children()
	positions := repeatedSequencePositions(parent, children, budget)
	if budget.err != nil {
		return nil
	}
	var out []evalCandidate
	for _, position := range positions {
		if !appendPositionSequences(&out, parent, ancestors, children, position, significantRanges, remaining, budget) {
			return nil
		}
	}
	return out
}

func repeatedSequencePositions(parent parser.ViewNode, children []parser.ViewNode, budget *evaluationBudget) []sequencePosition {
	var positions []sequencePosition
	byField := make(map[string]int)
	for i, child := range children {
		if !budget.take() {
			return nil
		}
		if !repeatedTraversalElement(parent, child) {
			continue
		}
		field := child.FieldName()
		group, exists := byField[field]
		if !exists {
			group = len(positions)
			byField[field] = group
			positions = append(positions, sequencePosition{field: field})
		}
		positions[group].indexes = append(positions[group].indexes, i)
	}
	return positions
}

func appendPositionSequences(out *[]evalCandidate, parent parser.ViewNode, ancestors []parser.ViewNode, children []parser.ViewNode, position sequencePosition, significantRanges map[string]parser.Range, remaining int, budget *evaluationBudget) bool {
	// A repeated field can have multiple runs if another named grammar member
	// occurs between values. Each run is an independent consecutive list.
	for runStart := 0; runStart < len(position.indexes); {
		runEnd := sequenceRunEnd(children, position.indexes, runStart)
		if !appendRunSequences(out, parent, ancestors, children, position.field, position.indexes[runStart:runEnd], significantRanges, remaining, budget) {
			return false
		}
		runStart = runEnd
	}
	return true
}

func sequenceRunEnd(children []parser.ViewNode, indexes []int, runStart int) int {
	runEnd := runStart + 1
	for runEnd < len(indexes) && onlyListSeparators(children, indexes[runEnd-1]+1, indexes[runEnd]) {
		runEnd++
	}
	return runEnd
}

func appendRunSequences(out *[]evalCandidate, parent parser.ViewNode, ancestors []parser.ViewNode, children []parser.ViewNode, field string, indexes []int, significantRanges map[string]parser.Range, remaining int, budget *evaluationBudget) bool {
	for length := len(indexes); length >= 1; length-- {
		for first := 0; first+length <= len(indexes); first++ {
			if !budget.take() {
				return false
			}
			*out = append(*out, makeSequenceCandidate(parent, ancestors, children, field, indexes, first, length, significantRanges))
			if len(*out) > remaining {
				budget.err = evaluationFailure("LIMIT_CANDIDATES", "resource", "traversal candidate limit exceeded", nil)
				return false
			}
		}
	}
	return true
}

func makeSequenceCandidate(parent parser.ViewNode, ancestors []parser.ViewNode, children []parser.ViewNode, field string, indexes []int, first, length int, significantRanges map[string]parser.Range) evalCandidate {
	last := first + length - 1
	start := indexes[first]
	end := sequenceTargetEnd(children, indexes[last]+1)
	firstNode, lastNode := children[start], children[indexes[last]]
	rng := significantRanges[nodeRangeKey(firstNode.Kind(), firstNode.Range())]
	lastRange := significantRanges[nodeRangeKey(lastNode.Kind(), lastNode.Range())]
	rng.EndByte, rng.End = lastRange.EndByte, lastRange.End
	elements := make([]parser.ViewNode, length)
	for i := range elements {
		elements[i] = children[indexes[first+i]]
	}
	return evalCandidate{
		target: sequenceMatchTarget(parent, field, start, end), rng: rng, sequence: true,
		parent: parent, elements: elements, ancestors: append([]parser.ViewNode(nil), ancestors...),
	}
}

func sequenceTargetEnd(children []parser.ViewNode, end int) int {
	// Authored separators following the final element remain matchable, but do
	// not enlarge the candidate range or equality value.
	for end < len(children) && !children[end].IsNamed() && !children[end].IsExtra() && listSeparator(children[end].Kind()) {
		end++
	}
	return end
}

func sequenceMatchTarget(parent parser.ViewNode, field string, start, end int) MatchTarget {
	switch parent.Kind() {
	case "statement_list", "statement_block":
		return viewSequenceTarget("statement_sequence", parent, start, end)
	case "source_file", "program":
		return viewSequenceTarget("declaration_sequence", parent, start, end)
	default:
		return viewRepeatedSequenceTarget(parent, field, start, end)
	}
}

func repeatedTraversalElement(parent, child parser.ViewNode) bool {
	if !child.IsNamed() || child.IsExtra() || child.Kind() == "comment" || !repeatedViewGrammarPosition(parent.Language(), parent, child) {
		return false
	}
	switch parent.Kind() {
	case "source_file", "program":
		return rootCategoryAccepts(parent.Language(), SnippetContextDeclarationList, child.Kind())
	case "statement_list", "statement_block":
		return rootCategoryAccepts(parent.Language(), SnippetContextStatementList, child.Kind())
	default:
		return true
	}
}

func onlyListSeparators(children []parser.ViewNode, start, end int) bool {
	for i := start; i < end; i++ {
		child := children[i]
		if child.IsExtra() || child.Kind() == "comment" {
			continue
		}
		if child.IsNamed() || !listSeparator(child.Kind()) {
			return false
		}
	}
	return true
}

func evaluateExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	if expr == nil || !budget.take() {
		return nil
	}
	switch expr.kind {
	case KindSnippet:
		return evaluateSnippetExpression(expr, candidate, incoming, budget)
	case KindRegex:
		return nil
	case KindAnd:
		return evaluateAndExpression(expr, candidate, incoming, nodes, budget)
	case KindOr:
		return evaluateOrExpression(expr, candidate, incoming, nodes, budget)
	case KindNot:
		return evaluateNotExpression(expr, candidate, incoming, nodes, budget)
	case KindMaybe:
		return evaluateMaybeExpression(expr, candidate, incoming, nodes, budget)
	case KindContains:
		return evaluateContainsExpression(expr, candidate, incoming, nodes, budget)
	case KindWithin:
		return evaluateWithinExpression(expr, candidate, incoming, nodes, budget)
	case KindWhere:
		return evaluateWhereExpression(expr, candidate, incoming, nodes, budget)
	default:
		budget.err = evaluationFailure("INTERNAL_ERROR", "internal", "unknown compiled expression", nil)
		return nil
	}
}

func evaluateSnippetExpression(expr *expression, candidate evalCandidate, incoming BindingSet, budget *evaluationBudget) []queryRecord {
	templates := expr.templates
	if len(templates) == 0 {
		templates = []Template{expr.template}
	}
	records := make([]queryRecord, 0, len(templates))
	for _, template := range templates {
		matched, ok := matchTemplateWithBudget(template, candidate.target, incoming, budget)
		if ok {
			records = append(records, queryRecord{rng: matched.Range(), bindings: matched.Bindings()})
		}
	}
	return records
}

func evaluateAndExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var records []queryRecord
	for childIndex, child := range expr.children {
		if childIndex == 0 {
			records = evaluateExpression(child, candidate, incoming, nodes, budget)
		} else {
			records = joinExpressionRecords(records, child, candidate, nodes, budget)
		}
		if len(records) == 0 || budget.err != nil {
			return nil
		}
	}
	return records
}

func joinExpressionRecords(leftRecords []queryRecord, child *expression, candidate evalCandidate, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var joined []queryRecord
	for _, left := range leftRecords {
		for _, right := range evaluateExpression(child, candidate, left.bindings, nodes, budget) {
			joined = append(joined, queryRecord{rng: coveringRange(left.rng, right.rng), bindings: right.bindings})
		}
	}
	return joined
}

func evaluateOrExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var records []queryRecord
	for _, child := range expr.children {
		records = append(records, evaluateExpression(child, candidate, incoming, nodes, budget)...)
		if budget.err != nil {
			return nil
		}
	}
	return records
}

func evaluateNotExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	if len(evaluateExpression(expr.children[0], candidate, incoming, nodes, budget)) == 0 && budget.err == nil {
		return []queryRecord{{rng: candidate.rng, bindings: incoming}}
	}
	return nil
}

func evaluateMaybeExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	records := evaluateExpression(expr.children[0], candidate, incoming, nodes, budget)
	if len(records) != 0 || budget.err != nil {
		return records
	}
	return []queryRecord{{rng: candidate.rng, bindings: incoming}}
}

func evaluateContainsExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var records []queryRecord
	for _, nested := range containmentCandidates(candidate, nodes, budget) {
		records = append(records, evaluateExpression(expr.children[0], nested, incoming, nodes, budget)...)
		if budget.err != nil {
			return nil
		}
	}
	return records
}

func evaluateWithinExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var records []queryRecord
	for _, containing := range withinCandidates(candidate, nodes, budget) {
		records = append(records, evaluateWithinCandidate(expr.children[0], candidate.rng, containing, incoming, nodes, budget)...)
		if budget.err != nil {
			return nil
		}
	}
	return records
}

func evaluateWithinCandidate(expr *expression, resultRange parser.Range, containing evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	records := evaluateExpression(expr, containing, incoming, nodes, budget)
	for i := range records {
		records[i].rng = resultRange
	}
	return records
}

func evaluateWhereExpression(expr *expression, candidate evalCandidate, incoming BindingSet, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	prefix := evaluateExpression(expr.children[0], candidate, incoming, nodes, budget)
	for _, item := range expr.constraints {
		prefix = filterConstraintRecords(item, prefix, nodes, budget)
		if len(prefix) == 0 || budget.err != nil {
			return nil
		}
	}
	return prefix
}

func filterConstraintRecords(item constraint, prefix []queryRecord, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var filtered []queryRecord
	for _, record := range prefix {
		filtered = append(filtered, evaluateConstraint(item, record, nodes, budget)...)
	}
	return filtered
}

func evaluateConstraint(item constraint, record queryRecord, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	if !budget.take() {
		return nil
	}
	bound, ok := record.bindings.Lookup(item.lhs.ID)
	if !ok || !bound.Valid() {
		return nil
	}
	if item.rhs.kind == KindRegex {
		return evaluateRegexConstraint(item, record, bound)
	}
	return evaluateStructuralConstraint(item, record, bound, nodes, budget)
}

func evaluateRegexConstraint(item constraint, record queryRecord, bound BoundValue) []queryRecord {
	if item.rhs.re.MatchString(bound.Text()) {
		return []queryRecord{record}
	}
	return nil
}

func boundSourceRanges(bound BoundValue) []parser.Range {
	if bound.value != nil && len(bound.value.sourceRanges) != 0 {
		return bound.value.sourceRanges
	}
	if bound.Kind() == BindingNode {
		return []parser.Range{bound.Range()}
	}
	return nil
}

func boundSourceKind(bound BoundValue, index int) string {
	if bound.value != nil && index < len(bound.value.sourceKinds) {
		return bound.value.sourceKinds[index]
	}
	return ""
}

func evaluateStructuralConstraint(item constraint, record queryRecord, bound BoundValue, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	var out []queryRecord
	for index, rng := range boundSourceRanges(bound) {
		candidate, exists := nodes[nodeRangeKey(boundSourceKind(bound, index), rng)]
		if !exists {
			budget.err = evaluationFailure("INTERNAL_ERROR", "internal", "bound source node is unavailable", nil)
			return nil
		}
		candidate.rng = bound.Range()
		out = appendConstraintMatches(out, item.rhs, candidate, record, nodes, budget)
	}
	return out
}

func appendConstraintMatches(out []queryRecord, rhs *expression, candidate evalCandidate, record queryRecord, nodes map[string]evalCandidate, budget *evaluationBudget) []queryRecord {
	for _, nested := range evaluateExpression(rhs, candidate, record.bindings, nodes, budget) {
		out = append(out, queryRecord{rng: record.rng, bindings: nested.bindings})
	}
	return out
}

func containmentCandidates(candidate evalCandidate, nodes map[string]evalCandidate, budget *evaluationBudget) []evalCandidate {
	out := []evalCandidate{candidate}
	seen, stack := initializeContainmentTraversal(candidate)
	for len(stack) > 0 {
		if !budget.take() {
			return nil
		}
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		var ok bool
		out, ok = appendContainedCandidate(out, node, seen, nodes, budget)
		if !ok {
			return nil
		}
		stack = appendReverseChildren(stack, node.Children())
	}
	return out
}

func initializeContainmentTraversal(candidate evalCandidate) (map[string]struct{}, []parser.ViewNode) {
	seen := make(map[string]struct{})
	if candidate.sequence {
		// A synthetic list's elements are its direct named children.
		return seen, appendReverseChildren(nil, candidate.elements)
	}
	if !candidate.node.Valid() {
		return seen, nil
	}
	seen[nodeRangeKey(candidate.node.Kind(), candidate.node.Range())] = struct{}{}
	return seen, appendReverseChildren(nil, candidate.node.Children())
}

func appendContainedCandidate(out []evalCandidate, node parser.ViewNode, seen map[string]struct{}, nodes map[string]evalCandidate, budget *evaluationBudget) ([]evalCandidate, bool) {
	if !evaluationCandidateNode(node) {
		return out, true
	}
	key := nodeRangeKey(node.Kind(), node.Range())
	if _, duplicate := seen[key]; duplicate {
		return out, true
	}
	nested, exists := nodes[key]
	if !exists {
		budget.err = evaluationFailure("INTERNAL_ERROR", "internal", "source candidate is unavailable", nil)
		return nil, false
	}
	seen[key] = struct{}{}
	return append(out, nested), true
}

func appendReverseChildren(stack, children []parser.ViewNode) []parser.ViewNode {
	for i := len(children) - 1; i >= 0; i-- {
		stack = append(stack, children[i])
	}
	return stack
}

func withinCandidates(candidate evalCandidate, nodes map[string]evalCandidate, budget *evaluationBudget) []evalCandidate {
	out := []evalCandidate{candidate}
	if candidate.sequence {
		parent, exists := nodes[nodeRangeKey(candidate.parent.Kind(), candidate.parent.Range())]
		if !exists {
			budget.err = evaluationFailure("INTERNAL_ERROR", "internal", "sequence parent candidate is unavailable", nil)
			return nil
		}
		out = append(out, parent)
	}
	for _, ancestor := range candidate.ancestors {
		if !budget.take() {
			return nil
		}
		if candidate.sequence && ancestor.Range() == candidate.parent.Range() {
			continue
		}
		containing, exists := nodes[nodeRangeKey(ancestor.Kind(), ancestor.Range())]
		if !exists {
			budget.err = evaluationFailure("INTERNAL_ERROR", "internal", "ancestor candidate is unavailable", nil)
			return nil
		}
		out = append(out, containing)
	}
	return out
}

func coveringRange(a, b parser.Range) parser.Range {
	if a.StartByte > b.StartByte {
		a.StartByte, a.Start = b.StartByte, b.Start
	}
	if a.EndByte < b.EndByte {
		a.EndByte, a.End = b.EndByte, b.End
	}
	return a
}

func nodeRangeKey(kind string, r parser.Range) string {
	return fmt.Sprintf("%s:%d:%d", kind, r.StartByte, r.EndByte)
}

func evaluationMatchKey(match EvaluationMatch) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%#v|", match.rng)
	for _, bound := range match.bindings.All() {
		value := bound.value
		fmt.Fprintf(&builder, "%d:%d:%#v:%#v:%q:%#v|", value.variable.ID, value.kind, value.rng, value.ranges, value.text, value.equality)
	}
	return builder.String()
}

// String gives useful deterministic formatting without defining task9's finding model.
func (m EvaluationMatch) String() string {
	var names []string
	for _, value := range m.bindings.All() {
		names = append(names, value.Variable().Name+"="+value.Text())
	}
	return fmt.Sprintf("[%d,%d) {%s}", m.rng.StartByte, m.rng.EndByte, strings.Join(names, ","))
}
