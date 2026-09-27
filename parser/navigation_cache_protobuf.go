package parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"reflect"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	navigationProtoDeclarationStrings     = 17
	navigationProtoDeclarationValues      = 2
	navigationProtoTypeDeclarationStrings = 8
	navigationProtoTypeDeclarationValues  = 2
	navigationProtoImportStrings          = 9
	navigationProtoImportValues           = 3
	navigationProtoCallStrings            = 16
	navigationProtoCallValues             = 5
	navigationProtoExportStrings          = 8
	navigationProtoExportValues           = 1
	navigationProtoFieldStrings           = 8
	navigationProtoFieldValues            = 2
	navigationProtoTypeUsageStrings       = 6
	navigationProtoTypeUsageValues        = 1
	navigationProtoMemberAccessStrings    = 8
	navigationProtoMemberAccessValues     = 2
	navigationProtoMaximumColumnValues    = 16 << 20
)

type navigationProtoGraph struct {
	strings                []string
	declarationStrings     []uint32
	declarationValues      []uint32
	typeDeclarationStrings []uint32
	typeDeclarationValues  []uint32
	importStrings          []uint32
	importValues           []uint32
	importTargetPaths      []uint32
	callStrings            []uint32
	callValues             []uint32
	callCandidateTargetIDs []uint32
	callReceiverMembers    []uint32
	exportStrings          []uint32
	exportValues           []uint32
	fieldStrings           []uint32
	fieldValues            []uint32
	typeUsageStrings       []uint32
	typeUsageValues        []uint32
	memberAccessStrings    []uint32
	memberAccessValues     []uint32
	repositoryRoots        []uint32
	presentSlices          uint32
}

type navigationProtoStringTable struct {
	values []string
	ids    map[string]uint32
}

func newNavigationProtoStringTable() *navigationProtoStringTable {
	return &navigationProtoStringTable{values: []string{""}, ids: map[string]uint32{"": 0}}
}

func (table *navigationProtoStringTable) intern(value string) uint32 {
	if id, ok := table.ids[value]; ok {
		return id
	}
	id := uint32(len(table.values))
	table.ids[value] = id
	table.values = append(table.values, value)
	return id
}

func marshalNavigationCacheEntry(entry navigationCacheEntry) ([]byte, error) {
	graph, err := marshalNavigationProtoGraph(entry.Graph)
	if err != nil {
		return nil, err
	}
	checksum := sha256.Sum256(graph)
	content := make([]byte, 0, len(graph)+len(entry.Schema)+len(entry.Digest)+48)
	content = appendNavigationProtoBytes(content, 1, []byte(entry.Schema))
	content = appendNavigationProtoBytes(content, 2, []byte(entry.Digest))
	if entry.Recovered {
		content = protowire.AppendTag(content, 3, protowire.VarintType)
		content = protowire.AppendVarint(content, 1)
	}
	content = appendNavigationProtoBytes(content, 4, graph)
	content = appendNavigationProtoBytes(content, 5, checksum[:])
	return content, nil
}

func unmarshalNavigationCacheEntry(content []byte) (navigationCacheEntry, error) {
	decoder := navigationCacheEntryDecoder{}
	for len(content) > 0 {
		var err error
		content, err = decoder.consume(content)
		if err != nil {
			return navigationCacheEntry{}, err
		}
	}
	checksum := sha256.Sum256(decoder.graph)
	if len(decoder.graph) == 0 || !bytes.Equal(decoder.checksum, checksum[:]) {
		return navigationCacheEntry{}, fmt.Errorf("navigation cache graph checksum mismatch")
	}
	graph, err := unmarshalNavigationProtoGraph(decoder.graph)
	if err != nil {
		return navigationCacheEntry{}, err
	}
	decoder.entry.Graph = graph
	return decoder.entry, nil
}

type navigationCacheEntryDecoder struct {
	entry    navigationCacheEntry
	graph    []byte
	checksum []byte
}

