package shard

import "testing"

func TestParseShardOptions(t *testing.T) {
	t.Setenv("GREPPLE_PORT", "")
	t.Setenv("GREPPLE_ROOT", "")
	t.Setenv("GREPPLE_ZOEKT_INDEX", "")
	t.Setenv("GREPPLE_ZOEKT_PORT", "")
	t.Setenv("GREPPLE_ZOEKT_BIN", "")

	options, err := parseShardOptions([]string{
		"--host", "127.0.0.1",
		"--port", "9000",
		"--root", "/tmp/repos",
		"--zoekt-port", "6071",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.host != "127.0.0.1" || options.port != 9000 || options.root != "/tmp/repos" {
		t.Fatalf("unexpected options: %#v", options)
	}
	if options.zoekt.port != 6071 || options.zoekt.indexDir != defaultZoektIndex("/tmp/repos") {
		t.Fatalf("unexpected zoekt options: %#v", options.zoekt)
	}
}

func TestParseShardOptionsRejectsZeroZoektPort(t *testing.T) {
	t.Setenv("GREPPLE_ZOEKT_PORT", "")
	if _, err := parseShardOptions([]string{"--zoekt-port", "0"}); err == nil {
		t.Fatal("expected zero Zoekt port to be rejected")
	}
}
