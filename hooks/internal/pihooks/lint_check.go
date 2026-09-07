package pihooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	maxReportBytes   = 8 * 1024 * 1024
	maxFeedbackBytes = 9_000
	reviveVersion    = "v1.16.0"
)

type commandSpec struct {
	command string
	args    []string
}

type commandResult struct {
	stdout   string
	stderr   string
	code     int
	spawnErr error
	overflow bool
}

type boundedBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		if remaining > len(data) {
			remaining = len(data)
		}
		_, _ = buffer.buffer.Write(data[:remaining])
	}
	if remaining < len(data) {
		buffer.overflow = true
	}
	return len(data), nil
}

func (buffer *boundedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func (buffer *boundedBuffer) Overflowed() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.overflow
}

func runCommand(spec commandSpec, cwd string) commandResult {
	command := exec.Command(spec.command, spec.args...)
	command.Dir = cwd
	command.Env = os.Environ()
	stdout := &boundedBuffer{limit: maxReportBytes}
	stderr := &boundedBuffer{limit: maxReportBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := commandResult{stdout: stdout.String(), stderr: stderr.String(), overflow: stdout.Overflowed() || stderr.Overflowed()}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.code = exitError.ExitCode()
		return result
	}
	result.code = -1
	result.spawnErr = err
	return result
}

// HandleLintHook executes the Stop-hook lint pipeline and returns structured
// hook output. Empty output means the event needs no continuation.
func HandleLintHook(input []byte, hookRoot string) []byte {
	cwd, ok := stopEventCWD(input)
	if !ok || !isDirectory(cwd) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
		return handleGoLintHook(cwd, hookRoot)
	}
	return handleMermaidOnlyHook(cwd, hookRoot)
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func handleGoLintHook(cwd, hookRoot string) []byte {
	autoFixed := gofmtAutofix(cwd)
	revive, ok := resolveRevive(cwd)
	if !ok {
		return marshalStopFeedback("revive is not installed and could not be resolved via `go run`. Install it with:\n\n  go install github.com/mgechev/revive@latest\n\n(no network fetch is needed once the module cache is warm).", nil)
	}
	packages, err := goPackages(cwd)
	if err != nil {
		return marshalStopFeedback(err.Error(), nil)
	}
	if len(packages) == 0 {
		return marshalStopFeedback("go list ./... returned no lintable packages.", nil)
	}
	args := append(append([]string{}, revive.args...), "-formatter", "json", "-config", filepath.Join(cwd, "revive.toml"))
	result := runCommand(commandSpec{command: revive.command, args: append(args, packages...)}, cwd)
	return evaluateLintResult(result, cwd, hookRoot, autoFixed)
}

func handleMermaidOnlyHook(cwd, hookRoot string) []byte {
	diagnostics, err := AnalyzeMermaidSchemas(cwd)
	if err != nil {
		return marshalStopFeedback("The Mermaid code schema check failed: "+err.Error(), nil)
	}
	if len(diagnostics) == 0 {
		return nil
	}
	return renderDiagnosticFeedback(diagnostics, hookRoot, nil)
}

func renderDiagnosticFeedback(diagnostics []Diagnostic, hookRoot string, autoFixed []string) []byte {
	group := SelectNextDiagnosticGroup(diagnostics)
	guide, err := LoadGuideForGroup(hookRoot, group)
	if err != nil {
		return marshalStopFeedback("lint remediation guide could not be loaded: "+err.Error(), nil)
	}
	progress := len(diagnostics)
	return marshalStopFeedback(FormatDiagnosticFeedback(group, progress, guide, autoFixed), &progress)
}

func stopEventCWD(input []byte) (string, bool) {
	var event struct {
		HookEventName string `json:"hook_event_name"`
		CWD           string `json:"cwd"`
	}
	if json.Unmarshal(input, &event) != nil || event.HookEventName != "Stop" || event.CWD == "" {
		return "", false
	}
	return event.CWD, true
}

