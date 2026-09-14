package search

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

func appendBoundaryFieldTypeUsages(usages []parser.NavigationTypeUsage, fields []parser.NavigationField, declarations map[string]parser.NavigationDeclaration) []parser.NavigationTypeUsage {
	for _, field := range fields {
		if field.Type == "" || field.Path == "" || field.Embedded {
			continue
		}
		id := fmt.Sprintf("field:%s:%s:%s:%s:%d", field.Language, field.Path, field.OwnerType, field.Name, field.Line)
		declarations[id] = parser.NavigationDeclaration{ID: id, Name: field.OwnerType + "." + field.Name, Kind: "field", Language: field.Language, Path: field.Path, Container: field.OwnerType, Package: field.Package, Visibility: field.Visibility, Start: field.Line, End: field.Line}
		usages = append(usages, parser.NavigationTypeUsage{CallerID: id, Type: field.Type, ImportPath: field.ImportPath, Role: "field", Language: field.Language, Path: field.Path, Line: field.Line})
	}
	return usages
}

func classifyBoundaryTypePolicy(spread *BoundaryTypeSpread, policy BoundaryPolicy) {
	spread.Spread = boundaryTypeSpreadClass(*spread, policy)
	spread.Containment = boundaryContainmentForConsumers(policy, spread.OwnerFile, boundaryUsageConsumers(spread.UsageDetails))
	if spread.OwnerFile == "" {
		spread.Containment = BoundaryContainmentUnknown
	}
	if spread.Origin == BoundaryTypeOriginThirdParty && boundaryHasPublicFieldExposure(spread.UsageDetails) {
		spread.Reasons = appendUniqueBoundaryReason(spread.Reasons, "transitive-public-exposure")
	}
	categories := boundaryUsageCategories(policy, spread.UsageDetails)
	if hasBoundaryCategory(categories, "utility-hub") || hasBoundaryCategory(categories, "test-framework") || hasBoundaryCategory(categories, "declarative-configuration") || hasBoundaryCategory(categories, "lifecycle-cleanup") || hasBoundaryCategory(categories, "adapter-protocol") {
		spread.Risk = downrankBoundaryRisk(spread.Risk)
		spread.Reasons = appendUniqueBoundaryReason(spread.Reasons, "repository-approved-usage-role")
	}
	if spread.Containment == BoundaryContainmentEscaped {
		spread.Reasons = appendUniqueBoundaryReason(spread.Reasons, "containment-escaped")
	}
}

func boundaryHasPublicFieldExposure(details []BoundaryTypeUsage) bool {
	for _, detail := range details {
		if detail.Role == "field" && detail.Public {
			return true
		}
	}
	return false
}

func boundaryTypeSpreadClass(spread BoundaryTypeSpread, policy BoundaryPolicy) BoundarySpread {
	if len(spread.PublicExposures) > 0 {
		return BoundarySpreadPublicAPI
	}
	layers := map[string]bool{}
	if layer := boundaryLayerForPath(policy, spread.OwnerFile); layer != "" {
		layers[layer] = true
	}
	for _, usage := range spread.UsageDetails {
		if layer := boundaryLayerForPath(policy, usage.Path); layer != "" {
			layers[layer] = true
		}
	}
	if len(layers) > 1 {
		return BoundarySpreadCrossLayer
	}
	if boundaryTypeCrossPackage(spread) {
		return BoundarySpreadCrossPackage
	}
	return BoundarySpreadPackageInternal
}

func boundaryUsageConsumers(details []BoundaryTypeUsage) []BoundaryConsumer {
	result := make([]BoundaryConsumer, 0, len(details))
	for _, detail := range details {
		result = append(result, detail.BoundaryConsumer)
	}
	return result
}

func boundaryUsageCategories(policy BoundaryPolicy, details []BoundaryTypeUsage) []string {
	set := map[string]bool{}
	for _, detail := range details {
		for _, category := range boundaryPathCategories(policy, detail.Path) {
			set[category] = true
		}
	}
	result := make([]string, 0, len(set))
	for category := range set {
		result = append(result, category)
	}
	sort.Strings(result)
	return result
}

func hasBoundaryCategory(categories []string, expected string) bool {
	for _, category := range categories {
		if category == expected {
			return true
		}
	}
	return false
}

func appendUniqueBoundaryReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func downrankBoundaryRisk(risk BoundaryRisk) BoundaryRisk {
	switch risk {
	case BoundaryRiskCritical:
		return BoundaryRiskHigh
	case BoundaryRiskHigh:
		return BoundaryRiskMedium
	case BoundaryRiskMedium:
		return BoundaryRiskLow
	case BoundaryRiskLow:
		return BoundaryRiskInformational
	default:
		return risk
	}
}

func normalizedBoundaryCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}
