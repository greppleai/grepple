package extract

import (
	"os"
	"regexp"
	"testing"
)

func TestSharedFocusedProjectionHasNoLanguageDispatch(t *testing.T) {
	t.Helper()
	shared := []string{"class.go", "flow.go", "scope.go", "generate.go", "navigation.go"}
	languageDispatch := regexp.MustCompile(`(?:Language|language|owner\.Language|declaration\.Language)\s*(?:==|!=)\s*"(?:go|javascript|typescript|tsx|python|java|kotlin|csharp|rust|c|cpp)"`)
	prefixedFunction := regexp.MustCompile(`(?m)^func (?:\([^\n]*\) )?(?:go|rust|python|typeScript|javaScript|java|kotlin|cSharp|cFamily|ecma)[A-Z]`)
	for _, path := range shared {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if match := languageDispatch.Find(content); match != nil {
			t.Fatalf("%s contains direct language dispatch %q; register a focused language capability instead", path, match)
		}
		if match := prefixedFunction.Find(content); match != nil {
			t.Fatalf("%s contains language-owned helper %q", path, match)
		}
	}
}
