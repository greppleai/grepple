package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/greppleai/grepple/internal/search"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NavigationSelection witnesses exact selected content and matching lines.
type NavigationSelection = search.NavigationSelection

// CaptureNavigationSelections performs no additional source reads.
func CaptureNavigationSelections(batch SearchBatch) []NavigationSelection {
	value, ok := batch.(searchBatch)
	if !ok {
		return nil
	}
	var selected []NavigationSelection
	total := 0
	for _, match := range value.matches {
		if len(match.Content) > 8<<20 || len(match.MatchLines) > 4096 {
			continue
		}
		total += len(match.Content)
		if total > 16<<20 {
			return nil
		}
		digest := sha256.Sum256([]byte(match.Content))
		var lines []int
		for line := range match.MatchLines {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		selected = append(selected, NavigationSelection{Path: match.DisplayPath, Language: match.Language, Digest: hex.EncodeToString(digest[:]), Lines: lines})
	}
	return selected
}

// RestoreNavigationSelections reads checked immutable source once, validates its
// digest, and restores original hits without executing the query or parsing ASTs.
// The host supplies only paths already checked against a fresh authorized lease.
func RestoreNavigationSelections(ctx context.Context, selected []NavigationSelection, paths []string) (SearchBatch, error) {
	if len(selected) == 0 || len(selected) > 100 || len(paths) != len(selected) {
		return nil, fmt.Errorf("invalid selected navigation rows")
	}
	matches := make([]search.FileMatch, 0, len(selected))
	var total int64
	for i, row := range selected {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		match, size, err := restoreNavigationSelection(row, paths[i], 32<<20-total)
		if err != nil {
			return nil, err
		}
		total += size
		matches = append(matches, match)
	}
	return searchBatch{matches: matches}, nil
}
func restoreNavigationSelection(row NavigationSelection, path string, remaining int64) (search.FileMatch, int64, error) {
	var match search.FileMatch
	if len(row.Digest) != 64 || len(row.Lines) == 0 || len(row.Lines) > 1<<20 || remaining <= 0 {
		return match, 0, fmt.Errorf("invalid selected navigation witness")
	}
	file, err := os.Open(path)
	if err != nil {
		return match, 0, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 8<<20 || stat.Size() > remaining {
		return match, 0, fmt.Errorf("selected navigation source budget exceeded")
	}
	bytes, err := io.ReadAll(io.LimitReader(file, min(int64(8<<20), remaining)+1))
	if err != nil {
		return match, 0, err
	}
	if int64(len(bytes)) > remaining || len(bytes) > 8<<20 {
		return match, 0, fmt.Errorf("selected navigation source budget exceeded")
	}
	content := strings.ToValidUTF8(string(bytes), "\uFFFD")
	digest := sha256.Sum256([]byte(content))
	if hex.EncodeToString(digest[:]) != row.Digest {
		return match, 0, fmt.Errorf("selected navigation source changed")
	}
	lines := search.SplitLines(content)
	hits := map[int]bool{}
	for _, line := range row.Lines {
		if line < 1 || line > len(lines) || hits[line] {
			return match, 0, fmt.Errorf("invalid selected navigation hit")
		}
		hits[line] = true
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return match, 0, err
	}
	return search.FileMatch{File: absolute, DisplayPath: row.Path, Language: row.Language, Content: content, MatchLines: hits, SegmentsReady: true}, int64(len(bytes)), nil
}
