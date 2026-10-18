package outputspill

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservedFinalizationModesAndDeduplication(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		args          []string
		options       Options
		mode          OutputMode
		artifact      int64
	}{
		{name: "empty", payload: "", mode: ModeInline},
		{name: "small text", payload: "short\n", mode: ModeInline},
		{name: "small json", args: []string{"--json"}, payload: `{"ok":true}` + "\n", mode: ModeInline},
		{name: "no spill", payload: strings.Repeat("private", 20), options: Options{Disabled: true}, mode: ModeInline},
		{name: "large text", payload: strings.Repeat("private", 20), mode: ModeSpilled, artifact: 140},
		{name: "large json", args: []string{"--json"}, payload: strings.Repeat("private", 20), mode: ModeSpilled, artifact: 140},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			options := tc.options
			options.Directory = root
			for iteration := 0; iteration < 2; iteration++ {
				stdout, err := os.CreateTemp(t.TempDir(), "stdout")
				if err != nil {
					t.Fatal(err)
				}
				original := os.Stdout
				os.Stdout = stdout
				var reports []OutputReport
				err = RunWithFinalizerObserved(tc.args, options, 16, root, func(int) error {
					_, e := io.WriteString(os.Stdout, tc.payload)
					return e
				}, nil, func(report OutputReport) { reports = append(reports, report) })
				os.Stdout = original
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(stdout.Name())
				_ = stdout.Close()
				if err != nil {
					t.Fatal(err)
				}
				if len(reports) != 1 {
					t.Fatalf("report count = %d", len(reports))
				}
				report := reports[0]
				wantArtifact := tc.artifact
				if iteration == 1 {
					wantArtifact = 0
				} // existing content-addressed artifact
				if !report.LogicalObserved || report.LogicalBytes != int64(len(tc.payload)) || report.StdoutBytes != int64(len(data)) || report.Mode != tc.mode || report.ArtifactBytes != wantArtifact {
					t.Fatalf("iteration %d report=%+v stdout=%q", iteration, report, data)
				}
				if tc.mode == ModeInline && string(data) != tc.payload {
					t.Fatalf("inline output changed: %q", data)
				}
				if tc.mode == ModeSpilled {
					digest := sha256.Sum256([]byte(tc.payload))
					extension := ".txt"
					if len(tc.args) != 0 {
						extension = ".json"
					}
					artifact, err := os.ReadFile(filepath.Join(root, hex.EncodeToString(digest[:])+extension))
					if err != nil || string(artifact) != tc.payload {
						t.Fatalf("artifact changed: %v", err)
					}
					if !strings.Contains(string(data), "grepple-artifact-v1") && len(tc.args) != 0 {
						t.Fatalf("missing JSON descriptor: %q", data)
					}
				}
			}
		})
	}
}

type partialWriter struct {
	n    int
	err  error
	data []byte
}

func (w *partialWriter) Write(p []byte) (int, error) {
	n := min(len(p), w.n)
	w.data = append(w.data, p[:n]...)
	return n, w.err
}

func TestObservedFinalizationPartialWritesAndErrors(t *testing.T) {
	failure := errors.New("private command failure")
	for _, tc := range []struct {
		name, payload string
		threshold     int
		writer        *partialWriter
		commandErr    error
		wantError     error
		mode          OutputMode
		artifact      int64
	}{
		{name: "inline short write", payload: "private output", threshold: 100, writer: &partialWriter{n: 3}, wantError: io.ErrShortWrite, mode: ModeInline},
		{name: "inline command failure", payload: "private output", threshold: 100, writer: &partialWriter{n: 3, err: io.ErrClosedPipe}, commandErr: failure, wantError: failure, mode: ModeInline},
		{name: "descriptor partial write", payload: strings.Repeat("private", 10), threshold: 1, writer: &partialWriter{n: 4, err: io.ErrClosedPipe}, wantError: io.ErrClosedPipe, mode: ModeSpilled, artifact: 70},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			temporary, err := os.CreateTemp(root, "spill")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(temporary, tc.payload); err != nil {
				t.Fatal(err)
			}
			var reports []OutputReport
			err = finishOutputSpill(temporary, temporary.Name(), root, root, tc.threshold, []string{"search"}, tc.writer, tc.commandErr, func(report OutputReport) { reports = append(reports, report) })
			if !errors.Is(err, tc.wantError) || len(reports) != 1 {
				t.Fatalf("error=%v reports=%+v", err, reports)
			}
			r := reports[0]
			if !r.LogicalObserved || r.LogicalBytes != int64(len(tc.payload)) || r.StdoutBytes != int64(len(tc.writer.data)) || r.StdoutBytes != int64(tc.writer.n) || r.Mode != tc.mode || r.ArtifactBytes != tc.artifact {
				t.Fatalf("incorrect partial-write accounting: %+v", r)
			}
		})
	}
}

func TestObservedFinalizationBypassAndBrokenPipe(t *testing.T) {
	root := t.TempDir()
	called := false
	err := RunWithFinalizerObserved([]string{"artifacts", "clean"}, Options{Disabled: true, Directory: root}, 1, root, func(int) error { return nil }, nil, func(OutputReport) { called = true })
	if err != nil || called {
		t.Fatalf("bypass observed finalization: %v %v", err, called)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	defer writer.Close()
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	var report OutputReport
	err = RunWithFinalizerObserved(nil, Options{Directory: root}, 100, root, func(int) error { _, e := io.WriteString(os.Stdout, "private output"); return e }, nil, func(r OutputReport) { report = r })
	if err == nil {
		t.Fatal("expected broken pipe")
	}
	if report.Mode != ModeInline || report.LogicalBytes != 14 || report.StdoutBytes != 0 {
		t.Fatalf("broken pipe counts: %+v", report)
	}
}

func TestObservedFinalizationFailedStatAndPersist(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, path string
		logical    bool
	}{
		{name: "stat fails", path: filepath.Join(root, "missing")},
		{name: "persist fails", logical: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp(root, "spill")
			if err != nil {
				t.Fatal(err)
			}
			path := tc.path
			if tc.logical {
				path = file.Name()
				if _, err := io.WriteString(file, "private output"); err != nil {
					t.Fatal(err)
				}
			}
			var report OutputReport
			err = finishOutputSpill(file, path, filepath.Join(root, "missing-directory"), root, 1, nil, &partialWriter{n: 1}, nil, func(r OutputReport) { report = r })
			if err == nil || report.LogicalObserved != tc.logical || report.StdoutBytes != 0 || report.Mode != "" || report.ArtifactBytes != 0 {
				t.Fatalf("failed boundary invented output: err=%v report=%+v", err, report)
			}
		})
	}
}
