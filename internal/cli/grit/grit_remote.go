package grit

import (
	"context"

	"github.com/greppleai/grepple/internal/wire"
)

func collectGritRemote(ctx context.Context, request wire.GritRequest, server string, required int, supplied ...Dependencies) (wire.GritResponse, error) {
	dependencies := Dependencies{}
	if len(supplied) > 0 {
		dependencies = supplied[0]
	}
	if required <= 0 {
		required = wire.MaxGritPageLimit
	}
	collected := emptyGritResponse()
	offset := 0
	for len(collected.Findings) < required {
		if err := ctx.Err(); err != nil {
			return wire.GritResponse{}, err
		}
		pageLimit := required - len(collected.Findings)
		if pageLimit > wire.MaxGritPageLimit {
			pageLimit = wire.MaxGritPageLimit
		}
		request.Skip = intPointer(offset)
		request.Limit = intPointer(pageLimit)
		page, err := dependencies.requestRemote(ctx, request, server)
		if err != nil {
			return wire.GritResponse{}, err
		}
		appendGritRemotePage(&collected, page, offset == 0)
		offset += len(page.Findings)
		if len(page.Findings) == 0 || offset >= page.Total {
			break
		}
	}
	return collected, nil
}

func emptyGritResponse() wire.GritResponse {
	return wire.GritResponse{
		Findings: []wire.GritFinding{}, Diagnostics: []wire.GritDiagnostic{},
		Truncations: []wire.GritTruncation{}, ShardErrors: []string{},
	}
}

func appendGritRemotePage(collected *wire.GritResponse, page wire.GritResponse, first bool) {
	if first {
		collected.Metadata = page.Metadata
	}
	collected.Findings = append(collected.Findings, page.Findings...)
	collected.Diagnostics = append(collected.Diagnostics, page.Diagnostics...)
	collected.Truncations = append(collected.Truncations, page.Truncations...)
	collected.ShardErrors = append(collected.ShardErrors, page.ShardErrors...)
	collected.Statistics = addGritStatistics(collected.Statistics, page.Statistics)
	if page.Total > collected.Total {
		collected.Total = page.Total
	}
}

func intPointer(value int) *int {
	return &value
}

func requiredGritRemoteFindings(skip, limit int) int {
	if limit == 0 {
		return wire.MaxGritPageLimit
	}
	maximum := int(^uint(0) >> 1)
	if skip > maximum-limit {
		return maximum
	}
	return skip + limit
}
