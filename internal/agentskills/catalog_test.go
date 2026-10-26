package agentskills

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func repositoryBundle(t *testing.T) Bundle {
	t.Helper()
	source, err := NewLocalSource(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
func TestCatalogMatchesRepositoryAndAllSkillDirectories(t *testing.T) {
	bundle := repositoryBundle(t)
	entries, err := os.ReadDir(filepath.Join("..", "..", ".agents", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(bundle.files) {
		t.Fatalf("uncatalogued skill directories: %d vs %d", len(entries), len(bundle.files))
	}
	for _, entry := range entries {
		if _, ok := bundle.files[entry.Name()]; !ok {
			t.Errorf("uncatalogued skill: %s", entry.Name())
		}
	}
	documentation := string(bundle.files["grepple-write"]["SKILL.md"])
	for _, alternative := range []string{"content_lines", "request.json", "references/write.md"} {
		if strings.Contains(documentation, alternative) {
			t.Errorf("write skill documents an alternative to heredoc transactions: %s", alternative)
		}
	}
	if len(bundle.files["grepple-write"]) != 1 {
		t.Fatal("write skill must be self-contained")
	}
}

func TestClaudePluginUsesCanonicalSkills(t *testing.T) {
	root := filepath.Join("..", "..")
	marketplaceData, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil {
		t.Fatal(err)
	}
	var marketplace struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(marketplaceData, &marketplace); err != nil {
		t.Fatal(err)
	}
	if marketplace.Name != "grepple" || len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != "grepple" || marketplace.Plugins[0].Source != "./.agents" {
		t.Fatalf("marketplace must point to the canonical skills root: %+v", marketplace)
	}
	pluginData, err := os.ReadFile(filepath.Join(root, ".agents", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plugin struct {
		Name   string          `json:"name"`
		Skills json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(pluginData, &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin.Name != marketplace.Plugins[0].Name || len(plugin.Skills) != 0 {
		t.Fatalf("plugin must use the default .agents/skills directory: %+v", plugin)
	}
}

func TestClaudePluginHooksAreBundled(t *testing.T) {
	root := filepath.Join("..", "..", ".agents")
	data, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Args []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	for event, script := range map[string]string{
		"SessionStart": "install-cli.mjs", "PreToolUse": "grep-guard.mjs",
	} {
		handlers := config.Hooks[event]
		if len(handlers) != 1 || len(handlers[0].Hooks) != 1 || len(handlers[0].Hooks[0].Args) != 2 || handlers[0].Hooks[0].Args[0] != "${CLAUDE_PLUGIN_ROOT}/scripts/"+script {
			t.Fatalf("%s must run bundled %s: %+v", event, script, handlers)
		}
		if _, err := os.Stat(filepath.Join(root, "scripts", script)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReleaseTagNeverFallsBackToMovingRefs(t *testing.T) {
	for _, version := range []string{"dev", "unknown", "main", "latest", "v0.0.5-dirty", "v0.0.5-3-gabcdef", "0.0.5-3-gabcdef-dirty", "v0.0.5/../main", "v01.2.3"} {
		if _, err := ReleaseTag(version); err == nil {
			t.Errorf("accepted %s", version)
		}
	}
	for _, version := range []string{"0.0.5", "v0.0.5", "v1.2.3-rc.1"} {
		tag, err := ReleaseTag(version)
		if err != nil || tag != "v"+strings.TrimPrefix(version, "v") {
			t.Errorf("%s => %s, %v", version, tag, err)
		}
	}
}
func TestOwnedNamesRetainHistoricalRegistry(t *testing.T) {
	names := OwnedNames()
	owned := map[string]bool{}
	for _, name := range names {
		owned[name] = true
	}
	for _, name := range []string{"architecture-boundary-review", "architecture-diagram-workflow", "architecture-lookup-discovery", "change-impact-analysis", "delegated-research-with-ask", "local-grep-and-search", "remote-grep-and-search", "structural-pattern-audit", "grepple-write"} {
		if !owned[name] {
			t.Errorf("historical name removed: %s", name)
		}
	}
	// Compare every available committed revision and the working tree. A shallow
	// checkout still validates the baseline above; full history guards future edits.
	commits, err := exec.Command("git", "log", "--format=%H", "--", "owned-skills.txt").Output()
	if err != nil {
		return
	}
	for _, commit := range strings.Fields(string(commits)) {
		data, err := exec.Command("git", "show", commit+":"+ownershipPath).Output()
		if err != nil {
			continue
		}
		old := ownershipLines(data)
		if len(names) < len(old) || !reflect.DeepEqual(names[:len(old)], old) {
			t.Fatal("ownership registry is not append-only relative to " + commit)
		}
	}
}
func ownershipLines(data []byte) []string {
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	return names
}

type mapSource map[string][]byte

func (source mapSource) Read(ctx context.Context, file string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := source[file]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}
func testSource(t *testing.T) mapSource {
	t.Helper()
	bundle := repositoryBundle(t)
	source := mapSource{catalogPath: catalogData, ownershipPath: ownershipData}
	for name, files := range bundle.files {
		for file, data := range files {
			source[".agents/skills/"+name+"/"+file] = data
		}
	}
	return source
}
func TestLoadRejectsTamperingAndIncompleteBundle(t *testing.T) {
	for _, file := range []string{catalogPath, ownershipPath, ".agents/skills/grepple-write/SKILL.md"} {
		t.Run(file, func(t *testing.T) {
			source := testSource(t)
			source[file] = []byte("modified")
			if _, err := Load(context.Background(), source); err == nil {
				t.Fatal("tampered source accepted")
			}
		})
	}
	source := testSource(t)
	delete(source, ".agents/skills/grepple-write/SKILL.md")
	if _, err := Load(context.Background(), source); err == nil {
		t.Fatal("missing resource accepted")
	}
}
func TestCatalogValidationRejectsTraversalAndUnownedNames(t *testing.T) {
	for _, file := range []string{"../escape", "/absolute", "a\\b", "C:escape", markerName} {
		if err := validateSkill(skill{Name: "grepple-test", Files: []skillFile{{Path: file, SHA256: strings.Repeat("a", 64)}}}); err == nil {
			t.Errorf("accepted %q", file)
		}
	}
	var catalog catalog
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, s := range catalog.Skills {
		if !strings.HasPrefix(s.Name, "grepple-") {
			t.Errorf("unprefixed skill: %s", s.Name)
		}
	}
}
