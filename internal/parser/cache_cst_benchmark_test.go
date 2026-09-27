package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

const benchmarkCSTValueWidth = 11

type benchmarkCST struct {
	nodes []benchmarkCSTNode
}

type benchmarkCSTNode struct {
	kind, field                                 string
	file, parent, childIndex, childCount, flags uint32
	startByte, endByte                          uint32
	startRow, startColumn, endRow, endColumn    uint32
}

type benchmarkPackedCST struct {
	strings     []string
	nodeStrings []uint32
	nodeValues  []uint32
}

var (
	benchmarkLoadedCST       benchmarkCST
	benchmarkLoadedPackedCST benchmarkPackedCST
)

func TestNavigationBenchmarkCSTProtobufRoundTrips(t *testing.T) {
	tree, err := adapterForLanguage("go").Parse("package sample\nfunc Run() { println(\"ok\") }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	original := benchmarkCST{}
	appendNavigationBenchmarkCSTNode(&original, tree.RootNode(), 0, 0, 0, "")
	decoded, err := unmarshalNavigationCSTMessages(marshalNavigationCSTMessages(original))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("message-oriented CST protobuf round trip differs: got %+v want %+v", decoded, original)
	}
	packed, err := unmarshalNavigationCSTPacked(marshalNavigationCSTPacked(original))
	if err != nil {
		t.Fatal(err)
	}
	if len(packed.strings) == 0 || packed.strings[0] != "" || len(packed.nodeStrings) != len(original.nodes)*2 || len(packed.nodeValues) != len(original.nodes)*benchmarkCSTValueWidth {
		t.Fatalf("packed CST columns have unexpected lengths: %+v", packed)
	}
}

func navigationBenchmarkCST(b *testing.B, sources []navigationBenchmarkSource) benchmarkCST {
	b.Helper()
	result := benchmarkCST{}
	for fileIndex, source := range sources {
		tree, err := adapterForLanguage("go").Parse(source.content)
		if err != nil {
			b.Fatal(err)
		}
		appendNavigationBenchmarkCSTNode(&result, tree.RootNode(), uint32(fileIndex), 0, 0, "")
		tree.Close()
	}
	return result
}

func benchmarkNavigationCSTProjection(b *testing.B, sources []navigationBenchmarkSource, totalSourceBytes int) {
	reportNavigationBenchmarkCorpus(b, sources, totalSourceBytes)
	for range b.N {
		loaded := benchmarkCST{}
		for fileIndex, source := range sources {
			tree, err := adapterForLanguage("go").Parse(source.content)
			if err != nil {
				b.Fatal(err)
			}
			appendNavigationBenchmarkCSTNode(&loaded, tree.RootNode(), uint32(fileIndex), 0, 0, "")
			tree.Close()
		}
		benchmarkLoadedCST = loaded
	}
}

func appendNavigationBenchmarkCSTNode(result *benchmarkCST, node *syntaxNode, file, parent, childIndex uint32, field string) {
	start, end := node.StartPosition(), node.EndPosition()
	index := uint32(len(result.nodes))
	result.nodes = append(result.nodes, benchmarkCSTNode{
		kind: node.Kind(), field: field, file: file, parent: parent, childIndex: childIndex,
		childCount: uint32(node.ChildCount()), flags: benchmarkCSTNodeFlags(node),
		startByte: uint32(node.StartByte()), endByte: uint32(node.EndByte()),
		startRow: uint32(start.Row), startColumn: uint32(start.Column), endRow: uint32(end.Row), endColumn: uint32(end.Column),
	})
	for child := uint(0); child < node.ChildCount(); child++ {
		appendNavigationBenchmarkCSTNode(result, node.Child(child), file, index+1, uint32(child), node.FieldNameForChild(uint32(child)))
	}
}

func benchmarkCSTNodeFlags(node *syntaxNode) uint32 {
	var flags uint32
	if node.IsNamed() {
		flags |= 1
	}
	if node.IsExtra() {
		flags |= 2
	}
	if node.IsMissing() {
		flags |= 4
	}
	if node.IsError() {
		flags |= 8
	}
	if node.HasError() {
		flags |= 16
	}
	return flags
}

func writeNavigationBenchmarkCSTMessages(b *testing.B, cst benchmarkCST) (string, int) {
	b.Helper()
	content := marshalNavigationCSTMessages(cst)
	path := filepath.Join(b.TempDir(), "full-cst-messages.pb")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path, len(content)
}

func writeNavigationBenchmarkCSTPacked(b *testing.B, cst benchmarkCST) (string, int) {
	b.Helper()
	content := marshalNavigationCSTPacked(cst)
	path := filepath.Join(b.TempDir(), "full-cst-packed.pb")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path, len(content)
}

func benchmarkNavigationCSTMessagesDisk(b *testing.B, path string, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		loaded, err := unmarshalNavigationCSTMessages(content)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkLoadedCST = loaded
	}
}

func benchmarkNavigationCSTPackedDisk(b *testing.B, path string, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		loaded, err := unmarshalNavigationCSTPacked(content)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkLoadedPackedCST = loaded
	}
}

