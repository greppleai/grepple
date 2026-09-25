package archdaemon

import (
	"container/list"
	"fmt"

	"github.com/greppleai/grepple/analysis"
)

const maxCachedRoots = 4
const maxCachedVariantsPerRoot = 8
const maxCachedReportBytes = 64 << 20

const architectureVariant = "architecture"
const graphVariant = "graph"
const resolveVariant = "resolve"

type cachedReport struct {
	root, key, kind string
	value           any
	bytes           int
}

type reportCache struct {
	byRoot map[string]map[string]*list.Element
	order  *list.List
	bytes  int
}

func newReportCache() reportCache {
	return reportCache{byRoot: make(map[string]map[string]*list.Element), order: list.New()}
}

func variantID(kind, key string) string { return kind + ":" + key }

func (cache *reportCache) get(root, key string) (analysis.ArchitectureReport, bool) {
	value, ok := cache.getVariant(root, architectureVariant, key)
	if !ok {
		return analysis.ArchitectureReport{}, false
	}
	report, ok := value.(analysis.ArchitectureReport)
	return report, ok
}

func (cache *reportCache) put(root, key string, report analysis.ArchitectureReport, size int) error {
	return cache.putVariant(root, architectureVariant, key, report, size)
}

func (cache *reportCache) getVariant(root, kind, key string) (any, bool) {
	entry := cache.byRoot[root][variantID(kind, key)]
	if entry == nil {
		return nil, false
	}
	cache.order.MoveToFront(entry)
	return entry.Value.(cachedReport).value, true
}

func (cache *reportCache) putVariant(root, kind, key string, value any, size int) error {
	if size < 0 || size > maxCachedReportBytes {
		return fmt.Errorf("report exceeds cache memory budget")
	}
	entries := cache.byRoot[root]
	if entries == nil {
		entries = make(map[string]*list.Element)
		cache.byRoot[root] = entries
	}
	id := variantID(kind, key)
	if previous := entries[id]; previous != nil {
		cache.evict(previous)
		entries = cache.byRoot[root]
		if entries == nil {
			entries = make(map[string]*list.Element)
			cache.byRoot[root] = entries
		}
	}
	entry := cache.order.PushFront(cachedReport{root: root, key: key, kind: kind, value: value, bytes: size})
	entries[id] = entry
	cache.bytes += size
	for len(entries) > maxCachedVariantsPerRoot {
		for oldest := cache.order.Back(); oldest != nil; oldest = oldest.Prev() {
			if oldest.Value.(cachedReport).root == root {
				cache.evict(oldest)
				break
			}
		}
	}
	for len(cache.byRoot) > maxCachedRoots {
		// Root recency is its most recently touched variant, not its oldest one.
		seen := make(map[string]bool, len(cache.byRoot))
		oldestRoot := ""
		for item := cache.order.Front(); item != nil; item = item.Next() {
			candidate := item.Value.(cachedReport).root
			if !seen[candidate] {
				seen[candidate] = true
				oldestRoot = candidate
			}
		}
		for _, item := range cache.byRoot[oldestRoot] {
			cache.evict(item)
		}
	}
	for cache.bytes > maxCachedReportBytes {
		cache.evict(cache.order.Back())
	}
	return nil
}

func (cache *reportCache) evict(entry *list.Element) {
	if entry == nil {
		return
	}
	value := entry.Value.(cachedReport)
	entries := cache.byRoot[value.root]
	delete(entries, variantID(value.kind, value.key))
	if len(entries) == 0 {
		delete(cache.byRoot, value.root)
	}
	cache.bytes -= value.bytes
	cache.order.Remove(entry)
}
