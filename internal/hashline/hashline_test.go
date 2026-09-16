package hashline

import (
	"reflect"
	"testing"
)

func TestLinesMatchesHashlineV1CompatibilityVectors(t *testing.T) {
	content := "alpha\nbeta  \nalpha\n\n"
	expected := []string{"VAS", "nF3", "wOG", "Asx", "hmj"}
	if actual := Lines(content); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("hashes=%#v expected=%#v", actual, expected)
	}
}

func TestLinesCanonicalizesCRLFAndTrailingWhitespace(t *testing.T) {
	lf := Lines("alpha\nbeta\n")
	crlf := Lines("alpha\r\nbeta\t\r\n")
	if !reflect.DeepEqual(lf, crlf) {
		t.Fatalf("lf=%#v crlf=%#v", lf, crlf)
	}
}

func TestValid(t *testing.T) {
	for _, value := range []string{"VAS", "A-_", "012"} {
		if !Valid(value) {
			t.Fatalf("expected valid anchor %q", value)
		}
	}
	for _, value := range []string{"", "ab", "abcd", "a+b", "éab"} {
		if Valid(value) {
			t.Fatalf("expected invalid anchor %q", value)
		}
	}
}
