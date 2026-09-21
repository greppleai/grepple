package anchors

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/internal/usersettings"
)

func TestNativeAnchorLookupUsesBuiltInHashline(t *testing.T) {
	content := "alpha\nbeta\n"
	request := anchorProtocolRequest{ProtocolVersion: anchorProtocolVersion, Files: []anchorProtocolRequestFile{{Path: "/tmp/source", Content: content, Lines: []int{1, 2, 3}}}}
	lookup, err := nativeAnchorLookup(request, map[string]string{"/tmp/source": "source.txt"})
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
		settings usersettings.Anchors
	}{
		{name: "product default"},
		{name: "enabled without provider", settings: usersettings.Anchors{EnabledByDefault: true}},
		{name: "explicit native", settings: usersettings.Anchors{EnabledByDefault: true, DefaultProvider: "native"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settingsPath := filepath.Join(t.TempDir(), "settings.json")
			writeJSONFile(t, settingsPath, usersettings.Config{Anchors: testCase.settings})
			t.Setenv("GREPPLE_SETTINGS", settingsPath)
			anchors, enabled, err := Read("source.txt", content, []int{1, 2, 3})
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
