#!/usr/bin/env python3
"""Compare immutable graph snapshots; does not claim production invalidation parity."""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import signal
import socket
import statistics
import subprocess
import time


def run(command):
    start = time.perf_counter_ns()
    process = subprocess.run(command, check=True, capture_output=True, text=True)
    return json.loads(process.stdout), time.perf_counter_ns() - start


def percentile(values, p):
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, int((len(ordered) - 1) * p))]


def summarize(records):
    result = {"samples": len(records)}
    for key in ["wall_ns", "load_ns", "query_ns", "encode_ns"]:
        values = [r[key] for r in records if key in r]
        if values:
            result[key] = {"p50": statistics.median(values), "p95": percentile(values, .95)}
    result["rss_kb"] = max(r.get("rss_kb", 0) for r in records)
    return result


class UnixConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("localhost", timeout=60)
        self.path = path

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(self.path)


def health(path):
    connection = UnixConnection(path)
    try:
        connection.request("GET", "/health")
        response = connection.getresponse()
        assert response.status == 200
        return json.loads(response.read())
    finally:
        connection.close()


def benchmark(data, datasets, rounds, repeats, backends):
    results = {"methodology": "fresh processes with warm filesystem page cache; immutable graph snapshots; one DB thread; GOMAXPROCS=4", "datasets": {}}
    for dataset in datasets:
        meta = data / f"{dataset}.json.meta.json"
        info = json.loads(meta.read_text())
        counts = {k: v for k, v in info.items() if k != "queries"}
        result = {"info": counts, "backends": {}, "sizes": {ext: (data / f"{dataset}.{ext}").stat().st_size for ext in ["json", "pb", "duckdb", "column.duckdb"]}}
        oracles = None
        for backend in backends:
            print(dataset, backend, flush=True)
            binary = data / ("duck-index" if backend.startswith("duckdb") else "go-index")
            artifact = data / (f"{dataset}.column.duckdb" if backend == "duckdb-col" else f"{dataset}.{'duckdb' if backend == 'duckdb' else 'pb'}")
            common = [str(binary), "-backend", backend, "-input", str(artifact), "-meta", str(meta)]
            verified = data / f"{dataset}.{backend}.parity.json"
            subprocess.run(common + ["-mode", "verify", "-out", str(verified)], check=True)
            digests = [r["digest"] for r in json.loads(verified.read_text())]
            if oracles is None:
                oracles = digests
            assert digests == oracles, (dataset, backend, "semantic mismatch")
            resident, _ = run(common + ["-mode", "bench", "-rounds", str(rounds)])
            for ordinal, r in enumerate(resident["records"]):
                assert r["digest"] == oracles[ordinal % len(oracles)]
            entry = {"resident": summarize(resident["records"]), "resident_load_ns": resident["load_ns"]}
            (data / f"{dataset}.{backend}.resident.json").write_text(json.dumps(resident))
            cli_records = []
            # Interleave shallow/deep and outgoing/incoming/bidirectional requests.
            for repeat in range(repeats):
                for ordinal in range(len(oracles)):
                    r, elapsed = run(common + ["-mode", "query", "-query", str(ordinal)])
                    assert r["digest"] == oracles[ordinal]
                    r["wall_ns"] = elapsed
                    r["query_ordinal"] = ordinal
                    cli_records.append(r)
            entry["cli"] = summarize(cli_records)
            (data / f"{dataset}.{backend}.cli.json").write_text(json.dumps(cli_records))
            sock = data / f"{dataset}-{backend}.sock"
            process = subprocess.Popen(common + ["-mode", "serve", "-socket", str(sock)], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
            try:
                start = time.monotonic()
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(process.stderr.read())
                    try:
                        startup = health(str(sock))
                        break
                    except (OSError, http.client.HTTPException):
                        if time.monotonic() - start > 60:
                            raise TimeoutError("daemon startup")
                        time.sleep(.02)
                # Warm every query on the resident daemon before timed client forks.
                client = [str(data / "go-index"), "-mode", "client", "-meta", str(meta), "-socket", str(sock)]
                for ordinal in range(len(oracles)):
                    r, _ = run(client + ["-query", str(ordinal)])
                    assert r["digest"] == oracles[ordinal]
                daemon_records = []
                for repeat in range(repeats):
                    for ordinal in range(len(oracles)):
                        r, elapsed = run(client + ["-query", str(ordinal)])
                        assert r["digest"] == oracles[ordinal]
                        r["wall_ns"] = elapsed
                        r["query_ordinal"] = ordinal
                        daemon_records.append(r)
                entry["daemon"] = summarize(daemon_records)
                entry["daemon_startup"] = startup
                (data / f"{dataset}.{backend}.daemon.json").write_text(json.dumps(daemon_records))
            finally:
                process.send_signal(signal.SIGTERM)
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                process.stderr.close()
            result["backends"][backend] = entry
        results["datasets"][dataset] = result
        (data / "summary.json").write_text(json.dumps(results, indent=2))
    return results


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--data", type=Path, default=Path("/workspace/.tmp/navigation-duckdb"))
    parser.add_argument("--datasets", nargs="+", default=["base", "scale10"])
    parser.add_argument("--backends", nargs="+", default=["scan-pb","adj-pb","duckdb","duckdb-col"])
    parser.add_argument("--rounds", type=int, default=7)
    parser.add_argument("--repeats", type=int, default=2)
    args = parser.parse_args()
    benchmark(args.data, args.datasets, args.rounds, args.repeats,args.backends)