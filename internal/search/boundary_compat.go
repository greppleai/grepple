package search

import (
	boundaryanalysis "github.com/greppleai/grepple/internal/boundaryanalysis"
	"github.com/greppleai/grepple/internal/parser"
)

const BoundaryPolicySchema = boundaryanalysis.BoundaryPolicySchema

const (
	BoundaryRiskInformational = boundaryanalysis.BoundaryRiskInformational
	BoundaryRiskLow           = boundaryanalysis.BoundaryRiskLow
	BoundaryRiskMedium        = boundaryanalysis.BoundaryRiskMedium
	BoundaryRiskHigh          = boundaryanalysis.BoundaryRiskHigh
	BoundaryRiskCritical      = boundaryanalysis.BoundaryRiskCritical

	BoundarySpreadPackageInternal = boundaryanalysis.BoundarySpreadPackageInternal
	BoundarySpreadCrossPackage    = boundaryanalysis.BoundarySpreadCrossPackage
	BoundarySpreadCrossLayer      = boundaryanalysis.BoundarySpreadCrossLayer
	BoundarySpreadPublicAPI       = boundaryanalysis.BoundarySpreadPublicAPI

	BoundaryContainmentApproved = boundaryanalysis.BoundaryContainmentApproved
	BoundaryContainmentEscaped  = boundaryanalysis.BoundaryContainmentEscaped
	BoundaryContainmentUnknown  = boundaryanalysis.BoundaryContainmentUnknown

	BoundaryTypeSurfacePublicAPI        = boundaryanalysis.BoundaryTypeSurfacePublicAPI
	BoundaryTypeSurfacePrivateSignature = boundaryanalysis.BoundaryTypeSurfacePrivateSignature
	BoundaryTypeSurfaceField            = boundaryanalysis.BoundaryTypeSurfaceField
	BoundaryTypeSurfaceBodyLocal        = boundaryanalysis.BoundaryTypeSurfaceBodyLocal
	BoundaryTypeSurfaceUnknown          = boundaryanalysis.BoundaryTypeSurfaceUnknown

	BoundaryTypeOriginLocal           = boundaryanalysis.BoundaryTypeOriginLocal
	BoundaryTypeOriginFirstParty      = boundaryanalysis.BoundaryTypeOriginFirstParty
	BoundaryTypeOriginStandardLibrary = boundaryanalysis.BoundaryTypeOriginStandardLibrary
	BoundaryTypeOriginThirdParty      = boundaryanalysis.BoundaryTypeOriginThirdParty
	BoundaryTypeOriginUnresolved      = boundaryanalysis.BoundaryTypeOriginUnresolved
)

type BoundaryRisk = boundaryanalysis.BoundaryRisk
type BoundaryBreadth = boundaryanalysis.BoundaryBreadth
type BoundarySurface = boundaryanalysis.BoundarySurface
type BoundaryConsumer = boundaryanalysis.BoundaryConsumer
type BoundaryPattern = boundaryanalysis.BoundaryPattern
type BoundaryCandidate = boundaryanalysis.BoundaryCandidate
type BoundaryPolicy = boundaryanalysis.BoundaryPolicy
type BoundaryLayer = boundaryanalysis.BoundaryLayer
type BoundaryContainmentRule = boundaryanalysis.BoundaryContainmentRule
type BoundaryFacadeRule = boundaryanalysis.BoundaryFacadeRule
type BoundaryPathClassification = boundaryanalysis.BoundaryPathClassification
type BoundarySpread = boundaryanalysis.BoundarySpread
type BoundaryContainment = boundaryanalysis.BoundaryContainment
type BoundaryFacadeBypass = boundaryanalysis.BoundaryFacadeBypass
type BoundaryTypeSurface = boundaryanalysis.BoundaryTypeSurface
type BoundaryTypeSurfaces = boundaryanalysis.BoundaryTypeSurfaces
type BoundaryTypeOrigin = boundaryanalysis.BoundaryTypeOrigin
type BoundaryTypeRoles = boundaryanalysis.BoundaryTypeRoles
type BoundaryTypeUsage = boundaryanalysis.BoundaryTypeUsage
type BoundaryTypeSpread = boundaryanalysis.BoundaryTypeSpread

func AnalyzeBoundaries(graph parser.NavigationGraph, minimum int) ([]BoundaryCandidate, error) {
	return boundaryanalysis.AnalyzeBoundaries(graph, minimum)
}

func AnalyzeBoundariesWithPolicy(graph parser.NavigationGraph, minimum int, policy BoundaryPolicy) ([]BoundaryCandidate, error) {
	return boundaryanalysis.AnalyzeBoundariesWithPolicy(graph, minimum, policy)
}

func AnalyzeTypeBoundaries(graph parser.NavigationGraph, minimum int) ([]BoundaryTypeSpread, error) {
	return boundaryanalysis.AnalyzeTypeBoundaries(graph, minimum)
}

func AnalyzeTypeBoundariesWithPolicy(graph parser.NavigationGraph, minimum int, policy BoundaryPolicy) ([]BoundaryTypeSpread, error) {
	return boundaryanalysis.AnalyzeTypeBoundariesWithPolicy(graph, minimum, policy)
}

func AnalyzeFacadeBypasses(graph parser.NavigationGraph, policy BoundaryPolicy) []BoundaryFacadeBypass {
	return boundaryanalysis.AnalyzeFacadeBypasses(graph, policy)
}

func ValidateBoundaryPolicy(policy BoundaryPolicy) error {
	return boundaryanalysis.ValidateBoundaryPolicy(policy)
}
