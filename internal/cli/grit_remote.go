package cli

import (
	"context"

	"github.com/greppleai/grepple/api"
)

func collectGritRemote(ctx context.Context, request api.GritRequest, server string, required int) (api.GritResponse, error) {
	if required <= 0 {
		required = api.MaxGritPageLimit
	}
	collected := emptyGritResponse()
	offset := 0
	for len(collected.Findings) < required {
		if err := ctx.Err(); err != nil {
			return api.GritResponse{}, err
		}
		pageLimit := required - len(collected.Findings)
		if pageLimit > api.MaxGritPageLimit {
			pageLimit = api.MaxGritPageLimit
		}
		request.Skip = intPointer(offset)
		request.Limit = intPointer(pageLimit)
		page, err := requestGritRemote(ctx, request, server)
		if err != nil {
			return api.GritResponse{}, err
		}
		appendGritRemotePage(&collected, page, offset == 0)
		offset += len(page.Findings)
		if len(page.Findings) == 0 || offset >= page.Total {
			break
		}
	}
	return collected, nil
}

func emptyGritResponse() api.GritResponse {
	return api.GritResponse{
		Findings: []api.GritFinding{}, Diagnostics: []api.GritDiagnostic{},
		Truncations: []api.GritTruncation{}, ShardErrors: []string{},
	}
}

func appendGritRemotePage(collected *api.GritResponse, page api.GritResponse, first bool) {
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
		return api.MaxGritPageLimit
	}
	maximum := int(^uint(0) >> 1)
	if skip > maximum-limit {
		return maximum
	}
	return skip + limit
}
