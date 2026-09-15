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
		if requestCount == 1 && (!strings.Contains(string(encoded), "where is parsing") || requestBody["model"] != "gpt-5.6-luna") {
			t.Fatalf("request body did not contain question: %s", encoded)
		}
		if requestCount == 2 && !strings.Contains(string(encoded), "source evidence") {
			t.Fatalf("second request did not contain tool evidence: %s", encoded)
		}
		writer.Header().Set("content-type", "application/json")
		if requestCount == 1 {
			fmt.Fprint(writer, `{"id":"response-1","object":"response","created_at":1,"status":"completed","model":"gpt-5.3-codex","output":[{"id":"tool","type":"function_call","status":"completed","call_id":"call-1","name":"read","arguments":"{\"path\":\"evidence.txt\",\"start_line\":1,\"end_line\":1}"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
			return
		}
		fmt.Fprint(writer, `{"id":"response-2","object":"response","created_at":1,"status":"completed","model":"gpt-5.3-codex","output":[{"id":"message","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"source-backed answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	return server, &requestCount
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
