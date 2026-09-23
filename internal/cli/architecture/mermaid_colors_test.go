package architecture

import (
	"fmt"
	"math"
	"testing"
)

func TestArchitectureMermaidColorsAreDeterministicDistinctAndThemeVisible(t *testing.T) {
	seen := map[string]bool{}
	var previous [3]int
	for index := range 64 {
		color := architectureMermaidColor(index)
		if color != architectureMermaidColor(index) {
			t.Fatalf("color %d is not deterministic", index)
		}
		if seen[color] {
			t.Fatalf("color %q repeated in first 64 directory colors", color)
		}
		seen[color] = true
		red, green, blue := parseArchitectureColor(t, color)
		luminance := architectureRelativeLuminance(float64(red)/255, float64(green)/255, float64(blue)/255)
		lightContrast := 1.05 / (luminance + 0.05)
		darkContrast := (luminance + 0.05) / 0.05
		if lightContrast < 4.5 || darkContrast < 4.5 {
			t.Fatalf("color %q contrast light=%.2f dark=%.2f", color, lightContrast, darkContrast)
		}
		current := [3]int{red, green, blue}
		if index > 0 {
			distance := math.Sqrt(float64(square(current[0]-previous[0]) + square(current[1]-previous[1]) + square(current[2]-previous[2])))
			if distance < 120 {
				t.Fatalf("adjacent colors %d and %d are too similar: distance %.1f", index-1, index, distance)
			}
		}
		previous = current
	}
}

func TestArchitectureMermaidUsesSourceDirectoryColorForEveryLink(t *testing.T) {
	relations := []compressedArchitectureRelation{
		{From: "alpha", To: "beta", Kinds: []string{"import"}},
		{From: "alpha", To: "gamma", Kinds: []string{"call"}},
		{From: "beta", To: "gamma", Kinds: []string{"type-reference"}},
	}
	got := architectureRelationsMermaid(relations, false, 0)
	for _, expected := range []string{
		"style d0 stroke:#1576d7,stroke-width:2px",
		"linkStyle 0 stroke:#1576d7,stroke-width:2px",
		"linkStyle 1 stroke:#1576d7,stroke-width:2px",
		"linkStyle 2 stroke:#e71742,stroke-width:2px",
	} {
		if !containsLine(got, expected) {
			t.Fatalf("Mermaid output missing %q:\n%s", expected, got)
		}
	}
}

func parseArchitectureColor(t *testing.T, color string) (int, int, int) {
	t.Helper()
	var red, green, blue int
	if _, err := fmt.Sscanf(color, "#%02x%02x%02x", &red, &green, &blue); err != nil {
		t.Fatalf("parse color %q: %v", color, err)
	}
	return red, green, blue
}

func square(value int) int { return value * value }

func containsLine(text, expected string) bool {
	for _, line := range splitLines(text) {
		if line == "    "+expected {
			return true
		}
	}
	return false
}

func splitLines(text string) []string {
	var lines []string
	start := 0
	for index, char := range text {
		if char == '\n' {
			lines = append(lines, text[start:index])
			start = index + 1
		}
	}
	return lines
}
