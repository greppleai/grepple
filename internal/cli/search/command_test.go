package search

import (
	"reflect"
	"testing"
)

func TestCommandDelegatesSearchArguments(t *testing.T) {
	var received []string
	command := New(Dependencies{Execute: func(args []string) error { received = append([]string(nil), args...); return nil }})
	if err := command.Run([]string{"needle", "src"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"needle", "src"}; !reflect.DeepEqual(received, want) {
		t.Fatalf("args=%v want=%v", received, want)
	}
}
