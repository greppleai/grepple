package archdaemon

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/analysis"
)

const maxContextFileBytes = 8 << 20

// architectureSnapshot reads exactly the CLI-selected sources. Missing or
// unreadable sources retain current-invocation accounting but are not cached.
func architectureSnapshot(root string, paths []string, maxFiles int) ([]analysis.Source, string, bool, error) {
	if maxFiles < 0 {
		return nil, "", false, fmt.Errorf("invalid max-files")
	}
	absolute, err := validatedArchitecturePaths(root, paths)
	if err != nil {
		return nil, "", false, err
	}
	sources := analysis.ReadSources(paths)
	digest := sha256.New()
	writeKeyPart(digest, protocol)
	writeKeyPart(digest, root)
	writeKeyPart(digest, fmt.Sprint(maxFiles))
	cacheable := true
	for _, source := range sources {
		writeKeyPart(digest, source.Path)
		if source.ReadError != nil {
			cacheable = false
			continue
		}
		writeKeyPart(digest, string(source.Content))
	}
	if !fingerprintArchitectureContext(digest, ancestorArchitectureContextPaths(absolute)) {
		cacheable = false
	}
	return sources, hex.EncodeToString(digest.Sum(nil)), cacheable, nil
}

func validatedArchitecturePaths(root string, paths []string) ([]string, error) {
	absolute := make([]string, 0, len(paths))
	for _, path := range paths {
		resolved, err := filepath.Abs(path)
		if err != nil || !withinRoot(root, resolved) || !withinRootResolved(root, resolved) {
			return nil, fmt.Errorf("source outside worker root")
		}
		absolute = append(absolute, resolved)
	}
	return absolute, nil
}

func ancestorArchitectureContextPaths(absolute []string) []string {
	candidates := make(map[string]bool)
	for _, path := range absolute {
		for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
			for _, name := range []string{"go.mod", "go.work", "tsconfig.json", "grepple.yaml"} {
				candidates[filepath.Join(directory, name)] = true
			}
			if filepath.Dir(directory) == directory {
				break
			}
		}
	}
	ordered := make([]string, 0, len(candidates))
	for path := range candidates {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	return ordered
}

func fingerprintArchitectureContext(digest hash.Hash, paths []string) bool {
	for _, path := range paths {
		writeKeyPart(digest, path)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			writeKeyPart(digest, "absent")
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxContextFileBytes {
			return false
		}
		file, err := os.Open(path)
		if err != nil {
			return false
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxContextFileBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(content) > maxContextFileBytes {
			return false
		}
		writeKeyPart(digest, string(content))
	}
	return true
}

func withinRoot(root, absolute string) bool {
	relative, err := filepath.Rel(root, absolute)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func withinRootResolved(root, absolute string) bool {
	resolved, err := filepath.EvalSymlinks(absolute)
	return os.IsNotExist(err) || (err == nil && withinRoot(root, resolved))
}

func writeKeyPart(digest hash.Hash, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = digest.Write(size[:])
	_, _ = io.WriteString(digest, value)
}
