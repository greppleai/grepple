package anchors

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alexflint/go-arg"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/usersettings"
)

const anchorDoctorSchema = "grepple-anchor-doctor-v1"

type anchorDoctorArgs struct {
	Provider string `arg:"--provider" placeholder:"NAME" help:"diagnose a named provider instead of the configured default"`
	JSON     bool   `arg:"--json" help:"emit machine-readable diagnostics"`
}

func (anchorDoctorArgs) Description() string {
	return "Validate anchor-provider configuration and perform one temporary-file protocol round trip."
}

type anchorDoctorReport struct {
	Schema           string              `json:"schema"`
	OK               bool                `json:"ok"`
	SettingsPath     string              `json:"settingsPath,omitempty"`
	Provider         string              `json:"provider,omitempty"`
	Command          []string            `json:"command,omitempty"`
	ProtocolVersion  int                 `json:"protocolVersion"`
	TimeoutMS        int                 `json:"timeoutMs,omitempty"`
	MaxResponseBytes int                 `json:"maxResponseBytes"`
	MaxStderrBytes   int                 `json:"maxStderrBytes"`
	ResponseBytes    int                 `json:"responseBytes,omitempty"`
	Checks           []anchorDoctorCheck `json:"checks"`
	Guidance         []string            `json:"guidance,omitempty"`
}

type anchorDoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// WriteHelp writes anchors command help.
func WriteHelp() error {
	return cliruntime.NewOutput(os.Stdout).WriteString("Diagnose or configure user-owned edit-anchor providers.\nUsage:\n  grepple anchors doctor [--provider NAME] [--json]\n  grepple anchors setup --provider NAME --command /ABSOLUTE/PATH [OPTIONS]\n\nRun grepple help anchors doctor or grepple help anchors setup for details.\n")
}

// RunDoctor diagnoses the configured anchor provider.
func RunDoctor(args []string) error {
	values := anchorDoctorArgs{}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple anchors doctor"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	report := diagnoseAnchorProvider(values.Provider)
	var renderErr error
	if values.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		renderErr = encoder.Encode(report)
	} else {
		renderErr = renderAnchorDoctorReport(report)
	}
	if renderErr != nil {
		return renderErr
	}
	if !report.OK {
		return cliruntime.NewExitError(1)
	}
	return nil
}

func diagnoseAnchorProvider(requestedProvider string) anchorDoctorReport {
	report := anchorDoctorReport{
		Schema: anchorDoctorSchema, ProtocolVersion: anchorProtocolVersion,
		MaxResponseBytes: maxAnchorResponseBytes, MaxStderrBytes: maxAnchorProviderStderr,
		Checks: []anchorDoctorCheck{},
	}
	settingsPath, err := usersettings.Path()
	if err != nil {
		return failAnchorDoctor(report, "settings", err.Error())
	}
	report.SettingsPath = settingsPath
	name, provider, err := usersettings.ResolveProvider(requestedProvider)
	if err != nil {
		return failAnchorDoctor(report, "configuration", err.Error())
	}
	report.Provider, report.Command, report.TimeoutMS = name, append([]string(nil), provider.Command...), provider.TimeoutMS
	report.Checks = append(report.Checks, anchorDoctorCheck{Name: "configuration", Status: "ok", Detail: "provider settings are valid"})
	if _, err := exec.LookPath(provider.Command[0]); err != nil {
		return failAnchorDoctor(report, "executable", err.Error())
	}
	report.Checks = append(report.Checks, anchorDoctorCheck{Name: "executable", Status: "ok", Detail: provider.Command[0]})
	responseBytes, err := anchorDoctorRoundTrip(provider)
	if err != nil {
		return failAnchorDoctor(report, "round-trip", err.Error())
	}
	report.ResponseBytes = responseBytes
	report.Checks = append(report.Checks,
		anchorDoctorCheck{Name: "round-trip", Status: "ok", Detail: "temporary-file request and response validated"},
		anchorDoctorCheck{Name: "limits", Status: "ok", Detail: fmt.Sprintf("timeout=%dms response=%d/%d bytes stderr-cap=%d bytes", report.TimeoutMS, responseBytes, report.MaxResponseBytes, report.MaxStderrBytes)},
	)
	report.OK = true
	return report
}

func anchorDoctorRoundTrip(provider usersettings.Provider) (int, error) {
	directory, err := os.MkdirTemp("", "grepple-anchor-doctor-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(directory)
	path := directory + string(os.PathSeparator) + "doctor.go"
	content := "package doctor\n// grepple anchor doctor\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return 0, err
	}
	request := anchorProtocolRequest{ProtocolVersion: anchorProtocolVersion, Files: []anchorProtocolRequestFile{{Path: path, Content: content, SHA256: anchorDigest(content), Lines: []int{2}}}}
	response, err := invokeAnchorProvider(provider, request)
	if err != nil {
		return 0, sanitizeAnchorDoctorError(err, path)
	}
	if _, err := validateAnchorResponse(request, response, map[string]string{path: "<temporary-file>"}); err != nil {
		return 0, sanitizeAnchorDoctorError(err, path)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return 0, err
	}
	return len(encoded), nil
}

func sanitizeAnchorDoctorError(err error, temporaryPath string) error {
	return errors.New(strings.ReplaceAll(err.Error(), temporaryPath, "<temporary-file>"))
}

func failAnchorDoctor(report anchorDoctorReport, check, detail string) anchorDoctorReport {
	report.Checks = append(report.Checks, anchorDoctorCheck{Name: check, Status: "failed", Detail: detail})
	report.Guidance = []string{
		"Configure an absolute executable under anchors.providers in " + usersettings.DisplayPath(report.SettingsPath) + ".",
		"Select it with anchors.default_provider or rerun with --provider NAME.",
		"See docs/anchor-providers.md for protocol-v1 request and response examples.",
	}
	return report
}

func renderAnchorDoctorReport(report anchorDoctorReport) error {
	status := "FAIL"
	if report.OK {
		status = "OK"
	}
	var output strings.Builder
	fmt.Fprintf(&output, "anchor doctor %s protocol=%d\n", status, report.ProtocolVersion)
	if report.SettingsPath != "" {
		fmt.Fprintf(&output, "settings: %s\n", report.SettingsPath)
	}
	if report.Provider != "" {
		fmt.Fprintf(&output, "provider: %s\n", report.Provider)
		fmt.Fprintf(&output, "command: %s\n", quotedAnchorDoctorCommand(report.Command))
	}
	for _, check := range report.Checks {
		fmt.Fprintf(&output, "[%s] %s", check.Status, check.Name)
		if check.Detail != "" {
			fmt.Fprintf(&output, ": %s", check.Detail)
		}
		output.WriteByte('\n')
	}
	for _, guidance := range report.Guidance {
		fmt.Fprintf(&output, "setup: %s\n", guidance)
	}
	return cliruntime.NewOutput(os.Stdout).WriteString(output.String())
}

func quotedAnchorDoctorCommand(command []string) string {
	quoted := make([]string, len(command))
	for index, value := range command {
		quoted[index] = shellquote.Argument(value)
	}
	return strings.Join(quoted, " ")
}
