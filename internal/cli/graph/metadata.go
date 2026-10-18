package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/wire"
)

func compileGraphMetadataSelector(purposes, domains, functional []string) (directorymeta.SelectorV1, error) {
	return directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{
		Purposes: purposes, DomainAreas: domains, FunctionalAreas: functional,
	})
}

// appendGraphMetadataFlags preserves requested (not normalized) values for replay.
func appendGraphMetadataFlags(parts []string, purposes, domains, functional []string) []string {
	for _, group := range []struct {
		flag   string
		values []string
	}{{"--purpose", purposes}, {"--domain-area", domains}, {"--functional-area", functional}} {
		for _, value := range group.values {
			parts = append(parts, group.flag, shellquote.Argument(value))
		}
	}
	return parts
}

func graphMetadataDiagnostics(existing []wire.ResultDiagnostic, filtered, noMatches bool) []wire.ResultDiagnostic {
	if !filtered {
		return existing
	}
	existing = append(existing, wire.ResultDiagnostic{Code: "metadata-selected-sources", Message: "source counts and completeness cover only checksum-current, file-owned metadata matches within the requested local paths; excluded declarations and edges are not searched"})
	if noMatches {
		existing = append(existing, wire.ResultDiagnostic{Code: "metadata-no-matches", Message: "no declaration matched in the selected sources; check grepple overview or grepple verify for missing or stale annotations"})
	}
	return existing
}

// selectGraphMetadataPaths only removes paths from the source-policy universe.
// Never assemble a graph input list from YAML or ancestor directory labels.
func selectGraphMetadataPaths(paths []string, selector directorymeta.SelectorV1) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err := directorymeta.FindRoot(cwd)
	if err != nil {
		return nil, err
	}
	actualRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("inspect graph metadata root: %w", err)
	}
	absolute := make([]string, 0, len(paths))
	keys := make([]string, 0, len(paths))
	for _, path := range paths {
		full, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		actual, err := filepath.EvalSymlinks(full)
		if err != nil {
			return nil, fmt.Errorf("inspect graph metadata source: %w", err)
		}
		relative, err := filepath.Rel(actualRoot, actual)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return nil, fmt.Errorf("graph source %q is outside local metadata root", path)
		}
		absolute = append(absolute, actual)
		keys = append(keys, filepath.ToSlash(relative))
	}
	inventory, err := directorymeta.BuildInventoryV1(actualRoot, absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect local graph metadata: %w", err)
	}
	matching := selector.MatchingFiles(inventory)
	selected := make([]string, 0, len(paths))
	for index, path := range paths {
		if matching[keys[index]] {
			selected = append(selected, path)
		}
	}
	return selected, nil
}

func requireSelectedGraphAt(at string, paths []string) error {
	separator := strings.LastIndex(at, ":")
	if separator <= 0 {
		return fmt.Errorf("location requires PATH:LINE")
	}
	line := at[separator+1:]
	if dash := strings.IndexByte(line, '-'); dash >= 0 {
		line = line[:dash]
	}
	parsed, err := strconv.Atoi(line)
	if err != nil || parsed < 1 {
		return fmt.Errorf("location requires a positive line")
	}
	full, err := filepath.Abs(at[:separator])
	if err != nil {
		return err
	}
	for _, path := range paths {
		candidate, err := filepath.Abs(path)
		if err == nil && filepath.Clean(candidate) == filepath.Clean(full) {
			return nil
		}
	}
	return fmt.Errorf("--at file is not among current metadata-selected graph sources; run grepple overview or grepple verify")
}
