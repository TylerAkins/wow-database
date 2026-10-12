#!/usr/bin/env python3
"""Source Forever database: agent runs `ingest --url` when the user pastes links."""

from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TOOLS = Path(__file__).resolve().parent
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

from quest_db.http import PageClient
from quest_db.ingest import ingest_html
from quest_db.sync import (
    backfill_eligibility,
    rebuild_zone_map,
    sync_object_details,
    sync_quest_details,
    sync_zone_starters,
    sync_sources,
)

DEFAULT_DATA_ROOT = ROOT / "data" / "forever" / "raw"
DEFAULT_CACHE = ROOT / ".cache" / "quest-html"


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    data_root = Path(args.data_root)
    data_root.mkdir(parents=True, exist_ok=True)

    if args.command == "ingest":
        return _cmd_ingest(args, data_root)

    client = _make_client(args)

    if args.command == "sync-sources":
        sync_sources(data_root, client, force=args.force)
        if args.rebuild_zone_map:
            rebuild_zone_map(data_root, _att_quests_path(args))
        return 0

    if args.command == "sync-quests":
        sync_quest_details(
            data_root,
            client,
            limit=args.limit,
            quest_ids=args.quest,
            force=args.force,
            attunement_seed_path=Path(args.attunement_seed) if args.attunement_seed else None,
        )
        if args.rebuild_zone_map:
            rebuild_zone_map(data_root, _att_quests_path(args))
        return 0

    if args.command == "sync-zones":
        sync_zone_starters(
            data_root,
            client,
            limit=args.limit,
            zone_ids=args.zone,
            force=args.force,
        )
        return 0

    if args.command == "sync-objects":
        sync_object_details(
            data_root,
            client,
            limit=args.limit,
            object_ids=args.object,
            item_ids=args.item,
            force=args.force,
        )
        return 0

    if args.command == "rebuild-zone-map":
        rebuild_zone_map(data_root, _att_quests_path(args))
        return 0

    if args.command == "backfill-eligibility":
        count = backfill_eligibility(data_root)
        print(f"updated {count} quest details")
        return 0

    raise SystemExit(f"Unknown command: {args.command}")


def _cmd_ingest(args: argparse.Namespace, data_root: Path) -> int:
    urls = list(args.url or [])
    if args.url_file:
        text = Path(args.url_file).read_text(encoding="utf-8")
        urls.extend(line.strip() for line in text.splitlines() if line.strip() and not line.strip().startswith("#"))

    if not urls:
        raise SystemExit("ingest requires --url and/or --url-file")

    client = _make_client(args)
    reports: list[dict] = []

    for index, url in enumerate(urls):
        if args.html_file and len(urls) == 1:
            html = Path(args.html_file).read_text(encoding="utf-8", errors="replace")
        else:
            html = client.get_html(url, force=args.force)
        report = ingest_html(data_root, url, html)
        reports.append(report)
        print(json.dumps(report, indent=2, sort_keys=True))
        if index + 1 < len(urls) and args.pause > 0:
            time.sleep(args.pause)

    if args.rebuild_zone_map:
        rebuild_zone_map(data_root, _att_quests_path(args))
    return 0


def _make_client(args: argparse.Namespace):
    if args.browser:
        # Playwright is optional so the Python HTTP commands still run without it.
        try:
            from quest_db.browser import BrowserPageClient
        except ImportError as exc:
            raise SystemExit("Browser fetch needs Playwright: pip3 install playwright") from exc

        print("Opening Chrome. Leave the window alone until this command finishes.", flush=True)
        return BrowserPageClient(
            cache_dir=Path(args.cache_dir),
            min_interval_s=args.delay,
            batch_size=args.batch_size,
            batch_pause_s=args.batch_pause,
        )
    return PageClient(
        cache_dir=Path(args.cache_dir),
        min_interval_s=args.delay,
        batch_size=args.batch_size,
        batch_pause_s=args.batch_pause,
    )


def _att_quests_path(args: argparse.Namespace) -> Path:
    if not args.att_quests:
        raise SystemExit("rebuild-zone-map requires --att-quests /path/to/ForeverQuests.lua")
    return Path(args.att_quests)


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "command",
        choices=(
            "ingest",
            "sync-sources",
            "sync-quests",
            "sync-objects",
            "sync-zones",
            "rebuild-zone-map",
            "backfill-eligibility",
        ),
        help="ingest: scan pasted URL(s); sync-*: bulk (optional)",
    )
    parser.add_argument(
        "--url",
        action="append",
        default=[],
        help="Source Forever URL to scan (repeatable)",
    )
    parser.add_argument("--url-file", help="Text file with one URL per line")
    parser.add_argument(
        "--html-file",
        help="Use saved HTML instead of fetching (single --url only)",
    )
    parser.add_argument("--data-root", default=str(DEFAULT_DATA_ROOT))
    parser.add_argument("--cache-dir", default=str(DEFAULT_CACHE))
    parser.add_argument(
        "--delay",
        type=float,
        default=1.25,
        help="Minimum seconds between uncached Source HTTP requests",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=0,
        help="After this many network fetches, wait --batch-pause seconds (0 disables)",
    )
    parser.add_argument(
        "--batch-pause",
        type=float,
        default=0.0,
        help="Extra seconds to wait after each batch of --batch-size fetches",
    )
    parser.add_argument(
        "--pause",
        type=float,
        default=1.5,
        help="Pause between multiple URLs in one ingest run",
    )
    parser.add_argument("--force", action="store_true", help="Ignore HTML cache / re-fetch")
    parser.add_argument(
        "--browser",
        action="store_true",
        help="Fetch with local Chrome instead of Python. Run this on your machine.",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=None,
        help="Max quest detail pages to fetch (sync-quests only)",
    )
    parser.add_argument(
        "--quest",
        action="append",
        type=int,
        default=None,
        help="Only this quest id (repeatable, sync-quests only)",
    )
    parser.add_argument(
        "--zone",
        action="append",
        type=int,
        default=None,
        help="Only these zone ids (repeatable, sync-zones). Example: 17 for the Barrens",
    )
    parser.add_argument(
        "--object",
        action="append",
        type=int,
        default=None,
        help="Only these object ids (repeatable). Omitting this fetches every object in object_index.json",
    )
    parser.add_argument(
        "--item",
        action="append",
        type=int,
        default=None,
        help="Also fetch these item ids (repeatable). Use for starters that are not in the object list",
    )
    parser.add_argument(
        "--rebuild-zone-map",
        action="store_true",
        help="After ingest/sync, refresh zone_ui_map_ids.json from an external quest pin file",
    )
    parser.add_argument(
        "--att-quests",
        default=None,
        help="External ForeverQuests.lua used only by rebuild-zone-map",
    )
    parser.add_argument(
        "--attunement-seed",
        default=None,
        help="JSON list of attunement quest IDs (sync-quests only)",
    )
    return parser.parse_args(argv)


if __name__ == "__main__":
    raise SystemExit(main())
