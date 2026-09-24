#!/usr/bin/env python3
"""Reproducible, bounded Grepple CLI benchmarks on explicit repository checkouts."""

import argparse
import csv
import hashlib
import json
import os
from pathlib import Path
import random
import re
import shlex
import signal
import subprocess
import sys
import threading
import time

DEFAULT_CASES = (
    "tree=tree --depth 1 .",
    "directory=architecture directory --depth 2 --max-nodes 80 .",
)
SOURCE_FIELDS = ("discovered", "selected", "parsed", "skipped", "failed", "recovered")
SOURCE_RE = re.compile(r"\b(?:sources=)?discovered:(\d+),selected:(\d+),parsed:(\d+),skipped:(\d+),failed:(\d+),recovered:(\d+)")
JSON_SOURCES_RE = re.compile(r'"sources"\s*:\s*\{([^{}]{0,1024})\}', re.DOTALL)
JSON_SOURCE_FIELD_RE = re.compile(r'"(' + '|'.join(SOURCE_FIELDS) + r')"\s*:\s*(\d+)')


def parse_sources(preview):
    human = SOURCE_RE.search(preview)
    if human:
        return dict(zip(SOURCE_FIELDS, map(int, human.groups())))
    document = JSON_SOURCES_RE.search(preview)
    if document:
        fields = {name: int(value) for name, value in JSON_SOURCE_FIELD_RE.findall(document.group(1))}
        if all(name in fields for name in SOURCE_FIELDS):
            return {name: fields[name] for name in SOURCE_FIELDS}
    return None


def case_from_text(text):
    name, separator, command = text.partition("=")
    if not separator or not re.fullmatch(r"[a-zA-Z][a-zA-Z0-9_-]*", name):
        raise ValueError(f"case must be NAME=ARGUMENTS: {text!r}")
    argv = shlex.split(command)
    if not argv or argv[0].startswith("-"):
        raise ValueError(f"case requires a Grepple command: {text!r}")
    return name, argv


def select_repositories(paths, sample, seed):
    if not paths:
        raise ValueError("provide --repo, --url, or --repo-list")
    if sample < 0 or sample > len(paths):
        raise ValueError("--sample must be between 0 and the repository count")
    if sample:
        return random.Random(seed).sample(paths, sample)
    return paths


def git(repo, *args):
    return subprocess.check_output(["git", "-C", str(repo), *args], text=True, stderr=subprocess.PIPE).strip()


def repository_identity(repo):
    root = Path(git(repo, "rev-parse", "--show-toplevel")).resolve()
    if root != repo.resolve():
        raise ValueError(f"expected checkout root, got {repo} (root: {root})")
    return {"path": str(root), "revision": git(root, "rev-parse", "HEAD"),
            "status_before": git(root, "status", "--porcelain", "--untracked-files=all")}


def peak_rss_kib(pid, done, samples):
    """Linux process high-water mark; None on systems without /proc."""
    path = Path(f"/proc/{pid}/status")
    while not done.is_set():
        try:
            for line in path.read_text().splitlines():
                if line.startswith("VmHWM:"):
                    samples.append(int(line.split()[1]))
                    break
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            break
        done.wait(0.05)


