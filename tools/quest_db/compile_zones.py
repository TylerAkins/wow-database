"""Compile lossless per-zone quest bundles from canonical source snapshots."""

from __future__ import annotations

import json
import shutil
import tempfile
from collections.abc import Iterable, Sequence
from pathlib import Path
from typing import Any

from .sources import SOURCE_PAGES, SourcePage

SCHEMA_VERSION = 1


class ZoneCompileError(ValueError):
    """Raised when canonical inputs cannot produce a valid zone bundle."""


def build_zone_bundles(
    raw_root: Path,
    game_version: str,
    zone_slugs: Sequence[str] | None = None,
    *,
    source_pages: Iterable[SourcePage] = SOURCE_PAGES,
) -> dict[str, str]:
    """Return output filename to deterministic JSON content for selected zones."""
    pages = _zone_pages(source_pages)
    selected = _select_pages(pages, zone_slugs)
    quest_index = _read_object(raw_root / "quest_index.json", "quest index")

    rendered: dict[str, str] = {}
    for page in selected:
        plain_slug = _plain_slug(page.slug)
        source_path = raw_root / "sources" / f"{page.slug.replace('/', '_')}.json"
        source = _read_object(source_path, f"zone source {page.slug}")
        _validate_source(source, page)
        rows = source.get("quests")
        if not isinstance(rows, list):
            raise ZoneCompileError(f"{source_path}: quests must be an array")
        if source.get("questCount") != len(rows):
            raise ZoneCompileError(
                f"{source_path}: questCount {source.get('questCount')!r} "
                f"does not match {len(rows)} rows"
            )

        zone_ids: set[int] = set()
        quest_ids: set[int] = set()
        quests: dict[str, Any] = {}
        for row in rows:
            if not isinstance(row, dict):
                raise ZoneCompileError(f"{source_path}: each quest row must be an object")
            quest_id = row.get("id")
            if not isinstance(quest_id, int) or isinstance(quest_id, bool) or quest_id <= 0:
                raise ZoneCompileError(f"{source_path}: malformed quest id {quest_id!r}")
            if quest_id in quest_ids:
                raise ZoneCompileError(f"{source_path}: duplicate quest id {quest_id}")
            quest_ids.add(quest_id)

            zone_id = row.get("category")
            if not isinstance(zone_id, int) or isinstance(zone_id, bool) or zone_id <= 0:
                raise ZoneCompileError(
                    f"{source_path}: quest {quest_id} has malformed zone id {zone_id!r}"
                )
            zone_ids.add(zone_id)

            key = str(quest_id)
            index_record = quest_index.get(key)
            if not isinstance(index_record, dict):
                raise ZoneCompileError(f"{source_path}: quest {quest_id} is missing from quest_index.json")
            if index_record.get("id") != quest_id:
                raise ZoneCompileError(
                    f"{source_path}: quest index record {key} has id {index_record.get('id')!r}"
                )

            detail_path = raw_root / "details" / f"{quest_id}.json"
            detail = _read_object(detail_path, f"quest detail {quest_id}")
            if detail.get("questId") != quest_id:
                raise ZoneCompileError(
                    f"{detail_path}: questId {detail.get('questId')!r} does not match {quest_id}"
                )
            quests[key] = {"index": index_record, "detail": detail}

        if len(zone_ids) != 1:
            raise ZoneCompileError(
                f"{source_path}: expected one zone id, found {sorted(zone_ids)}"
            )

        ordered_quests = {str(quest_id): quests[str(quest_id)] for quest_id in sorted(quest_ids)}
        bundle = {
            "schemaVersion": SCHEMA_VERSION,
            "gameVersion": game_version,
            "zone": {
                "slug": plain_slug,
                "sourceSlug": page.slug,
                "sourceUrl": page.url,
                "sourceFetchedAt": source.get("fetchedAt"),
                "zoneId": next(iter(zone_ids)),
            },
            "questCount": len(ordered_quests),
            "quests": ordered_quests,
        }
        rendered[f"{plain_slug}.json"] = json.dumps(
            bundle,
            indent=2,
            ensure_ascii=False,
        ) + "\n"
    return rendered


