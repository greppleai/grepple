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

type anchorSetupArgs struct {
	Provider        string   `arg:"--provider,required" placeholder:"NAME" help:"provider name to add or update"`
	Command         string   `arg:"--command,required" placeholder:"ABSOLUTE_PATH" help:"absolute provider executable path"`
	CommandArgs     []string `arg:"--command-arg,separate" placeholder:"ARG" help:"append one literal executable argument; repeatable"`
	TimeoutMS       int      `arg:"--timeout-ms" placeholder:"N" help:"provider timeout in milliseconds (default 5000)"`
	SetDefault      bool     `arg:"--set-default" help:"select this provider as the user default"`
	EnableByDefault bool     `arg:"--enable-by-default" help:"use this default provider for eligible searches; requires this provider to be default"`
	Write           bool     `arg:"--write" help:"atomically write the previewed user settings"`
	Force           bool     `arg:"--force" help:"replace an existing provider with different settings; requires --write"`
}

func (anchorSetupArgs) Description() string {
	return "Preview or explicitly write a user-owned anchor provider. Without --write, settings are printed and the filesystem is unchanged."
}

// RunSetup previews or writes anchor-provider settings.
func RunSetup(args []string) error {
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
	if err := usersettings.Save(settingsPath, settings); err != nil {
		return err
	}
	message := fmt.Sprintf("anchor setup written settings=%s provider=%s\nnext: grepple anchors doctor --provider %s\n", settingsPath, values.Provider, shellquote.Argument(values.Provider))
	return cliruntime.NewOutput(os.Stdout).WriteString(message)
}

func prepareAnchorSetup(values anchorSetupArgs) (usersettings.Config, string, usersettings.Provider, error) {
	if values.Force && !values.Write {
		return usersettings.Config{}, "", usersettings.Provider{}, fmt.Errorf("--force requires --write")
	}
	name := strings.TrimSpace(values.Provider)
	if name == "" || name != values.Provider {
		return usersettings.Config{}, "", usersettings.Provider{}, fmt.Errorf("--provider must be a non-empty name without surrounding whitespace")
	}
	provider := usersettings.Provider{Command: append([]string{values.Command}, values.CommandArgs...), TimeoutMS: values.TimeoutMS}
	if err := usersettings.ValidateProvider(provider); err != nil {
		return usersettings.Config{}, "", usersettings.Provider{}, fmt.Errorf("anchor provider %q: %w", name, err)
	}
	if _, err := exec.LookPath(provider.Command[0]); err != nil {
		return usersettings.Config{}, "", usersettings.Provider{}, fmt.Errorf("anchor provider %q executable: %w", name, err)
	}
	settingsPath, err := usersettings.Path()
	if err != nil {
		return usersettings.Config{}, "", usersettings.Provider{}, err
	}
	settings, err := usersettings.Load()
	if err != nil {
		return usersettings.Config{}, "", usersettings.Provider{}, err
	}
	if settings.Anchors.Providers == nil {
		settings.Anchors.Providers = map[string]usersettings.Provider{}
	}
	if existing, found := settings.Anchors.Providers[name]; found && !equalAnchorProviderSettings(existing, provider) && !values.Force {
		return usersettings.Config{}, "", usersettings.Provider{}, fmt.Errorf("anchor provider %q already has different settings; inspect the preview and pass --write --force to replace it", name)
	}
	settings.Anchors.Providers[name] = provider
	if values.SetDefault {
		settings.Anchors.DefaultProvider = name
	}
	if values.EnableByDefault {
		if settings.Anchors.DefaultProvider != name {
			return usersettings.Config{}, "", usersettings.Provider{}, fmt.Errorf("--enable-by-default requires --set-default or an existing default_provider of %q", name)
		}
		settings.Anchors.EnabledByDefault = true
	}
	return settings, settingsPath, provider, nil
}

func equalAnchorProviderSettings(left, right usersettings.Provider) bool {
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

func renderAnchorSetupPreview(settings usersettings.Config, settingsPath, provider string) error {
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	var output strings.Builder
	fmt.Fprintf(&output, "anchor setup preview settings=%s provider=%s (no files changed)\n", settingsPath, provider)
	output.Write(encoded)
	output.WriteByte('\n')
	fmt.Fprintf(&output, "write: rerun with --write after reviewing this user-owned configuration\n")
	fmt.Fprintf(&output, "verify: grepple anchors doctor --provider %s\n", shellquote.Argument(provider))
	return cliruntime.NewOutput(os.Stdout).WriteString(output.String())
}
