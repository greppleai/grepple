package parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"google.golang.org/protobuf/encoding/protowire"
)

// UnmarshalNavigationDeclarationArtifact verifies the native envelope but
// projects only callable/type declarations. Resolution does not need the much
// larger calls/imports/usages graph. No representation is retained by this API.
func UnmarshalNavigationDeclarationArtifact(content []byte) (NavigationFactArtifact, error) {
	envelope := navigationCacheEntryDecoder{}
	for len(content) > 0 {
		var err error
		content, err = envelope.consume(content)
		if err != nil {
			return NavigationFactArtifact{}, err
		}
	}
	checksum := sha256.Sum256(envelope.graph)
	if envelope.entry.Schema != navigationCacheSchema || len(envelope.graph) == 0 || !bytes.Equal(checksum[:], envelope.checksum) {
		return NavigationFactArtifact{}, fmt.Errorf("invalid navigation artifact envelope")
	}
	decoder := navigationProtoGraphDecoder{}
	content = envelope.graph
	for len(content) > 0 {
		field, wire, n := protowire.ConsumeTag(content)
		if n < 0 {
			return NavigationFactArtifact{}, protowire.ParseError(n)
		}
		if (field >= 2 && field <= 5) || field == 22 {
			var err error
			content, err = decoder.consume(content)
			if err != nil {
				return NavigationFactArtifact{}, err
			}
			continue
		}
		size := protowire.ConsumeFieldValue(field, wire, content[n:])
		if size < 0 {
			return NavigationFactArtifact{}, protowire.ParseError(size)
		}
		content = content[n+size:]
	}
	if err := loadDeclarationStrings(envelope.graph, &decoder.graph); err != nil {
		return NavigationFactArtifact{}, err
	}
	graph, err := navigationGraphFromProtoColumns(decoder.graph)
	if err != nil {
		return NavigationFactArtifact{}, err
	}
	return NavigationFactArtifact{Digest: envelope.entry.Digest, Recovered: envelope.entry.Recovered, Graph: graph}, nil
}

// The two passes avoid allocating/copying every string used only by calls and
// usages. Packed declaration columns identify precisely which strings are needed.
func loadDeclarationStrings(content []byte, graph *navigationProtoGraph) error {
	var count uint32
	if err := visitDeclarationStrings(content, func(_ []byte) { count++ }); err != nil {
		return err
	}
	if count > navigationProtoMaximumColumnValues {
		return fmt.Errorf("navigation string table too large")
	}
	needed := make([]bool, count)
	for _, column := range [][]uint32{graph.declarationStrings, graph.typeDeclarationStrings} {
		for _, id := range column {
			if id >= count {
				return fmt.Errorf("navigation string index out of range")
			}
			needed[id] = true
		}
	}
	graph.strings = make([]string, count)
	var id uint32
	return visitDeclarationStrings(content, func(value []byte) {
		if needed[id] || id == 0 {
			graph.strings[id] = string(value)
		}
		id++
	})
}
func visitDeclarationStrings(content []byte, visit func([]byte)) error {
	for len(content) > 0 {
		field, wire, n := protowire.ConsumeTag(content)
		if n < 0 {
			return protowire.ParseError(n)
		}
		content = content[n:]
		if field == 1 {
			value, size := protowire.ConsumeBytes(content)
			if wire != protowire.BytesType || size < 0 {
				return fmt.Errorf("invalid navigation string table")
			}
			visit(value)
			content = content[size:]
		} else {
			size := protowire.ConsumeFieldValue(field, wire, content)
			if size < 0 {
				return protowire.ParseError(size)
			}
			content = content[size:]
		}
	}
	return nil
}
