package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type datasetInfo struct {
	Files        int                    `json:"files"`
	Stats        navigation.SourceStats `json:"source_stats"`
	BuildNS      int64                  `json:"build_ns"`
	Replicas     int                    `json:"replicas"`
	Declarations int                    `json:"declarations"`
	Calls        int                    `json:"calls"`
	Queries      []request              `json:"queries"`
}

func exportDataset(repos, out string, replicas int) error {
	var paths []string
	for _, repo := range strings.Split(repos, ",") {
		command := exec.Command("git", "-C", repo, "ls-files", "-z")
		data, err := command.Output()
		if err != nil {
			return err
		}
		for _, path := range strings.Split(string(data), "\x00") {
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "generated") || strings.HasSuffix(path, ".pb.go") || strings.HasPrefix(path, "testdata/") {
				continue
			}
			paths = append(paths, filepath.Join(repo, path))
		}
	}
	sort.Strings(paths)
	start := time.Now()
	graph, stats := navigation.NewGraphEngine(navigation.BuildOptions{DisableCache: true}).BuildFiles(paths)
	duration := time.Since(start)
	if stats.Failed != 0 {
		return fmt.Errorf("source failures: %+v", stats)
	}
	baseQueries := makeQueries(graph)
	graph = replicateGraph(graph, replicas)
	info := datasetInfo{Files: len(paths), Stats: stats, BuildNS: duration.Nanoseconds(), Replicas: replicas, Declarations: len(graph.Declarations), Calls: len(graph.Calls)}
	for _, q := range baseQueries {
		for i, id := range q.Roots {
			q.Roots[i] = "r0/" + id
		}
		info.Queries = append(info.Queries, q)
	}
	if err := writeJSON(out, graph); err != nil {
		return err
	}
	return writeJSON(out+".meta.json", info)
}
func makeQueries(graph parser.NavigationGraph) []request {
	degree := map[string]int{}
	valid := map[string]bool{}
	for _, d := range graph.Declarations {
		valid[d.ID] = true
	}
	for _, c := range graph.Calls {
		if len(callTargets(c)) != 0 {
			degree[c.CallerID]++
		}
		for _, target := range callTargets(c) {
			degree[target]++
		}
	}
	var candidates []string
	for id, n := range degree {
		if n >= 3 && valid[id] {
			candidates = append(candidates, id)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if degree[candidates[i]] != degree[candidates[j]] {
			return degree[candidates[i]] < degree[candidates[j]]
		}
		return candidates[i] < candidates[j]
	})
	if len(candidates) == 0 {
		panic("no connected navigation roots")
	}
	roots := []string{candidates[len(candidates)/2], candidates[len(candidates)*9/10]}
	var queries []request
	for _, root := range roots {
		for _, dir := range []navigation.NavigationQueryDirection{navigation.NavigationQueryCallers, navigation.NavigationQueryCallees, navigation.NavigationQueryImpact} {
			for _, depth := range []int{1, 3} {
				queries = append(queries, request{Roots: []string{root}, Direction: dir, Depth: depth})
			}
		}
	}
	return queries
}
func replicateGraph(base parser.NavigationGraph, n int) parser.NavigationGraph {
	result := emptyGraph()
	for r := 0; r < n; r++ {
		prefix := fmt.Sprintf("r%d/", r)
		for _, d := range base.Declarations {
			d.ID = prefix + d.ID
			d.Path = prefix + d.Path
			if d.Container != "" {
				d.Container = prefix + terminal(d.Container)
			}
			if d.Receiver != "" {
				d.Receiver = prefix + terminal(d.Receiver)
			}
			result.Declarations = append(result.Declarations, d)
		}
		for _, c := range base.Calls {
			c.ID = prefix + c.ID
			c.CallerID = prefix + c.CallerID
			if c.TargetID != "" {
				c.TargetID = prefix + c.TargetID
			}
			targets := make([]string, len(c.CandidateTargetIDs))
			for i, id := range c.CandidateTargetIDs {
				targets[i] = prefix + id
			}
			c.CandidateTargetIDs = targets
			c.Path = prefix + c.Path
			if c.ReceiverRootType != "" {
				c.ReceiverRootType = prefix + terminal(c.ReceiverRootType)
			}
			if c.ReceiverType != "" {
				c.ReceiverType = prefix + terminal(c.ReceiverType)
			}
			result.Calls = append(result.Calls, c)
		}
		for _, f := range base.TypeDeclarations {
			f.Path = prefix + f.Path
			result.TypeDeclarations = append(result.TypeDeclarations, f)
		}
		for _, f := range base.Imports {
			f.Path = prefix + f.Path
			f.TargetPaths = nil
			result.Imports = append(result.Imports, f)
		}
		for _, f := range base.Exports {
			f.Path = prefix + f.Path
			result.Exports = append(result.Exports, f)
		}
		for _, f := range base.Fields {
			f.OwnerType = prefix + terminal(f.OwnerType)
			f.Path = prefix + f.Path
			result.Fields = append(result.Fields, f)
		}
		for _, f := range base.TypeUsages {
			f.CallerID = prefix + f.CallerID
			f.Path = prefix + f.Path
			result.TypeUsages = append(result.TypeUsages, f)
		}
		for _, f := range base.MemberAccesses {
			f.ID = prefix + f.ID
			f.CallerID = prefix + f.CallerID
			f.Path = prefix + f.Path
			result.MemberAccesses = append(result.MemberAccesses, f)
		}
	}
	return result
}
func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(value)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func loadGraph(path string) (parser.NavigationGraph, error) {
	var graph parser.NavigationGraph
	f, err := os.Open(path)
	if err != nil {
		return graph, err
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(&graph)
	return graph, err
}
func loadInfo(path string) (datasetInfo, error) {
	var info datasetInfo
	data, err := os.ReadFile(path)
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(data, &info)
	return info, err
}
