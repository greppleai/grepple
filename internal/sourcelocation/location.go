// Package sourcelocation parses command-neutral PATH:LINE source locations.
package sourcelocation

import (
	"fmt"
	"strconv"
	"strings"
)

// Location is one optional line or explicit line-range source location.
type Location struct {
	Path        string
	Start       int
	End         int
	ExplicitEnd bool
}

// Parse parses PATH, PATH:LINE, or PATH:START-END.
func Parse(value string) (Location, error) {
	index := strings.LastIndex(value, ":")
	if index < 1 {
		if strings.TrimSpace(value) == "" {
			return Location{}, fmt.Errorf("source path is required")
		}
		return Location{Path: value}, nil
	}
	location := Location{Path: value[:index]}
	lineText := value[index+1:]
	if dash := strings.IndexByte(lineText, '-'); dash >= 0 {
		location.ExplicitEnd = true
		end, err := strconv.Atoi(lineText[dash+1:])
		if err != nil || end < 1 {
			return Location{}, fmt.Errorf("--at end line must be positive")
		}
		location.End = end
		lineText = lineText[:dash]
	}
	start, err := strconv.Atoi(lineText)
	if err != nil || start < 1 {
		return Location{}, fmt.Errorf("--at line must be positive")
	}
	if location.ExplicitEnd && location.End < start {
		return Location{}, fmt.Errorf("--at end line must not precede start line")
	}
	location.Start = start
	if !location.ExplicitEnd {
		location.End = start
	}
	return location, nil
}

// ParseLine parses one PATH:LINE source selector and returns its starting line.
func ParseLine(value string) (string, int, error) {
	location, err := Parse(value)
	if err != nil {
		return "", 0, err
	}
	if location.Start == 0 {
		return "", 0, fmt.Errorf("--at must be PATH:LINE")
	}
	return location.Path, location.Start, nil
}
