package cli

import (
	"strings"
	"testing"
)

func TestSetupCommandParsingAndHelp(t *testing.T) {
	for _, agent := range []string{"grip", "pi", "claude", "opencode", "codex"} {
		values, help, err := parseApplicationArgs([]string{"setup", agent, "--dry-run", "--project", "--source-dir", "."})
		if err != nil || help || values.Setup == nil || values.Setup.Agent != agent || !values.Setup.DryRun || !values.Setup.Project {
			t.Fatalf("%s parse: %#v %v", agent, values, err)
		}
	}
	values, help, err := parseApplicationArgs([]string{"setup", "grip", "--skills-dir", "/host/agent/skills"})
	if err != nil || help || values.Setup == nil || values.Setup.SkillsDir != "/host/agent/skills" {
		t.Fatalf("skills-dir parse: %#v %v", values, err)
	}
	text := captureStdout(t, func() {
		if err := Run([]string{"setup", "--help"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(text, "grip") || !strings.Contains(text, "codex") || !strings.Contains(text, "--source-dir") || !strings.Contains(text, "--skills-dir") {
		t.Fatal("incomplete setup help", text)
	}
	text = captureStdout(t, func() {
		if err := Run([]string{"setup", "--list"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(text, "grepple-write") {
		t.Fatal("setup dispatch failed")
	}
}
