#!/usr/bin/env python3
"""Package validated extracted DB2 JSONL tables as a deterministic gzip snapshot."""
from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import shutil
import tempfile
from pathlib import Path

# Content-decryption keys are not part of the published game-data snapshot.
EXCLUDED_TABLES = {"TactKey": "Contains content-decryption keys; retained only in the local extraction."}


def package_snapshot(input_dir: Path, output_dir: Path) -> None:
    if output_dir.resolve().is_relative_to(input_dir.resolve()) or input_dir.resolve().is_relative_to(output_dir.resolve()):
        raise ValueError("input and output directories must be separate")
    if output_dir.exists() and not output_dir.is_dir():
        raise ValueError("output must be a directory")
    manifest_bytes = (input_dir / "manifest.json").read_bytes()
    manifest = json.loads(manifest_bytes)
    if manifest.get("schemaVersion") != 1 or manifest.get("gameVersion") != "forever" or manifest.get("source") != "foreverclient":
        raise ValueError("unsupported extraction manifest")
    output_dir.parent.mkdir(parents=True, exist_ok=True)
    temporary = Path(tempfile.mkdtemp(prefix=".client-snapshot-", dir=output_dir.parent))
    files = {}
    seen_tables = set()
    try:
        for table in manifest["tables"]:
            name = table["name"]
            if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9_-]+", name) or name in seen_tables:
                raise ValueError(f"invalid or duplicate table name: {name!r}")
            seen_tables.add(name)
            if table["status"] not in {"written", "partial_encrypted"} or name in EXCLUDED_TABLES:
                continue
            path = input_dir / (name + ".jsonl")
            if not path.resolve().is_relative_to(input_dir.resolve()):
                raise ValueError(f"table escapes input directory: {name}")
            digest = hashlib.sha256()
            ids = set()
            with path.open("rb") as source, (temporary / (name + ".jsonl.gz")).open("wb") as destination:
                with gzip.GzipFile(filename="", mode="wb", fileobj=destination, compresslevel=6, mtime=0) as compressed:
                    for line_number, raw in enumerate(source, 1):
                        value = json.loads(raw)
                        if line_number == 1:
                            expected = {
                                "kind": "table", "schemaVersion": 1, "gameVersion": "forever", "source": "foreverclient",
                                "table": name, "fileDataId": table["fileDataId"], "clientBuild": manifest["client"]["buildNumber"],
                                "definitionSha256": table["definitionSha256"],
                            }
                            if any(value.get(key) != item for key, item in expected.items()):
                                raise ValueError(f"{name}: header differs from manifest")
                        else:
                            row_id = value.get("id")
                            if value.get("kind") != "record" or type(row_id) is not int or not isinstance(value.get("fields"), dict) or row_id in ids:
                                raise ValueError(f"{name}:{line_number}: invalid or duplicate record")
                            ids.add(row_id)
                        digest.update(raw)
                        compressed.write(raw)
            if not path.stat().st_size or len(ids) != table.get("recordCount", 0):
                raise ValueError(f"{name}: record count differs from manifest")
            files[name + ".jsonl.gz"] = {"recordsSha256": digest.hexdigest(), "recordCount": len(ids)}
        if not files:
            raise ValueError("extraction has no publishable table files")
        (temporary / "manifest.json").write_bytes(manifest_bytes)
        index = {"schemaVersion": 1, "manifestSha256": hashlib.sha256(manifest_bytes).hexdigest(), "files": files,
                 "excludedTables": {name: reason for name, reason in EXCLUDED_TABLES.items() if name in seen_tables}}
        (temporary / "files.json").write_text(json.dumps(index, sort_keys=True, indent=2) + "\n", encoding="utf-8")
        old = output_dir.with_name(output_dir.name + ".old")
        if old.exists():
            raise ValueError(f"previous snapshot backup already exists: {old}")
        if output_dir.exists():
            os.replace(output_dir, old)
        try:
            os.replace(temporary, output_dir)
        except OSError:
            if old.exists():
                os.replace(old, output_dir)
            raise
        if old.exists():
            shutil.rmtree(old)
        print(f"Packaged {len(files)} tables and {sum(v['recordCount'] for v in files.values())} records into {output_dir}")
    finally:
        if temporary.exists():
            shutil.rmtree(temporary)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, default=Path(".local/foreverclient"))
    parser.add_argument("--out", type=Path, default=Path("data/forever/client-extract"))
    args = parser.parse_args()
    try:
        package_snapshot(args.input, args.out)
    except (OSError, ValueError, KeyError, TypeError) as exc:
        parser.exit(1, f"client-snapshot: {exc}\n")


if __name__ == "__main__":
    main()
