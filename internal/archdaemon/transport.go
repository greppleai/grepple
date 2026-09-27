// Package archdaemon provides an optional authenticated user-level architecture cache.
package archdaemon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/storagepaths"
)

const protocol = "grepple-architecture-daemon-v2"
const maxDescriptorBytes = 4096
const maxRequestBytes = 1 << 20
const maxResponseBytes = 128 << 20

type descriptor struct {
	Protocol string `json:"protocol"`
	Version  string `json:"version"`
	Address  string `json:"address"`
	Token    string `json:"token"`
}

type request struct {
	Root       string                       `json:"root"`
	Paths      []string                     `json:"paths"`
	MaxFiles   int                          `json:"maxFiles"`
	Key        string                       `json:"key,omitempty"`
	Kind       string                       `json:"kind,omitempty"`
	Report     *analysis.ArchitectureReport `json:"report,omitempty"`
	GraphQuery *analysis.GraphQuery         `json:"graphQuery,omitempty"`
	Graph      *analysis.GraphReport        `json:"graph,omitempty"`
	Selection  *ResolveSelection            `json:"selection,omitempty"`
	Projection *ResolveProjection           `json:"projection,omitempty"`
}

type response struct {
	Report     analysis.ArchitectureReport `json:"report"`
	Graph      *analysis.GraphReport       `json:"graph,omitempty"`
	Projection *ResolveProjection          `json:"projection,omitempty"`
}

func descriptorPath() string {
	return filepath.Join(storagepaths.DaemonCache(), "daemon.json")
}

func readDescriptor() (descriptor, bool) {
	if !permittedCacheDirectory() {
		return descriptor{}, false
	}
	path := descriptorPath()
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDescriptorBytes || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return descriptor{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > maxDescriptorBytes {
		return descriptor{}, false
	}
	var details descriptor
	if json.Unmarshal(content, &details) != nil || details.Protocol != protocol || details.Version != sourceIdentity() || !loopbackAddress(details.Address) {
		return descriptor{}, false
	}
	token, err := hex.DecodeString(details.Token)
	return details, err == nil && len(token) == 32
}

func loopbackAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	return err == nil && host == "127.0.0.1" && port != "0" && port != ""
}

func newToken() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return hex.EncodeToString(secret), nil
}

func localClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext,
	}}
}

// Query returns a previously published architecture report. A cache miss or
// incompatible worker is not an error: the client must build locally.
func Query(paths []string, maxFiles int) (analysis.ArchitectureReport, bool) {
	root, err := os.Getwd()
	if err != nil {
		return analysis.ArchitectureReport{}, false
	}
	body, ok := postDaemon("/architecture", request{Root: root, Paths: paths, MaxFiles: maxFiles}, maxRequestBytes)
	if !ok {
		return analysis.ArchitectureReport{}, false
	}
	var result response
	if json.Unmarshal(body, &result) != nil || result.Report.Schema != analysis.ArchitectureSchema {
		return analysis.ArchitectureReport{}, false
	}
	return result.Report, true
}

// Key fingerprints the caller's already-read sources before it builds the
// report. The worker independently rechecks this key before publishing it.
func Key(paths []string, maxFiles int, sources []analysis.Source) (string, bool) {
	if _, ok := readDescriptor(); !ok {
		return "", false
	}
	root, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return architectureFingerprint(root, paths, maxFiles, sources)
}

// Store best-effort publishes a locally constructed report to the shared cache.
func Store(paths []string, maxFiles int, key string, report analysis.ArchitectureReport) bool {
	if key == "" || report.Schema != analysis.ArchitectureSchema {
		return false
	}
	root, err := os.Getwd()
	if err != nil {
		return false
	}
	_, ok := postDaemon("/store", request{Root: root, Paths: paths, MaxFiles: maxFiles, Key: key, Report: &report}, maxResponseBytes)
	return ok
}

func postDaemon(route string, payload request, maxBody int64) ([]byte, bool) {
	details, ok := readDescriptor()
	if !ok {
		return nil, false
	}
	encoded, err := json.Marshal(payload)
	if err != nil || int64(len(encoded)) > maxBody {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+details.Address+route, bytes.NewReader(encoded))
	if err != nil {
		return nil, false
	}
	httpRequest.Header.Set("X-Grepple-Token", details.Token)
	httpRequest.Header.Set("Content-Type", "application/json")
	client := localClient()
	defer client.CloseIdleConnections()
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		return nil, false
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK || httpResponse.ContentLength > maxResponseBytes {
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	return body, err == nil && len(body) <= maxResponseBytes
}

func permittedCacheDirectory() bool {
	path := storagepaths.DaemonCache()
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir() && (runtime.GOOS == "windows" || info.Mode().Perm()&0o077 == 0)
}
