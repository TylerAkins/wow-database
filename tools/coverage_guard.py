#!/usr/bin/env python3
"""Fail when a source snapshot loses more records than configured."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path
from typing import Any


def check_retention(previous: int, current: int, minimum: float) -> bool:
    if previous <= 0:
        return True
    return current / previous >= minimum


def manifest_dirs(base: str) -> set[str]:
    paths = subprocess.run(
        ["git", "ls-tree", "-r", "--name-only", base, "--", "data/era/sources", "data/forever/sources"],
        check=True, capture_output=True, text=True,
    ).stdout.splitlines()
    return {str(Path(path).parent) for path in paths if path.endswith("/manifest.json")}


def guard(base: str, config_path: Path) -> list[str]:
    config: dict[str, Any] = json.loads(config_path.read_text(encoding="utf-8"))
    minimum = config.get("minimumSourceRecordRetention")
    if not isinstance(minimum, (int, float)) or isinstance(minimum, bool) or not 0 < minimum <= 1:
        raise ValueError("minimumSourceRecordRetention must be a number in (0, 1]")
    current_paths = {path.parent.as_posix(): path for path in Path("data").glob("*/sources/*/manifest.json")}
    failures = []
    for directory in sorted(manifest_dirs(base) & current_paths.keys()):
        old_bytes = subprocess.run(["git", "show", f"{base}:{directory}/manifest.json"], check=True, capture_output=True).stdout
        old_count = json.loads(old_bytes).get("recordCount")
        new_count = json.loads(current_paths[directory].read_text(encoding="utf-8")).get("recordCount")
        if not isinstance(old_count, int) or old_count < 0 or not isinstance(new_count, int) or new_count < 0:
            raise ValueError(f"{directory}: manifest recordCount must be a nonnegative integer")
        if not check_retention(old_count, new_count, float(minimum)):
            failures.append(f"{directory}: record count fell from {old_count} to {new_count}, below {minimum:.0%} retention")
    return failures


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="origin/main")
    parser.add_argument("--config", type=Path, default=Path("schemas/validation.json"))
    args = parser.parse_args()
    try:
        failures = guard(args.base, args.config)
    except (OSError, ValueError, json.JSONDecodeError, subprocess.CalledProcessError) as exc:
        print(f"coverage-guard: {exc}", file=sys.stderr)
        return 1
    if failures:
        print("coverage-guard: source coverage regression requires review", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1
    print("Source record counts passed the configured retention threshold.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