func (decoder *navigationCacheEntryDecoder) consume(content []byte) ([]byte, error) {
	field, wireType, tagLength := protowire.ConsumeTag(content)
	if tagLength < 0 {
		return nil, protowire.ParseError(tagLength)
	}
	content = content[tagLength:]
	if field == 3 {
		value, valueLength := protowire.ConsumeVarint(content)
		if wireType != protowire.VarintType || valueLength < 0 || value > 1 {
			return nil, fmt.Errorf("invalid navigation cache recovered field")
		}
		decoder.entry.Recovered = value == 1
		return content[valueLength:], nil
	}
	if field < 1 || field > 5 {
		valueLength := protowire.ConsumeFieldValue(field, wireType, content)
		if valueLength < 0 {
			return nil, protowire.ParseError(valueLength)
		}
		return content[valueLength:], nil
	}
	value, valueLength := protowire.ConsumeBytes(content)
	if wireType != protowire.BytesType || valueLength < 0 {
		return nil, fmt.Errorf("invalid navigation cache field %d", field)
	}
	switch field {
	case 1:
		decoder.entry.Schema = string(value)
	case 2:
		decoder.entry.Digest = string(value)
	case 4:
		decoder.graph = value
	case 5:
		decoder.checksum = value
	}
	return content[valueLength:], nil
}

func marshalNavigationProtoGraph(graph NavigationGraph) ([]byte, error) {
	if err := validateNavigationProtoIntegerRanges(reflect.ValueOf(graph)); err != nil {
		return nil, err
	}
	table := newNavigationProtoStringTable()
	packed := navigationProtoGraph{presentSlices: navigationProtoPresentSlices(graph)}
	for _, declaration := range graph.Declarations {
		packed.declarationStrings = appendNavigationProtoStringIDs(packed.declarationStrings, table,
			declaration.ID, declaration.Name, declaration.Kind, declaration.Language, declaration.Path,
			declaration.Container, declaration.Receiver, declaration.ResultType, declaration.ResultImportPath,
			declaration.Package, declaration.PackageID, declaration.ModuleID, declaration.Scope,
			declaration.Entrypoint, string(declaration.Visibility), declaration.VisibilityDetail, declaration.Signature)
		packed.declarationValues = appendNavigationProtoInts(packed.declarationValues, declaration.Start, declaration.End)
	}
	for _, declaration := range graph.TypeDeclarations {
		packed.typeDeclarationStrings = appendNavigationProtoStringIDs(packed.typeDeclarationStrings, table,
			declaration.Name, declaration.Kind, declaration.Language, declaration.Path, declaration.Container,
			declaration.Package, declaration.PackageID, declaration.ModuleID)
		packed.typeDeclarationValues = appendNavigationProtoInts(packed.typeDeclarationValues, declaration.Start, declaration.End)
	}
	for _, imported := range graph.Imports {
		packed.importStrings = appendNavigationProtoStringIDs(packed.importStrings, table,
			imported.Alias, imported.ImportPath, imported.Imported, imported.Kind, imported.Scope,
			imported.VisibilityDetail, imported.TargetPathHint, imported.Language, imported.Path)
		packed.importValues = appendNavigationProtoInts(packed.importValues, imported.Line, len(imported.TargetPaths))
		packed.importValues = append(packed.importValues, navigationProtoBool(imported.Inline))
		packed.importTargetPaths = appendNavigationProtoStringIDs(packed.importTargetPaths, table, imported.TargetPaths...)
	}
	for _, call := range graph.Calls {
		packed.callStrings = appendNavigationProtoStringIDs(packed.callStrings, table,
			call.ID, call.CallerID, call.TargetID, call.Name, call.Display, call.Qualifier, call.ImportPath,
			call.ReceiverType, call.ReceiverRootType, call.ReceiverRootImport, call.ReceiverFactory,
			call.ReceiverFactoryImport, call.ResolvedName, call.Confidence, call.Language, call.Path)
		packed.callValues = appendNavigationProtoInts(packed.callValues, call.Line, call.EnclosingStart, call.EnclosingEnd,
			len(call.CandidateTargetIDs), len(call.ReceiverMembers))
		packed.callCandidateTargetIDs = appendNavigationProtoStringIDs(packed.callCandidateTargetIDs, table, call.CandidateTargetIDs...)
		packed.callReceiverMembers = appendNavigationProtoStringIDs(packed.callReceiverMembers, table, call.ReceiverMembers...)
	}
	for _, exported := range graph.Exports {
		packed.exportStrings = appendNavigationProtoStringIDs(packed.exportStrings, table,
			exported.Name, exported.LocalName, exported.ImportPath, exported.ImportedName, exported.Scope,
			exported.VisibilityDetail, exported.Language, exported.Path)
		packed.exportValues = appendNavigationProtoInts(packed.exportValues, exported.Line)
	}
	for _, field := range graph.Fields {
		packed.fieldStrings = appendNavigationProtoStringIDs(packed.fieldStrings, table,
			field.OwnerType, field.Name, field.Type, field.ImportPath, field.Language, field.Path,
			field.Package, string(field.Visibility))
		packed.fieldValues = appendNavigationProtoInts(packed.fieldValues, field.Line)
		packed.fieldValues = append(packed.fieldValues, navigationProtoBool(field.Embedded))
	}
	for _, usage := range graph.TypeUsages {
		packed.typeUsageStrings = appendNavigationProtoStringIDs(packed.typeUsageStrings, table,
			usage.CallerID, usage.Type, usage.ImportPath, usage.Role, usage.Language, usage.Path)
		packed.typeUsageValues = appendNavigationProtoInts(packed.typeUsageValues, usage.Line)
	}
	for _, access := range graph.MemberAccesses {
		packed.memberAccessStrings = appendNavigationProtoStringIDs(packed.memberAccessStrings, table,
			access.ID, access.CallerID, access.ReceiverType, access.Receiver, access.Member,
			access.Operation, access.Language, access.Path)
		packed.memberAccessValues = appendNavigationProtoInts(packed.memberAccessValues, access.Line, access.StartByte)
	}
	packed.repositoryRoots = appendNavigationProtoStringIDs(packed.repositoryRoots, table, graph.RepositoryRoots...)
	packed.strings = table.values
	return marshalNavigationProtoColumns(packed), nil
}

