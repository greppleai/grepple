package navigation

import (
	"fmt"
	"github.com/greppleai/grepple/internal/parser"
)

// Detail blocks reuse native checksummed fact serialization. Each selected batch
// decodes a touched block once, not a repository graph. No source strings live here.
const snapshotDetailBlockRows = 16

// A generation-owned source returns only the requested encoded detail block.
// Its owner must pin the underlying immutable generation through hydration.
type snapshotDetailBlockSource interface {
	ReadBlock(uint32) ([]byte, error)
}

type snapshotDetailTable[T any] struct {
	source     snapshotDetailBlockSource
	blocks     [][]byte
	count      uint32
	nonNil     bool
	selectRows func(parser.NavigationGraph) []T
}

func newSnapshotDetailTable[T any](records []T, assign func(*parser.NavigationGraph, []T), selectRows func(parser.NavigationGraph) []T, encoder *snapshotBlockEncoder) (snapshotDetailTable[T], error) {
	if uint64(len(records)) >= uint64(^uint32(0)) {
		return snapshotDetailTable[T]{}, fmt.Errorf("navigation detail count exceeds uint32 capacity")
	}
	table := snapshotDetailTable[T]{count: uint32(len(records)), nonNil: records != nil, selectRows: selectRows, blocks: make([][]byte, 0, (len(records)+snapshotDetailBlockRows-1)/snapshotDetailBlockRows)}
	for start := 0; start < len(records); start += snapshotDetailBlockRows {
		end := min(start+snapshotDetailBlockRows, len(records))
		graph := parser.NavigationGraph{}
		assign(&graph, records[start:end])
		data, err := parser.MarshalNavigationFactArtifact(parser.NavigationFactArtifact{Digest: "compact-navigation-detail-v1", Graph: graph})
		if err != nil {
			return table, err
		}
		data, err = encoder.encode(data)
		if err != nil {
			return table, err
		}
		table.blocks = append(table.blocks, data)
	}
	return table, nil
}
func (table snapshotDetailTable[T]) encodedBlock(block uint32) ([]byte, error) {
	if table.source != nil {
		return table.source.ReadBlock(block)
	}
	if uint64(block) >= uint64(len(table.blocks)) {
		return nil, fmt.Errorf("navigation detail block outside table")
	}
	return table.blocks[block], nil
}

func (table snapshotDetailTable[T]) readBlock(block uint32) ([]T, error) {
	encoded, err := table.encodedBlock(block)
	if err != nil {
		return nil, err
	}
	data, err := decodeSnapshotBlock(encoded)
	if err != nil {
		return nil, err
	}
	fact, err := parser.UnmarshalNavigationFactArtifact(data)
	if err != nil {
		return nil, fmt.Errorf("navigation detail block %d: %w", block, err)
	}
	if fact.Digest != "compact-navigation-detail-v1" {
		return nil, fmt.Errorf("navigation detail identity mismatch")
	}
	rows := table.selectRows(fact.Graph)
	expected := min(uint32(snapshotDetailBlockRows), table.count-block*snapshotDetailBlockRows)
	if uint32(len(rows)) != expected {
		return nil, fmt.Errorf("navigation detail block %d count mismatch", block)
	}
	return rows, nil
}

func (table snapshotDetailTable[T]) read(rows []uint32) ([]T, error) {
	result := make([]T, 0, len(rows))
	var loaded []T
	var previous uint32
	haveBlock := false
	for _, row := range rows {
		if row >= table.count {
			return nil, fmt.Errorf("navigation detail row %d outside %d records", row, table.count)
		}
		block := row / snapshotDetailBlockRows
		if !haveBlock || block != previous {
			var err error
			loaded, err = table.readBlock(block)
			if err != nil {
				return nil, err
			}
			previous = block
			haveBlock = true
		}
		index := row % snapshotDetailBlockRows
		if index >= uint32(len(loaded)) {
			return nil, fmt.Errorf("navigation detail block %d is incomplete", block)
		}
		result = append(result, loaded[index])
	}
	return result, nil
}

type snapshotDetails struct {
	declarations snapshotDetailTable[parser.NavigationDeclaration]
	calls        snapshotDetailTable[parser.NavigationCall]
	types        snapshotDetailTable[parser.NavigationTypeDeclaration]
	imports      snapshotDetailTable[parser.NavigationImport]
	exports      snapshotDetailTable[parser.NavigationExport]
	fields       snapshotDetailTable[parser.NavigationField]
	usages       snapshotDetailTable[parser.NavigationTypeUsage]
	accesses     snapshotDetailTable[parser.NavigationMemberAccess]
}

func newSnapshotDetails(graph parser.NavigationGraph) (*snapshotDetails, error) {
	encoder, err := newSnapshotBlockEncoder()
	if err != nil {
		return nil, err
	}
	defer encoder.writer.Close()
	details := &snapshotDetails{}
	details.declarations, err = newSnapshotDetailTable(graph.Declarations, func(g *parser.NavigationGraph, records []parser.NavigationDeclaration) { g.Declarations = records }, func(g parser.NavigationGraph) []parser.NavigationDeclaration { return g.Declarations }, encoder)
	if err != nil {
		return nil, err
	}
	details.calls, err = newSnapshotDetailTable(graph.Calls, assignSnapshotCallDetails, func(g parser.NavigationGraph) []parser.NavigationCall { return g.Calls }, encoder)
	if err != nil {
		return nil, err
	}
	details.types, err = newSnapshotDetailTable(graph.TypeDeclarations, func(g *parser.NavigationGraph, records []parser.NavigationTypeDeclaration) {
		g.TypeDeclarations = records
	}, func(g parser.NavigationGraph) []parser.NavigationTypeDeclaration { return g.TypeDeclarations }, encoder)
	if err != nil {
		return nil, err
	}
	details.imports, err = newSnapshotDetailTable(graph.Imports, func(g *parser.NavigationGraph, records []parser.NavigationImport) { g.Imports = records }, func(g parser.NavigationGraph) []parser.NavigationImport { return g.Imports }, encoder)
	if err != nil {
		return nil, err
	}
	details.exports, err = newSnapshotDetailTable(graph.Exports, func(g *parser.NavigationGraph, records []parser.NavigationExport) { g.Exports = records }, func(g parser.NavigationGraph) []parser.NavigationExport { return g.Exports }, encoder)
	if err != nil {
		return nil, err
	}
	details.fields, err = newSnapshotDetailTable(graph.Fields, func(g *parser.NavigationGraph, records []parser.NavigationField) { g.Fields = records }, func(g parser.NavigationGraph) []parser.NavigationField { return g.Fields }, encoder)
	if err != nil {
		return nil, err
	}
	details.usages, err = newSnapshotDetailTable(graph.TypeUsages, func(g *parser.NavigationGraph, records []parser.NavigationTypeUsage) { g.TypeUsages = records }, func(g parser.NavigationGraph) []parser.NavigationTypeUsage { return g.TypeUsages }, encoder)
	if err != nil {
		return nil, err
	}
	details.accesses, err = newSnapshotDetailTable(graph.MemberAccesses, func(g *parser.NavigationGraph, records []parser.NavigationMemberAccess) { g.MemberAccesses = records }, func(g parser.NavigationGraph) []parser.NavigationMemberAccess { return g.MemberAccesses }, encoder)
	if err != nil {
		return nil, err
	}
	return details, nil
}

func (table snapshotDetailTable[T]) readAll() ([]T, error) {
	if !table.nonNil {
		return nil, nil
	}
	return table.read(allSnapshotRows(table.count))
}
