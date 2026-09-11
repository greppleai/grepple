package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	protobufDeclarationStringWidth = 13
	protobufCallStringWidth        = 14
)

type benchmarkPackedNavigationGraph struct {
	strings            []string
	declarationStrings []uint32
	declarationRanges  []uint32
	callStrings        []uint32
	callRanges         []uint32
}

type benchmarkProtobufStringTable struct {
	values []string
	ids    map[string]uint32
}

var benchmarkPackedGraph benchmarkPackedNavigationGraph

func TestNavigationBenchmarkProtobufRoundTrips(t *testing.T) {
	graph := BuildNavigationGraph(`package sample
type Client struct{}
func NewClient() *Client { return &Client{} }
func (*Client) Load() {}
func Run() { client := NewClient(); client.Load() }
`, "go", "sample.go")
	decoded, err := unmarshalNavigationProtobufMessages(marshalNavigationProtobufMessages(graph))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, graph) {
		t.Fatalf("message-oriented protobuf round trip differs: got %+v want %+v", decoded, graph)
	}
	packed, err := unmarshalNavigationProtobufPacked(marshalNavigationProtobufPacked(graph))
	if err != nil {
		t.Fatal(err)
	}
	if len(packed.strings) == 0 || packed.strings[0] != "" || len(packed.declarationStrings) != len(graph.Declarations)*protobufDeclarationStringWidth || len(packed.declarationRanges) != len(graph.Declarations)*2 {
		t.Fatalf("packed declaration columns have unexpected lengths: %+v", packed)
	}
	if len(packed.callStrings) != len(graph.Calls)*protobufCallStringWidth || len(packed.callRanges) != len(graph.Calls)*3 {
		t.Fatalf("packed call columns have unexpected lengths: %+v", packed)
	}
}

func writeNavigationBenchmarkProtobufMessages(b *testing.B, graph NavigationGraph) (string, int) {
	b.Helper()
	content := marshalNavigationProtobufMessages(graph)
	path := filepath.Join(b.TempDir(), "navigation-messages.pb")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path, len(content)
}

func writeNavigationBenchmarkProtobufPacked(b *testing.B, graph NavigationGraph) (string, int) {
	b.Helper()
	content := marshalNavigationProtobufPacked(graph)
	path := filepath.Join(b.TempDir(), "navigation-packed.pb")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		b.Fatal(err)
	}
	return path, len(content)
}

func benchmarkNavigationProtobufMessagesDisk(b *testing.B, path string, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		graph, err := unmarshalNavigationProtobufMessages(content)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkGraph = graph
	}
}

func benchmarkNavigationProtobufPackedDisk(b *testing.B, path string, size int) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ReportMetric(float64(size), "cache-bytes")
	for range b.N {
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		graph, err := unmarshalNavigationProtobufPacked(content)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPackedGraph = graph
	}
}

func marshalNavigationProtobufMessages(graph NavigationGraph) []byte {
	content := make([]byte, 0)
	for _, declaration := range graph.Declarations {
		message := marshalNavigationDeclarationMessage(declaration)
		content = protowire.AppendTag(content, 1, protowire.BytesType)
		content = protowire.AppendBytes(content, message)
	}
	for _, call := range graph.Calls {
		message := marshalNavigationCallMessage(call)
		content = protowire.AppendTag(content, 2, protowire.BytesType)
		content = protowire.AppendBytes(content, message)
	}
	return content
}

func marshalNavigationDeclarationMessage(declaration NavigationDeclaration) []byte {
	content := make([]byte, 0)
	for index, value := range navigationDeclarationStrings(declaration) {
		content = appendProtobufString(content, protowire.Number(index+1), value)
	}
	content = appendProtobufUint32(content, 14, uint32(declaration.Start))
	return appendProtobufUint32(content, 15, uint32(declaration.End))
}

func marshalNavigationCallMessage(call NavigationCall) []byte {
	content := make([]byte, 0)
	for index, value := range navigationCallStrings(call) {
		content = appendProtobufString(content, protowire.Number(index+1), value)
	}
	content = appendProtobufUint32(content, 15, uint32(call.Line))
	content = appendProtobufUint32(content, 16, uint32(call.EnclosingStart))
	return appendProtobufUint32(content, 17, uint32(call.EnclosingEnd))
}

func appendProtobufString(content []byte, field protowire.Number, value string) []byte {
	if value == "" {
		return content
	}
	content = protowire.AppendTag(content, field, protowire.BytesType)
	return protowire.AppendString(content, value)
}

func appendRequiredProtobufString(content []byte, field protowire.Number, value string) []byte {
	content = protowire.AppendTag(content, field, protowire.BytesType)
	return protowire.AppendString(content, value)
}

