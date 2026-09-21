// Package outputspill captures large command output in immutable artifacts.
package outputspill

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/storagepaths"
)

const defaultSpillThresholdBytes = 64 * 1024

var outputSpillMutex sync.Mutex

// Options contains process-level output spill controls.
type Options struct {
	Disabled  bool
	Threshold int
	Directory string
}

// Descriptor points to immutable spilled output.
type Descriptor struct {
	Schema         string          `json:"schema"`
	Path           string          `json:"path"`
	Bytes          int64           `json:"bytes"`
	Digest         string          `json:"digest"`
	Format         string          `json:"format"`
	OriginalSchema string          `json:"originalSchema,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	Sources        json.RawMessage `json:"sources,omitempty"`
	Rerun          string          `json:"rerun"`
}

// Parse removes process-level spill flags from command arguments.
func Parse(args []string) ([]string, Options, error) {
	options := Options{Threshold: -1}
	filtered := make([]string, 0, len(args))
	literal := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if literal {
			filtered = append(filtered, argument)
			continue
		}
		if argument == "--" {
			literal = true
			filtered = append(filtered, argument)
			continue
		}
		if argument == "--no-spill" {
			options.Disabled = true
			continue
		}
		directory, directoryConsumed, directoryMatched, directoryErr := parseArtifactDirectory(args, index)
		if directoryErr != nil {
			return nil, options, directoryErr
		}
		if directoryMatched {
			options.Directory = directory
			index += directoryConsumed
			continue
		}
		threshold, consumed, matched, err := parseSpillThreshold(args, index)
		if err != nil {
			return nil, options, err
		}
		if matched {
			options.Threshold = threshold
			index += consumed
			continue
		}
		filtered = append(filtered, argument)
	}
	return filtered, options, nil
}

func parseArtifactDirectory(args []string, index int) (directory string, consumed int, matched bool, err error) {
	argument := args[index]
	switch {
	case argument == "--artifact-dir":
		if index+1 >= len(args) {
			return "", 0, true, fmt.Errorf("--artifact-dir requires a path")
		}
		directory, consumed = args[index+1], 1
	case strings.HasPrefix(argument, "--artifact-dir="):
		directory = strings.TrimPrefix(argument, "--artifact-dir=")
	default:
		return "", 0, false, nil
	}
	if strings.TrimSpace(directory) == "" {
		return "", 0, true, fmt.Errorf("--artifact-dir requires a path")
	}
	return filepath.Clean(directory), consumed, true, nil
}
func parseSpillThreshold(args []string, index int) (threshold, consumed int, matched bool, err error) {
	argument := args[index]
	value := ""
	switch {
	case argument == "--spill-threshold-bytes":
		if index+1 >= len(args) {
			return 0, 0, true, fmt.Errorf("--spill-threshold-bytes requires a positive byte count")
		}
		value, consumed = args[index+1], 1
	case strings.HasPrefix(argument, "--spill-threshold-bytes="):
		value = strings.TrimPrefix(argument, "--spill-threshold-bytes=")
	default:
		return 0, 0, false, nil
	}
	threshold, err = strconv.Atoi(value)
	if err != nil || threshold < 1 {
		return 0, 0, true, fmt.Errorf("--spill-threshold-bytes must be positive")
	}
	return threshold, consumed, true, nil
}

// Run captures command output and spills oversized results to an immutable artifact.
func Run(args []string, options Options, defaultThreshold int, workingDirectory string, run func(int) error) error {
	if options.Disabled && len(args) >= 2 && args[0] == "artifacts" && args[1] == "clean" {
		return run(int(^uint(0) >> 1))
	}
	if options.Disabled {
		options.Threshold = int(^uint(0) >> 1)
	}
	outputSpillMutex.Lock()
	defer outputSpillMutex.Unlock()
	threshold := options.Threshold
	if threshold < 1 {
		threshold = defaultThreshold
	}
	if threshold < 1 {
		threshold = defaultSpillThresholdBytes
	}
	outputDirectory, err := resolveOutputArtifactDirectory(options.Directory, workingDirectory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDirectory, 0o700); err != nil {
		if options.Directory != "" || strings.TrimSpace(os.Getenv("GREPPLE_ARTIFACT_DIR")) != "" {
			return err
		}
		outputDirectory = filepath.Join(os.TempDir(), "grepple", "output")
		if fallbackErr := os.MkdirAll(outputDirectory, 0o700); fallbackErr != nil {
			return fmt.Errorf("create artifact directory: %v; temporary fallback: %w", err, fallbackErr)
		}
	}
	temporary, err := os.CreateTemp(outputDirectory, ".grepple-spill-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	_ = temporary.Chmod(0o600)
	originalStdout := os.Stdout
	defer func() { os.Stdout = originalStdout }()
	os.Stdout = temporary
	commandErr := run(threshold)
	os.Stdout = originalStdout
	return finishOutputSpill(temporary, temporaryPath, outputDirectory, workingDirectory, threshold, args, originalStdout, commandErr)
}

func resolveOutputArtifactDirectory(configured, workingDirectory string) (string, error) {
	if configured == "" {
		return storagepaths.OutputArtifacts()
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured), nil
	}
	return filepath.Join(workingDirectory, configured), nil
}

func finishOutputSpill(temporary *os.File, temporaryPath, outputDirectory, workingDirectory string, threshold int, args []string, stdout io.Writer, commandErr error) error {
	if closeErr := temporary.Close(); commandErr == nil && closeErr != nil {
		commandErr = closeErr
	}
	info, statErr := os.Stat(temporaryPath)
	if statErr != nil {
		_ = os.Remove(temporaryPath)
		if commandErr != nil {
			return commandErr
		}
		return statErr
	}
	if info.Size() <= int64(threshold) {
		copyErr := copySpillToStdout(temporaryPath, stdout)
		_ = os.Remove(temporaryPath)
		if commandErr != nil {
			return commandErr
		}
		return copyErr
	}
	descriptor, err := persistSpilledOutput(temporaryPath, outputDirectory, workingDirectory, info.Size(), args)
	if err == nil {
		err = writeSpillDescriptor(stdout, descriptor, outputIsJSON(args))
	}
	if commandErr != nil {
		return commandErr
	}
	return err
}

func copySpillToStdout(path string, stdout io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(stdout, file)
	return err
}

func persistSpilledOutput(temporaryPath, outputDirectory, workingDirectory string, size int64, args []string) (Descriptor, error) {
	file, err := os.Open(temporaryPath)
	if err != nil {
		return Descriptor{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return Descriptor{}, err
	}
	if err := file.Close(); err != nil {
		return Descriptor{}, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	format := "text"
	extension := ".txt"
	if outputIsJSON(args) {
		format, extension = "json", ".json"
	}
	finalPath := filepath.Join(outputDirectory, digest+extension)
	if _, err := os.Stat(finalPath); err == nil {
		_ = os.Remove(temporaryPath)
	} else if err := os.Rename(temporaryPath, finalPath); err != nil {
		return Descriptor{}, err
	}
	displayPath, err := filepath.Rel(workingDirectory, finalPath)
	if err != nil {
		displayPath = finalPath
	}
	descriptor := Descriptor{Schema: "grepple-artifact-v1", Path: filepath.ToSlash(displayPath), Bytes: size, Digest: "sha256:" + digest, Format: format, Rerun: spillRerunCommand(args)}
	if format == "json" {
		file, openErr := os.Open(finalPath)
		if openErr == nil {
			var envelope struct {
				Schema   string          `json:"schema"`
				Metadata json.RawMessage `json:"metadata"`
				Sources  json.RawMessage `json:"sources"`
			}
			if json.NewDecoder(file).Decode(&envelope) == nil {
				descriptor.OriginalSchema = envelope.Schema
				descriptor.Metadata = envelope.Metadata
				descriptor.Sources = envelope.Sources
			}
			_ = file.Close()
		}
	}
	return descriptor, nil
}

func writeSpillDescriptor(writer io.Writer, descriptor Descriptor, jsonMode bool) error {
	if jsonMode {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(descriptor)
	}
	_, err := fmt.Fprintf(writer, "grepple output spilled path=%s bytes=%d digest=%s format=%s\nread: %s\nrerun: %s\n", descriptor.Path, descriptor.Bytes, descriptor.Digest, descriptor.Format, descriptor.Path, descriptor.Rerun)
	return err
}

func outputIsJSON(args []string) bool {
	for _, argument := range args {
		if argument == "--" {
			return false
		}
		if argument == "--json" || argument == "--json-matches" {
			return true
		}
	}
	return false
}

func spillRerunCommand(args []string) string {
	quoted := make([]string, 0, len(args)+2)
	quoted = append(quoted, "grepple")
	for _, argument := range args {
		quoted = append(quoted, shellquote.Argument(argument))
	}
	quoted = append(quoted, "--no-spill")
	return strings.Join(quoted, " ")
}
