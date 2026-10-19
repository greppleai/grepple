package render

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/greppleai/grepple/internal/memory"
)

const memoryFooterLimit = 768

// safeMemoryDescription is defense in depth for human and JSON renderers.
// The schema already refuses ordinary controls; drop directional format
// controls too, since terminal output must not be spoofed by untrusted text.
func safeMemoryDescription(value string, limit int) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		size := utf8.RuneLen(r)
		if size < 0 || b.Len()+size > limit {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func memoryFooter(found memory.Discovery) string {
	if len(found.References) == 0 && found.Complete {
		return ""
	}
	for shown := len(found.References); shown >= 0; shown-- {
		var b strings.Builder
		if shown > 0 {
			b.WriteString("\nMemories located:\n")
		}
		for _, ref := range found.References[:shown] {
			fmt.Fprintf(&b, "%s — %s (grepple memory recall %s)\n", ref.Name, safeMemoryDescription(ref.Description, 120), ref.Name)
		}
		omitted := found.Omitted + len(found.References) - shown
		if omitted > 0 {
			fmt.Fprintf(&b, "... %d more memories; refine search or recall by name\n", omitted)
		}
		if !found.Complete {
			b.WriteString("memory references incomplete\n")
		}
		if b.Len() <= memoryFooterLimit {
			return b.String()
		}
	}
	return "memory references incomplete\n"
}