func appendProtobufUint32(content []byte, field protowire.Number, value uint32) []byte {
	if value == 0 {
		return content
	}
	content = protowire.AppendTag(content, field, protowire.VarintType)
	return protowire.AppendVarint(content, uint64(value))
}

func unmarshalNavigationProtobufMessages(content []byte) (NavigationGraph, error) {
	graph := NavigationGraph{}
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return NavigationGraph{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		message, valueLength := protowire.ConsumeBytes(content)
		if wireType != protowire.BytesType || valueLength < 0 {
			return NavigationGraph{}, fmt.Errorf("invalid navigation protobuf field %d", field)
		}
		content = content[valueLength:]
		switch field {
		case 1:
			declaration, err := unmarshalNavigationDeclarationMessage(message)
			if err != nil {
				return NavigationGraph{}, err
			}
			graph.Declarations = append(graph.Declarations, declaration)
		case 2:
			call, err := unmarshalNavigationCallMessage(message)
			if err != nil {
				return NavigationGraph{}, err
			}
			graph.Calls = append(graph.Calls, call)
		}
	}
	return graph, nil
}

func unmarshalNavigationDeclarationMessage(content []byte) (NavigationDeclaration, error) {
	var stringsByField [protobufDeclarationStringWidth]string
	var start, end uint32
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return NavigationDeclaration{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		if field <= protobufDeclarationStringWidth {
			value, valueLength := protowire.ConsumeString(content)
			if wireType != protowire.BytesType || valueLength < 0 {
				return NavigationDeclaration{}, fmt.Errorf("invalid declaration string field %d", field)
			}
			stringsByField[field-1] = value
			content = content[valueLength:]
			continue
		}
		value, valueLength := protowire.ConsumeVarint(content)
		if wireType != protowire.VarintType || valueLength < 0 {
			return NavigationDeclaration{}, fmt.Errorf("invalid declaration integer field %d", field)
		}
		if field == 14 {
			start = uint32(value)
		} else if field == 15 {
			end = uint32(value)
		}
		content = content[valueLength:]
	}
	return navigationDeclarationFromProtobuf(stringsByField, start, end), nil
}

func unmarshalNavigationCallMessage(content []byte) (NavigationCall, error) {
	var stringsByField [protobufCallStringWidth]string
	var line, enclosingStart, enclosingEnd uint32
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return NavigationCall{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		if field <= protobufCallStringWidth {
			value, valueLength := protowire.ConsumeString(content)
			if wireType != protowire.BytesType || valueLength < 0 {
				return NavigationCall{}, fmt.Errorf("invalid call string field %d", field)
			}
			stringsByField[field-1] = value
			content = content[valueLength:]
			continue
		}
		value, valueLength := protowire.ConsumeVarint(content)
		if wireType != protowire.VarintType || valueLength < 0 {
			return NavigationCall{}, fmt.Errorf("invalid call integer field %d", field)
		}
		switch field {
		case 15:
			line = uint32(value)
		case 16:
			enclosingStart = uint32(value)
		case 17:
			enclosingEnd = uint32(value)
		}
		content = content[valueLength:]
	}
	return navigationCallFromProtobuf(stringsByField, line, enclosingStart, enclosingEnd), nil
}

func navigationDeclarationStrings(declaration NavigationDeclaration) [protobufDeclarationStringWidth]string {
	return [protobufDeclarationStringWidth]string{
		declaration.ID, declaration.Name, declaration.Kind, declaration.Language, declaration.Path,
		declaration.Container, declaration.Receiver, declaration.ResultType, declaration.ResultImportPath,
		declaration.Package, declaration.PackageID, declaration.ModuleID, declaration.Scope,
	}
}

func navigationCallStrings(call NavigationCall) [protobufCallStringWidth]string {
	return [protobufCallStringWidth]string{
		call.ID, call.CallerID, call.TargetID, call.Name, call.Display, call.Qualifier, call.ImportPath,
		call.ReceiverType, call.ReceiverFactory, call.ReceiverFactoryImport, call.ResolvedName,
		call.Confidence, call.Language, call.Path,
	}
}

func navigationDeclarationFromProtobuf(values [protobufDeclarationStringWidth]string, start, end uint32) NavigationDeclaration {
	return NavigationDeclaration{
		ID: values[0], Name: values[1], Kind: values[2], Language: values[3], Path: values[4],
		Container: values[5], Receiver: values[6], ResultType: values[7], ResultImportPath: values[8],
		Package: values[9], PackageID: values[10], ModuleID: values[11], Scope: values[12],
		Start: int(start), End: int(end),
	}
}

