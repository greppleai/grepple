package navigation

import "strings"

// Do not keep substrings backed by complete protobuf string columns alive.
func cloneSnapshotRowKeys(source map[string][]uint32) map[string][]uint32 {
	cloned := make(map[string][]uint32, len(source))
	for key, value := range source {
		cloned[strings.Clone(key)] = value
	}
	return cloned
}