func marshalNavigationCSTMessages(cst benchmarkCST) []byte {
	content := make([]byte, 0)
	for _, node := range cst.nodes {
		message := marshalNavigationCSTNode(node)
		content = protowire.AppendTag(content, 1, protowire.BytesType)
		content = protowire.AppendBytes(content, message)
	}
	return content
}

func marshalNavigationCSTNode(node benchmarkCSTNode) []byte {
	content := appendProtobufString(nil, 1, node.kind)
	content = appendProtobufString(content, 2, node.field)
	for index, value := range benchmarkCSTNodeValues(node) {
		content = appendProtobufUint32(content, protowire.Number(index+3), value)
	}
	return content
}

func unmarshalNavigationCSTMessages(content []byte) (benchmarkCST, error) {
	result := benchmarkCST{}
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return benchmarkCST{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		message, valueLength := protowire.ConsumeBytes(content)
		if field != 1 || wireType != protowire.BytesType || valueLength < 0 {
			return benchmarkCST{}, fmt.Errorf("invalid CST protobuf field %d", field)
		}
		content = content[valueLength:]
		node, err := unmarshalNavigationCSTNode(message)
		if err != nil {
			return benchmarkCST{}, err
		}
		result.nodes = append(result.nodes, node)
	}
	return result, nil
}

func unmarshalNavigationCSTNode(content []byte) (benchmarkCSTNode, error) {
	var kind, fieldName string
	var values [benchmarkCSTValueWidth]uint32
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return benchmarkCSTNode{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		if field <= 2 {
			value, valueLength := protowire.ConsumeString(content)
			if wireType != protowire.BytesType || valueLength < 0 {
				return benchmarkCSTNode{}, fmt.Errorf("invalid CST string field %d", field)
			}
			if field == 1 {
				kind = value
			} else {
				fieldName = value
			}
			content = content[valueLength:]
			continue
		}
		value, valueLength := protowire.ConsumeVarint(content)
		if wireType != protowire.VarintType || valueLength < 0 || field > benchmarkCSTValueWidth+2 {
			return benchmarkCSTNode{}, fmt.Errorf("invalid CST integer field %d", field)
		}
		values[field-3] = uint32(value)
		content = content[valueLength:]
	}
	return benchmarkCSTNodeFromValues(kind, fieldName, values), nil
}

func benchmarkCSTNodeValues(node benchmarkCSTNode) [benchmarkCSTValueWidth]uint32 {
	return [benchmarkCSTValueWidth]uint32{
		node.file, node.parent, node.childIndex, node.childCount, node.flags,
		node.startByte, node.endByte, node.startRow, node.startColumn, node.endRow, node.endColumn,
	}
}

func benchmarkCSTNodeFromValues(kind, field string, values [benchmarkCSTValueWidth]uint32) benchmarkCSTNode {
	return benchmarkCSTNode{
		kind: kind, field: field, file: values[0], parent: values[1], childIndex: values[2], childCount: values[3], flags: values[4],
		startByte: values[5], endByte: values[6], startRow: values[7], startColumn: values[8], endRow: values[9], endColumn: values[10],
	}
}

func marshalNavigationCSTPacked(cst benchmarkCST) []byte {
	table := benchmarkProtobufStringTable{values: []string{""}, ids: map[string]uint32{"": 0}}
	packed := benchmarkPackedCST{}
	for _, node := range cst.nodes {
		packed.nodeStrings = append(packed.nodeStrings, table.intern(node.kind), table.intern(node.field))
		values := benchmarkCSTNodeValues(node)
		packed.nodeValues = append(packed.nodeValues, values[:]...)
	}
	packed.strings = table.values
	return marshalNavigationPackedCST(packed)
}

func marshalNavigationPackedCST(cst benchmarkPackedCST) []byte {
	content := make([]byte, 0)
	for _, value := range cst.strings {
		content = appendRequiredProtobufString(content, 1, value)
	}
	content = appendProtobufPackedUint32(content, 2, cst.nodeStrings)
	return appendProtobufPackedUint32(content, 3, cst.nodeValues)
}

func unmarshalNavigationCSTPacked(content []byte) (benchmarkPackedCST, error) {
	result := benchmarkPackedCST{}
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return benchmarkPackedCST{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		value, valueLength := protowire.ConsumeBytes(content)
		if wireType != protowire.BytesType || valueLength < 0 {
			return benchmarkPackedCST{}, fmt.Errorf("invalid packed CST field %d", field)
		}
		content = content[valueLength:]
		switch field {
		case 1:
			result.strings = append(result.strings, string(value))
		case 2:
			result.nodeStrings = appendPackedProtobufValues(result.nodeStrings, value)
		case 3:
			result.nodeValues = appendPackedProtobufValues(result.nodeValues, value)
		}
	}
	if len(result.nodeStrings)%2 != 0 || len(result.nodeValues)%benchmarkCSTValueWidth != 0 || len(result.nodeStrings)/2 != len(result.nodeValues)/benchmarkCSTValueWidth {
		return benchmarkPackedCST{}, fmt.Errorf("invalid packed CST column lengths")
	}
	return result, nil
}
