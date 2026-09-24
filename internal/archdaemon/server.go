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
	root   string
	token  string
	mu     sync.Mutex
	key    string
	report analysis.ArchitectureReport
}

// Serve runs a foreground worker for the current directory until ctx is cancelled.
// It never changes process-global working directory or standard streams.
func Serve(ctx context.Context) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if !permittedCacheDirectory(root) {
		return fmt.Errorf("greppled requires a private cache directory")
	}
	if existing, ok := readDescriptor(root); ok && daemonAlive(existing) {
		return fmt.Errorf("greppled is already running for %s", root)
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
	details := descriptor{Protocol: protocol, Root: root, Version: sourceIdentity(), Address: listener.Addr().String(), Token: token}
	if err := publishDescriptor(root, details); err != nil {
		return err
	}
	defer clearDescriptor(root, token)
	worker := &service{root: root, token: token}
	server := &http.Server{
		Handler:           worker,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       10 * time.Second,
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
	if httpRequest.URL.Path != "/architecture" || httpRequest.Method != http.MethodPost {
		http.NotFound(writer, httpRequest)
		return
	}
	body := http.MaxBytesReader(writer, httpRequest.Body, maxRequestBytes)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request request
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF || request.MaxFiles < 0 {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	report, err := s.architecture(request)
	if err != nil {
		http.Error(writer, "source selection changed or could not be read", http.StatusServiceUnavailable)
		return
	}
	content, err := json.Marshal(response{Report: report})
	if err != nil || len(content) > maxResponseBytes {
		http.Error(writer, "report exceeds response limit", http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(content)
}

func (s *service) architecture(request request) (analysis.ArchitectureReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sources, key, cacheable, err := architectureSnapshot(s.root, request.Paths, request.MaxFiles)
	if err != nil {
		return analysis.ArchitectureReport{}, err
	}
	if cacheable && key == s.key {
		return s.report, nil
	}
	universe, err := analysis.NewUniverse(sources, request.MaxFiles)
	if err != nil {
		return analysis.ArchitectureReport{}, err
	}
	report := analysis.BuildArchitecture(universe)
	universe.Close()
	if cacheable {
		_, after, stable, err := architectureSnapshot(s.root, request.Paths, request.MaxFiles)
		if err != nil || !stable || after != key {
			return analysis.ArchitectureReport{}, fmt.Errorf("sources changed while building")
		}
		s.key, s.report = key, report
	} else {
		s.key, s.report = "", analysis.ArchitectureReport{}
	}
	return report, nil
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

func publishDescriptor(root string, details descriptor) error {
	path := descriptorPath(root)
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

func clearDescriptor(root, token string) {
	if details, ok := readDescriptor(root); ok && details.Token == token {
		_ = os.Remove(descriptorPath(root))
	}
}
