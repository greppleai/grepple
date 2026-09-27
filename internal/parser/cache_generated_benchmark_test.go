package parser

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
)

type generatedProtobufBenchmarkFiles struct {
	graphMessagesPath, graphPackedPath, cstMessagesPath, cstPackedPath string
	graphMessagesSize, graphPackedSize, cstMessagesSize, cstPackedSize int
}

var (
	benchmarkGeneratedGraph       *BenchmarkPBNavigationGraph
	benchmarkGeneratedPackedGraph *BenchmarkPBPackedNavigationGraph
	benchmarkGeneratedCST         *BenchmarkPBCST
	benchmarkGeneratedPackedCST   *BenchmarkPBPackedCST
)

func TestGeneratedNavigationProtobufDecodersAgree(t *testing.T) {
	graph := BuildNavigationGraph("package sample\nfunc Run() { helper() }\nfunc helper() {}\n", "go", "sample.go")
	expected := generatedNavigationGraph(graph)
	content, err := proto.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	standard := &BenchmarkPBNavigationGraph{}
	if err := proto.Unmarshal(content, standard); err != nil {
		t.Fatal(err)
	}
	generated := &BenchmarkPBNavigationGraph{}
	if err := generated.UnmarshalVT(content); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(expected, standard) || !proto.Equal(expected, generated) {
		t.Fatalf("generated protobuf decoders differ: standard=%+v vt=%+v", standard, generated)
	}
}

func benchmarkGeneratedProtobufFormats(b *testing.B, graph NavigationGraph, cst benchmarkCST) {
	files := writeGeneratedProtobufBenchmarkFiles(b, graph, cst)
	b.Run("navigation-protobuf-generated-messages-disk", func(b *testing.B) {
		benchmarkGeneratedGraphMessagesDisk(b, files.graphMessagesPath, files.graphMessagesSize, false)
	})
	b.Run("navigation-vtprotobuf-generated-messages-disk", func(b *testing.B) {
		benchmarkGeneratedGraphMessagesDisk(b, files.graphMessagesPath, files.graphMessagesSize, true)
	})
	b.Run("navigation-protobuf-generated-packed-disk", func(b *testing.B) {
		benchmarkGeneratedGraphPackedDisk(b, files.graphPackedPath, files.graphPackedSize, false)
	})
	b.Run("navigation-vtprotobuf-generated-packed-disk", func(b *testing.B) {
		benchmarkGeneratedGraphPackedDisk(b, files.graphPackedPath, files.graphPackedSize, true)
	})
	b.Run("full-cst-protobuf-generated-messages-disk", func(b *testing.B) {
		benchmarkGeneratedCSTMessagesDisk(b, files.cstMessagesPath, files.cstMessagesSize, false)
	})
	b.Run("full-cst-vtprotobuf-generated-messages-disk", func(b *testing.B) {
		benchmarkGeneratedCSTMessagesDisk(b, files.cstMessagesPath, files.cstMessagesSize, true)
	})
	b.Run("full-cst-protobuf-generated-packed-disk", func(b *testing.B) {
		benchmarkGeneratedCSTPackedDisk(b, files.cstPackedPath, files.cstPackedSize, false)
	})
	b.Run("full-cst-vtprotobuf-generated-packed-disk", func(b *testing.B) {
		benchmarkGeneratedCSTPackedDisk(b, files.cstPackedPath, files.cstPackedSize, true)
	})
}

func writeGeneratedProtobufBenchmarkFiles(b *testing.B, graph NavigationGraph, cst benchmarkCST) generatedProtobufBenchmarkFiles {
	b.Helper()
	directory := b.TempDir()
	graphMessages := generatedNavigationGraph(graph)
	graphPacked := generatedPackedNavigationGraph(graph)
	cstMessages := generatedNavigationCST(cst)
	cstPacked := generatedPackedNavigationCST(cst)
	return generatedProtobufBenchmarkFiles{
		graphMessagesPath: writeGeneratedProtobufBenchmarkFile(b, directory, "generated-navigation-messages.pb", graphMessages),
		graphPackedPath:   writeGeneratedProtobufBenchmarkFile(b, directory, "generated-navigation-packed.pb", graphPacked),
		cstMessagesPath:   writeGeneratedProtobufBenchmarkFile(b, directory, "generated-cst-messages.pb", cstMessages),
		cstPackedPath:     writeGeneratedProtobufBenchmarkFile(b, directory, "generated-cst-packed.pb", cstPacked),
		graphMessagesSize: proto.Size(graphMessages), graphPackedSize: proto.Size(graphPacked),
		cstMessagesSize: proto.Size(cstMessages), cstPackedSize: proto.Size(cstPacked),
	}
}

func writeGeneratedProtobufBenchmarkFile(b *testing.B, directory, name string, message proto.Message) string {
	b.Helper()
	content, err := proto.Marshal(message)
	if err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path
}

