package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/greppleai/grepple/internal/gritql"
	"github.com/greppleai/grepple/internal/storagepaths"
)

const relationCacheVersion = 1

type relationResultCache struct {
	Version   int               `json:"version"`
	Signature string            `json:"signature"`
	Digests   map[string]string `json:"digests"`
	Findings  []Finding         `json:"findings"`
}

func relationSourceDigests(ctx context.Context, root string, paths []string) (map[string]string, bool) {
	digests := make(map[string]string, len(paths))
	for _, path := range paths {
		if ctx.Err() != nil {
			return nil, false
		}
		digest, ok := hookFileDigest(root, path)
		if !ok {
			return nil, false
		}
		digests[path] = digest
	}
	return digests, true
}

func readRelationCache(path, signature string, digests map[string]string) ([]Finding, bool) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxHookResultCacheBytes {
		return nil, false
	}
	var saved relationResultCache
	if err := json.NewDecoder(io.LimitReader(file, maxHookResultCacheBytes+1)).Decode(&saved); err != nil || saved.Version != relationCacheVersion || saved.Signature != signature || saved.Findings == nil || !reflect.DeepEqual(saved.Digests, digests) {
		return nil, false
	}
	return saved.Findings, true
}

func writeRelationCache(path string, cache relationResultCache) {
	encoded, err := json.Marshal(cache)
	if err != nil || len(encoded) > maxHookResultCacheBytes {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".hook-relation-*")
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

// scanRelationalRulesCached validates every source's bytes and the executable
// and rule identity before reusing a whole-snapshot cross-file result. Any
// failed fingerprint forces a fresh scan; incomplete scans are never cached.
func scanRelationalRulesCached(ctx context.Context, root string, paths []string, rules []compiledRule, workers int) ([]Finding, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("incomplete relational scan: %w", err)
	}
	var found []Finding
	for _, item := range rules {
		eligible := make([]string, 0, len(paths))
		for _, source := range paths {
			if gritql.MatchesGlobs(source, item.Include, item.Exclude) {
				eligible = append(eligible, source)
			}
		}
		signature, signatureErr := hookCacheSignature([]compiledRule{item})
		digests, complete := relationSourceDigests(ctx, root, eligible)
		cachePath := filepath.Join(storagepaths.Cache(root), "hook-relation-"+item.ID+"-v1.json")
		if signatureErr == nil && complete {
			if cached, hit := readRelationCache(cachePath, signature, digests); hit {
				found = append(found, cached...)
				continue
			}
		}
		fresh, err := scanRelationalRules(ctx, root, paths, []compiledRule{item}, workers)
		if err != nil {
			return nil, err
		}
		found = append(found, fresh...)
		if signatureErr == nil && complete {
			after, ok := relationSourceDigests(ctx, root, eligible)
			if !ok || !reflect.DeepEqual(after, digests) {
				return nil, fmt.Errorf("hook %s: source changed during relational scan", item.ID)
			}
			writeRelationCache(cachePath, relationResultCache{Version: relationCacheVersion, Signature: signature, Digests: digests, Findings: append([]Finding{}, fresh...)})
		}
	}
	return found, nil
}