func navigationCallFromProtobuf(values [protobufCallStringWidth]string, line, enclosingStart, enclosingEnd uint32) NavigationCall {
	return NavigationCall{
		ID: values[0], CallerID: values[1], TargetID: values[2], Name: values[3], Display: values[4],
		Qualifier: values[5], ImportPath: values[6], ReceiverType: values[7], ReceiverFactory: values[8],
		ReceiverFactoryImport: values[9], ResolvedName: values[10], Confidence: values[11],
		Language: values[12], Path: values[13], Line: int(line), EnclosingStart: int(enclosingStart), EnclosingEnd: int(enclosingEnd),
	}
}

func marshalNavigationProtobufPacked(graph NavigationGraph) []byte {
	table := benchmarkProtobufStringTable{values: []string{""}, ids: map[string]uint32{"": 0}}
	packed := benchmarkPackedNavigationGraph{}
	for _, declaration := range graph.Declarations {
		for _, value := range navigationDeclarationStrings(declaration) {
			packed.declarationStrings = append(packed.declarationStrings, table.intern(value))
		}
		packed.declarationRanges = append(packed.declarationRanges, uint32(declaration.Start), uint32(declaration.End))
	}
	for _, call := range graph.Calls {
		for _, value := range navigationCallStrings(call) {
			packed.callStrings = append(packed.callStrings, table.intern(value))
		}
		packed.callRanges = append(packed.callRanges, uint32(call.Line), uint32(call.EnclosingStart), uint32(call.EnclosingEnd))
	}
	packed.strings = table.values
	return marshalPackedNavigationGraph(packed)
}

func (table *benchmarkProtobufStringTable) intern(value string) uint32 {
	if id, ok := table.ids[value]; ok {
		return id
	}
	id := uint32(len(table.values))
	table.ids[value] = id
	table.values = append(table.values, value)
	return id
}

func marshalPackedNavigationGraph(graph benchmarkPackedNavigationGraph) []byte {
	content := make([]byte, 0)
	for _, value := range graph.strings {
		content = appendRequiredProtobufString(content, 1, value)
	}
	content = appendProtobufPackedUint32(content, 2, graph.declarationStrings)
	content = appendProtobufPackedUint32(content, 3, graph.declarationRanges)
	content = appendProtobufPackedUint32(content, 4, graph.callStrings)
	return appendProtobufPackedUint32(content, 5, graph.callRanges)
}

func appendProtobufPackedUint32(content []byte, field protowire.Number, values []uint32) []byte {
	if len(values) == 0 {
		return content
	}
	packed := make([]byte, 0, len(values))
	for _, value := range values {
		packed = protowire.AppendVarint(packed, uint64(value))
	}
	content = protowire.AppendTag(content, field, protowire.BytesType)
	return protowire.AppendBytes(content, packed)
}

func unmarshalNavigationProtobufPacked(content []byte) (benchmarkPackedNavigationGraph, error) {
	graph := benchmarkPackedNavigationGraph{}
	for len(content) > 0 {
		field, wireType, tagLength := protowire.ConsumeTag(content)
		if tagLength < 0 {
			return benchmarkPackedNavigationGraph{}, protowire.ParseError(tagLength)
		}
		content = content[tagLength:]
		value, valueLength := protowire.ConsumeBytes(content)
		if wireType != protowire.BytesType || valueLength < 0 {
			return benchmarkPackedNavigationGraph{}, fmt.Errorf("invalid packed protobuf field %d", field)
		}
		content = content[valueLength:]
		switch field {
		case 1:
			graph.strings = append(graph.strings, string(value))
		case 2:
			graph.declarationStrings = appendPackedProtobufValues(graph.declarationStrings, value)
		case 3:
			graph.declarationRanges = appendPackedProtobufValues(graph.declarationRanges, value)
		case 4:
			graph.callStrings = appendPackedProtobufValues(graph.callStrings, value)
		case 5:
			graph.callRanges = appendPackedProtobufValues(graph.callRanges, value)
		}
	}
	if len(graph.declarationStrings)%protobufDeclarationStringWidth != 0 || len(graph.declarationRanges)%2 != 0 || len(graph.callStrings)%protobufCallStringWidth != 0 || len(graph.callRanges)%3 != 0 {
		return benchmarkPackedNavigationGraph{}, fmt.Errorf("invalid packed navigation column lengths")
	}
	return graph, nil
}

func appendPackedProtobufValues(destination []uint32, content []byte) []uint32 {
	for len(content) > 0 {
		value, valueLength := protowire.ConsumeVarint(content)
		if valueLength < 0 {
			return destination
		}
		destination = append(destination, uint32(value))
		content = content[valueLength:]
	}
	return destination
}
