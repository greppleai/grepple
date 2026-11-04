package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestDestinationMatrix(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	cwd := filepath.Join(t.TempDir(), "project")
	defaults := map[string]string{"grip": ".grip/agent/skills", "pi": ".pi/agent/skills", "claude": ".claude/skills", "opencode": ".config/opencode/skills", "codex": ".agents/skills"}
	projects := map[string]string{"grip": ".grip/skills", "pi": ".pi/skills", "claude": ".claude/skills", "opencode": ".opencode/skills", "codex": ".agents/skills"}
	for agent, path := range defaults {
		actual, err := destination(agent, home, cwd, false, func(string) string { return "" })
		if err != nil || actual != filepath.Join(home, filepath.FromSlash(path)) {
			t.Errorf("%s: %s %v", agent, actual, err)
		}
	}
	for agent, path := range projects {
		actual, err := destination(agent, home, cwd, true, func(string) string { return "" })
		if err != nil || actual != filepath.Join(cwd, filepath.FromSlash(path)) {
			t.Errorf("project %s: %s %v", agent, actual, err)
		}
	}
}
func TestEnvironmentPathsAndAgentValidation(t *testing.T) {
	base := t.TempDir()
	getenv := func(key string) string {
		if key == "GRIP_CODING_AGENT_DIR" || key == "PI_CODING_AGENT_DIR" || key == "XDG_CONFIG_HOME" {
			return base
		}
		return ""
	}
	grip, err := destination("grip", "", "", false, getenv)
	if err != nil || grip != filepath.Join(base, "skills") {
		t.Fatal(grip, err)
	}
	project, err := destination("grip", "", base, true, getenv)
	if err != nil || project != filepath.Join(base, ".grip", "skills") {
		t.Fatal(project, err)
	}
	pi, err := destination("pi", "", "", false, getenv)
	if err != nil || pi != filepath.Join(base, "skills") {
		t.Fatal(pi, err)
	}
	opencode, err := destination("opencode", "", "", false, getenv)
	if err != nil || opencode != filepath.Join(base, "opencode", "skills") {
		t.Fatal(opencode, err)
	}
	for _, values := range []*Args{{}, {Agent: "unknown"}, {Agent: "pi", AgentFlag: "claude"}} {
		if _, err := selectedAgent(values); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	for _, agent := range []string{"pi", "grip"} {
		if _, err = destination(agent, "", "", false, func(string) string { return "relative" }); err == nil || !strings.Contains(err.Error(), strings.ToUpper(agent)+"_CODING_AGENT_DIR") {
			t.Fatalf("relative %s agent directory accepted: %v", agent, err)
		}
	}
}
func TestLocalDevelopmentSetupAndReadOnlyList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	application := cliruntime.Environment{Output: &stdout, ErrorOutput: &stderr}
	if err := Execute(application, &Args{List: true}, "dev"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "delegated-research-with-ask") || !strings.Contains(stdout.String(), "grepple-write") {
		t.Fatal("incomplete registry listing")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "custom-pi"))
	t.Setenv("GRIP_CODING_AGENT_DIR", filepath.Join(home, "custom-grip"))
	source := filepath.Join("..", "..", "..")
	for _, agent := range []string{"grip", "pi", "claude", "opencode", "codex"} {
		if err := Execute(application, &Args{Agent: agent, SourceDir: source}, "dev"); err != nil {
			t.Fatalf("%s: %v", agent, err)
		}
		target, err := destination(agent, home, "", false, os.Getenv)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(filepath.Join(target, "grepple-write", "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGripSkillsDirCommand(t *testing.T) {
	cwd := t.TempDir()
	source, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	application := cliruntime.Environment{
		Output: &stdout, ErrorOutput: &stderr,
		RepositoryContext: cliruntime.RepositoryServices{WorkingDirectoryFunc: func() string { return cwd }},
	}
	cmd := New(application, "dev")
	t.Setenv("GRIP_CODING_AGENT_DIR", "relative-invalid")
	for _, path := range []string{"relative/skills", filepath.Join(t.TempDir(), "host", "skills")} {
		target := path
		if !filepath.IsAbs(target) {
			target = filepath.Join(cwd, target)
		}
		args := []string{"grip", "--skills-dir", path, "--source-dir", source}
		if err := cmd.Run(append(append([]string{}, args...), "--dry-run")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("dry run created destination: %v", err)
		}
		if err := cmd.Run(args); err != nil {
			t.Fatal(err)
		}
		skill := filepath.Join(target, "grepple-write", "SKILL.md")
		if _, err := os.Stat(skill); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Run(args); err != nil {
			t.Fatalf("repeat install: %v", err)
		}
		if err := os.WriteFile(skill, []byte("user changes"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Run(args); err == nil {
			t.Fatal("modified managed skill was overwritten")
		}
		content, err := os.ReadFile(skill)
		if err != nil || string(content) != "user changes" {
			t.Fatalf("modified skill not preserved: %q %v", content, err)
		}
	}
	if err := cmd.Run([]string{"grip", "--project", "--skills-dir", "somewhere", "--source-dir", source}); err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("expected conflicting options error, got %v", err)
	}
}
func TestDevelopmentBuildDoesNotImplicitlyDownloadLatest(t *testing.T) {
	for _, version := range []string{"dev", "v0.0.5-8-gabcdef-dirty"} {
		if _, _, err := selectedSource("", version); err == nil {
			t.Fatal("development source accepted implicitly")
		}
	}
	_, tag, err := selectedSource("", "0.0.5")
	if err != nil || tag != "v0.0.5" {
		t.Fatal(tag, err)
	}
}
