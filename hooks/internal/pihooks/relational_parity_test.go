package pihooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/hookruntime"
)

func TestGritQLRelationIntegrationCases(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  int
	}{
		{"grouped-generic-and-pointer", map[string]string{
			"a/types.go":   "package a\ntype (\n list[T any] struct{}\n pair[A,B any] struct{}\n)\n",
			"a/methods.go": "package a\nfunc (l *list[T]) One() {}\nfunc (*list[T]) Two() {}\nfunc (p pair[A,B]) Three() {}\n",
		}, 3},
		{"package-and-directory", map[string]string{
			"a/types.go":         "package a\ntype thing struct{}\n",
			"a/methods.go":       "package a\nfunc (thing) Report() {}\n",
			"a/external_test.go": "package a_test\nfunc (thing) Excluded() {}\n",
			"b/methods.go":       "package a\nfunc (thing) OtherDirectory() {}\n",
		}, 1},
		{"ambiguous-variants", map[string]string{
			"a/type_linux.go":   "package a\ntype thing struct{}\n",
			"a/type_windows.go": "package a\ntype thing struct{}\n",
			"a/method.go":       "package a\nfunc (thing) Ambiguous() {}\n",
		}, 0},
		{"local-type-does-not-hide-global", map[string]string{
			"a/types.go":  "package a\ntype thing struct{}\n",
			"a/helper.go": "package a\nfunc helper() { type thing struct{} }\n",
			"a/method.go": "package a\nfunc (thing) Global() {}\n",
		}, 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := makeRepository(t, test.files)
			legacy, err := AnalyzeRepository(root)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := hookruntime.CheckRelation(context.Background(), root, "same-file-struct-methods")
			if err != nil {
				t.Fatal(err)
			}
			if len(actual) != test.want || len(legacy) != test.want {
				t.Fatalf("adapter=%+v relation=%+v want %d", legacy, actual, test.want)
			}
			for index, item := range actual {
				old := legacy[index]
				if filepath.Join(root, filepath.FromSlash(item.Path)) != old.Position.Start.Filename || item.Line != old.Position.Start.Line || item.Message != old.Failure || item.Severity != old.Severity {
					t.Fatalf("finding[%d] legacy=%+v relation=%+v", index, old, item)
				}
			}
		})
	}
}

func TestRelationalGoConfigRejectsUnknownReceiverKind(t *testing.T) {
	root := makeRepository(t, map[string]string{"type.go": "package sample\ntype thing struct{}\n", "method.go": "package sample\nfunc (thing) Bad() {}\n"})
	configPath := filepath.Join(root, ".grepple", "hooks", "same-file-struct-methods.yaml")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(strings.Replace(string(contents), "type_identifier", "not_a_kind", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AnalyzeRepository(root); err == nil || !strings.Contains(err.Error(), "descendant kind") {
		t.Fatalf("invalid config error=%v", err)
	}
}
