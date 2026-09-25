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

// architectureSnapshot independently reads the client's selected files under
// its root; source paths retain their original spelling for report parity.
func architectureSnapshot(root string, paths []string, maxFiles int) ([]analysis.Source, string, bool, error) {
	absolute, err := validatedArchitecturePaths(root, paths)
	if err != nil || maxFiles < 0 {
		return nil, "", false, fmt.Errorf("invalid repository source selection")
	}
	sources := make([]analysis.Source, 0, len(paths))
	for index, path := range paths {
		content, readErr := os.ReadFile(absolute[index])
		sources = append(sources, analysis.Source{Path: path, Content: content, ReadError: readErr})
	}
	key, cacheable := architectureFingerprint(root, paths, maxFiles, sources)
	return sources, key, cacheable, nil
}

// architectureFingerprint hashes exactly the caller-owned source bytes and
// external navigation/metadata context, before the caller builds a report.
func architectureFingerprint(root string, paths []string, maxFiles int, sources []analysis.Source) (string, bool) {
	if maxFiles < 0 || len(paths) != len(sources) {
		return "", false
	}
	absolute, err := validatedArchitecturePaths(root, paths)
	if err != nil {
		return "", false
	}
	digest := sha256.New()
	writeKeyPart(digest, protocol)
	writeKeyPart(digest, root)
	writeKeyPart(digest, fmt.Sprint(maxFiles))
	for index, source := range sources {
		if source.Path != paths[index] || source.ReadError != nil {
			return "", false
		}
		writeKeyPart(digest, source.Path)
		writeKeyPart(digest, string(source.Content))
	}
	if !fingerprintArchitectureContext(digest, ancestorArchitectureContextPaths(absolute)) {
		return "", false
	}
	return hex.EncodeToString(digest.Sum(nil)), true
}

func validatedArchitecturePaths(root string, paths []string) ([]string, error) {
	if !filepath.IsAbs(root) || root != filepath.Clean(root) {
		return nil, fmt.Errorf("invalid repository root")
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("unavailable repository root")
	}
	absolute := make([]string, 0, len(paths))
	for _, path := range paths {
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(root, resolved)
		}
		resolved = filepath.Clean(resolved)
		if !withinRoot(root, resolved) || !withinRootResolved(root, resolved) {
			return nil, fmt.Errorf("source outside repository root")
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
