package search

func RequestFromParams(params Params) Request {
	// A selector is local-only. An accidental remote conversion must not send an
	// unverified path or a selector-shaped reference to an older backend.
	if params.AtScope != nil {
		return Request{}
	}
	query := params.Query
	request := Request{Query: &query, Globs: params.Globs, Regex: &params.Regex, IgnoreCase: &params.IgnoreCase, InvertMatch: &params.InvertMatch, Sort: params.Sort, Files: params.Files, LineRanges: params.LineRanges, EnclosingRanges: params.EnclosingRanges, AnnotateScopes: params.AnnotateScopes, Related: params.Related, NoRelated: params.NoRelated, At: params.At, FollowRelated: params.FollowRelated, SkipSegments: params.SkipSegments, CountByRepo: params.CountByRepo}
	request.MaxFiles, request.Skip, request.Limit = &params.MaxFiles, &params.Skip, &params.Limit
	request.BeforeContext, request.AfterContext = &params.BeforeContext, &params.AfterContext
	request.Repo, request.ExcludeRepo = append([]string(nil), params.Repo...), append([]string(nil), params.ExcludeRepo...)
	return request
}
