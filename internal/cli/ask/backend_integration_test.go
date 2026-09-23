package ask

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/aiprovider"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandUsesProviderReadToolAndWritesLog(t *testing.T) {
	root := t.TempDir()
	logDirectory := filepath.Join(t.TempDir(), "ask-logs")
	t.Setenv("GREPPLE_ASK_LOG_DIR", logDirectory)
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".grepple"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".grepple", "grepple.json"), []byte(`{"ask":{"model":"codex/gpt-5.6-luna"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("source evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	server, requestCount := newAskTestServer(t)
	defer server.Close()
	credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv(aiprovider.CredentialsPathEnv, credentialsPath)
	t.Setenv("GREPPLE_CODEX_API_URL", server.URL)
	store, err := aiprovider.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	access := askTestJWT(map[string]any{"exp": float64(time.Now().Add(time.Hour).Unix())})
	if err := store.Save("codex", map[string]any{"accessToken": access, "refreshToken": "refresh", "accountId": "account"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	application := cliruntime.Environment{Output: &output, ErrorOutput: io.Discard}
	if err := New(application).Run([]string{"where", "is", "parsing"}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "source-backed answer\n" || *requestCount != 2 {
		t.Fatalf("answer=%q requests=%d", output.String(), *requestCount)
	}
	assertAskLog(t, logDirectory)
}

func assertAskLog(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ask log entries=%v err=%v", entries, err)
	}
	path := filepath.Join(directory, entries[0].Name())
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
		if !json.Valid(line) {
			t.Fatalf("invalid JSONL line: %s", line)
		}
	}
	for _, eventType := range []string{"session.start", "llm.timing", "tool.call", "tool.timing", "tool.result", "step.finish", "session.finish", "session.performance"} {
		if !bytes.Contains(content, []byte(`"type":"`+eventType+`"`)) {
			t.Fatalf("ask log missing %s: %s", eventType, content)
		}
	}
	if !bytes.Contains(content, []byte(`"schema":"grepple-ask-performance-v1"`)) || !bytes.Contains(content, []byte(`"llmDurationMs":`)) || !bytes.Contains(content, []byte(`"llmTimeToFirstOutputMs":`)) || !bytes.Contains(content, []byte(`"toolWallDurationMs":`)) {
		t.Fatalf("ask log lacks performance breakdown: %s", content)
	}
	if bytes.Contains(content, []byte(`"type":"model.chunk"`)) {
		t.Fatalf("ask log contains noisy model chunks: %s", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("ask log mode=%v", info.Mode().Perm())
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
		body := string(encoded)
		if requestCount == 1 {
			if !strings.Contains(body, "where is parsing") || requestBody["model"] != "gpt-5.6-luna" || requestBody["stream"] != true {
				t.Fatalf("request body did not contain question: %s", encoded)
			}
			if !strings.Contains(body, `"name":"search_code"`) || !strings.Contains(body, `"name":"navigate_code"`) || !strings.Contains(body, `"name":"read_file"`) || strings.Contains(body, `"name":"grepple"`) {
				t.Fatalf("request did not contain direct typed tools: %s", encoded)
			}
		}
		if requestCount == 2 && !strings.Contains(body, "source evidence") {
			t.Fatalf("second request did not contain tool evidence: %s", encoded)
		}
		writer.Header().Set("content-type", "text/event-stream")
		if requestCount == 1 {
			writeAskSSE(writer,
				`{"type":"response.output_item.added","output_index":0,"item":{"id":"tool","type":"function_call","status":"in_progress","call_id":"call-1","name":"read_file","arguments":""}}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"id":"tool","type":"function_call","status":"completed","call_id":"call-1","name":"read_file","arguments":"{\"path\":\"evidence.txt\",\"start_line\":1,\"end_line\":1}"}}`,
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

func askTestJWT(payload map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	content, _ := json.Marshal(payload)
	return header + "." + base64.RawURLEncoding.EncodeToString(content) + ".signature"
}

func TestSimpleReadToolIsBoundedAndConfined(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	application := cliruntime.Environment{}
	response, err := runSimpleReadTool(application, root, readToolInput{Path: "source.go", StartLine: 2, EndLine: 3})
	if err != nil || response.IsError || !strings.Contains(response.Content, "ldf│2│two\nnhJ│3│three") {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	response, err = runSimpleReadTool(application, root, readToolInput{Path: "source.go", StartLine: 2, EndLine: 30})
	if err != nil || response.IsError || !strings.Contains(response.Content, "2│two") || !strings.Contains(response.Content, "3│three") || !strings.Contains(response.Content, "warning: EOF") {
		t.Fatalf("clamped read response=%+v err=%v", response, err)
	}
	response, err = runSimpleReadTool(application, root, readToolInput{Path: "source.go", StartLine: 4, EndLine: 30})
	if err != nil || !response.IsError || !strings.Contains(response.Content, "outside file") {
		t.Fatalf("outside read response=%+v err=%v", response, err)
	}
	response, err = runSimpleReadTool(application, root, readToolInput{Path: "../outside"})
	if err != nil || !response.IsError {
		t.Fatalf("escaping read response=%+v err=%v", response, err)
	}
}

func TestReadToolBatchesLocalRanges(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{"first.go": "one\ntwo\n", "second.go": "three\nfour\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
	response, err := runAskReadTool(t.Context(), cliruntime.Environment{}, root, "", readToolInput{Files: []readToolFileInput{
		{Path: "first.go", StartLine: 2, EndLine: 2}, {Path: "second.go", StartLine: 1, EndLine: 2},
	}})
	if err != nil || response.IsError {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	for _, expected := range []string{"== first.go ==", "2│two", "== second.go ==", "1│three", "2│four"} {
		if !strings.Contains(response.Content, expected) {
			t.Fatalf("batch output missing %q:\n%s", expected, response.Content)
		}
	}
}

func TestSearchIsDirectAndConfined(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n\nfunc Parse() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	application := cliruntime.Environment{}
	value, err := runAskSearch(context.Background(), application, root, "", askSearchInput{Query: "Parse", Mode: "snippets"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(value)
	if !bytes.Contains(encoded, []byte("source.go")) || !bytes.Contains(encoded, []byte("func Parse")) {
		t.Fatalf("direct search result=%s", encoded)
	}
	if _, err := runAskSearch(context.Background(), application, root, "", askSearchInput{Query: "Parse", Paths: []string{root}}); err != nil {
		t.Fatalf("direct search rejected the workspace root: %v", err)
	}
	if _, err := runAskSearch(context.Background(), application, root, "", askSearchInput{Query: "root", Paths: []string{"../outside"}}); err == nil {
		t.Fatal("direct search accepted a path outside the workspace")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := runAskSearch(context.Background(), application, root, "", askSearchInput{Query: "root", Paths: []string{"escape"}}); err == nil {
		t.Fatal("direct search accepted a symlink escape")
	}
}

func TestRemoteToolsUseSelectedServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/public/raw":
			if request.URL.Query().Get("repo") != "owner/repo@tag~v1.0.0" {
				t.Fatalf("raw repo=%q", request.URL.Query().Get("repo"))
			}
			_, _ = writer.Write([]byte("answer\n"))
		case "/public/repos":
			_, _ = writer.Write([]byte(`{"ok":true,"count":1,"repos":[{"repo":"owner/repo","selector":"owner/repo@tag~v1.0.0","ref":"v1.0.0","refKind":"tag"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	application := cliruntime.Environment{}
	read, err := runAskRemoteReadTool(context.Background(), application, server.URL, readToolInput{Repository: "owner/repo@tag~v1.0.0", Path: "source.go", StartLine: 7, EndLine: 7})
	if err != nil || read.IsError || !strings.Contains(read.Content, "7│answer") {
		t.Fatalf("remote read=%+v err=%v", read, err)
	}
	refs, err := runAskRepositoryRefs(context.Background(), application, server.URL, askRepositoryRefsInput{Repository: "owner/repo", Kind: "tag"})
	if err != nil || refs.Count != 1 || refs.Repos[0].Selector != "owner/repo@tag~v1.0.0" {
		t.Fatalf("remote refs=%+v err=%v", refs, err)
	}
}

func TestGraphAndArchitectureSupportRemoteRepository(t *testing.T) {
	var operations []api.AnalysisOperation
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var input api.AnalysisRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		operations = append(operations, input.Operation)
		schemas := map[api.AnalysisOperation]string{
			api.AnalysisGraph:        "grepple-navigation-graph-v7",
			api.AnalysisArchitecture: "grepple-directory-architecture-v5",
		}
		result, _ := json.Marshal(map[string]any{"schema": schemas[input.Operation]})
		_ = json.NewEncoder(writer).Encode(api.AnalysisResponse{Schema: "grepple-remote-analysis-v1", Operation: input.Operation, Repository: input.Repository, Found: true, Complete: true, Result: result})
	}))
	defer server.Close()
	application := cliruntime.Environment{}
	session := newResearchSession(application, t.Context(), nil, t.TempDir(), server.URL)
	defer session.Close()
	if _, err := runAskGraphWithSession(session, t.TempDir(), askGraphInput{Direction: "impact", Symbol: "Run", Repository: "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runAskArchitectureWithSession(session, t.TempDir(), askArchitectureInput{Operation: "directory", Repository: "owner/repo"}); err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || operations[0] != api.AnalysisGraph || operations[1] != api.AnalysisArchitecture {
		t.Fatalf("operations=%v", operations)
	}
}
