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

	"charm.land/fantasy"
	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/usersettings"
	"go.yaml.in/yaml/v3"
)

type Args struct {
	Force         bool     `arg:"--force" help:"replace all existing grepple.yaml files, including current ones"`
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
	plan, err := planGeneration(ctx, application, values.Paths, values.Force, values.OnlyDirectory)
	if err != nil {
		return err
	}
	if !plan.needsGeneration() {
		return runGeneration(ctx, application, root, plan, values.Concurrency, nil)
	}
	inventory, err := areaInventory(ctx, application, root)
	if err != nil {
		return err
	}
	provider, modelID, err := configuredModel(ctx)
	if err != nil {
		return err
	}
	model, err := provider.LanguageModel(ctx, modelID)
	if err != nil {
		return err
	}
	prompts := areaPrompts(application, root, values.Concurrency, inventory)
	return runGeneration(ctx, application, root, plan, values.Concurrency, func(ctx context.Context, job generationJob) (directorymeta.Metadata, error) {
		prompt, err := prompts(ctx, job)
		if err != nil {
			return directorymeta.Metadata{}, err
		}
		return generateDirectoryMetadata(ctx, application, model, root, job.directory, prompt, job.files)
	})
}

func generateDirectoryMetadata(ctx context.Context, application cliruntime.Context, model fantasy.LanguageModel, root, directory, prompt string, files []directorymeta.File) (directorymeta.Metadata, error) {
	var validationErr error
	existing, _ := directorymeta.Read(directory) // Invalid prior YAML cannot be trusted as membership.
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
		metadata, err := parseGeneratedMetadataWithExisting(result.Answer, files, existing)
		if err == nil {
			return metadata, nil
		}
		validationErr = err
		prompt = fmt.Sprintf("%s\n\nYour previous YAML was invalid: %s. Correct it and return complete YAML only. Previous response:\n%s", prompt, err, result.Answer)
	}
	return directorymeta.Metadata{}, validationErr
}
func generationPrompt(root, directory string, files []directorymeta.File) (string, error) {
	return generationPromptWithAreas(root, directory, files, nil)
}

