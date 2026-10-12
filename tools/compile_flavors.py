#!/usr/bin/env python3
"""Compile the Forever database from canonical source snapshots."""

from __future__ import annotations

import argparse
import subprocess
from pathlib import Path


FLAVORS = ("forever",)


def compile_flavors(data_root: Path) -> list[str]:
    compiled: list[str] = []
    for flavor in FLAVORS:
        flavor_root = data_root / flavor
        sources = flavor_root / "sources"
        if not sources.is_dir():
            print(f"Skipping {flavor} compilation: no source directory at {sources}")
            continue
        subprocess.run(
            [
                "go",
                "run",
                "./cmd/database",
                "compile",
                "--flavor",
                flavor,
                "--sources",
                str(sources),
                "--out",
                str(flavor_root / "compilation"),
            ],
            check=True,
        )
        compiled.append(flavor)
    return compiled


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path("data"))
    args = parser.parse_args()
    compile_flavors(args.root)


if __name__ == "__main__":
    main()
