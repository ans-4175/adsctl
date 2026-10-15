#!/usr/bin/env python3
"""Exercise the real installer using local release fixtures, without network access."""

import hashlib
import io
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parent.parent
BASE = "https://github.com/imrenagicom/adsctl/releases"
VERSION = "9.8.7"
BINARY = b'''#!/bin/sh
if [ "$1" = config ]; then
  printf '{"ok":true,"data":{"path":"%s/config.yaml"}}\\n' "$ADSCTL_HOME"
else
  printf '%s\\n' '{"ok":true,"data":{"version":"9.8.7"}}'
fi
'''


class InstallerTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="adsctl-installer-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.target = self.root / "user's adsctl home"
        self.fixtures = self.root / "fixtures"
        self.fixtures.mkdir()
        tools = self.root / "tools"
        tools.mkdir()
        curl = tools / "curl"
        curl.write_text(f"#!{sys.executable}\n" + '''import os, pathlib, shutil, sys
args = sys.argv[1:]
url = args[-1]
base = "https://github.com/imrenagicom/adsctl/releases"
assert url.startswith(base + "/"), "Unexpected download host"
fixtures = pathlib.Path(os.environ["ADSCTL_INSTALL_TEST_FIXTURES"])
with (fixtures / "requests").open("a") as log:
    log.write(url + "\\n")
if url == base + "/latest":
    print(base + "/tag/v9.8.7", end="")
else:
    if os.environ.get("ADSCTL_INSTALL_TEST_FAIL_DOWNLOAD"):
        sys.exit(22)
    assert "/download/v9.8.7/" in url, "Unexpected release"
    target = args[args.index("--output") + 1]
    shutil.copyfile(fixtures / url.rsplit("/", 1)[1], target)
''')
        curl.chmod(0o755)
        uname = tools / "uname"
        uname.write_text('''#!/bin/sh
case "$1" in
  -s) printf '%s\\n' "$ADSCTL_INSTALL_TEST_OS" ;;
  -m) printf '%s\\n' "$ADSCTL_INSTALL_TEST_ARCH" ;;
esac
''')
        uname.chmod(0o755)
        self.env = os.environ.copy()
        self.env.update({
            "ADSCTL_HOME": str(self.target),
            "ADSCTL_VERSION": "latest",
            "ADSCTL_INSTALL_TEST_FIXTURES": str(self.fixtures),
            "ADSCTL_INSTALL_TEST_OS": "Linux",
            "ADSCTL_INSTALL_TEST_ARCH": "x86_64",
            "PATH": str(tools) + os.pathsep + self.env["PATH"],
        })
        self.archive("linux", "amd64")

    def archive(self, system, arch, binary=None, symlink=False):
        if binary is None:
            binary = BINARY
        path = self.fixtures / f"adsctl_{VERSION}_{system}_{arch}.tar.gz"
        with tarfile.open(path, "w:gz") as archive:
            member = tarfile.TarInfo("adsctl")
            member.mode = 0o755
            if symlink:
                member.type = tarfile.SYMTYPE
                member.linkname = "/etc/passwd"
                archive.addfile(member)
            else:
                member.size = len(binary)
                archive.addfile(member, io.BytesIO(binary))
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        (self.fixtures / "checksums.txt").write_text(f"{digest}  {path.name}\n")
        return path

    def run_install(self, *args, success=True):
        result = subprocess.run(["sh", str(ROOT / "install.sh"), *args],
                                env=self.env, cwd=self.root, text=True,
                                capture_output=True, timeout=20)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def existing(self):
        (self.target / "bin").mkdir(parents=True)
        (self.target / "bin" / "adsctl").write_bytes(b"previous executable")
        (self.target / "config.yaml").write_text("version: 1\nprofiles: {}\n# keep me\n")

    def test_fresh_install_and_source_with_quoted_path(self):
        result = self.run_install()
        self.assertIn("Add adsctl to PATH", result.stdout)
        self.assertEqual((self.target / "config.yaml").stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.target / "bin" / "adsctl").stat().st_mode & 0o777, 0o755)
        self.assertTrue((self.target / "secrets").is_dir())
        self.assertTrue((self.target / "state").is_dir())
        sourced = subprocess.run(["sh", "-c",
                                  '. "$1"; . "$1"; printf "%s\\n%s\\n" "$ADSCTL_HOME" "$PATH"; adsctl version',
                                  "installer-test", str(self.target / "env")],
                                 env=self.env, text=True, capture_output=True, check=True)
        lines = sourced.stdout.splitlines()
        self.assertEqual(lines[0], str(self.target))
        self.assertEqual(lines[1].split(os.pathsep).count(str(self.target / "bin")), 1)
        self.assertTrue(json.loads(lines[2])["ok"])
        source_line = next(line.strip() for line in result.stdout.splitlines()
                           if line.startswith("  . ") and "env" in line)
        printed = subprocess.run(["sh", "-c", source_line + "; adsctl version"],
                                 env=self.env, text=True, capture_output=True, check=True)
        self.assertTrue(json.loads(printed.stdout)["ok"])
        startup_lines = [line.strip() for line in result.stdout.splitlines()
                         if line.startswith("  printf ") and " >> " in line]
        if startup_lines:
            # Exercise the printed append command against a private test profile.
            # Never execute its original redirect into the user's startup file.
            profile = self.root / "test shell profile"
            command = startup_lines[0].rpartition(" >> ")[0]
            subprocess.run(["sh", "-c", command + " >> " + shlex.quote(str(profile))],
                           env=self.env, check=True)
            self.assertEqual(profile.read_text().strip(), source_line)

    def test_reinstall_preserves_configuration_secrets_and_state(self):
        self.run_install()
        config = self.target / "config.yaml"
        config.write_text("version: 1\nprofiles: {}\n# existing configuration\n")
        secret = self.target / "secrets" / "meta-token"
        secret.write_text("local test fixture")
        state = self.target / "state" / "run.json"
        state.write_text('{"status":"paused"}')
        self.archive("linux", "amd64", BINARY + b"# replacement\n")
        self.run_install()
        self.assertIn("existing configuration", config.read_text())
        self.assertEqual(secret.read_text(), "local test fixture")
        self.assertEqual(state.read_text(), '{"status":"paused"}')
        self.assertIn(b"replacement", (self.target / "bin" / "adsctl").read_bytes())

    def test_bad_checksum_preserves_existing_installation(self):
        self.existing()
        (self.fixtures / "checksums.txt").write_text("0" * 64 + f"  adsctl_{VERSION}_linux_amd64.tar.gz\n")
        result = self.run_install(success=False)
        self.assertIn("Checksum mismatch", result.stderr)
        self.assertEqual((self.target / "bin" / "adsctl").read_bytes(), b"previous executable")
        self.assertIn("keep me", (self.target / "config.yaml").read_text())

    def test_download_failure_preserves_existing_installation(self):
        self.existing()
        self.env["ADSCTL_INSTALL_TEST_FAIL_DOWNLOAD"] = "1"
        self.run_install(success=False)
        self.assertEqual((self.target / "bin" / "adsctl").read_bytes(), b"previous executable")

    def test_pinned_version_does_not_resolve_latest(self):
        self.run_install("--version", "v9.8.7")
        self.assertNotIn(BASE + "/latest", (self.fixtures / "requests").read_text())

    def test_darwin_arm64_selection(self):
        self.env["ADSCTL_INSTALL_TEST_OS"] = "Darwin"
        self.env["ADSCTL_INSTALL_TEST_ARCH"] = "arm64"
        self.archive("darwin", "arm64")
        self.run_install()
        self.assertIn("darwin_arm64.tar.gz", (self.fixtures / "requests").read_text())

    def test_unsupported_architecture_stops_before_download(self):
        self.env["ADSCTL_INSTALL_TEST_ARCH"] = "sparc"
        self.run_install(success=False)
        self.assertFalse((self.fixtures / "requests").exists())
        self.assertFalse(self.target.exists())

    def test_invalid_version_stops_before_download(self):
        self.run_install("--version", "../../invalid", success=False)
        self.assertFalse((self.fixtures / "requests").exists())

    def test_symlink_binary_is_rejected(self):
        self.archive("linux", "amd64", symlink=True)
        self.run_install(success=False)
        self.assertFalse(self.target.exists())

    def test_nonworking_binary_is_rejected_before_replacing_existing(self):
        self.existing()
        self.archive("linux", "amd64", b"#!/bin/sh\nexit 42\n")
        self.run_install(success=False)
        self.assertEqual((self.target / "bin" / "adsctl").read_bytes(), b"previous executable")

    def test_release_without_adsctl_home_support_is_rejected(self):
        self.existing()
        self.archive("linux", "amd64", b'#!/bin/sh\nprintf \'%s\\n\' \'{"ok":true,"data":{"path":"/legacy/config.yaml"}}\'\n')
        result = self.run_install(success=False)
        self.assertIn("predates ADSCTL_HOME support", result.stderr)
        self.assertEqual((self.target / "bin" / "adsctl").read_bytes(), b"previous executable")

    def test_executable_directory_is_not_treated_as_an_installation(self):
        destination = self.target / "bin" / "adsctl"
        destination.mkdir(parents=True)
        self.run_install(success=False)
        self.assertEqual(list(destination.iterdir()), [])

    def test_shell_setup_directory_does_not_replace_existing_binary(self):
        self.existing()
        (self.target / "env").mkdir()
        self.run_install(success=False)
        self.assertEqual((self.target / "bin" / "adsctl").read_bytes(), b"previous executable")


if __name__ == "__main__":
    unittest.main()
