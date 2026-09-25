package archdaemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/greppleai/grepple/analysis"
)

type service struct {
	token string
	mu    sync.Mutex
	cache reportCache
}

// Serve runs one foreground, user-level report cache until ctx is cancelled.
// It does not inspect any repository until an authenticated client requests it.
func Serve(ctx context.Context) error {
	if !permittedCacheDirectory() {
		return fmt.Errorf("greppled requires a private absolute user cache directory")
	}
	if existing, ok := readDescriptor(); ok && daemonAlive(existing) {
		return fmt.Errorf("greppled is already running")
	}
	lockPath := descriptorPath() + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		info, statErr := os.Stat(lockPath)
		if statErr == nil && time.Since(info.ModTime()) > 30*time.Second {
			_ = os.Remove(lockPath)
			lock, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
	}
	if err != nil {
		return fmt.Errorf("greppled startup lock: %w", err)
	}
	defer os.Remove(lockPath)
	_ = lock.Close()
	if existing, ok := readDescriptor(); ok && daemonAlive(existing) {
		return fmt.Errorf("greppled is already running")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	token, err := newToken()
	if err != nil {
		return err
	}
	details := descriptor{Protocol: protocol, Version: sourceIdentity(), Address: listener.Addr().String(), Token: token}
	if err := publishDescriptor(details); err != nil {
		return err
	}
	_ = os.Remove(lockPath)
	defer clearDescriptor(token)
	worker := &service{token: token, cache: newReportCache()}
	server := &http.Server{
		Handler: worker, ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 120 * time.Second,
		IdleTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *service) ServeHTTP(writer http.ResponseWriter, httpRequest *http.Request) {
	if subtle.ConstantTimeCompare([]byte(httpRequest.Header.Get("X-Grepple-Token")), []byte(s.token)) != 1 {
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
		return
	}
	if httpRequest.URL.Path == "/health" && httpRequest.Method == http.MethodGet {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if httpRequest.Method != http.MethodPost || (httpRequest.URL.Path != "/architecture" && httpRequest.URL.Path != "/graph" && httpRequest.URL.Path != "/resolve" && httpRequest.URL.Path != "/store") {
		http.NotFound(writer, httpRequest)
		return
	}
	maximum := int64(maxRequestBytes)
	if httpRequest.URL.Path == "/store" {
		maximum = maxResponseBytes
	}
	body := http.MaxBytesReader(writer, httpRequest.Body, maximum)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request request
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF || request.MaxFiles < 0 {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	if httpRequest.URL.Path == "/store" {
		if err := s.store(request); err != nil {
			http.Error(writer, "report could not be cached", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusOK)
		return
	}
	var result response
	var hit bool
	switch httpRequest.URL.Path {
	case "/architecture":
		result.Report, hit = s.architecture(request)
	case "/graph":
		result, hit = s.variant(request, graphVariant)
	case "/resolve":
		result, hit = s.variant(request, resolveVariant)
	}
	if !hit {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	content, err := json.Marshal(result)
	if err != nil || len(content) > maxResponseBytes {
		http.Error(writer, "report exceeds response limit", http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(content)
}

func (s *service) architecture(request request) (analysis.ArchitectureReport, bool) {
	sources, key, stable, err := architectureSnapshot(request.Root, request.Paths, request.MaxFiles)
	if err != nil || !stable {
		return analysis.ArchitectureReport{}, false
	}
	_ = sources
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cache.get(request.Root, key)
}

func (s *service) store(payload request) error {
	kind := payload.Kind
	if kind == "" {
		kind = architectureVariant
	}
	if len(payload.Key) != 64 {
		return fmt.Errorf("invalid report key")
	}
	var result response
	switch kind {
	case architectureVariant:
		if payload.Report == nil || payload.Report.Schema != analysis.ArchitectureSchema {
			return fmt.Errorf("invalid architecture report")
		}
		result.Report = *payload.Report
	case graphVariant:
		if payload.Graph == nil || payload.Graph.Schema != analysis.GraphSchema {
			return fmt.Errorf("invalid graph report")
		}
		result.Graph = payload.Graph
	case resolveVariant:
		if payload.Projection == nil || payload.Projection.Schema != ResolveProjectionSchema {
			return fmt.Errorf("invalid resolve projection")
		}
		result.Projection = payload.Projection
	default:
		return fmt.Errorf("invalid report kind")
	}
	_, base, stable, err := architectureSnapshot(payload.Root, payload.Paths, payload.MaxFiles)
	if err != nil || !stable {
		return fmt.Errorf("source snapshot changed")
	}
	key, ok := selectedVariantKey(base, payload, kind)
	if !ok || key != payload.Key {
		return fmt.Errorf("report selection or snapshot changed")
	}
	content, err := json.Marshal(result)
	if err != nil || len(content) > maxResponseBytes {
		return fmt.Errorf("report exceeds cache limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Guard against a mutation between the initial read and publication.
	_, after, unchanged, err := architectureSnapshot(payload.Root, payload.Paths, payload.MaxFiles)
	if err != nil || !unchanged || after != base {
		return fmt.Errorf("source snapshot changed during publication")
	}
	switch kind {
	case architectureVariant:
		return s.cache.put(payload.Root, key, *payload.Report, len(content))
	case graphVariant:
		return s.cache.putVariant(payload.Root, kind, key, *payload.Graph, len(content))
	default:
		return s.cache.putVariant(payload.Root, kind, key, *payload.Projection, len(content))
	}
}

func daemonAlive(details descriptor) bool {
	client := localClient()
	client.Timeout = time.Second
	defer client.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodGet, "http://"+details.Address+"/health", nil)
	if err != nil {
		return false
	}
	request.Header.Set("X-Grepple-Token", details.Token)
	result, err := client.Do(request)
	if err != nil {
		return false
	}
	_ = result.Body.Close()
	return result.StatusCode == http.StatusNoContent
}

func publishDescriptor(details descriptor) error {
	path := descriptorPath()
	content, err := json.Marshal(details)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".daemon-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

func clearDescriptor(token string) {
	if details, ok := readDescriptor(); ok && details.Token == token {
		_ = os.Remove(descriptorPath())
	}
}
