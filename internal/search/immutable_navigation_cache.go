package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Only callers supplying an immutable revision opt into this process-local cache.
// It retains resolved graphs AND their read-only lookup maps/source previews.
// The byte budget is conservative estimated retention, not an exact heap limit.
var immutableRelatedLookups = immutableNavigationCache{entries: make(map[string]*immutableNavigationEntry), maxEntries: 4, maxBytes: 128 << 20, ttl: 15 * time.Minute}

type immutableNavigationEntry struct {
	ready         chan struct{}
	index         *navigationIndex
	err           error
	bytes         int64
	used, expires time.Time
}
type immutableNavigationCache struct {
	mu              sync.Mutex
	entries         map[string]*immutableNavigationEntry
	maxEntries      int
	maxBytes, bytes int64
	ttl             time.Duration
}

func immutableNavigationKey(revision string, files []string) string {
	ordered := append([]string(nil), files...)
	sort.Strings(ordered)
	hash := sha256.New()
	cwd, _ := os.Getwd()
	for _, part := range append([]string{revision, cwd}, ordered...) {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// resolve coalesces concurrent builds. Waiters can cancel; native work in the
// builder still finishes before the owning search returns, as in uncached search.
func (c *immutableNavigationCache) resolve(ctx context.Context, key string, build func() (*navigationIndex, error)) (*navigationIndex, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.expire(time.Now())
	if entry := c.entries[key]; entry != nil {
		entry.used = time.Now()
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-entry.ready:
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return entry.index, entry.err
	}
	if !c.makeRoom() {
		c.mu.Unlock()
		index, err := build()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return index, err
	}
	entry := &immutableNavigationEntry{ready: make(chan struct{}), used: time.Now()}
	c.entries[key] = entry
	c.mu.Unlock()
	index, err := build()
	cost := int64(0)
	if err == nil {
		cost = immutableNavigationCost(index)
	}
	c.mu.Lock()
	entry.index = index
	entry.err = err
	entry.bytes = cost
	entry.expires = time.Now().Add(c.ttl)
	// An oversized index is shared with existing waiters but never retained.
	if err != nil || ctx.Err() != nil || cost > c.maxBytes || !c.makeByteRoom(cost, key) {
		delete(c.entries, key)
	} else {
		c.bytes += cost
	}
	close(entry.ready)
	c.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return index, err
}

// All bookkeeping helpers run under the cache mutex; pending entries cannot be evicted.
func (c *immutableNavigationCache) expire(now time.Time) {
	for key, entry := range c.entries {
		if entry.index != nil && !now.Before(entry.expires) {
			c.remove(key)
		}
	}
}
func (c *immutableNavigationCache) remove(key string) {
	c.bytes -= c.entries[key].bytes
	delete(c.entries, key)
}
func (c *immutableNavigationCache) oldest(except string) string {
	key := ""
	var oldest time.Time
	for candidate, entry := range c.entries {
		if candidate != except && entry.index != nil && (key == "" || entry.used.Before(oldest)) {
			key = candidate
			oldest = entry.used
		}
	}
	return key
}
func (c *immutableNavigationCache) makeRoom() bool {
	for len(c.entries) >= c.maxEntries {
		key := c.oldest("")
		if key == "" {
			return false
		}
		c.remove(key)
	}
	return true
}
func (c *immutableNavigationCache) makeByteRoom(cost int64, except string) bool {
	for c.bytes+cost > c.maxBytes {
		key := c.oldest(except)
		if key == "" {
			return false
		}
		c.remove(key)
	}
	return true
}
func immutableNavigationCost(index *navigationIndex) int64 {
	graph, err := json.Marshal(index.graph)
	if err != nil {
		return 1 << 62
	}
	cost := int64(len(graph))*4 + int64(len(index.contents))*512
	for path, content := range index.contents {
		cost += int64(len(filepath.Clean(path))+len(content)) * 2
	}
	return cost
}
