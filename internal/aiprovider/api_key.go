package aiprovider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

const (
	anthropicProviderName             = "anthropic"
	anthropicSubscriptionProviderName = "anthropic-subscription"
	openAIProviderName                = "openai"
	anthropicAPIBase                  = "https://api.anthropic.com"
	openAIAPIBase                     = "https://api.openai.com/v1"
)

type apiKeyCredentials struct {
	APIKey string `json:"apiKey"`
}

type apiKeyProvider struct {
	store        *Store
	client       *http.Client
	name         string
	defaultModel string
	environment  []string
	prompt       string
	build        func(string) (fantasy.Provider, error)
}

func newAnthropicProvider(store *Store, client *http.Client) *apiKeyProvider {
	provider := &apiKeyProvider{
		store: store, client: client, name: anthropicProviderName,
		defaultModel: "claude-sonnet-4-5-20250929",
		environment:  []string{"GREPPLE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"},
		prompt:       "Anthropic API key",
	}
	provider.build = func(key string) (fantasy.Provider, error) {
		return fantasyanthropic.New(
			fantasyanthropic.WithAPIKey(key),
			fantasyanthropic.WithBaseURL(environmentOr("GREPPLE_ANTHROPIC_API_URL", anthropicAPIBase)),
			fantasyanthropic.WithHTTPClient(client),
		)
	}
	return provider
}

func newAnthropicSubscriptionProvider(store *Store, client *http.Client) *apiKeyProvider {
	provider := &apiKeyProvider{
		store: store, client: client, name: anthropicSubscriptionProviderName,
		defaultModel: "claude-sonnet-4-5-20250929",
		environment:  []string{"GREPPLE_ANTHROPIC_OAUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN"},
		prompt:       "Anthropic subscription OAuth token (from 'claude setup-token')",
	}
	provider.build = func(token string) (fantasy.Provider, error) {
		return fantasyanthropic.New(
			fantasyanthropic.WithName(anthropicSubscriptionProviderName),
			fantasyanthropic.WithBaseURL(environmentOr("GREPPLE_ANTHROPIC_API_URL", anthropicAPIBase)),
			fantasyanthropic.WithHeaders(map[string]string{
				"Authorization":  "Bearer " + token,
				"anthropic-beta": "oauth-2025-04-20",
				"User-Agent":     "grepple/anthropic-subscription",
				"X-App":          "cli",
				"x-api-key":      "",
			}),
			fantasyanthropic.WithHTTPClient(client),
		)
	}
	return provider
}

func newOpenAIProvider(store *Store, client *http.Client) *apiKeyProvider {
	provider := &apiKeyProvider{
		store: store, client: client, name: openAIProviderName,
		defaultModel: "gpt-5.1",
		environment:  []string{"GREPPLE_OPENAI_API_KEY", "OPENAI_API_KEY"},
		prompt:       "OpenAI API key",
	}
	provider.build = func(key string) (fantasy.Provider, error) {
		return fantasyopenai.New(
			fantasyopenai.WithAPIKey(key),
			fantasyopenai.WithBaseURL(environmentOr("GREPPLE_OPENAI_API_URL", openAIAPIBase)),
			fantasyopenai.WithHTTPClient(client),
			fantasyopenai.WithUseResponsesAPI(),
		)
	}
	return provider
}

func (p *apiKeyProvider) Name() string { return p.name }

func (p *apiKeyProvider) DefaultModel() string { return p.defaultModel }

func (p *apiKeyProvider) Login(ctx context.Context, options LoginOptions) error {
	if options.ReadSecret == nil {
		if firstEnvironment(p.environment...) != "" {
			return nil
		}
		return fmt.Errorf("%s is not set; provide it in the environment or run login interactively", strings.Join(p.environment, " or "))
	}
	key, err := options.ReadSecret(ctx, p.prompt)
	if err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("%s cannot be empty", p.prompt)
	}
	return p.store.Save(p.Name(), apiKeyCredentials{APIKey: key})
}

func (p *apiKeyProvider) Logout() error { return p.store.Delete(p.Name()) }

func (p *apiKeyProvider) LoggedIn() (bool, error) {
	if firstEnvironment(p.environment...) != "" {
		return true, nil
	}
	var credentials apiKeyCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	return found && strings.TrimSpace(credentials.APIKey) != "", err
}

func (p *apiKeyProvider) LanguageModel(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	key, err := p.apiKey()
	if err != nil {
		return nil, err
	}
	provider, err := p.build(key)
	if err != nil {
		return nil, err
	}
	return provider.LanguageModel(ctx, modelID)
}

func (p *apiKeyProvider) apiKey() (string, error) {
	if key := firstEnvironment(p.environment...); key != "" {
		return key, nil
	}
	var credentials apiKeyCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(credentials.APIKey) == "" {
		return "", fmt.Errorf("%s is not logged in; run 'grepple ai-provider login %s'", p.Name(), p.Name())
	}
	return credentials.APIKey, nil
}

func firstEnvironment(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}
