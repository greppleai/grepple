package cli

import "go.opentelemetry.io/otel/attribute"

// traceCommandAttributes records only selected typed modes, never argument values.
// An absent option means unreported, not that a flag was explicitly set to false.
// Parsing normalizes aliases and defaults, so these are not literal argv tokens.
func (values *Arguments) traceCommandAttributes() []attribute.KeyValue {
	name := values.traceCommandName()
	attrs := []attribute.KeyValue{attribute.String("cli.command.name", name)}
	selected := func(key string, enabled bool) {
		if enabled {
			attrs = append(attrs, attribute.Bool(key, true))
		}
	}
	selected("cli.command.option.no_spill", values.NoSpill)
	selected("cli.command.option.no_repo_config", values.NoRepositoryConfig)
	selected("cli.command.option.no_config_ignore", values.NoConfigIgnore)
	selected("cli.command.option.production_only", values.ProductionOnly)
	if name == "graph.callers" || name == "graph.callees" || name == "graph.resolve" || name == "architecture" {
		selected("cli.command.option.daemon_requested", values.Daemon)
	}

	switch name {
	case "search":
		if values.Search == nil {
			return attrs
		}
		search := values.Search
		// --local wins over remote selection in optionsFromArgs. An invalid
		// --local/--remote-only combination has no reliable executed mode.
		if !(search.Local && (search.RemoteOnly || search.Cursor != "")) {
			remote := !search.Local && (search.Remote || search.RemoteOnly || search.Cursor != "" || search.Server != "")
			mode := "local"
			if remote && (search.RemoteOnly || search.Cursor != "") {
				mode = "remote"
			} else if remote {
				mode = "mixed"
			}
			attrs = append(attrs, attribute.String("cli.command.source_mode", mode))
		}
		format := "text"
		if search.JSON || search.JSONMatches {
			format = "json"
		} else if search.CountSummary || search.CountByRepo {
			format = "compact"
		}
		attrs = append(attrs, attribute.String("cli.command.output_format", format))
		selected("cli.command.option.at", search.At != "")
		selected("cli.command.option.outline", search.Outline)
		selected("cli.command.option.files", search.Files)
		selected("cli.command.option.files_with_matches", search.FilesWithMatches)
		selected("cli.command.option.count_summary", search.CountSummary || search.CountByRepo)
		selected("cli.command.option.related_requested", search.Related || search.FollowRelated > 0)
		selected("cli.command.option.no_related_requested", search.NoRelated)
		selected("cli.command.option.line_only", search.LineOnly)
		selected("cli.command.option.fixed_strings", search.Fixed)
		selected("cli.command.option.ignore_case", search.IgnoreCase)
		selected("cli.command.option.json_matches", search.JSONMatches)
		selected("cli.command.option.cursor_requested", search.Cursor != "")
		selected("cli.command.option.metadata_filter_requested", len(search.Purposes)+len(search.DomainAreas)+len(search.FunctionalAreas)+len(search.Keywords) > 0)
	case "graph.callers", "graph.callees":
		query := values.Graph.Callers
		if name == "graph.callees" {
			query = values.Graph.Callees
		}
		if query != nil {
			mode := "local"
			if query.Repository != "" {
				mode = "remote"
			}
			attrs = append(attrs, attribute.String("cli.command.source_mode", mode))
			format := "compact"
			if query.JSON {
				format = "json"
			}
			attrs = append(attrs, attribute.String("cli.command.output_format", format))
			selected("cli.command.option.at", query.At != "")
		}
	case "graph.resolve":
		if values.Graph.Resolve != nil {
			attrs = append(attrs, attribute.String("cli.command.source_mode", "local"))
			format := "compact"
			if values.Graph.Resolve.JSON {
				format = "json"
			}
			attrs = append(attrs, attribute.String("cli.command.output_format", format))
		}
	case "tree":
		if values.Tree != nil {
			// Legacy positional OWNER/REPO is normalized later: no local claim.
			if values.Tree.Repository != "" {
				attrs = append(attrs, attribute.String("cli.command.source_mode", "remote"))
			}
			format := "text"
			if values.Tree.JSON {
				format = "json"
			}
			attrs = append(attrs, attribute.String("cli.command.output_format", format))
			selected("cli.command.option.metadata_filter_requested", len(values.Tree.Purposes)+len(values.Tree.DomainAreas)+len(values.Tree.FunctionalAreas)+len(values.Tree.Keywords) > 0)
		}
	}
	return attrs
}