func appendNavigationProtoInts(destination []uint32, values ...int) []uint32 {
	for _, value := range values {
		destination = append(destination, uint32(value))
	}
	return destination
}

func navigationProtoBool(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}

func appendNavigationProtoStringIDs(destination []uint32, table *navigationProtoStringTable, values ...string) []uint32 {
	for _, value := range values {
		destination = append(destination, table.intern(value))
	}
	return destination
}

func validateNavigationProtoIntegerRanges(value reflect.Value) error {
	switch value.Kind() {
	case reflect.Int:
		integer := value.Int()
		if integer < 0 || uint64(integer) > math.MaxUint32 {
			return fmt.Errorf("navigation cache integer outside uint32: %d", integer)
		}
	case reflect.Struct:
		return validateNavigationProtoValueRange(value.NumField(), value.Field)
	case reflect.Slice:
		if uint64(value.Len()) > math.MaxUint32 {
			return fmt.Errorf("navigation cache slice exceeds uint32 length")
		}
		return validateNavigationProtoValueRange(value.Len(), value.Index)
	}
	return nil
}

func validateNavigationProtoValueRange(length int, valueAt func(int) reflect.Value) error {
	for index := 0; index < length; index++ {
		if err := validateNavigationProtoIntegerRanges(valueAt(index)); err != nil {
			return err
		}
	}
	return nil
}

func marshalNavigationProtoColumns(graph navigationProtoGraph) []byte {
	content := make([]byte, 0)
	for _, value := range graph.strings {
		content = appendNavigationProtoBytes(content, 1, []byte(value))
	}
	columns := []struct {
		field protowire.Number
		value []uint32
	}{
		{2, graph.declarationStrings}, {3, graph.declarationValues},
		{4, graph.typeDeclarationStrings}, {5, graph.typeDeclarationValues},
		{6, graph.importStrings}, {7, graph.importValues}, {8, graph.importTargetPaths},
		{9, graph.callStrings}, {10, graph.callValues}, {11, graph.callCandidateTargetIDs}, {12, graph.callReceiverMembers},
		{13, graph.exportStrings}, {14, graph.exportValues}, {15, graph.fieldStrings}, {16, graph.fieldValues},
		{17, graph.typeUsageStrings}, {18, graph.typeUsageValues}, {19, graph.memberAccessStrings},
		{20, graph.memberAccessValues}, {21, graph.repositoryRoots},
	}
	for _, column := range columns {
		content = appendNavigationProtoPacked(content, column.field, column.value)
	}
	if graph.presentSlices != 0 {
		content = protowire.AppendTag(content, 22, protowire.VarintType)
		content = protowire.AppendVarint(content, uint64(graph.presentSlices))
	}
	return content
}

func appendNavigationProtoBytes(content []byte, field protowire.Number, value []byte) []byte {
	content = protowire.AppendTag(content, field, protowire.BytesType)
	return protowire.AppendBytes(content, value)
}

