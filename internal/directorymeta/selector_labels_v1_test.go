package directorymeta

import (
	"reflect"
	"testing"
)

func TestSelectedLabelsV1RestrictsAncestorUnionsAndBoundsTrust(t *testing.T) {
	inventory := InventoryV1{Files: map[string]FileStateV1{
		"a/one.go":        {Status: StatusCurrent, File: FileV1{Purpose: "Implementation", DomainAreas: []string{"Billing"}, Keywords: []string{"c++"}}},
		"a/deep/two.go":   {Status: StatusCurrent, File: FileV1{Purpose: "Testing", DomainAreas: []string{"Payments"}, FunctionalAreas: []string{"API"}}},
		"a/deep/stale.go": {Status: StatusStale, File: FileV1{Purpose: "Fake", DomainAreas: []string{"Poison"}}},
	}}
	match, err := CompileSelectorsV1(SelectorsV1{Purposes: []string{"TESTING"}, DomainAreas: []string{"payments"}})
	if err != nil {
		t.Fatal(err)
	}
	union := SelectedLabelsV1(inventory, match.MatchingFiles(inventory))
	for _, dir := range []string{".", "a", "a/deep"} {
		label := union[dir]
		if !reflect.DeepEqual(label.Purposes, []string{"Testing"}) || !reflect.DeepEqual(label.DomainAreas, []string{"Payments"}) || !reflect.DeepEqual(label.FunctionalAreas, []string{"API"}) || len(label.Keywords) > 0 {
			t.Fatalf("dir %q union=%+v", dir, label)
		}
	}
	if len(union) != 3 {
		t.Fatalf("unexpected ancestor labels: %+v", union)
	}
	selected := SelectedLabelsV1(inventory, map[string]bool{"a/deep/stale.go": true, "a/absent.go": true})
	if len(selected) != 0 || len(FileLabelsV1(inventory.Files["a/deep/stale.go"]).Purposes) != 0 {
		t.Fatalf("untrusted labels=%+v", selected)
	}
}
