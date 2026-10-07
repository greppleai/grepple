"""Offline installer tests; never change the user's installation or shell profile."""
import hashlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        self.destination = self.root / "bin with spaces"
        self.destination.mkdir()
        self.old = self.destination / "grepple"
        self.old.write_text("old binary")
        self.home = self.root / "home"
        self.home.mkdir()
        self.profile = self.home / ".profile"
        self.profile.write_text("unchanged profile\n")
        self.archive = self.root / "fixture.tar.gz"
        self.make_archive()
        self.manifest = self.root / "checksums.txt"
        self.write_manifest()
        self.write_tool("uname", "#!/bin/sh\ncase \"$1\" in -s) echo \"$TEST_OS\";; -m) echo \"$TEST_ARCH\";; esac\n")
        self.write_tool("curl", f"#!{sys.executable}\n" + '''import os, shutil, sys
args = sys.argv[1:]
url = args[-1]
with open(os.environ["TEST_CALLS"], "a") as log:
    log.write(url + "\\n")
assert args[0] == "--disable"
assert args[args.index("--proto") + 1] == "=https"
assert args[args.index("--proto-redir") + 1] == "=https"
if os.environ.get("TEST_DOWNLOAD_FAIL"):
    sys.exit(22)
if url.endswith("/latest"):
    print("https://github.com/greppleai/grepple/releases/tag/" + os.environ.get("TEST_TAG", "v0.0.5"), end="")
else:
    source = "TEST_MANIFEST" if url.endswith("checksums.txt") else "TEST_ARCHIVE"
    shutil.copyfile(os.environ[source], args[args.index("--output") + 1])
''')
        self.env = dict(os.environ, HOME=str(self.home), PATH=str(self.tools) + os.pathsep + os.environ["PATH"],
                        GREPPLE_BIN_DIR=str(self.destination), TEST_OS="Linux", TEST_ARCH="x86_64",
                        TEST_ARCHIVE=str(self.archive), TEST_MANIFEST=str(self.manifest),
                        TEST_CALLS=str(self.root / "calls"))

    def write_tool(self, name, content):
        path = self.tools / name
        path.write_text(content)
        path.chmod(0o755)

    def make_archive(self, name="grepple", symlink=False):
        with tarfile.open(self.archive, "w:gz") as archive:
            item = tarfile.TarInfo(name)
            content = b"#!/bin/sh\necho grepple-fixture\n"
            item.mode = 0o755
            if symlink:
                item.type = tarfile.SYMTYPE
                item.linkname = "/etc/passwd"
                archive.addfile(item)
            else:
                item.size = len(content)
                archive.addfile(item, io.BytesIO(content))

    def write_manifest(self, os_name="linux", arch="amd64", checksum=None):
        digest = checksum or hashlib.sha256(self.archive.read_bytes()).hexdigest()
        self.manifest.write_text(f"{digest}  grepple_0.0.5_{os_name}_{arch}.tar.gz\n")

    def run_install(self, *args):
        return subprocess.run(["sh", str(INSTALLER), *args], env=self.env,
                              capture_output=True, text=True, timeout=10)

    def assert_preserved(self, result):
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertEqual(self.old.read_text(), "old binary")
        self.assertEqual(self.profile.read_text(), "unchanged profile\n")
        self.assertFalse(list(self.destination.glob(".grepple-install.*")))

    def test_linux_upgrade_with_spaces_and_path_guidance(self):
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Add", result.stdout)
        self.assertTrue(os.access(self.old, os.X_OK))
        self.assertEqual(self.profile.read_text(), "unchanged profile\n")
        self.assertIn("grepple_0.0.5_linux_amd64.tar.gz", (self.root / "calls").read_text())

    def test_mac_arm_and_custom_directory_on_path(self):
        custom = self.root / "custom"
        self.env.update(TEST_OS="Darwin", TEST_ARCH="arm64", PATH=self.env["PATH"] + os.pathsep + str(custom))
        self.write_manifest("darwin", "arm64")
        result = self.run_install("--bin-dir", str(custom))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Run: grepple --version", result.stdout)
        self.assertTrue((custom / "grepple").is_file())
        self.assertEqual(self.old.read_text(), "old binary")

    def test_other_supported_platforms(self):
        for os_name, machine, asset_os, asset_arch in (("Linux", "aarch64", "linux", "arm64"),
                                                      ("Darwin", "x86_64", "darwin", "amd64")):
            with self.subTest(os_name=os_name):
                self.env.update(TEST_OS=os_name, TEST_ARCH=machine)
                self.write_manifest(asset_os, asset_arch)
                result = self.run_install()
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_shasum_fallback_without_sha256sum(self):
        for command in ("sh", "mktemp", "awk", "tar", "gzip", "rm", "mkdir", "cp", "chmod", "mv", "shasum"):
            executable = shutil.which(command)
            if executable is None:
                self.skipTest(f"{command} unavailable")
            (self.tools / command).symlink_to(executable)
        self.env["PATH"] = str(self.tools)
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_checksum_failure_preserves_old_binary(self):
        self.write_manifest(checksum="0" * 64)
        self.assert_preserved(self.run_install())

    def test_missing_or_duplicate_manifest_entry(self):
        for manifest in ("", self.manifest.read_text() * 2):
            with self.subTest(manifest=manifest):
                self.manifest.write_text(manifest)
                self.assert_preserved(self.run_install())

    def test_bad_archive_paths_and_symlinks_are_rejected(self):
        for name, symlink in (("../outside", False), ("grepple", True)):
            with self.subTest(name=name, symlink=symlink):
                self.make_archive(name, symlink)
                self.write_manifest()
                self.assert_preserved(self.run_install())

    def test_network_failure_preserves_old_binary(self):
        self.env["TEST_DOWNLOAD_FAIL"] = "1"
        self.assert_preserved(self.run_install())

    def test_unsupported_os_arch_and_tag_are_rejected(self):
        for key, value in (("TEST_OS", "Windows_NT"), ("TEST_ARCH", "i686"), ("TEST_TAG", "v0.0.5/../bad")):
            with self.subTest(key=key):
                old_value = self.env.get(key)
                self.env[key] = value
                self.assert_preserved(self.run_install())
                if old_value is None:
                    del self.env[key]
                else:
                    self.env[key] = old_value

    def test_destination_symlink_is_not_followed_or_replaced(self):
        self.old.unlink()
        self.old.symlink_to(self.profile)
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(self.old.is_symlink())
        self.assertEqual(self.profile.read_text(), "unchanged profile\n")

    def test_help_and_unknown_arguments_do_not_download(self):
        self.assertEqual(self.run_install("--help").returncode, 0)
        self.assert_preserved(self.run_install("--bin-dir"))
        self.assert_preserved(self.run_install("--unexpected"))
        self.assertFalse((self.root / "calls").exists())


if __name__ == "__main__":
    unittest.main()
