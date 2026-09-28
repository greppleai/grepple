package hook

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHookEnabledDefaultsAndToggle(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, "case.go", "package demo\nfunc run(ok bool) { if ok {} }\n")
	path := filepath.Join(root, ".grepple/hooks/go-empty-if.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, setting string
		wantActive    bool
	}{
		{"omitted", "", true},
		{"explicit true", "enabled: true\n", true},
		{"disabled", "enabled: false\n", false},
		{"re-enabled", "enabled: true\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := strings.Replace(string(original), "version: 1\n", "version: 1\n"+test.setting, 1)
			writeHookTestFile(t, root, ".grepple/hooks/go-empty-if.yaml", config)
			report, status, err := runHookTest(t, "--all", "--id", "go-empty-if")
			if err != nil {
				t.Fatal(err)
			}
			if test.wantActive {
				if status != 1 || report.Files != 1 || !reflect.DeepEqual(report.Hooks, []string{"go-empty-if"}) || len(report.Findings) != 1 {
					t.Fatalf("enabled: report=%+v status=%d", report, status)
				}
			} else if status != 0 || report.Files != 0 || len(report.Hooks) != 0 || len(report.Findings) != 0 {
				t.Fatalf("disabled: report=%+v status=%d", report, status)
			}
		})
	}
}

func TestHookEnabledFiltersBeforeScanningEachEngine(t *testing.T) {
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	writeHookTestFile(t, root, "case.go", "package demo\ntype worker struct{}\nfunc (*worker) unused(ok bool) { if ok {} }\n")
	for _, id := range []string{"go-empty-if", "go-mccabe", "go-uncalled-methods"} {
		t.Run(id, func(t *testing.T) {
			fixture := filepath.Join(sourceDirectory, "..", "..", "..", ".grepple", "hooks", id+".yaml")
			content, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			config := strings.Replace(string(content), "version: 1\n", "version: 1\nenabled: false\n", 1)
			writeHookTestFile(t, root, ".grepple/hooks/"+id+".yaml", config)
			report, status, err := runHookTest(t, "--all", "--id", id)
			if err != nil || status != 0 || report.Files != 0 || len(report.Hooks) != 0 || len(report.Findings) != 0 {
				t.Fatalf("disabled %s: report=%+v status=%d err=%v", id, report, status, err)
			}
			config = strings.Replace(config, "enabled: false", "enabled: true", 1)
			writeHookTestFile(t, root, ".grepple/hooks/"+id+".yaml", config)
			report, status, err = runHookTest(t, "--all", "--id", id)
			if err != nil || report.Files != 1 || !reflect.DeepEqual(report.Hooks, []string{id}) || status != 0 && status != 1 {
				t.Fatalf("enabled %s: report=%+v status=%d err=%v", id, report, status, err)
			}
		})
	}
}

func TestHookEnabledRejectsNonBooleanAndInvalidRules(t *testing.T) {
	root := testHookRepository(t)
	path := filepath.Join(root, ".grepple/hooks/go-empty-if.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"null", "\"false\"", "nope", "0"} {
		t.Run(value, func(t *testing.T) {
			config := strings.Replace(string(original), "version: 1\n", "version: 1\nenabled: "+value+"\n", 1)
			writeHookTestFile(t, root, ".grepple/hooks/go-empty-if.yaml", config)
			if _, _, err := runHookTest(t, "--all", "--id", "go-empty-if"); err == nil || !strings.Contains(err.Error(), "enabled must be true or false") {
				t.Fatalf("enabled=%s: error=%v", value, err)
			}
		})
	}
	config := strings.Replace(string(original), "version: 1\n", "version: 1\nenabled: false\n", 1)
	config = strings.Replace(config, "language go", "language bogus", 1)
	writeHookTestFile(t, root, ".grepple/hooks/go-empty-if.yaml", config)
	if _, _, err := runHookTest(t, "--all", "--id", "go-empty-if"); err == nil {
		t.Fatal("disabled hooks must still be validated")
	}
}

func TestHookEnabledMixedSelection(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, "dot.go", "package demo\nimport . \"fmt\"\n")
	path := filepath.Join(root, ".grepple/hooks/go-empty-if.yaml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-empty-if.yaml", strings.Replace(string(original), "version: 1\n", "version: 1\nenabled: false\n", 1))
	report, status, err := runHookTest(t, "--all", "--id", "go-empty-if", "--id", "go-direct-dot-import")
	if err != nil || status != 1 || !reflect.DeepEqual(report.Hooks, []string{"go-direct-dot-import"}) || len(report.Findings) != 1 || report.Findings[0].ID != "go-direct-dot-import" {
		t.Fatalf("mixed: report=%+v status=%d err=%v", report, status, err)
	}
}
