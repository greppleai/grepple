package hook

import (
	"context"
	"reflect"
	"testing"
)

func TestEmptyBlockRuleCoversReviveControlFlowAndExemptions(t *testing.T) {
	root := testHookRepository(t)
	const source = `package demo
func emptyFunction() {}
func emptyReturn() int {}
func (receiver *Args) emptyMethod() {}
func (receiver *Args) emptyMethodReturn() int {}
func generic[T any]() {}
func callCondition() bool { return true }
func check(condition bool) {
    _ = func() {}
    _ = func() int {}
    if condition {}
    if condition {} else {}
    for condition {}
    for {}
    for callCondition() {}
    for range []int{} {}
    switch condition {}
    switch {}
    switch item := any(condition).(type) {}
    select {}
    {}
    _ = func() { if condition {} }
}
func genericTwo[T, U any]() {}
func typedGeneric[T, U any]() int {}
func genericTriple[T, U, V any]() {}
func nonempty(condition bool) { if condition { target() } }
`
	writeHookTestFile(t, root, "blocks.go", source)
	rules, err := loadRules(root, []string{"go-empty-if"})
	if err != nil {
		t.Fatal(err)
	}
	findings, err := scanRulesUncached(context.Background(), root, []string{"blocks.go"}, rules, false, 4)
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, finding := range findings {
		lines = append(lines, finding.Line)
	}
	want := []int{11, 12, 12, 13, 14, 16, 17, 18, 19, 21, 22}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("empty block lines=%v want=%v", lines, want)
	}
}
