package cliruntime

import (
	"errors"
	"reflect"
	"testing"
)

func TestCommandFunc(t *testing.T) {
	wantErr := errors.New("failed")
	var received []string
	var command Command = CommandFunc(func(args []string) error {
		received = append([]string(nil), args...)
		return wantErr
	})
	if err := command.Run([]string{"--json", "value"}); !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want %v", err, wantErr)
	}
	if want := []string{"--json", "value"}; !reflect.DeepEqual(received, want) {
		t.Fatalf("Run args = %v, want %v", received, want)
	}
}
