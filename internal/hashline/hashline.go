// Package hashline implements Grepple's stable, file-local line anchors.
package hashline

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode"
)

const (
	// Length is the number of characters in a line anchor.
	Length           = 3
	alphabet         = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	prime32v1 uint32 = 2654435761
	prime32v2 uint32 = 2246822519
	prime32v3 uint32 = 3266489917
	prime32v4 uint32 = 668265263
	prime32v5 uint32 = 374761393
)

// Lines returns one deterministic, unique anchor for every newline-delimited
// line in content, including the final empty line when content ends in a newline.
func Lines(content string) []string {
	lines := strings.Split(content, "\n")
	hashes := make([]string, len(lines))
	assigned := make(map[string]bool, len(lines))
	for index, line := range lines {
		canonical := CanonicalLine(line)
		hash := hashString(canonical)
		for retry := 1; assigned[hash]; retry++ {
			hash = hashString(fmt.Sprintf("%s:R%d", canonical, retry))
		}
		assigned[hash] = true
		hashes[index] = hash
	}
	return hashes
}

// CanonicalLine removes carriage returns and trailing whitespace exactly as the
// hashline-v1 compatibility contract requires.
func CanonicalLine(line string) string {
	line = strings.ReplaceAll(line, "\r", "")
	return strings.TrimRightFunc(line, func(character rune) bool {
		return unicode.IsSpace(character) || character == '\uFEFF'
	})
}

// Valid reports whether value has the canonical anchor alphabet and length.
func Valid(value string) bool {
	if len(value) != Length {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune(alphabet, character) {
			return false
		}
	}
	return true
}

func hashString(value string) string {
	hash := xxhash32([]byte(value), 0)
	value18 := hash >> (32 - Length*6)
	result := make([]byte, Length)
	for index := 0; index < Length; index++ {
		shift := uint((Length - 1 - index) * 6)
		result[index] = alphabet[(value18>>shift)&63]
	}
	return string(result)
}

func xxhash32(input []byte, seed uint32) uint32 {
	index := 0
	var hash uint32
	if len(input) >= 16 {
		v1 := seed + prime32v1 + prime32v2
		v2 := seed + prime32v2
		v3 := seed
		v4 := seed - prime32v1
		limit := len(input) - 16
		for index <= limit {
			v1 = round32(v1, binary.LittleEndian.Uint32(input[index:]))
			index += 4
			v2 = round32(v2, binary.LittleEndian.Uint32(input[index:]))
			index += 4
			v3 = round32(v3, binary.LittleEndian.Uint32(input[index:]))
			index += 4
			v4 = round32(v4, binary.LittleEndian.Uint32(input[index:]))
			index += 4
		}
		hash = rotateLeft32(v1, 1) + rotateLeft32(v2, 7) + rotateLeft32(v3, 12) + rotateLeft32(v4, 18)
	} else {
		hash = seed + prime32v5
	}
	hash += uint32(len(input))
	for index+4 <= len(input) {
		hash += binary.LittleEndian.Uint32(input[index:]) * prime32v3
		hash = rotateLeft32(hash, 17) * prime32v4
		index += 4
	}
	for index < len(input) {
		hash += uint32(input[index]) * prime32v5
		hash = rotateLeft32(hash, 11) * prime32v1
		index++
	}
	hash ^= hash >> 15
	hash *= prime32v2
	hash ^= hash >> 13
	hash *= prime32v3
	hash ^= hash >> 16
	return hash
}

func round32(accumulator, input uint32) uint32 {
	accumulator += input * prime32v2
	accumulator = rotateLeft32(accumulator, 13)
	return accumulator * prime32v1
}

func rotateLeft32(value uint32, count uint) uint32 {
	return value<<count | value>>(32-count)
}
