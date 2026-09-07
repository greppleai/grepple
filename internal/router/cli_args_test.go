package router

import "testing"

func TestParseArgs(t *testing.T) {
	for _, key := range []string{
		"GREPPLE_PORT", "GREPPLE_TIMEOUT",
		"GREPPLE_BACKENDS", "SHARD_HOSTS",
		"WATCH_REPOS", "WATCH_ORGS",
		"GREPPLE_RECONCILE_INTERVAL", "GREPPLE_RECONCILE_BATCH",
	} {
		t.Setenv(key, "")
	}

	options, err := parseArgs([]string{
		"--host", "127.0.0.1",
		"--port", "8081",
		"--backend", "shard-a:8787",
		"--backend", "http://shard-b:8787",
		"--timeout", "2500",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.host != "127.0.0.1" || options.port != 8081 {
		t.Fatalf("unexpected listener options: %#v", options)
	}
	if len(options.backends) != 2 || options.backends[0] != "http://shard-a:8787" {
		t.Fatalf("unexpected backends: %#v", options.backends)
	}
	if options.timeout.Milliseconds() != 2500 {
		t.Fatalf("unexpected timeout: %s", options.timeout)
	}
	if options.reconcileBatch != 50 {
		t.Fatalf("reconcileBatch=%d want default 50", options.reconcileBatch)
	}
	if opts, err := parseArgs([]string{"--backend", "d:8787", "--reconcile-batch", "200"}); err != nil || opts.reconcileBatch != 200 {
		t.Fatalf("--reconcile-batch override: %#v err=%v", opts, err)
	}
	if _, err := parseArgs([]string{"--backend", "d:8787", "--reconcile-batch", "-1"}); err == nil {
		t.Fatal("expected error for negative --reconcile-batch")
	}
	if _, err := parseArgs([]string{"--backend", "http://bad host"}); err == nil {
		t.Fatal("expected invalid backend URL error")
	}
}
