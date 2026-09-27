package hook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/internal/storagepaths"
)

const hookResultCacheVersion = 1
const maxHookResultCacheBytes = 8 << 20
const maxCachedSourceBytes = 10 << 20

type cachedFile struct {
	Digest   string    `json:"digest"`
	Findings []Finding `json:"findings,omitempty"`
}

type hookResultCache struct {
	Version   int                   `json:"version"`
	Signature string                `json:"signature"`
	Files     map[string]cachedFile `json:"files"`
}

func hookCacheSignature(rules []compiledRule) (string, error) {
	// The binary identity invalidates cached results when evaluator semantics
	// change even without a config or compatibility-version change.
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	binaryFile, err := os.Open(binary)
	if err != nil {
		return "", err
	}
	binaryHash := sha256.New()
	_, readErr := io.Copy(binaryHash, binaryFile)
	closeErr := binaryFile.Close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	hash := sha256.New()
	_, _ = hash.Write(binaryHash.Sum(nil))
	for _, rule := range rules {
		encoded, err := json.Marshal(rule.rule)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(encoded)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readHookResultCache(path, signature string) hookResultCache {
	fresh := hookResultCache{Version: hookResultCacheVersion, Signature: signature, Files: make(map[string]cachedFile)}
	file, err := os.Open(path)
	if err != nil {
		return fresh
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxHookResultCacheBytes {
		return fresh
	}
	var saved hookResultCache
	if err := json.NewDecoder(io.LimitReader(file, maxHookResultCacheBytes+1)).Decode(&saved); err != nil || saved.Version != fresh.Version || saved.Signature != signature || saved.Files == nil {
		return fresh
	}
	return saved
}

func writeHookResultCache(path string, cache hookResultCache) {
	encoded, err := json.Marshal(cache)
	if err != nil || len(encoded) > maxHookResultCacheBytes {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".hook-cache-*")
	if err != nil {
		return
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(encoded); err != nil {
		temp.Close()
		return
	}
	if err := temp.Close(); err == nil {
		_ = os.Rename(temp.Name(), path)
	}
}

func hookFileDigest(root, path string) (string, bool) {
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return "", false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCachedSourceBytes {
		return "", false
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, maxCachedSourceBytes+1)); err != nil {
		return "", false
	}
	return hex.EncodeToString(hash.Sum(nil)), true
}

func scanRules(ctx context.Context, root string, paths []string, rules []compiledRule, all bool, workers int) ([]Finding, error) {
	signature, err := hookCacheSignature(rules)
	if err != nil {
		return scanRulesUncached(ctx, root, paths, rules, all, workers)
	}
	cachePath := filepath.Join(storagepaths.Cache(root), "hook-results-v1.json")
	cache := readHookResultCache(cachePath, signature)
	found := make([]Finding, 0)
	pending := make([]string, 0)
	fingerprints := make(map[string]string, len(paths))
	for _, path := range paths {
		digest, ok := hookFileDigest(root, path)
		if !ok {
			pending = append(pending, path)
			continue
		}
		if cached, hit := cache.Files[path]; hit && cached.Digest == digest {
			found = append(found, cached.Findings...)
			continue
		}
		fingerprints[path] = digest
		pending = append(pending, path)
	}
	if len(pending) > 0 {
		fresh, err := scanRulesUncached(ctx, root, pending, rules, all, workers)
		if err != nil {
			return nil, err
		}
		found = append(found, fresh...)
		byPath := make(map[string][]Finding, len(fresh))
		for _, item := range fresh {
			byPath[item.Path] = append(byPath[item.Path], item)
		}
		for _, path := range pending {
			before := fingerprints[path]
			if before == "" {
				continue
			}
			after, ok := hookFileDigest(root, path)
			if ok && before == after {
				cache.Files[path] = cachedFile{Digest: before, Findings: byPath[path]}
			}
		}
		writeHookResultCache(cachePath, cache)
	}
	sort.Slice(found, func(i, j int) bool {
		left, right := found[i], found[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.ID < right.ID
	})
	return found, nil
}
