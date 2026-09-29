package anchors

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/anchor"
	"github.com/greppleai/grepple/internal/config"
)

func TestNativeAnchorLookupUsesBuiltInHashline(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	writeJSONFile(t, settingsPath, config.UserSettings{})
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	lookup, err := anchor.Generate([]anchor.File{{Path: "/tmp/source", DisplayPath: "source.txt", Content: "alpha\nbeta\n", Lines: []int{1, 2, 3}}})
	if err != nil {
		t.Fatal(err)
	}
	expected := map[int]string{1: "VAS", 2: "nF3", 3: "Asx"}
	if !reflect.DeepEqual(lookup["source.txt"], expected) {
		t.Fatalf("lookup=%#v expected=%#v", lookup, expected)
	}
}

func TestDefaultReadAnchorsUseNativeWithoutConfiguredCommand(t *testing.T) {
	content := "alpha\nbeta\n"
	cases := []struct {
		name     string
		settings config.Anchors
	}{
		{name: "product default"},
		{name: "enabled without provider", settings: config.Anchors{EnabledByDefault: true}},
		{name: "explicit native", settings: config.Anchors{EnabledByDefault: true, DefaultProvider: "native"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			writeJSONFile(t, settingsPath, config.UserSettings{Anchors: testCase.settings})
			t.Setenv("GREPPLE_SETTINGS", settingsPath)
			anchors, enabled, err := anchor.Read("source.txt", content, []int{1, 2, 3})
			if err != nil {
				t.Fatal(err)
			}
			expected := map[int]string{1: "VAS", 2: "nF3", 3: "Asx"}
			if !enabled || !reflect.DeepEqual(anchors, expected) {
				t.Fatalf("enabled=%v anchors=%#v expected=%#v", enabled, anchors, expected)
			}
		})
	}
}
