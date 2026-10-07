package hook

import (
	"strings"
	"testing"
)

const interfaceConstructorFixture = `version: 1
id: interface-api
engine: gritql-relational-v1
include: ["**/*.go"]
severity: error
unsuppressible: true
message: "invalid export {{left.name}}"
relation:
  go_module: example.test/project
  left_query: |
    language go
    and { function_declaration(name=$name), maybe function_declaration(result=$result) } where { $name <: r"^\\p{Lu}" }
  right_query: |
    language go
    type_spec(name=$type, type=interface_type())
  partition_query: |
    language go
    source_file() as $source
  left_key: {binding: result, projection: go-interface-returns}
  right_key: {binding: type}
  partition_key: {binding: source}
  scope: repository
  mode: unmatched_left_any
`

const interfaceMethodFixture = `version: 1
id: interface-api
engine: gritql-relational-v1
include: ["**/*.go"]
severity: error
unsuppressible: true
message: "invalid export {{left.name}}"
relation:
  go_module: example.test/project
  left_query: |
    language go
    method_declaration(name=$name) where { $name <: r"^\\p{Lu}" }
  right_query: |
    language go
    method_elem(name=$name)
  partition_query: |
    language go
    source_file() as $source
  left_key: {binding: name, projection: go-method-signatures}
  right_key: {binding: name}
  partition_key: {binding: source}
  scope: repository
  mode: unmatched_left
`

func TestInterfaceConstructorProjection(t *testing.T) {
	cases := map[string]struct {
		source   string
		findings int
	}{
		"local":                            {"type Local interface { Run() }\nfunc New() Local { return nil }", 0},
		"named results":                    {"type Local interface { Run() }\nfunc New() (value Local, err error) { return }", 0},
		"imported alias":                   {"import contract \"example.test/project/contracts\"\nfunc New() (contract.API,error) { return nil,nil }", 0},
		"imported package name":            {"import \"example.test/project/contracts\"\nfunc New() unusual.API { return nil }", 0},
		"inline capability":                {"func New() interface { Run() } { return nil }", 0},
		"error alone":                      {"func New() error { return nil }", 1},
		"any alone":                        {"func New() any { return nil }", 1},
		"empty literal":                    {"func New() interface{} { return nil }", 1},
		"empty named":                      {"type Empty interface{}\nfunc New() Empty { return nil }", 1},
		"concrete and error":               {"type Value struct{}\nfunc New() (*Value,error) { return nil,nil }", 1},
		"pointer interface":                {"type Local interface {Run()}\nfunc New() *Local { return nil }", 1},
		"slice interface":                  {"type Local interface {Run()}\nfunc New() []Local { return nil }", 1},
		"same name is concrete":            {"type API struct{}\nfunc New() API { return API{} }", 1},
		"no results":                       {"func Exported() {}", 1},
		"type parameter shadows interface": {"type Product interface {Run()}\nfunc New[Product any]() Product {var value Product;return value}", 1},
		"generic interface":                {"type Product[T any] interface {Run(T)}\nfunc New() Product[string] {return nil}", 0},
		"private":                          {"func helper() error { return nil }", 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := testHookRepository(t)
			writeHookTestFile(t, root, ".grepple/hooks/interface-api.yaml", interfaceConstructorFixture)
			writeHookTestFile(t, root, "contracts/api.go", "package unusual\ntype API interface { Run() }\n")
			writeHookTestFile(t, root, "factory.go", "// Leading comments deliberately shift the source-file start.\npackage p\n"+c.source+"\n")
			assertInterfaceReport(t, c.findings)
		})
	}
}

func assertInterfaceReport(t *testing.T, want int) {
	t.Helper()
	report, status, err := runHookTest(t, "--all", "--id", "interface-api")
	if err != nil || len(report.Findings) != want || (status != 0) != (want != 0) {
		t.Fatalf("report=%+v status=%d err=%v", report, status, err)
	}
}

