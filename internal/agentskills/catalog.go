// Package agentskills owns version-pinned skill bundles and safe installation.
package agentskills

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

//go:embed owned-skills.txt
var ownershipData []byte

//go:embed catalog.json
var catalogData []byte

const catalogPath = "internal/agentskills/catalog.json"
const ownershipPath = "internal/agentskills/owned-skills.txt"
const maxFileBytes = 1024 * 1024
const maxBundleBytes = 16 * 1024 * 1024

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var releaseVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(\+[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`)
var describeVersion = regexp.MustCompile(`-[0-9]+-g[0-9a-f]+$`)

type catalog struct {
	Schema int           `json:"schema"`
	Skills []skill       `json:"skills"`
	Legacy []legacySkill `json:"legacy"`
}
type skill struct {
	Name  string      `json:"name"`
	Files []skillFile `json:"files"`
}
type skillFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type legacySkill struct {
	Name    string   `json:"name"`
	SHA256s []string `json:"sha256s"`
}

// Bundle is a completely validated, immutable set of source-owned skill files.
type Bundle struct {
	catalog catalog
	files   map[string]map[string][]byte
}

// ReleaseTag accepts exact released versions, never moving refs or dirty descriptions.
func ReleaseTag(version string) (string, error) {
	if strings.HasSuffix(version, "-dirty") || describeVersion.MatchString(version) || !releaseVersion.MatchString(version) {
		return "", fmt.Errorf("setup requires an exact release version; use --source-dir for an explicit development checkout (build: %s)", version)
	}
	return "v" + strings.TrimPrefix(version, "v"), nil
}

// OwnedNames includes active, renamed and retired names; the registry is append-only.
func OwnedNames() []string {
	var names []string
	for _, line := range strings.Split(string(ownershipData), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	return names
}

// ActiveNames returns current skill names in stable catalog order.
func ActiveNames() ([]string, error) {
	c, err := readCatalog()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(c.Skills))
	for _, s := range c.Skills {
		names = append(names, s.Name)
	}
	return names, nil
}

func readCatalog() (catalog, error) {
	var c catalog
	decoder := json.NewDecoder(bytes.NewReader(catalogData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, err
	}
	if c.Schema != 1 || len(c.Skills) == 0 {
		return c, fmt.Errorf("unsupported skill catalog")
	}
	owned, err := ownershipSet()
	if err != nil {
		return c, err
	}
	active := map[string]bool{}
	for _, s := range c.Skills {
		if !owned[s.Name] || !strings.HasPrefix(s.Name, "grepple-") || active[s.Name] {
			return c, fmt.Errorf("invalid active skill %q", s.Name)
		}
		active[s.Name] = true
		if err := validateSkill(s); err != nil {
			return c, err
		}
	}
	for _, old := range c.Legacy {
		if !owned[old.Name] {
			return c, fmt.Errorf("legacy name is not owned: %s", old.Name)
		}
	}
	return c, nil
}
func ownershipSet() (map[string]bool, error) {
	owned := map[string]bool{}
	for _, name := range OwnedNames() {
		if !skillName.MatchString(name) || len(name) > 64 || owned[name] {
			return nil, fmt.Errorf("invalid ownership registry")
		}
		owned[name] = true
	}
	return owned, nil
}
func validateSkill(s skill) error {
	paths := map[string]bool{}
	if len(s.Files) == 0 || len(s.Files) > 64 {
		return fmt.Errorf("invalid file count for %s", s.Name)
	}
	for _, file := range s.Files {
		digest, err := hex.DecodeString(file.SHA256)
		if !safeRelative(file.Path) || file.Path == markerName || paths[file.Path] || err != nil || len(digest) != 32 {
			return fmt.Errorf("invalid catalog file in %s", s.Name)
		}
		paths[file.Path] = true
	}
	if !paths["SKILL.md"] {
		return fmt.Errorf("skill %s lacks SKILL.md", s.Name)
	}
	return nil
}
func safeRelative(value string) bool {
	return value != "" && value != "." && path.Clean(value) == value && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\\:\x00")
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Load downloads all files and checks compatibility/integrity before installation.
func Load(ctx context.Context, source Source) (Bundle, error) {
	bundle := Bundle{files: map[string]map[string][]byte{}}
	if err := checkSourceCatalog(ctx, source); err != nil {
		return bundle, err
	}
	c, err := readCatalog()
	if err != nil {
		return bundle, err
	}
	bundle.catalog = c
	total := 0
	for _, s := range c.Skills {
		files, size, err := loadSkill(ctx, source, s)
		if err != nil {
			return bundle, err
		}
		total += size
		if total > maxBundleBytes {
			return bundle, fmt.Errorf("skill bundle exceeds size limit")
		}
		bundle.files[s.Name] = files
	}
	return bundle, nil
}
func checkSourceCatalog(ctx context.Context, source Source) error {
	for file, want := range map[string][]byte{catalogPath: catalogData, ownershipPath: ownershipData} {
		data, err := source.Read(ctx, file)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, want) {
			return fmt.Errorf("skill catalog does not match this binary; rebuild for this checkout or use its matching release")
		}
	}
	return nil
}
func loadSkill(ctx context.Context, source Source, s skill) (map[string][]byte, int, error) {
	files := map[string][]byte{}
	total := 0
	for _, file := range s.Files {
		data, err := source.Read(ctx, ".agents/skills/"+s.Name+"/"+file.Path)
		if err != nil {
			return nil, 0, err
		}
		total += len(data)
		if len(data) > maxFileBytes || digest(data) != file.SHA256 {
			return nil, 0, fmt.Errorf("skill checksum or size mismatch: %s/%s", s.Name, file.Path)
		}
		files[file.Path] = data
	}
	if !bytes.HasPrefix(files["SKILL.md"], []byte("---\nname: "+s.Name+"\n")) {
		return nil, 0, fmt.Errorf("skill frontmatter name mismatch: %s", s.Name)
	}
	return files, total, nil
}
