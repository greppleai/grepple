"""Run with: PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_benchmark_repositories.py'."""

import csv
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("benchmark-repositories.py")
spec = importlib.util.spec_from_file_location("benchmark_repositories", SCRIPT)
benchmark = importlib.util.module_from_spec(spec)
spec.loader.exec_module(benchmark)


class RepositoryBenchmarkTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repo = self.root / "checkout"
        self.repo.mkdir()
        for argv in (("git", "init", "-q"), ("git", "config", "user.name", "Benchmark"),
                     ("git", "config", "user.email", "bench@example.test")):
            subprocess.run(argv, cwd=self.repo, check=True)
        (self.repo / "README.md").write_text("fixture\n")
        subprocess.run(["git", "add", "README.md"], cwd=self.repo, check=True)
        subprocess.run(["git", "commit", "-qm", "fixture"], cwd=self.repo, check=True)
        self.binary = self.root / "fake-grepple"
        self.binary.write_text("""#!/usr/bin/env python3
import os, pathlib, sys, time
if 'stall' in sys.argv:
    time.sleep(10)
else:
    cache = pathlib.Path(os.environ['GREPPLE_NAVIGATION_CACHE_DIR'])
    cache.mkdir(parents=True, exist_ok=True)
    print('cache=' + str((cache / 'visited').exists()))
    (cache / 'visited').write_text('yes')
    print('sources=discovered:7,selected:4,parsed:4,skipped:3,failed:0,recovered:0')
""")
        self.binary.chmod(0o755)

    def test_random_selection_and_invalid_case(self):
        self.assertEqual(benchmark.select_repositories(["a", "b", "c"], 2, 42),
                         benchmark.select_repositories(["a", "b", "c"], 2, 42))
        with self.assertRaises(ValueError):
            benchmark.case_from_text("no-command=")
        with self.assertRaises(ValueError):
            benchmark.select_repositories(["a"], 2, 42)

    def test_json_source_counts_and_incomplete_preview(self):
        preview = '{"sources": {"failed": 0, "parsed": 4, "selected": 4, "recovered": 1, "skipped": 3, "discovered": 7}}'
        self.assertEqual(benchmark.parse_sources(preview), dict(discovered=7, selected=4, parsed=4,
                                                                skipped=3, failed=0, recovered=1))
        self.assertIsNone(benchmark.parse_sources('{"sources": {"discovered": 7'))

    def test_manifest_cache_and_checkout_unchanged(self):
        output = self.root / "results"
        completed = subprocess.run([sys.executable, str(SCRIPT), "--binary", str(self.binary),
                                    "--repo", str(self.repo), "--case", "sample=simulate .",
                                    "--runs", "2", "--output", str(output)], capture_output=True, text=True)
        self.assertEqual(completed.returncode, 0, completed.stderr)
        report = json.loads((output / "results.json").read_text())
        self.assertEqual(len(report["measurements"]), 2)
        self.assertEqual([row["cache_state"] for row in report["measurements"]], ["cold", "warm"])
        self.assertEqual([row["sources"]["parsed"] for row in report["measurements"]], [4, 4])
        self.assertTrue(all(row["peak_rss_kib"] is None or row["peak_rss_kib"] > 0
                            for row in report["measurements"]))
        self.assertEqual(report["repositories"][0]["revision"], benchmark.git(self.repo, "rev-parse", "HEAD"))
        self.assertFalse(report["repositories"][0]["changed_by_benchmark"])
        self.assertTrue((output / "results.csv").is_file())
        with (output / "results.csv").open(newline="") as stream:
            self.assertEqual([row["parsed"] for row in csv.DictReader(stream)], ["4", "4"])
        self.assertFalse(any(self.repo.glob(".grepple/**")))

    def test_timeout_recorded_without_checkout_mutation(self):
        output = self.root / "timed-out"
        output.mkdir()
        result = benchmark.invoke(self.binary, self.repo, ["stall"], output / "cache",
                                  0.05, output, "stall", False)
        self.assertTrue(result["timed_out"])
        self.assertIsNone(result["exit_code"])
        self.assertEqual(benchmark.git(self.repo, "status", "--porcelain"), "")


if __name__ == "__main__":
    unittest.main()