def invoke(binary, repo, argv, cache_dir, timeout, output_dir, label, keep_output):
    cache_dir.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["GREPPLE_NAVIGATION_CACHE_DIR"] = str(cache_dir)
    stdout_path = output_dir / f"{label}.stdout"
    stderr_path = output_dir / f"{label}.stderr"
    full_command = [str(binary), "--no-spill", *argv]
    time_binary = Path("/usr/bin/time")
    time_path = output_dir / f"{label}.time"
    execution = ([str(time_binary), "-f", "%M", "-o", str(time_path), "--", *full_command]
                 if time_binary.is_file() else full_command)
    samples = []
    start = time.monotonic()
    with stdout_path.open("wb") as stdout, stderr_path.open("wb") as stderr:
        process = subprocess.Popen(execution, cwd=repo, env=env, stdout=stdout, stderr=stderr,
                                   start_new_session=True)
        done = threading.Event()
        sampler = None
        if not time_binary.is_file():
            sampler = threading.Thread(target=peak_rss_kib, args=(process.pid, done, samples), daemon=True)
            sampler.start()
        timed_out = False
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
        finally:
            done.set()
            if sampler is not None:
                sampler.join()
    seconds = time.monotonic() - start
    rss_method = "proc-sampled"
    peak = max(samples, default=None)
    if time_path.is_file():
        rss_method = "gnu-time"
        try:
            peak = int(time_path.read_text().strip().splitlines()[-1])
        except (ValueError, IndexError):
            peak = None
        time_path.unlink()
    with stdout_path.open("rb") as stream:
        preview = stream.read(8192).decode("utf-8", "replace")
    with stderr_path.open("rb") as stream:
        stderr_excerpt = stream.read(700).decode("utf-8", "replace")
    result = {"command": full_command, "seconds": round(seconds, 4), "exit_code": None if timed_out else process.returncode,
              "timed_out": timed_out, "peak_rss_kib": peak, "rss_method": rss_method,
              "stdout_bytes": stdout_path.stat().st_size, "stderr_bytes": stderr_path.stat().st_size,
              "stderr_excerpt": stderr_excerpt, "sources": parse_sources(preview)}
    if keep_output:
        result.update(stdout=str(stdout_path), stderr=str(stderr_path))
    else:
        stdout_path.unlink()
        stderr_path.unlink()
    return result


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(args):
    entries = list(args.repo) + list(args.url)
    if args.repo_list:
        entries += [line.strip() for line in args.repo_list.read_text().splitlines()
                    if line.strip() and not line.lstrip().startswith("#")]
    entries = select_repositories(entries, args.sample, args.seed)
    cases = [case_from_text(text) for text in (args.case or DEFAULT_CASES)]
    if len({name for name, _ in cases}) != len(cases):
        raise ValueError("case names must be unique")
    if args.runs < 1 or args.timeout <= 0 or args.clone_timeout <= 0:
        raise ValueError("--runs, --timeout, and --clone-timeout must be positive")
    binary = args.binary.resolve(strict=True)
    if not binary.is_file():
        raise ValueError(f"not an executable file: {binary}")
    output_dir = args.output.resolve()
    for entry in entries:
        if entry.startswith("https://"):
            continue
        repo = Path(entry).expanduser().resolve(strict=True)
        if repo == output_dir or repo in output_dir.parents:
            raise ValueError(f"--output must be outside checkout {repo}")
    output_dir.mkdir(parents=True, exist_ok=False)
    manifest = {"schema": "grepple-repository-benchmark-v1", "grepple_binary": str(binary),
                "grepple_sha256": sha256(binary), "seed": args.seed, "sample": args.sample,
                "timeout_seconds": args.timeout, "runs_per_case": args.runs,
                "cases": [{"name": name, "argv": argv} for name, argv in cases], "repositories": [], "measurements": []}
    for repo_index, entry in enumerate(entries):
        if entry.startswith("https://"):
            repo = output_dir / "clones" / str(repo_index)
            repo.parent.mkdir(parents=True, exist_ok=True)
            subprocess.run(["git", "clone", "--depth", "1", "--", entry, str(repo)],
                           check=True, timeout=args.clone_timeout, stdout=subprocess.DEVNULL)
        else:
            repo = Path(entry).expanduser().resolve(strict=True)
        identity = repository_identity(repo)
        manifest["repositories"].append(identity)
        for name, argv in cases:
            cache = output_dir / "cache" / str(repo_index) / name
            for iteration in range(args.runs):
                label = f"repo{repo_index}-{name}-{iteration}"
                result = invoke(binary, repo, argv, cache, args.timeout, output_dir, label, args.keep_output)
                result.update(repo_index=repo_index, case=name, iteration=iteration,
                              cache_state="cold" if iteration == 0 else "warm")
                manifest["measurements"].append(result)
                print(f"{label}: {result['seconds']:.2f}s exit={result['exit_code']} timeout={result['timed_out']}", flush=True)
        identity["status_after"] = git(repo, "status", "--porcelain", "--untracked-files=all")
        identity["changed_by_benchmark"] = identity["status_after"] != identity["status_before"]
    (output_dir / "results.json").write_text(json.dumps(manifest, indent=2) + "\n")
    fields = ("repo_index", "case", "iteration", "cache_state", "seconds", "exit_code", "timed_out",
              "peak_rss_kib", "stdout_bytes", "stderr_bytes", *SOURCE_FIELDS)
    with (output_dir / "results.csv").open("w", newline="") as output:
        writer = csv.DictWriter(output, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        for row in manifest["measurements"]:
            writer.writerow({**row, **(row["sources"] or {})})
    if any(repo["changed_by_benchmark"] for repo in manifest["repositories"]):
        raise RuntimeError("benchmark changed a repository checkout; inspect results.json")
    return 0 if all(not row["timed_out"] and row["exit_code"] == 0 for row in manifest["measurements"]) else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="Grepple executable to benchmark")
    parser.add_argument("--repo", action="append", default=[], help="existing checkout root; repeatable")
    parser.add_argument("--url", action="append", default=[], help="HTTPS repository URL to shallow-clone into --output")
    parser.add_argument("--repo-list", type=Path, help="one local checkout or HTTPS URL per line")
    parser.add_argument("--sample", type=int, default=0, help="randomly choose N explicit repositories (0 = all)")
    parser.add_argument("--seed", type=int, default=42, help="reproducible repository sample seed")
    parser.add_argument("--case", action="append", help="NAME=Grepple arguments (shlex syntax), repeatable")
    parser.add_argument("--runs", type=int, default=2, help="one cold run plus warm repetitions")
    parser.add_argument("--timeout", type=float, default=60, help="seconds per command")
    parser.add_argument("--clone-timeout", type=float, default=120, help="seconds per clone")
    parser.add_argument("--output", required=True, type=Path, help="new directory for results/cache/clones")
    parser.add_argument("--keep-output", action="store_true", help="retain command stdout/stderr files")
    args = parser.parse_args()
    try:
        return run(args)
    except (OSError, ValueError, subprocess.CalledProcessError, subprocess.TimeoutExpired, RuntimeError) as error:
        parser.exit(2, f"benchmark: {error}\n")


if __name__ == "__main__":
    sys.exit(main())