package extract

import (
	"fmt"
	goparser "go/parser"
	"go/token"
	"strings"
)

func validGoTypeExpression(expression string) bool {
	_, err := goparser.ParseFile(token.NewFileSet(), "underlying.go", "package underlying\ntype value "+expression, 0)
	return err == nil
}

func malformedNote(line int) error {
	return fmt.Errorf("Line %d: malformed note; expected 'note \"text\"' or 'note for <declared-node> \"text\"'", line)
}

// ParseClassDiagram parses the supported Mermaid class schema.
func ParseClassDiagram(source string) (*ClassDiagram, error) {
	parser := newClassParser()
	for index, raw := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if err := parser.parseLine(strings.TrimSpace(raw), index+1); err != nil {
			return nil, err
		}
	}
	if !parser.sawHeader {
		return nil, fmt.Errorf("Diagram is empty")
	}
	if parser.current != nil {
		return nil, fmt.Errorf("Diagram node '%s' is missing '}'", parser.current.Name)
	}
	if parser.namespace != "" {
		return nil, fmt.Errorf("Namespace '%s' opened on line %d is missing '}'", parser.namespace, parser.namespaceLine)
	}
	if err := validateRelationMultiplicities(parser.diagram); err != nil {
		return nil, err
	}
	if err := validateRelationEndpoints(parser.diagram); err != nil {
		return nil, err
	}
	if err := applyClassDefaults(parser.diagram); err != nil {
		return nil, err
	}
	finalizeDiagramTypes(parser.diagram)
	return parser.diagram, nil
}
