package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/greppleai/grepple/internal/parser"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type measurement struct {
	Backend     string `json:"backend"`
	LoadNS      int64  `json:"load_ns"`
	QueryNS     int64  `json:"query_ns"`
	EncodeNS    int64  `json:"encode_ns"`
	ResultBytes int    `json:"result_bytes"`
	Digest      string `json:"digest"`
	RSSKB       int64  `json:"rss_kb"`
	HeapBytes   uint64 `json:"heap_bytes"`
}
type reply struct {
	Measurement measurement     `json:"measurement"`
	Graph       json.RawMessage `json:"graph"`
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func main() {
	mode := flag.String("mode", "query", "export, pack, build, query, serve, client, bench, verify")
	backend := flag.String("backend", "scan", "scan, adj, scan-pb, adj-pb, duckdb or duckdb-col")
	input := flag.String("input", "", "graph JSON or DuckDB file")
	meta := flag.String("meta", "", "dataset metadata")
	socket := flag.String("socket", "", "private Unix socket")
	repos := flag.String("repos", "/workspace/grepple,/workspace/grepple-backend", "explicit tracked source repositories")
	out := flag.String("out", "", "artifact path")
	replicas := flag.Int("replicas", 1, "disconnected replicas of actual resolved graph")
	queryNumber := flag.Int("query", 0, "query ordinal")
	rounds := flag.Int("rounds", 5, "mixed workload rounds")
	flag.Parse()
	runtime.GOMAXPROCS(4)
	if *mode == "export" {
		die(exportDataset(*repos, *out, *replicas))
		return
	}
	if *mode == "build" {
		g, e := loadGraph(*input)
		die(e)
		builder := buildDuck
		if *backend == "duckdb-col" {
			builder = selectedColumnBuilder
		}
		duration, e := builder(*out, g)
		die(e)
		die(json.NewEncoder(os.Stdout).Encode(map[string]any{"build_ns": duration.Nanoseconds(), "rss_kb": rssKB()}))
		return
	}
	if *mode == "pack" {
		graph, err := loadGraph(*input)
		die(err)
		start := time.Now()
		payload, err := parser.MarshalNavigationFactArtifact(parser.NavigationFactArtifact{Digest: "experiment", Graph: graph})
		die(err)
		die(os.WriteFile(*out, payload, 0600))
		die(json.NewEncoder(os.Stdout).Encode(map[string]any{"pack_ns": time.Since(start).Nanoseconds(), "bytes": len(payload)}))
		return
	}
	info, e := loadInfo(*meta)
	die(e)
	if *queryNumber < 0 || *queryNumber >= len(info.Queries) {
		die(fmt.Errorf("invalid query ordinal"))
	}
	q := info.Queries[*queryNumber]
	if *mode == "client" {
		r, e := queryDaemon(*socket, q)
		die(e)
		sum := sha256.Sum256(r.Graph)
		if r.Measurement.Digest != hex.EncodeToString(sum[:]) {
			die(fmt.Errorf("transport result digest mismatch"))
		}
		die(json.NewEncoder(os.Stdout).Encode(r.Measurement))
		return
	}
	start := time.Now()
	index, e := openIndex(*backend, *input)
	die(e)
	defer index.Close()
	load := time.Since(start)
	switch *mode {
	case "serve":
		die(serve(index, *backend, *socket, load))
	case "bench":
		die(bench(index, *backend, load, info.Queries, *rounds))
	case "verify":
		die(verify(index, *backend, info.Queries, *out))
	default:
		r, e := measure(index, *backend, q, load)
		die(e)
		die(json.NewEncoder(os.Stdout).Encode(r.Measurement))
	}
}
func openIndex(backend, path string) (queryIndex, error) {
	if backend == "duckdb-col" {
		return selectedColumnOpener(path)
	}
	if backend == "duckdb" {
		return openDuck(path)
	}
	var graph parser.NavigationGraph
	var err error
	if strings.HasSuffix(backend, "-pb") {
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		artifact, e := parser.UnmarshalNavigationFactArtifact(data)
		if e != nil {
			return nil, e
		}
		graph = artifact.Graph
		backend = strings.TrimSuffix(backend, "-pb")
	} else {
		graph, err = loadGraph(path)
	}
	if err != nil {
		return nil, err
	}
	if backend == "adj" {
		return newAdjacency(graph), nil
	}
	if backend != "scan" {
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
	return &scanIndex{graph: graph}, nil
}
func measure(index queryIndex, backend string, q request, load time.Duration) (reply, error) {
	start := time.Now()
	graph, err := index.Query(context.Background(), q)
	queryTime := time.Since(start)
	if err != nil {
		return reply{}, err
	}
	start = time.Now()
	data, err := json.Marshal(graph)
	encodeTime := time.Since(start)
	if err != nil {
		return reply{}, err
	}
	digest := sha256.Sum256(data)
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return reply{Graph: data, Measurement: measurement{Backend: backend, LoadNS: load.Nanoseconds(), QueryNS: queryTime.Nanoseconds(), EncodeNS: encodeTime.Nanoseconds(), ResultBytes: len(data), Digest: hex.EncodeToString(digest[:]), RSSKB: rssKB(), HeapBytes: stats.HeapAlloc}}, nil
}
func rssKB() int64 {
	data, _ := os.ReadFile("/proc/self/status")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			fields := strings.Fields(line)
			v, _ := strconv.ParseInt(fields[1], 10, 64)
			return v
		}
	}
	return 0
}
func bench(index queryIndex, backend string, load time.Duration, queries []request, rounds int) error {
	for _, q := range queries {
		if _, err := measure(index, backend, q, load); err != nil {
			return err
		}
	}
	var records []measurement
	for range rounds {
		for _, q := range queries {
			r, err := measure(index, backend, q, load)
			if err != nil {
				return err
			}
			records = append(records, r.Measurement)
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"backend": backend, "load_ns": load.Nanoseconds(), "records": records, "rss_kb": rssKB()})
}
func verify(index queryIndex, backend string, queries []request, out string) error {
	var records []measurement
	for _, q := range queries {
		r, err := measure(index, backend, q, 0)
		if err != nil {
			return err
		}
		records = append(records, r.Measurement)
	}
	return writeJSON(out, records)
}
func serve(index queryIndex, backend, socket string, load time.Duration) error {
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		return fmt.Errorf("socket path already exists: %s", socket)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"load_ns": load.Nanoseconds(), "rss_kb": rssKB()})
	})
	mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		var q request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		if err := decoder.Decode(&q); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		result, err := measure(index, backend, q, load)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second, IdleTimeout: 5 * time.Second}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stopped)
	go func() {
		<-stopped
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func queryDaemon(socket string, q request) (reply, error) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	data, err := json.Marshal(q)
	if err != nil {
		return reply{}, err
	}
	response, err := client.Post("http://unix/query", "application/json", bytes.NewReader(data))
	if err != nil {
		return reply{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return reply{}, fmt.Errorf("daemon HTTP %d", response.StatusCode)
	}
	var result reply
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}
