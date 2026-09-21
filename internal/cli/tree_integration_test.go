package cli

import (
	"strings"
	"testing"
)

func TestRunTreeListsLocalCheckoutByDefault(t *testing.T) {
	root := chdirTemp(t)
	writeGraphSource(t, root, "src/main.go", "package main\n")
	writeGraphSource(t, root, "src/nested/helper.go", "package nested\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"tree", "src", "--depth", "2"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"./src\n", "├── nested/", "│   └── helper.go", "└── main.go"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("local tree missing %q:\n%s", expected, output)
		}
	}
}