func generationPromptWithAreas(root, directory string, files []directorymeta.File, inventory []directorymeta.AreaReference) (string, error) {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Generate grepple.yaml for directory %q. Inspect the directory with the available Grepple tools before answering. Return YAML only with description, responsibilities, and files. Each description is one concise sentence. Responsibilities are concise action phrases. Classify every file with exactly one kind: production, test, fixture, generated, vendor, or unknown. Use unknown only when source evidence cannot support another classification. Preserve every supplied file path and checksum exactly. Include every supplied file. Independently discover cohesive feature areas from source behavior and tests, especially when the repository inventory is empty: look for concrete capabilities or workflows and propose concise, reusable lowercase-hyphenated area names based on evidence, not a preset vocabulary. Tag the files that implement or test each supported area, not every file in the directory. Reuse an inventory area only when local source evidence supports it; do not invent tags from directory or file names alone. If no meaningful feature area is evidenced, leave areas empty. Retain existing file areas even if no local call edge appears; they are hand-authored semantic hints. For every new area on each file include a matching area_proposals entry (path, area, action: add, evidence with an actual selected filename and positive line number such as example.go:12, plus reason); propose removals with action: remove and evidence instead of deleting tags. Never use the literal placeholder SOURCE:LINE or SOURCE:12 as a citation. Area tags use lowercase letters, digits and hyphens. Never infer an area solely from its name or from missing call edges. area_proposals are printed for review but not written to grepple.yaml.\n\n", displayPath(root, directory))
	if existing, err := os.ReadFile(filepath.Join(directory, directorymeta.FileName)); err == nil {
		fmt.Fprintf(&prompt, "Existing grepple.yaml to use as prior context and improve:\n%s\nEND EXISTING METADATA\n\n", existing)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	prompt.WriteString("Repository-wide existing area tags (repository metadata hints, not proof; stale entries need review):\n")
	if len(inventory) == 0 {
		prompt.WriteString("(none)\n")
	}
	for _, ref := range inventory {
		fmt.Fprintf(&prompt, "- %s %s [%s, %s]: %s\n", ref.Area, ref.Path, ref.Kind, ref.Status, ref.Description)
	}
	prompt.WriteString("\nAuthoritative file manifest:\n")
	for _, file := range files {
		fmt.Fprintf(&prompt, "- path: %s\n  checksum: %s\n", file.Path, file.Checksum)
	}
	return prompt.String(), nil
}

func parseGeneratedMetadata(value string, files []directorymeta.File) (directorymeta.Metadata, error) {
	return parseGeneratedMetadataWithExisting(value, files, directorymeta.Metadata{})
}

func parseGeneratedMetadataWithExisting(value string, files []directorymeta.File, existing directorymeta.Metadata) (directorymeta.Metadata, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```yaml")
	value = strings.TrimPrefix(value, "```yml")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(strings.TrimSpace(value), "```")
	var response struct {
		directorymeta.Metadata `yaml:",inline"`
		AreaProposals          []directorymeta.AreaProposal `yaml:"area_proposals"`
	}
	if err := yaml.Unmarshal([]byte(value), &response); err != nil {
		return directorymeta.Metadata{}, err
	}
	generated := response.Metadata
	if strings.TrimSpace(generated.Description) == "" || len(generated.Responsibilities) == 0 {
		return generated, fmt.Errorf("model returned no directory description or responsibilities")
	}
	manifest := make(map[string]bool, len(files))
	for _, file := range files {
		manifest[file.Path] = true
	}
	proposals := map[string]bool{}
	for _, proposal := range response.AreaProposals {
		if !manifest[proposal.Path] || !directorymeta.ValidArea(proposal.Area) || (proposal.Action != "add" && proposal.Action != "remove") || !hasLocalSourceCitation(proposal.Evidence, files) {
			return generated, fmt.Errorf("invalid area proposal for %q (area=%q action=%q evidence=%q): selected path, area, action and local filename:line evidence (e.g. example.go:12) are required", proposal.Path, proposal.Area, proposal.Action, proposal.Evidence)
		}
		proposals[proposal.Path+"\x00"+proposal.Area+"\x00"+proposal.Action] = true
	}
	existingAreas := map[string][]string{}
	for _, file := range existing.Files {
		if manifest[file.Path] && len(directorymeta.ValidateAreas(file.Areas)) == 0 {
			existingAreas[file.Path] = file.Areas
		}
	}
	generatedFiles := map[string]directorymeta.File{}
	for _, file := range generated.Files {
		if !manifest[file.Path] || generatedFiles[file.Path].Path != "" {
			return generated, fmt.Errorf("model returned unexpected or duplicate file %q", file.Path)
		}
		if _, valid := sourcedomain.ParseKind(file.Kind); !valid || strings.TrimSpace(file.Kind) == "" {
			return generated, fmt.Errorf("model returned invalid source kind %q for %s", file.Kind, file.Path)
		}
		if strings.TrimSpace(file.Description) == "" {
			return generated, fmt.Errorf("model returned no description for %s", file.Path)
		}
		if issues := directorymeta.ValidateAreas(file.Areas); len(issues) != 0 {
			return generated, fmt.Errorf("model returned %s for %s", issues[0], file.Path)
		}
		generatedFiles[file.Path] = file
	}
	generated.Files = append([]directorymeta.File(nil), files...)
	for index := range generated.Files {
		file := &generated.Files[index]
		classified, ok := generatedFiles[file.Path]
		if !ok {
			return generated, fmt.Errorf("model omitted %s", file.Path)
		}
		file.Description = strings.TrimSpace(classified.Description)
		file.Kind = classified.Kind
		seen := map[string]bool{}
		for _, area := range existingAreas[file.Path] {
			file.Areas = append(file.Areas, area)
			seen[area] = true
		}
		for _, area := range classified.Areas {
			if seen[area] {
				continue
			}
			if !proposals[file.Path+"\x00"+area+"\x00add"] {
				return generated, fmt.Errorf("new area %q for %s lacks an add proposal with source evidence", area, file.Path)
			}
			file.Areas = append(file.Areas, area)
			seen[area] = true
		}
	}
	generated.AreaProposals = response.AreaProposals
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
