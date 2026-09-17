package parser

import (
	"os"
	"regexp"
	"testing"
)

func TestSharedNavigationHelpersHaveNoLanguagePrefixedFunctions(t *testing.T) {
	content, err := os.ReadFile("language_navigation.go")
	if err != nil {
		t.Fatal(err)
	}
	prefixed := regexp.MustCompile(`(?m)^func (?:go|rust|python|typeScript|javaScript|java|kotlin|cSharp|cFamily|ecma)[A-Z]`)
	if match := prefixed.Find(content); match != nil {
		t.Fatalf("shared parser navigation contains language callback %q", match)
	}
}
