package search

import "testing"

func TestRelatedPreviewUnavailablePreservesFacts(t *testing.T) {
	for _, preview := range []*RelatedPreview{{Content: "", Start: 300, End: 303}, {Content: "one\n", Start: 1, End: 303}, {Content: "one\n", Start: 0, End: 1}} {
		points := relatedSymbols([]RelatedPoint{{Name: "Target", Path: "target.rs", Start: 300, End: 303, Preview: preview}})
		if len(points) != 1 || points[0].Name != "Target" || points[0].Start != 300 || points[0].End != 303 || len(points[0].Segments) != 0 {
			t.Fatal("unavailable preview changed navigation facts")
		}
	}
}