func appendNavigationProtoPacked(content []byte, field protowire.Number, values []uint32) []byte {
	if len(values) == 0 {
		return content
	}
	packedSize := 0
	for _, value := range values {
		packedSize += protowire.SizeVarint(uint64(value))
	}
	content = protowire.AppendTag(content, field, protowire.BytesType)
	content = protowire.AppendVarint(content, uint64(packedSize))
	for _, value := range values {
		content = protowire.AppendVarint(content, uint64(value))
	}
	return content
}

func unmarshalNavigationProtoGraph(content []byte) (NavigationGraph, error) {
	decoder := navigationProtoGraphDecoder{}
	for len(content) > 0 {
		var err error
		content, err = decoder.consume(content)
		if err != nil {
			return NavigationGraph{}, err
		}
	}
	return navigationGraphFromProtoColumns(decoder.graph)
}

type navigationProtoGraphDecoder struct{ graph navigationProtoGraph }

func (decoder *navigationProtoGraphDecoder) consume(content []byte) ([]byte, error) {
	field, wireType, tagLength := protowire.ConsumeTag(content)
	if tagLength < 0 {
		return nil, protowire.ParseError(tagLength)
	}
	content = content[tagLength:]
	if field == 22 {
		value, valueLength := protowire.ConsumeVarint(content)
		if wireType != protowire.VarintType || valueLength < 0 || value > math.MaxUint32 {
			return nil, fmt.Errorf("invalid navigation slice-presence field")
		}
		decoder.graph.presentSlices = uint32(value)
		return content[valueLength:], nil
	}
	if field < 1 || field > 21 {
		valueLength := protowire.ConsumeFieldValue(field, wireType, content)
		if valueLength < 0 {
			return nil, protowire.ParseError(valueLength)
		}
		return content[valueLength:], nil
	}
	value, valueLength := protowire.ConsumeBytes(content)
	if wireType != protowire.BytesType || valueLength < 0 {
		return nil, fmt.Errorf("invalid packed navigation field %d", field)
	}
	if field == 1 {
		decoder.graph.strings = append(decoder.graph.strings, string(value))
		return content[valueLength:], nil
	}
	column := navigationProtoColumn(&decoder.graph, field)
	var err error
	*column, err = appendNavigationProtoPackedValues(*column, value)
	if err != nil {
		return nil, fmt.Errorf("navigation protobuf field %d: %w", field, err)
	}
	return content[valueLength:], nil
}

func navigationProtoColumn(graph *navigationProtoGraph, field protowire.Number) *[]uint32 {
	switch field {
	case 2:
		return &graph.declarationStrings
	case 3:
		return &graph.declarationValues
	case 4:
		return &graph.typeDeclarationStrings
	case 5:
		return &graph.typeDeclarationValues
	case 6:
		return &graph.importStrings
	case 7:
		return &graph.importValues
	case 8:
		return &graph.importTargetPaths
	case 9:
		return &graph.callStrings
	case 10:
		return &graph.callValues
	case 11:
		return &graph.callCandidateTargetIDs
	case 12:
		return &graph.callReceiverMembers
	case 13:
		return &graph.exportStrings
	case 14:
		return &graph.exportValues
	case 15:
		return &graph.fieldStrings
	case 16:
		return &graph.fieldValues
	case 17:
		return &graph.typeUsageStrings
	case 18:
		return &graph.typeUsageValues
	case 19:
		return &graph.memberAccessStrings
	case 20:
		return &graph.memberAccessValues
	case 21:
		return &graph.repositoryRoots
	default:
		return nil
	}
}

func appendNavigationProtoPackedValues(destination []uint32, content []byte) ([]uint32, error) {
	for len(content) > 0 {
		if len(destination) >= navigationProtoMaximumColumnValues {
			return nil, fmt.Errorf("column exceeds value limit")
		}
		value, valueLength := protowire.ConsumeVarint(content)
		if valueLength < 0 || value > math.MaxUint32 {
			return nil, fmt.Errorf("invalid uint32 column value")
		}
		destination = append(destination, uint32(value))
		content = content[valueLength:]
	}
	return destination, nil
}

