package router

import (
	"encoding/binary"
	"fmt"
	"sort"
)

func murmur3(data []byte, seed uint32) uint32 {
	const c1 uint32 = 0xcc9e2d51
	const c2 uint32 = 0x1b873593
	h := seed
	blocks := len(data) / 4
	for i := 0; i < blocks; i++ {
		k := binary.LittleEndian.Uint32(data[i*4:])
		k *= c1
		k = k<<15 | k>>17
		k *= c2
		h ^= k
		h = h<<13 | h>>19
		h = h*5 + 0xe6546b64
	}
	var k uint32
	tail := data[blocks*4:]
	if len(tail) >= 3 {
		k ^= uint32(tail[2]) << 16
	}
	if len(tail) >= 2 {
		k ^= uint32(tail[1]) << 8
	}
	if len(tail) >= 1 {
		k ^= uint32(tail[0])
		k *= c1
		k = k<<15 | k>>17
		k *= c2
		h ^= k
	}
	h ^= uint32(len(data))
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

type ringSlot struct {
	hash uint32
	node string
}
type hashRing struct {
	slots []ringSlot
	nodes []string
}

func newRing(nodes []string) *hashRing {
	r := &hashRing{nodes: append([]string{}, nodes...)}
	for _, n := range nodes {
		for i := 0; i < 200; i++ {
			r.slots = append(r.slots, ringSlot{murmur3([]byte(fmt.Sprintf("%s#%d", n, i)), 0), n})
		}
	}
	sort.Slice(r.slots, func(i, j int) bool {
		if r.slots[i].hash != r.slots[j].hash {
			return r.slots[i].hash < r.slots[j].hash
		}
		return r.slots[i].node < r.slots[j].node
	})
	return r
}
func (r *hashRing) get(key string) string {
	target := murmur3([]byte(key), 0)
	i := sort.Search(len(r.slots), func(i int) bool {
		return r.slots[i].hash >= target
	})
	return r.slots[i%len(r.slots)].node
}
