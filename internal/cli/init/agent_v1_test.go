package initcommand

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestInitV1AgentRetriesInvalidResponseAndPreservesReviewedAnnotations(t *testing.T) {
	root, application, _ := initTestContext(t)
	writeInitSource(t, root, "pkg/one.go", "package pkg\n")
	directory := filepath.Join(root, "pkg")
	manifest, err := directorymeta.FilesForDirectoryV1(root, directory, []string{"pkg/one.go"})
	if err != nil {
		t.Fatal(err)
	}
	prior := manifest[0]
	prior.Description, prior.Purpose = "Human reviewed source.", "Implementation"
	prior.DomainAreas = []string{"Human reviewed", "DUPLICATE"}
	if err := directorymeta.WriteDirectoryV1(root, directory, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{prior}}); err != nil {
		t.Fatal(err)
	}
	job := generationJobV1{directory: directory, files: manifest}
	calls := 0
	got, err := generateDirectoryMetadataWithRunnerV1(context.Background(), application, nil, root, job, "Original manifest prompt", func(_ context.Context, request agent.Request) (agent.Result, error) {
		calls++
		if request.SystemPrompt == "" || len(request.Tools) == 0 {
			t.Fatalf("missing read-only tool configuration: %+v", request)
		}
		if calls == 1 {
			return agent.Result{Answer: "files: []"}, nil
		}
		if !strings.Contains(request.Prompt, "model omitted 1 selected files") || !strings.Contains(request.Prompt, "Original manifest prompt") {
			t.Fatalf("missing retry feedback: %q", request.Prompt)
		}
		return agent.Result{Answer: "files:\n- name: one.go\n  checksum: fabricated\n  description: Refreshed from source.\n  purpose: Custom capability\n  domainAreas: [duplicate, New feature]\n  keywords: [c++]"}, nil
	})
	if err != nil || calls != 2 || len(got.Files) != 1 {
		t.Fatalf("generation=%+v calls=%d err=%v", got, calls, err)
	}
	file := got.Files[0]
	if file.Checksum != prior.Checksum || file.Purpose != "custom-capability" || file.Description != "Refreshed from source." || len(file.DomainAreas) != 3 || file.DomainAreas[0] != "duplicate" || file.DomainAreas[1] != "human-reviewed" || file.DomainAreas[2] != "new-feature" || len(file.Keywords) != 1 || file.Keywords[0] != "c++" {
		t.Fatalf("untrusted or missing annotation: %+v", file)
	}
}

func TestInitV1AgentBoundsValidationRetryAndPropagatesRunError(t *testing.T) {
	root, application, _ := initTestContext(t)
	writeInitSource(t, root, "one.go", "package main\n")
	manifest, err := directorymeta.FilesForDirectoryV1(root, root, []string{"one.go"})
	if err != nil {
		t.Fatal(err)
	}
	job := generationJobV1{directory: root, files: manifest}
	calls := 0
	_, err = generateDirectoryMetadataWithRunnerV1(context.Background(), application, nil, root, job, "manifest", func(context.Context, agent.Request) (agent.Result, error) {
		calls++
		return agent.Result{Answer: "files:\n- name: invented.go\n  description: Invalid.\n  purpose: Testing"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected") || calls != 2 {
		t.Fatalf("invalid model reply accepted: calls=%d err=%v", calls, err)
	}
	sentinel := errors.New("model unavailable")
	calls = 0
	_, err = generateDirectoryMetadataWithRunnerV1(context.Background(), application, nil, root, job, "manifest", func(context.Context, agent.Request) (agent.Result, error) {
		calls++
		return agent.Result{}, sentinel
	})
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("run error retried or swallowed: calls=%d err=%v", calls, err)
	}
	if _, err := os.Stat(directorymeta.RepositoryPath(root)); !os.IsNotExist(err) {
		t.Fatalf("agent wrote snapshot: %v", err)
	}
}
