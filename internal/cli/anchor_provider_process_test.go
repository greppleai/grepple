package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestAnchorProviderProcess(_ *testing.T) {
	if os.Getenv("GREPPLE_TEST_ANCHOR_PROVIDER") != "1" {
		return
	}
	if os.Getenv("GREPPLE_TEST_ANCHOR_PROVIDER_SLOW") == "1" {
		time.Sleep(200 * time.Millisecond)
	}
	type requestFile struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Lines  []int  `json:"lines"`
	}
	type request struct {
		ProtocolVersion int           `json:"protocol_version"`
		Files           []requestFile `json:"files"`
	}
	type responseLine struct {
		Line   int    `json:"line"`
		Anchor string `json:"anchor"`
	}
	type responseFile struct {
		Path    string         `json:"path"`
		SHA256  string         `json:"sha256"`
		Anchors []responseLine `json:"anchors"`
	}
	type response struct {
		ProtocolVersion int            `json:"protocol_version"`
		Files           []responseFile `json:"files"`
	}
	var input request
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		os.Exit(41)
	}
	output := response{ProtocolVersion: input.ProtocolVersion}
	for _, file := range input.Files {
		result := responseFile{Path: file.Path, SHA256: file.SHA256}
		for _, line := range file.Lines {
			result.Anchors = append(result.Anchors, responseLine{Line: line, Anchor: fmt.Sprintf("A%02d", line)})
		}
		output.Files = append(output.Files, result)
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		os.Exit(42)
	}
	os.Exit(0)
}