func evaluateLintResult(result commandResult, cwd, hookRoot string, autoFixed []string) []byte {
	if result.overflow {
		return marshalStopFeedback("revive JSON output exceeded 8 MiB. Run `revive -config revive.toml ./...` manually and reduce the diagnostic set.", nil)
	}
	if result.spawnErr != nil {
		return marshalStopFeedback("revive could not run: "+result.spawnErr.Error(), nil)
	}
	diagnostics, err := ParseReviveReport(result.stdout)
	if err != nil {
		return marshalStopFeedback(withStderr("revive failed, but its JSON report could not be parsed: "+err.Error(), result.stderr), nil)
	}
	custom, err := AnalyzeRepository(cwd)
	if err != nil {
		return marshalStopFeedback("The same-file-struct-methods check failed: "+err.Error(), nil)
	}
	diagnostics = append(diagnostics, custom...)
	mermaidDiagnostics, err := AnalyzeMermaidSchemas(cwd)
	if err != nil {
		return marshalStopFeedback("The Mermaid code schema check failed: "+err.Error(), nil)
	}
	diagnostics = append(diagnostics, mermaidDiagnostics...)
	group := SelectNextDiagnosticGroup(diagnostics)
	if len(group) == 0 {
		return cleanLintFeedback(result, autoFixed)
	}
	guide, err := LoadGuideForGroup(hookRoot, group)
	if err != nil {
		return marshalStopFeedback("lint remediation guide could not be loaded: "+err.Error(), nil)
	}
	progress := len(diagnostics)
	return marshalStopFeedback(FormatDiagnosticFeedback(group, progress, guide, autoFixed), &progress)
}

func cleanLintFeedback(result commandResult, autoFixed []string) []byte {
	if result.code != 0 {
		return marshalStopFeedback(withStderr("revive failed without a diagnostic.", result.stderr), nil)
	}
	return autoFixedFeedback(autoFixed)
}

func withStderr(message, stderr string) string {
	details := truncateUTF8(strings.TrimSpace(stderr), maxFeedbackBytes)
	if details == "" {
		return message
	}
	return message + "\n\n" + details
}

func autoFixedFeedback(files []string) []byte {
	if len(files) == 0 {
		return nil
	}
	message := fmt.Sprintf("gofmt auto-formatted %d file(s); no lint issues remain:\n- %s\n\nIf you still have checks or edits that read these files, re-run them against the formatted content.", len(files), strings.Join(files, "\n- "))
	return marshalStopFeedback(message, nil)
}

func marshalStopFeedback(additionalContext string, progress *int) []byte {
	type specific struct {
		HookEventName        string `json:"hookEventName"`
		AdditionalContext    string `json:"additionalContext"`
		ContinuationProgress *int   `json:"continuationProgress,omitempty"`
	}
	output := struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{HookSpecificOutput: specific{
		HookEventName:        "Stop",
		AdditionalContext:    truncateUTF8(additionalContext, maxFeedbackBytes),
		ContinuationProgress: progress,
	}}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil
	}
	return encoded
}

func gofmtAutofix(cwd string) []string {
	listed := runCommand(commandSpec{command: "gofmt", args: []string{"-l", "."}}, cwd)
	if listed.spawnErr != nil || listed.code != 0 {
		return nil
	}
	files := nonEmptyLines(listed.stdout)
	if len(files) == 0 {
		return nil
	}
	written := runCommand(commandSpec{command: "gofmt", args: append([]string{"-w"}, files...)}, cwd)
	if written.spawnErr != nil || written.code != 0 {
		return nil
	}
	return files
}

func goPackages(cwd string) ([]string, error) {
	listed := runCommand(commandSpec{command: "go", args: []string{"list", "./..."}}, cwd)
	if listed.spawnErr != nil {
		return nil, fmt.Errorf("go list ./... could not run: %w", listed.spawnErr)
	}
	if listed.code != 0 {
		return nil, fmt.Errorf("go list ./... failed%s", stderrSuffix(listed.stderr))
	}
	packages := nonEmptyLines(listed.stdout)
	result := packages[:0]
	for _, packageName := range packages {
		if !strings.Contains(packageName, "/examples/") {
			result = append(result, packageName)
		}
	}
	return result, nil
}

func stderrSuffix(stderr string) string {
	details := truncateUTF8(strings.TrimSpace(stderr), maxFeedbackBytes)
	if details == "" {
		return "."
	}
	return ":\n\n" + details
}

func resolveRevive(cwd string) (commandSpec, bool) {
	onPath := runCommand(commandSpec{command: "revive", args: []string{"-version"}}, cwd)
	if onPath.spawnErr == nil && onPath.code == 0 {
		return commandSpec{command: "revive"}, true
	}
	goPath := runCommand(commandSpec{command: "go", args: []string{"env", "GOPATH"}}, cwd)
	if goPath.spawnErr == nil && goPath.code == 0 {
		name := "revive"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		candidate := filepath.Join(strings.TrimSpace(goPath.stdout), "bin", name)
		if _, err := os.Stat(candidate); err == nil {
			return commandSpec{command: candidate}, true
		}
	}
	module := "github.com/mgechev/revive@" + reviveVersion
	viaGoRun := runCommand(commandSpec{command: "go", args: []string{"run", module, "-version"}}, cwd)
	if viaGoRun.spawnErr == nil && viaGoRun.code == 0 {
		return commandSpec{command: "go", args: []string{"run", module}}, true
	}
	return commandSpec{}, false
}

func nonEmptyLines(value string) []string {
	var result []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
