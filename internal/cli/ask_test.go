package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/aiprovider"
)

func TestRunAskUsesFantasyProviderAndReadTool(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".grepple"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grepple", "grepple.json"), []byte(`{"ai":{"model":"gpt-5.6-luna"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("source evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	server, requestCount := newAskTestServer(t)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv(aiprovider.CredentialsPathEnv, path)
	t.Setenv("GREPPLE_CODEX_API_URL", server.URL)
	store, err := aiprovider.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	access := askTestJWT(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())})
	if err := store.Save("codex", map[string]any{"accessToken": access, "refreshToken": "refresh", "accountId": "account"}); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := runAsk([]string{"where", "is", "parsing"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "source-backed answer\n" || *requestCount != 2 {
		t.Fatalf("answer=%q requests=%d", output, *requestCount)
	}
}

func newAskTestServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			http.NotFound(writer, request)
			return
		}
		var requestBody map[string]any
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(requestBody)
		requestCount++
		if requestCount == 1 && (!strings.Contains(string(encoded), "where is parsing") || requestBody["model"] != "gpt-5.6-luna" || requestBody["stream"] != true) {
			t.Fatalf("request body did not contain question: %s", encoded)
		}
		if requestCount == 2 && !strings.Contains(string(encoded), "source evidence") {
			t.Fatalf("second request did not contain tool evidence: %s", encoded)
		}
		writer.Header().Set("content-type", "text/event-stream")
		if requestCount == 1 {
			writeAskSSE(writer,
				`{"type":"response.output_item.added","output_index":0,"item":{"id":"tool","type":"function_call","status":"in_progress","call_id":"call-1","name":"read","arguments":""}}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"id":"tool","type":"function_call","status":"completed","call_id":"call-1","name":"read","arguments":"{\"path\":\"evidence.txt\",\"start_line\":1,\"end_line\":1}"}}`,
				askCompletedEvent("response-1"),
			)
			return
		}
		writeAskSSE(writer,
			`{"type":"response.output_item.added","output_index":0,"item":{"id":"message","type":"message","status":"in_progress","role":"assistant","content":[]}}`,
			`{"type":"response.output_text.delta","item_id":"message","output_index":0,"content_index":0,"delta":"source-backed answer"}`,
			`{"type":"response.output_item.done","output_index":0,"item":{"id":"message","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"source-backed answer","annotations":[]}]}}`,
			askCompletedEvent("response-2"),
		)
	}))
	return server, &requestCount
}

func writeAskSSE(writer http.ResponseWriter, events ...string) {
	for _, event := range events {
		fmt.Fprintf(writer, "data: %s\n\n", event)
	}
}

func askCompletedEvent(id string) string {
	return fmt.Sprintf(`{"type":"response.completed","response":{"id":%q,"object":"response","created_at":1,"status":"completed","model":"gpt-5.6-luna","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`, id)
}

func TestSimpleReadToolIsBoundedAndConfined(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	response, err := runSimpleReadTool(root, readToolInput{Path: "source.go", StartLine: 2, EndLine: 3})
	if err != nil || response.IsError || !strings.Contains(response.Content, "2│two\n3│three") {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	response, err = runSimpleReadTool(root, readToolInput{Path: "../outside"})
	if err != nil || !response.IsError {
		t.Fatalf("escaping read response=%+v err=%v", response, err)
	}
}

func TestResearchToolRejectsRecursiveAndMutatingCommands(t *testing.T) {
	for _, args := range [][]string{{"ask", "question"}, {"ai-provider", "login"}, {"artifacts", "clean"}, {"anchors", "setup", "--write"}, {"rules", "add"}} {
		response, err := runGreppleResearchTool(context.Background(), greppleToolInput{Args: args})
		if err != nil || !response.IsError {
			t.Fatalf("args=%v response=%+v err=%v", args, response, err)
		}
	}
}

func TestBoundedBufferDisclosesTruncation(t *testing.T) {
	buffer := &boundedBuffer{limit: 4}
	if count, err := buffer.Write([]byte("abcdef")); err != nil || count != 6 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if buffer.String() != "abcd" || !buffer.truncated {
		t.Fatalf("buffer=%q truncated=%v", buffer.String(), buffer.truncated)
	}
}

func TestAIProviderListShowsCodexLoggedOut(t *testing.T) {
	t.Setenv(aiprovider.CredentialsPathEnv, filepath.Join(t.TempDir(), "credentials.json"))
	output := captureStdout(t, func() {
		if err := runAIProvider([]string{"list"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "codex\tlogged-out\n" {
		t.Fatalf("output=%q", output)
	}
}

func askTestJWT(payload map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	content, _ := json.Marshal(payload)
	return header + "." + base64.RawURLEncoding.EncodeToString(content) + ".signature"
}
