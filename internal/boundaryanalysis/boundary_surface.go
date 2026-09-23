package boundaryanalysis

// BoundaryTypeSurface distinguishes representation exposure from implementation use.
type BoundaryTypeSurface string

// Boundary type surface values identify representation and implementation roles.
const (
	BoundaryTypeSurfacePublicAPI        BoundaryTypeSurface = "public-api"
	BoundaryTypeSurfacePrivateSignature BoundaryTypeSurface = "private-signature"
	BoundaryTypeSurfaceField            BoundaryTypeSurface = "field-representation"
	BoundaryTypeSurfaceBodyLocal        BoundaryTypeSurface = "body-local"
	BoundaryTypeSurfaceUnknown          BoundaryTypeSurface = "unknown"
)

// BoundaryTypeSurfaces counts type usage by architectural surface.
type BoundaryTypeSurfaces struct {
	PublicAPI           int `json:"publicApi"`
	PrivateSignature    int `json:"privateSignature"`
	FieldRepresentation int `json:"fieldRepresentation"`
	BodyLocal           int `json:"bodyLocal"`
	Unknown             int `json:"unknown"`
}

func boundaryTypeSurface(role string, public bool) BoundaryTypeSurface {
	if role == "field" {
		return BoundaryTypeSurfaceField
	}
	if public {
		return BoundaryTypeSurfacePublicAPI
	}
	switch role {
	case "parameter", "result", "receiver":
		return BoundaryTypeSurfacePrivateSignature
	case "local":
		return BoundaryTypeSurfaceBodyLocal
	default:
		return BoundaryTypeSurfaceUnknown
	}
}

func boundaryTypeSurfaces(details []BoundaryTypeUsage) BoundaryTypeSurfaces {
	var result BoundaryTypeSurfaces
	for _, detail := range details {
		if detail.Public && detail.Surface == BoundaryTypeSurfaceField {
			result.PublicAPI++
		}
		switch detail.Surface {
		case BoundaryTypeSurfacePublicAPI:
			result.PublicAPI++
		case BoundaryTypeSurfacePrivateSignature:
			result.PrivateSignature++
		case BoundaryTypeSurfaceField:
			result.FieldRepresentation++
		case BoundaryTypeSurfaceBodyLocal:
			result.BodyLocal++
		default:
			result.Unknown++
		}
	}
	return result
}
