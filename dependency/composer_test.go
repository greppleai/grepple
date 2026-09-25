package dependency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComposerResolverMatchesLockedNamespaces(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "composer.json")
	if err := os.WriteFile(manifest, []byte(`{"name":"example/app","version":"1.0.0","require":{"acme/widgets":"^2","acme/ui":"^1","acme/dev":"dev-main"},"require-dev":{"acme/testing":"^3"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := `{"packages":[{"name":"acme/widgets","version":"2.4.0","autoload":{"psr-4":{"Acme\\":"src/"}}},{"name":"acme/ui","version":"1.2.0","autoload":{"psr-4":{"Acme\\UI\\":["lib/","src/"]}}},{"name":"acme/dev","version":"dev-main","autoload":{"psr-4":{"Development\\":"src/"}}},{"name":"unlisted/pkg","version":"8.0.0","autoload":{"psr-4":{"Unlisted\\":"src/"}}},{"name":"local/path","version":"1.0.0","dist":{"type":"path"},"autoload":{"psr-4":{"Local\\":"src/"}}}],"packages-dev":[{"name":"acme/testing","version":"3.1.0","autoload":{"psr-0":{"Tests\\":"tests/"}}}]}`
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := NewComposerResolver()
	resolution, err := resolver.Resolve(manifest)
	if err != nil || !resolution.Applicable {
		t.Fatalf("resolve = %+v, %v", resolution, err)
	}
	for path, module := range map[string]string{"\\Acme\\Widget": "acme/widgets", "Acme\\UI\\Button": "acme/ui", "Tests\\Suite": "acme/testing"} {
		match := resolver.Match(path, resolution.Dependencies)
		if match.Exact == nil || match.Exact.Module != module || match.Exact.Version == "" {
			t.Errorf("match %q = %+v, want %s", path, match, module)
		}
	}
	for _, path := range []string{"AcmeOther\\Thing", "Unlisted\\Thing", "Local\\Thing", "Development\\Thing"} {
		if match := resolver.Match(path, resolution.Dependencies); match.Exact != nil || len(match.Candidates) != 0 {
			t.Errorf("unexpected match %q = %+v", path, match)
		}
	}
	module, err := resolver.DiscoverModule(manifest, ".")
	if err != nil || module == nil || module.Module != "example/app" || module.Version != "1.0.0" {
		t.Fatalf("module = %+v, %v", module, err)
	}
}

func TestComposerResolverLeavesUnversionedAndAmbiguousImportsUnresolved(t *testing.T) {
	resolver := NewComposerResolver()
	root := t.TempDir()
	manifest := filepath.Join(root, "composer.json")
	if err := os.WriteFile(manifest, []byte(`{"require":{"pkg/a":"^1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if resolution, err := resolver.Resolve(manifest); err != nil || !resolution.Applicable || len(resolution.Dependencies) != 0 {
		t.Fatalf("without lock = %+v, %v", resolution, err)
	}
	candidates := []Evidence{{Ecosystem: "composer", ImportName: "Shared\\", Module: "pkg/a", Version: "1.0.0"}, {Ecosystem: "composer", ImportName: "Shared\\", Module: "pkg/b", Version: "2.0.0"}}
	if match := resolver.Match("Shared\\Type", candidates); match.Exact != nil || len(match.Candidates) != 2 {
		t.Fatalf("ambiguous match = %+v", match)
	}
}

func TestComposerAutoloadRulesAcceptsStringAndArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "composer.json")
	if err := os.WriteFile(path, []byte(`{"autoload":{"psr-4":{"App\\":"src/","Example\\":["lib/","tests/"]}},"autoload-dev":{"psr-4":{"App\\":"dev/","Tests\\":"tests/"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rules, err := ComposerAutoloadRules(path)
	if err != nil || len(rules["App\\"]) != 2 || len(rules["Example\\"]) != 2 || len(rules["Tests\\"]) != 1 {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
}
