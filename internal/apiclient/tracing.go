package apiclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/greppleai/grepple/internal/clitelemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// traceRequest only uses an explicit active invocation span. It never reads
// global SDK configuration and never exports the request URL, error or body.
func traceRequest(request *http.Request, category string, propagate bool) (*http.Request, func(*http.Response, error)) {
	parent := trace.SpanFromContext(request.Context())
	if !clitelemetry.Enabled(request.Context()) || !parent.SpanContext().IsValid() || category == "" {
		return request, func(*http.Response, error) {}
	}
	ctx, span := parent.TracerProvider().Tracer("grepple.cli.http").Start(request.Context(), "cli.http."+category, trace.WithSpanKind(trace.SpanKindClient))
	request = request.WithContext(ctx)
	if propagate {
		sc := span.SpanContext()
		if sc.IsValid() {
			flags := "00"
			if sc.IsSampled() {
				flags = "01"
			}
			request.Header.Set("traceparent", fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), sc.SpanID(), flags))
		}
	}
	return request, func(response *http.Response, err error) {
		method := "other"
		switch request.Method {
		case http.MethodGet, http.MethodPost, http.MethodDelete:
			method = request.Method
		}
		span.SetAttributes(attribute.String("http.request.method", method))
		switch {
		case errors.Is(err, context.Canceled):
			span.SetAttributes(attribute.String("grepple.outcome", "canceled"))
			span.SetStatus(codes.Error, "canceled")
		case errors.Is(err, context.DeadlineExceeded):
			span.SetAttributes(attribute.String("grepple.outcome", "timeout"))
			span.SetStatus(codes.Error, "timeout")
		case err != nil:
			span.SetAttributes(attribute.String("grepple.outcome", "network_error"))
			span.SetStatus(codes.Error, "network_error")
		case response != nil:
			statusClass := fmt.Sprintf("%dxx", response.StatusCode/100)
			if response.StatusCode < 100 || response.StatusCode >= 600 {
				statusClass = "other"
			}
			span.SetAttributes(attribute.String("http.response.status_class", statusClass))
			if response.StatusCode >= 400 {
				span.SetStatus(codes.Error, "http_error")
			}
		}
		span.End()
	}
}

// Only fixed backend operations participate. Rules have a dynamic ID but the
// span name never incorporates it. Auth bootstrap/logout get spans but no parent.
func backendCategory(target *url.URL) string {
	if target == nil || target.User != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return ""
	}
	switch target.Path {
	case "/public/search", "/public/search/pages":
		return "search"
	case "/public/navigation/resolve":
		return "navigation"
	case "/public/grit":
		return "grit"
	case "/public/analysis":
		return "analysis"
	case "/public/raw":
		return "raw"
	case "/public/tree":
		return "tree"
	case "/public/repos":
		return "repos"
	case "/public/rules":
		return "rules"
	}
	if tail, ok := strings.CutPrefix(target.Path, "/public/rules/"); ok {
		if tail != "" && !strings.Contains(tail, "/") {
			return "rules"
		}
		if id, ok := strings.CutSuffix(tail, "/results"); ok && id != "" && !strings.Contains(id, "/") {
			return "rules"
		}
	}
	return ""
}
