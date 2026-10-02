#!/usr/bin/env python3
"""Compile deterministic per-zone quest JSON files."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TOOLS = Path(__file__).resolve().parent
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

from quest_db.compile_zones import (
    ZoneCompileError,
    build_zone_bundles,
    check_zone_bundles,
    write_zone_bundles,
)
from quest_db.questie_import import render_collections


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="forever", help="Game data version to compile")
    parser.add_argument(
        "--zone",
        action="append",
        default=None,
        help="Compile only this plain zone slug, such as durotar (repeatable)",
    )
    parser.add_argument("--output", type=Path, help="Alternate compiled zone directory")
    parser.add_argument(
        "--check",
        action="store_true",
        help="Validate committed files without writing changes",
    )
    args = parser.parse_args(argv)

    version_root = ROOT / "data" / args.version
    raw_root = version_root / "raw"
    output_dir = args.output or version_root / "compiled" / "zones"
    collection_dir = version_root / "compiled" / "collections"
    try:
        rendered = build_zone_bundles(raw_root, args.version, args.zone)
        collections = render_collections(raw_root) if args.zone is None and args.version == "forever" else {}
        if args.check:
            differences = check_zone_bundles(
                output_dir,
                rendered,
                check_all=args.zone is None,
            )
            if collections and args.output is None:
                differences.extend(check_zone_bundles(collection_dir, collections, check_all=True))
            if differences:
                for difference in differences:
                    print(difference, file=sys.stderr)
                return 1
            print(f"Validated {len(rendered)} compiled zone file(s)")
            return 0
        write_zone_bundles(output_dir, rendered, replace_all=args.zone is None)
        if collections and args.output is None:
            write_zone_bundles(collection_dir, collections, replace_all=True)
    except ZoneCompileError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    print(f"Wrote {len(rendered)} compiled zone file(s) to {output_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
