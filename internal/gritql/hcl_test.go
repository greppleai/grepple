package gritql

import (
	"context"
	"testing"
)

func TestHCLReadOnlyStructuralMatching(t *testing.T) {
	program, err := Compile([]byte("language hcl\n`resource \"aws_instance\" \"web\" { ami = $value }`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{
		Path: "main.tf", Content: []byte("resource \"aws_instance\" \"web\" { ami = var.ami }\nresource \"aws_instance\" \"other\" { ami = var.ami }\n"),
	}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 || len(result.Findings()) != 1 {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
	if got := result.Findings()[0].Text(); got != `resource "aws_instance" "web" { ami = var.ami }` {
		t.Fatalf("unexpected HCL match: %q", got)
	}
}
