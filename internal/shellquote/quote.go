// Package shellquote formats command arguments for continuation commands.
package shellquote

import "strings"

// Argument quotes one shell argument when needed.
func Argument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
