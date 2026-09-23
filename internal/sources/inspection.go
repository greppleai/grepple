// Package sources owns repository source policy, discovery, metadata-backed classification, and inspection.
package sources

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/filedigest"
)

const Schema = "grepple-source-scope-v2"

type Environment struct {
	Root, ConfigPath               string
	IgnorePaths                    []string
	IgnoreDisabled, ProductionOnly bool
}

type Config struct {
	Loaded        bool   `json:"loaded"`
	Path          string `json:"path,omitempty"`
	Digest        string `json:"digest,omitempty"`
	IgnoreEnabled bool   `json:"ignoreEnabled"`
}

type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Report struct {
	Schema                  string     `json:"schema"`
	Root                    string     `json:"root"`
	Config                  Config     `json:"config"`
	ProductionOnly          bool       `json:"productionOnly"`
	DiscoveredFiles         int        `json:"discoveredFiles"`
	SelectedFiles           int        `json:"selectedFiles"`
	ExcludedFiles           int        `json:"excludedFiles"`
	OmittedSubtrees         int        `json:"omittedSubtrees"`
	UnconditionalExclusions []string   `json:"unconditionalExclusions"`
	Classifications         []Count    `json:"classifications"`
	Exclusions              []Count    `json:"exclusions"`
	Decisions               []Decision `json:"decisions"`
}

func Build(paths []string, environment Environment, workingDirectory string) (Report, error) {
	root, configPath := environment.Root, environment.ConfigPath
	if len(paths) == 0 {
		paths = []string{root}
	}
	options := InspectionOptions{Root: root, IgnoreRoot: root, ProductionOnly: environment.ProductionOnly}
	if !environment.IgnoreDisabled {
		options.IgnorePaths = append([]string(nil), environment.IgnorePaths...)
	}
	decisions, err := Inspect(paths, options)
	if err != nil {
		return Report{}, err
	}
	report := Report{Schema: Schema, Root: displayPath(root, workingDirectory), ProductionOnly: options.ProductionOnly, Config: Config{Loaded: configPath != "", Path: displayPath(configPath, workingDirectory), IgnoreEnabled: configPath != "" && !environment.IgnoreDisabled}, Decisions: decisions, UnconditionalExclusions: []string{".git/**", ".grepple/**", ".worktrees/**"}}
	if configPath != "" {
		report.Config.Digest, err = filedigest.SHA256(configPath)
		if err != nil {
			return Report{}, err
		}
	}
	classifications, exclusions := map[string]int{}, map[string]int{}
	for _, decision := range decisions {
		if decision.Subtree {
			report.OmittedSubtrees++
			continue
		}
		report.DiscoveredFiles++
		classifications[decision.Classification]++
		if decision.Selected {
			report.SelectedFiles++
		} else {
			report.ExcludedFiles++
			exclusions[decision.Reason]++
		}
	}
	report.Classifications = sortedCounts(classifications)
	report.Exclusions = sortedCounts(exclusions)
	return report, nil
}

func sortedCounts(values map[string]int) []Count {
	names := make([]string, 0, len(values))
	for name := range values {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]Count, 0, len(names))
	for _, name := range names {
		result = append(result, Count{Name: name, Count: values[name]})
	}
	return result
}

func displayPath(path, workingDirectory string) string {
	if path == "" {
		return ""
	}
	if relative, err := filepath.Rel(workingDirectory, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}
