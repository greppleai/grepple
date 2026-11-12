package aiprovider

import (
	"fmt"
	"strings"
)

// ResolveSelection applies explicit provider/model selection.
func ResolveSelection(explicitProvider, explicitModel string) (string, string, error) {
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
