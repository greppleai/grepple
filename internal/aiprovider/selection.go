package aiprovider

import (
	"fmt"
	"strings"
)

// ResolveSelection applies explicit and configured provider/model precedence.
func ResolveSelection(explicitProvider, explicitModel, configuredModel string) (string, string, error) {
	provider := defaultSelectionProvider(explicitProvider)
	if explicit := strings.TrimSpace(explicitModel); explicit != "" {
		selectedProvider, model, prefixed, err := parseModelSelector(explicit)
		if err != nil {
			return "", "", err
		}
		if !prefixed {
			return provider, model, nil
		}
		if strings.TrimSpace(explicitProvider) != "" && provider != selectedProvider {
			return "", "", fmt.Errorf("--provider %q conflicts with --model provider %q", provider, selectedProvider)
		}
		return selectedProvider, model, nil
	}
	configured := strings.TrimSpace(configuredModel)
	if configured == "" {
		return provider, "", nil
	}
	selectedProvider, model, prefixed, err := parseModelSelector(configured)
	if err != nil {
		return "", "", fmt.Errorf("configured ask.model: %w", err)
	}
	if prefixed {
		if strings.TrimSpace(explicitProvider) == "" || provider == selectedProvider {
			return selectedProvider, model, nil
		}
		return provider, "", nil
	}
	if provider == "codex" {
		return provider, model, nil
	}
	return provider, "", nil
}

func parseModelSelector(value string) (provider, model string, prefixed bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false, fmt.Errorf("model cannot be empty")
	}
	provider, model, found := strings.Cut(value, "/")
	if !found {
		return "", value, false, nil
	}
	provider, model = strings.TrimSpace(strings.ToLower(provider)), strings.TrimSpace(model)
	if provider == "" || model == "" {
		return "", "", false, fmt.Errorf("model must use <provider>/<model>")
	}
	return provider, model, true, nil
}

func defaultSelectionProvider(value string) string {
	if value = strings.TrimSpace(strings.ToLower(value)); value != "" {
		return value
	}
	return "codex"
}
