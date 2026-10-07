package agentskills

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Source reads bounded repository-relative content without executing it.
type Source interface {
	Read(context.Context, string) ([]byte, error)
}

type remoteSource struct {
	base   string
	client *http.Client
}

// NewRemoteSource pins downloads to one exact tag in the Grepple repository.
func NewRemoteSource(tag string) (Source, error) {
	exact, err := ReleaseTag(tag)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return &remoteSource{base: "https://raw.githubusercontent.com/greppleai/grepple/" + url.PathEscape(exact) + "/", client: client}, nil
}
func (source *remoteSource) Read(ctx context.Context, file string) ([]byte, error) {
	if !safeRelative(file) {
		return nil, fmt.Errorf("invalid skill source path")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.base+file, nil)
	if err != nil {
		return nil, err
	}
	response, err := source.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download skill source %s failed: %w", file, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("skill source %s returned HTTP %d; this release may not contain setup skills", file, response.StatusCode)
	}
	return readBounded(response.Body)
}

type localSource struct{ root string }

// NewLocalSource is an explicit development-only alternative to release downloads.
func NewLocalSource(root string) (Source, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("skill source must be a real directory")
	}
	return &localSource{root: absolute}, nil
}
func (source *localSource) Read(ctx context.Context, file string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := confinedPath(source.root, file)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("skill source is not a regular file: %s", file)
	}
	reader, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readBounded(reader)
}
func readBounded(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("skill source exceeds file size limit")
	}
	return data, nil
}
func confinedPath(root, relative string) (string, error) {
	if !safeRelative(relative) {
		return "", fmt.Errorf("invalid skill path")
	}
	current := root
	current = filepath.Join(root, filepath.FromSlash(relative))
	for walk := current; walk != root; walk = filepath.Dir(walk) {
		info, err := os.Lstat(walk)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing skill symlink: %s", relative)
		}
	}
	return current, nil
}
