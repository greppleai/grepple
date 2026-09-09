package mermaidcode

import (
	"strings"
	"testing"
)

const cardinalitySource = `
class Item {}
class Singular { item: Item = new Item(); }
class Collection { items: Item[] = []; }
function singular(item: Item): Item { return item; }
function collection(items: Item[]): Item[] { return items; }
`

func cardinalityDiagram(relation string) string {
	return `classDiagram
 class Item
 class Singular
 class Collection
 class singular {
  +singular(Item): Item
 }
 <<function>> singular
 class collection {
  +collection(Item[]): Item[]
 }
 <<function>> collection
 ` + relation + "\n"
}

func TestAssociationTargetMultiplicityMatchesReferenceSyntax(t *testing.T) {
	tests := []struct {
		name, relation string
		valid          bool
		message        string
	}{
		{name: "type singular one", relation: `Singular "1" --> "1" Item`, valid: true},
		{name: "type collection many", relation: `Collection "1" --> "*" Item`, valid: true},
		{name: "function singular one", relation: `singular --> "1" Item`, valid: true},
		{name: "function collection many", relation: `collection --> "*" Item`, valid: true},
		{name: "omitted accepts singular", relation: `Singular --> Item`, valid: true},
		{name: "omitted accepts collection", relation: `Collection --> Item`, valid: true},
		{name: "many rejects singular", relation: `Singular --> "*" Item`, message: "a collection of 'Item'"},
		{name: "one rejects collection", relation: `Collection --> "1" Item`, message: "a non-collection reference to 'Item'"},
		{name: "function many rejects singular", relation: `singular --> "*" Item`, message: "a collection of 'Item'"},
		{name: "function one rejects collection", relation: `collection --> "1" Item`, message: "a non-collection reference to 'Item'"},
		{name: "reversed arrow", relation: `Item "*" <-- "1" Collection`, valid: true},
		{name: "reversed arrow mismatch", relation: `Item "1" <-- Collection`, message: "a non-collection reference to 'Item'"},
		{name: "forward composition", relation: `Collection "1" *-- "*" Item`, valid: true},
		{name: "reversed composition", relation: `Item "*" --* "1" Collection`, valid: true},
		{name: "forward aggregation mismatch", relation: `Singular o-- "*" Item`, message: "a collection of 'Item'"},
		{name: "reversed aggregation mismatch", relation: `Item "1" --o Collection`, message: "a non-collection reference to 'Item'"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertAssociationCardinality(t, test.relation, test.valid, test.message)
		})
	}
}

func assertAssociationCardinality(t *testing.T, relation string, valid bool, message string) {
	t.Helper()
	diagnostics, err := CheckClassDiagram(cardinalityDiagram(relation), []Source{{"cardinality.ts", cardinalitySource}})
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		if len(diagnostics) != 0 {
			t.Fatalf("valid association produced diagnostics: %+v", diagnostics)
		}
		return
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, message) {
		t.Fatalf("expected diagnostic containing %q, got %+v", message, diagnostics)
	}
}

func TestAssociationRejectsUnsupportedMultiplicity(t *testing.T) {
	for _, relation := range []string{
		`Singular --> "0..1" Item`,
		`Singular "many" --> Item`,
		`Item "0..*" <-- Collection`,
		`Item --o "2" Collection`,
	} {
		t.Run(relation, func(t *testing.T) {
			_, err := ParseClassDiagram(cardinalityDiagram(relation))
			if err == nil || !strings.Contains(err.Error(), "unsupported association multiplicity") {
				t.Fatalf("expected unsupported multiplicity error, got %v", err)
			}
		})
	}
}
