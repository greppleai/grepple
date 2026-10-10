package navigation

import "container/list"

// pagedCache is bounded scratch owned by one query, never shared by leases.
type pagedCache[T any] struct {
	entries      map[pagedPage]*list.Element
	order        *list.List
	bytes, limit int
}
type pagedCacheEntry[T any] struct {
	key   pagedPage
	value T
	size  int
}

func newPagedCache[T any](limit int) *pagedCache[T] {
	return &pagedCache[T]{entries: make(map[pagedPage]*list.Element), order: list.New(), limit: limit}
}
func (c *pagedCache[T]) get(key pagedPage) (T, bool) {
	if element, ok := c.entries[key]; ok {
		c.order.MoveToFront(element)
		return element.Value.(pagedCacheEntry[T]).value, true
	}
	var zero T
	return zero, false
}
func (c *pagedCache[T]) put(key pagedPage, value T, size int) {
	size = max(size, 0) + 128
	if size > c.limit {
		return
	}
	if element, ok := c.entries[key]; ok {
		c.bytes -= element.Value.(pagedCacheEntry[T]).size
		c.order.Remove(element)
		delete(c.entries, key)
	}
	for c.bytes+size > c.limit || len(c.entries) >= 1024 {
		element := c.order.Back()
		entry := element.Value.(pagedCacheEntry[T])
		delete(c.entries, entry.key)
		c.bytes -= entry.size
		c.order.Remove(element)
	}
	c.entries[key] = c.order.PushFront(pagedCacheEntry[T]{key: key, value: value, size: size})
	c.bytes += size
}
