package outputspill

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRunWithFinalizerPreservesCommandErrorAndOutput(t *testing.T) {
	root := t.TempDir()
	output, err := os.Create(filepath.Join(root, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = output
	t.Cleanup(func() { os.Stdout = original; _ = output.Close() })
	var stages []string
	failure := errors.New("private command failure")
	err = RunWithFinalizer([]string{"search"}, Options{Directory: root, Threshold: 64}, 64, root, func(int) error {
		stages = append(stages, "command")
		_, _ = os.Stdout.WriteString("small output\n")
		return failure
	}, func(finish func() error) error {
		stages = append(stages, "finalize")
		if os.Stdout != output {
			t.Error("command stdout was not restored before finalization")
		}
		return finish()
	})
	if !errors.Is(err, failure) || !reflect.DeepEqual(stages, []string{"command", "finalize"}) {
		t.Fatalf("changed error or execution order: %v, %v", err, stages)
	}
	data, readErr := os.ReadFile(output.Name())
	if readErr != nil || string(data) != "small output\n" {
		t.Fatalf("changed output: %q, %v", data, readErr)
	}
}

func TestRunWithFinalizerSkipsArtifactCleanBypass(t *testing.T) {
	failure := errors.New("command failed")
	called := false
	err := RunWithFinalizer([]string{"artifacts", "clean"}, Options{Disabled: true}, 64, t.TempDir(), func(int) error { return failure }, func(finish func() error) error {
		called = true
		return finish()
	})
	if !errors.Is(err, failure) || called {
		t.Fatalf("bypass changed: err=%v finalizer=%t", err, called)
	}
}
