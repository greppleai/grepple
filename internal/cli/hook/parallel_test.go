package hook

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/greppleai/grepple/gritql"
)

func TestParallelHookMatchesOrderedSerialResults(t *testing.T) {
	root := testHookRepository(t)
	for i := 0; i < 8; i++ {
		writeHookTestFile(t, root, fmt.Sprintf("case%02d.go", i), fmt.Sprintf("package demo\nimport . \"fmt\"\nfunc f%d(ok bool) { if ok {} }\n", i))
	}
	rules, err := loadRules(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 8)
	for i := range paths {
		paths[i] = fmt.Sprintf("case%02d.go", i)
	}
	serial, err := scanRulesUncached(context.Background(), root, paths, rules, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	parallel, err := scanRulesUncached(context.Background(), root, paths, rules, true, 4)
	if err != nil || !reflect.DeepEqual(parallel, serial) || len(parallel) != 16 {
		t.Fatalf("parallel=%v serial=%v err=%v", parallel, serial, err)
	}
	writeHookTestFile(t, root, "case07.go", "package demo\nimport . \"fmt\"\nfunc broken( {\n")
	_, serialErr := scanRulesUncached(context.Background(), root, paths, rules, true, 1)
	_, parallelErr := scanRulesUncached(context.Background(), root, paths, rules, true, 4)
	if serialErr == nil || parallelErr == nil || !strings.Contains(serialErr.Error(), "SOURCE_PARSE") || !strings.Contains(parallelErr.Error(), "SOURCE_PARSE") {
		t.Fatalf("serial error=%v parallel error=%v", serialErr, parallelErr)
	}
}

type observingHookFS struct {
	files  fstest.MapFS
	active atomic.Int32
	max    atomic.Int32
}

func (filesystem *observingHookFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(filesystem.files, name)
}

func (filesystem *observingHookFS) Open(name string) (fs.File, error) {
	current := filesystem.active.Add(1)
	for previous := filesystem.max.Load(); current > previous; previous = filesystem.max.Load() {
		if filesystem.max.CompareAndSwap(previous, current) {
			break
		}
	}
	time.Sleep(15 * time.Millisecond)
	defer filesystem.active.Add(-1)
	return filesystem.files.Open(name)
}

func TestParallelHookActuallyReadsFilesConcurrently(t *testing.T) {
	program, err := gritql.Compile([]byte("language go\n`if $condition {}`"), gritql.CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filesystem := &observingHookFS{files: make(fstest.MapFS)}
	candidates := make([]gritql.ScanCandidate, 8)
	for i := range candidates {
		path := fmt.Sprintf("pkg/file%02d.go", i)
		filesystem.files[path] = &fstest.MapFile{Data: []byte("package demo\nfunc f(ok bool) { if ok {} }\n")}
		candidates[i] = gritql.ScanCandidate{Path: path, ReadPath: path}
	}
	programs := []gritql.ProgramScan{{Program: program, PatternID: "empty-if"}}
	rows, err := scanProgramFiles(context.Background(), filesystem, programs, candidates, gritql.ScanOptions{}, 4, false)
	if err != nil || len(rows) != len(candidates) || filesystem.max.Load() < 2 {
		t.Fatalf("rows=%d max concurrent reads=%d err=%v", len(rows), filesystem.max.Load(), err)
	}
	var found []string
	for _, row := range rows {
		for _, finding := range row[0].Result.Findings() {
			found = append(found, finding.Path())
		}
	}
	if len(found) != 8 || !sort.StringsAreSorted(found) {
		t.Fatalf("finding paths=%v", found)
	}
}

func TestHookRejectsInvalidWorkerCounts(t *testing.T) {
	root := testHookRepository(t)
	if _, err := os.Stat(filepath.Join(root, ".grepple", "hooks")); err != nil {
		t.Fatal(err)
	}
	for _, workers := range []string{"0", "5", "-1"} {
		if _, _, err := runHookTest(t, "--all", "--workers", workers); err == nil || !strings.Contains(err.Error(), "--workers") {
			t.Fatalf("workers=%s error=%v", workers, err)
		}
	}
}
