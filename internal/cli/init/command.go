// Package initcommand generates repository-owned directory metadata.
package initcommand

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/usersettings"
	"go.yaml.in/yaml/v3"
)

type Args struct {
	Force         bool     `arg:"--force" help:"replace existing grepple.yaml files"`
	OnlyDirectory string   `arg:"--only-directory" placeholder:"PATH" help:"generate metadata for exactly one directory without its ancestors"`
	Paths         []string `arg:"positional" placeholder:"PATH" help:"source path or glob; defaults to the repository"`
}

type command struct{ context cliruntime.Context }

func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

func (command *command) Run(args []string) error {
	values := Args{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple init"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(command.context.Stdout())
			return nil
		}
		return err
	}
	return Execute(command.context, &values)
}

// Execute generates directory metadata from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	provider, modelID, err := configuredModel(context.Background())
	if err != nil {
		return err
	}
	model, err := provider.LanguageModel(context.Background(), modelID)
	if err != nil {
		return err
	}
	return generate(context.Background(), application, model, values.Paths, values.Force, values.OnlyDirectory)
}

func generate(ctx context.Context, application cliruntime.Context, model fantasy.LanguageModel, globs []string, force bool, onlyDirectory string) error {
	root := application.Repository().WorkingDirectory()
	var exactDirectory string
	if onlyDirectory != "" && len(globs) != 0 {
		return fmt.Errorf("--only-directory cannot be combined with positional paths")
	}
	if onlyDirectory != "" {
		resolved, err := confinedDirectory(root, onlyDirectory)
		if err != nil {
			return err
		}
		exactDirectory = resolved
		globs = nil
	}
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return err
	}
	paths, err := sourcedomain.ListWithPolicy(ctx, globs, root, policy)
	if err != nil {
		return err
	}
	if exactDirectory != "" {
		paths = pathsWithinDirectory(root, exactDirectory, paths)
	}
	var directories []string
	if onlyDirectory != "" {
		directories = []string{exactDirectory}
	} else {
		directories, err = directorymeta.Directories(root, paths)
		if err != nil {
			return err
		}
	}
	for _, directory := range directories {
		metadataPath := filepath.Join(directory, directorymeta.FileName)
		if !force {
			if _, statErr := os.Stat(metadataPath); statErr == nil {
				fmt.Fprintln(application.Stdout(), "skip", displayPath(root, directory))
				continue
			}
		}
		files, err := directorymeta.FilesForDirectory(root, directory, paths)
		if err != nil {
			return err
		}
		prompt, err := generationPrompt(root, directory, files)
		if err != nil {
			return err
		}
		metadata, err := generateDirectoryMetadata(ctx, application, model, root, directory, prompt, files)
		if err != nil {
			return fmt.Errorf("generate %s: %w", displayPath(root, directory), err)
		}
		if err := directorymeta.Write(directory, metadata); err != nil {
			return err
		}
		fmt.Fprintln(application.Stdout(), "write", filepath.ToSlash(filepath.Join(displayPath(root, directory), directorymeta.FileName)))
	}
	return nil
}

func generateDirectoryMetadata(ctx context.Context, application cliruntime.Context, model fantasy.LanguageModel, root, directory, prompt string, files []directorymeta.File) (directorymeta.Metadata, error) {
	var validationErr error
	for attempt := 0; attempt < 2; attempt++ {
		result, err := agent.Run(ctx, nil, agent.Request{
			Name:         "directory metadata",
			Prompt:       prompt,
			SystemPrompt: initSystemPrompt(root, directory),
			ModelFactory: func(context.Context) (fantasy.LanguageModel, error) { return model, nil },
			Tools:        initTools(application, root, directory),
		})
		if err != nil {
			return directorymeta.Metadata{}, err
		}
		metadata, err := parseGeneratedMetadata(result.Answer, files)
		if err == nil {
			return metadata, nil
		}
		validationErr = err
		prompt = fmt.Sprintf("%s\n\nYour previous YAML was invalid: %s. Correct it and return complete YAML only. Previous response:\n%s", prompt, err, result.Answer)
	}
	return directorymeta.Metadata{}, validationErr
}
func generationPrompt(root, directory string, files []directorymeta.File) (string, error) {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Generate grepple.yaml for directory %q. Inspect the directory with the available Grepple tools before answering. Return YAML only with description, responsibilities, and files. Each description is one concise sentence. Responsibilities are concise action phrases. Classify every file with exactly one kind: production, test, fixture, generated, vendor, or unknown. Use unknown only when source evidence cannot support another classification. Preserve every supplied file path and checksum exactly. Include every supplied file.\n\n", displayPath(root, directory))
	if existing, err := os.ReadFile(filepath.Join(directory, directorymeta.FileName)); err == nil {
		fmt.Fprintf(&prompt, "Existing grepple.yaml to use as prior context and improve:\n%s\nEND EXISTING METADATA\n\n", existing)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	prompt.WriteString("Authoritative file manifest:\n")
	for _, file := range files {
		fmt.Fprintf(&prompt, "- path: %s\n  checksum: %s\n", file.Path, file.Checksum)
	}
	return prompt.String(), nil
}

func parseGeneratedMetadata(value string, files []directorymeta.File) (directorymeta.Metadata, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```yaml")
	value = strings.TrimPrefix(value, "```yml")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(strings.TrimSpace(value), "```")
	var generated directorymeta.Metadata
	if err := yaml.Unmarshal([]byte(value), &generated); err != nil {
		return generated, err
	}
	if strings.TrimSpace(generated.Description) == "" {
		return generated, fmt.Errorf("model returned an empty directory description")
	}
	if len(generated.Responsibilities) == 0 {
		return generated, fmt.Errorf("model returned no directory responsibilities")
	}
	type generatedFile struct {
		description string
		kind        sourcedomain.Kind
	}
	generatedFiles := map[string]generatedFile{}
	for _, file := range generated.Files {
		description := strings.TrimSpace(file.Description)
		if strings.TrimSpace(file.Kind) == "" {
			return generated, fmt.Errorf("model returned no source kind for %s", file.Path)
		}
		kind, valid := sourcedomain.ParseKind(file.Kind)
		if !valid {
			return generated, fmt.Errorf("model returned invalid source kind %q for %s", file.Kind, file.Path)
		}
		value := generatedFile{description: description, kind: kind}
		generatedFiles[filepath.ToSlash(file.Path)] = value
		generatedFiles[filepath.Base(filepath.FromSlash(file.Path))] = value
	}
	generated.Files = append([]directorymeta.File(nil), files...)
	for index := range generated.Files {
		classified := generatedFiles[generated.Files[index].Path]
		generated.Files[index].Description = classified.description
		generated.Files[index].Kind = string(classified.kind)
		if generated.Files[index].Description == "" {
			return generated, fmt.Errorf("model returned no description for %s", generated.Files[index].Path)
		}
	}
	return generated, nil
}

func configuredModel(ctx context.Context) (aiprovider.Provider, string, error) {
	preferences, err := usersettings.LoadAskPreferences()
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
