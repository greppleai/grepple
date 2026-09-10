package extract

import (
	"go/ast"
	goparser "go/parser"
	"strings"
)

type goTypeOccurrences struct {
	one  bool
	many bool
}

// packageGoTypeCardinality derives cardinality for the package-bundle IR from
// occurrences in a normalized Go type expression. An occurrence is "many" if
// an array, slice, map, or channel encloses it. If the same exact type evidence
// contains both one and many occurrences, many wins as the deterministic,
// conservative representation. Synthetic tuples are analyzed component-wise.
// Invalid expressions contain no evidence; package IR normally only supplies
// normalized expressions produced from valid Go source.
func packageGoTypeCardinality(value, target string) (string, bool) {
	occurrences := goTypeOccurrences{}
	for _, component := range packageGoTypeComponents(value) {
		expression, err := goparser.ParseExpr(component)
		if err != nil {
			continue
		}
		walkGoTypeOccurrences(expression, target, false, &occurrences)
	}
	if occurrences.many {
		return "many", true
	}
	if occurrences.one {
		return "one", true
	}
	return "", false
}

func packageGoTypeComponents(value string) []string {
	if strings.HasPrefix(value, "tuple<") && strings.HasSuffix(value, ">") {
		return splitParameters(value[len("tuple<") : len(value)-1])
	}
	// Variadic parameters are represented as synthetic "...T" schema values,
	// not ParseExpr-compatible expressions. Variadicity is not a repeated type
	// container for relation cardinality; inspect its element type normally.
	if strings.HasPrefix(value, "...") {
		return []string{strings.TrimSpace(strings.TrimPrefix(value, "..."))}
	}
	if value == "" {
		return nil
	}
	return []string{value}
}

func walkGoTypeOccurrences(expression ast.Expr, target string, repeated bool, occurrences *goTypeOccurrences) {
	if expression == nil {
		return
	}
	walk := func(child ast.Expr) {
		walkGoTypeOccurrences(child, target, repeated, occurrences)
	}
	walkRepeated := func(child ast.Expr) {
		walkGoTypeOccurrences(child, target, true, occurrences)
	}

	switch node := expression.(type) {
	case *ast.Ident:
		if node.Name == target {
			if repeated {
				occurrences.many = true
			} else {
				occurrences.one = true
			}
		}
	case *ast.ParenExpr:
		walk(node.X)
	case *ast.StarExpr:
		walk(node.X)
	case *ast.ArrayType:
		// Array lengths are values, not type references.
		walkRepeated(node.Elt)
	case *ast.MapType:
		walkRepeated(node.Key)
		walkRepeated(node.Value)
	case *ast.ChanType:
		walkRepeated(node.Value)
	case *ast.IndexExpr:
		walk(node.X)
		walk(node.Index)
	case *ast.IndexListExpr:
		walk(node.X)
		for _, index := range node.Indices {
			walk(index)
		}
	case *ast.FuncType:
		walkGoFieldList(node.Params, target, repeated, occurrences)
		walkGoFieldList(node.Results, target, repeated, occurrences)
	case *ast.StructType:
		walkGoFieldList(node.Fields, target, repeated, occurrences)
	case *ast.InterfaceType:
		walkGoFieldList(node.Methods, target, repeated, occurrences)
	case *ast.Ellipsis:
		walk(node.Elt)
	case *ast.UnaryExpr:
		// Type-set approximation (~T).
		walk(node.X)
	case *ast.BinaryExpr:
		// Type-set unions (A | B).
		walk(node.X)
		walk(node.Y)
	case *ast.SelectorExpr:
		// A selector's qualifier and selected name are not unqualified local
		// type references. Type arguments nested in its receiver still are.
		walkGoSelectorReceiver(node.X, target, repeated, occurrences)
	}
}

func walkGoFieldList(fields *ast.FieldList, target string, repeated bool, occurrences *goTypeOccurrences) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		// Field and parameter names are deliberately ignored.
		walkGoTypeOccurrences(field.Type, target, repeated, occurrences)
	}
}

func walkGoSelectorReceiver(expression ast.Expr, target string, repeated bool, occurrences *goTypeOccurrences) {
	switch node := expression.(type) {
	case *ast.Ident:
		return
	case *ast.SelectorExpr:
		walkGoSelectorReceiver(node.X, target, repeated, occurrences)
	case *ast.IndexExpr:
		walkGoSelectorReceiver(node.X, target, repeated, occurrences)
		walkGoTypeOccurrences(node.Index, target, repeated, occurrences)
	case *ast.IndexListExpr:
		walkGoSelectorReceiver(node.X, target, repeated, occurrences)
		for _, index := range node.Indices {
			walkGoTypeOccurrences(index, target, repeated, occurrences)
		}
	default:
		walkGoTypeOccurrences(expression, target, repeated, occurrences)
	}
}
