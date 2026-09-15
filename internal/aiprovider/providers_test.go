package aiprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"charm.land/fantasy"
)

type apiKeyProviderTest struct {
	name     string
	provider *apiKeyProvider
	model    string
	answer   string
}

func TestAPIKeyProvidersStoreSecretsAndConstructModels(t *testing.T) {
	for _, name := range []string{"GREPPLE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY", "GREPPLE_OPENAI_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(name, "")
	}
	server := httptest.NewServer(apiKeyProviderTestHandler(t))
	defer server.Close()
	t.Setenv("GREPPLE_ANTHROPIC_API_URL", server.URL)
	t.Setenv("GREPPLE_OPENAI_API_URL", server.URL)
	tests := []apiKeyProviderTest{
		{name: "anthropic", provider: newAnthropicProvider(testStore(t), server.Client()), model: "claude-sonnet-4-5-20250929", answer: "anthropic answer"},
		{name: "openai", provider: newOpenAIProvider(testStore(t), server.Client()), model: "gpt-5.1", answer: "openai answer"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { runAPIKeyProviderTest(t, test) })
	}
}

func apiKeyProviderTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("content-type", "application/json")
		switch request.URL.Path {
		case "/v1/messages":
			if request.Header.Get("x-api-key") != "secret" {
				t.Fatalf("Anthropic API key header=%q", request.Header.Get("x-api-key"))
			}
			fmt.Fprint(writer, `{"id":"message","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"anthropic answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		case "/responses":
			if request.Header.Get("Authorization") != "Bearer secret" {
				t.Fatalf("OpenAI authorization=%q", request.Header.Get("Authorization"))
			}
			fmt.Fprint(writer, `{"id":"response","object":"response","created_at":1,"status":"completed","model":"gpt-5.1","output":[{"id":"message","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"openai answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		default:
			http.NotFound(writer, request)
		}
	})
}

func runAPIKeyProviderTest(t *testing.T, test apiKeyProviderTest) {
	t.Helper()
	if err := test.provider.Login(context.Background(), LoginOptions{ReadSecret: fixedSecret("secret")}); err != nil {
		t.Fatal(err)
	}
	loggedIn, err := test.provider.LoggedIn()
	if err != nil || !loggedIn {
		t.Fatalf("loggedIn=%v err=%v", loggedIn, err)
	}
	model, err := test.provider.LanguageModel(context.Background(), test.model)
	if err != nil {
		t.Fatal(err)
	}
	response, err := model.Generate(context.Background(), fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage("question")}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Content.Text() != test.answer {
		t.Fatalf("answer=%q", response.Content.Text())
	}
}

func TestEnvironmentAPIKeyDoesNotPersistDuringNoninteractiveLogin(t *testing.T) {
	store := testStore(t)
	t.Setenv("GREPPLE_OPENAI_API_KEY", "environment-secret")
	provider := newOpenAIProvider(store, http.DefaultClient)
	if err := provider.Login(context.Background(), LoginOptions{}); err != nil {
		t.Fatal(err)
	}
	var credentials apiKeyCredentials
	found, err := store.Load(provider.Name(), &credentials)
	if err != nil || found {
		t.Fatalf("environment credential persisted: found=%v credentials=%+v err=%v", found, credentials, err)
	}
}

func TestAnthropicSubscriptionUsesOAuthHeaders(t *testing.T) {
	t.Setenv("GREPPLE_ANTHROPIC_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_API_KEY", "must-not-leak")
	var authorization, beta, app, apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		beta = request.Header.Get("anthropic-beta")
		app = request.Header.Get("X-App")
		apiKey = request.Header.Get("x-api-key")
		writer.Header().Set("content-type", "text/event-stream")
		fmt.Fprint(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"message\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(writer, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(writer, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer\"}}\n\n")
		fmt.Fprint(writer, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(writer, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":1}}\n\n")
		fmt.Fprint(writer, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()
	t.Setenv("GREPPLE_ANTHROPIC_API_URL", server.URL)
	provider := newAnthropicSubscriptionProvider(testStore(t), server.Client())
	if err := provider.Login(context.Background(), LoginOptions{ReadSecret: fixedSecret("oauth-token")}); err != nil {
		t.Fatal(err)
	}
	model, err := provider.LanguageModel(context.Background(), provider.DefaultModel())
	if err != nil {
		t.Fatal(err)
	}
	answer, err := streamModelText(model)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "answer" {
		t.Fatalf("stream answer=%q", answer)
	}
	if authorization != "Bearer oauth-token" || !strings.Contains(beta, "oauth-2025-04-20") || app != "cli" || apiKey != "" {
		t.Fatalf("authorization=%q beta=%q app=%q apiKey=%q", authorization, beta, app, apiKey)
	}
}

func TestCopilotDeviceLoginAndLanguageModel(t *testing.T) {
	var exchangeAuthorization, modelAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("content-type", "application/json")
		switch request.URL.Path {
		case "/device/code":
			fmt.Fprint(writer, `{"device_code":"device","user_code":"CODE","verification_uri":"https://github.example/device","expires_in":900,"interval":1}`)
		case "/oauth/access_token":
			fmt.Fprint(writer, `{"access_token":"github-token"}`)
		case "/copilot_internal/v2/token":
			exchangeAuthorization = request.Header.Get("Authorization")
			fmt.Fprint(writer, `{"token":"copilot-token","expires_at":9999999999}`)
		case "/chat/completions":
			modelAuthorization = request.Header.Get("Authorization")
			body, _ := io.ReadAll(request.Body)
			if strings.Contains(string(body), `"stream":true`) {
				writer.Header().Set("content-type", "text/event-stream")
				fmt.Fprint(writer, "data: {\"id\":\"completion\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4.1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"copilot answer\"},\"finish_reason\":null}]}\n\n")
				fmt.Fprint(writer, "data: {\"id\":\"completion\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4.1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n")
				fmt.Fprint(writer, "data: [DONE]\n\n")
				return
			}
			fmt.Fprint(writer, `{"id":"completion","object":"chat.completion","created":1,"model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"copilot answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	provider := newCopilotProvider(testStore(t), server.Client())
	provider.loginURL = server.URL
	provider.githubAPI = server.URL
	provider.copilotAPI = server.URL
	var output strings.Builder
	if err := provider.Login(context.Background(), LoginOptions{NoBrowser: true, Output: &output}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "CODE") {
		t.Fatalf("login output=%q", output.String())
	}
	model, err := provider.LanguageModel(context.Background(), provider.DefaultModel())
	if err != nil {
		t.Fatal(err)
	}
	answer, err := streamModelText(model)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "copilot answer" || exchangeAuthorization != "token github-token" || modelAuthorization != "Bearer copilot-token" {
		t.Fatalf("answer=%q exchange=%q model=%q", answer, exchangeAuthorization, modelAuthorization)
	}
}

func TestBedrockUsesAWSDefaultCredentialChain(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_REGION", "us-east-1")
	provider := newBedrockProvider(testStore(t), http.DefaultClient)
	if err := provider.Login(context.Background(), LoginOptions{}); err != nil {
		t.Fatal(err)
	}
	loggedIn, err := provider.LoggedIn()
	if err != nil || !loggedIn {
		t.Fatalf("loggedIn=%v err=%v", loggedIn, err)
	}
	model, err := provider.LanguageModel(context.Background(), provider.DefaultModel())
	if err != nil {
		t.Fatal(err)
	}
	if model.Provider() != bedrockProviderName {
		t.Fatalf("provider=%q", model.Provider())
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	loggedIn, err = provider.LoggedIn()
	if err != nil || loggedIn {
		t.Fatalf("stale Bedrock marker reported logged in: loggedIn=%v err=%v", loggedIn, err)
	}
}

func streamModelText(model fantasy.LanguageModel) (string, error) {
	stream, err := model.Stream(context.Background(), fantasy.Call{Prompt: fantasy.Prompt{fantasy.NewUserMessage("question")}})
	if err != nil {
		return "", err
	}
	var text strings.Builder
	var streamErr error
	stream(func(part fantasy.StreamPart) bool {
		if part.Type == fantasy.StreamPartTypeTextDelta {
			text.WriteString(part.Delta)
		}
		if part.Type == fantasy.StreamPartTypeError {
			streamErr = part.Error
			return false
		}
		return true
	})
	return text.String(), streamErr
}

func fixedSecret(secret string) func(context.Context, string) (string, error) {
	return func(context.Context, string) (string, error) { return secret, nil }
}
