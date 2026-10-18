package directorymeta

import (
	"bytes"
	"strings"
	"testing"
)

const sampleV1 = `schema: v1
directory:
  ".":
    files:
      - name: main.go
        checksum: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        description: Entry point.
        purpose: Implementation
        domainAreas: ["Parser", " parser "]
        functionalAreas: ["CLI"]
        keywords: ["c++", "C++"]
`

func TestDocumentV1RoundTrip(t *testing.T) {
	document, err := ParseDocumentV1([]byte(sampleV1))
	if err != nil {
		t.Fatal(err)
	}
	file := document.Directory["."].Files[0]
	if file.Purpose != "implementation" || len(file.DomainAreas) != 1 || file.DomainAreas[0] != "parser" || len(file.Keywords) != 1 || file.Keywords[0] != "c++" || LabelKey("  IMPLEMENTATION ") != "implementation" {
		t.Fatalf("normalized metadata: %+v", file)
	}
	first, err := MarshalDocumentV1(document)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseDocumentV1(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalDocumentV1(again)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("non-deterministic output: %s; %s; %v", first, second, err)
	}
}

func TestV1CanonicalizesEveryLabelCategoryAndPreservesNonLabels(t *testing.T) {
	original := strings.Replace(sampleV1, "purpose: Implementation", "purpose: Custom  Domain Work", 1)
	original = strings.Replace(original, `domainAreas: ["Parser", " parser "]`, `domainAreas: ["Source  Parsing", "source-parsing"]`, 1)
	original = strings.Replace(original, `functionalAreas: ["CLI"]`, `functionalAreas: ["CLI Navigation"]`, 1)
	original = strings.Replace(original, `keywords: ["c++", "C++"]`, `keywords: ["Tree Sitter", "tree-sitter", "C++"]`, 1)
	document, err := ParseDocumentV1([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	file := document.Directory["."].Files[0]
	if file.Purpose != "custom-domain-work" || !equalStrings(file.DomainAreas, []string{"source-parsing"}) || !equalStrings(file.FunctionalAreas, []string{"cli-navigation"}) || !equalStrings(file.Keywords, []string{"c++", "tree-sitter"}) || file.Description != "Entry point." || file.Checksum != strings.Repeat("a", 64) {
		t.Fatalf("canonical labels altered unrelated fields: %+v", file)
	}
	serialized, err := MarshalDocumentV1(document)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(serialized, []byte("Source  Parsing")) || bytes.Contains(serialized, []byte("Tree Sitter")) || !bytes.Contains(serialized, []byte("source-parsing")) {
		t.Fatalf("not canonical on disk: %s", serialized)
	}
}

func equalStrings(got, want []string) bool {
	return strings.Join(got, "\x00") == strings.Join(want, "\x00")
}

func TestDocumentV1RejectsMalformedAndOldSchemas(t *testing.T) {
	for _, content := range []string{
		"version: 1\ndirectories: {}\n",
		"schema: v2\ndirectory: {}\n",
		"schema: v1\ndirectory: null\n",
		"schema: v1\ndirectory: {}\nschema: v1\n",
		sampleV1 + "---\nschema: v1\n",
		strings.Replace(sampleV1, `"."`, `"../escape"`, 1),
		strings.Replace(sampleV1, "main.go", "../main.go", 1),
		strings.Replace(sampleV1, "main.go", "main.go\n      - name: main.go", 1),
		strings.Replace(sampleV1, "Entry point.", "  ", 1),
		strings.Replace(sampleV1, "Implementation", "  ", 1),
		strings.Replace(sampleV1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bad", 1),
		sampleV1 + "unexpected: true\n",
	} {
		if _, err := ParseDocumentV1([]byte(content)); err == nil {
			t.Errorf("accepted invalid document: %q", content)
		}
	}
}

func TestDocumentV1RejectsDuplicateFileAndNestedUnknownFields(t *testing.T) {
	duplicate := sampleV1 + `      - name: main.go
        checksum: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        description: Second entry.
        purpose: Testing
`
	for _, value := range []string{
		duplicate,
		strings.Replace(sampleV1, "purpose: Implementation", "purpose: Implementation\n        responsibilities: [wrong]", 1),
		strings.Replace(sampleV1, "main.go", "nested/main.go", 1),
		strings.Replace(sampleV1, `"."`, `"a/../b"`, 1),
		strings.Replace(sampleV1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", strings.Repeat("A", 64), 1),
	} {
		if _, err := ParseDocumentV1([]byte(value)); err == nil {
			t.Errorf("accepted invalid document: %q", value)
		}
	}
}
