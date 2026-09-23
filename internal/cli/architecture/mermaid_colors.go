package architecture

import (
	"fmt"
	"math"
)

const architectureMermaidTargetLuminance = 0.18

// architectureMermaidColor returns a deterministic, high-saturation color at
// a luminance that remains legible against both light and dark backgrounds.
func architectureMermaidColor(index int) string {
	hue := math.Mod(210+float64(index)*137.50776405003785, 360)
	low, high := 0.0, 1.0
	for range 28 {
		lightness := (low + high) / 2
		red, green, blue := architectureHSL(hue, 0.82, lightness)
		if architectureRelativeLuminance(red, green, blue) < architectureMermaidTargetLuminance {
			low = lightness
		} else {
			high = lightness
		}
	}
	red, green, blue := architectureHSL(hue, 0.82, (low+high)/2)
	return fmt.Sprintf("#%02x%02x%02x", architectureColorByte(red), architectureColorByte(green), architectureColorByte(blue))
}

func architectureHSL(hue, saturation, lightness float64) (float64, float64, float64) {
	chroma := (1 - math.Abs(2*lightness-1)) * saturation
	x := chroma * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	match := lightness - chroma/2
	var red, green, blue float64
	switch {
	case hue < 60:
		red, green = chroma, x
	case hue < 120:
		red, green = x, chroma
	case hue < 180:
		green, blue = chroma, x
	case hue < 240:
		green, blue = x, chroma
	case hue < 300:
		red, blue = x, chroma
	default:
		red, blue = chroma, x
	}
	return red + match, green + match, blue + match
}

func architectureRelativeLuminance(red, green, blue float64) float64 {
	linear := func(component float64) float64 {
		if component <= 0.04045 {
			return component / 12.92
		}
		return math.Pow((component+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(red) + 0.7152*linear(green) + 0.0722*linear(blue)
}

func architectureColorByte(component float64) int {
	return int(math.Round(255 * min(1, max(0, component))))
}