func navigationGraphFromProtoColumns(packed navigationProtoGraph) (NavigationGraph, error) {
	if len(packed.strings) == 0 || packed.strings[0] != "" {
		return NavigationGraph{}, fmt.Errorf("navigation protobuf has invalid string table")
	}
	if err := validateNavigationProtoColumns(packed); err != nil {
		return NavigationGraph{}, err
	}
	decoder := navigationProtoDecoder{packed: packed}
	graph := decoder.newGraph()
	decoders := []func(*NavigationGraph) error{
		decoder.declarations, decoder.typeDeclarations, decoder.imports, decoder.calls,
		decoder.exports, decoder.fields, decoder.typeUsages, decoder.memberAccesses,
	}
	for _, decode := range decoders {
		if err := decode(&graph); err != nil {
			return NavigationGraph{}, err
		}
	}
	var err error
	graph.RepositoryRoots, err = decoder.stringList(packed.repositoryRoots, 0, len(packed.repositoryRoots))
	if err != nil {
		return NavigationGraph{}, err
	}
	applyNavigationProtoSlicePresence(&graph, packed.presentSlices)
	return graph, nil
}

type navigationProtoDecoder struct{ packed navigationProtoGraph }

func (decoder navigationProtoDecoder) newGraph() NavigationGraph {
	packed := decoder.packed
	return NavigationGraph{
		Declarations:     make([]NavigationDeclaration, len(packed.declarationStrings)/navigationProtoDeclarationStrings),
		TypeDeclarations: make([]NavigationTypeDeclaration, len(packed.typeDeclarationStrings)/navigationProtoTypeDeclarationStrings),
		Imports:          make([]NavigationImport, len(packed.importStrings)/navigationProtoImportStrings),
		Calls:            make([]NavigationCall, len(packed.callStrings)/navigationProtoCallStrings),
		Exports:          make([]NavigationExport, len(packed.exportStrings)/navigationProtoExportStrings),
		Fields:           make([]NavigationField, len(packed.fieldStrings)/navigationProtoFieldStrings),
		TypeUsages:       make([]NavigationTypeUsage, len(packed.typeUsageStrings)/navigationProtoTypeUsageStrings),
		MemberAccesses:   make([]NavigationMemberAccess, len(packed.memberAccessStrings)/navigationProtoMemberAccessStrings),
	}
}

func (decoder navigationProtoDecoder) stringList(values []uint32, offset, width int) ([]string, error) {
	if width == 0 {
		return nil, nil
	}
	result := make([]string, width)
	for index, id := range values[offset : offset+width] {
		if uint64(id) >= uint64(len(decoder.packed.strings)) {
			return nil, fmt.Errorf("navigation protobuf string index %d out of range", id)
		}
		result[index] = decoder.packed.strings[id]
	}
	return result, nil
}

