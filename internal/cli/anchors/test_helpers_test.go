package anchors

import (
	"io"
	"os"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func Run(args []string) error {
	if len(args) >= 2 && args[0] == "anchors" {
		return New(cliruntime.Environment{}).Run(args[1:])
	}
	if len(args) >= 3 && args[0] == "help" && args[1] == "anchors" {
		switch args[2] {
		case "doctor":
			return RunDoctor([]string{"--help"})
		case "setup":
			return RunSetup([]string{"--help"})
		}
	}
	return New(cliruntime.Environment{}).Run(args)
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = previous }()
	run()
	_ = writer.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	return string(content)
}
