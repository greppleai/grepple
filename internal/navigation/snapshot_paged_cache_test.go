package navigation

import "testing"

func TestPagedLRUEvictsColdPagesWithoutFlushingHotPage(t *testing.T) {
	cache := newPagedCache[[]byte](1024)
	hot := pagedPage{"hot", 0}
	cache.put(hot, []byte("hot"), 3)
	for i := uint32(0); i < 100; i++ {
		if _, ok := cache.get(hot); !ok {
			t.Fatal("hot page flushed")
		}
		cache.put(pagedPage{"cold", i}, make([]byte, 200), 200)
		if cache.bytes > cache.limit || len(cache.entries) > 1024 {
			t.Fatal("cache grew beyond budget")
		}
	}
	other := newPagedCache[[]byte](1024)
	if _, ok := other.get(hot); ok {
		t.Fatal("cache shared across queries")
	}
}
