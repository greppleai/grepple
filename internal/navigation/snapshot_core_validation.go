package navigation

import "fmt"

// Detail row bounds are checked separately once the generation's table counts
// are attached. This verifies the relationships required for safe core traversal.
func validateSnapshotCore(snapshot *querySnapshot) error {
	nodes := uint64(len(snapshot.names))
	calls := uint64(len(snapshot.calls))
	if len(snapshot.groups) != len(snapshot.calls) {
		return fmt.Errorf("snapshot duplicate-group count mismatch")
	}
	if err := validateSnapshotCallReferences(snapshot, nodes, calls); err != nil {
		return err
	}
	for _, csr := range []snapshotCSR{snapshot.outgoing, snapshot.incoming} {
		if err := validateSnapshotCSR(csr, nodes, calls); err != nil {
			return err
		}
	}
	if err := validateSnapshotOwnerReferences(snapshot, nodes); err != nil {
		return err
	}
	return validateSnapshotDuplicateRows(snapshot.duplicates, calls)
}

func validateSnapshotCSR(csr snapshotCSR, nodes, calls uint64) error {
	if uint64(len(csr.offsets)) != nodes+1 || csr.offsets[0] != 0 {
		return fmt.Errorf("snapshot adjacency offset count mismatch")
	}
	previous := uint32(0)
	for _, offset := range csr.offsets {
		if offset < previous || uint64(offset) > uint64(len(csr.edges)) {
			return fmt.Errorf("snapshot adjacency offset outside core")
		}
		previous = offset
	}
	if uint64(previous) != uint64(len(csr.edges)) {
		return fmt.Errorf("snapshot adjacency final offset mismatch")
	}
	for _, edge := range csr.edges {
		if uint64(edge) >= calls {
			return fmt.Errorf("snapshot adjacency edge outside core")
		}
	}
	return nil
}

func validateSnapshotDuplicateRows(rows map[uint32][]uint32, calls uint64) error {
	for key, values := range rows {
		if uint64(key) >= calls {
			return fmt.Errorf("snapshot duplicate key outside core")
		}
		for _, value := range values {
			if uint64(value) >= calls {
				return fmt.Errorf("snapshot duplicate row outside core")
			}
		}
	}
	return nil
}

func validateSnapshotCallReferences(snapshot *querySnapshot, nodes, calls uint64) error {
	for _, call := range snapshot.calls {
		if uint64(call.caller) >= nodes || uint64(call.targets)+uint64(call.count) > uint64(len(snapshot.targets)) {
			return fmt.Errorf("snapshot call reference outside core")
		}
		if call.direct && call.count != 1 {
			return fmt.Errorf("snapshot direct target count mismatch")
		}
	}
	for _, target := range snapshot.targets {
		if uint64(target) >= nodes {
			return fmt.Errorf("snapshot target outside core")
		}
	}
	for _, group := range snapshot.groups {
		if uint64(group) >= calls {
			return fmt.Errorf("snapshot call group outside core")
		}
	}
	return nil
}

func validateSnapshotOwnerReferences(snapshot *querySnapshot, nodes uint64) error {
	for _, rows := range []map[uint32][]uint32{snapshot.declarations, snapshot.usages, snapshot.accesses} {
		for node := range rows {
			if uint64(node) >= nodes {
				return fmt.Errorf("snapshot owner outside core")
			}
		}
	}
	return nil
}
