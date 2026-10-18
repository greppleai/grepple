package directorymeta

import (
	"strings"
	"testing"
)

func TestSelectorsV1ORWithinAndSameFileAcrossCategories(t *testing.T) {
	inventory := InventoryV1{Files: map[string]FileStateV1{
		"backend/one.go":     {Status: StatusCurrent, File: FileV1{Purpose: "Implementation", DomainAreas: []string{"Payments"}, FunctionalAreas: []string{"API"}, Keywords: []string{"c++"}}},
		"backend/two.go":     {Status: StatusCurrent, File: FileV1{Purpose: "Testing", DomainAreas: []string{"Billing"}, FunctionalAreas: []string{"API"}, Keywords: []string{"Go"}}},
		"backend/stale.go":   {Status: StatusStale, File: FileV1{Purpose: "Implementation", DomainAreas: []string{"Billing"}, Keywords: []string{"c++"}}},
		"backend/missing.go": {Status: StatusMissing},
	}}
	for _, test := range []struct {
		name      string
		selection SelectorsV1
		want      string
	}{
		{"OR then AND", SelectorsV1{Purposes: []string{"testing", "implementation", "TESTING"}, DomainAreas: []string{"payments"}, Keywords: []string{"c++"}}, "backend/one.go"},
		{"different files do not join", SelectorsV1{Purposes: []string{"Testing"}, DomainAreas: []string{"Payments"}}, ""},
		{"exact not substring", SelectorsV1{DomainAreas: []string{"Pay"}}, ""},
		{"stale never trusts labels", SelectorsV1{DomainAreas: []string{"Billing"}, Keywords: []string{"c++"}}, ""},
		{"keyword punctuation", SelectorsV1{Keywords: []string{"c++"}}, "backend/one.go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector, err := CompileSelectorsV1(test.selection)
			if err != nil || !selector.Requested() {
				t.Fatalf("compile=%+v err=%v", selector, err)
			}
			got := selector.MatchingFiles(inventory)
			if test.want == "" && len(got) != 0 || test.want != "" && (len(got) != 1 || !got[test.want]) {
				t.Fatalf("matches=%v want=%q", got, test.want)
			}
		})
	}
	unfiltered, err := CompileSelectorsV1(SelectorsV1{})
	if err != nil || unfiltered.Requested() || len(unfiltered.MatchingFiles(inventory)) != 4 {
		t.Fatalf("unfiltered=%+v err=%v", unfiltered, err)
	}
	for _, test := range []struct {
		selector SelectorsV1
		flag     string
	}{
		{SelectorsV1{Purposes: []string{""}}, "--purpose"},
		{SelectorsV1{DomainAreas: []string{strings.Repeat("z", 81)}}, "--domain-area"},
		{SelectorsV1{FunctionalAreas: []string{"\x00"}}, "--functional-area"},
		{SelectorsV1{Keywords: []string{"\n"}}, "--keyword"},
	} {
		if _, err := CompileSelectorsV1(test.selector); err == nil || !strings.Contains(err.Error(), test.flag) {
			t.Fatalf("invalid %s accepted: %v", test.flag, err)
		}
	}
}

func TestSelectorsV1MatchSpacesOrKebabCaseAgainstCanonicalLabels(t *testing.T) {
	file := FileV1{Purpose: "Custom  Domain Work", DomainAreas: []string{"Source Parsing"}, FunctionalAreas: []string{"CLI Navigation"}, Keywords: []string{"Tree Sitter", "c++"}}
	document := DocumentV1{Schema: "v1", Directory: map[string]DirectoryV1{".": {Files: []FileV1{{Name: "a.go", Checksum: strings.Repeat("a", 64), Description: "Source.", Purpose: file.Purpose, DomainAreas: file.DomainAreas, FunctionalAreas: file.FunctionalAreas, Keywords: file.Keywords}}}}}
	if err := document.Normalize(); err != nil {
		t.Fatal(err)
	}
	selected := FileStateV1{Status: StatusCurrent, File: document.Directory["."].Files[0]}
	for _, values := range []SelectorsV1{
		{Purposes: []string{"CUSTOM DOMAIN WORK"}, DomainAreas: []string{"Source Parsing"}, FunctionalAreas: []string{"cli-navigation"}, Keywords: []string{"TREE SITTER"}},
		{Purposes: []string{"custom-domain-work"}, DomainAreas: []string{"source-parsing"}, FunctionalAreas: []string{"CLI NAVIGATION"}, Keywords: []string{"tree-sitter"}},
	} {
		selector, err := CompileSelectorsV1(values)
		if err != nil || !selector.Matches(selected) {
			t.Fatalf("selector %+v failed on %+v: %v", values, selected, err)
		}
	}
}
