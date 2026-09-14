package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
)

type anchorSetupArgs struct {
	Provider        string   `arg:"--provider,required" placeholder:"NAME" help:"provider name to add or update"`
	Command         string   `arg:"--command,required" placeholder:"ABSOLUTE_PATH" help:"absolute provider executable path"`
	CommandArgs     []string `arg:"--command-arg,separate" placeholder:"ARG" help:"append one literal executable argument; repeatable"`
	TimeoutMS       int      `arg:"--timeout-ms" placeholder:"N" help:"provider timeout in milliseconds (default 5000)"`
	SetDefault      bool     `arg:"--set-default" help:"select this provider as the user default"`
	EnableByDefault bool     `arg:"--enable-by-default" help:"anchor eligible searches by default; requires this provider to be default"`
	Write           bool     `arg:"--write" help:"atomically write the previewed user settings"`
	Force           bool     `arg:"--force" help:"replace an existing provider with different settings; requires --write"`
}

func (anchorSetupArgs) Description() string {
	return "Preview or explicitly write a user-owned anchor provider. Without --write, settings are printed and the filesystem is unchanged."
}

func runAnchorsSetup(args []string) error {
	values := anchorSetupArgs{}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple anchors setup"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			fmt.Fprintln(os.Stdout, "Safety: preview is the default; --write is required to change user settings, and --force is required to replace a different provider.")
			return nil
		}
		return err
	}
	settings, settingsPath, _, err := prepareAnchorSetup(values)
	if err != nil {
		return err
	}
	if !values.Write {
		return renderAnchorSetupPreview(settings, settingsPath, values.Provider)
	}
	if err := writeUserSettingsAtomic(settingsPath, settings); err != nil {
		return err
	}
	message := fmt.Sprintf("anchor setup written settings=%s provider=%s\nnext: grepple anchors doctor --provider %s\n", settingsPath, values.Provider, quoteCommandArgument(values.Provider))
	return stdoutWriter().writeString(message)
}

func prepareAnchorSetup(values anchorSetupArgs) (userSettings, string, anchorProviderSettings, error) {
	if values.Force && !values.Write {
		return userSettings{}, "", anchorProviderSettings{}, fmt.Errorf("--force requires --write")
	}
	name := strings.TrimSpace(values.Provider)
	if name == "" || name != values.Provider {
		return userSettings{}, "", anchorProviderSettings{}, fmt.Errorf("--provider must be a non-empty name without surrounding whitespace")
	}
	provider := anchorProviderSettings{Command: append([]string{values.Command}, values.CommandArgs...), TimeoutMS: values.TimeoutMS}
	if err := validateAnchorProviderSettings(provider); err != nil {
		return userSettings{}, "", anchorProviderSettings{}, fmt.Errorf("anchor provider %q: %w", name, err)
	}
	if _, err := exec.LookPath(provider.Command[0]); err != nil {
		return userSettings{}, "", anchorProviderSettings{}, fmt.Errorf("anchor provider %q executable: %w", name, err)
	}
	settingsPath, err := userSettingsPath()
	if err != nil {
		return userSettings{}, "", anchorProviderSettings{}, err
	}
	settings, err := loadUserSettings()
	if err != nil {
		return userSettings{}, "", anchorProviderSettings{}, err
	}
	if settings.Anchors.Providers == nil {
		settings.Anchors.Providers = map[string]anchorProviderSettings{}
	}
	if existing, found := settings.Anchors.Providers[name]; found && !equalAnchorProviderSettings(existing, provider) && !values.Force {
		return userSettings{}, "", anchorProviderSettings{}, fmt.Errorf("anchor provider %q already has different settings; inspect the preview and pass --write --force to replace it", name)
	}
	settings.Anchors.Providers[name] = provider
	if values.SetDefault {
		settings.Anchors.DefaultProvider = name
	}
	if values.EnableByDefault {
		if settings.Anchors.DefaultProvider != name {
			return userSettings{}, "", anchorProviderSettings{}, fmt.Errorf("--enable-by-default requires --set-default or an existing default_provider of %q", name)
		}
		settings.Anchors.EnabledByDefault = true
	}
	return settings, settingsPath, provider, nil
}

func equalAnchorProviderSettings(left, right anchorProviderSettings) bool {
	if left.TimeoutMS != right.TimeoutMS || len(left.Command) != len(right.Command) {
		return false
	}
	for index := range left.Command {
		if left.Command[index] != right.Command[index] {
			return false
		}
	}
	return true
}

func renderAnchorSetupPreview(settings userSettings, settingsPath, provider string) error {
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	var output strings.Builder
	fmt.Fprintf(&output, "anchor setup preview settings=%s provider=%s (no files changed)\n", settingsPath, provider)
	output.Write(encoded)
	output.WriteByte('\n')
	fmt.Fprintf(&output, "write: rerun with --write after reviewing this user-owned configuration\n")
	fmt.Fprintf(&output, "verify: grepple anchors doctor --provider %s\n", quoteCommandArgument(provider))
	return stdoutWriter().writeString(output.String())
}

func writeUserSettingsAtomic(path string, settings userSettings) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create Grepple settings directory: %w", err)
	}
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	temporary, err := os.CreateTemp(directory, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary Grepple settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write Grepple settings: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace Grepple settings: %w", err)
	}
	return nil
}
