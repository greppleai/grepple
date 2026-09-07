package router

import (
	"fmt"
	"testing"
)

func TestHashRingDistributionAndMinimalRemapping(t *testing.T) {
	nodes := []string{"http://a:8787", "http://b:8787", "http://c:8787"}
	ring := newRing(nodes)
	smaller := newRing(nodes[:2])
	counts := map[string]int{}
	before := map[string]string{}

	for index := 0; index < 6000; index++ {
		key := fmt.Sprintf("org/repo-%d", index)
		node := ring.get(key)
		counts[node]++
		before[key] = node
	}
	for _, node := range nodes {
		if counts[node] < 1400 || counts[node] > 2600 {
			t.Errorf("uneven %s=%d", node, counts[node])
		}
	}
	for key, node := range before {
		if node != nodes[2] && smaller.get(key) != node {
			t.Fatalf("survivor remapped: %s", key)
		}
	}
}