func TestInterfaceMethodSignatures(t *testing.T) {
	cases := map[string]struct {
		method   string
		findings int
	}{
		"matches":         {"func (value) Run(name string, count int) error {return nil}", 0},
		"wrong parameter": {"func (value) Run(name int, count int) error {return nil}", 1},
		"wrong result":    {"func (value) Run(name string, count int) bool {return false}", 1},
		"no contract":     {"func (value) Extra() {}", 1},
		"builtin error":   {"func (value) Error() string {return \"x\"}", 0},
		"wrong Error":     {"func (value) Error() int {return 0}", 1},
		"private":         {"func (value) helper() {}", 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := testHookRepository(t)
			writeHookTestFile(t, root, ".grepple/hooks/interface-api.yaml", interfaceMethodFixture)
			writeHookTestFile(t, root, "contracts/api.go", "package contract\ntype API interface { Run(string,int) error }\n")
			writeHookTestFile(t, root, "implementation.go", "package p\ntype value struct{}\n"+c.method+"\n")
			assertInterfaceReport(t, c.findings)
		})
	}
}

func TestInterfaceProjectionsCacheAndChangedReporting(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/interface-api.yaml", interfaceMethodFixture)
	writeHookTestFile(t, root, "contracts/api.go", "package contract\ntype API interface { Run(string) }\n")
	writeHookTestFile(t, root, "implementation.go", "package p\ntype value struct{}\n//grepple interface-api ignored\nfunc (value) Run(string) {}\n")
	assertInterfaceReport(t, 0)
	writeHookTestFile(t, root, "contracts/api.go", "package contract\ntype API interface { Run(int) }\n")
	assertInterfaceReport(t, 1)
	assertInterfaceReport(t, 1)
	// Filtering output must still retain unchanged contract declarations.
	config := strings.Replace(interfaceMethodFixture, "  go_module:", "  left_include: [implementation.go]\n  go_module:", 1)
	writeHookTestFile(t, root, ".grepple/hooks/interface-api.yaml", config)
	assertInterfaceReport(t, 1)
}

func TestInterfaceMethodAliasesAndVariadicFields(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/interface-api.yaml", interfaceMethodFixture)
	writeHookTestFile(t, root, "data/value.go", "package unusual\ntype Value struct{}\n")
	writeHookTestFile(t, root, "contracts/api.go", "package contract\nimport data \"example.test/project/data\"\ntype API interface { Run(data.Value, ...string) error }\n")
	writeHookTestFile(t, root, "implementation.go", "package p\nimport other \"example.test/project/data\"\ntype value struct{}\nfunc (value) Run(argument other.Value, rest ...string) error {return nil}\n")
	assertInterfaceReport(t, 0)
	writeHookTestFile(t, root, "implementation.go", "package p\nimport other \"example.test/project/data\"\ntype value struct{}\nfunc (value) Run(argument other.Value, rest []string) error {return nil}\n")
	assertInterfaceReport(t, 1)
}

func TestInterfaceProjectionFailureIsNotACleanResult(t *testing.T) {
	for _, config := range []string{
		strings.Replace(interfaceConstructorFixture, "  go_module: example.test/project\n", "", 1),
		strings.Replace(interfaceConstructorFixture, "scope: repository", "scope: directory", 1),
		strings.Replace(interfaceConstructorFixture, "source_file() as $source", "package_clause($source)", 1),
		strings.Replace(interfaceMethodFixture, "mode: unmatched_left", "mode: unmatched_left_any", 1),
	} {
		t.Run(config, func(t *testing.T) {
			root := testHookRepository(t)
			writeHookTestFile(t, root, ".grepple/hooks/interface-api.yaml", config)
			writeHookTestFile(t, root, "factory.go", "package p\nfunc Exported() {}\n")
			if _, _, err := runHookTest(t, "--all", "--id", "interface-api"); err == nil {
				t.Fatal("invalid/incomplete projection accepted")
			}
		})
	}
}
