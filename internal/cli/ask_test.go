package cli

import (
	"bytes"
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
	logDirectory := filepath.Join(t.TempDir(), "ask-logs")
	t.Setenv(askLogDirectoryEnv, logDirectory)
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

func assertAskRequest(t *testing.T, requestCount int, requestBody map[string]any, encoded []byte) {
	t.Helper()
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
		assertAskRequest(t, requestCount, requestBody, encoded)
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

func TestSimpleReadToolIsBoundedAndConfined(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "settings.json"))
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

func TestAskReadToolBatchesAnchoredLocalRanges(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{"first.go": "one\ntwo\n", "second.go": "three\nfour\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{
		EnabledByDefault: true, DefaultProvider: "test",
		Providers: map[string]anchorProviderSettings{"test": {Command: []string{os.Args[0], "-test.run=TestAnchorProviderProcess"}}},
	}})
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER", "1")
	response, err := runAskReadTool(t.Context(), root, "", readToolInput{Files: []readToolFileInput{
		{Path: "first.go", StartLine: 2, EndLine: 2}, {Path: "second.go", StartLine: 1, EndLine: 2},
	}})
	if err != nil || response.IsError {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	for _, expected := range []string{"== first.go ==", "A02│2│two", "== second.go ==", "A01│1│three", "A02│2│four"} {
		if !strings.Contains(response.Content, expected) {
			t.Fatalf("batch output missing %q:\n%s", expected, response.Content)
		}
	}
}

func TestAskResearchToolsAreTypedAndDirect(t *testing.T) {
	names := askResearchToolNames()
	if strings.Contains(strings.Join(names, ","), "grepple") {
		t.Fatalf("generic CLI tool is still exposed: %v", names)
	}
	info, _ := json.Marshal(askResearchToolInfo(newAskResearchTools(t.TempDir(), "https://example.invalid")))
	for _, field := range []string{`"query"`, `"location"`, `"operation"`, `"direction"`, `"repository"`} {
		if !bytes.Contains(info, []byte(field)) {
			t.Fatalf("typed tool schemas missing %s: %s", field, info)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("package sample\n\nfunc Parse() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	value, err := runAskSearch(context.Background(), root, "", askSearchInput{Query: "Parse", Mode: "snippets"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(value)
	if !bytes.Contains(encoded, []byte("source.go")) || !bytes.Contains(encoded, []byte("func Parse")) {
		t.Fatalf("direct search result=%s", encoded)
	}
	if _, err := runAskSearch(context.Background(), root, "", askSearchInput{Query: "Parse", Paths: []string{root}}); err != nil {
		t.Fatalf("direct search rejected the workspace root: %v", err)
	}
	if _, err := runAskSearch(context.Background(), root, "", askSearchInput{Query: "root", Paths: []string{"../outside"}}); err == nil {
		t.Fatal("direct search accepted a path outside the workspace")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := runAskSearch(context.Background(), root, "", askSearchInput{Query: "root", Paths: []string{"escape"}}); err == nil {
		t.Fatal("direct search accepted a symlink escape")
	}
}

func TestAskAnalysisToolsCallInternalEngines(t *testing.T) {
	root := t.TempDir()
	source := "package sample\n\nfunc Parse() { helper() }\nfunc helper() {}\n"
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	architecture, err := runAskArchitecture(root, askArchitectureInput{Operation: "resolve", Symbol: "Parse", Paths: []string{"source.go"}})
	if err != nil || !jsonContains(architecture, "Parse") {
		t.Fatalf("architecture=%+v err=%v", architecture, err)
	}
	graph, err := runAskGraph(root, askGraphInput{Direction: "callees", Symbol: "Parse", Paths: []string{"source.go"}, Depth: 1})
	if err != nil || !jsonContains(graph, "helper") {
		t.Fatalf("graph=%+v err=%v", graph, err)
	}
	navigation, err := runAskNavigate(context.Background(), root, "", askNavigateInput{Location: "source.go:1", FollowDepth: 2})
	if err != nil || !jsonContains(navigation, "related graph construction was skipped") {
		t.Fatalf("navigation=%+v err=%v", navigation, err)
	}
	structural, err := runAskStructural(context.Background(), root, "", askStructuralInput{Query: "language go\n`helper()`", Paths: []string{"source.go"}})
	if err != nil || len(structural.Findings) != 1 {
		t.Fatalf("structural findings=%d err=%v", len(structural.Findings), err)
	}
	sources, err := buildSourceScopeReport([]string{"source.go"})
	if err != nil || sources.SelectedFiles != 1 {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}
}

func TestAskRemoteToolsUseSelectedServer(t *testing.T) {
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
	read, err := runAskRemoteReadTool(context.Background(), server.URL, readToolInput{Repository: "owner/repo@tag~v1.0.0", Path: "source.go", StartLine: 7, EndLine: 7})
	if err != nil || read.IsError || !strings.Contains(read.Content, "7│answer") {
		t.Fatalf("remote read=%+v err=%v", read, err)
	}
	refs, err := runAskRepositoryRefs(context.Background(), server.URL, askRepositoryRefsInput{Repository: "owner/repo", Kind: "tag"})
	if err != nil || refs.Count != 1 || refs.Repos[0].Selector != "owner/repo@tag~v1.0.0" {
		t.Fatalf("remote refs=%+v err=%v", refs, err)
	}
}

func jsonContains(value any, text string) bool {
	encoded, _ := json.Marshal(value)
	return bytes.Contains(encoded, []byte(text))
}

func TestAskToolResultDisclosesTruncation(t *testing.T) {
	response, err := askToolResult(strings.Repeat("x", defaultToolOutputSize+1), nil)
	if err != nil || response.IsError || !strings.Contains(response.Content, `"truncated":true`) {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}

func TestAIProviderListShowsBuiltInsLoggedOut(t *testing.T) {
	t.Setenv(aiprovider.CredentialsPathEnv, filepath.Join(t.TempDir(), "credentials.json"))
	for _, name := range []string{"GREPPLE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY", "GREPPLE_ANTHROPIC_OAUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "GREPPLE_OPENAI_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(name, "")
	}
	output := captureStdout(t, func() {
		if err := runAIProvider([]string{"list"}); err != nil {
			t.Fatal(err)
		}
	})
	want := "anthropic\tlogged-out\nanthropic-subscription\tlogged-out\nbedrock\tlogged-out\ncodex\tlogged-out\ncopilot\tlogged-out\nopenai\tlogged-out\n"
	if output != want {
		t.Fatalf("output=%q", output)
	}
}

func askTestJWT(payload map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	content, _ := json.Marshal(payload)
	return header + "." + base64.RawURLEncoding.EncodeToString(content) + ".signature"
}
