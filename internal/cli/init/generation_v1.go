package initcommand

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"go.yaml.in/yaml/v3"
)

// generationPromptV1 gives the agent a manifest it must annotate rather than
// allowing a response to define the source scope or its checksums.
func generationPromptV1(root, directory string, files []directorymeta.FileV1) (string, error) {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Annotate only these selected direct source files in %q. Inspect their content using the read-only tools before answering. Return YAML with exactly one top-level files list, containing an entry for every manifest name. Each entry must have name, a concise source-grounded description, and a purpose; also add relevant domainAreas, functionalAreas and keywords when supported by source evidence. Suggested purposes are documentation, implementation, testing, configuration, ci-cd, agent-skills, examples and fixture; grounded custom purposes are allowed. Pick a file's primary role: ci-cd for pipeline/workflow definitions, agent-skills for agent skill or instruction files, and implementation for code implementing either feature. Use domainAreas for the feature or subject and functionalAreas for the capability or workflow. Reuse coherent categories across files. Keep labels short. Write purposes, domain areas, functional areas and keywords in lowercase, replacing spaces with hyphens (for example source-parsing, c++). Do not include directory descriptions, responsibilities, kinds or area proposals. Never invent, omit or rename files. Checksums are supplied by Grepple; you may omit them in your response.\n\n", displayPath(root, directory))
	if existing, err := directorymeta.ReadDocumentV1(root); err == nil {
		key, keyErr := directorymeta.DirectoryKey(root, directory)
		if keyErr != nil {
			return "", keyErr
		}
		if entry, ok := existing.Directory[key]; ok {
			content, marshalErr := yaml.Marshal(entry)
			if marshalErr != nil {
				return "", marshalErr
			}
			fmt.Fprintf(&prompt, "Previously reviewed file annotations (context only; recheck changed files):\n%s\n", content)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	prompt.WriteString("Authoritative selected file manifest:\n")
	for _, file := range files {
		fmt.Fprintf(&prompt, "- name: %q\n  checksum: %q\n", file.Name, file.Checksum)
	}
	return prompt.String(), nil
}

// parseGeneratedV1 is deliberately strict about model-controlled membership.
// Model-supplied checksums are never trusted; only the selected manifest's
// checksums are persisted. Current hand-edited labels survive a refresh.
func parseGeneratedV1(answer string, manifest []directorymeta.FileV1, existing directorymeta.DirectoryV1) (directorymeta.DirectoryV1, error) {
	value := strings.TrimSpace(answer)
	if strings.HasPrefix(value, "```") {
		first, remaining, found := strings.Cut(value, "\n")
		if !found || (first != "```" && first != "```yaml" && first != "```yml") || !strings.HasSuffix(remaining, "```") {
			return directorymeta.DirectoryV1{}, errors.New("invalid fenced YAML response")
		}
		value = strings.TrimSpace(strings.TrimSuffix(remaining, "```"))
	}
	decoder := yaml.NewDecoder(strings.NewReader(value))
	decoder.KnownFields(true)
	var generated directorymeta.DirectoryV1
	if err := decoder.Decode(&generated); err != nil {
		return directorymeta.DirectoryV1{}, fmt.Errorf("parse generated files: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return directorymeta.DirectoryV1{}, errors.New("multiple generated YAML documents")
		}
		return directorymeta.DirectoryV1{}, err
	}
	selected := make(map[string]directorymeta.FileV1, len(manifest))
	for _, file := range manifest {
		if file.Name == "" || selected[file.Name].Name != "" {
			return directorymeta.DirectoryV1{}, fmt.Errorf("invalid or duplicate selected file %q", file.Name)
		}
		selected[file.Name] = file
	}
	prior := make(map[string]directorymeta.FileV1, len(existing.Files))
	for _, file := range existing.Files {
		prior[file.Name] = file
	}
	result := directorymeta.DirectoryV1{Files: make([]directorymeta.FileV1, 0, len(manifest))}
	seen := map[string]bool{}
	for _, file := range generated.Files {
		authoritative, ok := selected[file.Name]
		if !ok || seen[file.Name] {
			return directorymeta.DirectoryV1{}, fmt.Errorf("unexpected or duplicate generated file %q", file.Name)
		}
		seen[file.Name] = true
		file.Checksum = authoritative.Checksum
		if previous, ok := prior[file.Name]; ok && previous.Checksum == file.Checksum {
			file.DomainAreas = append(file.DomainAreas, previous.DomainAreas...)
			file.FunctionalAreas = append(file.FunctionalAreas, previous.FunctionalAreas...)
			file.Keywords = append(file.Keywords, previous.Keywords...)
		}
		result.Files = append(result.Files, file)
	}
	if len(seen) != len(selected) {
		return directorymeta.DirectoryV1{}, fmt.Errorf("model omitted %d selected files", len(selected)-len(seen))
	}
	document := directorymeta.DocumentV1{Schema: "v1", Directory: map[string]directorymeta.DirectoryV1{".": result}}
	if err := document.Normalize(); err != nil {
		return directorymeta.DirectoryV1{}, fmt.Errorf("invalid generated annotation: %w", err)
	}
	return document.Directory["."], nil
}

// The system prompt requests only V1 file annotations, never obsolete directory fields.
func initSystemPromptV1(root, directory string) string {
	var out bytes.Buffer
	fmt.Fprintf(&out, "You document selected files for Grepple. Repository root: %q. Directory: %q. Use directory_tree first, then grep_code and read_file as needed; tools are read-only and confined to this directory. Every file annotation must be grounded in source behavior. Reply only with YAML containing a files list with name, description, purpose and optional domainAreas, functionalAreas and keywords. Do not infer metadata from names alone.\n", root, directory)
	return out.String()
}
