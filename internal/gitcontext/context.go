// Package gitcontext discovers canonical repository identities from Git metadata.
package gitcontext

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var repoIDPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Current returns the repository identity for the current working directory.
func Current() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return From(cwd)
}

// From returns the repository identity containing start.
func From(start string) string {
	root := findGitRoot(start)
	if root == "" {
		return ""
	}
	if id := repoIDForRemote(root, "origin"); id != "" {
		return id
	}
	command := exec.Command("git", "-C", root, "remote")
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	for _, remote := range strings.Fields(string(output)) {
		if id := repoIDForRemote(root, remote); id != "" {
			return id
		}
	}
	return ""
}

func repoIDForRemote(root, remote string) string {
	command := exec.Command("git", "-C", root, "remote", "get-url", remote)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return repoIDFromRemote(string(output))
}

func findGitRoot(start string) string {
	directory, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	if info, statErr := os.Stat(directory); statErr == nil && !info.IsDir() {
		directory = filepath.Dir(directory)
	}
	for {
		if _, err := os.Lstat(filepath.Join(directory, ".git")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}

func repoIDFromRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	path := remote
	if parsed, err := url.Parse(remote); err == nil && parsed.Scheme != "" {
		path = parsed.Path
	} else if colon := strings.Index(remote, ":"); colon >= 0 {
		path = remote[colon+1:]
	}
	path = strings.TrimSuffix(strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/"), ".git")
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(parts) < 2 {
		return ""
	}
	owner, name := parts[len(parts)-2], parts[len(parts)-1]
	if !repoIDPart.MatchString(owner) || !repoIDPart.MatchString(name) {
		return ""
	}
	return owner + "/" + name
}
