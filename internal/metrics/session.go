package metrics

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxSessionLineBytes = 64 << 20

type sessionHeader struct {
	Type      string `json:"type"`
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
}

type entry struct {
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	ParentID      *string         `json:"parentId"`
	Timestamp     string          `json:"timestamp"`
	Message       json.RawMessage `json:"message"`
	CustomType    string          `json:"customType"`
	Data          json.RawMessage `json:"data"`
	Provider      string          `json:"provider"`
	ModelID       string          `json:"modelId"`
	ThinkingLevel string          `json:"thinkingLevel"`
	TokensBefore  int64           `json:"tokensBefore"`
}

type sessionBranch struct {
	Header  sessionHeader
	Entries []entry
}

func readSession(path, leafID string) (sessionBranch, error) {
	file, err := os.Open(path)
	if err != nil {
		return sessionBranch{}, err
	}
	defer file.Close()
	return decodeSession(file, leafID)
}

func decodeSession(reader io.Reader, leafID string) (sessionBranch, error) {
	header, entries, err := scanSession(reader)
	if err != nil {
		return sessionBranch{}, err
	}
	if len(entries) == 0 {
		return sessionBranch{Header: header}, nil
	}
	byID, err := indexEntries(entries)
	if err != nil {
		return sessionBranch{}, err
	}
	if leafID == "" {
		leafID = entries[len(entries)-1].ID
	}
	branch, err := selectBranch(byID, leafID, len(entries))
	if err != nil {
		return sessionBranch{}, err
	}
	return sessionBranch{Header: header, Entries: branch}, nil
}

func scanSession(reader io.Reader) (sessionHeader, []entry, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxSessionLineBytes)
	line := 0
	var header sessionHeader
	entries := make([]entry, 0, 128)
	for scanner.Scan() {
		line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		if line == 1 {
			if err := decodeSessionHeader(scanner.Bytes(), &header); err != nil {
				return sessionHeader{}, nil, err
			}
			continue
		}
		value, err := decodeSessionEntry(scanner.Bytes(), line)
		if err != nil {
			return sessionHeader{}, nil, err
		}
		entries = append(entries, value)
	}
	if err := scanner.Err(); err != nil {
		return sessionHeader{}, nil, fmt.Errorf("read session: %w", err)
	}
	if header.Type == "" {
		return sessionHeader{}, nil, errors.New("empty session")
	}
	return header, entries, nil
}

func decodeSessionHeader(data []byte, header *sessionHeader) error {
	if err := json.Unmarshal(data, header); err != nil {
		return fmt.Errorf("session header: %w", err)
	}
	if header.Type != "session" {
		return fmt.Errorf("first record has type %q, want session", header.Type)
	}
	if header.Version < 2 || header.Version > 3 {
		return fmt.Errorf("unsupported Pi session version %d", header.Version)
	}
	return nil
}

func decodeSessionEntry(data []byte, line int) (entry, error) {
	var value entry
	if err := json.Unmarshal(data, &value); err != nil {
		return entry{}, fmt.Errorf("session line %d: %w", line, err)
	}
	if value.ID == "" {
		return entry{}, fmt.Errorf("session line %d: missing entry id", line)
	}
	return value, nil
}

func indexEntries(entries []entry) (map[string]entry, error) {
	byID := make(map[string]entry, len(entries))
	for _, value := range entries {
		if _, exists := byID[value.ID]; exists {
			return nil, fmt.Errorf("duplicate entry id %q", value.ID)
		}
		byID[value.ID] = value
	}
	return byID, nil
}

func selectBranch(byID map[string]entry, leafID string, capacity int) ([]entry, error) {
	if _, ok := byID[leafID]; !ok {
		return nil, fmt.Errorf("leaf entry %q not found", leafID)
	}
	branch := make([]entry, 0, capacity)
	seen := make(map[string]bool)
	for leafID != "" {
		if seen[leafID] {
			return nil, fmt.Errorf("cycle at entry %q", leafID)
		}
		seen[leafID] = true
		value, ok := byID[leafID]
		if !ok {
			return nil, fmt.Errorf("entry references missing parent %q", leafID)
		}
		branch = append(branch, value)
		if value.ParentID == nil {
			break
		}
		leafID = *value.ParentID
	}
	reverseEntries(branch)
	return branch, nil
}

func reverseEntries(entries []entry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

type sessionDiscovery struct {
	paths []string
	seen  map[string]bool
}

// DiscoverSessions returns deterministic JSONL inputs. Directories are walked
// without following directory symlinks. With no inputs it uses Pi's default
// session root under the supplied home directory.
func DiscoverSessions(inputs []string, home string) ([]string, error) {
	if len(inputs) == 0 {
		inputs = []string{filepath.Join(home, ".pi", "agent", "sessions")}
	}
	discovery := sessionDiscovery{seen: make(map[string]bool)}
	for _, input := range inputs {
		if err := discovery.addInput(input); err != nil {
			return nil, err
		}
	}
	sort.Strings(discovery.paths)
	return discovery.paths, nil
}

func (discovery *sessionDiscovery) addInput(input string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(input, discovery.visit)
	}
	if filepath.Ext(input) != ".jsonl" {
		return fmt.Errorf("session input %q is not a .jsonl file", input)
	}
	return discovery.addPath(input)
}

func (discovery *sessionDiscovery) visit(path string, item fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if item.Type()&os.ModeSymlink != 0 && item.IsDir() {
		return filepath.SkipDir
	}
	if item.IsDir() || filepath.Ext(path) != ".jsonl" {
		return nil
	}
	return discovery.addPath(path)
}

func (discovery *sessionDiscovery) addPath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !discovery.seen[absolute] {
		discovery.seen[absolute] = true
		discovery.paths = append(discovery.paths, absolute)
	}
	return nil
}

func entryTime(value entry) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value.Timestamp)
	return parsed
}
