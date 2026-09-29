package parser

import "testing"

const terraformExample = `variable "ami" {
  type = string
}

resource "aws_instance" "web" {
  ami = var.ami
  tags = {
    Name = "web"
  }
  lifecycle {
    prevent_destroy = true
  }
}

output "instance_id" {
  value = aws_instance.web.id
}
`

func TestHCLFileClassification(t *testing.T) {
	service := NewParser()
	for _, path := range []string{"main.tf", "dev.tfvars", "config.hcl", "backend.tfbackend"} {
		if got := service.LanguageFor(path); got != "hcl" {
			t.Fatalf("%s classified as %s", path, got)
		}
	}
	if got := service.LanguageFor("main.tf.json"); got != "text" {
		t.Fatalf("tf.json should not use the HCL grammar: %s", got)
	}
}

func TestHCLSyntaxOutlineAndNavigation(t *testing.T) {
	service := NewParser()
	document, err := service.Parse("hcl", terraformExample)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if document.tree.RootNode().HasError() {
		t.Fatal("valid Terraform configuration parsed with errors")
	}
	outline := service.Outline(document, "main.tf")
	for _, name := range []string{"variable.ami", "resource.aws_instance.web", "output.instance_id"} {
		mustFind(t, outline.Symbols, "block", name)
	}
	resource := mustFind(t, outline.Symbols, "block", "resource.aws_instance.web")
	mustFind(t, resource.Children, "attribute", "ami")
	mustFind(t, resource.Children, "block", "lifecycle")
	graph := service.NavigationGraph(document, "main.tf")
	if len(graph.Declarations) != 3 || len(graph.Imports) != 0 || len(graph.Calls) != 0 || len(graph.TypeDeclarations) != 0 {
		t.Fatalf("HCL navigation facts: %+v", graph)
	}
	for _, declaration := range graph.Declarations {
		if declaration.Kind != "block" || declaration.Entrypoint != "" {
			t.Fatalf("unexpected Terraform block declaration: %+v", declaration)
		}
	}
	if graph.Declarations[1].Name != "resource.aws_instance.web" {
		t.Fatalf("resource address: %+v", graph.Declarations)
	}
	segments, status := service.Segments(document, map[int]bool{7: true})
	if status != SegmentBuildStructured || len(segments) == 0 {
		t.Fatalf("HCL syntax segments: %s %+v", status, segments)
	}
	grammar := service.GetGrammar("hcl")
	if !grammar.NodeKind("block") || grammar.FieldCardinality("object_elem", "key") != GrammarCardinalityOne {
		t.Fatal("missing pinned HCL grammar metadata")
	}
}

func TestHCLOnlyNavigatesLiteralTopLevelTerraformBlocks(t *testing.T) {
	graph := BuildNavigationGraph(`resource "aws_instance" "${var.name}" { ami = "test" }
custom "other" { value = 1 }
resource "aws_instance" "plain" {
  tags = { Name = upper("web") }
  provisioner "local-exec" { command = "echo hello" }
}
`, "hcl", "main.tf")
	if len(graph.Declarations) != 1 || graph.Declarations[0].Name != "resource.aws_instance.plain" {
		t.Fatalf("unsupported/dynamic blocks should not become declarations: %+v", graph.Declarations)
	}
	if len(graph.Calls) != 1 || graph.Calls[0].Name != "upper" || graph.Calls[0].TargetID != "" {
		t.Fatalf("HCL function calls should remain unresolved without Terraform functions: %+v", graph.Calls)
	}
}