def write_zone_bundles(
    output_dir: Path,
    rendered: dict[str, str],
    *,
    replace_all: bool,
) -> None:
    """Atomically replace the output directory after every file is staged."""
    if output_dir.exists() and not output_dir.is_dir():
        raise ZoneCompileError(f"Output path is not a directory: {output_dir}")
    output_dir.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix=f".{output_dir.name}.stage-", dir=output_dir.parent))
    backup: Path | None = None
    try:
        if output_dir.is_dir() and not replace_all:
            shutil.copytree(output_dir, stage, dirs_exist_ok=True)
        for filename, content in rendered.items():
            (stage / filename).write_text(content, encoding="utf-8")

        if output_dir.exists():
            backup = Path(tempfile.mkdtemp(prefix=f".{output_dir.name}.backup-", dir=output_dir.parent))
            backup.rmdir()
            output_dir.rename(backup)
        try:
            stage.rename(output_dir)
        except Exception:
            if backup is not None and backup.exists() and not output_dir.exists():
                backup.rename(output_dir)
            raise
        if backup is not None:
            shutil.rmtree(backup)
    finally:
        if stage.exists():
            shutil.rmtree(stage)


def check_zone_bundles(
    output_dir: Path,
    rendered: dict[str, str],
    *,
    check_all: bool,
) -> list[str]:
    """Return human-readable differences without changing the output directory."""
    differences: list[str] = []
    expected_names = set(rendered)
    actual_names = (
        {path.name for path in output_dir.glob("*.json")}
        if output_dir.is_dir()
        else set()
    )
    if check_all:
        for name in sorted(actual_names - expected_names):
            differences.append(f"unexpected compiled file: {name}")
    for name, expected in rendered.items():
        path = output_dir / name
        if not path.is_file():
            differences.append(f"missing compiled file: {name}")
        elif path.read_text(encoding="utf-8") != expected:
            differences.append(f"compiled file differs: {name}")
    return differences


def _zone_pages(source_pages: Iterable[SourcePage]) -> list[SourcePage]:
    pages = [page for page in source_pages if page.kind == "zone"]
    names: dict[str, str] = {}
    for page in pages:
        plain_slug = _plain_slug(page.slug)
        existing = names.get(plain_slug)
        if existing is not None:
            raise ZoneCompileError(
                f"Duplicate zone output name {plain_slug!r}: {existing!r} and {page.slug!r}"
            )
        names[plain_slug] = page.slug
    return sorted(pages, key=lambda page: _plain_slug(page.slug))


def _select_pages(
    pages: list[SourcePage],
    zone_slugs: Sequence[str] | None,
) -> list[SourcePage]:
    if not zone_slugs:
        return pages
    by_slug = {_plain_slug(page.slug): page for page in pages}
    unknown = sorted(set(zone_slugs) - set(by_slug))
    if unknown:
        raise ZoneCompileError(f"Unknown zone slug(s): {', '.join(unknown)}")
    return [by_slug[slug] for slug in dict.fromkeys(zone_slugs)]


def _plain_slug(source_slug: str) -> str:
    plain_slug = source_slug.rsplit("/", 1)[-1]
    if not plain_slug or plain_slug in {".", ".."}:
        raise ZoneCompileError(f"Malformed source slug: {source_slug!r}")
    return plain_slug


def _validate_source(source: dict[str, Any], page: SourcePage) -> None:
    if source.get("kind") != "zone":
        raise ZoneCompileError(f"Source {page.slug!r} is not marked as a zone")
    if source.get("slug") != page.slug:
        raise ZoneCompileError(
            f"Source slug {source.get('slug')!r} does not match {page.slug!r}"
        )
    if source.get("url") != page.url:
        raise ZoneCompileError(
            f"Source URL {source.get('url')!r} does not match {page.url!r}"
        )
    if not isinstance(source.get("fetchedAt"), str):
        raise ZoneCompileError(f"Source {page.slug!r} has no fetchedAt timestamp")


def _read_object(path: Path, label: str) -> dict[str, Any]:
    if not path.is_file():
        raise ZoneCompileError(f"Missing {label}: {path}")
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ZoneCompileError(f"Cannot read {label} {path}: {exc}") from exc
    if not isinstance(value, dict):
        raise ZoneCompileError(f"{path}: {label} must be a JSON object")
    return value
