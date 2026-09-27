package hook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/gritql"
	"github.com/greppleai/grepple/internal/storagepaths"
	"github.com/greppleai/grepple/internal/parser"
)

func validateMetricMessage(message string, query *gritql.MetricQuery) error {
	if query == nil {
		return fmt.Errorf("missing metric query")
	}
	for _, match := range relationPlaceholder.FindAllStringSubmatch(message, -1) {
		switch match[1] {
		case "score", "above":
		case "name":
			if query.Spec.NameField == "" {
				return fmt.Errorf("metric message uses name without a scope name field")
			}
		default:
			return fmt.Errorf("unknown metric message placeholder %q", match[1])
		}
	}
	unmatched := relationPlaceholder.ReplaceAllString(message, "")
	if strings.Contains(unmatched, "{{") || strings.Contains(unmatched, "}}") {
		return fmt.Errorf("invalid metric message placeholder")
	}
	return nil
}

func renderMetricMessage(message string, query *gritql.MetricQuery, result gritql.MetricResult) string {
	return relationPlaceholder.ReplaceAllStringFunc(message, func(token string) string {
		switch token[2 : len(token)-2] {
		case "name":
			return result.Name
		case "score":
			return strconv.Itoa(result.Score)
		case "above":
			return strconv.Itoa(query.Above)
		default:
			return token // unreachable after configuration validation
		}
	})
}

func metricApplies(path string, rule compiledRule) bool {
	return parser.LanguageFor(path) == rule.metric.Spec.Scope.Language() && gritql.MatchesGlobs(path, rule.Include, rule.Exclude)
}

// scanMetricRulesCached keeps the same content-validated per-file cache contract
// as structural hooks. The complete selected metric set is keyed by rule and
// executable identity; no failed or incomplete file is recorded as clean.
func scanMetricRulesCached(ctx context.Context, root string, paths []string, rules []compiledRule, all bool) ([]Finding, error) {
	deadline := 30 * time.Second
	if all {
		deadline = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	if len(paths) > maxSharedScanFiles {
		return nil, fmt.Errorf("metric scan exceeds file ceiling")
	}
	signature, err := hookCacheSignature(rules)
	if err != nil {
		return scanMetricRules(ctx, root, paths, rules, nil)
	}
	cachePath := filepath.Join(storagepaths.Cache(root), "hook-metrics-v1.json")
	cache := readHookResultCache(cachePath, signature)
	results, err := scanMetricRules(ctx, root, paths, rules, &cache)
	if err != nil {
		return nil, err
	}
	writeHookResultCache(cachePath, cache)
	return results, nil
}

func scanMetricRules(ctx context.Context, root string, paths []string, rules []compiledRule, cache *hookResultCache) ([]Finding, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("incomplete metric scan: open repository: %w", err)
	}
	defer rootHandle.Close()
	found := make([]Finding, 0)
	var totalBytes int64
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("incomplete metric scan: %w", err)
		}
		applicable := make([]compiledRule, 0, len(rules))
		for _, rule := range rules {
			if metricApplies(path, rule) {
				applicable = append(applicable, rule)
			}
		}
		if len(applicable) == 0 {
			continue
		}
		digest, valid := metricFileDigest(rootHandle, path)
		if !valid {
			return nil, fmt.Errorf("incomplete metric scan: cannot fingerprint %s", path)
		}
		if cache != nil {
			if old, ok := cache.Files[path]; ok && old.Digest == digest {
				found = append(found, old.Findings...)
				if len(found) > maxSharedRuleResults {
					return nil, fmt.Errorf("metric finding ceiling exceeded")
				}
				continue
			}
		}
		info, err := rootHandle.Lstat(filepath.FromSlash(path))
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("incomplete metric scan: source %s is not a regular file: %v", path, err)
		}
		file, err := rootHandle.Open(filepath.FromSlash(path))
		if err != nil {
			return nil, fmt.Errorf("incomplete metric scan: open %s: %w", path, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxCachedSourceBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("incomplete metric scan: read %s: %w", path, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("incomplete metric scan: close %s: %w", path, closeErr)
		}
		if len(content) > maxCachedSourceBytes {
			return nil, fmt.Errorf("incomplete metric scan: source %s exceeds 10 MiB limit", path)
		}
		actual := sha256.Sum256(content)
		if hex.EncodeToString(actual[:]) != digest {
			return nil, fmt.Errorf("incomplete metric scan: source %s changed during read", path)
		}
		totalBytes += int64(len(content))
		if totalBytes > maxSharedScanBytes {
			return nil, fmt.Errorf("incomplete metric scan: aggregate byte limit exceeded")
		}
		document, err := parser.ParseDocument(parser.LanguageFor(path), string(content))
		if err != nil {
			return nil, fmt.Errorf("incomplete metric scan: parse %s: %w", path, err)
		}
		fresh := make([]Finding, 0)
		for _, rule := range applicable {
			scores, evaluationErr := gritql.AnalyzeMetrics(ctx, rule.metric.Spec, document, gritql.EvaluateOptions{})
			if evaluationErr != nil {
				document.Close()
				var failure *gritql.EvaluationError
				if errors.As(evaluationErr, &failure) {
					return nil, fmt.Errorf("incomplete metric scan: %s: %s: %s: %w", rule.ID, path, failure.Code, evaluationErr)
				}
				return nil, fmt.Errorf("incomplete metric scan: %s: %s: %w", rule.ID, path, evaluationErr)
			}
			for _, score := range scores {
				if score.Score > rule.metric.Above {
					fresh = append(fresh, Finding{ID: rule.ID, Path: path, Line: score.Range.Start.Line, Column: score.Range.Start.Column, Severity: rule.Severity, Message: renderMetricMessage(rule.Message, rule.metric, score)})
				}
			}
		}
		document.Close()
		found = append(found, fresh...)
		if len(found) > maxSharedRuleResults {
			return nil, fmt.Errorf("metric finding ceiling exceeded")
		}
		if cache != nil {
			after, ok := metricFileDigest(rootHandle, path)
			if !ok || after != digest {
				return nil, fmt.Errorf("incomplete metric scan: source %s changed during evaluation", path)
			}
			cache.Files[path] = cachedFile{Digest: digest, Findings: fresh}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("incomplete metric scan: %w", err)
	}
	return found, nil
}

func metricFileDigest(root *os.Root, path string) (string, bool) {
	info, err := root.Lstat(filepath.FromSlash(path))
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCachedSourceBytes {
		return "", false
	}
	file, err := root.Open(filepath.FromSlash(path))
	if err != nil {
		return "", false
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCachedSourceBytes {
		return "", false
	}
	hash := sha256.New()
	read, err := io.Copy(hash, io.LimitReader(file, maxCachedSourceBytes+1))
	if err != nil || read > maxCachedSourceBytes {
		return "", false
	}
	return hex.EncodeToString(hash.Sum(nil)), true
}
