#!/usr/bin/env python3
"""Select changed generated directories for the database update pull request."""

from __future__ import annotations

import subprocess
import sys


GENERATED_PATHS = (
    "data/era/sources",
    "data/era/compilation",
    "data/forever/sources",
    "data/forever/compilation",
    "export/forever",
)


def changed_generated_paths(status: bytes) -> list[str]:
    changed = [entry[3:] for entry in status.split(b"\0") if len(entry) >= 4]
    return [
        path
        for path in GENERATED_PATHS
        if any(
            changed_path == path.encode() or changed_path.startswith(path.encode() + b"/")
            for changed_path in changed
        )
    ]


def main() -> int:
    try:
        status = subprocess.run(
            ["git", "status", "--porcelain=v1", "-z", "--untracked-files=all"],
            check=True,
            capture_output=True,
        ).stdout
    except subprocess.CalledProcessError as exc:
        print(f"changed-generated-paths: git status failed: {exc}", file=sys.stderr)
        return 1

    paths = changed_generated_paths(status)
    if not paths:
        print("changed-generated-paths: no generated paths are changed", file=sys.stderr)
        return 1

    print("paths<<EOF")
    print("\n".join(paths))
    print("EOF")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