func generatedNavigationGraph(graph NavigationGraph) *BenchmarkPBNavigationGraph {
	result := &BenchmarkPBNavigationGraph{
		Declarations: make([]*BenchmarkPBNavigationDeclaration, 0, len(graph.Declarations)),
		Calls:        make([]*BenchmarkPBNavigationCall, 0, len(graph.Calls)),
	}
	for _, declaration := range graph.Declarations {
		result.Declarations = append(result.Declarations, &BenchmarkPBNavigationDeclaration{
			Id: declaration.ID, Name: declaration.Name, Kind: declaration.Kind, Language: declaration.Language,
			Path: declaration.Path, Container: declaration.Container, Receiver: declaration.Receiver,
			ResultType: declaration.ResultType, ResultImportPath: declaration.ResultImportPath,
			Package: declaration.Package, PackageId: declaration.PackageID, ModuleId: declaration.ModuleID,
			Scope: declaration.Scope, Start: uint32(declaration.Start), End: uint32(declaration.End),
		})
	}
	for _, call := range graph.Calls {
		result.Calls = append(result.Calls, &BenchmarkPBNavigationCall{
			Id: call.ID, CallerId: call.CallerID, TargetId: call.TargetID, Name: call.Name, Display: call.Display,
			Qualifier: call.Qualifier, ImportPath: call.ImportPath, ReceiverType: call.ReceiverType,
			ReceiverFactory: call.ReceiverFactory, ReceiverFactoryImport: call.ReceiverFactoryImport,
			ResolvedName: call.ResolvedName, Confidence: call.Confidence, Language: call.Language, Path: call.Path,
			Line: uint32(call.Line), EnclosingStart: uint32(call.EnclosingStart), EnclosingEnd: uint32(call.EnclosingEnd),
		})
	}
	return result
}

func generatedPackedNavigationGraph(graph NavigationGraph) *BenchmarkPBPackedNavigationGraph {
	table := benchmarkProtobufStringTable{values: []string{""}, ids: map[string]uint32{"": 0}}
	result := &BenchmarkPBPackedNavigationGraph{}
	for _, declaration := range graph.Declarations {
		for _, value := range navigationDeclarationStrings(declaration) {
			result.DeclarationStrings = append(result.DeclarationStrings, table.intern(value))
		}
		result.DeclarationRanges = append(result.DeclarationRanges, uint32(declaration.Start), uint32(declaration.End))
	}
	for _, call := range graph.Calls {
		for _, value := range navigationCallStrings(call) {
			result.CallStrings = append(result.CallStrings, table.intern(value))
		}
		result.CallRanges = append(result.CallRanges, uint32(call.Line), uint32(call.EnclosingStart), uint32(call.EnclosingEnd))
	}
	result.Strings = table.values
	return result
}

func generatedNavigationCST(cst benchmarkCST) *BenchmarkPBCST {
	result := &BenchmarkPBCST{Nodes: make([]*BenchmarkPBCSTNode, 0, len(cst.nodes))}
	for _, node := range cst.nodes {
		result.Nodes = append(result.Nodes, &BenchmarkPBCSTNode{
			Kind: node.kind, Field: node.field, File: node.file, Parent: node.parent,
			ChildIndex: node.childIndex, ChildCount: node.childCount, Flags: node.flags,
			StartByte: node.startByte, EndByte: node.endByte, StartRow: node.startRow,
			StartColumn: node.startColumn, EndRow: node.endRow, EndColumn: node.endColumn,
		})
	}
	return result
}

func generatedPackedNavigationCST(cst benchmarkCST) *BenchmarkPBPackedCST {
	table := benchmarkProtobufStringTable{values: []string{""}, ids: map[string]uint32{"": 0}}
	result := &BenchmarkPBPackedCST{}
	for _, node := range cst.nodes {
		result.NodeStrings = append(result.NodeStrings, table.intern(node.kind), table.intern(node.field))
		values := benchmarkCSTNodeValues(node)
		result.NodeValues = append(result.NodeValues, values[:]...)
	}
	result.Strings = table.values
	return result
}

func benchmarkGeneratedGraphMessagesDisk(b *testing.B, path string, size int, vt bool) {
	b.Helper()
	reportGeneratedProtobufBenchmark(b, size)
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		message := &BenchmarkPBNavigationGraph{}
		if vt {
			err = message.UnmarshalVT(content)
		} else {
			err = proto.Unmarshal(content, message)
		}
		if err != nil {
			b.Fatal(err)
		}
		benchmarkGeneratedGraph = message
	}
}

func benchmarkGeneratedGraphPackedDisk(b *testing.B, path string, size int, vt bool) {
	b.Helper()
	reportGeneratedProtobufBenchmark(b, size)
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		message := &BenchmarkPBPackedNavigationGraph{}
		if vt {
			err = message.UnmarshalVT(content)
		} else {
			err = proto.Unmarshal(content, message)
		}
		if err != nil {
			b.Fatal(err)
		}
		benchmarkGeneratedPackedGraph = message
	}
}

func benchmarkGeneratedCSTMessagesDisk(b *testing.B, path string, size int, vt bool) {
	b.Helper()
	reportGeneratedProtobufBenchmark(b, size)
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		message := &BenchmarkPBCST{}
		if vt {
			err = message.UnmarshalVT(content)
		} else {
			err = proto.Unmarshal(content, message)
		}
		if err != nil {
			b.Fatal(err)
		}
		benchmarkGeneratedCST = message
	}
}

func benchmarkGeneratedCSTPackedDisk(b *testing.B, path string, size int, vt bool) {
	b.Helper()
	reportGeneratedProtobufBenchmark(b, size)
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		message := &BenchmarkPBPackedCST{}
		if vt {
			err = message.UnmarshalVT(content)
		} else {
			err = proto.Unmarshal(content, message)
		}
		if err != nil {
			b.Fatal(err)
		}
		benchmarkGeneratedPackedCST = message
	}
}

func reportGeneratedProtobufBenchmark(b *testing.B, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
}
