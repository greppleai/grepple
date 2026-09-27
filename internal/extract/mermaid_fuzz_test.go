package extract

import "testing"

func FuzzParseClassDiagram(f *testing.F) {
	f.Add("classDiagram\r\n class Store {\r\n  +load(Item): Item\r\n }\r\n")
	f.Add("classDiagram\n class Broken {\n +run(\n")
	f.Add("%% grepple:unknown value\n")
	f.Fuzz(func(t *testing.T, diagram string) {
		if len(diagram) > 64*1024 {
			t.Skip()
		}
		_, _ = ParseClassDiagram(diagram)
	})
}

func FuzzParseFlowchart(f *testing.F) {
	f.Add("flowchart TD\r\n start[Start]\r\n finish[Finish]\r\n start --> finish\r\n")
	f.Add("flowchart TD\n start[\n start -->\n")
	f.Add("%% grepple:truncated max-nodes 20\n")
	f.Fuzz(func(t *testing.T, diagram string) {
		if len(diagram) > 64*1024 {
			t.Skip()
		}
		_, _ = ParseFlowchart(diagram)
	})
}
