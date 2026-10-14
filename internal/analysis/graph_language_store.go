package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

const maxLanguageManifestBytes = 32 << 20

// Generation is a sibling directory containing atomically published immutable
// language cores. Only a complete checksummed manifest makes it visible.
type languageManifest struct {
	Key, Generation string
	State           storedGraphMetadata
	Version         int
	Roots           []languageRootRow    `json:"-"`
	RootSources     []languageRootSource `json:"-"`
	RootCatalog     []byte
	Parts           []languageManifestPart
}
type languageManifestPart struct {
	Languages []string
	graph     parser.NavigationGraph
}
type languageManifestEnvelope struct {
	Digest string
	Data   json.RawMessage
}

func languagePartitionKey(key string, languages []string) string {
	sum := sha256.Sum256([]byte(key + ":languages:" + strings.Join(languages, ",")))
	return hex.EncodeToString(sum[:])
}
func languagePartitionPath(path, generation string, index int) string {
	return filepath.Join(filepath.Dir(path), generation, fmt.Sprintf("%d.gnav", index))
}
func readLanguageManifest(path, key string) (languageManifest, error) {
	var manifest languageManifest
	file, err := os.Open(path + ".languages.json")
	if err != nil {
		return manifest, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return manifest, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxLanguageManifestBytes {
		return manifest, fmt.Errorf("invalid language manifest size")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxLanguageManifestBytes+1))
	if err != nil {
		return manifest, err
	}
	if len(data) > maxLanguageManifestBytes {
		return manifest, fmt.Errorf("language manifest too large")
	}
	var envelope languageManifestEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return manifest, err
	}
	digest := sha256.Sum256(envelope.Data)
	if hex.EncodeToString(digest[:]) != envelope.Digest {
		return manifest, fmt.Errorf("language manifest checksum mismatch")
	}
	if err := json.Unmarshal(envelope.Data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != 3 {
		return languageManifest{}, fmt.Errorf("unsupported root catalog schema")
	}
	if err := decodeLanguageRootCatalog(&manifest); err != nil {
		return languageManifest{}, err
	}
	if err := validateLanguageManifest(manifest, path, key); err != nil {
		return languageManifest{}, err
	}
	return manifest, nil
}
func validateLanguageManifest(manifest languageManifest, path, key string) error {
	if manifest.Version != 3 || manifest.Key != key || !strings.HasPrefix(manifest.Generation, filepath.Base(path)+".languages-") || filepath.Base(manifest.Generation) != manifest.Generation || len(manifest.Parts) == 0 || len(manifest.Parts) > 64 {
		return fmt.Errorf("invalid language generation")
	}
	languages := map[string]bool{}
	for _, part := range manifest.Parts {
		if len(part.Languages) == 0 {
			return fmt.Errorf("empty language partition")
		}
		for _, language := range part.Languages {
			if languages[language] {
				return fmt.Errorf("duplicate language partition")
			}
			languages[language] = true
		}
	}
	for _, root := range manifest.Roots {
		if uint64(root.Source) >= uint64(len(manifest.RootSources)) || !languages[manifest.RootSources[root.Source].Language] {
			return fmt.Errorf("unowned language root")
		}
	}
	return nil
}
func publishLanguageManifest(path, key string, manifest *languageManifest, operations navigation.GraphOperations) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	directory, err := os.MkdirTemp(filepath.Dir(path), filepath.Base(path)+".languages-")
	if err != nil {
		return err
	}
	manifest.Key, manifest.Generation = key, filepath.Base(directory)
	for index, part := range manifest.Parts {
		snapshot, err := operations.PrepareQuery(part.graph)
		if err != nil {
			return err
		}
		save := operations.SnapshotStore().SavePaged
		if os.Getenv("GREPPLE_NAVIGATION_EAGER_PARTITIONS") == "1" {
			save = operations.SnapshotStore().Save
		}
		if err := save(languagePartitionPath(path, manifest.Generation, index), languagePartitionKey(key, part.Languages), snapshot, nil); err != nil {
			return err
		}
	}
	return publishLanguageManifestPointer(path, manifest)
}
func publishLanguageManifestPointer(path string, manifest *languageManifest) error {
	catalog, err := encodeLanguageRootCatalog(*manifest)
	if err != nil {
		return err
	}
	encodedManifest := *manifest
	encodedManifest.RootCatalog = catalog
	data, err := json.Marshal(encodedManifest)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	envelope, err := json.Marshal(languageManifestEnvelope{Digest: hex.EncodeToString(digest[:]), Data: data})
	if err != nil {
		return err
	}
	if len(envelope) > maxLanguageManifestBytes {
		return fmt.Errorf("language manifest too large")
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".language-manifest-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	if _, err := temporary.Write(envelope); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), path+".languages.json"); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}
