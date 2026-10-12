import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tools.compile_flavors import compile_flavors


class CompileFlavorsTests(unittest.TestCase):
    def test_skips_flavors_without_source_directories(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "data"
            (root / "forever" / "sources" / "allthethings").mkdir(parents=True)
            calls_file = Path(temp) / "go-calls.txt"
            fake_bin = Path(temp) / "bin"
            fake_bin.mkdir()
            fake_go = fake_bin / "go"
            fake_go.write_text(
                "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GO_CALLS_FILE\"\n",
                encoding="utf-8",
            )
            fake_go.chmod(0o755)
            with patch.dict(
                os.environ,
                PATH=f"{fake_bin}{os.pathsep}{os.environ.get('PATH', '')}",
                GO_CALLS_FILE=str(calls_file),
            ):
                compiled = compile_flavors(root)

            self.assertEqual(compiled, ["forever"])
            self.assertEqual(
                calls_file.read_text(encoding="utf-8").splitlines(),
                [
                    "run ./cmd/database compile --flavor forever "
                    f"--sources {root / 'forever' / 'sources'} "
                    f"--out {root / 'forever' / 'compilation'}"
                ],
            )

    def test_compiles_only_forever_when_era_directory_exists(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "data"
            (root / "era" / "sources").mkdir(parents=True)
            (root / "forever" / "sources").mkdir(parents=True)
            calls_file = Path(temp) / "go-calls.txt"
            fake_bin = Path(temp) / "bin"
            fake_bin.mkdir()
            fake_go = fake_bin / "go"
            fake_go.write_text(
                "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$GO_CALLS_FILE\"\n",
                encoding="utf-8",
            )
            fake_go.chmod(0o755)
            env = dict(
                os.environ,
                PATH=f"{fake_bin}{os.pathsep}{os.environ.get('PATH', '')}",
                GO_CALLS_FILE=str(calls_file),
            )
            result = subprocess.run(
                ["python3", "tools/compile_flavors.py", "--root", str(root)],
                check=True,
                env=env,
                capture_output=True,
                text=True,
            )

            self.assertNotIn("Skipping", result.stdout)
            self.assertEqual(
                [line.split("--flavor ", 1)[1].split()[0] for line in calls_file.read_text(encoding="utf-8").splitlines()],
                ["forever"],
            )


if __name__ == "__main__":
    unittest.main()
