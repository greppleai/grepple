package cli

import (
	"fmt"
	"sort"

	"github.com/greppleai/grepple/api"
)

type relatedTypeDefinition struct {
	name, path string
	artifact   *api.NavigationArtifactIdentity
	start, end int
	segments   []api.ResultSegment
}

func collectRelatedTypeDefinitions(results []api.FileResult) []relatedTypeDefinition {
	definitions := make(map[string]relatedTypeDefinition)
	for _, result := range results {
		collectRelatedTypePoints(result.Related, definitions)
	}
	ordered := make([]relatedTypeDefinition, 0, len(definitions))
	for _, definition := range definitions {
		ordered = append(ordered, definition)
	}
	sort.Slice(ordered, func(i, j int) bool {
		leftArtifact, rightArtifact := relatedTypeArtifactDigest(ordered[i]), relatedTypeArtifactDigest(ordered[j])
		if leftArtifact != rightArtifact {
			return leftArtifact < rightArtifact
		}
		if ordered[i].path != ordered[j].path {
			return ordered[i].path < ordered[j].path
		}
		if ordered[i].start != ordered[j].start {
			return ordered[i].start < ordered[j].start
		}
		if ordered[i].end != ordered[j].end {
			return ordered[i].end < ordered[j].end
		}
		return ordered[i].name < ordered[j].name
	})
	return ordered
}

func relatedTypeArtifactDigest(definition relatedTypeDefinition) string {
	if definition.artifact == nil {
		return ""
	}
	return definition.artifact.Digest
}

func collectRelatedTypePoints(points []api.RelatedSymbol, definitions map[string]relatedTypeDefinition) {
	for _, point := range points {
		if point.Direction == "type" && point.Path != "" && len(point.Segments) > 0 {
			artifactKey := ""
			if point.Artifact != nil {
				artifactKey = point.Artifact.Digest
			}
			key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", artifactKey, point.Path, point.Start, point.End)
			if _, exists := definitions[key]; !exists {
				definitions[key] = relatedTypeDefinition{
					name: point.Name, path: point.Path, artifact: point.Artifact, start: point.Start, end: point.End,
					segments: append([]api.ResultSegment(nil), point.Segments...),
				}
			}
		}
		collectRelatedTypePoints(point.Related, definitions)
	}
}

func (renderer segmentRenderer) renderRelatedTypeDefinitions(definitions []relatedTypeDefinition) error {
	if len(definitions) == 0 {
		return nil
	}
	if err := renderer.output.writeString("\nRelated type definitions:\n"); err != nil {
		return err
	}
	for _, definition := range definitions {
		path := definition.path
		suffix := ""
		if definition.artifact != nil {
			path = definition.artifact.Repository + ":" + path
			suffix = " [" + definition.artifact.Module + "@" + definition.artifact.Version + "; commit " + definition.artifact.Commit + "]"
		}
		if err := renderer.output.writeString(fmt.Sprintf("\n%s:%d-%d  %s%s\n", path, definition.start, definition.end, definition.name, suffix)); err != nil {
			return err
		}
		width := segmentLineWidth(definition.segments)
		for _, segment := range definition.segments {
			if err := renderer.renderSegment(definition.path, path, definition.artifact, segment, width, ""); err != nil {
				return err
			}
		}
	}
	return nil
}