func (decoder navigationProtoDecoder) declarations(graph *NavigationGraph) error {
	for index := range graph.Declarations {
		s, err := decoder.stringList(decoder.packed.declarationStrings, index*navigationProtoDeclarationStrings, navigationProtoDeclarationStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.declarationValues[index*navigationProtoDeclarationValues:]
		graph.Declarations[index] = NavigationDeclaration{ID: s[0], Name: s[1], Kind: s[2], Language: s[3], Path: s[4], Container: s[5], Receiver: s[6], ResultType: s[7], ResultImportPath: s[8], Package: s[9], PackageID: s[10], ModuleID: s[11], Scope: s[12], Entrypoint: s[13], Visibility: NavigationVisibility(s[14]), VisibilityDetail: s[15], Signature: s[16], Start: int(v[0]), End: int(v[1])}
	}
	return nil
}

func (decoder navigationProtoDecoder) typeDeclarations(graph *NavigationGraph) error {
	for index := range graph.TypeDeclarations {
		s, err := decoder.stringList(decoder.packed.typeDeclarationStrings, index*navigationProtoTypeDeclarationStrings, navigationProtoTypeDeclarationStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.typeDeclarationValues[index*navigationProtoTypeDeclarationValues:]
		graph.TypeDeclarations[index] = NavigationTypeDeclaration{Name: s[0], Kind: s[1], Language: s[2], Path: s[3], Container: s[4], Package: s[5], PackageID: s[6], ModuleID: s[7], Start: int(v[0]), End: int(v[1])}
	}
	return nil
}

func (decoder navigationProtoDecoder) imports(graph *NavigationGraph) error {
	targetOffset := 0
	for index := range graph.Imports {
		s, err := decoder.stringList(decoder.packed.importStrings, index*navigationProtoImportStrings, navigationProtoImportStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.importValues[index*navigationProtoImportValues:]
		targets, err := decoder.stringList(decoder.packed.importTargetPaths, targetOffset, int(v[1]))
		if err != nil {
			return err
		}
		targetOffset += int(v[1])
		graph.Imports[index] = NavigationImport{Alias: s[0], ImportPath: s[1], Imported: s[2], Kind: s[3], Scope: s[4], VisibilityDetail: s[5], TargetPathHint: s[6], Language: s[7], Path: s[8], Line: int(v[0]), Inline: v[2] == 1, TargetPaths: targets}
	}
	return nil
}

func (decoder navigationProtoDecoder) calls(graph *NavigationGraph) error {
	candidateOffset, memberOffset := 0, 0
	for index := range graph.Calls {
		s, err := decoder.stringList(decoder.packed.callStrings, index*navigationProtoCallStrings, navigationProtoCallStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.callValues[index*navigationProtoCallValues:]
		candidates, err := decoder.stringList(decoder.packed.callCandidateTargetIDs, candidateOffset, int(v[3]))
		if err != nil {
			return err
		}
		members, err := decoder.stringList(decoder.packed.callReceiverMembers, memberOffset, int(v[4]))
		if err != nil {
			return err
		}
		candidateOffset, memberOffset = candidateOffset+int(v[3]), memberOffset+int(v[4])
		graph.Calls[index] = NavigationCall{ID: s[0], CallerID: s[1], TargetID: s[2], Name: s[3], Display: s[4], Qualifier: s[5], ImportPath: s[6], ReceiverType: s[7], ReceiverRootType: s[8], ReceiverRootImport: s[9], ReceiverFactory: s[10], ReceiverFactoryImport: s[11], ResolvedName: s[12], Confidence: s[13], Language: s[14], Path: s[15], Line: int(v[0]), EnclosingStart: int(v[1]), EnclosingEnd: int(v[2]), CandidateTargetIDs: candidates, ReceiverMembers: members}
	}
	return nil
}

func (decoder navigationProtoDecoder) exports(graph *NavigationGraph) error {
	for index := range graph.Exports {
		s, err := decoder.stringList(decoder.packed.exportStrings, index*navigationProtoExportStrings, navigationProtoExportStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.exportValues[index*navigationProtoExportValues:]
		graph.Exports[index] = NavigationExport{Name: s[0], LocalName: s[1], ImportPath: s[2], ImportedName: s[3], Scope: s[4], VisibilityDetail: s[5], Language: s[6], Path: s[7], Line: int(v[0])}
	}
	return nil
}

func (decoder navigationProtoDecoder) fields(graph *NavigationGraph) error {
	for index := range graph.Fields {
		s, err := decoder.stringList(decoder.packed.fieldStrings, index*navigationProtoFieldStrings, navigationProtoFieldStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.fieldValues[index*navigationProtoFieldValues:]
		graph.Fields[index] = NavigationField{OwnerType: s[0], Name: s[1], Type: s[2], ImportPath: s[3], Language: s[4], Path: s[5], Package: s[6], Visibility: NavigationVisibility(s[7]), Line: int(v[0]), Embedded: v[1] == 1}
	}
	return nil
}

func (decoder navigationProtoDecoder) typeUsages(graph *NavigationGraph) error {
	for index := range graph.TypeUsages {
		s, err := decoder.stringList(decoder.packed.typeUsageStrings, index*navigationProtoTypeUsageStrings, navigationProtoTypeUsageStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.typeUsageValues[index*navigationProtoTypeUsageValues:]
		graph.TypeUsages[index] = NavigationTypeUsage{CallerID: s[0], Type: s[1], ImportPath: s[2], Role: s[3], Language: s[4], Path: s[5], Line: int(v[0])}
	}
	return nil
}

func (decoder navigationProtoDecoder) memberAccesses(graph *NavigationGraph) error {
	for index := range graph.MemberAccesses {
		s, err := decoder.stringList(decoder.packed.memberAccessStrings, index*navigationProtoMemberAccessStrings, navigationProtoMemberAccessStrings)
		if err != nil {
			return err
		}
		v := decoder.packed.memberAccessValues[index*navigationProtoMemberAccessValues:]
		graph.MemberAccesses[index] = NavigationMemberAccess{ID: s[0], CallerID: s[1], ReceiverType: s[2], Receiver: s[3], Member: s[4], Operation: s[5], Language: s[6], Path: s[7], Line: int(v[0]), StartByte: int(v[1])}
	}
	return nil
}

func validateNavigationProtoColumns(graph navigationProtoGraph) error {
	columns := []struct {
		name                    string
		strings, values         []uint32
		stringWidth, valueWidth int
	}{
		{"declarations", graph.declarationStrings, graph.declarationValues, navigationProtoDeclarationStrings, navigationProtoDeclarationValues},
		{"type declarations", graph.typeDeclarationStrings, graph.typeDeclarationValues, navigationProtoTypeDeclarationStrings, navigationProtoTypeDeclarationValues},
		{"imports", graph.importStrings, graph.importValues, navigationProtoImportStrings, navigationProtoImportValues},
		{"calls", graph.callStrings, graph.callValues, navigationProtoCallStrings, navigationProtoCallValues},
		{"exports", graph.exportStrings, graph.exportValues, navigationProtoExportStrings, navigationProtoExportValues},
		{"fields", graph.fieldStrings, graph.fieldValues, navigationProtoFieldStrings, navigationProtoFieldValues},
		{"type usages", graph.typeUsageStrings, graph.typeUsageValues, navigationProtoTypeUsageStrings, navigationProtoTypeUsageValues},
		{"member accesses", graph.memberAccessStrings, graph.memberAccessValues, navigationProtoMemberAccessStrings, navigationProtoMemberAccessValues},
	}
	for _, column := range columns {
		if len(column.strings)%column.stringWidth != 0 || len(column.values)%column.valueWidth != 0 || len(column.strings)/column.stringWidth != len(column.values)/column.valueWidth {
			return fmt.Errorf("navigation protobuf has invalid %s column widths", column.name)
		}
	}
	importTargets := uint64(0)
	for index := 1; index < len(graph.importValues); index += navigationProtoImportValues {
		importTargets += uint64(graph.importValues[index])
	}
	if importTargets != uint64(len(graph.importTargetPaths)) {
		return fmt.Errorf("navigation protobuf has invalid import target count")
	}
	candidates, members := uint64(0), uint64(0)
	for index := 3; index < len(graph.callValues); index += navigationProtoCallValues {
		candidates += uint64(graph.callValues[index])
		members += uint64(graph.callValues[index+1])
	}
	if candidates != uint64(len(graph.callCandidateTargetIDs)) || members != uint64(len(graph.callReceiverMembers)) {
		return fmt.Errorf("navigation protobuf has invalid call list counts")
	}
	for index := 2; index < len(graph.importValues); index += navigationProtoImportValues {
		if graph.importValues[index] > 1 {
			return fmt.Errorf("navigation protobuf has invalid import boolean")
		}
	}
	for index := 1; index < len(graph.fieldValues); index += navigationProtoFieldValues {
		if graph.fieldValues[index] > 1 {
			return fmt.Errorf("navigation protobuf has invalid field boolean")
		}
	}
	return nil
}

func navigationProtoPresentSlices(graph NavigationGraph) uint32 {
	var present uint32
	slices := []bool{
		graph.Declarations != nil, graph.TypeDeclarations != nil, graph.Imports != nil,
		graph.Calls != nil, graph.Exports != nil, graph.Fields != nil,
		graph.TypeUsages != nil, graph.MemberAccesses != nil, graph.RepositoryRoots != nil,
	}
	for index, exists := range slices {
		if exists {
			present |= 1 << index
		}
	}
	return present
}

func applyNavigationProtoSlicePresence(graph *NavigationGraph, present uint32) {
	if present&(1<<0) == 0 {
		graph.Declarations = nil
	}
	if present&(1<<1) == 0 {
		graph.TypeDeclarations = nil
	}
	if present&(1<<2) == 0 {
		graph.Imports = nil
	}
	if present&(1<<3) == 0 {
		graph.Calls = nil
	}
	if present&(1<<4) == 0 {
		graph.Exports = nil
	}
	if present&(1<<5) == 0 {
		graph.Fields = nil
	}
	if present&(1<<6) == 0 {
		graph.TypeUsages = nil
	}
	if present&(1<<7) == 0 {
		graph.MemberAccesses = nil
	}
	if present&(1<<8) == 0 {
		graph.RepositoryRoots = nil
	} else if graph.RepositoryRoots == nil {
		graph.RepositoryRoots = []string{}
	}
}
