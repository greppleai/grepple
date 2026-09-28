package hook

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestStandaloneFunctionsWarning(t *testing.T) {
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	config, err := os.ReadFile(filepath.Join(sourceDirectory, "..", "..", "..", ".grepple", "hooks", "go-standalone-functions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-standalone-functions.yaml", string(config))
	writeHookTestFile(t, root, "api/service.go", `package api

type Service interface { Run() }
type impl struct{}
func (impl) Run() {}
func NewService() Service { return &impl{} }
func NewValueService() Service { return impl{} }
func NewServiceWithError() (Service, error) { return &impl{}, nil }
func DelegatingService() Service { return NewService() }
func Pure() int { return 7 }
func WrongConcrete() impl { return impl{} }
func ConcreteWithError() (impl, error) { return impl{}, nil }
func InterfaceLast() (impl, Service) { return impl{}, nil }
func NamedResults() (value impl, service Service) { return impl{}, nil }
func Inline() interface{ Run() } { return impl{} }
func NoReturn() {}
func StructPointer() *impl { return &impl{} }
func StructSlice() []Service { return nil }
func Imported() other.Service { return nil }
func CrossFile() CrossInterface { return nil }
func init() {}
func main() {}
func (impl) Method() impl { return impl{} }
`)
	writeHookTestFile(t, root, "api/types.go", "package api\ntype CrossInterface interface { Run() }\n")
	writeHookTestFile(t, root, "other/types.go", "package other\ntype Service interface { Run() }\n")
	writeHookTestFile(t, root, "internal/service/helpers.go", `package service
func navigationFieldKey() {}
func BuildGraphFromDocuments() {}
func Éclair() {}
func échec() {}
`)
	writeHookTestFile(t, root, "api/service_test.go", "package api\nfunc TestHelper() {}\n")
	writeHookTestFile(t, root, "internal/service/testdata/fixture.go", "package service\nfunc Fixture() {}\n")

	report, status, err := runHookTest(t, "--all", "--id", "go-standalone-functions")
	if err != nil || status != 1 {
		t.Fatalf("hook: status=%d err=%v report=%+v", status, err, report)
	}
	if !reflect.DeepEqual(report.Hooks, []string{"go-standalone-functions"}) {
		t.Fatalf("hooks=%v", report.Hooks)
	}
	var got []string
	for _, finding := range report.Findings {
		if finding.Severity != "warning" {
			t.Fatalf("finding severity=%q", finding.Severity)
		}
		got = append(got, finding.Path+":"+strconv.Itoa(finding.Line))
	}
	want := []string{
		"api/service.go:10", "api/service.go:11", "api/service.go:16",
		"api/service.go:17", "api/service.go:18", "api/service.go:19",
		"internal/service/helpers.go:3", "internal/service/helpers.go:4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings=%v, want %v", got, want)
	}
}

func TestStandaloneFunctionsDefaultReportsOnlyChanged(t *testing.T) {
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	config, err := os.ReadFile(filepath.Join(sourceDirectory, "..", "..", "..", ".grepple", "hooks", "go-standalone-functions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-standalone-functions.yaml", string(config))
	gitHookTest(t, root, "init", "-q")
	writeHookTestFile(t, root, "api/types.go", "package api\ntype Service interface { Run() }\n")
	writeHookTestFile(t, root, "api/old.go", "package api\nfunc Old() int { return 0 }\nfunc old() int { return 0 }\n")
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "baseline")
	writeHookTestFile(t, root, "api/new.go", "package api\nfunc New() int { return 1 }\nfunc new() int { return 1 }\n")

	report, status, err := runHookTest(t, "--id", "go-standalone-functions")
	if err != nil || status != 1 || len(report.Findings) != 1 || report.Findings[0].Path != "api/new.go" {
		t.Fatalf("changed report: status=%d err=%v report=%+v", status, err, report)
	}
	report, status, err = runHookTest(t, "--all", "--id", "go-standalone-functions")
	if err != nil || status != 1 || len(report.Findings) != 2 || report.Findings[0].Path != "api/new.go" || report.Findings[1].Path != "api/old.go" {
		t.Fatalf("all report: status=%d err=%v report=%+v", status, err, report)
	}
	gitHookTest(t, root, "add", "api/new.go")
	gitHookTest(t, root, "commit", "-qm", "new source")
	report, status, err = runHookTest(t, "--id", "go-standalone-functions")
	if err != nil || status != 0 || len(report.Findings) != 0 {
		t.Fatalf("clean report: status=%d err=%v report=%+v", status, err, report)
	}
}
