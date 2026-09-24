// Package archdaemon provides an optional, local, authenticated architecture worker.
package archdaemon

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/greppleai/grepple/analysis"
	"github.com/greppleai/grepple/internal/storagepaths"
)

const protocol = "grepple-architecture-daemon-v1"
const maxDescriptorBytes = 4096
const maxRequestBytes = 1 << 20
const maxResponseBytes = 128 << 20

type descriptor struct {
	Protocol string `json:"protocol"`
	Root     string `json:"root"`
	Version  string `json:"version"`
	Address  string `json:"address"`
	Token    string `json:"token"`
}

type request struct {
	Paths    []string `json:"paths"`
	MaxFiles int      `json:"maxFiles"`
}

type response struct {
	Report analysis.ArchitectureReport `json:"report"`
}

func descriptorPath(root string) string {
	key := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(storagepaths.Cache(root), "daemon-"+hex.EncodeToString(key[:8])+".json")
}

func readDescriptor(root string) (descriptor, bool) {
	if !permittedCacheDirectory(root) {
		return descriptor{}, false
	}
	path := descriptorPath(root)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDescriptorBytes || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return descriptor{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil || len(content) > maxDescriptorBytes {
		return descriptor{}, false
	}
	var details descriptor
	if json.Unmarshal(content, &details) != nil || details.Protocol != protocol || details.Root != root || details.Version != sourceIdentity() || !loopbackAddress(details.Address) {
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

// Query obtains an architecture report only from the authenticated worker for
// this directory. Any unavailable or incompatible daemon falls back to the CLI.
func Query(paths []string, maxFiles int) (analysis.ArchitectureReport, bool) {
	root, err := os.Getwd()
	if err != nil {
		return analysis.ArchitectureReport{}, false
	}
	details, ok := readDescriptor(root)
	if !ok {
		return analysis.ArchitectureReport{}, false
	}
	encoded, err := json.Marshal(request{Paths: paths, MaxFiles: maxFiles})
	if err != nil || len(encoded) > maxRequestBytes {
		return analysis.ArchitectureReport{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+details.Address+"/architecture", bytes.NewReader(encoded))
	if err != nil {
		return analysis.ArchitectureReport{}, false
	}
	httpRequest.Header.Set("X-Grepple-Token", details.Token)
	httpRequest.Header.Set("Content-Type", "application/json")
	client := localClient()
	defer client.CloseIdleConnections()
	httpResponse, err := client.Do(httpRequest)
	if err != nil {
		return analysis.ArchitectureReport{}, false
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK || httpResponse.ContentLength > maxResponseBytes {
		return analysis.ArchitectureReport{}, false
	}
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return analysis.ArchitectureReport{}, false
	}
	var result response
	if json.Unmarshal(body, &result) != nil || result.Report.Schema != analysis.ArchitectureSchema {
		return analysis.ArchitectureReport{}, false
	}
	return result.Report, true
}

func permittedCacheDirectory(root string) bool {
	path := filepath.Dir(descriptorPath(root))
	info, err := os.Stat(path)
	return err == nil && info.IsDir() && (runtime.GOOS == "windows" || info.Mode().Perm()&0o077 == 0)
}
