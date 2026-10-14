package apiclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/greppleai/grepple/internal/clitelemetry"
	"github.com/greppleai/grepple/internal/wire"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func tracingTestScope(t *testing.T) (context.Context, trace.Span, *tracetest.InMemoryExporter, *sdktrace.TracerProvider) {
	t.Helper()
	export := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(export))
	ctx, root := provider.Tracer("test").Start(clitelemetry.WithInvocationTrace(t.Context()), "cli.invocation")
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return ctx, root, export, provider
}

func TestBackendRequestCategoriesAndParentage(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	ctx, root, export, _ := tracingTestScope(t)
	var mu sync.Mutex
	var headers = make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		headers[req.URL.Path] = req.Header.Get("traceparent")
		mu.Unlock()
		if req.Header.Get("tracestate") != "" || req.Header.Get("baggage") != "" {
			t.Error("unexpected baggage or tracestate")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client := New().(*apiClient)
	for _, path := range []string{"/public/search", "/public/search/pages", "/public/navigation/resolve", "/public/grit", "/public/analysis", "/public/raw", "/public/tree", "/public/repos", "/public/rules", "/public/rules/private-id", "/public/rules/private-id/results"} {
		response, err := client.do(ctx, http.MethodGet, server.URL+path+"?secret=value", "", nil)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		response.Body.Close()
	}
	root.End()
	mu.Lock()
	defer mu.Unlock()
	spans := export.GetSpans()
	if len(spans) != len(headers)+1 {
		t.Fatalf("spans=%d headers=%d", len(spans), len(headers))
	}
	for path, parent := range headers {
		if !strings.HasPrefix(parent, "00-"+root.SpanContext().TraceID().String()+"-") || !strings.HasSuffix(parent, "-01") {
			t.Fatalf("%s parent=%q", path, parent)
		}
	}
	for _, span := range spans {
		if span.Name == "cli.invocation" {
			continue
		}
		if !strings.HasPrefix(span.Name, "cli.http.") || span.SpanKind != trace.SpanKindClient || span.Parent.SpanID() != root.SpanContext().SpanID() {
			t.Fatalf("unsafe or orphaned span: %+v", span)
		}
		if len(span.Attributes) != 2 || len(span.Events) != 0 || strings.Contains(span.Name, "private") {
			t.Fatalf("unsafe attributes: %+v", span)
		}
	}
}

func TestTracedRedirectsDoNotCrossOriginAndDisabledBehaviorIsUnchanged(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	var leaked atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("traceparent") != "" {
			leaked.Add(1)
		}
		fmt.Fprint(w, `{}`)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, destination.URL+"/public/repos", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client := New().(*apiClient)
	ctx, root, _, _ := tracingTestScope(t)
	if response, err := client.do(ctx, http.MethodGet, origin.URL+"/public/repos", "", nil); err == nil {
		response.Body.Close()
		t.Fatal("cross-origin traced redirect was followed")
	}
	root.End()
	if leaked.Load() != 0 {
		t.Fatal("traceparent leaked across origins")
	}
	response, err := client.do(context.Background(), http.MethodGet, origin.URL+"/public/repos", "", nil)
	if err != nil {
		t.Fatalf("disabled request lost previous redirect behavior: %v", err)
	}
	response.Body.Close()
	if leaked.Load() != 0 {
		t.Fatal("disabled request acquired traceparent")
	}
}

func TestAuthProviderRequestsHeaderlessAndConcurrentTracesIsolated(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	ctxA, rootA, exportA, _ := tracingTestScope(t)
	ctxB, rootB, exportB, _ := tracingTestScope(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/login/") || strings.HasPrefix(req.URL.Path, "/auth/") {
			if req.Header.Get("traceparent") != "" {
				t.Error("auth or provider request leaked traceparent")
			}
		} else if !strings.Contains(req.Header.Get("traceparent"), rootA.SpanContext().TraceID().String()) && !strings.Contains(req.Header.Get("traceparent"), rootB.SpanContext().TraceID().String()) {
			t.Error("backend request lost its invocation parent")
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client := New().(*apiClient)
	var wg sync.WaitGroup
	for _, ctx := range []context.Context{ctxA, ctxB} {
		wg.Add(1)
		go func(ctx context.Context) {
			defer wg.Done()
			for _, path := range []string{"/public/repos", "/auth/config", "/auth/logout", "/login/device/code", "/auth/refresh"} {
				var response *http.Response
				var err error
				if strings.HasPrefix(path, "/public/") {
					response, err = client.do(ctx, http.MethodGet, server.URL+path, "", nil)
				} else {
					var req *http.Request
					req, err = http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
					if err == nil {
						response, err = client.authDo(req)
					}
				}
				if err != nil {
					t.Error(err)
					continue
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
			}
		}(ctx)
	}
	wg.Wait()
	rootA.End()
	rootB.End()
	for _, scope := range []struct {
		root  trace.Span
		spans []tracetest.SpanStub
	}{{rootA, exportA.GetSpans()}, {rootB, exportB.GetSpans()}} {
		if len(scope.spans) != 4 { // root, repos, auth/config, auth/logout
			t.Fatalf("unexpected auth/provider spans: %d", len(scope.spans))
		}
		for _, span := range scope.spans {
			if span.SpanContext.TraceID() != scope.root.SpanContext().TraceID() {
				t.Fatal("concurrent traces mingled")
			}
		}
	}
}

func TestCanceledOrInvalidContextsNeverInjectTraceparent(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	client := New().(*apiClient)
	ctx, root, export, _ := tracingTestScope(t)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err := client.do(ctx, http.MethodGet, "http://127.0.0.1:1/public/repos", "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	root.End()
	if len(export.GetSpans()) != 2 {
		t.Fatal("canceled call did not record outcome")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("traceparent") != "" {
			t.Error("invalid context injected traceparent")
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	response, err := client.do(context.Background(), http.MethodGet, server.URL+"/public/repos", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	// A foreign valid span alone must not enable tracing when user settings are off.
	foreign := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tracetest.NewInMemoryExporter()))
	defer foreign.Shutdown(context.Background())
	foreignCtx, foreignSpan := foreign.Tracer("other").Start(t.Context(), "foreign")
	response, err = client.do(foreignCtx, http.MethodGet, server.URL+"/public/repos", "", nil)
	foreignSpan.End()
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}

func TestUnsampledParentStillPropagatesWithoutExport(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0)))
	defer provider.Shutdown(context.Background())
	ctx, root := provider.Tracer("test").Start(clitelemetry.WithInvocationTrace(t.Context()), "cli.invocation")
	defer root.End()
	var parent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		parent = req.Header.Get("traceparent")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	response, err := New().(*apiClient).do(ctx, http.MethodGet, server.URL+"/public/repos", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !strings.HasSuffix(parent, "-00") || !strings.Contains(parent, root.SpanContext().TraceID().String()) {
		t.Fatalf("unsampled context did not propagate: %q", parent)
	}
}

func TestTypedPagingAndRawRetainRequestParentWithoutPayloadAttributes(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	ctx, root, export, _ := tracingTestScope(t)
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.Header.Get("traceparent"), root.SpanContext().TraceID().String()) {
			t.Error("typed request lost parent")
		}
		seen = append(seen, req.URL.Path)
		if req.URL.Path == "/public/search/pages" {
			fmt.Fprintf(w, `{"pageId":"%s.0","results":[],"complete":true}`, strings.Repeat("A", 43))
			return
		}
		fmt.Fprint(w, `private-source-body`)
	}))
	defer server.Close()
	client := New().(*apiClient)
	if _, err := client.SearchPage(ctx, server.URL, SearchPageRequest{Search: &wire.SearchRequest{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Raw(ctx, server.URL, RawRequest{Repo: "secret-repository", Path: "private/path.go"}); err != nil {
		t.Fatal(err)
	}
	root.End()
	if len(seen) != 2 || seen[0] != "/public/search/pages" || seen[1] != "/public/raw" {
		t.Fatalf("typed paths: %v", seen)
	}
	for _, span := range export.GetSpans() {
		if span.Name == "cli.invocation" {
			continue
		}
		if len(span.Attributes) != 2 {
			t.Fatalf("unsafe attributes: %+v", span)
		}
		for _, attribute := range span.Attributes {
			if strings.Contains(attribute.Value.AsString(), "private") || strings.Contains(attribute.Value.AsString(), "secret") {
				t.Fatalf("payload leaked into attributes: %+v", span.Attributes)
			}
		}
	}
}

func TestTracedSameOriginRedirectRetainsParent(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	ctx, root, _, _ := tracingTestScope(t)
	defer root.End()
	var original, redirected string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/public/repos" {
			original = req.Header.Get("traceparent")
			http.Redirect(w, req, "/canonical", http.StatusTemporaryRedirect)
			return
		}
		redirected = req.Header.Get("traceparent")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	response, err := New().(*apiClient).do(ctx, http.MethodGet, server.URL+"/public/repos", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if original == "" || original != redirected {
		t.Fatalf("same-origin header mismatch: %q %q", original, redirected)
	}
}

func TestRedirectCallbackCannotChangeTracedRequestOrigin(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", t.TempDir()+"/absent.json")
	t.Setenv("GREPPLE_TOKEN", "")
	ctx, root, _, _ := tracingTestScope(t)
	defer root.End()
	var sent atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		sent.Add(1)
		if req.Header.Get("traceparent") != "" {
			t.Error("callback redirected traceparent to different origin")
		}
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/canonical", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	transport := origin.Client()
	transport.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		url, err := url.Parse(other.URL + "/public/repos")
		if err != nil {
			return err
		}
		next.URL = url
		return nil
	}
	response, err := New(WithHTTPClient(transport)).(*apiClient).do(ctx, http.MethodGet, origin.URL+"/public/repos", "", nil)
	if err == nil {
		response.Body.Close()
		t.Fatal("cross-origin callback redirect followed")
	}
	if sent.Load() != 0 {
		t.Fatal("callback sent request to different origin")
	}
}
