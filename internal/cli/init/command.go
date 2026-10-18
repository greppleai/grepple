// Package initcommand generates repository-owned directory metadata.
package initcommand

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

type Args struct {
	Force         bool     `arg:"--force" help:"regenerate all selected directory entries, including current ones"`
	Concurrency   int      `arg:"--concurrency" default:"1" placeholder:"N" help:"generate metadata for N directories in parallel (default: 1)"`
	OnlyDirectory string   `arg:"--only-directory" placeholder:"PATH" help:"generate metadata for exactly one directory without its ancestors"`
	Paths         []string `arg:"positional" placeholder:"PATH" help:"source path or glob; defaults to the repository"`
}

// Execute generates directory metadata from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	if values.Concurrency < 1 {
		return fmt.Errorf("--concurrency must be at least 1")
	}
	ctx := context.Background()
	root := application.Repository().WorkingDirectory()
	plan, err := planGenerationV1(ctx, application, values.Paths, values.Force, values.OnlyDirectory)
	if err != nil {
		return err
	}
	if !plan.needsGeneration() {
		return runGenerationV1(ctx, application, root, plan, values.Concurrency, nil)
	}
	provider, modelID, err := configuredModel(ctx, application.Configuration())
	if err != nil {
		return err
	}
	model, err := provider.LanguageModel(ctx, modelID)
	if err != nil {
		return err
	}
	return runGenerationV1(ctx, application, root, plan, values.Concurrency, func(ctx context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
		prompt, err := generationPromptV1(root, job.directory, job.files)
		if err != nil {
			return directorymeta.DirectoryV1{}, err
		}
		return generateDirectoryMetadataV1(ctx, application, model, root, job, prompt)
	})
}

func configuredModel(ctx context.Context, settings cliruntime.Configuration) (aiprovider.Provider, string, error) {
	preferences, err := settings.AskPreferences()
	if err != nil {
		return nil, "", err
	}
	providerName, modelID, err := aiprovider.ResolveSelection("", "", preferences.Model)
	if err != nil {
		return nil, "", err
	}
	store, err := aiprovider.NewStore()
	if err != nil {
		return nil, "", err
	}
	provider, err := aiprovider.NewRegistry(store, &http.Client{Timeout: 5 * time.Minute}).Provider(providerName)
	if err != nil {
		return nil, "", err
	}
	if modelID == "" {
		modelID = provider.DefaultModel()
	}
	_ = ctx
	return provider, modelID, nil
}
func confinedDirectory(root, value string) (string, error) {
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(value))
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(resolvedRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("directory %q is outside the repository", value)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", value)
	}
	return path, nil
}

func displayPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	if relative == "." {
		return "."
	}
	return filepath.ToSlash(relative)
}
