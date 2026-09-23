package boundaryanalysis

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// BoundaryRisk is the heuristic review priority of a boundary signal.
type BoundaryRisk string

// Boundary risk values are ordered by boundaryRiskRank, not lexically.
const (
	BoundaryRiskInformational BoundaryRisk = "informational"
	BoundaryRiskLow           BoundaryRisk = "low"
	BoundaryRiskMedium        BoundaryRisk = "medium"
	BoundaryRiskHigh          BoundaryRisk = "high"
	BoundaryRiskCritical      BoundaryRisk = "critical"
)

func boundaryRiskRank(risk BoundaryRisk) int {
	switch risk {
	case BoundaryRiskCritical:
		return 4
	case BoundaryRiskHigh:
		return 3
	case BoundaryRiskMedium:
		return 2
	case BoundaryRiskLow:
		return 1
	default:
		return 0
	}
}

// BoundaryBreadth summarizes distinct external consumers of an owner file.
type BoundaryBreadth struct {
	Functions int `json:"functions"`
	Files     int `json:"files"`
	Packages  int `json:"packages"`
}

// BoundarySurface summarizes externally used callables from an owner file.
type BoundarySurface struct {
	External int `json:"external"`
	Declared int `json:"declared"`
}

// BoundaryConsumer identifies one callable containing a boundary-crossing workflow.
type BoundaryConsumer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Package string `json:"package,omitempty"`
	Line    int    `json:"line"`
}

// BoundaryPattern is a repeated callable set, sequence, or member/call combination.
type BoundaryPattern struct {
	Interactions []string           `json:"interactions"`
	Occurrences  int                `json:"occurrences"`
	Files        int                `json:"files"`
	Packages     int                `json:"packages"`
	Consumers    []BoundaryConsumer `json:"consumers"`
}

type boundaryInteraction struct {
	label string
}

type boundaryPatternGroup struct {
	interactions []string
	consumers    []BoundaryConsumer
}

func boundaryTerminalTypeName(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "*"))
	if index := strings.LastIndexAny(value, ".:/"); index >= 0 {
		value = value[index+1:]
	}
	if index := strings.IndexByte(value, '['); index >= 0 {
		value = value[:index]
	}
	return value
}

func boundaryDeclarationType(declaration parser.NavigationDeclaration) string {
	if receiver := boundaryTerminalTypeName(declaration.Receiver); receiver != "" {
		return receiver
	}
	if container := boundaryTerminalTypeName(declaration.Container); container != "" {
		return container
	}
	if declaration.Kind != "method" && declaration.Kind != "constructor" {
		return ""
	}
	separator := strings.LastIndex(declaration.Name, ".")
	if separator <= 0 {
		return ""
	}
	return boundaryTerminalTypeName(declaration.Name[:separator])
}

func boundaryCallableName(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func boundaryConsumer(declaration parser.NavigationDeclaration) BoundaryConsumer {
	return BoundaryConsumer{ID: declaration.ID, Name: declaration.Name, Path: declaration.Path, Package: boundaryPackage(declaration), Line: declaration.Start}
}

func boundaryPackage(declaration parser.NavigationDeclaration) string {
	if declaration.Package != "" {
		return declaration.Package
	}
	directory := filepath.ToSlash(filepath.Dir(declaration.Path))
	if directory == "" {
		return "."
	}
	return directory
}

func boundaryBreadth(consumers []BoundaryConsumer) BoundaryBreadth {
	files := make(map[string]bool)
	packages := make(map[string]bool)
	for _, consumer := range consumers {
		files[consumer.Path] = true
		packages[consumer.Package] = true
	}
	return BoundaryBreadth{Functions: len(consumers), Files: len(files), Packages: len(packages)}
}

func addBoundaryPattern(groups map[string]*boundaryPatternGroup, interactions []string, consumer BoundaryConsumer) {
	key := strings.Join(interactions, "\x00")
	group := groups[key]
	if group == nil {
		group = &boundaryPatternGroup{interactions: append([]string(nil), interactions...)}
		groups[key] = group
	}
	group.consumers = append(group.consumers, consumer)
}

func boundaryPatterns(groups map[string]*boundaryPatternGroup, minimum int) []BoundaryPattern {
	patterns := make([]BoundaryPattern, 0, len(groups))
	for _, group := range groups {
		if len(group.consumers) < minimum {
			continue
		}
		sortBoundaryConsumers(group.consumers)
		breadth := boundaryBreadth(group.consumers)
		patterns = append(patterns, BoundaryPattern{Interactions: group.interactions, Occurrences: len(group.consumers), Files: breadth.Files, Packages: breadth.Packages, Consumers: group.consumers})
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].Occurrences != patterns[j].Occurrences {
			return patterns[i].Occurrences > patterns[j].Occurrences
		}
		return strings.Join(patterns[i].Interactions, "\x00") < strings.Join(patterns[j].Interactions, "\x00")
	})
	return patterns
}

func uniqueSortedBoundaryInteractions(interactions []string) []string {
	set := make(map[string]bool, len(interactions))
	for _, interaction := range interactions {
		set[interaction] = true
	}
	return sortedBoundaryKeys(set)
}

func sortedBoundaryKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func boundaryCombination(interactions []boundaryInteraction) ([]string, bool, bool) {
	combination := make([]string, 0, len(interactions))
	hasMember, hasCall := false, false
	for _, interaction := range interactions {
		combination = append(combination, interaction.label)
		if strings.HasSuffix(interaction.label, "()") {
			hasCall = true
		} else {
			hasMember = true
		}
	}
	return uniqueSortedBoundaryInteractions(combination), hasMember, hasCall
}

func sortBoundaryConsumers(consumers []BoundaryConsumer) {
	sort.Slice(consumers, func(i, j int) bool {
		if consumers[i].Path != consumers[j].Path {
			return consumers[i].Path < consumers[j].Path
		}
		if consumers[i].Line != consumers[j].Line {
			return consumers[i].Line < consumers[j].Line
		}
		return consumers[i].ID < consumers[j].ID
	})
}
