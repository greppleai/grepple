package parser

import (
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// NavigationVisibility classifies whether a declaration is externally visible.
type NavigationVisibility string

const (
	// NavigationVisibilityPublic marks language-public or exported declarations.
	NavigationVisibilityPublic NavigationVisibility = "public"
	// NavigationVisibilityNonPublic marks declarations known not to be public.
	NavigationVisibilityNonPublic NavigationVisibility = "non-public"
	// NavigationVisibilityUnknown marks languages without a reliable visibility rule.
	NavigationVisibilityUnknown NavigationVisibility = "unknown"
)

func navigationDeclarationVisibility(node *sitter.Node, language, name, content string) NavigationVisibility {
	terminal := name
	if index := strings.LastIndex(terminal, "."); index >= 0 {
		terminal = terminal[index+1:]
	}
	header := navigationDeclarationHeader(node, content)
	switch language {
	case "go":
		return goNavigationVisibility(terminal)
	case "python":
		return pythonNavigationVisibility(terminal)
	case "rust":
		return visibilityFromRequiredModifier(header, "pub")
	case "java":
		return javaNavigationVisibility(node, header)
	case "kotlin":
		return visibilityFromModifiers(header, true)
	case "csharp":
		return cSharpNavigationVisibility(header)
	case "javascript", "typescript", "tsx":
		return typeScriptNavigationVisibility(node, terminal, header)
	default:
		return NavigationVisibilityUnknown
	}
}

func goNavigationVisibility(name string) NavigationVisibility {
	first, _ := utf8.DecodeRuneInString(name)
	if first != utf8.RuneError && unicode.IsUpper(first) {
		return NavigationVisibilityPublic
	}
	return NavigationVisibilityNonPublic
}

func pythonNavigationVisibility(name string) NavigationVisibility {
	if strings.HasPrefix(name, "_") {
		return NavigationVisibilityNonPublic
	}
	return NavigationVisibilityPublic
}

func javaNavigationVisibility(node *sitter.Node, header string) NavigationVisibility {
	if visibility := visibilityFromModifiers(header, false); visibility != NavigationVisibilityUnknown {
		return visibility
	}
	if navigationHasAncestor(node, "interface_declaration", "annotation_type_declaration") {
		return NavigationVisibilityPublic
	}
	return NavigationVisibilityNonPublic
}

func cSharpNavigationVisibility(header string) NavigationVisibility {
	if visibility := visibilityFromModifiers(header, false); visibility != NavigationVisibilityUnknown {
		return visibility
	}
	return NavigationVisibilityNonPublic
}

func typeScriptNavigationVisibility(node *sitter.Node, name, header string) NavigationVisibility {
	if strings.HasPrefix(name, "#") || navigationHeaderHasWord(header, "private") || navigationHeaderHasWord(header, "protected") {
		return NavigationVisibilityNonPublic
	}
	if navigationHasAncestor(node, "class_body", "interface_body", "object_type", "export_statement") {
		return NavigationVisibilityPublic
	}
	return NavigationVisibilityNonPublic
}

func navigationDeclarationHeader(node *sitter.Node, content string) string {
	end := int(node.EndByte())
	for _, field := range []string{"body", "consequence"} {
		if body := node.ChildByFieldName(field); body != nil {
			end = int(body.StartByte())
			break
		}
	}
	start := int(node.StartByte())
	if start < 0 || end < start || end > len(content) {
		return ""
	}
	return content[start:end]
}

func visibilityFromRequiredModifier(header, modifier string) NavigationVisibility {
	if navigationHeaderHasWord(header, modifier) {
		return NavigationVisibilityPublic
	}
	return NavigationVisibilityNonPublic
}

func visibilityFromModifiers(header string, defaultPublic bool) NavigationVisibility {
	if navigationHeaderHasWord(header, "public") {
		return NavigationVisibilityPublic
	}
	for _, modifier := range []string{"private", "protected", "internal"} {
		if navigationHeaderHasWord(header, modifier) {
			return NavigationVisibilityNonPublic
		}
	}
	if defaultPublic {
		return NavigationVisibilityPublic
	}
	return NavigationVisibilityUnknown
}

func navigationHeaderHasWord(header, word string) bool {
	fields := strings.FieldsFunc(header, func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value) && value != '_'
	})
	for _, field := range fields {
		if field == word {
			return true
		}
	}
	return false
}

func navigationHasAncestor(node *sitter.Node, kinds ...string) bool {
	wanted := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		wanted[kind] = true
	}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if wanted[parent.Kind()] {
			return true
		}
	}
	return false
}
