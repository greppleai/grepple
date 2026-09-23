package sourcelocation

import "testing"

func TestParseLine(t *testing.T) {
	path, line, err := ParseLine("dir/file.go:42")
	if err != nil || path != "dir/file.go" || line != 42 {
		t.Fatalf("path=%q line=%d err=%v", path, line, err)
	}
	for _, value := range []string{"file.go", "file.go:0", "file.go:nope"} {
		if _, _, err := ParseLine(value); err == nil {
			t.Fatalf("ParseLine(%q) succeeded", value)
		}
	}
}

func TestParseExplicitRange(t *testing.T) {
	selector, err := Parse("dir/file.go:10-14")
	if err != nil {
		t.Fatal(err)
	}
	if selector.Path != "dir/file.go" || selector.Start != 10 || selector.End != 14 || !selector.ExplicitEnd {
		t.Fatalf("selector=%+v", selector)
	}
	if _, err := Parse("dir/file.go:14-10"); err == nil {
		t.Fatal("descending range succeeded")
	}
}
