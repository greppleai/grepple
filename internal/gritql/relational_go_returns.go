package gritql

import (
	"fmt"
	"sort"
)

// goResultTypeKeys projects the top-level types in a Go function's declared
// results. An absent optional result binding means the function has no results.
// Only a direct interface result satisfies the rule without a named lookup;
// pointers, slices and imported selectors are not mistaken for local interfaces.
func goResultTypeKeys(finding Finding, bindingName string) ([]string, bool, error) {
	var result StructuralNode
	for _, binding := range finding.Bindings() {
		if binding.Name() != bindingName {
			continue
		}
		var ok bool
		result, ok = binding.Node()
		if !ok {
			return nil, false, fmt.Errorf("%s: Go result %q must bind one node", finding.Path(), bindingName)
		}
		break
	}
	if !result.Valid() {
		return nil, false, nil
	}
	types := []StructuralNode{result}
	if result.Kind() == "parameter_list" {
		types = nil
		for _, parameter := range result.Children() {
			if parameter.Kind() != "parameter_declaration" {
				continue
			}
			var typ StructuralNode
			for _, child := range parameter.Children() {
				if child.NodeKind() != "" {
					typ = child // the type follows any declared result names
				}
			}
			if typ.Valid() {
				types = append(types, typ)
			}
		}
	}
	var keys []string
	for _, typ := range types {
		if typ.Kind() == "interface_type" {
			return nil, true, nil
		}
		if typ.Kind() == "generic_type" {
			// A generic named interface still has its own local declaration.
			for _, child := range typ.Children() {
				if child.Kind() == "type_identifier" {
					typ = child
					break
				}
			}
		}
		if typ.Kind() != "type_identifier" {
			continue
		}
		if typ.Lexeme() == "error" || typ.Lexeme() == "any" {
			return nil, true, nil // predeclared interface types
		}
		encoded, err := typ.MarshalJSON()
		if err != nil {
			return nil, false, err
		}
		keys = append(keys, string(encoded))
	}
	return keys, false, nil
}

// unmatchedAnyRelationFacts reports one finding per left declaration when no
// declared result type matches a right-side declaration in its Go package.
func unmatchedAnyRelationFacts(left, right []relationFact) []RelationHit {
	seen := make(map[string]bool, len(right))
	for _, fact := range right {
		seen[fact.group+"\x00"+fact.partition+"\x00"+fact.key] = true
	}
	var hits []RelationHit
	for _, fact := range left {
		if fact.satisfied {
			continue
		}
		matched := false
		for _, key := range fact.keys {
			if seen[fact.group+"\x00"+fact.partition+"\x00"+key] {
				matched = true
				break
			}
		}
		if !matched {
			hits = append(hits, RelationHit{Left: fact.finding})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i].Left, hits[j].Left
		if a.Path() != b.Path() {
			return a.Path() < b.Path()
		}
		return a.StartByte() < b.StartByte()
	})
	return hits
}
